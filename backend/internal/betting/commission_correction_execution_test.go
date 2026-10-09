package betting

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func correctionExecutionActor(f commissionBatchFixture, action string) access.Account {
	a := commissionPaymentActor(f)
	for i := range a.Roles {
		if a.Roles[i].BrandID == f.betting.brand {
			a.Roles[i].Permissions = append(a.Roles[i].Permissions,
				access.Permission{Resource: "commission_correction", Action: action, Scope: access.ScopeBrand})
		}
	}
	return a
}

func correctionExecutionPolicyActor(f commissionBatchFixture) access.Account {
	a := commissionPaymentActor(f)
	for i := range a.Roles {
		if a.Roles[i].BrandID == f.betting.brand {
			a.Roles[i].Permissions = append(a.Roles[i].Permissions,
				access.Permission{Resource: "commission_correction_policy", Action: "write", Scope: access.ScopeBrand})
		}
	}
	return a
}

func correctionExecutionPolicyRead(t *testing.T, f commissionBatchFixture) commission.CorrectionExecutionPolicy {
	t.Helper()
	tx, err := f.betting.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	policy, err := f.service.CorrectionExecutionPolicyTx(context.Background(), tx, f.betting.brand)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func updateCorrectionExecutionPolicy(t *testing.T, f commissionBatchFixture, actor access.Account, policy commission.CorrectionExecutionPolicy, enabled bool) (commission.CorrectionExecutionPolicy, error) {
	t.Helper()
	var updated commission.CorrectionExecutionPolicy
	err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		var err error
		updated, err = f.service.UpdateCorrectionExecutionPolicyTx(context.Background(), tx, f.betting.brand, actor,
			commission.PaymentPolicyInput{Version: policy.Version, Enabled: enabled, Reason: "explicitly configure independent correction execution gate"}, commissionPaymentMeta(actor))
		return err
	})
	return updated, err
}

func enableCorrectionExecutionPolicy(t *testing.T, f commissionBatchFixture) commission.CorrectionExecutionPolicy {
	t.Helper()
	before := correctionExecutionPolicyRead(t, f)
	if before.Enabled {
		t.Fatalf("fresh execution fixture unexpectedly has compensation gate enabled: %+v", before)
	}
	updated, err := updateCorrectionExecutionPolicy(t, f, correctionExecutionPolicyActor(f), before, true)
	if err != nil || !updated.Enabled || updated.Version != before.Version+1 || updated.AuditLogID == "" {
		t.Fatalf("explicitly opt in to independent correction execution: before=%+v after=%+v err=%v", before, updated, err)
	}
	return updated
}

