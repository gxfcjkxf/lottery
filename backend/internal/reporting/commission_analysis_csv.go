package reporting

import (
	"encoding/csv"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"
)

var commissionAnalysisCSVFields = []string{
	"record_type", "brand_id", "snapshot_at", "timezone", "from", "to", "group_by",
	"agent_id", "member_id", "cycle_id", "key", "label",
	"selected_cycle_count", "ready_cycle_count", "unready_cycle_count", "legacy_policy_blocked_cycle_count",
	"observed_calculated_points", "calculated_points", "paid_entry_count", "paid_points",
	"adjustment_entry_count", "adjustment_credit_points", "adjustment_debit_points",
	"correction_entry_count", "correction_credit_points", "correction_debit_points",
	"posting_entry_count", "actual_net_points", "manual_adjustment_net_points",
	"effective_target_points", "calculation_minus_actual_points", "effective_minus_actual_points",
	"calculation_complete", "effective_target_complete",
}

func commissionAnalysisCoverageValues(v CommissionAnalysisCoverage) []string {
	return []string{v.SelectedCycleCount, v.ReadyCycleCount, v.UnreadyCycleCount, v.LegacyPolicyBlockedCycleCount}
}

func validCommissionAnalysisCoverage(v CommissionAnalysisCoverage) bool {
	values := commissionAnalysisCoverageValues(v)
	if decimalFields(values, -1) != nil {
		return false
	}
	selected := commissionAnalysisBig(v.SelectedCycleCount)
	ready := commissionAnalysisBig(v.ReadyCycleCount)
	unready := commissionAnalysisBig(v.UnreadyCycleCount)
	legacy := commissionAnalysisBig(v.LegacyPolicyBlockedCycleCount)
	return selected.Cmp(new(big.Int).Add(ready, unready)) == 0 && legacy.Cmp(selected) <= 0
}

func commissionAnalysisBig(value string) *big.Int {
	n, _ := new(big.Int).SetString(value, 10)
	return n
}

func commissionAnalysisAdd(values ...string) *big.Int {
	out := new(big.Int)
	for _, value := range values {
		out.Add(out, commissionAnalysisBig(value))
	}
	return out
}

func commissionAnalysisSub(left, right string) *big.Int {
	return new(big.Int).Sub(commissionAnalysisBig(left), commissionAnalysisBig(right))
}

func commissionAnalysisSigned(value string) bool { return signedDecimal.MatchString(value) }

func commissionAnalysisEqualNullable(got *string, want *big.Int) bool {
	return got != nil && (*got == want.String())
}

func commissionAnalysisTotalsValues(v CommissionAnalysisTotals) []string {
	values := []string{v.ObservedCalculatedPoints, "", v.PaidEntryCount, v.PaidPoints,
		v.AdjustmentEntryCount, v.AdjustmentCreditPoints, v.AdjustmentDebitPoints,
		v.CorrectionEntryCount, v.CorrectionCreditPoints, v.CorrectionDebitPoints,
		v.PostingEntryCount, v.ActualNetPoints, v.ManualAdjustmentNetPoints, "", "", ""}
	if v.CalculatedPoints != nil {
		values[1] = *v.CalculatedPoints
	}
	if v.EffectiveTargetPoints != nil {
		values[13] = *v.EffectiveTargetPoints
	}
	if v.CalculationMinusActualPoints != nil {
		values[14] = *v.CalculationMinusActualPoints
	}
	if v.EffectiveMinusActualPoints != nil {
		values[15] = *v.EffectiveMinusActualPoints
	}
	return values
}

