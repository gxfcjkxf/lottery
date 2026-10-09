//go:build capacity && !windows

package httpapi

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/capacity"
	"github.com/gxfcjkxf/lottery/backend/internal/notification"
)

// The background periods belong to different games but the same 500 wallets
// used by the foreground load. No wallet or prize state is manufactured.
func TestBettingWithSettlementCapacity(t *testing.T) {
	runFinancialCapacity(t, false, false)
}

func TestBettingWithSettlementAndWithdrawalCapacity(t *testing.T) {
	runFinancialCapacity(t, true, false)
}

func TestBettingWithSettlementWithdrawalAndCommissionCapacity(t *testing.T) {
	runFinancialCapacity(t, true, true)
}

func runFinancialCapacity(t *testing.T, withWithdrawals, withCommission bool) {
	capacitySafety(t)
	path := os.Getenv("LOTTERY_CAPACITY_REPORT")
	if path == "" {
		t.Fatal("LOTTERY_CAPACITY_REPORT must name a fresh report")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("capacity report must not already exist")
	}
	f := newCapacityMembers(t, 500, 2, 20, withCommission)
	var withdrawalActors map[string]access.Account
	if withWithdrawals {
		withdrawalActors = prepareCapacityWithdrawals(t, f)
	}
	var commissionBoundary time.Time
	if withCommission {
		commissionBoundary = prepareCapacityCommission(t, f)
	}
	background := prepareCapacitySettlement(t, f)
	if len(background.OrderIDs) != 500 || len(background.Jobs) != 2 {
		t.Fatal("expected 500 real background orders and two settlement jobs")
	}
	ctx := context.Background()
	if withCommission {
		// Foreground bets must belong to the next genuine calendar window.
		// Otherwise their unfinalized orders correctly prevent prior-cycle payout.
		if wait := time.Until(commissionBoundary); wait > 0 {
			time.Sleep(wait)
		}
	}
	before, err := capacity.SnapshotDB(ctx, f.DB)
	if err != nil {
		t.Fatal(err)
	}
	var databaseVersion string
	if err = f.DB.QueryRow(ctx, `SHOW server_version`).Scan(&databaseVersion); err != nil {
		t.Fatal(err)
	}
	known := make(map[string]bool, 15500)
	for _, id := range background.OrderIDs {
		known[id] = true
	}
	var knownMu sync.Mutex
	var settlementErrors, notificationErrors atomic.Int64
	var workers sync.WaitGroup
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	startSettlement := make(chan struct{})
	var startOnce sync.Once
	settlementCompleted := make(chan struct{})
	var completeOnce sync.Once
	var withdrawalReport capacity.Report
	var withdrawalRunErr error
	var commissionRunErr error
	startedAt := time.Now().UTC()
	for i := 0; i < 2; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			select {
			case <-workerCtx.Done():
				return
			case <-startSettlement:
			}
			service := betting.Service{DB: f.DB}
			for workerCtx.Err() == nil {
				n, e := service.ProcessSettlements(workerCtx, 100)
				if e != nil {
					if workerCtx.Err() == nil {
						settlementErrors.Add(1)
					}
					return
				}
				if n == 0 {
					var completed int
					if e = f.DB.QueryRow(workerCtx, `SELECT count(*) FROM settlement_jobs WHERE state='completed'`).Scan(&completed); e != nil {
						if workerCtx.Err() == nil {
							settlementErrors.Add(1)
						}
						return
					}
					if completed == len(background.Jobs) {
						completeOnce.Do(func() { close(settlementCompleted) })
						return
					}
					select {
					case <-workerCtx.Done():
						return
					case <-time.After(10 * time.Millisecond):
					}
				}
			}
		}()
	}
	if withCommission {
		workers.Add(1)
		go func() {
			defer workers.Done()
			select {
			case <-workerCtx.Done():
				return
			case <-settlementCompleted:
				commissionRunErr = processCapacityCommission(workerCtx, f)
			}
		}()
	}
	if withWithdrawals {
		workers.Add(1)
		go func() {
			defer workers.Done()
			select {
			case <-workerCtx.Done():
				return
			case <-settlementCompleted:
			}
			withdrawalReport, withdrawalRunErr = processCapacityWithdrawals(workerCtx, f, withdrawalActors)
		}()
	}
	workers.Add(1)
	go func() {
		defer workers.Done()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-workerCtx.Done():
				return
			case <-ticker.C:
				_, e := (notification.Service{DB: f.DB}).Process(workerCtx, 100)
				if e != nil {
					if workerCtx.Err() == nil {
						notificationErrors.Add(1)
					}
					return
				}
			}
		}
	}()
	plan := capacity.Plan{Rate: 500, Duration: 30 * time.Second, MaxInFlight: 500, RequestTimeout: 10 * time.Second}
	report, runErr := capacity.Run(ctx, plan, func(requestCtx context.Context, index int) capacity.Observation {
		user := f.Users[index%len(f.Users)]
		response, e := f.request(requestCtx, user, "/bet-orders", "mixed-bet-"+user.Member+"-"+strconv.Itoa(index), user.Body)
		if e != nil {
			return capacity.Observation{Status: response.status, Err: "request_failed"}
		}
		var envelope struct {
			Success bool `json:"success"`
			Data    struct {
				ID     string `json:"id"`
				Member string `json:"brand_member_id"`
				Points string `json:"total_points"`
			} `json:"data"`
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal(response.body, &envelope) != nil {
			return capacity.Observation{Status: response.status, Err: "invalid_receipt"}
		}
		if response.status != 201 || !envelope.Success {
			return capacity.Observation{Status: response.status, Code: envelope.Error.Code, Err: "unsuccessful_receipt"}
		}
		if !uuidPattern.MatchString(envelope.Data.ID) || envelope.Data.Member != user.Member || envelope.Data.Points != "1" {
			return capacity.Observation{Status: response.status, Err: "invalid_receipt"}
		}
		knownMu.Lock()
		known[envelope.Data.ID] = true
		knownMu.Unlock()
		startOnce.Do(func() { close(startSettlement) })
		return capacity.Observation{Status: response.status, OrderID: envelope.Data.ID}
	})
	loadEndedAt := time.Now().UTC()
	f.Server.Close()
	cancel()
	workers.Wait()
	integrity, err := readCapacityIntegrity(ctx, f, known)
	if err != nil {
		t.Fatal(err)
	}
	after, err := capacity.SnapshotDB(ctx, f.DB)
	if err != nil {
		t.Fatal(err)
	}
	var completedJobs, paidTargets, prizeEntries, prizePoints, badPrizeLinks, creditsDuringLoad, wonOrders, settledPeriods int64
	if err = f.DB.QueryRow(ctx, `SELECT
	 (SELECT count(*) FROM settlement_jobs WHERE state='completed'),
	 (SELECT count(*) FROM settlement_targets WHERE state='paid'),
	 (SELECT count(*) FROM point_ledger_entries WHERE entry_type='prize'),
	 (SELECT coalesce(sum((delta_snapshot->'winning'->>'available')::bigint),0) FROM point_ledger_entries WHERE entry_type='prize'),
	 (SELECT count(*) FROM bet_orders o LEFT JOIN point_ledger_entries l ON l.id=o.payout_entry_id WHERE o.status='won' AND
	  (l.id IS NULL OR o.settlement_calculation_id IS NULL OR l.entry_type<>'prize' OR l.reference_type<>'settlement_calculation' OR l.reference_id<>o.settlement_calculation_id OR l.brand_id<>o.brand_id OR l.account_id<>o.account_id OR l.member_id<>o.brand_member_id)),
	 (SELECT count(*) FROM point_ledger_entries WHERE entry_type='prize' AND created_at>=$1 AND created_at<=$2),
	 (SELECT count(*) FROM bet_orders WHERE status='won'),
	 (SELECT count(*) FROM periods WHERE status='settled')`, startedAt, loadEndedAt).Scan(&completedJobs, &paidTargets, &prizeEntries, &prizePoints, &badPrizeLinks, &creditsDuringLoad, &wonOrders, &settledPeriods); err != nil {
		t.Fatal(err)
	}
	requestsOK := runErr == nil && report.Planned == 15000 && report.Started == 15000 && report.Completed == 15000 && report.Succeeded == 15000 && report.Dropped == 0 && len(report.ErrorCounts) == 0
	expectedBalance := int64(49989500)
	if withWithdrawals {
		expectedBalance -= 25000
	}
	if withCommission {
		expectedBalance += 50
	}
	financeOK := integrity.Orders == 15500 && integrity.Debits == 15500 && integrity.StakePoints == 15500 && integrity.DebitPoints == 15500 && integrity.KnownUniqueReceipts == 15500 && integrity.UnknownCommittedOrders == 0 && integrity.BalancePoints == expectedBalance && integrity.BadOrderLinks == 0 && integrity.BadAccountBalances == 0 && integrity.LateOrders == 0 && completedJobs == 2 && paidTargets == 500 && prizeEntries == 500 && prizePoints == 5000 && badPrizeLinks == 0 && creditsDuringLoad == 500 && wonOrders == 500 && settledPeriods == 2
	delta := capacity.DBCounterDeltas(before, after)
	workerOK := settlementErrors.Load() == 0 && notificationErrors.Load() == 0 && delta.Deadlocks == 0 && !delta.CounterResetDetected
	artifact := map[string]any{
		"schema_version": 1, "profile": "betting_with_settlement", "scope": "isolated TCP betting + real settlement workers on the same wallets; preauthenticated synthetic sessions; no TLS or production logging",
		"users": 500, "brands": 2, "pool_max_connections": 20, "settlement_workers": 2, "database_version": databaseVersion,
		"plan": plan, "report": report, "scheduled_seconds": 30, "load_started_at_utc": startedAt, "load_ended_at_utc": loadEndedAt,
		"background_orders": 500, "background_completed_jobs": completedJobs, "paid_targets": paidTargets, "prize_entries": prizeEntries, "prize_points": prizePoints, "bad_prize_links": badPrizeLinks, "prize_credits_during_load": creditsDuringLoad,
		"won_orders": wonOrders, "settled_periods": settledPeriods,
		"integrity": integrity, "database_before": before, "database_after": after, "database_delta": delta,
		"settlement_worker_errors": settlementErrors.Load(), "notification_worker_errors": notificationErrors.Load(),
		"request_checks_passed": requestsOK, "financial_checks_passed": financeOK, "worker_checks_passed": workerOK,
		"production_capacity_accepted": false, "replica_and_redis_capacity_verified": false,
	}
	withdrawalsOK := true
	if withWithdrawals {
		var paid, spent, paidDuringLoad, pendingPoints, badLinks, badCycles int64
		if err = f.DB.QueryRow(ctx, `SELECT
		 (SELECT count(*) FROM withdrawal_orders WHERE state='paid'),
		 (SELECT coalesce(sum(points),0) FROM withdrawal_orders WHERE state='paid'),
		 (SELECT count(*) FROM withdrawal_orders WHERE state='paid' AND completed_at>=$1 AND completed_at<=$2),
		 (SELECT coalesce(sum(points),0) FROM point_buckets WHERE state='withdrawal'),
		 (SELECT count(*) FROM withdrawal_orders o LEFT JOIN point_ledger_entries l ON l.id=o.paid_entry_id WHERE o.state<>'paid' OR l.id IS NULL OR l.entry_type<>'withdrawal_paid' OR l.reference_type<>'withdrawal' OR l.reference_id<>o.id OR l.member_id<>o.member_id OR l.brand_id<>o.brand_id OR l.account_id<>o.account_id),
		 (SELECT count(*) FROM withdrawal_orders o LEFT JOIN withdrawal_turnover_cycles c ON c.brand_id=o.brand_id AND c.member_id=o.member_id WHERE c.last_paid_order_id IS DISTINCT FROM o.id OR c.cutoff_version IS DISTINCT FROM o.reserve_version OR c.cutoff_at IS DISTINCT FROM o.created_at)`, startedAt, loadEndedAt).Scan(&paid, &spent, &paidDuringLoad, &pendingPoints, &badLinks, &badCycles); err != nil {
			t.Fatal(err)
		}
		withdrawalsOK = withdrawalRunErr == nil && withdrawalReport.Planned == 500 && withdrawalReport.Started == 500 && withdrawalReport.Completed == 500 && withdrawalReport.Succeeded == 500 && withdrawalReport.Dropped == 0 && len(withdrawalReport.ErrorCounts) == 0 && paid == 500 && spent == 25000 && paidDuringLoad == 500 && pendingPoints == 0 && badLinks == 0 && badCycles == 0
		artifact["profile"] = "betting_with_settlement_withdrawals"
		artifact["withdrawal_report"] = withdrawalReport
		artifact["withdrawal_points"] = spent
		artifact["withdrawal_paid_orders"] = paid
		artifact["withdrawal_paid_during_load"] = paidDuringLoad
		artifact["withdrawal_pending_points"] = pendingPoints
		artifact["withdrawal_bad_links"] = badLinks
		artifact["withdrawal_bad_cycles"] = badCycles
		artifact["withdrawal_checks_passed"] = withdrawalsOK
		artifact["withdrawal_turnover_multiple"] = "0.000001"
		artifact["scope"] = "isolated TCP bets and genuine qualifying withdrawal applications; real settlement and withdrawal actions on the same wallets; no external payments, TLS or production logging"
	}
	commissionOK := true
	if withCommission {
		var cycles, payments, entries, credited, duringLoad, badLinks, targets, outOfWindow int64
		if err = f.DB.QueryRow(ctx, `SELECT
		 (SELECT count(*) FROM commission_cycles WHERE state='ready'),
		 (SELECT count(*) FROM commission_payments WHERE state='paid'),
		 (SELECT count(*) FROM point_ledger_entries WHERE entry_type='commission'),
		 (SELECT coalesce(sum((delta_snapshot->'commission'->>'available')::bigint),0) FROM point_ledger_entries WHERE entry_type='commission'),
		 (SELECT count(*) FROM point_ledger_entries WHERE entry_type='commission' AND created_at>=$1 AND created_at<=$2),
		 (SELECT count(*) FROM commission_payment_targets t LEFT JOIN point_ledger_entries l ON l.id=t.ledger_entry_id WHERE t.state<>'paid' OR l.id IS NULL OR l.entry_type<>'commission' OR l.reference_type<>'commission_payment_target' OR l.reference_id<>t.id OR l.member_id<>t.member_id OR l.brand_id<>t.brand_id OR (l.delta_snapshot->'commission'->>'available')::bigint<>t.points),
		 (SELECT coalesce(sum(target_count),0) FROM commission_cycles),
		 (SELECT count(*) FROM commission_cycle_targets t JOIN commission_cycles c ON c.id=t.cycle_id JOIN bet_orders o ON o.id=t.order_id WHERE o.placed_at<c.window_from OR o.placed_at>=c.window_to)`, startedAt, loadEndedAt).Scan(&cycles, &payments, &entries, &credited, &duringLoad, &badLinks, &targets, &outOfWindow); err != nil {
			t.Fatal(err)
		}
		commissionOK = commissionRunErr == nil && cycles == 2 && payments == 2 && entries == 2 && credited == 50 && duringLoad == 2 && badLinks == 0 && targets == 500 && outOfWindow == 0
		artifact["profile"] = "betting_with_settlement_withdrawals_commission"
		artifact["commission_ready_cycles"] = cycles
		artifact["commission_paid_payments"] = payments
		artifact["commission_entries"] = entries
		artifact["commission_points"] = credited
		artifact["commission_credits_during_load"] = duringLoad
		artifact["commission_bad_links"] = badLinks
		artifact["commission_cycle_targets"] = targets
		artifact["commission_out_of_window_orders"] = outOfWindow
		artifact["commission_checks_passed"] = commissionOK
		artifact["commission_boundary_utc"] = commissionBoundary
		artifact["scope"] = "isolated TCP bets and qualifying withdrawals; genuine settlement, weekly snapshot commission calculation and automatic payout on the same wallets; no external payments, TLS or production logging"
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encodeErr := encoder.Encode(artifact)
	closeErr := file.Close()
	if encodeErr != nil || closeErr != nil {
		t.Fatal(encodeErr, closeErr)
	}
	t.Logf("mixed capacity planned=%d succeeded=%d prize_credits=%d p95_ms=%.2f p99_ms=%.2f", report.Planned, report.Succeeded, creditsDuringLoad, report.EndToEnd.P95Ms, report.EndToEnd.P99Ms)
	if !requestsOK || !financeOK || !workerOK || !withdrawalsOK || !commissionOK {
		t.Fatal("mixed capacity checks failed; see sanitized report")
	}
}
