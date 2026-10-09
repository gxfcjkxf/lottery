package reporting

import (
	"encoding/csv"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"
)

func analysisString(value string) *string { return &value }

func commissionAnalysisCSVFixture() CommissionAnalysisReport {
	q := CommissionAnalysisQuery{From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		To: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), GroupBy: "agent", Limit: 20}
	// Same-generation manual adjustment: calculation 10, actual/effective 12.
	totals := CommissionAnalysisTotals{
		ObservedCalculatedPoints: "10", CalculatedPoints: analysisString("10"),
		PaidEntryCount: "1", PaidPoints: "10", AdjustmentEntryCount: "1",
		AdjustmentCreditPoints: "2", AdjustmentDebitPoints: "0",
		CorrectionEntryCount: "0", CorrectionCreditPoints: "0", CorrectionDebitPoints: "0",
		PostingEntryCount: "2", ActualNetPoints: "12", ManualAdjustmentNetPoints: "2",
		EffectiveTargetPoints: analysisString("12"), CalculationMinusActualPoints: analysisString("-2"),
		EffectiveMinusActualPoints: analysisString("0"), CalculationComplete: true, EffectiveTargetComplete: true,
	}
	key := "0199a000-0000-7000-8000-000000000002"
	return CommissionAnalysisReport{
		BrandID:    "0199a000-0000-7000-8000-000000000001",
		SnapshotAt: time.Date(2026, 2, 2, 3, 4, 5, 0, time.UTC), Timezone: "Asia/Singapore", Query: q,
		Coverage: CommissionAnalysisCoverage{SelectedCycleCount: "1", ReadyCycleCount: "1", UnreadyCycleCount: "0"},
		Summary:  totals, Items: []Group[CommissionAnalysisTotals]{{Key: key, Label: key, Totals: totals}}, TotalGroups: "1",
	}
}

