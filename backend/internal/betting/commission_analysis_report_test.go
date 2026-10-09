package betting

import (
	"context"
	"encoding/csv"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/reporting"
	"github.com/jackc/pgx/v5"
)

type commissionAnalysisExpectedTotals struct {
	observed, calculated                               string
	calculationComplete                                bool
	paidCount, paid                                    string
	adjustmentCount, adjustmentCredit, adjustmentDebit string
	correctionCount, correctionCredit, correctionDebit string
	postingCount, actual, manualNet                    string
	effective, calculationGap, effectiveGap            string
	effectiveComplete                                  bool
}

func commissionAnalysisString(value string) *string { return &value }

func assertCommissionAnalysisTotals(t *testing.T, got reporting.CommissionAnalysisTotals, want commissionAnalysisExpectedTotals) {
	t.Helper()
	var calculated, effective, calculationGap, effectiveGap *string
	if want.calculationComplete {
		calculated = commissionAnalysisString(want.calculated)
		calculationGap = commissionAnalysisString(want.calculationGap)
	}
	if want.effectiveComplete {
		effective = commissionAnalysisString(want.effective)
		effectiveGap = commissionAnalysisString(want.effectiveGap)
	}
	expected := reporting.CommissionAnalysisTotals{
		ObservedCalculatedPoints: want.observed, CalculatedPoints: calculated,
		PaidEntryCount: want.paidCount, PaidPoints: want.paid,
		AdjustmentEntryCount: want.adjustmentCount, AdjustmentCreditPoints: want.adjustmentCredit,
		AdjustmentDebitPoints: want.adjustmentDebit,
		CorrectionEntryCount:  want.correctionCount, CorrectionCreditPoints: want.correctionCredit,
		CorrectionDebitPoints: want.correctionDebit, PostingEntryCount: want.postingCount,
		ActualNetPoints: want.actual, ManualAdjustmentNetPoints: want.manualNet,
		EffectiveTargetPoints: effective, CalculationMinusActualPoints: calculationGap,
		EffectiveMinusActualPoints: effectiveGap,
		CalculationComplete:        want.calculationComplete, EffectiveTargetComplete: want.effectiveComplete,
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("analysis totals = %+v, want %+v", got, expected)
	}
}

// Every query calls the read service inside a read-only repeatable-read
// transaction. This verifies the service's behavior at that isolation level;
// it does not assert the HTTP handler's transaction isolation. Fingerprinting
// also guards against accidental financial writes hidden behind a report read.
func readCommissionAnalysisSnapshot(t *testing.T, f commissionBatchFixture, brand string, q reporting.CommissionAnalysisQuery, export bool) (reporting.CommissionAnalysisReport, []byte, error) {
	t.Helper()
	ctx := context.Background()
	before := commissionReportFinancialFingerprint(t, f)
	tx, err := f.betting.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	service := reporting.Service{DB: f.betting.db}
	report, err := service.CommissionAnalysisRead(ctx, tx, brand, q)
	var body []byte
	if err == nil && export {
		report, err = service.CommissionAnalysisExport(ctx, tx, brand, q)
		if err == nil {
			body, err = reporting.CommissionAnalysisCSV(report)
		}
	}
	if after := commissionReportFinancialFingerprint(t, f); after != before {
		t.Fatal("commission analysis read changed financial accounts, buckets, or ledger rows")
	}
	return report, body, err
}

func commissionAnalysisCycleQuery(cycle commission.Cycle, group string) reporting.CommissionAnalysisQuery {
	return reporting.CommissionAnalysisQuery{
		From: cycle.WindowTo.UTC(), To: cycle.WindowTo.UTC().Add(time.Nanosecond),
		GroupBy: group, Limit: 20,
	}
}

