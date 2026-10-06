package main

import (
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/workbench"
)

func workbenchReady[T any](data T) workbench.Section[T] {
	return workbench.Section[T]{Status: "ready", Data: &data}
}

// workbenchExamples returns a serialized example built from the authoritative
// workbench DTO. It is data only and does not open a database or create users.
func workbenchExamples() map[string]any {
	const id = "11111111-1111-4111-8111-111111111111"
	snapshotAt := time.Date(2026, 10, 6, 12, 30, 0, 0, time.UTC)
	dayFrom := time.Date(2026, 10, 6, 0, 0, 0, 0, time.FixedZone("SGT", 8*60*60))
	completedAt := snapshotAt.Add(-time.Hour)
	lastAttemptAt := snapshotAt.Add(-15 * time.Minute)
	snapshot := workbench.Snapshot{
		BrandID: id, SnapshotAt: snapshotAt, Timezone: "Asia/Singapore", DayFrom: dayFrom,
		Brand:          workbenchReady(workbench.Brand{Name: "Example", Code: "example", State: "active"}),
		Periods:        workbenchReady(workbench.Periods{Pending: "2", Betting: "1", Closed: "3", WaitingDraw: "1", Drawn: "4", Settling: "0", RefundPending: "0", RefundFailed: "0"}),
		Orders:         workbenchReady(workbench.Orders{Placed: "18", Abnormal: "1"}),
		TodayBets:      workbenchReady(workbench.TodayBets{OrderCount: "18", StakePoints: "9000000000000000000", CancelledCount: "2", AbnormalCount: "1"}),
		Settlement:     workbenchReady(workbench.Settlement{Processing: "1", AwaitingApproval: "0", Paying: "0", Failed: "0"}),
		Recharges:      workbenchReady(workbench.Recharges{PendingCount: "2", PendingPoints: "500"}),
		Ledger:         workbenchReady(workbench.Ledger{EntryCount: "24", NetPoints: "-500", RechargePoints: "1000", PrizeCreditPoints: "200", PrizeReversalPoints: "100", RefundPoints: "100"}),
		Balances:       workbenchReady(workbench.Balances{AccountCount: "12", AvailablePoints: "9000000000000000000", FrozenPoints: "100", WithdrawalPoints: "50", TotalPoints: "9000000000000000150"}),
		Reconciliation: workbenchReady(workbench.Reconciliation{LatestJob: &workbench.ReconciliationJob{ID: id, State: "completed", CreatedAt: snapshotAt.Add(-2 * time.Hour), CompletedAt: &completedAt, TargetCount: "12", CheckedCount: "12", RepairableCount: "0", CorruptCount: "0", FailedCount: "0"}}),
		Sources:        workbenchReady(workbench.Sources{AdapterState: "stub", ConfiguredGames: "2", EnabledAPISources: "1", EnabledDOMSources: "0", AttemptsToday: "3", FailedToday: "1", NoDataToday: "0", LastAttemptAt: &lastAttemptAt}),
		Withdrawals:    workbench.Section[struct{}]{Status: "not_implemented"},
		Commissions:    workbench.Section[struct{}]{Status: "not_implemented"},
		Rewards:        workbench.Section[struct{}]{Status: "not_implemented"},
	}
	return map[string]any{"AdminWorkbench": snapshot}
}
