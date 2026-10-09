package betting

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/workbench"
	"github.com/jackc/pgx/v5"
)

func commissionWorkbenchRead(t *testing.T, f commissionBatchFixture, want map[string]string) workbench.Commissions {
	t.Helper()
	ctx := context.Background()
	before := commissionReportFingerprint(t, f)
	tx, err := f.betting.db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	out, err := (workbench.Service{}).ReadTx(ctx, tx, f.actor, f.betting.brand)
	_ = tx.Rollback(ctx)
	if err != nil || out.Commissions.Status != "ready" || out.Commissions.Data == nil {
		t.Fatal("real commission task snapshot", out.Commissions, err)
	}
	encoded, _ := json.Marshal(out.Commissions.Data)
	var fields map[string]string
	_ = json.Unmarshal(encoded, &fields)
	if len(fields) != 18 {
		t.Fatal("unexpected task summary fields", string(encoded))
	}
	for key, value := range want {
		if fields[key] != value {
			t.Fatalf("%s=%s want%s, summary=%s", key, fields[key], value, encoded)
		}
	}
	if after := commissionReportFingerprint(t, f); after != before {
		t.Fatal("readonly workbench changed financial, audit or event facts")
	}
	return *out.Commissions.Data
}

func TestCommissionWorkbenchRealManualCycleAndMultipleTargetsAreCountedOnce(t *testing.T) {
	f := newCommissionBatchFixture(t)
	f, _ = addActualChildAgentBeneficiary(t, f)
	ctx := context.Background()
	commissionWorkbenchRead(t, f, map[string]string{"discovery_pending_count": "4", "cycle_processing_count": "0", "payment_processing_count": "0"})
	waitCommissionBoundary(t, f.boundary)
	cycle := createCommissionCycle(t, f)
	commissionWorkbenchRead(t, f, map[string]string{"cycle_processing_count": "1", "cycle_waiting_count": "0"})
	if _, err := f.service.ProcessCycles(ctx, 1); err != nil {
		t.Fatal(err)
	}
	commissionWorkbenchRead(t, f, map[string]string{"cycle_processing_count": "0", "cycle_waiting_count": "1", "cycle_ready_count": "0"})
	eligibilitySettle(t, f.betting)
	advanceCommissionWorker(t, f, 30)
	cycle = readCommissionCycle(t, f, cycle.ID)
	if cycle.State != "ready" || cycle.EarningCount != "2" {
		t.Fatal("real multibeneficiary cycle not ready", cycle)
	}
	commissionWorkbenchRead(t, f, map[string]string{"cycle_ready_count": "1", "cycle_stale_count": "0", "payment_awaiting_approval_count": "0"})
	policy := commissionPaymentPolicy(t, f)
	actor := commissionPaymentActor(f)
	if err := updateCommissionPaymentPolicy(t, f, actor, true, policy.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessPayments(ctx, 100); err != nil {
		t.Fatal(err)
	}
	payments := commissionPayments(t, f)
	if len(payments.Items) != 1 || payments.Items[0].TargetCount != "2" {
		t.Fatal("actual targets not created", payments)
	}
	payment := payments.Items[0]
	commissionWorkbenchRead(t, f, map[string]string{"payment_awaiting_approval_count": "1", "payment_processing_count": "0"})
	if err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, e := f.service.ApprovePaymentTx(ctx, tx, f.betting.brand, payment.ID, actor, commission.RetryCycleInput{Version: payment.Version, Reason: "approve real workbench fixture targets"}, commissionPaymentMeta(actor))
		return e
	}); err != nil {
		t.Fatal(err)
	}
	commissionWorkbenchRead(t, f, map[string]string{"payment_awaiting_approval_count": "0", "payment_processing_count": "1"})
	if _, err := f.service.ProcessPayments(ctx, 100); err != nil {
		t.Fatal(err)
	}
	commissionWorkbenchRead(t, f, map[string]string{"payment_processing_count": "0", "payment_blocked_count": "0", "payment_failed_count": "0"})
	// A real new draw changes the epoch before the old ready cycle is replaced.
	createFixtureCorrection(t, f.betting, correctedDigits(0, 0, 0))
	commissionWorkbenchRead(t, f, map[string]string{"cycle_ready_count": "0", "cycle_stale_count": "1"})
}

func TestCommissionWorkbenchCorrectionPauseFailureAndHistoricalBlockedState(t *testing.T) {
	f, payment, plan := readyAutomaticCorrectionPlanFixture(t)
	ctx := context.Background()
	commissionWorkbenchRead(t, f, map[string]string{"cycle_ready_count": "1", "payment_blocked_count": "1", "plan_ready_count": "1", "execution_processing_count": "0"})
	enableCorrectionExecutionPolicy(t, f)
	if _, err := f.service.ProcessCorrectionExecutions(ctx, 1); err != nil {
		t.Fatal(err)
	}
	execution := correctionExecutionsRead(t, f).Items[0]
	if execution.State != commission.CorrectionExecutionApplying || execution.PlanID != plan.ID {
		t.Fatal(execution)
	}
	commissionWorkbenchRead(t, f, map[string]string{"execution_processing_count": "1", "execution_paused_count": "0"})
	postCorrectionExecutionCommissionFreeze(t, f, 1, true)
	if _, err := f.service.ProcessCorrectionExecutions(ctx, 100); err != nil {
		t.Fatal(err)
	}
	execution = correctionExecutionRead(t, f, execution.ID)
	if execution.State != commission.CorrectionExecutionPaused {
		t.Fatal(execution)
	}
	commissionWorkbenchRead(t, f, map[string]string{"execution_processing_count": "0", "execution_paused_count": "1", "execution_failed_count": "0"})
	postCorrectionExecutionCommissionFreeze(t, f, 1, false)
	if _, err := continueCorrectionExecution(t, f, execution, correctionExecutionActor(f, "continue")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.betting.db.Exec(ctx, `CREATE FUNCTION fail_workbench_correction_ledger() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.entry_type='commission_correction' THEN RAISE EXCEPTION 'owned ledger outage'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_workbench_correction_ledger BEFORE INSERT ON point_ledger_entries FOR EACH ROW EXECUTE FUNCTION fail_workbench_correction_ledger()`); err != nil {
		t.Fatal(err)
	}
	_, _ = f.service.ProcessCorrectionExecutions(ctx, 100)
	if _, err := f.betting.db.Exec(ctx, `DROP TRIGGER fail_workbench_correction_ledger ON point_ledger_entries; DROP FUNCTION fail_workbench_correction_ledger()`); err != nil {
		t.Fatal(err)
	}
	execution = correctionExecutionRead(t, f, execution.ID)
	if execution.State != commission.CorrectionExecutionFailed {
		t.Fatal("real technical failure not recorded", execution)
	}
	commissionWorkbenchRead(t, f, map[string]string{"execution_processing_count": "0", "execution_paused_count": "0", "execution_failed_count": "1"})
	if _, err := retryCorrectionExecution(t, f, execution, correctionExecutionActor(f, "execute_retry")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessCorrectionExecutions(ctx, 100); err != nil {
		t.Fatal(err)
	}
	if current := correctionExecutionRead(t, f, execution.ID); current.State != commission.CorrectionExecutionCompleted {
		t.Fatal(current)
	}
	if original := commissionPaymentRead(t, f, payment.ID); original.State != "blocked" {
		t.Fatal("original historical block unexpectedly cleared", original)
	}
	commissionWorkbenchRead(t, f, map[string]string{"payment_blocked_count": "1", "execution_processing_count": "0", "execution_failed_count": "0", "execution_paused_count": "0"})
}
