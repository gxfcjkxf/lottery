package betting

import (
	"context"
	"encoding/csv"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/reporting"
	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

func commissionAnalysisMulticycleZero() commissionAnalysisExpectedTotals {
	return commissionAnalysisExpectedTotals{
		observed: "0", calculated: "0", calculationComplete: true,
		paidCount: "0", paid: "0", adjustmentCount: "0", adjustmentCredit: "0", adjustmentDebit: "0",
		correctionCount: "0", correctionCredit: "0", correctionDebit: "0", postingCount: "0", actual: "0", manualNet: "0",
		effective: "0", calculationGap: "0", effectiveGap: "0", effectiveComplete: true,
	}
}

func assertCommissionAnalysisMulticycleGroups(t *testing.T, report reporting.CommissionAnalysisReport, coverage reporting.CommissionAnalysisCoverage, summary commissionAnalysisExpectedTotals, groups map[string]commissionAnalysisExpectedTotals) {
	t.Helper()
	if report.Coverage != coverage || report.TotalGroups != strconv.Itoa(len(groups)) || len(report.Items) != len(groups) {
		t.Fatalf("multicycle coverage/groups: coverage=%+v groups=%s/%d, want %+v/%d", report.Coverage, report.TotalGroups, len(report.Items), coverage, len(groups))
	}
	assertCommissionAnalysisTotals(t, report.Summary, summary)
	lastKey := ""
	for _, group := range report.Items {
		want, ok := groups[group.Key]
		if !ok || group.Label != group.Key || group.Key <= lastKey {
			t.Fatalf("unexpected, duplicated, or unsorted saved group: %+v", group)
		}
		lastKey = group.Key
		assertCommissionAnalysisTotals(t, group.Totals, want)
	}
}

// CSV expectations use the eighteen doc-38 fields, including blank nullable
// amounts and the apostrophe prefix required for negative signed amounts.
func commissionAnalysisMulticycleCSVValues(want commissionAnalysisExpectedTotals) []string {
	values := []string{want.observed, "", want.paidCount, want.paid,
		want.adjustmentCount, want.adjustmentCredit, want.adjustmentDebit,
		want.correctionCount, want.correctionCredit, want.correctionDebit,
		want.postingCount, want.actual, want.manualNet, "", "", "",
		strconv.FormatBool(want.calculationComplete), strconv.FormatBool(want.effectiveComplete)}
	if want.calculationComplete {
		values[1], values[14] = want.calculated, want.calculationGap
	}
	if want.effectiveComplete {
		values[13], values[15] = want.effective, want.effectiveGap
	}
	for _, i := range []int{11, 12, 14, 15} {
		if strings.HasPrefix(values[i], "-") {
			values[i] = "'" + values[i]
		}
	}
	return values
}

func assertCommissionAnalysisMulticycleCSV(t *testing.T, body []byte, q reporting.CommissionAnalysisQuery, coverage reporting.CommissionAnalysisCoverage, summary commissionAnalysisExpectedTotals, groups map[string]commissionAnalysisExpectedTotals) {
	t.Helper()
	if !strings.HasPrefix(string(body), "\xef\xbb\xbf") {
		t.Fatal("multicycle CSV is missing its UTF-8 BOM")
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\xef\xbb\xbf"))).ReadAll()
	if err != nil || len(rows) != len(groups)+2 {
		t.Fatalf("full multicycle CSV rows=%d, want %d: %v", len(rows), len(groups)+2, err)
	}
	wantFields := []string{"observed_calculated_points", "calculated_points", "paid_entry_count", "paid_points",
		"adjustment_entry_count", "adjustment_credit_points", "adjustment_debit_points", "correction_entry_count",
		"correction_credit_points", "correction_debit_points", "posting_entry_count", "actual_net_points",
		"manual_adjustment_net_points", "effective_target_points", "calculation_minus_actual_points", "effective_minus_actual_points",
		"calculation_complete", "effective_target_complete"}
	if len(rows[0]) != 34 || !reflect.DeepEqual(rows[0][16:], wantFields) {
		t.Fatalf("multicycle CSV totals header drifted: %v", rows[0])
	}
	wantCoverage := []string{coverage.SelectedCycleCount, coverage.ReadyCycleCount, coverage.UnreadyCycleCount, coverage.LegacyPolicyBlockedCycleCount}
	lastKey := ""
	for i, row := range rows[1:] {
		if len(row) != 34 || row[4] != q.From.UTC().Format(time.RFC3339Nano) || row[5] != q.To.UTC().Format(time.RFC3339Nano) || row[6] != q.GroupBy || !reflect.DeepEqual(row[12:16], wantCoverage) {
			t.Fatalf("multicycle CSV metadata/coverage drifted: %v", row)
		}
		want := summary
		if i == 0 {
			if row[0] != "summary" || row[10] != "" || row[11] != "" {
				t.Fatalf("multicycle CSV summary metadata: %v", row)
			}
		} else {
			var ok bool
			want, ok = groups[row[10]]
			if !ok || row[0] != "group" || row[11] != row[10] || row[10] <= lastKey {
				t.Fatalf("multicycle CSV omitted/duplicated a saved beneficiary or cycle: %v", row)
			}
			lastKey = row[10]
		}
		if !reflect.DeepEqual(row[16:], commissionAnalysisMulticycleCSVValues(want)) {
			t.Fatalf("multicycle CSV amounts/nullability = %v, want %v", row[16:], commissionAnalysisMulticycleCSVValues(want))
		}
	}
}

func openCommissionAnalysisSecondPeriod(t *testing.T, f commissionBatchFixture) commissionBatchFixture {
	t.Helper()
	ctx := context.Background()
	second := f
	now := time.Now().UTC()
	bettingTx(t, f.betting.db, func(tx pgx.Tx) error {
		var err error
		second.betting.period, err = (rulebook.Store{DB: f.betting.db}).OpenPeriod(ctx, tx, f.betting.brand, f.betting.game.ID,
			"analysis-second-"+ids.New()[:8], now, now.Add(20*time.Second), now.Add(22*time.Second))
		return err
	})
	second.boundary = second.betting.period.BetEndAt.UTC().Truncate(time.Second).Add(-3 * time.Second)
	weekday := int(second.boundary.Weekday())
	calendar := commission.Calendar{Timezone: "UTC", Cycle: "weekly", BoundaryTime: second.boundary.Format("15:04:05"), Weekday: &weekday}
	policy, err := f.service.Policy(ctx, f.betting.brand)
	if err != nil {
		t.Fatal(err)
	}
	if err = commissionBatchCallTx(t, f, func(tx pgx.Tx) error {
		_, err := f.service.Update(ctx, tx, f.betting.brand, f.actor, commission.PolicyInput{
			Version: policy.Version,
			Config:  commission.PolicyConfig{Enabled: true, Calendar: &calendar, PayoutMode: commission.PayoutManual},
			Reason:  "capture a second legitimate commission calendar for multicycle analysis",
		}, points.Metadata{ActorType: "admin", ActorID: f.actor.ID, RequestID: ids.New()})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	second.betting.input.PeriodID = second.betting.period.ID
	second.betting.input.Multiplier = 1
	second.betting.input.Selection = rules.Selection{Digits: [][]int{{0}, {0}, {0}}}
	second.orders = nil
	for i := 0; i < 2; i++ {
		order, err := placeBettingOrder(t, second.betting, second.betting.input, "analysis-second-bet-"+ids.New())
		if err != nil || order.TotalPoints != 1 || !order.PlacedAt.Before(second.boundary) {
			t.Fatalf("second-period real one-point bet missed its saved boundary: %+v err=%v", order, err)
		}
		second.orders = append(second.orders, order)
	}
	return second
}

func TestCommissionAnalysisActualMulticycleMultilevelCoveragePaginationAndCSV(t *testing.T) {
	f := newCommissionBatchFixture(t)
	f, childID := addActualChildAgentBeneficiary(t, f, 10)
	ctx := context.Background()
	waitCommissionBoundary(t, f.boundary)
	first := createCommissionCycle(t, f)
	eligibilitySettle(t, f.betting)
	advanceCommissionWorker(t, f, 40)
	first = readCommissionCycle(t, f, first.ID)
	if first.State != "ready" || !first.EvidenceCurrent || first.TotalPoints != "4" || first.EarningCount != "2" {
		t.Fatalf("first genuine multilevel run must round root 2.6 to 3 and child 1 to 1: %+v", first)
	}
	gate := commissionPaymentPolicy(t, f)
	if err := updateCommissionPaymentPolicy(t, f, commissionPaymentActor(f), true, gate.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessPayments(ctx, 20); err != nil {
		t.Fatal(err)
	}
	payments := commissionPayments(t, f)
	if len(payments.Items) != 1 || payments.Items[0].State != "awaiting_approval" || payments.Items[0].TotalPoints != "4" || payments.Items[0].TargetCount != "2" {
		t.Fatalf("first cycle must register two real manual beneficiaries: %+v", payments.Items)
	}
	paid := payManualCommissionForAdjustment(t, f, payments.Items[0])
	if paid.PaidCount != "2" || paid.PaidPoints != "4" || paid.State != "paid" {
		t.Fatalf("first cycle must actually pay both saved beneficiaries: %+v", paid)
	}
	targets := commissionTargets(t, f, paid.ID)
	var rootTarget *commission.Target
	childPaid := false
	if len(targets.Items) != 2 {
		t.Fatalf("first cycle real paid target count: %+v", targets)
	}
	for i := range targets.Items {
		target := &targets.Items[i]
		if target.State != "paid" || target.LedgerEntryID == nil || target.AdjustmentVersion == nil {
			t.Fatalf("target lacks real payment evidence: %+v", target)
		}
		switch target.AgentID {
		case f.node.ID:
			if target.OriginalPoints != 3 {
				t.Fatalf("root original rounded payment = %d, want 3", target.OriginalPoints)
			}
			rootTarget = target
		case childID:
			if target.OriginalPoints != 1 {
				t.Fatalf("child original rounded payment = %d, want 1", target.OriginalPoints)
			}
			childPaid = true
		default:
			t.Fatalf("unexpected original beneficiary: %+v", target)
		}
	}
	if rootTarget == nil || !childPaid {
		t.Fatalf("real root/child payout was incomplete: %+v", targets.Items)
	}
	adjustment, err := adjustCommission(t, f, f.betting.brand, *rootTarget, commissionAdjustmentActor(f), *rootTarget.AdjustmentVersion, 5)
	if err != nil || adjustment.PointsBefore != 3 || adjustment.PointsAfter != 5 || adjustment.DeltaPoints != 2 || adjustment.LedgerEntryID == "" {
		t.Fatalf("root must actually adjust from 3 to 5 while child retains 1: %+v err=%v", adjustment, err)
	}

	secondFixture := openCommissionAnalysisSecondPeriod(t, f)
	waitCommissionBoundary(t, secondFixture.boundary)
	second := createCommissionCycle(t, secondFixture)
	if second.ID == first.ID || !second.WindowTo.After(first.WindowTo) || reflect.DeepEqual(second.Calendar, first.Calendar) || second.State == "ready" {
		t.Fatalf("second genuine period must create a distinct later unready saved cycle: first=%+v second=%+v", first, second)
	}
	query := reporting.CommissionAnalysisQuery{From: first.WindowTo.UTC(), To: second.WindowTo.UTC().Add(time.Nanosecond), GroupBy: "cycle", Limit: 20}
	firstTotals := commissionAnalysisMulticycleZero()
	firstTotals.observed, firstTotals.calculated = "4", "4"
	firstTotals.paidCount, firstTotals.paid = "2", "4"
	firstTotals.adjustmentCount, firstTotals.adjustmentCredit, firstTotals.manualNet = "1", "2", "2"
	firstTotals.postingCount, firstTotals.actual, firstTotals.effective, firstTotals.calculationGap = "3", "6", "6", "-2"
	rootFirst := commissionAnalysisMulticycleZero()
	rootFirst.observed, rootFirst.calculated, rootFirst.paidCount, rootFirst.paid = "3", "3", "1", "3"
	rootFirst.adjustmentCount, rootFirst.adjustmentCredit, rootFirst.manualNet = "1", "2", "2"
	rootFirst.postingCount, rootFirst.actual, rootFirst.effective, rootFirst.calculationGap = "2", "5", "5", "-2"
	childFirst := commissionAnalysisMulticycleZero()
	childFirst.observed, childFirst.calculated, childFirst.paidCount, childFirst.paid = "1", "1", "1", "1"
	childFirst.postingCount, childFirst.actual, childFirst.effective = "1", "1", "1"
	pending := commissionAnalysisMulticycleZero()
	pending.calculationComplete, pending.effectiveComplete = false, false
	mixedSummary := firstTotals
	mixedSummary.calculationComplete, mixedSummary.effectiveComplete = false, false
	mixedRoot, mixedChild := rootFirst, childFirst
	mixedRoot.calculationComplete, mixedRoot.effectiveComplete = false, false
	mixedChild.calculationComplete, mixedChild.effectiveComplete = false, false
	mixedCoverage := reporting.CommissionAnalysisCoverage{SelectedCycleCount: "2", ReadyCycleCount: "1", UnreadyCycleCount: "1", LegacyPolicyBlockedCycleCount: "0"}
	mixedCases := []struct {
		group  string
		groups map[string]commissionAnalysisExpectedTotals
	}{
		{"cycle", map[string]commissionAnalysisExpectedTotals{first.ID: firstTotals, second.ID: pending}},
		{"agent", map[string]commissionAnalysisExpectedTotals{f.node.ID: mixedRoot, childID: mixedChild}},
	}
	for _, test := range mixedCases {
		q := query
		q.GroupBy = test.group
		report, body, err := readCommissionAnalysisSnapshot(t, f, f.betting.brand, q, true)
		if err != nil {
			t.Fatalf("mixed ready/unready %s cohort read/export: %v", test.group, err)
		}
		assertCommissionAnalysisMulticycleGroups(t, report, mixedCoverage, mixedSummary, test.groups)
		assertCommissionAnalysisMulticycleCSV(t, body, q, mixedCoverage, mixedSummary, test.groups)
	}

	eligibilitySettle(t, secondFixture.betting)
	advanceCommissionWorker(t, secondFixture, 60)
	second = readCommissionCycle(t, secondFixture, second.ID)
	first = readCommissionCycle(t, f, first.ID)
	if first.State != "ready" || !first.EvidenceCurrent || first.TotalPoints != "4" || second.State != "ready" || !second.EvidenceCurrent || second.CurrentRunID == nil || second.TotalPoints != "1" || second.EarningCount != "1" {
		t.Fatalf("both saved cycles must now be ready without reusing old-calendar accrual: first=%+v second=%+v", first, second)
	}
	// Inspect the real second run through the history service: the earlier
	// losing wagers are scanned but excluded by their saved calendar window.
	historyTx, err := f.betting.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	history, historyErr := f.service.CalculationsTx(ctx, historyTx, f.betting.brand, second.ID, *second.CurrentRunID, 100, 0)
	_ = historyTx.Rollback(ctx)
	if historyErr != nil {
		t.Fatal(historyErr)
	}
	oldLosing := map[string]bool{f.orders[0].ID: true, f.orders[1].ID: true, f.orders[3].ID: true}
	newOrders := map[string]bool{secondFixture.orders[0].ID: true, secondFixture.orders[1].ID: true}
	oldExcluded, newEligible := 0, 0
	for _, item := range history.Items {
		if oldLosing[item.OrderID] {
			if item.Reason != "other_cycle" || item.BasePoints != 0 {
				t.Fatalf("old saved-calendar wager leaked into the second accrual: %+v", item)
			}
			oldExcluded++
		}
		if newOrders[item.OrderID] {
			if item.Reason != "eligible" || item.BasePoints != 1 {
				t.Fatalf("second-period genuine loss did not contribute exactly one base point: %+v", item)
			}
			newEligible++
		}
	}
	if oldExcluded != 3 || newEligible != 2 {
		t.Fatalf("second run source separation: excluded old losses=%d eligible new losses=%d history=%+v", oldExcluded, newEligible, history.Items)
	}

	secondTotals := commissionAnalysisMulticycleZero()
	secondTotals.observed, secondTotals.calculated, secondTotals.effective = "1", "1", "1"
	secondTotals.calculationGap, secondTotals.effectiveGap = "1", "1"
	fullSummary := firstTotals
	fullSummary.observed, fullSummary.calculated, fullSummary.effective = "5", "5", "7"
	fullSummary.calculationGap, fullSummary.effectiveGap = "-1", "1"
	rootAll := rootFirst
	rootAll.observed, rootAll.calculated, rootAll.effective = "4", "4", "6"
	rootAll.calculationGap, rootAll.effectiveGap = "-1", "1"
	readyCoverage := reporting.CommissionAnalysisCoverage{SelectedCycleCount: "2", ReadyCycleCount: "2", UnreadyCycleCount: "0", LegacyPolicyBlockedCycleCount: "0"}
	readyCases := []struct {
		group  string
		groups map[string]commissionAnalysisExpectedTotals
	}{
		{"cycle", map[string]commissionAnalysisExpectedTotals{first.ID: firstTotals, second.ID: secondTotals}},
		{"agent", map[string]commissionAnalysisExpectedTotals{f.node.ID: rootAll, childID: childFirst}},
	}
	for _, test := range readyCases {
		q := query
		q.GroupBy = test.group
		full, _, err := readCommissionAnalysisSnapshot(t, f, f.betting.brand, q, false)
		if err != nil {
			t.Fatalf("ready multicycle %s report: %v", test.group, err)
		}
		assertCommissionAnalysisMulticycleGroups(t, full, readyCoverage, fullSummary, test.groups)
		for offset := 0; offset < 2; offset++ {
			pageQuery := q
			pageQuery.Limit, pageQuery.Offset = 1, offset
			page, _, err := readCommissionAnalysisSnapshot(t, f, f.betting.brand, pageQuery, false)
			if err != nil || page.TotalGroups != "2" || len(page.Items) != 1 || page.Coverage != readyCoverage ||
				!reflect.DeepEqual(page.Summary, full.Summary) || !reflect.DeepEqual(page.Items[0], full.Items[offset]) {
				t.Fatalf("%s page %d must preserve whole-filter totals and saved group order: %+v err=%v", test.group, offset, page, err)
			}
		}
		// Export ignores the JSON page size and includes both complete groups.
		q.Limit = 1
		exported, body, err := readCommissionAnalysisSnapshot(t, f, f.betting.brand, q, true)
		if err != nil {
			t.Fatalf("full %s CSV after pagination: %v", test.group, err)
		}
		assertCommissionAnalysisMulticycleGroups(t, exported, readyCoverage, fullSummary, test.groups)
		assertCommissionAnalysisMulticycleCSV(t, body, q, readyCoverage, fullSummary, test.groups)
	}
	rootQuery := query
	rootQuery.AgentID, rootQuery.MemberID = &f.node.ID, &f.node.MemberID
	rootReport, body, err := readCommissionAnalysisSnapshot(t, f, f.betting.brand, rootQuery, true)
	if err != nil {
		t.Fatalf("saved root beneficiary across both cycles: %v", err)
	}
	rootCycles := map[string]commissionAnalysisExpectedTotals{first.ID: rootFirst, second.ID: secondTotals}
	assertCommissionAnalysisMulticycleGroups(t, rootReport, readyCoverage, rootAll, rootCycles)
	assertCommissionAnalysisMulticycleCSV(t, body, rootQuery, readyCoverage, rootAll, rootCycles)
}
