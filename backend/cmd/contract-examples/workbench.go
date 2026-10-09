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
		Withdrawals:    workbenchReady(workbench.Withdrawals{ReviewingCount: "1", ReviewingPoints: "50", ProcessingCount: "2", ProcessingPoints: "9000000000000000000"}),
		Commissions: workbenchReady(workbench.Commissions{
			DiscoveryPendingCount: "2", DiscoveryFailedCount: "1",
			CycleProcessingCount: "1", CycleWaitingCount: "2", CycleReadyCount: "9007199254740993", CycleStaleCount: "1", CycleFailedCount: "1",
			PaymentAwaitingApprovalCount: "1", PaymentProcessingCount: "2", PaymentBlockedCount: "3", PaymentFailedCount: "1",
			PlanProcessingCount: "1", PlanReadyCount: "2", PlanFailedCount: "1",
			ExecutionAwaitingApprovalCount: "1", ExecutionProcessingCount: "2", ExecutionPausedCount: "3", ExecutionFailedCount: "1",
		}),
		Rewards: workbench.Section[workbench.Rewards]{Status: "ready", Data: &workbench.Rewards{GrantedCount: "1", PendingCount: "2", RevokedCount: "3"}},
	}
	return map[string]any{"AdminWorkbench": snapshot}
}
