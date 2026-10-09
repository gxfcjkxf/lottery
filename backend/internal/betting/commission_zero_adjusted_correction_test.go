package betting

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
)

func zeroOriginalManualOnlyFixture(t *testing.T) (commissionBatchFixture, commission.Payment, commission.Target, commission.Adjustment) {
	t.Helper()
	db := testdb.NewAtVersion(t, 73)
	f := newCommissionBatchFixtureFromBetting(t, newBettingFixtureWithDBWindow(t, db, storeTestBrand, 20*time.Second, 22*time.Second))
	ctx := context.Background()
	waitCommissionBoundary(t, f.boundary)
	c := createCommissionCycle(t, f)
	eligibilitySettle(t, f.betting)
	first := createFixtureCorrection(t, f.betting, correctedDigits(0, 0, 0))
	if _, err := f.betting.service.ProcessCorrections(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if got, err := f.betting.service.Correction(ctx, f.betting.brand, first.ID); err != nil || got.NewJobID == nil {
		t.Fatalf("genuine winning replacement: %+v %v", got, err)
	}
	if _, err := f.betting.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	advanceCommissionWorker(t, f, 40)
	c = readCommissionCycle(t, f, c.ID)
	if c.State != "ready" || c.TotalPoints != "0" || c.EarningCount != "1" {
		t.Fatalf("genuine original zero earning: %+v", c)
	}
	gate := commissionPaymentPolicy(t, f)
	if err := updateCommissionPaymentPolicy(t, f, commissionPaymentActor(f), true, gate.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessPayments(ctx, 20); err != nil {
		t.Fatal(err)
	}
	pay := payManualCommissionForAdjustment(t, f, commissionPayments(t, f).Items[0])
	targets := commissionTargets(t, f, pay.ID)
	if pay.PaidPoints != "0" || len(targets.Items) != 1 || targets.Items[0].OriginalPoints != 0 || targets.Items[0].LedgerEntryID != nil {
		t.Fatalf("original zero must not fabricate ledger: pay=%+v targets=%+v", pay, targets)
	}
	target := targets.Items[0]
	adj, err := adjustCommission(t, f, f.betting.brand, target, commissionAdjustmentActor(f), 1, 1)
	if err != nil || adj.DeltaPoints != 1 || adj.LedgerEntryID == "" {
		t.Fatalf("genuine zero-to-one manual credit: %+v %v", adj, err)
	}
	if wallet := commissionWalletBySource(t, f, target.MemberID); wallet[3][0] != 1 {
		t.Fatalf("real manual credit absent: %+v", wallet)
	}
	return f, pay, target, adj
}

func TestCommissionZeroOriginalManualCreditRequiresDifferenceNotNewFullPayment(t *testing.T) {
	t.Parallel()
	f, pay, target, adj := zeroOriginalManualOnlyFixture(t)
	ctx, db := context.Background(), f.betting.db
	c := readCommissionCycle(t, f, pay.CycleID)
	var err error
	before := correctionPlanSnapshot(t, f, pay.ID)
	adjustmentBefore := commissionManualPolicyAdjustmentEvidence(t, f, adj)
	var paymentOID, sourceOID, paymentAfter, sourceAfter uint32
	if err = db.QueryRow(ctx, `SELECT 'guard_commission_payment()'::regprocedure::oid,'commission_correction_source_valid(uuid,uuid,uuid)'::regprocedure::oid`).Scan(&paymentOID, &sourceOID); err != nil {
		t.Fatal(err)
	}
	if err = database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if before != correctionPlanSnapshot(t, f, pay.ID) || adjustmentBefore != commissionManualPolicyAdjustmentEvidence(t, f, adj) {
		t.Fatal("analysis/manual-only migration changed actual funds, original payment, adjustment or audit")
	}
	if err = db.QueryRow(ctx, `SELECT 'guard_commission_payment()'::regprocedure::oid,'commission_correction_source_valid(uuid,uuid,uuid)'::regprocedure::oid`).Scan(&paymentAfter, &sourceAfter); err != nil || paymentOID != paymentAfter || sourceOID != sourceAfter {
		t.Fatalf("migration replaced guard identities: %d/%d -> %d/%d err=%v", paymentOID, sourceOID, paymentAfter, sourceAfter, err)
	}
	second := createFixtureCorrection(t, f.betting, correctedDigits(1, 2, 2))
	if _, err = f.betting.service.ProcessCorrections(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if got, err := f.betting.service.Correction(ctx, f.betting.brand, second.ID); err != nil || got.NewJobID == nil {
		t.Fatalf("genuine losing replacement: %+v %v", got, err)
	}
	if _, err = f.betting.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	advanceCommissionWorker(t, f, 40)
	c = readCommissionCycle(t, f, c.ID)
	if c.State != "ready" || c.TotalPoints != "1" {
		t.Fatalf("new genuine one-point earning: %+v", c)
	}
	if _, err = f.service.ProcessPayments(ctx, 20); err != nil {
		t.Fatal(err)
	}
	original := commissionPaymentRead(t, f, pay.ID)
	if original.State != "blocked" || original.LastErrorCode == nil || *original.LastErrorCode != "COMMISSION_PAYMENT_CORRECTION_REQUIRED" {
		t.Fatalf("actual manual credit must block original zero payment, never stale it and register a fresh full payout: %+v", original)
	}
	if page := commissionPayments(t, f); len(page.Items) != 1 {
		t.Fatalf("manual-only actual credit generated another full payment: %+v", page)
	}
	if _, err = f.service.ProcessCorrectionPlans(ctx, 20); err != nil {
		t.Fatal(err)
	}
	plans := correctionPlansRead(t, f)
	if len(plans.Items) != 1 || plans.Items[0].State != commission.CorrectionPlanReady || plans.Items[0].BeforePoints != "1" || plans.Items[0].CalculatedPoints != "1" || plans.Items[0].NetPoints == nil || *plans.Items[0].NetPoints != "0" {
		t.Fatalf("manual-only actual credit should prepare zero difference: %+v", plans)
	}
	enableCorrectionExecutionPolicy(t, f)
	if _, err = f.service.ProcessCorrectionExecutions(ctx, 20); err != nil {
		t.Fatal(err)
	}
	executions := correctionExecutionsRead(t, f)
	if len(executions.Items) != 1 || executions.Items[0].State != commission.CorrectionExecutionAwaitingApproval {
		t.Fatalf("new difference requires fresh approval: %+v", executions)
	}
	x := executions.Items[0]
	if _, err = approveCorrectionExecution(t, f, x, correctionExecutionActor(f, "approve")); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.ProcessCorrectionExecutions(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if got := correctionExecutionRead(t, f, x.ID); got.State != commission.CorrectionExecutionCompleted || got.AppliedCreditPoints != "0" || got.AppliedDebitPoints != "0" {
		t.Fatalf("zero difference not complete: %+v", got)
	}
	if wallet := commissionWalletBySource(t, f, target.MemberID); wallet[3][0] != 1 {
		t.Fatalf("manual-only credit was paid twice: %+v", wallet)
	}
	var count int
	if err = f.betting.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE member_id=$1 AND entry_type IN('commission','commission_adjustment','commission_correction')`, target.MemberID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("zero original/difference fabricated more ledger entries: count=%d %v", count, err)
	}
}

func TestCommissionZeroOriginalLegacyStaleActualMoneyUpgradeFailsWithoutRepair(t *testing.T) {
	t.Parallel()
	f, pay, _, adj := zeroOriginalManualOnlyFixture(t)
	ctx := context.Background()
	next := createFixtureCorrection(t, f.betting, correctedDigits(1, 2, 2))
	if _, err := f.betting.service.ProcessCorrections(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if got, err := f.betting.service.Correction(ctx, f.betting.brand, next.ID); err != nil || got.NewJobID == nil {
		t.Fatalf("genuine replacement: %+v %v", got, err)
	}
	if _, err := f.betting.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	advanceCommissionWorker(t, f, 40)
	// Reproduce the old worker's metadata invalidation against the real 0073
	// guard. Money was created through services; no fund guard is disabled.
	err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		const reason = "historical original-zero invalidation ignored actual manual credit"
		log, err := audit.Append(ctx, tx, audit.Record{BrandID: f.betting.brand, ActorType: "system", Action: "commission.payment.invalidate", ResourceType: "commission_payment", ResourceID: pay.ID, Reason: reason, RequestID: ids.New(),
			Before: map[string]any{"version": pay.Version, "state": pay.State}, After: map[string]any{"version": pay.Version + 1, "state": "stale"}})
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE commission_payments SET state='stale',version=version+1,last_error_code=NULL,last_audit_log_id=$2 WHERE id=$1`, pay.ID, log)
		return err
	})
	if err != nil {
		t.Fatalf("reproduce real pre-0075 metadata transition: %v", err)
	}
	if got := commissionPaymentRead(t, f, pay.ID); got.State != "stale" {
		t.Fatal("legacy fixture did not preserve actual staled history", got)
	}
	before := correctionPlanSnapshot(t, f, pay.ID)
	adjustmentBefore := commissionManualPolicyAdjustmentEvidence(t, f, adj)
	err = database.Migrate(ctx, f.betting.db)
	if err == nil || !strings.Contains(err.Error(), "stale commission payment has actual money; explicit historical review required") {
		t.Fatalf("legacy actual staled money was silently admitted/repaired: %v", err)
	}
	if before != correctionPlanSnapshot(t, f, pay.ID) || adjustmentBefore != commissionManualPolicyAdjustmentEvidence(t, f, adj) {
		t.Fatal("refused upgrade moved or rewrote actual funds/history")
	}
	var version int
	if err = f.betting.db.QueryRow(ctx, `SELECT max(split_part(name,'_',1)::integer) FROM schema_migrations`).Scan(&version); err != nil || version != 73 {
		t.Fatalf("failed upgrade left partial 0074/0075 migrations: %d %v", version, err)
	}
}
