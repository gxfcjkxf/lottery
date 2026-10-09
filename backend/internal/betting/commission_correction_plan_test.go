package betting

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/attribution"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func correctionPlanActor(f commissionBatchFixture) access.Account {
	a := commissionPaymentActor(f)
	for i := range a.Roles {
		if a.Roles[i].BrandID == f.betting.brand {
			a.Roles[i].Permissions = append(a.Roles[i].Permissions,
				access.Permission{Resource: "commission_correction", Action: "retry", Scope: access.ScopeBrand})
		}
	}
	return a
}

func correctionPlanRead(t *testing.T, f commissionBatchFixture, id string) commission.CorrectionPlan {
	t.Helper()
	tx, err := f.betting.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	plan, err := f.service.CorrectionPlanTx(context.Background(), tx, f.betting.brand, id)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func correctionPlansRead(t *testing.T, f commissionBatchFixture) commission.CorrectionPlanPage {
	t.Helper()
	tx, err := f.betting.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	page, err := f.service.CorrectionPlansTx(context.Background(), tx, f.betting.brand, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func correctionPlanTargetsRead(t *testing.T, f commissionBatchFixture, planID string) commission.CorrectionPlanTargetPage {
	t.Helper()
	tx, err := f.betting.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	page, err := f.service.CorrectionPlanTargetsTx(context.Background(), tx, f.betting.brand, planID, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func correctionPlanSnapshot(t *testing.T, f commissionBatchFixture, paymentID string) string {
	t.Helper()
	ctx := context.Background()
	var payment, targets, ledger, accounts, buckets, heads, outbox, deliveries string
	err := f.betting.db.QueryRow(ctx, `
		SELECT coalesce((SELECT jsonb_agg(to_jsonb(p) ORDER BY p.id)::text FROM commission_payments p WHERE p.brand_id=$1 AND p.id=$2),'[]'),
		       coalesce((SELECT jsonb_agg(to_jsonb(t) ORDER BY t.id)::text FROM commission_payment_targets t WHERE t.brand_id=$1 AND t.payment_id=$2),'[]'),
		       coalesce((SELECT jsonb_agg(to_jsonb(l) ORDER BY l.id)::text FROM point_ledger_entries l WHERE l.brand_id=$1),'[]'),
		       coalesce((SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id)::text FROM point_accounts a WHERE a.brand_id=$1),'[]'),
		       coalesce((SELECT jsonb_agg(to_jsonb(b) ORDER BY b.account_id,b.source,b.state)::text FROM point_buckets b WHERE b.brand_id=$1),'[]'),
		       coalesce((SELECT jsonb_agg(to_jsonb(h) ORDER BY h.target_id)::text FROM commission_adjustment_heads h WHERE h.brand_id=$1),'[]'),
		       coalesce((SELECT jsonb_agg(to_jsonb(o) ORDER BY o.id)::text FROM outbox_events o WHERE o.brand_id=$1),'[]'),
		       coalesce((SELECT jsonb_agg(to_jsonb(d) ORDER BY d.event_id)::text FROM notification_deliveries d WHERE d.brand_id=$1),'[]')`, f.betting.brand, paymentID).
		Scan(&payment, &targets, &ledger, &accounts, &buckets, &heads, &outbox, &deliveries)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(struct {
		Payment, Targets, Ledger, Accounts, Buckets, Heads, Outbox, Deliveries string
	}{payment, targets, ledger, accounts, buckets, heads, outbox, deliveries})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func correctionPlanArtifactsSnapshot(t *testing.T, f commissionBatchFixture) string {
	t.Helper()
	ctx := context.Background()
	var plans, steps, targets, audits string
	err := f.betting.db.QueryRow(ctx, `
		SELECT coalesce((SELECT jsonb_agg(to_jsonb(p) ORDER BY p.id)::text FROM commission_correction_plans p WHERE p.brand_id=$1),'[]'),
		       coalesce((SELECT jsonb_agg(to_jsonb(s) ORDER BY s.plan_id,s.version)::text FROM commission_correction_plan_steps s WHERE s.brand_id=$1),'[]'),
		       coalesce((SELECT jsonb_agg(to_jsonb(t) ORDER BY t.plan_id,t.agent_id)::text FROM commission_correction_plan_targets t WHERE t.brand_id=$1),'[]'),
		       coalesce((SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id)::text FROM audit_logs a WHERE a.brand_id=$1),'[]')`, f.betting.brand).
		Scan(&plans, &steps, &targets, &audits)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(struct{ Plans, Steps, Targets, Audits string }{plans, steps, targets, audits})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func createAndPayAutomaticCorrectionFixture(t *testing.T) (commissionBatchFixture, commission.Payment, commission.Cycle) {
	t.Helper()
	f := newAutomaticCommissionBatchFixture(t)
	ctx := context.Background()
	waitCommissionBoundary(t, f.boundary)
	cycle := createCommissionCycle(t, f)
	eligibilitySettle(t, f.betting)
	advanceCommissionWorker(t, f, 30)
	cycle = readCommissionCycle(t, f, cycle.ID)
	if cycle.State != "ready" || cycle.TotalPoints != "1" || cycle.EarningCount != "1" || cycle.CurrentRunID == nil {
		t.Fatalf("actual automatic commission run did not calculate one point: %+v", cycle)
	}
	actor := commissionPaymentActor(f)
	gate := commissionPaymentPolicy(t, f)
	if err := updateCommissionPaymentPolicy(t, f, actor, true, gate.Version); err != nil {
		t.Fatal(err)
	}
	if steps, err := f.service.ProcessPayments(ctx, 1); err != nil || steps != 1 {
		t.Fatalf("register automatic payout: steps=%d err=%v", steps, err)
	}
	page := commissionPayments(t, f)
	if len(page.Items) != 1 {
		t.Fatalf("actual automatic payout page=%+v", page.Items)
	}
	payment := page.Items[0]
	for i := 0; i < 8 && payment.State != "paid"; i++ {
		if _, err := f.betting.db.Exec(ctx, `UPDATE commission_payments SET next_work_at=clock_timestamp() WHERE brand_id=$1 AND id=$2`, f.betting.brand, payment.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := f.service.ProcessPayments(ctx, 1); err != nil {
			t.Fatal(err)
		}
		payment = commissionPaymentRead(t, f, payment.ID)
	}
	if payment.State != "paid" || payment.PaidPoints != "1" || payment.PaidCount != "1" {
		t.Fatalf("real payment did not finish before result correction: %+v", payment)
	}
	return f, payment, cycle
}

// Corrected test draws are produced by the public correction and settlement
// workers, so this setup never fabricates a run, earning, target, or payment.
func correctCommissionRunToZero(t *testing.T, f commissionBatchFixture, cycleID string, oldRunID string) commission.Cycle {
	t.Helper()
	ctx := context.Background()
	c := createFixtureCorrection(t, f.betting, correctedDigits(0, 0, 0))
	if c.State != "reversing" {
		t.Fatalf("real result correction started in state %q", c.State)
	}
	if _, err := f.betting.service.ProcessCorrections(ctx, 20); err != nil {
		t.Fatal(err)
	}
	c, err := f.betting.service.Correction(ctx, f.betting.brand, c.ID)
	if err != nil || c.State != "resettling" || c.NewJobID == nil {
		t.Fatalf("correction did not create real replacement settlement: %+v err=%v", c, err)
	}
	if _, err := f.betting.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	advanceCommissionWorker(t, f, 40)
	cycle := readCommissionCycle(t, f, cycleID)
	if cycle.State != "ready" || !cycle.EvidenceCurrent || cycle.CurrentRunID == nil || *cycle.CurrentRunID == oldRunID || cycle.TotalPoints != "0" {
		t.Fatalf("result correction did not create a current zero-earning run: %+v", cycle)
	}
	return cycle
}

func TestCommissionCorrectionPlanIsPreviewOnlyForActuallyPaidCorrectedRun(t *testing.T) {
	t.Parallel()
	f, payment, oldCycle := createAndPayAutomaticCorrectionFixture(t)
	ctx := context.Background()
	current := correctCommissionRunToZero(t, f, payment.CycleID, payment.RunID)
	if _, err := f.service.ProcessPayments(ctx, 20); err != nil {
		t.Fatal(err)
	}
	blockedPayment := commissionPaymentRead(t, f, payment.ID)
	if blockedPayment.State != "blocked" || blockedPayment.PaidPoints != "1" || blockedPayment.PaidCount != "1" || blockedPayment.LastErrorCode == nil || *blockedPayment.LastErrorCode != "COMMISSION_PAYMENT_CORRECTION_REQUIRED" {
		t.Fatalf("paid original payout was not fenced pending correction review: %+v", blockedPayment)
	}
	before := correctionPlanSnapshot(t, f, payment.ID)
	for i := 0; i < 12; i++ {
		if _, err := f.service.ProcessCorrectionPlans(ctx, 1); err != nil {
			t.Fatal(err)
		}
	}
	page := correctionPlansRead(t, f)
	if page.TotalCount != "1" || len(page.Items) != 1 {
		t.Fatalf("one paid/corrected original should produce one idempotent plan: %+v", page)
	}
	plan := page.Items[0]
	if plan.State != commission.CorrectionPlanReady || plan.PaymentID != payment.ID || plan.CycleID != payment.CycleID || plan.RunID != *current.CurrentRunID || plan.PayoutMode != commission.PayoutAutomatic || plan.BeforePoints != "1" || plan.CalculatedPoints != "0" || plan.CreditPoints == nil || *plan.CreditPoints != "0" || plan.DebitPoints == nil || *plan.DebitPoints != "1" || plan.NetPoints == nil || *plan.NetPoints != "-1" || plan.TargetCount != "1" || plan.PlannedCount != "1" {
		t.Fatalf("correction plan is not the exact one-point debit preview: %+v", plan)
	}
	targets := correctionPlanTargetsRead(t, f, plan.ID)
	if targets.TotalCount != "1" || len(targets.Items) != 1 {
		t.Fatalf("plan target page=%+v", targets)
	}
	target := targets.Items[0]
	originalTargets := commissionTargets(t, f, payment.ID)
	if len(originalTargets.Items) != 1 || target.OriginalTargetID == nil || *target.OriginalTargetID != originalTargets.Items[0].ID || target.PointsBefore != 1 || target.PointsAfter != 0 || target.DeltaPoints != -1 {
		t.Fatalf("target did not join the actual paid target to corrected zero run: %+v original=%+v", target, originalTargets.Items)
	}
	// A plan's ready state is informational only. It must not alter any saved
	// payout, ledger, wallet, adjustment-head, or notification fingerprints.
	if after := correctionPlanSnapshot(t, f, payment.ID); after != before {
		t.Fatalf("planning mutated financial/original fingerprints\nbefore=%s\nafter=%s", before, after)
	}
	if n, err := f.service.ProcessCorrectionPlans(ctx, 20); err != nil || n != 0 {
		t.Fatalf("repeat worker duplicated completed plan: steps=%d err=%v", n, err)
	}
	page2 := correctionPlansRead(t, f)
	if page2.TotalCount != "1" || len(page2.Items) != 1 || page2.Items[0].ID != plan.ID {
		t.Fatalf("repeat worker changed plan cardinality: page=%+v", page2)
	}
	if commissionPaymentRead(t, f, payment.ID).State != "blocked" || oldCycle.CurrentRunID == nil || *oldCycle.CurrentRunID != payment.RunID {
		t.Fatal("preview incorrectly changed the original payment/current evidence")
	}
}

func TestCommissionCorrectionPlanRetryRequiresSeparateBrandPermission(t *testing.T) {
	t.Parallel()
	f, payment, _ := createAndPayAutomaticCorrectionFixture(t)
	current := correctCommissionRunToZero(t, f, payment.CycleID, payment.RunID)
	if _, err := f.service.ProcessPayments(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessCorrectionPlans(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	var plan commission.CorrectionPlan
	page := correctionPlansRead(t, f)
	if len(page.Items) == 1 {
		plan = page.Items[0]
	}
	if plan.ID == "" || plan.RunID != *current.CurrentRunID {
		t.Fatalf("read correction plan: %+v", plan)
	}
	viewer := f.actor
	viewer.Roles = []access.Role{{BrandID: f.betting.brand, Permissions: []access.Permission{{Resource: "commission", Action: "view", Scope: access.ScopeBrand}}}}
	err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, err := f.service.RetryCorrectionPlanTx(context.Background(), tx, f.betting.brand, plan.ID, viewer,
			commission.RetryCycleInput{Version: plan.Version, Reason: "retry must require its dedicated permission"}, commissionPaymentMeta(viewer))
		return err
	})
	if !errors.Is(err, commission.ErrDenied) {
		t.Fatalf("commission.view without commission_correction.retry.brand retry error=%v; want denied", err)
	}
	withRetry := correctionPlanActor(f)
	err = commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, err := f.service.RetryCorrectionPlanTx(context.Background(), tx, f.betting.brand, plan.ID, withRetry,
			commission.RetryCycleInput{Version: plan.Version, Reason: "ready preview is not an executable correction"}, commissionPaymentMeta(withRetry))
		return err
	})
	if !errors.Is(err, commission.ErrCorrectionPlanState) {
		t.Fatalf("retrying a ready preview should be non-executable, got %v", err)
	}
}

func TestCommissionCorrectionPlanFurtherActualEvidenceMakesPlanStale(t *testing.T) {
	t.Parallel()
	f, payment, _ := createAndPayAutomaticCorrectionFixture(t)
	first := correctCommissionRunToZero(t, f, payment.CycleID, payment.RunID)
	if _, err := f.service.ProcessPayments(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessCorrectionPlans(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	plans := correctionPlansRead(t, f)
	if len(plans.Items) != 1 || plans.Items[0].State != commission.CorrectionPlanReady {
		t.Fatalf("first preview not ready: %+v", plans)
	}
	// A second real corrected result creates a new settlement generation and
	// advances evidence; a frozen plan must become stale, never reuse old math.
	second := createFixtureCorrection(t, f.betting, correctedDigits(1, 2, 2))
	if second.State != "reversing" {
		t.Fatalf("second correction did not enter the real workflow: %+v", second)
	}
	if _, err := f.betting.service.ProcessCorrections(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	second, err := f.betting.service.Correction(context.Background(), f.betting.brand, second.ID)
	if err != nil || second.State != "resettling" || second.NewJobID == nil {
		t.Fatalf("second correction did not start new settlement: %+v err=%v", second, err)
	}
	if _, err = f.betting.service.ProcessSettlements(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	advanceCommissionWorker(t, f, 50)
	newCycle := readCommissionCycle(t, f, payment.CycleID)
	if newCycle.CurrentRunID == nil || *newCycle.CurrentRunID == *first.CurrentRunID || newCycle.EvidenceEpoch == nil || first.EvidenceEpoch == nil || *newCycle.EvidenceEpoch == *first.EvidenceEpoch {
		t.Fatalf("second correction did not advance evidence: first=%+v now=%+v", first, newCycle)
	}
	if _, err = f.service.ProcessCorrectionPlans(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	got := correctionPlanRead(t, f, plans.Items[0].ID)
	if got.State != commission.CorrectionPlanStale || got.RunID != plans.Items[0].RunID || got.NetPoints == nil || *got.NetPoints != "-1" {
		t.Fatalf("stale preview improperly reused/rewrote frozen plan math: %+v", got)
	}
}

func TestCommissionCorrectionPlanManualAdjustmentVersionBlocksWholePlan(t *testing.T) {
	t.Parallel()
	f := newCommissionBatchFixture(t)
	payment := payManualCommissionForAdjustment(t, f, prepareManualCommissionPayment(t, f))
	targets := commissionTargets(t, f, payment.ID)
	if len(targets.Items) != 1 {
		t.Fatalf("expected one actually paid manual target: %+v", targets)
	}
	if _, err := adjustCommission(t, f, f.betting.brand, targets.Items[0], commissionAdjustmentActor(f), 1, 2); err != nil {
		t.Fatalf("create real manual commission adjustment: %v", err)
	}
	var adjustmentVersion int64
	if err := f.betting.db.QueryRow(context.Background(), `SELECT version FROM commission_adjustment_heads WHERE brand_id=$1 AND target_id=$2`, f.betting.brand, targets.Items[0].ID).Scan(&adjustmentVersion); err != nil || adjustmentVersion != 2 {
		t.Fatalf("expected adjustment head version 2, got %d err=%v", adjustmentVersion, err)
	}
	current := correctCommissionRunToZero(t, f, payment.CycleID, payment.RunID)
	if _, err := f.service.ProcessPayments(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	if got := commissionPaymentRead(t, f, payment.ID); got.State != "blocked" {
		t.Fatalf("corrected original manual payment not blocked: %+v", got)
	}
	if _, err := f.service.ProcessCorrectionPlans(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	plans := correctionPlansRead(t, f)
	if len(plans.Items) != 1 {
		t.Fatalf("manual-adjustment source should produce one policy-blocked plan: %+v", plans)
	}
	plan := plans.Items[0]
	if plan.State != commission.CorrectionPlanBlocked || plan.LastErrorCode == nil || *plan.LastErrorCode != "COMMISSION_CORRECTION_MANUAL_POLICY_UNRESOLVED" || plan.CreditPoints != nil || plan.DebitPoints != nil || plan.NetPoints != nil || plan.PayoutMode != commission.PayoutManual || plan.RunID != *current.CurrentRunID {
		t.Fatalf("version>1 adjustment must block all amounts while preserving manual mode: %+v", plan)
	}
	page := correctionPlanTargetsRead(t, f, plan.ID)
	if page.TotalCount != "0" || len(page.Items) != 0 {
		t.Fatalf("blocked manual-policy plan must expose no executable targets (OPEN117): %+v", page)
	}
	mode, err := commission.MergeCorrectionMode(commission.PayoutManual, commission.PayoutNone)
	if err != nil || mode != commission.PayoutManual {
		t.Fatalf("manual source with no replacement payout mode must retain manual review: mode=%q err=%v", mode, err)
	}
}

func createAndPayMixedCommissionFixture(t *testing.T) (commissionBatchFixture, commission.Payment) {
	t.Helper()
	f := newCommissionBatchFixture(t)
	policy, err := f.service.Policy(context.Background(), f.betting.brand)
	if err != nil {
		t.Fatal(err)
	}
	config := policy.Config
	config.PayoutMode = commission.PayoutAutomatic
	if err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, err := f.service.Update(context.Background(), tx, f.betting.brand, f.actor,
			commission.PolicyInput{Version: policy.Version, Config: config, Reason: "capture actual mixed payout modes"}, commissionPaymentMeta(f.actor))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	in := f.betting.input
	in.Selection = rules.Selection{Digits: [][]int{{0}, {0}, {0}}}
	newOrder, err := placeBettingOrder(t, f.betting, in, "commission-correction-mixed-actual-order")
	if err != nil || !newOrder.PlacedAt.Before(f.boundary) {
		t.Fatalf("actual automatic-snapshot bet missed cycle boundary: %+v err=%v", newOrder, err)
	}
	waitCommissionBoundary(t, f.boundary)
	cycle := createCommissionCycle(t, f)
	eligibilitySettle(t, f.betting)
	advanceCommissionWorker(t, f, 40)
	cycle = readCommissionCycle(t, f, cycle.ID)
	if cycle.State != "ready" || cycle.TotalPoints != "1" {
		t.Fatalf("mixed actual snapshots did not calculate expected payout: %+v", cycle)
	}
	actor := commissionPaymentActor(f)
	gate := commissionPaymentPolicy(t, f)
	if err := updateCommissionPaymentPolicy(t, f, actor, true, gate.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessPayments(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	page := commissionPayments(t, f)
	if len(page.Items) != 1 || page.Items[0].PayoutMode != commission.PayoutMixed || page.Items[0].State != "awaiting_approval" {
		t.Fatalf("mixed payout must require whole-cycle manual approval: %+v", page.Items)
	}
	payment := page.Items[0]
	approver := commissionPaymentActor(f)
	if err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, err := f.service.ApprovePaymentTx(context.Background(), tx, f.betting.brand, payment.ID, approver,
			commission.RetryCycleInput{Version: payment.Version, Reason: "approve whole mixed payout"}, commissionPaymentMeta(approver))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if _, err := f.betting.db.Exec(context.Background(), `UPDATE commission_payments SET next_work_at=clock_timestamp() WHERE brand_id=$1 AND id=$2`, f.betting.brand, payment.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := f.service.ProcessPayments(context.Background(), 1); err != nil {
			t.Fatal(err)
		}
		payment = commissionPaymentRead(t, f, payment.ID)
		if payment.State == "paid" {
			return f, payment
		}
	}
	t.Fatalf("whole-cycle mixed payout did not reach paid: %+v", payment)
	return f, payment
}

func TestCommissionCorrectionPlanPreservesMixedModeAfterActualReview(t *testing.T) {
	t.Parallel()
	f, payment := createAndPayMixedCommissionFixture(t)
	current := correctCommissionRunToZero(t, f, payment.CycleID, payment.RunID)
	if _, err := f.service.ProcessPayments(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessCorrectionPlans(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	plans := correctionPlansRead(t, f)
	if len(plans.Items) != 1 || plans.Items[0].State != commission.CorrectionPlanReady || plans.Items[0].PayoutMode != commission.PayoutMixed || plans.Items[0].RunID != *current.CurrentRunID {
		t.Fatalf("mixed original payout review mode was lost in correction plan: %+v", plans)
	}
}

func TestCommissionCorrectionPlanHonorsPaymentGateAndDisabledBrand(t *testing.T) {
	t.Parallel()
	f, payment, _ := createAndPayAutomaticCorrectionFixture(t)
	correctCommissionRunToZero(t, f, payment.CycleID, payment.RunID)
	if _, err := f.service.ProcessPayments(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	actor := commissionPaymentActor(f)
	gate := commissionPaymentPolicy(t, f)
	if err := updateCommissionPaymentPolicy(t, f, actor, false, gate.Version); err != nil {
		t.Fatal(err)
	}
	if n, err := f.service.ProcessCorrectionPlans(context.Background(), 20); err != nil || n != 0 {
		t.Fatalf("closed payment gate planned a correction: steps=%d err=%v", n, err)
	}
	if plans := correctionPlansRead(t, f); plans.TotalCount != "0" || len(plans.Items) != 0 {
		t.Fatalf("closed gate created correction plan: %+v", plans)
	}
	gate = commissionPaymentPolicy(t, f)
	if err := updateCommissionPaymentPolicy(t, f, actor, true, gate.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := f.betting.db.Exec(context.Background(), `UPDATE brands SET status='disabled' WHERE id=$1`, f.betting.brand); err != nil {
		t.Fatal(err)
	}
	if n, err := f.service.ProcessCorrectionPlans(context.Background(), 20); err != nil || n != 0 {
		t.Fatalf("disabled brand planned a correction: steps=%d err=%v", n, err)
	}
	if plans := correctionPlansRead(t, f); plans.TotalCount != "0" || len(plans.Items) != 0 {
		t.Fatalf("disabled brand created correction plan: %+v", plans)
	}
	if _, err := f.betting.db.Exec(context.Background(), `UPDATE brands SET status='active' WHERE id=$1`, f.betting.brand); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessCorrectionPlans(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	if plans := correctionPlansRead(t, f); len(plans.Items) != 1 {
		t.Fatalf("re-enabled brand did not resume eligible planning: %+v", plans)
	}
}

func TestCommissionCorrectionPlanAuditFailureRollsBackCreation(t *testing.T) {
	t.Parallel()
	f, payment, _ := createAndPayAutomaticCorrectionFixture(t)
	correctCommissionRunToZero(t, f, payment.CycleID, payment.RunID)
	if _, err := f.service.ProcessPayments(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	before := correctionPlanSnapshot(t, f, payment.ID)
	if _, err := f.betting.db.Exec(context.Background(), `CREATE FUNCTION test_reject_commission_correction_plan_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='commission.correction_plan.create' THEN RAISE EXCEPTION 'test correction plan audit outage'; END IF; RETURN NEW; END $$; CREATE TRIGGER test_reject_commission_correction_plan_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION test_reject_commission_correction_plan_audit()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.betting.db.Exec(context.Background(), `DROP TRIGGER IF EXISTS test_reject_commission_correction_plan_audit ON audit_logs; DROP FUNCTION IF EXISTS test_reject_commission_correction_plan_audit()`)
	})
	if _, err := f.service.ProcessCorrectionPlans(context.Background(), 20); err == nil {
		t.Fatal("audit outage should fail correction-plan creation")
	}
	var plans, auditRows int
	if err := f.betting.db.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM commission_correction_plans WHERE brand_id=$1),(SELECT count(*) FROM audit_logs WHERE brand_id=$1 AND action='commission.correction_plan.create')`, f.betting.brand).Scan(&plans, &auditRows); err != nil || plans != 0 || auditRows != 0 {
		t.Fatalf("audit failure left plan/audit rows: plans=%d audit=%d err=%v", plans, auditRows, err)
	}
	if after := correctionPlanSnapshot(t, f, payment.ID); after != before {
		t.Fatalf("audit failure changed financial fingerprints\nbefore=%s\nafter=%s", before, after)
	}
}

func TestCommissionCorrectionPlanConcurrentWorkersCreateOnePlan(t *testing.T) {
	t.Parallel()
	f, payment, _ := createAndPayAutomaticCorrectionFixture(t)
	correctCommissionRunToZero(t, f, payment.CycleID, payment.RunID)
	if _, err := f.service.ProcessPayments(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	financialBefore := correctionPlanSnapshot(t, f, payment.ID)
	// Make the pool default stronger than the worker's required fresh-statement
	// isolation. The worker must explicitly begin READ COMMITTED transactions.
	poolConfig := f.betting.db.Config().Copy()
	poolConfig.ConnConfig.RuntimeParams["default_transaction_isolation"] = "repeatable read"
	concurrentPool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer concurrentPool.Close()
	var isolation string
	if err := concurrentPool.QueryRow(context.Background(), `SHOW default_transaction_isolation`).Scan(&isolation); err != nil || isolation != "repeatable read" {
		t.Fatal("isolation fixture did not apply", err)
	}
	concurrentService := commission.Service{DB: concurrentPool}
	start := make(chan struct{})
	var wg sync.WaitGroup
	const parallelWorkers = 8
	errs := make(chan error, parallelWorkers)
	for i := 0; i < parallelWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := concurrentService.ProcessCorrectionPlans(context.Background(), 20)
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent correction-plan worker: %v", err)
		}
	}
	plans := correctionPlansRead(t, f)
	if len(plans.Items) != 1 || plans.TotalCount != "1" || plans.Items[0].State != commission.CorrectionPlanReady {
		t.Fatalf("concurrent workers did not converge on one ready plan: %+v", plans)
	}
	targets := correctionPlanTargetsRead(t, f, plans.Items[0].ID)
	if targets.TotalCount != "1" || len(targets.Items) != 1 {
		t.Fatalf("concurrent workers duplicated/lost plan targets: %+v", targets)
	}
	var createAudits, createSteps int
	if err := f.betting.db.QueryRow(context.Background(), `
		SELECT (SELECT count(*) FROM audit_logs WHERE brand_id=$1 AND action='commission.correction_plan.create'),
		       (SELECT count(*) FROM commission_correction_plan_steps WHERE brand_id=$1 AND operation='create')`, f.betting.brand).Scan(&createAudits, &createSteps); err != nil || createAudits != 1 || createSteps != 1 {
		t.Fatalf("concurrent workers duplicated plan creation audit/step: audits=%d steps=%d err=%v", createAudits, createSteps, err)
	}
	if after := correctionPlanSnapshot(t, f, payment.ID); after != financialBefore {
		t.Fatal("correction planning changed original payment or financial fingerprints")
	}
	artifactsBeforeRepeat := correctionPlanArtifactsSnapshot(t, f)
	if steps, err := concurrentService.ProcessCorrectionPlans(context.Background(), 20); err != nil || steps != 0 {
		t.Fatalf("ready plan advanced on repeat: steps=%d err=%v", steps, err)
	}
	if after := correctionPlanArtifactsSnapshot(t, f); after != artifactsBeforeRepeat {
		t.Fatal("no-op repeat changed correction-plan or total audit fingerprints")
	}
}

func addActualChildAgentBeneficiary(t *testing.T, f commissionBatchFixture, multiplier ...points.Amount) (commissionBatchFixture, string) {
	t.Helper()
	ctx := context.Background()
	users, err := identity.New(f.betting.db)
	if err != nil {
		t.Fatal(err)
	}
	var rootCode string
	if err := f.betting.db.QueryRow(ctx, `SELECT code FROM join_codes WHERE brand_id=$1 AND agent_id=$2 AND owner_member_id=$3 AND status='active' ORDER BY created_at,id LIMIT 1`, f.betting.brand, f.node.ID, f.node.MemberID).Scan(&rootCode); err != nil {
		t.Fatal(err)
	}
	var secondMember identity.Authentication
	bettingTx(t, f.betting.db, func(tx pgx.Tx) error {
		out, err := users.Register(ctx, tx, f.betting.brand, identity.RegisterInput{
			Username: "bet_child_owner_" + strings.ReplaceAll(ids.New(), "-", "")[:12], Password: "test-commission-batch-password",
			Privacy: "dev-1", Terms: "dev-1", AgentCode: rootCode,
		}, identity.Metadata{Domain: "aurora.localhost", RequestID: ids.New()})
		if err != nil {
			return err
		}
		if out.Status != 201 {
			t.Fatalf("register actual child-agent owner status=%d error=%+v", out.Status, out.Error)
		}
		return json.Unmarshal(out.Data, &secondMember)
	})
	agents := agency.Service{DB: f.betting.db}
	agentPolicy, err := agents.Policy(ctx, f.betting.brand)
	if err != nil {
		t.Fatal(err)
	}
	parentID, parentVersion := f.node.ID, f.node.Version
	var child agency.Node
	bettingTx(t, f.betting.db, func(tx pgx.Tx) error {
		var err error
		child, err = agents.Create(ctx, tx, f.betting.brand, f.actor, agency.CreateInput{
			PolicyVersion: agentPolicy.Version, MemberID: secondMember.Member.ID, ParentID: &parentID, ParentVersion: &parentVersion,
			Config: agency.NodeConfig{Ratio: "0.1", Status: "active"}, Reason: "create actual second correction beneficiary",
		}, points.Metadata{ActorType: "admin", ActorID: f.actor.ID, RequestID: ids.New()})
		return err
	})
	codes := attribution.Service{DB: f.betting.db}
	var childCode attribution.Code
	bettingTx(t, f.betting.db, func(tx pgx.Tx) error {
		var err error
		childCode, err = codes.Create(ctx, tx, f.betting.brand, f.actor, attribution.CreateInput{
			Kind: "agent", OwnerMemberID: secondMember.Member.ID, AgentID: &child.ID, Reason: "actual child agent join code",
		}, points.Metadata{ActorType: "admin", ActorID: f.actor.ID, RequestID: ids.New()})
		return err
	})
	var player identity.Authentication
	bettingTx(t, f.betting.db, func(tx pgx.Tx) error {
		out, err := users.Register(ctx, tx, f.betting.brand, identity.RegisterInput{
			Username: "bet_child_player_" + strings.ReplaceAll(ids.New(), "-", "")[:12], Password: "test-commission-batch-password",
			Privacy: "dev-1", Terms: "dev-1", AgentCode: childCode.Code,
		}, identity.Metadata{Domain: "aurora.localhost", RequestID: ids.New()})
		if err != nil {
			return err
		}
		if out.Status != 201 {
			t.Fatalf("register actual child-agent player status=%d error=%+v", out.Status, out.Error)
		}
		return json.Unmarshal(out.Data, &player)
	})
	playerSession, err := users.Authenticate(ctx, f.betting.brand, player.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	playerFixture := f.betting
	playerFixture.user, playerFixture.member = playerSession, playerSession.Member.ID
	fundBettingWallet(t, playerFixture, 10)
	in := playerFixture.input
	in.Selection = rules.Selection{Digits: [][]int{{0}, {0}, {0}}}
	if len(multiplier) > 0 {
		in.Multiplier = multiplier[0]
	}
	order, err := placeBettingOrder(t, playerFixture, in, "commission-correction-child-beneficiary-bet")
	if err != nil || !order.PlacedAt.Before(f.boundary) {
		t.Fatalf("actual child-agent bet missed cycle boundary: %+v err=%v", order, err)
	}
	f.orders = append(f.orders, order)
	return f, child.ID
}

func TestCommissionCorrectionPlanPartialPaymentAddsUnpaidAgentFromZeroBase(t *testing.T) {
	t.Parallel()
	f := newAutomaticCommissionBatchFixture(t)
	f, childAgentID := addActualChildAgentBeneficiary(t, f)
	ctx := context.Background()
	waitCommissionBoundary(t, f.boundary)
	cycle := createCommissionCycle(t, f)
	eligibilitySettle(t, f.betting)
	advanceCommissionWorker(t, f, 40)
	cycle = readCommissionCycle(t, f, cycle.ID)
	if cycle.State != "ready" || cycle.CurrentRunID == nil || cycle.TotalPoints == "0" {
		t.Fatalf("real parent/child agent earnings did not settle: %+v", cycle)
	}
	gate := commissionPaymentPolicy(t, f)
	if err := updateCommissionPaymentPolicy(t, f, commissionPaymentActor(f), true, gate.Version); err != nil {
		t.Fatal(err)
	}
	if n, err := f.service.ProcessPayments(ctx, 1); err != nil || n != 1 {
		t.Fatalf("register actual multi-beneficiary payout: steps=%d err=%v", n, err)
	}
	payments := commissionPayments(t, f)
	if len(payments.Items) != 1 {
		t.Fatalf("fixture must produce at least two actual agent targets: %+v", payments)
	}
	payment := payments.Items[0]
	targetCount, err := strconv.Atoi(payment.TargetCount)
	if err != nil || targetCount < 2 {
		t.Fatalf("fixture must produce at least two actual agent targets: %+v", payment)
	}
	if _, err := f.service.ProcessPayments(ctx, 1); err != nil {
		t.Fatal(err)
	}
	partial := commissionPaymentRead(t, f, payment.ID)
	if partial.State != "paying" || partial.PaidCount != "1" || partial.PaidPoints == "0" {
		t.Fatalf("bounded actual payment did not leave one paid and one unpaid agent: %+v", partial)
	}
	paidTargets := commissionTargets(t, f, payment.ID)
	if len(paidTargets.Items) != 1 || paidTargets.Items[0].State != "paid" {
		t.Fatalf("only the first actually posted target should be materialized: %+v", paidTargets)
	}
	var childPaid bool
	for _, target := range paidTargets.Items {
		if target.AgentID == childAgentID && target.State == "paid" {
			childPaid = true
		}
	}
	if childPaid {
		t.Fatalf("expected the later-created child agent to remain unpaid after the first ordered target: %+v", paidTargets.Items)
	}
	correctCommissionRunToZero(t, f, payment.CycleID, payment.RunID)
	if _, err := f.betting.db.Exec(ctx, `UPDATE commission_payments SET next_work_at=clock_timestamp() WHERE brand_id=$1 AND id=$2`, f.betting.brand, payment.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessPayments(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if got := commissionPaymentRead(t, f, payment.ID); got.State != "blocked" || got.PaidCount != "1" {
		t.Fatalf("partial original payment must remain blocked without reclaim: %+v", got)
	}
	if _, err := f.service.ProcessCorrectionPlans(ctx, 20); err != nil {
		t.Fatal(err)
	}
	plans := correctionPlansRead(t, f)
	if len(plans.Items) != 1 || plans.Items[0].State != commission.CorrectionPlanReady {
		t.Fatalf("partial actual payment did not produce ready plan: %+v", plans)
	}
	targets := correctionPlanTargetsRead(t, f, plans.Items[0].ID)
	var childTarget *commission.CorrectionPlanTarget
	for i := range targets.Items {
		if targets.Items[i].AgentID == childAgentID {
			childTarget = &targets.Items[i]
		}
	}
	if childTarget == nil || childTarget.OriginalTargetID != nil || childTarget.AdjustmentVersion != nil || childTarget.EarningID == nil || childTarget.PointsBefore != 0 || childTarget.PointsAfter != 0 || childTarget.DeltaPoints != 0 {
		t.Fatalf("unpaid child agent must join actual replacement run from a zero baseline: %+v targets=%+v", childTarget, targets.Items)
	}
}