func assertCommissionAnalysisCycleReport(t *testing.T, report reporting.CommissionAnalysisReport, f commissionBatchFixture, cycle commission.Cycle, q reporting.CommissionAnalysisQuery, want commissionAnalysisExpectedTotals) {
	t.Helper()
	if report.BrandID != f.betting.brand || report.Query.From != q.From.UTC() || report.Query.To != q.To.UTC() ||
		report.Query.GroupBy != q.GroupBy || report.Timezone == "" || report.SnapshotAt.IsZero() {
		t.Fatalf("analysis report metadata drifted: %+v", report)
	}
	if report.Coverage != (reporting.CommissionAnalysisCoverage{
		SelectedCycleCount: "1", ReadyCycleCount: "1", UnreadyCycleCount: "0", LegacyPolicyBlockedCycleCount: "0",
	}) {
		t.Fatalf("analysis coverage = %+v, want one ready saved cycle", report.Coverage)
	}
	assertCommissionAnalysisTotals(t, report.Summary, want)
	if report.TotalGroups != "1" || len(report.Items) != 1 {
		t.Fatalf("analysis groups = %s/%d, want one: %+v", report.TotalGroups, len(report.Items), report.Items)
	}
	wantKey := cycle.ID
	if q.GroupBy == "agent" {
		wantKey = f.node.ID
	}
	if report.Items[0].Key != wantKey || report.Items[0].Label != wantKey {
		t.Fatalf("analysis group = %+v, want saved key %s", report.Items[0], wantKey)
	}
	assertCommissionAnalysisTotals(t, report.Items[0].Totals, want)
}