func TestCommissionAnalysisCSVContractAndExactSignedFields(t *testing.T) {
	r := commissionAnalysisCSVFixture()
	body, err := CommissionAnalysisCSV(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) > CSVByteLimit || !strings.HasPrefix(string(body), "\xef\xbb\xbfrecord_type,") {
		t.Fatalf("missing BOM/header or byte cap: %d bytes", len(body))
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\xef\xbb\xbf"))).ReadAll()
	if err != nil || len(rows) != 3 {
		t.Fatalf("bad CSV: rows=%d err=%v", len(rows), err)
	}
	if len(rows[0]) != 33 || len(rows[1]) != 33 || rows[1][0] != "summary" || rows[2][0] != "group" {
		t.Fatalf("CSV column count/order differs from doc 38: header=%v", rows[0])
	}
	if rows[1][12] != "1" || rows[1][13] != "1" || rows[1][14] != "0" {
		t.Fatalf("coverage columns drifted: %v", rows[1][12:15])
	}
	if rows[1][15] != "10" || rows[1][16] != "10" || rows[1][26] != "12" ||
		rows[1][27] != "2" || rows[1][28] != "12" || rows[1][29] != "'-2" || rows[1][30] != "0" ||
		rows[1][31] != "true" || rows[1][32] != "true" {
		t.Fatalf("exact totals/signed values/status changed: %v", rows[1])
	}
	if rows[2][10] != r.Items[0].Key || rows[2][11] != r.Items[0].Key {
		t.Fatalf("group metadata mismatch: %v", rows[2][10:12])
	}
}

func TestCommissionAnalysisCSVNewEpochDoesNotReapplyOldManualDelta(t *testing.T) {
	r := commissionAnalysisCSVFixture()
	newEpoch := r.Summary
	newEpoch.ObservedCalculatedPoints = "8"
	newEpoch.CalculatedPoints = analysisString("8")
	newEpoch.EffectiveTargetPoints = analysisString("8")
	newEpoch.CalculationMinusActualPoints = analysisString("-4")
	newEpoch.EffectiveMinusActualPoints = analysisString("-4")
	r.Summary, r.Items[0].Totals = newEpoch, newEpoch
	if _, err := CommissionAnalysisCSV(r); err != nil {
		t.Fatalf("valid corrected epoch was rejected: %v", err)
	}
}

func TestCommissionAnalysisCSVEmptyAgentSelectionWithUnreadyCoverage(t *testing.T) {
	r := commissionAnalysisCSVFixture()
	r.Coverage = CommissionAnalysisCoverage{SelectedCycleCount: "1", ReadyCycleCount: "0", UnreadyCycleCount: "1"}
	r.Items, r.TotalGroups = nil, "0"
	r.Summary = CommissionAnalysisTotals{
		ObservedCalculatedPoints: "0", PaidEntryCount: "0", PaidPoints: "0", AdjustmentEntryCount: "0",
		AdjustmentCreditPoints: "0", AdjustmentDebitPoints: "0", CorrectionEntryCount: "0",
		CorrectionCreditPoints: "0", CorrectionDebitPoints: "0", PostingEntryCount: "0",
		ActualNetPoints: "0", ManualAdjustmentNetPoints: "0", CalculationComplete: false, EffectiveTargetComplete: false,
	}
	body, err := CommissionAnalysisCSV(r)
	if err != nil {
		t.Fatalf("empty agent selection with selected unready cycle must preserve unknown summary: %v", err)
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\xef\xbb\xbf"))).ReadAll()
	if err != nil || len(rows) != 2 || rows[1][16] != "" || rows[1][28] != "" || rows[1][29] != "" || rows[1][30] != "" || rows[1][31] != "false" || rows[1][32] != "false" {
		t.Fatalf("nullable empty summary changed: rows=%v err=%v", rows, err)
	}
}

func TestValidCommissionAnalysisTotalsRejectsMalformedArithmeticAndNullability(t *testing.T) {
	base := commissionAnalysisCSVFixture().Summary
	cases := []struct {
		name string
		edit func(*CommissionAnalysisTotals)
	}{
		{"posting count", func(v *CommissionAnalysisTotals) { v.PostingEntryCount = "1" }},
		{"actual net", func(v *CommissionAnalysisTotals) { v.ActualNetPoints = "11" }},
		{"manual net", func(v *CommissionAnalysisTotals) { v.ManualAdjustmentNetPoints = "1" }},
		{"calculation gap", func(v *CommissionAnalysisTotals) { v.CalculationMinusActualPoints = analysisString("2") }},
		{"effective gap", func(v *CommissionAnalysisTotals) { v.EffectiveMinusActualPoints = analysisString("1") }},
		{"observed mismatch", func(v *CommissionAnalysisTotals) { v.ObservedCalculatedPoints = "9" }},
		{"incomplete with value", func(v *CommissionAnalysisTotals) { v.CalculationComplete = false }},
		{"complete without nullable value", func(v *CommissionAnalysisTotals) { v.CalculatedPoints = nil }},
		{"incomplete with difference", func(v *CommissionAnalysisTotals) { v.CalculationComplete = false; v.CalculatedPoints = nil }},
		{"effective complete without calculation", func(v *CommissionAnalysisTotals) {
			v.CalculationComplete = false
			v.CalculatedPoints = nil
			v.CalculationMinusActualPoints = nil
		}},
		{"noncanonical decimal", func(v *CommissionAnalysisTotals) { v.PaidPoints = "01" }},
		{"negative observed", func(v *CommissionAnalysisTotals) { v.ObservedCalculatedPoints = "-1" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := base
			tc.edit(&v)
			if validCommissionAnalysisTotals(v) {
				t.Fatal("accepted tampered totals")
			}
		})
	}
}

func TestCommissionAnalysisCSVRejectsCoverageGroupMathAndOrderingTampering(t *testing.T) {
	cases := []struct {
		name string
		edit func(*CommissionAnalysisReport)
	}{
		{"coverage equation", func(r *CommissionAnalysisReport) { r.Coverage.UnreadyCycleCount = "1" }},
		{"summary differs from groups", func(r *CommissionAnalysisReport) {
			r.Summary.ActualNetPoints = "13"
			r.Summary.CalculationMinusActualPoints = analysisString("-3")
			r.Summary.EffectiveMinusActualPoints = analysisString("-1")
		}},
		{"duplicate group key", func(r *CommissionAnalysisReport) { r.Items = append(r.Items, r.Items[0]); r.TotalGroups = "2" }},
		{"unsorted groups", func(r *CommissionAnalysisReport) {
			second := r.Items[0]
			second.Key, second.Label = "0199a000-0000-7000-8000-000000000003", "0199a000-0000-7000-8000-000000000003"
			r.Items = []Group[CommissionAnalysisTotals]{second, r.Items[0]}
			r.TotalGroups = "2"
		}},
		{"noncanonical group key", func(r *CommissionAnalysisReport) {
			r.Items[0].Key = strings.ToUpper(r.Items[0].Key)
			r.Items[0].Label = r.Items[0].Key
		}},
		{"cycle groups omit selected cycle", func(r *CommissionAnalysisReport) { r.Query.GroupBy = "cycle"; r.Items = nil; r.TotalGroups = "0" }},
		{"paged CSV", func(r *CommissionAnalysisReport) { r.Query.Offset = 1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := commissionAnalysisCSVFixture()
			tc.edit(&r)
			if _, err := CommissionAnalysisCSV(r); err != ErrInvalid {
				t.Fatalf("expected ErrInvalid, got %v", err)
			}
		})
	}
}

func TestCommissionAnalysisCSVExactIntegersAndNullablePropagation(t *testing.T) {
	r := commissionAnalysisCSVFixture()
	large := "900719925474099312345678901234567890"
	v := r.Summary
	v.ObservedCalculatedPoints = large
	v.CalculatedPoints = analysisString(large)
	v.PaidPoints = large
	v.ActualNetPoints = large
	v.ManualAdjustmentNetPoints = "0"
	v.AdjustmentCreditPoints, v.AdjustmentEntryCount = "0", "0"
	v.PaidEntryCount, v.PostingEntryCount = "1", "1"
	v.CalculationMinusActualPoints = analysisString("0")
	v.EffectiveTargetPoints = analysisString(large)
	v.EffectiveMinusActualPoints = analysisString("0")
	r.Summary, r.Items[0].Totals = v, v
	if _, err := CommissionAnalysisCSV(r); err != nil {
		t.Fatalf("rejected exact arbitrary-precision totals: %v", err)
	}

	partial := v
	partial.CalculatedPoints, partial.EffectiveTargetPoints = nil, nil
	partial.CalculationMinusActualPoints, partial.EffectiveMinusActualPoints = nil, nil
	partial.CalculationComplete, partial.EffectiveTargetComplete = false, false
	r.Summary, r.Items[0].Totals = partial, partial
	if _, err := CommissionAnalysisCSV(r); err != nil {
		t.Fatalf("rejected fully nullable incomplete group and summary: %v", err)
	}
	r.Summary = v
	if _, err := CommissionAnalysisCSV(r); err != ErrInvalid {
		t.Fatalf("accepted complete summary when a group propagates nullability: %v", err)
	}
}

func TestCommissionAnalysisCSVGroupLimitAndByteLimitReturnNoPartialBody(t *testing.T) {
	r := commissionAnalysisCSVFixture()
	r.Items = make([]Group[CommissionAnalysisTotals], ExportGroupLimit+1)
	if body, err := CommissionAnalysisCSV(r); err != ErrExportTooLarge || body != nil {
		t.Fatalf("group cap must return no bytes: len=%d err=%v", len(body), err)
	}

	r = commissionAnalysisCSVFixture()
	large := strings.Repeat("9", CSVByteLimit/2+100)
	v := r.Summary
	v.ObservedCalculatedPoints = "0"
	v.CalculatedPoints, v.EffectiveTargetPoints = nil, nil
	v.CalculationMinusActualPoints, v.EffectiveMinusActualPoints = nil, nil
	v.CalculationComplete, v.EffectiveTargetComplete = false, false
	v.PaidPoints, v.ActualNetPoints = large, large
	v.AdjustmentCreditPoints, v.AdjustmentDebitPoints = "0", "0"
	v.ManualAdjustmentNetPoints = "0"
	r.Summary, r.Items[0].Totals = v, v
	if body, err := CommissionAnalysisCSV(r); err != ErrExportTooLarge || body != nil {
		t.Fatalf("byte cap must return no partial body: bytes=%d err=%v", len(body), err)
	}
}

func TestCommissionAnalysisCSVAllColumnsArePresentAndStable(t *testing.T) {
	want := strings.Join([]string{
		"record_type", "brand_id", "snapshot_at", "timezone", "from", "to", "group_by", "agent_id", "member_id", "cycle_id", "key", "label",
		"selected_cycle_count", "ready_cycle_count", "unready_cycle_count",
		"observed_calculated_points", "calculated_points", "paid_entry_count", "paid_points", "adjustment_entry_count", "adjustment_credit_points", "adjustment_debit_points",
		"correction_entry_count", "correction_credit_points", "correction_debit_points", "posting_entry_count", "actual_net_points", "manual_adjustment_net_points",
		"effective_target_points", "calculation_minus_actual_points", "effective_minus_actual_points", "calculation_complete", "effective_target_complete",
	}, ",")
	if got := strings.Join(commissionAnalysisCSVFields, ","); got != want || len(commissionAnalysisCSVFields) != 33 {
		t.Fatalf("column contract drifted: %s", got)
	}
}

func TestCommissionAnalysisCSVLargeArithmeticUsesBigInt(t *testing.T) {
	large := new(big.Int).SetUint64(^uint64(0))
	if large.String() != "18446744073709551615" {
		t.Fatal(fmt.Sprint(large))
	}
	r := commissionAnalysisCSVFixture()
	v := r.Summary
	v.ObservedCalculatedPoints, v.CalculatedPoints = "18446744073709551616", analysisString("18446744073709551616")
	v.PaidPoints, v.ActualNetPoints = "18446744073709551617", "18446744073709551617"
	v.AdjustmentCreditPoints, v.AdjustmentDebitPoints = "0", "0"
	v.AdjustmentEntryCount, v.PaidEntryCount, v.PostingEntryCount = "0", "1", "1"
	v.ManualAdjustmentNetPoints = "0"
	v.CalculationMinusActualPoints = analysisString("-1")
	v.EffectiveTargetPoints, v.EffectiveMinusActualPoints = analysisString("18446744073709551616"), analysisString("-1")
	r.Summary, r.Items[0].Totals = v, v
	if _, err := CommissionAnalysisCSV(r); err != nil {
		t.Fatalf("rejected arbitrary precision subtraction: %v", err)
	}
}
