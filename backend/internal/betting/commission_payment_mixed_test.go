package betting

import (
	"context"
	"errors"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

func TestCommissionPaymentMixedOriginalModesRequireWholeCycleManualApproval(t *testing.T) {
	f := newCommissionBatchFixture(t)
	ctx := context.Background()
	policy, err := f.service.Policy(ctx, f.betting.brand)
	if err != nil {
		t.Fatal(err)
	}
	config := policy.Config
	config.PayoutMode = commission.PayoutAutomatic
	if err = commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, e := f.service.Update(ctx, tx, f.betting.brand, f.actor, commission.PolicyInput{Version: policy.Version, Config: config, Reason: "capture different mode only on a new genuine bet"}, commissionPaymentMeta(f.actor))
		return e
	}); err != nil {
		t.Fatal(err)
	}
	input := f.betting.input
	input.Selection = rules.Selection{Digits: [][]int{{0}, {0}, {0}}}
	order, err := placeBettingOrder(t, f.betting, input, "commission-mixed-new-automatic-order")
	if err != nil {
		t.Fatal(err)
	}
	if !order.PlacedAt.Before(f.boundary) {
		t.Fatal("new automatic bet missed original cycle boundary")
	}
	waitCommissionBoundary(t, f.boundary)
	cycle := createCommissionCycle(t, f)
	eligibilitySettle(t, f.betting)
	advanceCommissionWorker(t, f, 30)
	cycle = readCommissionCycle(t, f, cycle.ID)
	if cycle.State != "ready" || cycle.TotalPoints != "1" {
		t.Fatal("mixed-mode genuine bets did not finalize", cycle)
	}
	actor := commissionPaymentActor(f)
	gate := commissionPaymentPolicy(t, f)
	if err = updateCommissionPaymentPolicy(t, f, actor, true, gate.Version); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.ProcessPayments(ctx, 20); err != nil {
		t.Fatal(err)
	}
	page := commissionPayments(t, f)
	if len(page.Items) != 1 {
		t.Fatal("mixed cycle did not register exactly one decision", page)
	}
	p := page.Items[0]
	if p.State != "awaiting_approval" || p.PayoutMode != "mixed" || p.PaidCount != "0" || p.PaidPoints != "0" || p.LastErrorCode != nil {
		t.Fatal("mixed original snapshots must await whole-cycle manual review", p)
	}
	err = commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, e := f.service.RetryPaymentTx(ctx, tx, f.betting.brand, p.ID, actor,
			commission.RetryCycleInput{Version: p.Version, Reason: "retry cannot bypass required manual review"}, commissionPaymentMeta(actor))
		return e
	})
	if !errors.Is(err, commission.ErrPaymentState) {
		t.Fatalf("retry before mixed-cycle approval=%v", err)
	}
	var credits int
	if err = f.betting.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='commission'`).Scan(&credits); err != nil || credits != 0 {
		t.Fatal("mixed decision minted points", credits, err)
	}
	wallet, err := (points.Store{DB: f.betting.db}).Read(ctx, f.betting.brand, f.node.MemberID)
	if err != nil || wallet.CommissionPoints != 0 {
		t.Fatal("mixed decision mutated beneficiary wallet", wallet, err)
	}
	if steps, e := f.service.ProcessPayments(ctx, 20); e != nil || steps != 0 {
		t.Fatalf("mixed job advanced without manual approval: steps=%d err=%v", steps, e)
	}
	var approvalsBefore int
	if err = f.betting.db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='commission.payment.approve' AND resource_id=$1`, p.ID).Scan(&approvalsBefore); err != nil {
		t.Fatal(err)
	}
	// Exercise the database boundary directly with a structurally valid approval
	// audit, but a system actor. Service authorization is bypassed; the payment
	// trigger must still reject the transition. Roll back the audit and UPDATE.
	tx, err := f.betting.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	systemAudit, auditErr := audit.Append(ctx, tx, audit.Record{
		BrandID: f.betting.brand, ActorType: "system", Action: "commission.payment.approve",
		ResourceType: "commission_payment", ResourceID: p.ID,
		Reason: "test system actor cannot approve a mixed payout", RequestID: ids.New(),
		Before: map[string]any{"version": p.Version, "state": "awaiting_approval"},
		After:  map[string]any{"version": p.Version + 1, "state": "paying", "run_id": p.RunID, "error_code": nil},
	})
	if auditErr != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(auditErr)
	}
	_, directErr := tx.Exec(ctx, `UPDATE commission_payments SET state='paying',version=version+1,approved_by=$3,approval_actor_type='admin',approval_audit_log_id=$4,last_audit_log_id=$4,last_error_code=NULL,next_work_at=clock_timestamp() WHERE brand_id=$1 AND id=$2`,
		f.betting.brand, p.ID, actor.ID, systemAudit)
	if directErr == nil {
		_ = tx.Rollback(ctx)
		t.Fatal("database accepted a mixed payment approval with a system audit actor")
	}
	_ = tx.Rollback(ctx)
	if got := commissionPaymentRead(t, f, p.ID); got.State != "awaiting_approval" || got.Version != p.Version || got.PayoutMode != "mixed" {
		t.Fatalf("rejected direct SQL approval changed payment: %+v", got)
	}
	var approvalsAfter int
	if err = f.betting.db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='commission.payment.approve' AND resource_id=$1`, p.ID).Scan(&approvalsAfter); err != nil || approvalsAfter != approvalsBefore {
		t.Fatalf("rolled-back system approval audit count=%d before=%d err=%v", approvalsAfter, approvalsBefore, err)
	}
	if err = commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, e := f.service.ApprovePaymentTx(ctx, tx, f.betting.brand, p.ID, actor,
			commission.RetryCycleInput{Version: p.Version, Reason: "approve entire mixed-snapshot cycle"}, commissionPaymentMeta(actor))
		return e
	}); err != nil {
		t.Fatal(err)
	}
	approved := commissionPaymentRead(t, f, p.ID)
	if approved.State != "paying" || approved.PayoutMode != "mixed" || approved.Version != p.Version+1 {
		t.Fatalf("approval rewrote original mixed identity: %+v", approved)
	}
	if _, err = f.betting.db.Exec(ctx, `CREATE FUNCTION test_reject_mixed_commission_ledger() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.entry_type='commission' THEN RAISE EXCEPTION 'test mixed commission ledger outage'; END IF; RETURN NEW; END $$; CREATE TRIGGER test_reject_mixed_commission_ledger BEFORE INSERT ON point_ledger_entries FOR EACH ROW EXECUTE FUNCTION test_reject_mixed_commission_ledger()`); err != nil {
		t.Fatal(err)
	}
	if _, err = f.betting.db.Exec(ctx, `UPDATE commission_payments SET next_work_at=clock_timestamp() WHERE brand_id=$1 AND id=$2`, f.betting.brand, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.ProcessPayments(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if _, err = f.betting.db.Exec(ctx, `DROP TRIGGER test_reject_mixed_commission_ledger ON point_ledger_entries; DROP FUNCTION test_reject_mixed_commission_ledger()`); err != nil {
		t.Fatal(err)
	}
	failed := commissionPaymentRead(t, f, p.ID)
	if failed.State != "failed" || failed.PayoutMode != "mixed" || failed.PaidPoints != "0" || failed.PaidCount != "0" {
		t.Fatalf("failed mixed payout changed identity or credited early: %+v", failed)
	}
	if err = f.betting.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='commission'`).Scan(&credits); err != nil || credits != 0 {
		t.Fatalf("failed mixed payout left commission ledger entries=%d err=%v", credits, err)
	}
	if err = commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, e := f.service.RetryPaymentTx(ctx, tx, f.betting.brand, p.ID, actor,
			commission.RetryCycleInput{Version: failed.Version, Reason: "retry reviewed mixed payout after ledger recovery"}, commissionPaymentMeta(actor))
		return e
	}); err != nil {
		t.Fatal(err)
	}
	retrying := commissionPaymentRead(t, f, p.ID)
	if retrying.State != "paying" || retrying.PayoutMode != "mixed" || retrying.Version != failed.Version+1 {
		t.Fatalf("retry rewrote mixed payout identity: %+v", retrying)
	}
	for i := 0; i < 5; i++ {
		if _, err = f.service.ProcessPayments(ctx, 20); err != nil {
			t.Fatal(err)
		}
		if commissionPaymentRead(t, f, p.ID).State == "paid" {
			break
		}
		if _, err = f.betting.db.Exec(ctx, `UPDATE commission_payments SET next_work_at=clock_timestamp() WHERE id=$1`, p.ID); err != nil {
			t.Fatal(err)
		}
	}
	paid := commissionPaymentRead(t, f, p.ID)
	if paid.State != "paid" || paid.PaidPoints != "1" || paid.PaidCount != "1" || paid.PayoutMode != "mixed" {
		t.Fatalf("mixed payout after whole-cycle review=%+v", paid)
	}
	wallet, err = (points.Store{DB: f.betting.db}).Read(ctx, f.betting.brand, f.node.MemberID)
	if err != nil || wallet.CommissionPoints != 1 {
		t.Fatalf("approved mixed beneficiary wallet=%+v err=%v", wallet, err)
	}
	for i := 0; i < 3; i++ {
		if steps, e := f.service.ProcessPayments(ctx, 20); e != nil || steps != 0 {
			t.Fatalf("repeated mixed payout steps=%d err=%v", steps, e)
		}
	}
	if err = f.betting.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='commission'`).Scan(&credits); err != nil || credits != 1 {
		t.Fatalf("mixed payout duplicate ledger count=%d err=%v", credits, err)
	}

	// Correct a real settled order after payment. The changed evidence must block
	// the credited mixed payment, and neither approval nor retry can release it.
	correction := createFixtureCorrection(t, f.betting, correctedDigits(0, 0, 0))
	if correction.State != "reversing" {
		t.Fatalf("mixed payout correction started in state %q", correction.State)
	}
	if _, err = f.betting.service.ProcessCorrections(ctx, 20); err != nil {
		t.Fatal(err)
	}
	correction, err = f.betting.service.Correction(ctx, f.betting.brand, correction.ID)
	if err != nil || correction.State != "resettling" || correction.NewJobID == nil {
		t.Fatalf("mixed payout correction did not create replacement settlement: %+v err=%v", correction, err)
	}
	if _, err = f.betting.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	advanceCommissionWorker(t, f, 40)
	correctedCycle := readCommissionCycle(t, f, paid.CycleID)
	if correctedCycle.State != "ready" || !correctedCycle.EvidenceCurrent || correctedCycle.CurrentRunID == nil || *correctedCycle.CurrentRunID == paid.RunID {
		t.Fatalf("actual correction did not advance mixed payment evidence: %+v old_run=%s", correctedCycle, paid.RunID)
	}
	if _, err = f.betting.db.Exec(ctx, `UPDATE commission_payments SET next_work_at=clock_timestamp() WHERE brand_id=$1 AND id=$2`, f.betting.brand, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.ProcessPayments(ctx, 20); err != nil {
		t.Fatal(err)
	}
	blocked := commissionPaymentRead(t, f, p.ID)
	if blocked.State != "blocked" || blocked.PayoutMode != "mixed" || blocked.PaidPoints != "1" || blocked.PaidCount != "1" || blocked.LastErrorCode == nil || *blocked.LastErrorCode != "COMMISSION_PAYMENT_CORRECTION_REQUIRED" {
		t.Fatalf("actual correction did not keep credited mixed payment blocked: %+v", blocked)
	}
	for _, retry := range []bool{false, true} {
		err = commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
			in := commission.RetryCycleInput{Version: blocked.Version, Reason: "blocked corrected mixed payout cannot be reopened"}
			if retry {
				_, e := f.service.RetryPaymentTx(ctx, tx, f.betting.brand, p.ID, actor, in, commissionPaymentMeta(actor))
				return e
			}
			_, e := f.service.ApprovePaymentTx(ctx, tx, f.betting.brand, p.ID, actor, in, commissionPaymentMeta(actor))
			return e
		})
		if !errors.Is(err, commission.ErrPaymentState) {
			t.Fatalf("blocked corrected mixed payment retry=%t error=%v, want state conflict", retry, err)
		}
	}
	if got := commissionPaymentRead(t, f, p.ID); got.State != "blocked" || got.Version != blocked.Version || got.PaidPoints != "1" || got.PaidCount != "1" || got.PayoutMode != "mixed" {
		t.Fatalf("rejected blocked mixed payment actions changed it: %+v", got)
	}
	if err = f.betting.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='commission'`).Scan(&credits); err != nil || credits != 1 {
		t.Fatalf("blocked correction mixed payout changed commission credits=%d err=%v", credits, err)
	}
}