func TestCommissionAnalysisGenuinePaymentAdjustmentCorrectionReadAndExport(t *testing.T) {
	f := newCommissionBatchFixture(t)
	ctx := context.Background()

	// This helper creates the saved cycle through real bets, settlement, and
	// commission calculation, then registers—but does not pay—the manual payout.
	payment := prepareManualCommissionPayment(t, f)
	cycle := readCommissionCycle(t, f, payment.CycleID)
	if cycle.State != "ready" || cycle.TotalPoints != "1" || payment.PaidPoints != "0" || payment.PaidCount != "0" {
		t.Fatalf("expected one calculated point and an unpaid manual payment: cycle=%+v payment=%+v", cycle, payment)
	}
	query := commissionAnalysisCycleQuery(cycle, "cycle")
	readyUnpaid := commissionAnalysisExpectedTotals{
		observed: "1", calculated: "1", calculationComplete: true,
		paidCount: "0", paid: "0", adjustmentCount: "0", adjustmentCredit: "0", adjustmentDebit: "0",
		correctionCount: "0", correctionCredit: "0", correctionDebit: "0", postingCount: "0", actual: "0", manualNet: "0",
		effective: "1", calculationGap: "1", effectiveGap: "1", effectiveComplete: true,
	}
	initial, _, err := readCommissionAnalysisSnapshot(t, f, f.betting.brand, query, false)
	if err != nil {
		t.Fatalf("read original ready but unpaid cycle: %v", err)
	}
	assertCommissionAnalysisCycleReport(t, initial, f, cycle, query, readyUnpaid)
	// Change today's mutable ratio after the saved run. Analysis must continue
	// to use the calculation snapshot rather than recompute historical money.
	agents := agency.Service{DB: f.betting.db}
	agentPolicy, err := agents.Policy(ctx, f.betting.brand)
	if err != nil {
		t.Fatal(err)
	}
	if err = commissionBatchCallTx(t, f, func(tx pgx.Tx) error {
		_, updateErr := agents.Update(ctx, tx, f.betting.brand, f.actor, f.node.ID, agency.UpdateInput{
			Version: f.node.Version, PolicyVersion: agentPolicy.Version,
			Config: agency.NodeConfig{Ratio: "0.2", Status: "active", CanCreateChildren: true},
			Reason: "analysis must preserve the saved run when today's ratio changes",
		}, points.Metadata{RequestID: ids.New()})
		return updateErr
	}); err != nil {
		t.Fatal(err)
	}

	// The exact saved end is inclusive after the database's ceil-to-microsecond
	// handling. Original payment and later ledger rows are deliberately posted
	// after this cycle-window boundary but still belong to this saved cycle.
	paid := payManualCommissionForAdjustment(t, f, payment)
	if paid.State != "paid" || paid.PaidPoints != "1" || paid.PaidCount != "1" {
		t.Fatalf("manual payout did not post one real point: %+v", paid)
	}
	var paidAt time.Time
	if err := f.betting.db.QueryRow(ctx, `SELECT created_at FROM point_ledger_entries WHERE id=(
	 SELECT ledger_entry_id FROM commission_payment_targets WHERE brand_id=$1 AND payment_id=$2 AND state='paid')`,
		f.betting.brand, paid.ID).Scan(&paidAt); err != nil {
		t.Fatal(err)
	}
	if !paidAt.After(cycle.WindowTo) {
		t.Fatalf("fixture did not post the original payment after the selected cycle window: posted=%s window_to=%s", paidAt, cycle.WindowTo)
	}
	targets := commissionTargets(t, f, paid.ID)
	if len(targets.Items) != 1 || targets.Items[0].LedgerEntryID == nil || targets.Items[0].AdjustmentVersion == nil {
		t.Fatalf("real paid target is missing its ledger/version binding: %+v", targets)
	}
	target := targets.Items[0]
	adjustment, err := adjustCommission(t, f, f.betting.brand, target, commissionAdjustmentActor(f), *target.AdjustmentVersion, points.Amount(2))
	if err != nil || adjustment.DeltaPoints != 1 || adjustment.PointsAfter != 2 {
		t.Fatalf("real manual adjustment from one to two points: %+v err=%v", adjustment, err)
	}
	query.AgentID = &target.AgentID
	query.MemberID = &target.MemberID
	query.CycleID = &cycle.ID
	paidAdjusted := commissionAnalysisExpectedTotals{
		observed: "1", calculated: "1", calculationComplete: true,
		paidCount: "1", paid: "1", adjustmentCount: "1", adjustmentCredit: "1", adjustmentDebit: "0",
		correctionCount: "0", correctionCredit: "0", correctionDebit: "0", postingCount: "2", actual: "2", manualNet: "1",
		effective: "2", calculationGap: "-1", effectiveGap: "0", effectiveComplete: true,
	}
	paidReport, _, err := readCommissionAnalysisSnapshot(t, f, f.betting.brand, query, false)
	if err != nil {
		t.Fatalf("read paid target with manual adjustment: %v", err)
	}
	assertCommissionAnalysisCycleReport(t, paidReport, f, cycle, query, paidAdjusted)

	// A real corrected draw creates a new zero-point epoch. The original
	// payment remains historical actual money while its correction awaits the
	// separate execution gate and approval.
	correctCommissionRunToZero(t, f, paid.CycleID, paid.RunID)
	if _, err = f.service.ProcessPayments(ctx, 20); err != nil {
		t.Fatal(err)
	}
	blocked := commissionPaymentRead(t, f, paid.ID)
	if blocked.State != "blocked" || blocked.PaidCount != "1" || blocked.PaidPoints != "1" {
		t.Fatalf("corrected original payment must remain blocked with its paid history: %+v", blocked)
	}
	if _, err = f.service.ProcessCorrectionPlans(ctx, 100); err != nil {
		t.Fatal(err)
	}
	plans := correctionPlansRead(t, f)
	if len(plans.Items) != 1 || plans.Items[0].State != commission.CorrectionPlanReady || plans.Items[0].DebitPoints == nil || *plans.Items[0].DebitPoints != "2" {
		t.Fatalf("correction preview must use the real two-point effective target: %+v", plans.Items)
	}
	newEpoch := commissionAnalysisExpectedTotals{
		observed: "0", calculated: "0", calculationComplete: true,
		paidCount: "1", paid: "1", adjustmentCount: "1", adjustmentCredit: "1", adjustmentDebit: "0",
		correctionCount: "0", correctionCredit: "0", correctionDebit: "0", postingCount: "2", actual: "2", manualNet: "1",
		effective: "0", calculationGap: "-2", effectiveGap: "-2", effectiveComplete: true,
	}
	beforeExecution, _, err := readCommissionAnalysisSnapshot(t, f, f.betting.brand, query, false)
	if err != nil {
		t.Fatalf("read corrected epoch before actual execution: %v", err)
	}
	assertCommissionAnalysisCycleReport(t, beforeExecution, f, cycle, query, newEpoch)

	// Compensation is independently gated and manually approved with its own
	// permission; the old payment approval is not reused.
	enableCorrectionExecutionPolicy(t, f)
	if _, err = f.service.ProcessCorrectionExecutions(ctx, 100); err != nil {
		t.Fatal(err)
	}
	executions := correctionExecutionsRead(t, f)
	if len(executions.Items) != 1 || executions.Items[0].State != commission.CorrectionExecutionAwaitingApproval || executions.Items[0].DebitPoints != "2" {
		t.Fatalf("two-point correction must await independent manual approval: %+v", executions.Items)
	}
	execution := executions.Items[0]
	approved, err := approveCorrectionExecution(t, f, execution, correctionExecutionActor(f, "approve"))
	if err != nil || approved.State != commission.CorrectionExecutionApplying {
		t.Fatalf("freshly approve actual correction execution: %+v err=%v", approved, err)
	}
	bucketsBeforeExecution := correctionExecBuckets(t, f)
	if _, err = f.service.ProcessCorrectionExecutions(ctx, 100); err != nil {
		t.Fatal(err)
	}
	completed := correctionExecutionRead(t, f, execution.ID)
	if completed.State != commission.CorrectionExecutionCompleted || completed.DebitPoints != "2" || completed.AppliedDebitPoints != "2" {
		t.Fatalf("approved correction did not actually debit two points: %+v", completed)
	}
	assertCorrectionExecOnlyCommissionBucketDelta(t, bucketsBeforeExecution, correctionExecBuckets(t, f), f.node.MemberID, -2)
	if stillBlocked := commissionPaymentRead(t, f, paid.ID); stillBlocked.State != "blocked" || stillBlocked.PaidCount != "1" {
		t.Fatalf("actual correction rewrote the original blocked payment: %+v", stillBlocked)
	}
	finalTotals := commissionAnalysisExpectedTotals{
		observed: "0", calculated: "0", calculationComplete: true,
		paidCount: "1", paid: "1", adjustmentCount: "1", adjustmentCredit: "1", adjustmentDebit: "0",
		correctionCount: "1", correctionCredit: "0", correctionDebit: "2", postingCount: "3", actual: "0", manualNet: "1",
		effective: "0", calculationGap: "0", effectiveGap: "0", effectiveComplete: true,
	}
	final, body, err := readCommissionAnalysisSnapshot(t, f, f.betting.brand, query, true)
	if err != nil {
		t.Fatalf("read and export complete actual correction history: %v", err)
	}
	assertCommissionAnalysisCycleReport(t, final, f, cycle, query, finalTotals)
	csvRows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\xef\xbb\xbf"))).ReadAll()
	if err != nil || len(csvRows) != 3 || len(csvRows[0]) != 34 || len(csvRows[1]) != 34 || len(csvRows[2]) != 34 || csvRows[1][0] != "summary" || csvRows[2][0] != "group" {
		t.Fatalf("analysis CSV is not the full 34-column summary/group export: rows=%d err=%v", len(csvRows), err)
	}
	if csvRows[1][16] != "0" || csvRows[1][17] != "0" || csvRows[1][18] != "1" || csvRows[1][19] != "1" ||
		csvRows[1][20] != "1" || csvRows[1][21] != "1" || csvRows[1][22] != "0" || csvRows[1][23] != "1" ||
		csvRows[1][24] != "0" || csvRows[1][25] != "2" || csvRows[1][26] != "3" || csvRows[1][27] != "0" ||
		csvRows[1][28] != "1" || csvRows[1][29] != "0" ||
		csvRows[1][30] != "0" || csvRows[1][31] != "0" || csvRows[1][32] != "true" || csvRows[1][33] != "true" {
		t.Fatalf("CSV summary lost the real corrected financial facts: %v", csvRows[1])
	}

	// A one-nanosecond shift beyond the inclusive saved end ceilings both
	// database bounds past that cycle. Empty selections remain complete zeroes.
	emptyQuery := query
	emptyQuery.From = cycle.WindowTo.Add(time.Nanosecond)
	emptyQuery.To = cycle.WindowTo.Add(2 * time.Nanosecond)
	empty, _, err := readCommissionAnalysisSnapshot(t, f, f.betting.brand, emptyQuery, false)
	if err != nil {
		t.Fatalf("valid empty analysis range: %v", err)
	}
	if empty.Coverage != (reporting.CommissionAnalysisCoverage{SelectedCycleCount: "0", ReadyCycleCount: "0", UnreadyCycleCount: "0", LegacyPolicyBlockedCycleCount: "0"}) || empty.TotalGroups != "0" || len(empty.Items) != 0 ||
		empty.Summary.CalculatedPoints == nil || *empty.Summary.CalculatedPoints != "0" || !empty.Summary.CalculationComplete ||
		empty.Summary.EffectiveTargetPoints == nil || *empty.Summary.EffectiveTargetPoints != "0" || !empty.Summary.EffectiveTargetComplete {
		t.Fatalf("empty selection should be complete zero, not missing/unknown: %+v", empty)
	}
	assertCommissionAnalysisTotals(t, empty.Summary, commissionAnalysisExpectedTotals{
		observed: "0", calculated: "0", calculationComplete: true,
		paidCount: "0", paid: "0", adjustmentCount: "0", adjustmentCredit: "0", adjustmentDebit: "0",
		correctionCount: "0", correctionCredit: "0", correctionDebit: "0", postingCount: "0", actual: "0", manualNet: "0",
		effective: "0", calculationGap: "0", effectiveGap: "0", effectiveComplete: true,
	})
	badGroup := query
	badGroup.GroupBy = "day"
	if _, _, err = readCommissionAnalysisSnapshot(t, f, f.betting.brand, badGroup, false); !errors.Is(err, reporting.ErrInvalid) {
		t.Fatalf("day grouping error=%v, want ErrInvalid", err)
	}
	if _, _, err = readCommissionAnalysisSnapshot(t, f, storeTestOtherBrand, query, false); !errors.Is(err, reporting.ErrNotFound) {
		t.Fatalf("cross-brand saved-cycle filter error=%v, want ErrNotFound", err)
	}
	assertCommissionAnalysisCoupledSnapshotTamperingClosed(t, f, query)

	// Corrupt only this isolated test schema's previously actual ledger witness,
	// then ensure neither a broad read nor a filter matching the bad target can
	// silently turn broken source evidence into a zero-valued report.
	tx, err := f.betting.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `ALTER TABLE point_ledger_entries DISABLE TRIGGER ledger_immutable`); err == nil {
		tag, updateErr := tx.Exec(ctx, `UPDATE point_ledger_entries SET reference_type='corrupt_commission_analysis_test' WHERE id=$1`, *target.LedgerEntryID)
		err = updateErr
		if err == nil && tag.RowsAffected() != 1 {
			err = errors.New("corruption fixture did not update exactly one owned ledger row")
		}
	}
	if _, enableErr := tx.Exec(ctx, `ALTER TABLE point_ledger_entries ENABLE TRIGGER ledger_immutable`); err == nil {
		err = enableErr
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("create isolated corrupt-ledger witness: %v", err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	for _, filtered := range []bool{false, true} {
		corruptQuery := query
		if !filtered {
			corruptQuery.AgentID, corruptQuery.MemberID, corruptQuery.CycleID = nil, nil, nil
		}
		if _, _, err = readCommissionAnalysisSnapshot(t, f, f.betting.brand, corruptQuery, false); !errors.Is(err, reporting.ErrAnalysisIntegrity) {
			t.Fatalf("corrupt source with filtered=%v returned %v, want ErrAnalysisIntegrity", filtered, err)
		}
	}
}

func assertCommissionAnalysisCoupledSnapshotTamperingClosed(t *testing.T, f commissionBatchFixture, query reporting.CommissionAnalysisQuery) {
	t.Helper()
	ctx := context.Background()
	tx, err := f.betting.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	// Controlled, rollback-only metadata corruption in this random owned schema.
	// Both copies agree, so the ordinary epoch/order equality test alone cannot
	// prove that the captured policy was ever an approved immutable revision.
	if _, err = tx.Exec(ctx, `SET LOCAL session_replication_role=replica`); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"bet_orders", "commission_calculations"} {
		column, key := "commission_rule_snapshot", "id"
		if table == "commission_calculations" {
			column, key = "rule_snapshot", "order_id"
		}
		sql := `UPDATE ` + table + ` SET ` + column + `=jsonb_set(` + column + `,'{financial_policy,config,payout_mode}','"automatic"'::jsonb) WHERE brand_id=$1 AND ` + key + `=$2`
		if _, err = tx.Exec(ctx, sql, f.betting.brand, f.orders[0].ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = tx.Exec(ctx, `SET LOCAL session_replication_role=origin`); err != nil {
		t.Fatal(err)
	}
	var superficiallyCurrent bool
	if err = tx.QueryRow(ctx, `SELECT commission_payment_evidence_current(brand_id,id,current_run_id,evidence_epoch) FROM commission_cycles WHERE brand_id=$1 AND id=$2`, f.betting.brand, *query.CycleID).Scan(&superficiallyCurrent); err != nil || !superficiallyCurrent {
		t.Fatalf("fixture did not isolate the immutable revision proof: current=%v err=%v", superficiallyCurrent, err)
	}
	if _, err = (reporting.Service{DB: f.betting.db}).CommissionAnalysisRead(ctx, tx, f.betting.brand, query); !errors.Is(err, reporting.ErrAnalysisIntegrity) {
		t.Fatalf("matching but unapproved policy snapshots became known accrual: %v", err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if restored, _, err := readCommissionAnalysisSnapshot(t, f, f.betting.brand, query, false); err != nil || !restored.Summary.CalculationComplete {
		t.Fatalf("rollback did not restore genuine source evidence: %+v %v", restored, err)
	}
}

func TestCommissionAnalysisCreatedUnreadyCycleIsUnknownNotZero(t *testing.T) {
	f := newCommissionBatchFixture(t)
	waitCommissionBoundary(t, f.boundary)
	cycle := createCommissionCycle(t, f)
	if cycle.State == "ready" || cycle.CurrentRunID != nil {
		t.Fatalf("newly created cycle unexpectedly has completed calculation evidence: %+v", cycle)
	}
	query := commissionAnalysisCycleQuery(cycle, "cycle")
	report, _, err := readCommissionAnalysisSnapshot(t, f, f.betting.brand, query, false)
	if err != nil {
		t.Fatalf("read selected but unready cycle: %v", err)
	}
	if report.Coverage.SelectedCycleCount != "1" || report.Coverage.ReadyCycleCount != "0" || report.Coverage.UnreadyCycleCount != "1" ||
		report.Summary.CalculatedPoints != nil || report.Summary.EffectiveTargetPoints != nil ||
		report.Summary.CalculationMinusActualPoints != nil || report.Summary.EffectiveMinusActualPoints != nil ||
		report.Summary.CalculationComplete || report.Summary.EffectiveTargetComplete || report.Summary.ActualNetPoints != "0" {
		t.Fatalf("unready calculation must remain nullable rather than look like a completed zero: %+v", report)
	}
	if len(report.Items) != 1 || report.Items[0].Totals.CalculatedPoints != nil || report.Items[0].Totals.EffectiveTargetPoints != nil {
		t.Fatalf("cycle group hid unknown calculation state: %+v", report.Items)
	}
}