// validCommissionAnalysisTotals validates canonical decimal text and all
// arithmetic relationships without converting any amount to a machine type.
func validCommissionAnalysisTotals(v CommissionAnalysisTotals) bool {
	values := commissionAnalysisTotalsValues(v)
	for i, value := range values {
		switch i {
		case 11, 12:
			if !commissionAnalysisSigned(value) {
				return false
			}
		case 1, 13:
			if value != "" && !unsignedDecimal.MatchString(value) {
				return false
			}
		case 14, 15:
			if value != "" && !commissionAnalysisSigned(value) {
				return false
			}
		default:
			if !unsignedDecimal.MatchString(value) {
				return false
			}
		}
	}
	if !v.CalculationComplete && v.CalculatedPoints != nil ||
		v.CalculationComplete && v.CalculatedPoints == nil ||
		!v.EffectiveTargetComplete && v.EffectiveTargetPoints != nil ||
		v.EffectiveTargetComplete && v.EffectiveTargetPoints == nil ||
		v.EffectiveTargetComplete && !v.CalculationComplete {
		return false
	}
	if v.CalculationComplete {
		if *v.CalculatedPoints != v.ObservedCalculatedPoints ||
			!commissionAnalysisEqualNullable(v.CalculationMinusActualPoints,
				commissionAnalysisSub(*v.CalculatedPoints, v.ActualNetPoints)) {
			return false
		}
	} else if v.CalculationMinusActualPoints != nil {
		return false
	}
	if v.EffectiveTargetComplete {
		if !commissionAnalysisEqualNullable(v.EffectiveMinusActualPoints,
			commissionAnalysisSub(*v.EffectiveTargetPoints, v.ActualNetPoints)) {
			return false
		}
	} else if v.EffectiveMinusActualPoints != nil {
		return false
	}
	if commissionAnalysisBig(v.PostingEntryCount).Cmp(commissionAnalysisAdd(v.PaidEntryCount, v.AdjustmentEntryCount, v.CorrectionEntryCount)) != 0 {
		return false
	}
	actual := commissionAnalysisAdd(v.PaidPoints, v.AdjustmentCreditPoints, v.CorrectionCreditPoints)
	actual.Sub(actual, commissionAnalysisAdd(v.AdjustmentDebitPoints, v.CorrectionDebitPoints))
	if actual.String() != v.ActualNetPoints {
		return false
	}
	manual := commissionAnalysisSub(v.AdjustmentCreditPoints, v.AdjustmentDebitPoints)
	return manual.String() == v.ManualAdjustmentNetPoints
}

func commissionAnalysisGroupValid(q CommissionAnalysisQuery, item Group[CommissionAnalysisTotals]) bool {
	if item.Key == "" || item.Key != strings.ToLower(item.Key) || item.Label != item.Key || !uuid.MatchString(item.Key) {
		return false
	}
	return q.GroupBy == "cycle" || q.GroupBy == "agent"
}

func commissionAnalysisNullableValue(v CommissionAnalysisTotals, index int) *string {
	switch index {
	case 1:
		return v.CalculatedPoints
	case 13:
		return v.EffectiveTargetPoints
	case 14:
		return v.CalculationMinusActualPoints
	case 15:
		return v.EffectiveMinusActualPoints
	default:
		return nil
	}
}

