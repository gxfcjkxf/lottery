package main

import (
	"github.com/gxfcjkxf/lottery/backend/internal/reporting"
	"time"
)

func commissionAnalysisExample() reporting.CommissionAnalysisReport {
	const id = "11111111-1111-4111-8111-111111111111"
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	calculated, effective, gap := "8", "8", "-4"
	totals := reporting.CommissionAnalysisTotals{ObservedCalculatedPoints: "8", CalculatedPoints: &calculated,
		PaidEntryCount: "1", PaidPoints: "10", AdjustmentEntryCount: "1", AdjustmentCreditPoints: "2", AdjustmentDebitPoints: "0",
		CorrectionEntryCount: "0", CorrectionCreditPoints: "0", CorrectionDebitPoints: "0", PostingEntryCount: "2", ActualNetPoints: "12", ManualAdjustmentNetPoints: "2",
		EffectiveTargetPoints: &effective, CalculationMinusActualPoints: &gap, EffectiveMinusActualPoints: &gap, CalculationComplete: true, EffectiveTargetComplete: true}
	return reporting.CommissionAnalysisReport{BrandID: id, SnapshotAt: now, Timezone: "UTC", Query: reporting.CommissionAnalysisQuery{From: now.Add(-24 * time.Hour), To: now, GroupBy: "cycle", Limit: 20},
		Coverage: reporting.CommissionAnalysisCoverage{SelectedCycleCount: "1", ReadyCycleCount: "1", UnreadyCycleCount: "0"},
		Summary:  totals, Items: []reporting.Group[reporting.CommissionAnalysisTotals]{{Key: id, Label: id, Totals: totals}}, TotalGroups: "1"}
}
