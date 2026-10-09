package betting

import (
	"context"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
)

func TestCommissionManualAdjustmentCorrectionPostsConfirmedActualDifference(t *testing.T) {
	t.Parallel()
	f := newCommissionBatchFixtureWithWindow(t, 30*time.Second, 32*time.Second)
	ctx := context.Background()
	fundBettingWallet(t, f.betting, 40)
	// Together with the two original 000 bets, 33 losing points at 0.3
	// calculate 9.9, rounded to 10. Correcting the result to 000 leaves only
	// the 26-point 222 bet losing: 7.8, rounded to 8.
	for _, bet := range []struct {
		digit      int
		multiplier points.Amount
		key        string
	}{
		{0, 5, "commission-manual-policy-000-five"},
		{2, 26, "commission-manual-policy-222-twenty-six"},
	} {
		in := f.betting.input
		in.Selection = rules.Selection{Digits: [][]int{{bet.digit}, {bet.digit}, {bet.digit}}}
		in.Multiplier = bet.multiplier
		order, err := placeBettingOrder(t, f.betting, in, bet.key)
		if err != nil {
			t.Fatalf("place exact manual-policy bet with multiplier %d under existing caps: %v", bet.multiplier, err)
		}
		if !order.PlacedAt.Before(f.boundary) {
			t.Fatalf("exact manual-policy bet missed boundary: order=%+v boundary=%s", order, f.boundary)
		}
		f.orders = append(f.orders, order)
	}
	waitCommissionBoundary(t, f.boundary)
	cycle := createCommissionCycle(t, f)
	eligibilitySettle(t, f.betting)
	advanceCommissionWorker(t, f, 40)
	cycle = readCommissionCycle(t, f, cycle.ID)
	if cycle.State != "ready" || !cycle.EvidenceCurrent || cycle.TotalPoints != "10" || cycle.EarningCount != "1" || cycle.CurrentRunID == nil {
		t.Fatalf("actual 33-point loss did not calculate a current ten-point run: %+v", cycle)
	}
	actor := commissionPaymentActor(f)
	paymentPolicy := commissionPaymentPolicy(t, f)
	if paymentPolicy.Enabled {
		t.Fatalf("fresh payment gate unexpectedly enabled: %+v", paymentPolicy)
	}
	if n, err := f.service.ProcessPayments(ctx, 20); err != nil || n != 0 {
		t.Fatalf("closed payment gate processed=%d err=%v", n, err)
	}
	if page := commissionPayments(t, f); len(page.Items) != 0 {
		t.Fatalf("closed payment gate registered payments: %+v", page)
	}
	if err := updateCommissionPaymentPolicy(t, f, actor, true, paymentPolicy.Version); err != nil {
		t.Fatal(err)
	}
	if n, err := f.service.ProcessPayments(ctx, 20); err != nil || n == 0 {
		t.Fatalf("register exact manual payout: steps=%d err=%v", n, err)
	}
	payments := commissionPayments(t, f)
	if len(payments.Items) != 1 || payments.Items[0].State != "awaiting_approval" || payments.Items[0].PayoutMode != commission.PayoutManual || payments.Items[0].RunID != *cycle.CurrentRunID || payments.Items[0].TotalPoints != "10" || payments.Items[0].PaidPoints != "0" {
		t.Fatalf("actual ten-point manual payout must await approval: %+v", payments.Items)
	}
	payment := payManualCommissionForAdjustment(t, f, payments.Items[0])
	paidTargets := commissionTargets(t, f, payment.ID)
	if payment.PaidPoints != "10" || payment.PaidCount != "1" || len(paidTargets.Items) != 1 || paidTargets.Items[0].State != "paid" || paidTargets.Items[0].OriginalPoints != 10 || paidTargets.Items[0].AdjustmentVersion == nil {
		t.Fatalf("expected one genuine ten-point manual payout: payment=%+v targets=%+v", payment, paidTargets.Items)
	}
	target := paidTargets.Items[0]
	if wallet := commissionWalletBySource(t, f, target.MemberID); wallet[3][0] != 10 {
		t.Fatalf("actual original payout did not credit ten commission points: %+v", wallet)
	}

	adjustment, err := adjustCommission(t, f, f.betting.brand, target, commissionAdjustmentActor(f), *target.AdjustmentVersion, 12)
	if err != nil || adjustment.PointsBefore != 10 || adjustment.PointsAfter != 12 || adjustment.DeltaPoints != 2 || adjustment.LedgerEntryID == "" || adjustment.AuditLogID == "" {
		t.Fatalf("genuine 10-to-12 manual adjustment=%+v err=%v", adjustment, err)
	}
	if wallet := commissionWalletBySource(t, f, target.MemberID); wallet[3][0] != 12 {
		t.Fatalf("actual manual adjustment did not bring commission available to twelve: %+v", wallet)
	}
	adjustmentEvidence := commissionManualPolicyAdjustmentEvidence(t, f, adjustment)

	correction := createFixtureCorrection(t, f.betting, correctedDigits(0, 0, 0))
	if correction.State != "reversing" {
		t.Fatalf("real result correction started in state %q", correction.State)
	}
	if _, err := f.betting.service.ProcessCorrections(ctx, 20); err != nil {
		t.Fatal(err)
	}
	correction, err = f.betting.service.Correction(ctx, f.betting.brand, correction.ID)
	if err != nil || correction.State != "resettling" || correction.NewJobID == nil {
		t.Fatalf("correction did not create a real replacement settlement: %+v err=%v", correction, err)
	}
	if _, err := f.betting.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	advanceCommissionWorker(t, f, 40)
	current := readCommissionCycle(t, f, payment.CycleID)
	if current.State != "ready" || !current.EvidenceCurrent || current.CurrentRunID == nil || *current.CurrentRunID == payment.RunID || current.TotalPoints != "8" || current.EarningCount != "1" {
		t.Fatalf("actual 000 result correction did not create a current eight-point run: %+v", current)
	}
	if _, err := f.service.ProcessPayments(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if got := commissionPaymentRead(t, f, payment.ID); got.State != "blocked" || got.PaidPoints != "10" || got.PaidCount != "1" {
		t.Fatalf("correction must preserve the original actual payment as blocked: %+v", got)
	}
	if _, err := f.service.ProcessCorrectionPlans(ctx, 100); err != nil {
		t.Fatal(err)
	}
	plans := correctionPlansRead(t, f)
	if len(plans.Items) != 1 {
		t.Fatalf("expected one plan for the corrected actual payment: %+v", plans.Items)
	}
	plan := plans.Items[0]
	if plan.State != commission.CorrectionPlanReady || plan.LastErrorCode != nil || plan.PayoutMode != commission.PayoutManual || plan.RunID != *current.CurrentRunID || plan.BeforePoints != "12" || plan.CalculatedPoints != "8" || plan.CreditPoints == nil || *plan.CreditPoints != "0" || plan.DebitPoints == nil || *plan.DebitPoints != "4" || plan.NetPoints == nil || *plan.NetPoints != "-4" {
		t.Fatalf("actual 10-to-12-to-8 difference must be a ready manual-mode four-point debit: %+v", plan)
	}
	plannedTargets := correctionPlanTargetsRead(t, f, plan.ID)
	if plannedTargets.TotalCount != "1" || len(plannedTargets.Items) != 1 || plannedTargets.Items[0].OriginalTargetID == nil || *plannedTargets.Items[0].OriginalTargetID != target.ID || plannedTargets.Items[0].PointsBefore != 12 || plannedTargets.Items[0].PointsAfter != 8 || plannedTargets.Items[0].DeltaPoints != -4 {
		t.Fatalf("plan lost the adjusted paid-target basis or exact difference: %+v", plannedTargets)
	}

	// The independent execution gate remains closed until explicitly enabled.
	policy := correctionExecutionPolicyRead(t, f)
	if policy.Enabled {
		t.Fatalf("fresh correction-execution gate unexpectedly enabled: %+v", policy)
	}
	if n, err := f.service.ProcessCorrectionExecutions(ctx, 100); err != nil || n != 0 {
		t.Fatalf("default-off correction execution processed=%d err=%v", n, err)
	}
	if page := correctionExecutionsRead(t, f); page.TotalCount != "0" || len(page.Items) != 0 {
		t.Fatalf("closed execution gate created financial work: %+v", page)
	}
	enableCorrectionExecutionPolicy(t, f)
	if _, err := f.service.ProcessCorrectionExecutions(ctx, 100); err != nil {
		t.Fatal(err)
	}
	executions := correctionExecutionsRead(t, f)
	if len(executions.Items) != 1 || executions.Items[0].State != commission.CorrectionExecutionAwaitingApproval || executions.Items[0].PayoutMode != commission.PayoutManual || executions.Items[0].DebitPoints != "4" {
		t.Fatalf("manual-mode correction must wait for fresh approval with the exact debit: %+v", executions.Items)
	}
	if wallet := commissionWalletBySource(t, f, target.MemberID); wallet[3][0] != 12 {
		t.Fatalf("unapproved correction changed the actual twelve-point manual amount: %+v", wallet)
	}
	execution := executions.Items[0]
	approved, err := approveCorrectionExecution(t, f, execution, correctionExecutionActor(f, "approve"))
	if err != nil || approved.State != commission.CorrectionExecutionApplying {
		t.Fatalf("explicit correction approval=%+v err=%v", approved, err)
	}
	walletBeforeExecution := commissionWalletBySource(t, f, target.MemberID)
	if _, err := f.service.ProcessCorrectionExecutions(ctx, 100); err != nil {
		t.Fatal(err)
	}
	completed := correctionExecutionRead(t, f, execution.ID)
	if completed.State != commission.CorrectionExecutionCompleted || completed.DebitPoints != "4" || completed.AppliedDebitPoints != "4" || completed.AppliedCreditPoints != "0" || completed.AppliedCount != "1" {
		t.Fatalf("approved actual correction did not complete the exact four-point debit: %+v", completed)
	}
	actualTargets := correctionExecutionTargetsRead(t, f, execution.ID)
	if actualTargets.TotalCount != "1" || len(actualTargets.Items) != 1 || actualTargets.Items[0].State != commission.CorrectionExecutionTargetApplied || actualTargets.Items[0].PointsBefore != 12 || actualTargets.Items[0].PointsAfter != 8 || actualTargets.Items[0].DeltaPoints != -4 || actualTargets.Items[0].LedgerEntryID == nil {
		t.Fatalf("actual correction target is missing the -4 ledger application: %+v", actualTargets.Items)
	}
	var entryType, referenceType, referenceID, operationKey, ledgerDelta string
	if err := f.betting.db.QueryRow(ctx, `SELECT entry_type,reference_type,reference_id::text,operation_key,delta_snapshot->'commission'->>'available' FROM point_ledger_entries WHERE id=$1`, *actualTargets.Items[0].LedgerEntryID).Scan(&entryType, &referenceType, &referenceID, &operationKey, &ledgerDelta); err != nil {
		t.Fatal(err)
	}
	if entryType != "commission_correction" || referenceType != "commission_correction_target" || referenceID != actualTargets.Items[0].ID || operationKey != "commission-correction:"+actualTargets.Items[0].ID || ledgerDelta != "-4" {
		t.Fatalf("actual manual-source correction ledger witness type=%s ref=%s/%s op=%s delta=%s", entryType, referenceType, referenceID, operationKey, ledgerDelta)
	}
	walletBeforeExecution[3][0] = 8
	if wallet := commissionWalletBySource(t, f, target.MemberID); wallet != walletBeforeExecution {
		t.Fatalf("confirmed actual correction did not leave exactly eight commission points: %+v", wallet)
	}

	historyTx, err := f.betting.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	history, historyErr := f.service.AdjustmentsTx(ctx, historyTx, f.betting.brand, target.ID, 100, 0)
	_ = historyTx.Rollback(ctx)
	if historyErr != nil || history.TotalCount != "1" || len(history.Items) != 1 || history.Items[0].ID != adjustment.ID || history.Items[0].Version != 2 || history.Items[0].PointsBefore != 10 || history.Items[0].PointsAfter != 12 || history.Items[0].DeltaPoints != 2 || history.Items[0].LedgerEntryID != adjustment.LedgerEntryID || history.Items[0].AuditLogID != adjustment.AuditLogID {
		t.Fatalf("actual correction must retain the original manual adjustment audit/history: %+v err=%v", history, historyErr)
	}
	finalPayment := commissionPaymentRead(t, f, payment.ID)
	if finalPayment.State != "blocked" || finalPayment.PaidPoints != "10" || finalPayment.PaidCount != "1" {
		t.Fatalf("actual correction rewrote the immutable original payment: %+v", finalPayment)
	}
	if after := commissionManualPolicyAdjustmentEvidence(t, f, adjustment); after != adjustmentEvidence {
		t.Fatal("result correction or compensation rewrote the original adjustment, its +2 ledger entry, or its audit")
	}
	// A further generation must start from actual 8, not manual head 12 or
	// original 10, and must not revive the old manual +2 offset.
	nextDraw := createFixtureCorrection(t, f.betting, correctedDigits(1, 2, 2))
	if _, err = f.betting.service.ProcessCorrections(ctx, 20); err != nil {
		t.Fatal(err)
	}
	nextDraw, err = f.betting.service.Correction(ctx, f.betting.brand, nextDraw.ID)
	if err != nil || nextDraw.NewJobID == nil {
		t.Fatalf("next genuine settlement: %+v %v", nextDraw, err)
	}
	if _, err = f.betting.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	advanceCommissionWorker(t, f, 40)
	nextCycle := readCommissionCycle(t, f, payment.CycleID)
	if nextCycle.State != "ready" || nextCycle.TotalPoints != "10" || nextCycle.CurrentRunID == nil || *nextCycle.CurrentRunID == plan.RunID {
		t.Fatalf("next genuine calculation: %+v", nextCycle)
	}
	if _, err = f.service.ProcessPayments(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.ProcessCorrectionPlans(ctx, 20); err != nil {
		t.Fatal(err)
	}
	var nextPlan commission.CorrectionPlan
	for _, p := range correctionPlansRead(t, f).Items {
		if p.RunID == *nextCycle.CurrentRunID {
			nextPlan = p
		}
	}
	if nextPlan.State != "ready" || nextPlan.BeforePoints != "8" || nextPlan.CalculatedPoints != "10" || nextPlan.NetPoints == nil || *nextPlan.NetPoints != "2" {
		t.Fatalf("next generation lost actual net basis: %+v", nextPlan)
	}
	if _, err = f.service.ProcessCorrectionExecutions(ctx, 20); err != nil {
		t.Fatal(err)
	}
	var nextExecution commission.CorrectionExecution
	for _, x := range correctionExecutionsRead(t, f).Items {
		if x.PlanID == nextPlan.ID {
			nextExecution = x
		}
	}
	if nextExecution.State != commission.CorrectionExecutionAwaitingApproval {
		t.Fatalf("next generation reused old approval: %+v", nextExecution)
	}
	if _, err = approveCorrectionExecution(t, f, nextExecution, correctionExecutionActor(f, "approve")); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.ProcessCorrectionExecutions(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if got := correctionExecutionRead(t, f, nextExecution.ID); got.State != commission.CorrectionExecutionCompleted || got.AppliedCreditPoints != "2" || got.AppliedDebitPoints != "0" {
		t.Fatalf("next generation did not credit only actual difference: %+v", got)
	}
	walletBeforeExecution[3][0] = 10
	if got := commissionWalletBySource(t, f, target.MemberID); got != walletBeforeExecution {
		t.Fatalf("next generation changed another source or preserved manual offset: %+v", got)
	}
	if after := commissionManualPolicyAdjustmentEvidence(t, f, adjustment); after != adjustmentEvidence {
		t.Fatal("further generation rewrote manual history")
	}
}

func commissionManualPolicyAdjustmentEvidence(t *testing.T, f commissionBatchFixture, adjustment commission.Adjustment) string {
	t.Helper()
	var snapshot string
	err := f.betting.db.QueryRow(context.Background(), `SELECT jsonb_build_object('adjustment',to_jsonb(a),'ledger',to_jsonb(l),'audit',to_jsonb(log))::text
 FROM commission_adjustments a JOIN point_ledger_entries l ON l.id=a.ledger_entry_id JOIN audit_logs log ON log.id=a.audit_log_id
 WHERE a.brand_id=$1 AND a.id=$2`, f.betting.brand, adjustment.ID).Scan(&snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
