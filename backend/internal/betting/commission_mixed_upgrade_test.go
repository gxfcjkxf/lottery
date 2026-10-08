package betting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
)

func TestCommissionMixedPaymentUpgradeManualWholeCycleReview(t *testing.T) {
	db := testdb.NewAtVersion(t, 53)
	f := newCommissionBatchFixtureFromBetting(t, newBettingFixtureWithDBWindow(t, db, storeTestBrand, 45*time.Second, 47*time.Second))
	ctx := context.Background()

	// The fixture's actual wagers captured manual mode. Change the policy before
	// placing another real wager so the settled commission run contains both
	// immutable payout snapshots.
	policy, err := f.service.Policy(ctx, f.betting.brand)
	if err != nil {
		t.Fatal(err)
	}
	if err = commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, e := f.service.Update(ctx, tx, f.betting.brand, f.actor, commission.PolicyInput{
			Version: policy.Version,
			Config:  commission.PolicyConfig{Enabled: true, Calendar: policy.Config.Calendar, PayoutMode: commission.PayoutAutomatic},
			Reason:  "capture automatic mode on a new genuine wager for mixed upgrade review",
		}, commissionPaymentMeta(f.actor))
		return e
	}); err != nil {
		t.Fatal(err)
	}
	automaticInput := f.betting.input
	automaticInput.Selection = rules.Selection{Digits: [][]int{{0}, {0}, {0}}}
	automaticOrder, err := placeBettingOrder(t, f.betting, automaticInput, "commission-mixed-upgrade-automatic-order")
	if err != nil {
		t.Fatal(err)
	}
	if !automaticOrder.PlacedAt.Before(f.boundary) {
		t.Fatalf("automatic order placed at %s after cycle boundary %s", automaticOrder.PlacedAt, f.boundary)
	}
	f.orders = append(f.orders, automaticOrder)

	waitCommissionBoundary(t, f.boundary)
	cycle := createCommissionCycle(t, f)
	eligibilitySettle(t, f.betting)
	advanceCommissionWorker(t, f, 30)
	cycle = readCommissionCycle(t, f, cycle.ID)
	if cycle.State != "ready" || cycle.TotalPoints != "1" || cycle.EarningCount == "0" || cycle.CurrentRunID == nil || cycle.EvidenceEpoch == nil {
		t.Fatalf("mixed source snapshots did not produce the expected ready commission run: %+v", cycle)
	}
	var modes int
	if err = db.QueryRow(ctx, `SELECT count(DISTINCT commission_rule_snapshot->'financial_policy'->'config'->>'payout_mode') FROM bet_orders WHERE id=ANY($1::uuid[])`, []string{f.orders[0].ID, automaticOrder.ID}).Scan(&modes); err != nil || modes != 2 {
		t.Fatalf("original order snapshots contain %d payout modes, err=%v", modes, err)
	}

	actor := commissionPaymentActor(f)
	paymentPolicy := commissionPaymentPolicy(t, f)
	if paymentPolicy.Enabled {
		t.Fatalf("payment gate unexpectedly enabled before upgrade fixture setup: %+v", paymentPolicy)
	}
	if err = updateCommissionPaymentPolicy(t, f, actor, true, paymentPolicy.Version); err != nil {
		t.Fatal(err)
	}

	// Reproduce payment_store.createPayment's 0050 system audit and INSERT
	// exactly. At version 53 a mixed run is legitimately blocked; the payment
	// trigger verifies the complete immutable totals and current evidence.
	var mode, total string
	var targetCount int64
	if err = db.QueryRow(ctx, `SELECT commission_payment_mode($1),coalesce(sum(points),0)::text,count(*) FROM commission_earnings WHERE run_id=$1`, *cycle.CurrentRunID).Scan(&mode, &total, &targetCount); err != nil {
		t.Fatal(err)
	}
	if mode != "mixed" || total != "1" {
		t.Fatalf("historical payment source mode=%q total=%q, want mixed and one point", mode, total)
	}
	paymentID := ids.New()
	var creationAudit string
	if err = commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		var e error
		creationAudit, e = audit.Append(ctx, tx, audit.Record{
			BrandID: f.betting.brand, ActorType: "system", Action: "commission.payment.create",
			ResourceType: "commission_payment", ResourceID: paymentID,
			Reason: "register complete cycle for saved payout mode", RequestID: ids.New(),
			After: map[string]any{"version": 1, "state": "blocked", "run_id": *cycle.CurrentRunID, "payout_mode": mode, "total_points": total, "target_count": targetCount},
		})
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO commission_payments(id,brand_id,cycle_id,run_id,evidence_epoch,payout_mode,state,total_points,target_count,approval_actor_type,approval_audit_log_id,creation_audit_log_id,last_audit_log_id,last_error_code) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$12,$13)`,
			paymentID, f.betting.brand, cycle.ID, *cycle.CurrentRunID, *cycle.EvidenceEpoch, mode, "blocked", total, targetCount, nil, nil, creationAudit, "COMMISSION_PAYMENT_MODE_UNRESOLVED")
		return e
	}); err != nil {
		t.Fatal(err)
	}

	fingerprint := func() string {
		t.Helper()
		var raw string
		err := db.QueryRow(ctx, `SELECT jsonb_build_object(
			'orders',(SELECT jsonb_agg(to_jsonb(o) ORDER BY o.id) FROM bet_orders o),
			'calculations',(SELECT jsonb_agg(to_jsonb(c) ORDER BY c.id) FROM commission_calculations c),
			'allocations',(SELECT jsonb_agg(to_jsonb(a) ORDER BY a.run_id,a.order_id,a.agent_id) FROM commission_allocations a),
			'earnings',(SELECT jsonb_agg(to_jsonb(e) ORDER BY e.id) FROM commission_earnings e),
			'accounts',(SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM point_accounts a),
			'wallets',(SELECT jsonb_agg(to_jsonb(b) ORDER BY b.account_id,b.source,b.state) FROM point_buckets b),
			'ledger',(SELECT jsonb_agg(to_jsonb(l) ORDER BY l.id) FROM point_ledger_entries l),
			'payments',(SELECT jsonb_agg(to_jsonb(p) ORDER BY p.id) FROM commission_payments p),
			'audits',(SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM audit_logs a))::text`).Scan(&raw)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	before := fingerprint()
	if err = database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if after := fingerprint(); after != before {
		t.Fatal("0054 migration changed existing order/snapshot, calculation, wallet, ledger, payment, or audit history")
	}
	if err = database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if steps, processErr := f.service.ProcessPayments(ctx, 20); processErr != nil || steps != 0 {
		t.Fatalf("legacy mixed payment advanced without explicit approval: steps=%d err=%v", steps, processErr)
	}
	if after := fingerprint(); after != before {
		t.Fatal("repeat migration or unapproved worker changed historical financial evidence")
	}
	var schema string
	if err = db.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	var pinned bool
	if err = db.QueryRow(ctx, `SELECT coalesce(proconfig @> ARRAY[$1]::text[],false) FROM pg_proc WHERE oid=to_regprocedure('guard_commission_payment()')`, "search_path=pg_catalog, "+schema+", pg_temp").Scan(&pinned); err != nil || !pinned {
		t.Fatalf("0054 payment guard search path not pinned: pinned=%t err=%v", pinned, err)
	}
	payment := commissionPaymentRead(t, f, paymentID)
	if payment.State != "blocked" || payment.PayoutMode != "mixed" || payment.LastErrorCode == nil || *payment.LastErrorCode != "COMMISSION_PAYMENT_MODE_UNRESOLVED" || payment.PaidCount != "0" || payment.PaidPoints != "0" {
		t.Fatalf("historical mixed payment changed during migration: %+v", payment)
	}
	var targets, credits int
	if err = db.QueryRow(ctx, `SELECT (SELECT count(*) FROM commission_payment_targets WHERE payment_id=$1), (SELECT count(*) FROM point_ledger_entries WHERE entry_type='commission')`, paymentID).Scan(&targets, &credits); err != nil || targets != 0 || credits != 0 {
		t.Fatalf("historical blocked payment has targets=%d credits=%d err=%v", targets, credits, err)
	}

	approve := func(a access.Account, version int64, reason string) error {
		return commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
			_, e := f.service.ApprovePaymentTx(ctx, tx, f.betting.brand, paymentID, a,
				commission.RetryCycleInput{Version: version, Reason: reason}, commissionPaymentMeta(a))
			return e
		})
	}
	if err = updateCommissionPaymentPolicy(t, f, actor, false, commissionPaymentPolicy(t, f).Version); err != nil {
		t.Fatal(err)
	}
	if err = approve(actor, payment.Version, "mixed review while payment gate is disabled"); !errors.Is(err, commission.ErrPaymentState) {
		t.Fatalf("approval with disabled payment gate error=%v, want state conflict", err)
	}
	if err = updateCommissionPaymentPolicy(t, f, actor, true, commissionPaymentPolicy(t, f).Version); err != nil {
		t.Fatal(err)
	}
	viewer := actor
	viewer.Roles = []access.Role{{BrandID: f.betting.brand, Permissions: []access.Permission{{Resource: "commission", Action: "view", Scope: access.ScopeBrand}}}}
	if err = approve(viewer, payment.Version, "viewer cannot approve mixed payout"); !errors.Is(err, commission.ErrDenied) {
		t.Fatalf("viewer mixed approval error=%v, want denied", err)
	}
	super := actor
	super.SuperAdmin = true
	if err = approve(super, payment.Version, "super-admin cannot approve mixed payout"); !errors.Is(err, commission.ErrDenied) {
		t.Fatalf("super-admin mixed approval error=%v, want denied", err)
	}
	if err = approve(actor, payment.Version+1, "stale mixed approval version"); !errors.Is(err, commission.ErrPaymentVersion) {
		t.Fatalf("stale mixed approval version error=%v, want version conflict", err)
	}
	if got := commissionPaymentRead(t, f, paymentID); got.State != "blocked" || got.Version != payment.Version || got.PaidCount != "0" || got.PaidPoints != "0" {
		t.Fatalf("rejected mixed approvals changed payment: %+v", got)
	}
	if err = approve(actor, payment.Version, "approve whole-cycle mixed payout after review"); err != nil {
		t.Fatal(err)
	}
	paying := commissionPaymentRead(t, f, paymentID)
	if paying.State != "paying" || paying.PayoutMode != "mixed" || paying.Version != payment.Version+1 {
		t.Fatalf("approved mixed payment=%+v", paying)
	}
	if steps, processErr := f.service.ProcessPayments(ctx, 20); processErr != nil || steps != 2 {
		t.Fatalf("approved mixed payment processing steps=%d err=%v", steps, processErr)
	}
	paid := commissionPaymentRead(t, f, paymentID)
	if paid.State != "paid" || paid.PaidPoints != "1" || paid.PaidCount != "1" {
		t.Fatalf("approved mixed payment did not post exactly one point: %+v", paid)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='commission' AND reference_type='commission_payment_target' AND operation_key LIKE 'commission-payment:%'`).Scan(&credits); err != nil || credits != 1 {
		t.Fatalf("commission credits=%d err=%v, want one", credits, err)
	}
	wallet, err := (points.Store{DB: db}).Read(ctx, f.betting.brand, f.node.MemberID)
	if err != nil || wallet.CommissionPoints != 1 {
		t.Fatalf("beneficiary commission wallet=%+v err=%v, want one point", wallet, err)
	}
	if steps, processErr := f.service.ProcessPayments(ctx, 20); processErr != nil || steps != 0 {
		t.Fatalf("replayed mixed payment processing steps=%d err=%v", steps, processErr)
	}
}