// CommissionAnalysisCSV exports a complete, validated analysis report.
// It never returns a truncated body when either export limit is exceeded.
func CommissionAnalysisCSV(r CommissionAnalysisReport) ([]byte, error) {
	if len(r.Items) > ExportGroupLimit {
		return nil, ErrExportTooLarge
	}
	q := r.Query
	if !uuid.MatchString(r.BrandID) || r.BrandID != strings.ToLower(r.BrandID) ||
		r.SnapshotAt.IsZero() || r.SnapshotAt.Year() < 1 || r.SnapshotAt.Year() > 9999 ||
		q.Validate() != nil || q.Offset != 0 || r.Timezone == "" || r.Timezone == "Local" ||
		r.TotalGroups != fmt.Sprint(len(r.Items)) || !validCommissionAnalysisCoverage(r.Coverage) ||
		!validCommissionAnalysisTotals(r.Summary) {
		return nil, ErrInvalid
	}
	if _, err := time.LoadLocation(r.Timezone); err != nil {
		return nil, ErrInvalid
	}
	if q.GroupBy == "cycle" && commissionAnalysisBig(r.Coverage.SelectedCycleCount).Cmp(big.NewInt(int64(len(r.Items)))) != 0 {
		return nil, ErrInvalid
	}
	filters := []string{"", "", ""}
	for i, value := range []*string{q.AgentID, q.MemberID, q.CycleID} {
		if value != nil {
			filters[i] = *value
		}
	}
	meta := []string{r.BrandID, r.SnapshotAt.UTC().Format(time.RFC3339Nano), r.Timezone,
		q.From.UTC().Format(time.RFC3339Nano), q.To.UTC().Format(time.RFC3339Nano), q.GroupBy}
	for _, value := range append(append([]string{}, meta...), filters...) {
		if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return nil, ErrInvalid
		}
	}

	// Sum the always-present numeric columns using exact integer arithmetic.
	sums := make([]big.Int, 16)
	nullSeen := make([]bool, 16)
	lastKey := ""
	for i, item := range r.Items {
		if !commissionAnalysisGroupValid(q, item) || i > 0 && item.Key <= lastKey || !validCommissionAnalysisTotals(item.Totals) {
			return nil, ErrInvalid
		}
		lastKey = item.Key
		values := commissionAnalysisTotalsValues(item.Totals)
		for j := range values {
			if j == 1 || j == 13 || j == 14 || j == 15 {
				value := commissionAnalysisNullableValue(item.Totals, j)
				if value == nil {
					nullSeen[j] = true
				} else if n, ok := new(big.Int).SetString(*value, 10); ok {
					sums[j].Add(&sums[j], n)
				} else {
					return nil, ErrInvalid
				}
				continue
			}
			n, ok := new(big.Int).SetString(values[j], 10)
			if !ok {
				return nil, ErrInvalid
			}
			sums[j].Add(&sums[j], n)
		}
	}
	summaryValues := commissionAnalysisTotalsValues(r.Summary)
	for i, want := range summaryValues {
		if i == 1 || i == 13 || i == 14 || i == 15 {
			got := commissionAnalysisNullableValue(r.Summary, i)
			if nullSeen[i] {
				if got != nil {
					return nil, ErrInvalid
				}
				continue
			}
			// Agent grouping may have no captured beneficiary rows while an
			// unready selected cycle still makes the cohort summary unknown.
			if len(r.Items) == 0 && q.GroupBy == "agent" && r.Coverage.UnreadyCycleCount != "0" && got == nil {
				continue
			}
			if got == nil || sums[i].String() != want {
				return nil, ErrInvalid
			}
			continue
		}
		if sums[i].String() != want {
			return nil, ErrInvalid
		}
	}

	target := &cappedCSV{}
	if _, err := target.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return nil, err
	}
	w := csv.NewWriter(target)
	if err := w.Write(commissionAnalysisCSVFields); err != nil {
		return nil, err
	}
	coverage := commissionAnalysisCoverageValues(r.Coverage)
	write := func(record, key, label string, totals CommissionAnalysisTotals) error {
		clean := make([]string, 3)
		for i, raw := range []string{record, key, label} {
			v, err := csvText(raw)
			if err != nil {
				return err
			}
			clean[i] = v
		}
		values := commissionAnalysisTotalsValues(totals)
		for _, index := range []int{11, 12, 14, 15} {
			if strings.HasPrefix(values[index], "-") {
				values[index] = "'" + values[index]
			}
		}
		values = append(values, fmt.Sprint(totals.CalculationComplete), fmt.Sprint(totals.EffectiveTargetComplete))
		row := []string{clean[0], meta[0], meta[1], meta[2], meta[3], meta[4], meta[5],
			filters[0], filters[1], filters[2], clean[1], clean[2]}
		row = append(row, coverage...)
		row = append(row, values...)
		return w.Write(row)
	}
	if err := write("summary", "", "", r.Summary); err != nil {
		return nil, err
	}
	for _, item := range r.Items {
		if err := write("group", item.Key, item.Label, item.Totals); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		if errors.Is(err, ErrExportTooLarge) {
			return nil, ErrExportTooLarge
		}
		return nil, err
	}
	return target.data.Bytes(), nil
}