func correctionExecutionRead(t *testing.T, f commissionBatchFixture, id string) commission.CorrectionExecution {
	t.Helper()
	tx, err := f.betting.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	out, err := f.service.CorrectionExecutionTx(context.Background(), tx, f.betting.brand, id)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func correctionExecutionsRead(t *testing.T, f commissionBatchFixture) commission.CorrectionExecutionPage {
	t.Helper()
	tx, err := f.betting.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	page, err := f.service.CorrectionExecutionsTx(context.Background(), tx, f.betting.brand, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func correctionExecutionTargetsRead(t *testing.T, f commissionBatchFixture, id string) commission.CorrectionExecutionTargetPage {
	t.Helper()
	tx, err := f.betting.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	page, err := f.service.CorrectionExecutionTargetsTx(context.Background(), tx, f.betting.brand, id, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func readyAutomaticCorrectionPlanFixture(t *testing.T) (commissionBatchFixture, commission.Payment, commission.CorrectionPlan) {
	t.Helper()
	f, payment, _ := createAndPayAutomaticCorrectionFixture(t)
	correctCommissionRunToZero(t, f, payment.CycleID, payment.RunID)
	if _, err := f.service.ProcessPayments(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	if got := commissionPaymentRead(t, f, payment.ID); got.State != "blocked" || got.PaidCount != "1" {
		t.Fatalf("correction source payment is not blocked while preserving prior credit: %+v", got)
	}
	if _, err := f.service.ProcessCorrectionPlans(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	plans := correctionPlansRead(t, f)
	if len(plans.Items) != 1 || plans.Items[0].State != commission.CorrectionPlanReady {
		t.Fatalf("real corrected run did not produce ready preview: %+v", plans)
	}
	return f, payment, plans.Items[0]
}

func readyAutomaticCorrectionExecutionFixture(t *testing.T) (commissionBatchFixture, commission.Payment, commission.CorrectionPlan) {
	t.Helper()
	f, payment, plan := readyAutomaticCorrectionPlanFixture(t)
	enableCorrectionExecutionPolicy(t, f)
	return f, payment, plan
}

func readyPartialCorrectionExecutionFixture(t *testing.T, multiplier ...points.Amount) (commissionBatchFixture, commission.Payment, commission.CorrectionPlan, string) {
	t.Helper()
	f := newAutomaticCommissionBatchFixture(t)
	f, childAgentID := addActualChildAgentBeneficiary(t, f, multiplier...)
	ctx := context.Background()
	waitCommissionBoundary(t, f.boundary)
	cycle := createCommissionCycle(t, f)
	eligibilitySettle(t, f.betting)
	advanceCommissionWorker(t, f, 40)
	cycle = readCommissionCycle(t, f, cycle.ID)
	if cycle.State != "ready" || cycle.CurrentRunID == nil || cycle.TotalPoints == "0" {
		t.Fatalf("parent and child agents did not produce actual eligible earnings: %+v", cycle)
	}
	gate := commissionPaymentPolicy(t, f)
	if err := updateCommissionPaymentPolicy(t, f, commissionPaymentActor(f), true, gate.Version); err != nil {
		t.Fatal(err)
	}
	if n, err := f.service.ProcessPayments(ctx, 1); err != nil || n != 1 {
		t.Fatalf("register actual multi-agent payment: steps=%d err=%v", n, err)
	}
	paymentPage := commissionPayments(t, f)
	if len(paymentPage.Items) != 1 {
		t.Fatalf("multi-agent payment page=%+v", paymentPage)
	}
	payment := paymentPage.Items[0]
	if _, err := f.service.ProcessPayments(ctx, 1); err != nil {
		t.Fatal(err)
	}
	partial := commissionPaymentRead(t, f, payment.ID)
	if partial.State != "paying" || partial.PaidCount != "1" || partial.PaidPoints == "0" {
		t.Fatalf("expected an actually partial multi-agent payment: %+v", partial)
	}
	paidTargets := commissionTargets(t, f, payment.ID)
	if len(paidTargets.Items) != 1 || paidTargets.Items[0].AgentID == childAgentID {
		t.Fatalf("child agent should remain unpaid in the first bounded posting: %+v", paidTargets.Items)
	}
	correctCommissionRunToZero(t, f, payment.CycleID, payment.RunID)
	if _, err := f.betting.db.Exec(ctx, `UPDATE commission_payments SET next_work_at=clock_timestamp() WHERE brand_id=$1 AND id=$2`, f.betting.brand, payment.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessPayments(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if got := commissionPaymentRead(t, f, payment.ID); got.State != "blocked" || got.PaidCount != "1" {
		t.Fatalf("corrected partial payment lost its paid subset: %+v", got)
	}
	if _, err := f.service.ProcessCorrectionPlans(ctx, 100); err != nil {
		t.Fatal(err)
	}
	plans := correctionPlansRead(t, f)
	if len(plans.Items) != 1 || plans.Items[0].State != commission.CorrectionPlanReady {
		t.Fatalf("partial payment correction plan=%+v", plans)
	}
	enableCorrectionExecutionPolicy(t, f)
	return f, payment, plans.Items[0], childAgentID
}

func ensureCorrectionExecution(t *testing.T, f commissionBatchFixture) commission.CorrectionExecution {
	t.Helper()
	if _, err := f.service.ProcessCorrectionExecutions(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	page := correctionExecutionsRead(t, f)
	if page.TotalCount != "1" || len(page.Items) != 1 {
		t.Fatalf("expected exactly one execution for the ready plan: %+v", page)
	}
	return page.Items[0]
}

type correctionExecBucket struct {
	AccountID string `json:"account_id"`
	MemberID  string `json:"member_id"`
	Source    string `json:"source"`
	State     string `json:"state"`
	Points    int64  `json:"points"`
}

func correctionExecBuckets(t *testing.T, f commissionBatchFixture) map[string]correctionExecBucket {
	t.Helper()
	var raw []byte
	err := f.betting.db.QueryRow(context.Background(), `SELECT coalesce(jsonb_agg(jsonb_build_object(
	 'account_id',a.id::text,'member_id',a.brand_member_id::text,'source',b.source,'state',b.state,'points',b.points)
	 ORDER BY a.id,b.source,b.state),'[]'::jsonb) FROM point_buckets b JOIN point_accounts a ON a.brand_id=b.brand_id AND a.id=b.account_id WHERE b.brand_id=$1`, f.betting.brand).Scan(&raw)
	if err != nil {
		t.Fatal(err)
	}
	var rows []correctionExecBucket
	if err = json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	out := make(map[string]correctionExecBucket, len(rows))
	for _, row := range rows {
		out[row.AccountID+":"+row.Source+":"+row.State] = row
	}
	return out
}

func assertCorrectionExecOnlyCommissionBucketDelta(t *testing.T, before, after map[string]correctionExecBucket, member string, delta int64) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("bucket cardinality changed: before=%d after=%d", len(before), len(after))
	}
	changes := 0
	for key, old := range before {
		got, ok := after[key]
		if !ok {
			t.Fatalf("bucket disappeared: %s", key)
		}
		want := old.Points
		if old.MemberID == member && old.Source == "commission" && old.State == "available" {
			want += delta
		}
		if got.Points != want {
			t.Fatalf("unexpected bucket mutation at %s: before=%d after=%d want=%d", key, old.Points, got.Points, want)
		}
		if got.Points != old.Points {
			changes++
		}
	}
	wantChanges := 0
	if delta != 0 {
		wantChanges = 1
	}
	if changes != wantChanges {
		t.Fatalf("changed bucket count=%d want=%d", changes, wantChanges)
	}
}

type correctionExecutionPostingSnapshot struct {
	LedgerRows, BalanceHeads string
}

func correctionExecutionPostingRead(t *testing.T, f commissionBatchFixture) correctionExecutionPostingSnapshot {
	t.Helper()
	var out correctionExecutionPostingSnapshot
	err := f.betting.db.QueryRow(context.Background(), `SELECT
	 coalesce((SELECT jsonb_agg(to_jsonb(l) ORDER BY l.id)::text FROM point_ledger_entries l WHERE l.brand_id=$1),'[]'),
	 coalesce((SELECT jsonb_agg(to_jsonb(h) ORDER BY h.cycle_id,h.agent_id)::text FROM commission_correction_balance_heads h WHERE h.brand_id=$1),'[]')`, f.betting.brand).
		Scan(&out.LedgerRows, &out.BalanceHeads)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

type correctionExecutionOriginalEvidence struct {
	Payment, Targets, OriginalLedgers, AdjustmentHeads, Calculations, Earnings, Outbox, Deliveries string
}

func correctionExecutionOriginalSnapshot(t *testing.T, f commissionBatchFixture, paymentID string) correctionExecutionOriginalEvidence {
	t.Helper()
	ctx := context.Background()
	var out correctionExecutionOriginalEvidence
	err := f.betting.db.QueryRow(ctx, `SELECT
	 coalesce((SELECT to_jsonb(p)::text FROM commission_payments p WHERE p.brand_id=$1 AND p.id=$2),'null'),
	 coalesce((SELECT jsonb_agg(to_jsonb(t) ORDER BY t.id)::text FROM commission_payment_targets t WHERE t.brand_id=$1 AND t.payment_id=$2),'[]'),
	 coalesce((SELECT jsonb_agg(to_jsonb(l) ORDER BY l.id)::text FROM point_ledger_entries l WHERE l.brand_id=$1 AND l.reference_type='commission_payment_target' AND l.reference_id IN(SELECT id FROM commission_payment_targets WHERE payment_id=$2)),'[]'),
	 coalesce((SELECT jsonb_agg(to_jsonb(h) ORDER BY h.target_id)::text FROM commission_adjustment_heads h WHERE h.brand_id=$1),'[]'),
	 coalesce((SELECT jsonb_agg(to_jsonb(c) ORDER BY c.id)::text FROM commission_calculations c WHERE c.brand_id=$1),'[]'),
	 coalesce((SELECT jsonb_agg(to_jsonb(e) ORDER BY e.id)::text FROM commission_earnings e WHERE e.brand_id=$1),'[]'),
	 coalesce((SELECT jsonb_agg(to_jsonb(o) ORDER BY o.id)::text FROM outbox_events o WHERE o.brand_id=$1 AND o.event_type<>'commission.corrected'),'[]'),
	 coalesce((SELECT jsonb_agg(to_jsonb(d) ORDER BY d.event_id)::text FROM notification_deliveries d JOIN outbox_events e ON e.id=d.event_id WHERE d.brand_id=$1 AND e.event_type<>'commission.corrected'),'[]')`, f.betting.brand, paymentID).
		Scan(&out.Payment, &out.Targets, &out.OriginalLedgers, &out.AdjustmentHeads, &out.Calculations, &out.Earnings, &out.Outbox, &out.Deliveries)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCommissionCorrectionExecutionAutomaticDebitPostsOnlyCommissionAvailable(t *testing.T) {
	t.Parallel()
	f, payment, plan := readyAutomaticCorrectionExecutionFixture(t)
	bucketsBefore := correctionExecBuckets(t, f)
	evidenceBefore := correctionExecutionOriginalSnapshot(t, f, payment.ID)
	execution := ensureCorrectionExecution(t, f)
	if execution.State != commission.CorrectionExecutionCompleted || execution.PlanID != plan.ID || execution.RunID != plan.RunID || execution.PayoutMode != commission.PayoutAutomatic || execution.CreditPoints != "0" || execution.DebitPoints != "1" || execution.NetPoints != "-1" || execution.AppliedCreditPoints != "0" || execution.AppliedDebitPoints != "1" || execution.TargetCount != "1" || execution.AppliedCount != "1" || execution.CycleHoldActive {
		t.Fatalf("automatic one-point debit execution summary=%+v", execution)
	}
	targets := correctionExecutionTargetsRead(t, f, execution.ID)
	if targets.TotalCount != "1" || len(targets.Items) != 1 {
		t.Fatalf("automatic execution targets=%+v", targets)
	}
	target := targets.Items[0]
	if target.State != commission.CorrectionExecutionTargetApplied || target.PointsBefore != 1 || target.PointsAfter != 0 || target.DeltaPoints != -1 || target.LedgerEntryID == nil || target.AuditLogID == nil || target.FinancialVersion == nil || target.AppliedAt == nil {
		t.Fatalf("actual debit target missing application witness: %+v", target)
	}
	var entryType, referenceType, referenceID, operationKey, delta string
	if err := f.betting.db.QueryRow(context.Background(), `SELECT entry_type,reference_type,reference_id::text,operation_key,delta_snapshot->'commission'->>'available' FROM point_ledger_entries WHERE id=$1`, *target.LedgerEntryID).Scan(&entryType, &referenceType, &referenceID, &operationKey, &delta); err != nil {
		t.Fatal(err)
	}
	if entryType != "commission_correction" || referenceType != "commission_correction_target" || referenceID != target.ID || operationKey != "commission-correction:"+target.ID || delta != "-1" {
		t.Fatalf("correction ledger witness type=%s ref=%s/%s op=%s delta=%s", entryType, referenceType, referenceID, operationKey, delta)
	}
	assertCorrectionExecOnlyCommissionBucketDelta(t, bucketsBefore, correctionExecBuckets(t, f), f.node.MemberID, -1)
	if got := correctionExecutionOriginalSnapshot(t, f, payment.ID); !reflect.DeepEqual(got, evidenceBefore) {
		t.Fatalf("execution rewrote original payment/targets/ledger/heads/calculations/earnings/outbox/deliveries\nbefore=%+v\nafter=%+v", evidenceBefore, got)
	}
	if got := commissionPaymentRead(t, f, payment.ID); got.State != "blocked" || got.PaidPoints != "1" || got.PaidCount != "1" {
		t.Fatalf("execution must not unblock or rewrite original payment: %+v", got)
	}
}

func TestCommissionCorrectionExecutionHasIndependentDefaultOffPolicyGate(t *testing.T) {
	t.Parallel()
	f, payment, plan := readyAutomaticCorrectionPlanFixture(t)
	legacyGate := commissionPaymentPolicy(t, f)
	if !legacyGate.Enabled {
		t.Fatalf("fixture must have the independent original payout gate enabled: %+v", legacyGate)
	}
	policy := correctionExecutionPolicyRead(t, f)
	if policy.Enabled || policy.Version < 1 {
		t.Fatalf("execution policy must default disabled regardless of old payment gate: %+v", policy)
	}
	if n, err := f.service.ProcessCorrectionExecutions(context.Background(), 100); err != nil || n != 0 {
		t.Fatalf("legacy payout gate alone must not admit correction execution: steps=%d err=%v", n, err)
	}
	if page := correctionExecutionsRead(t, f); page.TotalCount != "0" || len(page.Items) != 0 {
		t.Fatalf("default-off gate allowed an automatic plan to become executable: %+v", page)
	}
	if got := commissionPaymentRead(t, f, payment.ID); got.State != "blocked" {
		t.Fatalf("gate check mutated the original blocked payment: %+v", got)
	}
	// The legacy payout-policy writer can write the old gate, but not this one.
	_, err := updateCorrectionExecutionPolicy(t, f, commissionPaymentActor(f), policy, true)
	if !errors.Is(err, commission.ErrDenied) {
		t.Fatalf("commission_payment_policy.write unexpectedly changed execution policy: %v", err)
	}
	// Having the new permission without the mandatory commission.view right is
	// also insufficient.
	noView := f.actor
	noView.Roles = append(noView.Roles[:0:0], noView.Roles...)
	for i := range noView.Roles {
		if noView.Roles[i].BrandID == f.betting.brand {
			filtered := make([]access.Permission, 0, len(noView.Roles[i].Permissions)+1)
			for _, permission := range noView.Roles[i].Permissions {
				if permission.Resource == "commission" && permission.Action == "view" || permission.Resource == "commission_correction_policy" {
					continue
				}
				filtered = append(filtered, permission)
			}
			noView.Roles[i].Permissions = filtered
			noView.Roles[i].Permissions = append(noView.Roles[i].Permissions,
				access.Permission{Resource: "commission_correction_policy", Action: "write", Scope: access.ScopeBrand})
		}
	}
	if _, err := updateCorrectionExecutionPolicy(t, f, noView, policy, true); !errors.Is(err, commission.ErrDenied) {
		t.Fatalf("policy writer without commission.view changed execution policy: %v", err)
	}
	updated := enableCorrectionExecutionPolicy(t, f)
	if updated.Version != policy.Version+1 {
		t.Fatalf("execution gate update was not versioned: before=%+v after=%+v", policy, updated)
	}
	if n, err := f.service.ProcessCorrectionExecutions(context.Background(), 100); err != nil || n == 0 {
		t.Fatalf("explicitly enabled gate did not admit ready automatic plan %s: steps=%d err=%v", plan.ID, n, err)
	}
	executions := correctionExecutionsRead(t, f)
	if executions.TotalCount != "1" || len(executions.Items) != 1 || executions.Items[0].State != commission.CorrectionExecutionCompleted {
		t.Fatalf("opted-in compensation execution=%+v", executions)
	}
}

func TestCommissionCorrectionExecutionZeroDeltaTargetHasNoLedgerPosting(t *testing.T) {
	t.Parallel()
	f, _, plan, childAgentID := readyPartialCorrectionExecutionFixture(t)
	bucketsBefore := correctionExecBuckets(t, f)
	planTargets := correctionPlanTargetsRead(t, f, plan.ID)
	var childPlanTarget bool
	for _, target := range planTargets.Items {
		if target.AgentID == childAgentID && target.OriginalTargetID == nil && target.PointsBefore == 0 && target.PointsAfter == 0 && target.DeltaPoints == 0 {
			childPlanTarget = true
		}
	}
	if !childPlanTarget {
		t.Fatalf("unpaid new child beneficiary missing from zero baseline preview: %+v", planTargets.Items)
	}
	execution := ensureCorrectionExecution(t, f)
	if execution.State != commission.CorrectionExecutionCompleted || execution.TargetCount != "2" || execution.AppliedCount != "2" || execution.DebitPoints != "1" || execution.AppliedDebitPoints != "1" {
		t.Fatalf("partial-payment execution summary=%+v", execution)
	}
	targets := correctionExecutionTargetsRead(t, f, execution.ID)
	var childExecutionTarget *commission.CorrectionExecutionTarget
	for i := range targets.Items {
		if targets.Items[i].AgentID == childAgentID {
			childExecutionTarget = &targets.Items[i]
		}
	}
	if childExecutionTarget == nil || childExecutionTarget.State != commission.CorrectionExecutionTargetApplied || childExecutionTarget.DeltaPoints != 0 || childExecutionTarget.LedgerEntryID != nil || childExecutionTarget.AuditLogID == nil || childExecutionTarget.FinancialVersion == nil {
		t.Fatalf("zero-delta child must advance actual head with audit but no ledger: %+v", childExecutionTarget)
	}
	var zeroLedgerCount int
	if err := f.betting.db.QueryRow(context.Background(), `SELECT count(*) FROM point_ledger_entries WHERE brand_id=$1 AND reference_id=$2`, f.betting.brand, childExecutionTarget.ID).Scan(&zeroLedgerCount); err != nil || zeroLedgerCount != 0 {
		t.Fatalf("zero-delta execution target created ledger entry count=%d err=%v", zeroLedgerCount, err)
	}
	assertCorrectionExecOnlyCommissionBucketDelta(t, bucketsBefore, correctionExecBuckets(t, f), f.node.MemberID, -1)
}

func TestCommissionCorrectionExecutionPartialOriginalPaymentCarriesActualHeadsAcrossRuns(t *testing.T) {
	t.Parallel()
	f := newAutomaticCommissionBatchFixture(t)
	f, childAgentID := addActualChildAgentBeneficiary(t, f, 10)
	ctx := context.Background()
	waitCommissionBoundary(t, f.boundary)
	cycle := createCommissionCycle(t, f)
	eligibilitySettle(t, f.betting)
	advanceCommissionWorker(t, f, 40)
	cycle = readCommissionCycle(t, f, cycle.ID)
	if cycle.State != "ready" || cycle.TotalPoints != "4" || cycle.CurrentRunID == nil {
		t.Fatalf("real parent/child settlement did not produce four actual points: %+v", cycle)
	}
	gate := commissionPaymentPolicy(t, f)
	if err := updateCommissionPaymentPolicy(t, f, commissionPaymentActor(f), true, gate.Version); err != nil {
		t.Fatal(err)
	}
	if n, err := f.service.ProcessPayments(ctx, 1); err != nil || n != 1 {
		t.Fatalf("register actual partial source payment: steps=%d err=%v", n, err)
	}
	paymentPage := commissionPayments(t, f)
	if len(paymentPage.Items) != 1 {
		t.Fatalf("source payment page=%+v", paymentPage)
	}
	payment := paymentPage.Items[0]
	if n, err := f.service.ProcessPayments(ctx, 1); err != nil || n != 1 {
		t.Fatalf("post only the first actual beneficiary: steps=%d err=%v", n, err)
	}
	partial := commissionPaymentRead(t, f, payment.ID)
	paidTargets := commissionTargets(t, f, payment.ID)
	if partial.State != "paying" || partial.PaidCount != "1" || partial.PaidPoints != "3" || len(paidTargets.Items) != 1 || paidTargets.Items[0].AgentID != f.node.ID || paidTargets.Items[0].OriginalPoints != 3 {
		t.Fatalf("expected real root-only three-point partial payment: payment=%+v targets=%+v", partial, paidTargets.Items)
	}

	// A real replacement result that still loses creates a four-point run:
	// the paid root remains 3 and the previously unpaid child newly earns 1.
	current := applyAnotherActualCommissionCorrection(t, f, 1, 2, 2)
	if current.TotalPoints != "4" || current.CurrentRunID == nil || *current.CurrentRunID == payment.RunID {
		t.Fatalf("actual 122 correction did not make a distinct four-point run: %+v", current)
	}
	if _, err := f.service.ProcessPayments(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if got := commissionPaymentRead(t, f, payment.ID); got.State != "blocked" || got.PaidPoints != "3" || got.PaidCount != "1" {
		t.Fatalf("correction must retain only the original root posting: %+v", got)
	}
	if _, err := f.service.ProcessCorrectionPlans(ctx, 100); err != nil {
		t.Fatal(err)
	}
	plans := correctionPlansRead(t, f)
	if len(plans.Items) != 1 || plans.Items[0].State != commission.CorrectionPlanReady || plans.Items[0].BeforePoints != "3" || plans.Items[0].CalculatedPoints != "4" || plans.Items[0].CreditPoints == nil || *plans.Items[0].CreditPoints != "1" || plans.Items[0].DebitPoints == nil || *plans.Items[0].DebitPoints != "0" || plans.Items[0].NetPoints == nil || *plans.Items[0].NetPoints != "1" {
		t.Fatalf("partial source must plan only the new child's actual one point: %+v", plans)
	}
	plan := plans.Items[0]
	planTargets := correctionPlanTargetsRead(t, f, plan.ID)
	var rootTarget, childTarget *commission.CorrectionPlanTarget
	for i := range planTargets.Items {
		target := &planTargets.Items[i]
		if target.AgentID == f.node.ID {
			rootTarget = target
		}
		if target.AgentID == childAgentID {
			childTarget = target
		}
	}
	if rootTarget == nil || rootTarget.PointsBefore != 3 || rootTarget.PointsAfter != 3 || rootTarget.DeltaPoints != 0 || childTarget == nil || childTarget.PointsBefore != 0 || childTarget.PointsAfter != 1 || childTarget.DeltaPoints != 1 || childTarget.OriginalTargetID != nil {
		t.Fatalf("ready partial difference plan did not preserve root baseline and new child baseline: %+v", planTargets.Items)
	}
	enableCorrectionExecutionPolicy(t, f)
	if n, err := f.service.ProcessCorrectionExecutions(ctx, 2); err != nil || n != 2 {
		t.Fatalf("create execution then apply only the root zero-difference head: steps=%d err=%v", n, err)
	}
	executions := correctionExecutionsRead(t, f)
	if len(executions.Items) != 1 || executions.Items[0].State != commission.CorrectionExecutionApplying || executions.Items[0].TargetCount != "2" || executions.Items[0].AppliedCount != "1" || executions.Items[0].AppliedDebitPoints != "0" || executions.Items[0].AppliedCreditPoints != "0" {
		t.Fatalf("first bounded application should persist only root's zero head: %+v", executions)
	}
	execution := executions.Items[0]
	firstTargets := correctionExecutionTargetsRead(t, f, execution.ID)
	if firstTargets.TotalCount != "1" || len(firstTargets.Items) != 1 || firstTargets.Items[0].AgentID != f.node.ID || firstTargets.Items[0].DeltaPoints != 0 || firstTargets.Items[0].LedgerEntryID != nil || firstTargets.Items[0].FinancialVersion == nil || *firstTargets.Items[0].FinancialVersion != 1 {
		t.Fatalf("root zero-difference target/head=%+v", firstTargets)
	}
	if rootWallet := commissionWalletBySource(t, f, f.node.MemberID); rootWallet[3][0] != 3 {
		t.Fatalf("zero-difference head duplicated or debited the original root payment: %+v", rootWallet)
	}
	if n, err := f.service.ProcessCorrectionExecutions(ctx, 1); err != nil || n != 1 {
		t.Fatalf("apply only the previously unpaid child's real +1: steps=%d err=%v", n, err)
	}
	secondTargets := correctionExecutionTargetsRead(t, f, execution.ID)
	var appliedRoot, appliedChild *commission.CorrectionExecutionTarget
	for i := range secondTargets.Items {
		target := &secondTargets.Items[i]
		if target.AgentID == f.node.ID {
			appliedRoot = target
		}
		if target.AgentID == childAgentID {
			appliedChild = target
		}
	}
	if execution = correctionExecutionRead(t, f, execution.ID); execution.State != commission.CorrectionExecutionApplying || execution.AppliedCount != "2" || execution.AppliedCreditPoints != "1" || execution.AppliedDebitPoints != "0" || appliedRoot == nil || appliedRoot.DeltaPoints != 0 || appliedChild == nil || appliedChild.DeltaPoints != 1 || appliedChild.PointsBefore != 0 || appliedChild.PointsAfter != 1 || appliedChild.LedgerEntryID == nil || appliedChild.FinancialVersion == nil || *appliedChild.FinancialVersion != 1 {
		t.Fatalf("partial actual application duplicated root or omitted child credit: execution=%+v targets=%+v", execution, secondTargets.Items)
	}
	assertActualCorrectionEvent(t, f, *appliedRoot)
	assertActualCorrectionEvent(t, f, *appliedChild)
	if rootWallet := commissionWalletBySource(t, f, f.node.MemberID); rootWallet[3][0] != 3 {
		t.Fatalf("child credit duplicated root's original three points: %+v", rootWallet)
	}
	if childWallet := commissionWalletBySource(t, f, childTarget.MemberID); childWallet[3][0] != 1 {
		t.Fatalf("actual child beneficiary did not receive exactly one commission point: %+v", childWallet)
	}

	// Supersede the still-applying execution with another actual result before
	// its separate completion step. Applied heads/history must survive staling.
	zeroRun := applyAnotherActualCommissionCorrection(t, f, 0, 0, 0)
	if zeroRun.TotalPoints != "0" || zeroRun.CurrentRunID == nil || *zeroRun.CurrentRunID == *current.CurrentRunID {
		t.Fatalf("second actual correction did not create a new zero run: %+v", zeroRun)
	}
	if n, err := f.service.ProcessCorrectionExecutions(ctx, 1); err != nil || n != 1 {
		t.Fatalf("stale the interrupted prior execution while preserving its target history: steps=%d err=%v", n, err)
	}
	stale := correctionExecutionRead(t, f, execution.ID)
	if stale.State != commission.CorrectionExecutionStale || stale.AppliedCount != "2" || stale.AppliedCreditPoints != "1" || stale.AppliedDebitPoints != "0" {
		t.Fatalf("staling erased already-applied partial correction history: %+v", stale)
	}
	assertActualCorrectionEvent(t, f, *appliedChild)
	staleTargets := correctionExecutionTargetsRead(t, f, stale.ID)
	if staleTargets.TotalCount != "2" || len(staleTargets.Items) != 2 {
		t.Fatalf("staling removed actual zero/+1 target history: %+v", staleTargets)
	}
	if _, err := f.service.ProcessPayments(ctx, 100); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessCorrectionPlans(ctx, 100); err != nil {
		t.Fatal(err)
	}
	currentPlans := correctionPlansRead(t, f)
	var nextPlan *commission.CorrectionPlan
	for i := range currentPlans.Items {
		if currentPlans.Items[i].State == commission.CorrectionPlanReady && currentPlans.Items[i].RunID == *zeroRun.CurrentRunID {
			nextPlan = &currentPlans.Items[i]
		}
	}
	if nextPlan == nil || nextPlan.BeforePoints != "4" || nextPlan.CalculatedPoints != "0" || nextPlan.CreditPoints == nil || *nextPlan.CreditPoints != "0" || nextPlan.DebitPoints == nil || *nextPlan.DebitPoints != "4" || nextPlan.NetPoints == nil || *nextPlan.NetPoints != "-4" {
		t.Fatalf("new plan must recover actual 3+1 head rather than resend or ignore it: plans=%+v", currentPlans)
	}
	nextTargets := correctionPlanTargetsRead(t, f, nextPlan.ID)
	var nextRoot, nextChild *commission.CorrectionPlanTarget
	for i := range nextTargets.Items {
		target := &nextTargets.Items[i]
		if target.AgentID == f.node.ID {
			nextRoot = target
		}
		if target.AgentID == childAgentID {
			nextChild = target
		}
	}
	if nextRoot == nil || nextRoot.PointsBefore != 3 || nextRoot.PointsAfter != 0 || nextRoot.DeltaPoints != -3 || nextRoot.PreviousCorrectionTargetID == nil || nextChild == nil || nextChild.PointsBefore != 1 || nextChild.PointsAfter != 0 || nextChild.DeltaPoints != -1 || nextChild.PreviousCorrectionTargetID == nil || *nextChild.PreviousCorrectionTargetID != appliedChild.ID || nextChild.FinancialVersion == nil || *nextChild.FinancialVersion != *appliedChild.FinancialVersion {
		t.Fatalf("next plan must carry the previously unpaid child's actual financial head: %+v", nextTargets.Items)
	}
	if rootWallet := commissionWalletBySource(t, f, f.node.MemberID); rootWallet[3][0] != 3 {
		t.Fatalf("planning next debit changed root funds: %+v", rootWallet)
	}
	if childWallet := commissionWalletBySource(t, f, nextChild.MemberID); childWallet[3][0] != 1 {
		t.Fatalf("planning next debit changed child funds: %+v", childWallet)
	}
	originalFinancialEvidence := correctionExecutionOriginalSnapshot(t, f, payment.ID)
	priorExecutionHistory := append([]commission.CorrectionExecutionTarget(nil), staleTargets.Items...)
	if n, err := f.service.ProcessCorrectionExecutions(ctx, 100); err != nil || n == 0 {
		t.Fatalf("apply the current actual -3/-1 differences: steps=%d err=%v", n, err)
	}
	allExecutions := correctionExecutionsRead(t, f)
	var recoveryExecution *commission.CorrectionExecution
	for i := range allExecutions.Items {
		if allExecutions.Items[i].PlanID == nextPlan.ID {
			recoveryExecution = &allExecutions.Items[i]
		}
	}
	if recoveryExecution == nil || recoveryExecution.State != commission.CorrectionExecutionCompleted || recoveryExecution.DebitPoints != "4" || recoveryExecution.AppliedDebitPoints != "4" || recoveryExecution.CreditPoints != "0" || recoveryExecution.AppliedCreditPoints != "0" || recoveryExecution.AppliedCount != "2" {
		t.Fatalf("actual 3+1 recovery execution=%+v all=%+v", recoveryExecution, allExecutions.Items)
	}
	recoveryTargets := correctionExecutionTargetsRead(t, f, recoveryExecution.ID)
	var recoveredRoot, recoveredChild *commission.CorrectionExecutionTarget
	for i := range recoveryTargets.Items {
		target := &recoveryTargets.Items[i]
		if target.AgentID == f.node.ID {
			recoveredRoot = target
		}
		if target.AgentID == childAgentID {
			recoveredChild = target
		}
	}
	if recoveryTargets.TotalCount != "2" || recoveredRoot == nil || recoveredRoot.DeltaPoints != -3 || recoveredRoot.FinancialVersion == nil || *recoveredRoot.FinancialVersion != 2 || recoveredChild == nil || recoveredChild.DeltaPoints != -1 || recoveredChild.FinancialVersion == nil || *recoveredChild.FinancialVersion != 2 {
		t.Fatalf("final recovery did not advance both actual financial heads to version 2: %+v", recoveryTargets)
	}
	if wallet := commissionWalletBySource(t, f, f.node.MemberID); wallet[3][0] != 0 {
		t.Fatalf("actual root -3 recovery did not clear commission.available: %+v", wallet)
	}
	if wallet := commissionWalletBySource(t, f, nextChild.MemberID); wallet[3][0] != 0 {
		t.Fatalf("actual child -1 recovery did not clear commission.available: %+v", wallet)
	}
	if got := correctionExecutionOriginalSnapshot(t, f, payment.ID); !reflect.DeepEqual(got, originalFinancialEvidence) {
		t.Fatalf("final recovery rewrote original payment/targets/ledger/heads/calculations/earnings/notifications: before=%+v after=%+v", originalFinancialEvidence, got)
	}
	if got := correctionExecutionTargetsRead(t, f, stale.ID); !reflect.DeepEqual(got.Items, priorExecutionHistory) || got.TotalCount != "2" {
		t.Fatalf("final recovery erased the old zero/+1 applied history: before=%+v after=%+v", priorExecutionHistory, got.Items)
	}
}

func TestCommissionCorrectionExecutionManualApprovalIsIndependentFromPaymentApproval(t *testing.T) {
	t.Parallel()
	f := newCommissionBatchFixture(t)
	payment := payManualCommissionForAdjustment(t, f, prepareManualCommissionPayment(t, f))
	correctCommissionRunToZero(t, f, payment.CycleID, payment.RunID)
	if _, err := f.service.ProcessPayments(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessCorrectionPlans(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	planPage := correctionPlansRead(t, f)
	if len(planPage.Items) != 1 || planPage.Items[0].State != commission.CorrectionPlanReady || planPage.Items[0].PayoutMode != commission.PayoutManual {
		t.Fatalf("manual source plan=%+v", planPage)
	}
	enableCorrectionExecutionPolicy(t, f)
	if _, err := f.service.ProcessCorrectionExecutions(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	page := correctionExecutionsRead(t, f)
	if len(page.Items) != 1 || page.Items[0].State != commission.CorrectionExecutionAwaitingApproval || page.Items[0].PayoutMode != commission.PayoutManual {
		t.Fatalf("manual correction execution should await fresh approval: %+v", page)
	}
	execution := page.Items[0]
	legacyApprover := commissionPaymentActor(f) // includes commission_payment.approve only.
	err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, err := f.service.ApproveCorrectionExecutionTx(context.Background(), tx, f.betting.brand, execution.ID, legacyApprover,
			commission.RetryCycleInput{Version: execution.Version, Reason: "legacy payout approval cannot approve compensation"}, commissionPaymentMeta(legacyApprover))
		return err
	})
	if !errors.Is(err, commission.ErrDenied) {
		t.Fatalf("old commission_payment.approve unexpectedly authorized correction execution: %v", err)
	}
	approver := correctionExecutionActor(f, "approve")
	updated, err := approveCorrectionExecution(t, f, execution, approver)
	if err != nil || updated.State != commission.CorrectionExecutionApplying || updated.ApprovedBy == nil || *updated.ApprovedBy != approver.ID || updated.ApprovalActorType == nil || *updated.ApprovalActorType != "admin" || updated.ApprovalAuditLogID == nil {
		t.Fatalf("fresh manual execution approval witness=%+v err=%v", updated, err)
	}
	if _, err := f.service.ProcessCorrectionExecutions(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	if got := correctionExecutionRead(t, f, execution.ID); got.State != commission.CorrectionExecutionCompleted {
		t.Fatalf("approved manual correction execution did not complete: %+v", got)
	}
}

func approveCorrectionExecution(t *testing.T, f commissionBatchFixture, execution commission.CorrectionExecution, actor access.Account) (commission.CorrectionExecution, error) {
	t.Helper()
	var updated commission.CorrectionExecution
	err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		var err error
		updated, err = f.service.ApproveCorrectionExecutionTx(context.Background(), tx, f.betting.brand, execution.ID, actor,
			commission.RetryCycleInput{Version: execution.Version, Reason: "fresh approval for actual correction execution"}, commissionPaymentMeta(actor))
		return err
	})
	return updated, err
}

func postCorrectionExecutionCommissionFreeze(t *testing.T, f commissionBatchFixture, amount points.Amount, freeze bool) {
	t.Helper()
	from, to, state := "available", "manual_frozen", "freeze"
	if !freeze {
		from, to, state = "manual_frozen", "available", "unfreeze"
	}
	allocation := []points.Allocation{{Source: "commission", State: from, Points: amount}}
	delta, err := points.AllocationDelta(allocation, from, to)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.betting.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = (points.Store{DB: f.betting.db}).Post(context.Background(), tx, points.Change{
		BrandID: f.betting.brand, MemberID: f.node.MemberID, EntryType: state, ReferenceType: "wallet_freeze",
		ReferenceID: ids.New(), OperationKey: "correction-execution-" + state + "-" + ids.New(),
		Reason: "test correction available-source hold", ActorType: "admin", ActorID: f.actor.ID, RequestID: ids.New(),
		Delta: delta, Allocation: allocation,
	})
	if err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func continueCorrectionExecution(t *testing.T, f commissionBatchFixture, execution commission.CorrectionExecution, actor access.Account) (commission.CorrectionExecution, error) {
	t.Helper()
	var updated commission.CorrectionExecution
	err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		var err error
		updated, err = f.service.ContinueCorrectionExecutionTx(context.Background(), tx, f.betting.brand, execution.ID, actor,
			commission.RetryCycleInput{Version: execution.Version, Reason: "explicitly continue held correction execution"}, commissionPaymentMeta(actor))
		return err
	})
	return updated, err
}

func TestCommissionCorrectionExecutionMixedModeRequiresFreshApproval(t *testing.T) {
	t.Parallel()
	f, payment := createAndPayMixedCommissionFixture(t)
	correctCommissionRunToZero(t, f, payment.CycleID, payment.RunID)
	if _, err := f.service.ProcessPayments(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessCorrectionPlans(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	plans := correctionPlansRead(t, f)
	if len(plans.Items) != 1 || plans.Items[0].PayoutMode != commission.PayoutMixed {
		t.Fatalf("mixed source mode not preserved in plan: %+v", plans)
	}
	enableCorrectionExecutionPolicy(t, f)
	if _, err := f.service.ProcessCorrectionExecutions(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	page := correctionExecutionsRead(t, f)
	if len(page.Items) != 1 || page.Items[0].PayoutMode != commission.PayoutMixed || page.Items[0].State != commission.CorrectionExecutionAwaitingApproval || page.Items[0].AppliedCount != "0" {
		t.Fatalf("mixed execution must remain mixed and await fresh review: %+v", page)
	}
	execution := page.Items[0]
	actor := correctionExecutionActor(f, "approve")
	if _, err := approveCorrectionExecution(t, f, execution, actor); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessCorrectionExecutions(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	if got := correctionExecutionRead(t, f, execution.ID); got.State != commission.CorrectionExecutionCompleted || got.PayoutMode != commission.PayoutMixed {
		t.Fatalf("freshly approved mixed execution=%+v", got)
	}
}

func TestCommissionCorrectionExecutionInsufficientSourcePausesAndRequiresContinue(t *testing.T) {
	t.Parallel()
	f, payment, plan := readyAutomaticCorrectionExecutionFixture(t)
	postCorrectionExecutionCommissionFreeze(t, f, 1, true)
	bucketsAfterFreeze := correctionExecBuckets(t, f)
	if _, err := f.service.ProcessCorrectionExecutions(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	executions := correctionExecutionsRead(t, f)
	if len(executions.Items) != 1 {
		t.Fatalf("execution created for held plan: %+v", executions)
	}
	execution := executions.Items[0]
	if execution.State != commission.CorrectionExecutionPaused || execution.LastErrorCode == nil || *execution.LastErrorCode != "COMMISSION_CORRECTION_AVAILABLE_INSUFFICIENT" || execution.PausedPlanTargetID == nil || !execution.CycleHoldActive {
		t.Fatalf("insufficient commission.available did not pause and latch cycle hold: %+v", execution)
	}
	assertCorrectionExecOnlyCommissionBucketDelta(t, bucketsAfterFreeze, correctionExecBuckets(t, f), f.node.MemberID, 0)
	var correctionRows int
	if err := f.betting.db.QueryRow(context.Background(), `SELECT count(*) FROM point_ledger_entries WHERE brand_id=$1 AND entry_type='commission_correction' AND reference_type='commission_correction_target'`, f.betting.brand).Scan(&correctionRows); err != nil || correctionRows != 0 {
		t.Fatalf("paused correction posted a ledger row: count=%d err=%v", correctionRows, err)
	}
	postCorrectionExecutionCommissionFreeze(t, f, 1, false)
	if n, err := f.service.ProcessCorrectionExecutions(context.Background(), 100); err != nil || n != 0 {
		t.Fatalf("restored balance must not auto-continue held execution: steps=%d err=%v", n, err)
	}
	stillPaused := correctionExecutionRead(t, f, execution.ID)
	if stillPaused.State != commission.CorrectionExecutionPaused || !stillPaused.CycleHoldActive {
		t.Fatalf("balance restoration silently resumed execution: %+v", stillPaused)
	}
	viewer := commissionPaymentActor(f)
	if _, err := continueCorrectionExecution(t, f, stillPaused, viewer); !errors.Is(err, commission.ErrDenied) {
		t.Fatalf("old payment approval permission continued held execution: %v", err)
	}
	planRetryOnly := f.actor
	for i := range planRetryOnly.Roles {
		if planRetryOnly.Roles[i].BrandID == f.betting.brand {
			planRetryOnly.Roles[i].Permissions = append(planRetryOnly.Roles[i].Permissions,
				access.Permission{Resource: "commission_correction", Action: "retry", Scope: access.ScopeBrand})
		}
	}
	if _, err := continueCorrectionExecution(t, f, stillPaused, planRetryOnly); !errors.Is(err, commission.ErrDenied) {
		t.Fatalf("correction-plan retry permission unexpectedly continued financial execution: %v", err)
	}
	continuationActor := correctionExecutionActor(f, "continue")
	continued, err := continueCorrectionExecution(t, f, stillPaused, continuationActor)
	if err != nil || continued.State != commission.CorrectionExecutionApplying || continued.PlanID != plan.ID {
		t.Fatalf("explicit current-version continue=%+v err=%v", continued, err)
	}
	if _, err = f.service.ProcessCorrectionExecutions(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	completed := correctionExecutionRead(t, f, execution.ID)
	if completed.State != commission.CorrectionExecutionCompleted || completed.CycleHoldActive || completed.AppliedDebitPoints != "1" {
		t.Fatalf("explicitly continued execution did not complete and clear hold: %+v", completed)
	}
	if got := commissionPaymentRead(t, f, payment.ID); got.State != "blocked" {
		t.Fatalf("execution changed the original blocked payment: %+v", got)
	}
}

func TestCommissionCorrectionExecutionCycleHoldSurvivesStaleRunAndBlocksNewPayment(t *testing.T) {
	t.Parallel()
	f, originalPayment, oldPlan := readyAutomaticCorrectionExecutionFixture(t)
	postCorrectionExecutionCommissionFreeze(t, f, 1, true)
	if _, err := f.service.ProcessCorrectionExecutions(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	executions := correctionExecutionsRead(t, f)
	if len(executions.Items) != 1 || executions.Items[0].State != commission.CorrectionExecutionPaused || !executions.Items[0].CycleHoldActive {
		t.Fatalf("first execution should pause and latch the cycle: %+v", executions)
	}
	paused := executions.Items[0]
	postCorrectionExecutionCommissionFreeze(t, f, 1, false)
	newCycle := applyAnotherActualCommissionCorrection(t, f, 1, 2, 2)
	if newCycle.CurrentRunID == nil || *newCycle.CurrentRunID == oldPlan.RunID {
		t.Fatalf("new actual draw did not supersede the paused plan evidence: %+v", newCycle)
	}
	if _, err := f.service.ProcessCorrectionExecutions(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	old := correctionExecutionRead(t, f, paused.ID)
	if old.State != commission.CorrectionExecutionStale || !old.CycleHoldActive {
		t.Fatalf("invalidating paused execution must retain cycle-wide hold: %+v", old)
	}
	if _, err := f.service.ProcessPayments(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	var newRunPaymentCount int
	if err := f.betting.db.QueryRow(context.Background(), `SELECT count(*) FROM commission_payments WHERE brand_id=$1 AND cycle_id=$2 AND run_id=$3`, f.betting.brand, originalPayment.CycleID, *newCycle.CurrentRunID).Scan(&newRunPaymentCount); err != nil {
		t.Fatal(err)
	}
	if newRunPaymentCount != 0 || commissionPaymentRead(t, f, originalPayment.ID).State != "blocked" {
		t.Fatalf("new evidence must not create a second payout or release the original blocked payment: new-run payments=%d old=%+v", newRunPaymentCount, commissionPaymentRead(t, f, originalPayment.ID))
	}
	if _, err := f.service.ProcessCorrectionPlans(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	plans := correctionPlansRead(t, f)
	var currentPlan *commission.CorrectionPlan
	for i := range plans.Items {
		if plans.Items[i].ID != oldPlan.ID && plans.Items[i].RunID == *newCycle.CurrentRunID && plans.Items[i].State == commission.CorrectionPlanReady {
			currentPlan = &plans.Items[i]
		}
	}
	if currentPlan == nil {
		t.Fatalf("new run did not produce a distinct current plan under held cycle: %+v", plans)
	}
	if _, err := f.service.ProcessCorrectionExecutions(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	allExecutions := correctionExecutionsRead(t, f)
	var heldExecution *commission.CorrectionExecution
	for i := range allExecutions.Items {
		if allExecutions.Items[i].PlanID == currentPlan.ID {
			heldExecution = &allExecutions.Items[i]
		}
	}
	if heldExecution == nil || heldExecution.State != commission.CorrectionExecutionPaused || heldExecution.LastErrorCode == nil || *heldExecution.LastErrorCode != "COMMISSION_CORRECTION_CYCLE_HELD" || !heldExecution.CycleHoldActive {
		t.Fatalf("new execution was automatically unheld after evidence changed: %+v", heldExecution)
	}
	continuationActor := correctionExecutionActor(f, "continue")
	continued, err := continueCorrectionExecution(t, f, *heldExecution, continuationActor)
	if err != nil || continued.State != commission.CorrectionExecutionApplying {
		t.Fatalf("explicit continuation of current held execution=%+v err=%v", continued, err)
	}
	if _, err := f.service.ProcessCorrectionExecutions(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	final := correctionExecutionRead(t, f, heldExecution.ID)
	if final.State != commission.CorrectionExecutionCompleted || final.CycleHoldActive {
		t.Fatalf("explicit current execution continuation failed to release cycle hold: %+v", final)
	}
}

func retryCorrectionExecution(t *testing.T, f commissionBatchFixture, execution commission.CorrectionExecution, actor access.Account) (commission.CorrectionExecution, error) {
	t.Helper()
	var updated commission.CorrectionExecution
	err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		var err error
		updated, err = f.service.RetryCorrectionExecutionTx(context.Background(), tx, f.betting.brand, execution.ID, actor,
			commission.RetryCycleInput{Version: execution.Version, Reason: "operator retries failed correction execution"}, commissionPaymentMeta(actor))
		return err
	})
	return updated, err
}

func TestCommissionCorrectionExecutionAuditAndLedgerFailureRollbackThenRetry(t *testing.T) {
	t.Parallel()
	f, payment, _ := readyAutomaticCorrectionExecutionFixture(t)
	bucketsBefore := correctionExecBuckets(t, f)
	postingsBefore := correctionExecutionPostingRead(t, f)
	if _, err := f.betting.db.Exec(context.Background(), `CREATE FUNCTION test_reject_commission_correction_execution_target_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action LIKE 'commission.correction_execution.%' AND (NEW.action LIKE '%target%' OR NEW.action LIKE '%apply%' OR NEW.action LIKE '%post%') THEN RAISE EXCEPTION 'test execution audit outage'; END IF; RETURN NEW; END $$; CREATE TRIGGER test_reject_commission_correction_execution_target_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION test_reject_commission_correction_execution_target_audit()`); err != nil {
		t.Fatal(err)
	}
	// The worker may persist the failure state and consume the returned error;
	// inspect the durable state below rather than asserting error propagation.
	_, _ = f.service.ProcessCorrectionExecutions(context.Background(), 100)
	_, _ = f.betting.db.Exec(context.Background(), `DROP TRIGGER IF EXISTS test_reject_commission_correction_execution_target_audit ON audit_logs; DROP FUNCTION IF EXISTS test_reject_commission_correction_execution_target_audit()`)
	executions := correctionExecutionsRead(t, f)
	if len(executions.Items) != 1 || executions.Items[0].State != commission.CorrectionExecutionFailed {
		t.Fatalf("audit failure did not leave a retryable failed execution: %+v", executions)
	}
	execution := executions.Items[0]
	assertCorrectionExecOnlyCommissionBucketDelta(t, bucketsBefore, correctionExecBuckets(t, f), f.node.MemberID, 0)
	if got := correctionExecutionPostingRead(t, f); !reflect.DeepEqual(got, postingsBefore) {
		t.Fatalf("audit failure did not roll back every ledger/head row: before=%+v after=%+v", postingsBefore, got)
	}
	targets := correctionExecutionTargetsRead(t, f, execution.ID)
	if targets.TotalCount != "0" || len(targets.Items) != 0 {
		t.Fatalf("audit failure persisted a target from the rolled-back step: %+v", targets)
	}
	var ledgerCount int
	if err := f.betting.db.QueryRow(context.Background(), `SELECT count(*) FROM point_ledger_entries WHERE brand_id=$1 AND entry_type='commission_correction'`, f.betting.brand).Scan(&ledgerCount); err != nil || ledgerCount != 0 {
		t.Fatalf("audit failure committed correction ledger rows=%d err=%v", ledgerCount, err)
	}
	noRetryRight := correctionExecutionActor(f, "approve")
	if _, err := retryCorrectionExecution(t, f, execution, noRetryRight); !errors.Is(err, commission.ErrDenied) {
		t.Fatalf("approval right without execute_retry authorized retry: %v", err)
	}
	planRetryOnly := f.actor
	for i := range planRetryOnly.Roles {
		if planRetryOnly.Roles[i].BrandID == f.betting.brand {
			planRetryOnly.Roles[i].Permissions = append(planRetryOnly.Roles[i].Permissions,
				access.Permission{Resource: "commission_correction", Action: "retry", Scope: access.ScopeBrand})
		}
	}
	if _, err := retryCorrectionExecution(t, f, execution, planRetryOnly); !errors.Is(err, commission.ErrDenied) {
		t.Fatalf("correction-plan retry right unexpectedly authorized execution retry: %v", err)
	}
	retryActor := correctionExecutionActor(f, "execute_retry")
	retrying, err := retryCorrectionExecution(t, f, execution, retryActor)
	if err != nil || retrying.State != commission.CorrectionExecutionApplying {
		t.Fatalf("explicit failed execution retry=%+v err=%v", retrying, err)
	}
	postingsBefore = correctionExecutionPostingRead(t, f)
	// A ledger outage also rolls back the entire target step, after which an
	// operator retry—not a background retry—may apply it.
	if _, err := f.betting.db.Exec(context.Background(), `CREATE FUNCTION test_reject_commission_correction_execution_ledger() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.entry_type='commission_correction' THEN RAISE EXCEPTION 'test execution ledger outage'; END IF; RETURN NEW; END $$; CREATE TRIGGER test_reject_commission_correction_execution_ledger BEFORE INSERT ON point_ledger_entries FOR EACH ROW EXECUTE FUNCTION test_reject_commission_correction_execution_ledger()`); err != nil {
		t.Fatal(err)
	}
	_, _ = f.service.ProcessCorrectionExecutions(context.Background(), 100)
	_, _ = f.betting.db.Exec(context.Background(), `DROP TRIGGER IF EXISTS test_reject_commission_correction_execution_ledger ON point_ledger_entries; DROP FUNCTION IF EXISTS test_reject_commission_correction_execution_ledger()`)
	failed := correctionExecutionRead(t, f, execution.ID)
	if failed.State != commission.CorrectionExecutionFailed || failed.Version <= retrying.Version {
		t.Fatalf("ledger failure did not produce a new failed version: %+v", failed)
	}
	assertCorrectionExecOnlyCommissionBucketDelta(t, bucketsBefore, correctionExecBuckets(t, f), f.node.MemberID, 0)
	if got := correctionExecutionPostingRead(t, f); !reflect.DeepEqual(got, postingsBefore) {
		t.Fatalf("ledger failure did not roll back every ledger/head row: before=%+v after=%+v", postingsBefore, got)
	}
	failedTargets := correctionExecutionTargetsRead(t, f, execution.ID)
	if failedTargets.TotalCount != "0" || len(failedTargets.Items) != 0 {
		t.Fatalf("ledger failure persisted a target from the rolled-back step: %+v", failedTargets)
	}
	if _, err := retryCorrectionExecution(t, f, failed, retryActor); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessCorrectionExecutions(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	completed := correctionExecutionRead(t, f, execution.ID)
	if completed.State != commission.CorrectionExecutionCompleted || completed.AppliedDebitPoints != "1" {
		t.Fatalf("operator retry after ledger recovery did not complete: %+v", completed)
	}
	if got := commissionPaymentRead(t, f, payment.ID); got.State != "blocked" {
		t.Fatalf("retry changed original payment: %+v", got)
	}
}

func applyAnotherActualCommissionCorrection(t *testing.T, f commissionBatchFixture, result ...int) commission.Cycle {
	ctx := context.Background()
	correction := createFixtureCorrection(t, f.betting, correctedDigits(result...))
	if correction.State != "reversing" {
		t.Fatalf("follow-up draw correction did not enter normal workflow: %+v", correction)
	}
	if _, err := f.betting.service.ProcessCorrections(ctx, 20); err != nil {
		t.Fatal(err)
	}
	correction, err := f.betting.service.Correction(ctx, f.betting.brand, correction.ID)
	if err != nil || correction.State != "resettling" || correction.NewJobID == nil {
		t.Fatalf("follow-up correction did not create actual settlement generation: %+v err=%v", correction, err)
	}
	if _, err := f.betting.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	advanceCommissionWorker(t, f, 50)
	var cycleID string
	if err = f.betting.db.QueryRow(ctx, `SELECT c.id::text FROM commission_cycles c JOIN bet_orders o ON o.brand_id=c.brand_id AND o.id=c.anchor_order_id WHERE c.brand_id=$1 AND o.period_id=$2 ORDER BY c.created_at DESC,c.id DESC LIMIT 1`, f.betting.brand, correction.PeriodID).Scan(&cycleID); err != nil {
		t.Fatal(err)
	}
	cycle := readCommissionCycle(t, f, cycleID)
	if cycle.State != "ready" || !cycle.EvidenceCurrent || cycle.CurrentRunID == nil {
		t.Fatalf("follow-up actual run is not current/ready: %+v", cycle)
	}
	return cycle
}

func TestCommissionCorrectionExecutionFurtherActualCorrectionUsesCumulativeBalanceHead(t *testing.T) {
	t.Parallel()
	f, payment, firstPlan, childAgentID := readyPartialCorrectionExecutionFixture(t, 10)
	walletBeforeDebit, err := (points.Store{DB: f.betting.db}).Read(context.Background(), f.betting.brand, f.node.MemberID)
	if err != nil || walletBeforeDebit.DisplayPoints < 2 {
		t.Fatalf("actual beneficiary wallet before debit=%+v err=%v", walletBeforeDebit, err)
	}
	capBelowPriorBalance := points.Amount(walletBeforeDebit.DisplayPoints - 1)
	policy, err := (points.Store{DB: f.betting.db}).ReadPolicy(context.Background(), f.betting.brand)
	if err != nil {
		t.Fatal(err)
	}
	saveCommissionPointCaps(t, f, commissionAdjustmentActor(f), policy.Version, &capBelowPriorBalance, nil)
	firstExecution := ensureCorrectionExecution(t, f)
	if firstExecution.State != commission.CorrectionExecutionCompleted || firstExecution.AppliedDebitPoints != "3" || firstExecution.AppliedCreditPoints != "0" {
		t.Fatalf("first actual debit execution=%+v", firstExecution)
	}
	firstTargets := correctionExecutionTargetsRead(t, f, firstExecution.ID)
	var firstRoot, firstChild *commission.CorrectionExecutionTarget
	for i := range firstTargets.Items {
		if firstTargets.Items[i].AgentID == f.node.ID {
			firstRoot = &firstTargets.Items[i]
		}
		if firstTargets.Items[i].AgentID == childAgentID {
			firstChild = &firstTargets.Items[i]
		}
	}
	if len(firstTargets.Items) != 2 || firstRoot == nil || firstChild == nil || firstRoot.State != commission.CorrectionExecutionTargetApplied || firstRoot.DeltaPoints != -3 || firstRoot.FinancialVersion == nil || *firstRoot.FinancialVersion != 1 || firstChild.State != commission.CorrectionExecutionTargetApplied || firstChild.DeltaPoints != 0 || firstChild.FinancialVersion == nil || *firstChild.FinancialVersion != 1 {
		t.Fatalf("first actual execution target=%+v", firstTargets.Items)
	}
	firstTargetID := firstRoot.ID
	if wallet := commissionWalletBySource(t, f, f.node.MemberID); wallet[3][0] != 0 {
		t.Fatalf("first debit did not reduce commission.available to zero: %+v", wallet)
	}
	walletAfterDebit, err := (points.Store{DB: f.betting.db}).Read(context.Background(), f.betting.brand, f.node.MemberID)
	if err != nil || walletAfterDebit.DisplayPoints > capBelowPriorBalance || walletBeforeDebit.DisplayPoints <= capBelowPriorBalance {
		t.Fatalf("negative correction must be permitted while lowering an over-cap balance: wallet=%+v cap=%d err=%v", walletAfterDebit, capBelowPriorBalance, err)
	}
	secondCycle := applyAnotherActualCommissionCorrection(t, f, 1, 2, 2)
	if secondCycle.TotalPoints != "4" || secondCycle.CurrentRunID == nil || *secondCycle.CurrentRunID == firstPlan.RunID {
		t.Fatalf("further true draw correction did not restore four actual earnings: %+v", secondCycle)
	}
	if _, err := f.service.ProcessPayments(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessCorrectionPlans(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	plans := correctionPlansRead(t, f)
	var nextPlan *commission.CorrectionPlan
	for i := range plans.Items {
		if plans.Items[i].ID != firstPlan.ID && plans.Items[i].State == commission.CorrectionPlanReady {
			nextPlan = &plans.Items[i]
		}
	}
	if nextPlan == nil || nextPlan.RunID != *secondCycle.CurrentRunID || nextPlan.BeforePoints != "0" || nextPlan.CalculatedPoints != "4" || nextPlan.CreditPoints == nil || *nextPlan.CreditPoints != "4" || nextPlan.DebitPoints == nil || *nextPlan.DebitPoints != "0" || nextPlan.NetPoints == nil || *nextPlan.NetPoints != "4" {
		t.Fatalf("second preview did not use actual cumulative balance head rather than reusing old paid base: plans=%+v", plans)
	}
	if oldPlan := correctionPlanRead(t, f, firstPlan.ID); oldPlan.State != commission.CorrectionPlanStale {
		t.Fatalf("prior preview was not marked stale after actual draw correction: %+v", oldPlan)
	}
	bucketsBeforeCapFailure := correctionExecBuckets(t, f)
	postingsBeforeCapFailure := correctionExecutionPostingRead(t, f)
	if _, err := f.service.ProcessCorrectionExecutions(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	executions := correctionExecutionsRead(t, f)
	var secondExecution *commission.CorrectionExecution
	for i := range executions.Items {
		if executions.Items[i].PlanID == nextPlan.ID {
			secondExecution = &executions.Items[i]
		}
	}
	if secondExecution == nil || secondExecution.State != commission.CorrectionExecutionFailed || secondExecution.AppliedCount != "0" {
		t.Fatalf("positive correction above max-balance cap must fail atomically: execution=%+v all=%+v", secondExecution, executions.Items)
	}
	assertCorrectionExecOnlyCommissionBucketDelta(t, bucketsBeforeCapFailure, correctionExecBuckets(t, f), f.node.MemberID, 0)
	if got := correctionExecutionPostingRead(t, f); !reflect.DeepEqual(got, postingsBeforeCapFailure) {
		t.Fatalf("technical cap failure did not roll back every ledger/head row: before=%+v after=%+v", postingsBeforeCapFailure, got)
	}
	if targets := correctionExecutionTargetsRead(t, f, secondExecution.ID); targets.TotalCount != "0" || len(targets.Items) != 0 {
		t.Fatalf("cap failure did not roll back its target: %+v", targets)
	}
	policy, err = (points.Store{DB: f.betting.db}).ReadPolicy(context.Background(), f.betting.brand)
	if err != nil {
		t.Fatal(err)
	}
	saveCommissionPointCaps(t, f, commissionAdjustmentActor(f), policy.Version, nil, nil)
	retryActor := correctionExecutionActor(f, "execute_retry")
	retrying, err := retryCorrectionExecution(t, f, *secondExecution, retryActor)
	if err != nil || retrying.State != commission.CorrectionExecutionApplying {
		t.Fatalf("operator retry after cap increase=%+v err=%v", retrying, err)
	}
	if _, err := f.service.ProcessCorrectionExecutions(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	secondCompleted := correctionExecutionRead(t, f, secondExecution.ID)
	if secondCompleted.State != commission.CorrectionExecutionCompleted || secondCompleted.AppliedCreditPoints != "4" || secondCompleted.AppliedDebitPoints != "0" {
		t.Fatalf("second cumulative execution after cap recovery=%+v", secondCompleted)
	}
	secondTargets := correctionExecutionTargetsRead(t, f, secondExecution.ID)
	var secondRoot, secondChild *commission.CorrectionExecutionTarget
	for i := range secondTargets.Items {
		if secondTargets.Items[i].AgentID == f.node.ID {
			secondRoot = &secondTargets.Items[i]
		}
		if secondTargets.Items[i].AgentID == childAgentID {
			secondChild = &secondTargets.Items[i]
		}
	}
	if len(secondTargets.Items) != 2 || secondRoot == nil || secondRoot.FinancialVersion == nil || *secondRoot.FinancialVersion != *firstRoot.FinancialVersion+1 || secondChild == nil || secondChild.FinancialVersion == nil || *secondChild.FinancialVersion != *firstChild.FinancialVersion+1 {
		t.Fatalf("financial-version head did not advance exactly across actual corrections: first=%+v second=%+v", firstTargets.Items, secondTargets.Items)
	}
	if wallet := commissionWalletBySource(t, f, f.node.MemberID); wallet[3][0] != 3 {
		t.Fatalf("further correction did not credit only the new positive difference: %+v", wallet)
	}
	if wallet := commissionWalletBySource(t, f, secondChild.MemberID); wallet[3][0] != 1 {
		t.Fatalf("further correction did not credit the actual child difference: %+v", wallet)
	}
	old := correctionExecutionRead(t, f, firstExecution.ID)
	if old.State != commission.CorrectionExecutionStale || old.AppliedDebitPoints != "3" {
		t.Fatalf("later draw correction must stale the historical execution while retaining its actual debit: %+v", old)
	}
	var oldTargetState string
	if err := f.betting.db.QueryRow(context.Background(), `SELECT state FROM commission_correction_execution_targets WHERE id=$1`, firstTargetID).Scan(&oldTargetState); err != nil || oldTargetState != "applied" {
		t.Fatalf("later evidence erased completed target state=%q err=%v", oldTargetState, err)
	}
	if got := commissionPaymentRead(t, f, payment.ID); got.State != "blocked" || got.PaidPoints != "3" {
		t.Fatalf("cumulative correction rewrote original payout: %+v", got)
	}
}

func TestCommissionCorrectionExecutionConcurrentWorkersAreIdempotentAndBounded(t *testing.T) {
	t.Parallel()
	f, payment, _ := readyAutomaticCorrectionExecutionFixture(t)
	// Do not rely on the server/operator's default transaction isolation. The
	// worker must explicitly use READ COMMITTED for its fresh post-claim query.
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
	if _, err := f.service.ProcessCorrectionExecutions(context.Background(), 0); !errors.Is(err, commission.ErrInvalid) {
		t.Fatalf("maxSteps=0 error=%v, want invalid", err)
	}
	if _, err := f.service.ProcessCorrectionExecutions(context.Background(), 101); !errors.Is(err, commission.ErrInvalid) {
		t.Fatalf("maxSteps=101 error=%v, want invalid", err)
	}
	start := make(chan struct{})
	var workers sync.WaitGroup
	const parallelWorkers = 8
	errs := make(chan error, parallelWorkers)
	for i := 0; i < parallelWorkers; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			_, err := concurrentService.ProcessCorrectionExecutions(context.Background(), 100)
			errs <- err
		}()
	}
	close(start)
	workers.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent execution worker: %v", err)
		}
	}
	executions := correctionExecutionsRead(t, f)
	if executions.TotalCount != "1" || len(executions.Items) != 1 || executions.Items[0].State != commission.CorrectionExecutionCompleted {
		t.Fatalf("concurrent workers failed to converge on one completed execution: %+v", executions)
	}
	targets := correctionExecutionTargetsRead(t, f, executions.Items[0].ID)
	if targets.TotalCount != "1" || len(targets.Items) != 1 || targets.Items[0].State != commission.CorrectionExecutionTargetApplied {
		t.Fatalf("concurrent workers duplicated/lost execution targets: %+v", targets)
	}
	var ledgerRows int
	if err := f.betting.db.QueryRow(context.Background(), `SELECT count(*) FROM point_ledger_entries WHERE brand_id=$1 AND entry_type='commission_correction' AND reference_type='commission_correction_target' AND reference_id IN(SELECT id FROM commission_correction_execution_targets WHERE execution_id=$2)`, f.betting.brand, executions.Items[0].ID).Scan(&ledgerRows); err != nil || ledgerRows != 1 {
		t.Fatalf("concurrent workers posted ledger rows=%d err=%v", ledgerRows, err)
	}
	if got := commissionPaymentRead(t, f, payment.ID); got.State != "blocked" {
		t.Fatalf("concurrent execution changed original payment: %+v", got)
	}
	beforeRepeat := correctionPlanSnapshot(t, f, payment.ID)
	if steps, err := f.service.ProcessCorrectionExecutions(context.Background(), 100); err != nil || steps != 0 {
		t.Fatalf("completed shared plan was registered or advanced again: steps=%d err=%v", steps, err)
	}
	if after := correctionPlanSnapshot(t, f, payment.ID); after != beforeRepeat {
		t.Fatal("no-op repeat rewrote completed financial evidence")
	}
}
