package main

import (
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/reporting"
)

func rewardReportExamples() map[string]any {
	const id = "11111111-1111-4111-8111-111111111111"
	from := time.Date(2026, 10, 8, 0, 0, 0, 123456789, time.UTC)
	q := reporting.RewardQuery{From: from, To: from.Add(time.Hour), GroupBy: "day", Limit: 20}
	p := reporting.RewardTotals{EntryCount: "1", GrantEntryCount: "0", GrantPoints: "0", ReversalEntryCount: "1", ReversalPoints: "9223372036854775807", NetPoints: "-9223372036854775807"}
	o := reporting.RewardOrderTotals{OrderCount: "3", OriginalPoints: "18446744073709551617", GrantedCount: "1", GrantedPoints: "9223372036854775807", PendingCount: "1", PendingPoints: "3", RevokedCount: "1", RevokedPoints: "9223372036854775807"}
	return map[string]any{
		"RewardReport":      reporting.RewardReport{BrandID: id, SnapshotAt: q.To, Timezone: "UTC", Query: q, Summary: p, Items: []reporting.Group[reporting.RewardTotals]{{Key: "2026-10-08", Label: "2026-10-08", Totals: p}}, TotalGroups: "1"},
		"RewardOrderReport": reporting.RewardOrderReport{BrandID: id, SnapshotAt: q.To, Timezone: "UTC", Query: q, Summary: o, Items: []reporting.Group[reporting.RewardOrderTotals]{{Key: "2026-10-08", Label: "2026-10-08", Totals: o}}, TotalGroups: "1"},
	}
}
