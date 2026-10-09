package main

import (
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/reporting"
)

func attributionReportExample() reporting.AttributionReport {
	const brand = "11111111-1111-4111-8111-111111111111"
	stamp := time.Date(2026, 10, 9, 0, 0, 0, 123456789, time.UTC)
	totals := reporting.AttributionTotals{
		BettingTotals: reporting.BettingTotals{
			OrderCount: "2", StakePoints: "18446744073709551614", PlacedCount: "0", WonCount: "0", LostCount: "2",
			AbnormalCount: "0", CancelledCount: "0", RefundPoints: "0", SettledStakePoints: "18446744073709551614",
			UnfinalizedStakePoints: "0", AbnormalStakePoints: "0", CurrentPrizePoints: "0", CorrectionOpenCount: "0",
		},
		FinalLostStakePoints: "18446744073709551614", LegacyAttributionCount: "0",
	}
	return reporting.AttributionReport{
		BrandID: brand, SnapshotAt: stamp, Timezone: "Asia/Manila",
		Query:   reporting.AttributionQuery{From: stamp.Add(-time.Hour), To: stamp, GroupBy: "join_method", Limit: 20, AgentScope: "direct"},
		Summary: totals, Items: []reporting.Group[reporting.AttributionTotals]{{Key: "domain", Label: "domain", Totals: totals}}, TotalGroups: "1",
	}
}
