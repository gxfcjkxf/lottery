package main

import (
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/reporting"
)

func withdrawalReportExamples() map[string]any {
	const id = "11111111-1111-4111-8111-111111111111"
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	totals := reporting.WithdrawalTotals{OrderCount: "1", RequestedPoints: "8", ReviewingCount: "0", ReviewingPoints: "0", ProcessingCount: "0", ProcessingPoints: "0", PaidCount: "1", PaidPoints: "8", RejectedCount: "0", RejectedPoints: "0", FailedCount: "0", FailedPoints: "0", CancelledCount: "0", CancelledPoints: "0"}
	report := reporting.WithdrawalReport{BrandID: id, SnapshotAt: to, Timezone: "Asia/Singapore", Query: reporting.Query{From: from, To: to, GroupBy: "day", Limit: 20, Offset: 0}, Summary: totals, Items: []reporting.Group[reporting.WithdrawalTotals]{{Key: "2026-10-01", Label: "2026-10-01", Totals: totals}}, TotalGroups: "1"}
	return map[string]any{"WithdrawalReportExample": report}
}
