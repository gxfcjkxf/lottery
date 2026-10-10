//go:build capacity && !windows

package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/capacity"
	"github.com/gxfcjkxf/lottery/backend/internal/notification"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5/pgxpool"
)

type mixedCapacityCase struct {
	model          string
	definition     rules.Definition
	selection      rules.Selection
	multiplier     points.Amount
	combinations   int
	expectedPoints points.Amount
	orders         int
	stakePoints    int64
}

func mixedCapacityCases() []mixedCapacityCase {
	three, two, one := 3, 2, 1
	makeDefinition := func(model rules.Model, selectionRule rules.SelectionRule, condition rules.Condition) rules.Definition {
		return rules.Definition{SchemaVersion: 1, Model: model, Selection: selectionRule, UnitPoints: 1,
			PrizeTiers: []rules.Tier{{Code: "EXACT", Condition: condition, Odds: "10", Exclusive: true}},
			Rounding:   "half_up", RoundingScope: "order", Limits: rules.Limits{MaxCombinations: 100, MaxMultiplier: 1000}}
	}
	return []mixedCapacityCase{
		{model: "DIGITS_0_9", definition: makeDefinition(rules.Model{Type: "DIGITS_0_9", Length: 3, AllowRepeat: true, Ordered: true}, rules.SelectionRule{Mode: "numbers"}, rules.Condition{Op: "equals", Field: "position_match", Value: &three}), selection: rules.Selection{Digits: [][]int{{1, 2, 3}, {1, 2, 3}, {1, 2, 3}}}, multiplier: 3, combinations: 27, expectedPoints: 81},
		{model: "X_PLUS_Y", definition: makeDefinition(rules.Model{Type: "X_PLUS_Y", RegularPool: rules.Pool{Min: 1, Max: 4}, SpecialPool: rules.Pool{Min: 10, Max: 12}, RegularCount: 2, SpecialCount: 1}, rules.SelectionRule{Mode: "numbers", RegularCount: 2, SpecialCount: 1}, rules.Condition{Op: "all", Children: []rules.Condition{{Op: "equals", Field: "regular_match", Value: &two}, {Op: "equals", Field: "special_match", Value: &one}}}), selection: rules.Selection{Regular: []int{1, 2, 3, 4}, Special: []int{10, 11, 12}}, multiplier: 4, combinations: 18, expectedPoints: 72},
		{model: "M_SELECT_N", definition: makeDefinition(rules.Model{Type: "M_SELECT_N", PoolSize: 9, TotalCount: 3, RegularPool: rules.Pool{Min: 1, Max: 9}, SpecialPool: rules.Pool{Min: 1, Max: 9}, RegularCount: 2, SpecialCount: 1}, rules.SelectionRule{Mode: "numbers", RegularCount: 2, SpecialCount: 1}, rules.Condition{Op: "all", Children: []rules.Condition{{Op: "equals", Field: "regular_match", Value: &two}, {Op: "equals", Field: "special_match", Value: &one}}}), selection: rules.Selection{Regular: []int{1, 2, 3, 4}, Special: []int{5, 6, 7}}, multiplier: 5, combinations: 18, expectedPoints: 90},
	}
}

func mixedCapacityValidation(c mixedCapacityCase) rules.ValidationCase {
	stake, prize, won := points.Amount(1), points.Amount(10), true
	selection := c.selection
	var draw rules.Draw
	switch c.model {
	case "DIGITS_0_9":
		selection = rules.Selection{Digits: [][]int{{1}, {2}, {3}}}
		draw = rules.Draw{Digits: []int{1, 2, 3}}
	case "X_PLUS_Y":
		selection = rules.Selection{Regular: []int{1, 2}, Special: []int{10}}
		draw = rules.Draw{Regular: []int{1, 2}, Special: []int{10}}
	case "M_SELECT_N":
		selection = rules.Selection{Regular: []int{1, 2}, Special: []int{5}}
		draw = rules.Draw{Regular: []int{1, 2}, Special: []int{5}}
	}
	return rules.ValidationCase{Name: "exact_regular_and_special", Selection: selection,
		Draw: draw, Multiplier: 1,
		ExpectedBetPoints: &stake, ExpectedPrizePoints: &prize, ExpectedWon: &won}
}

func TestMixedModelsCompoundGoldenRules(t *testing.T) {
	for _, c := range mixedCapacityCases() {
		t.Run(c.model, func(t *testing.T) {
			validation := mixedCapacityValidation(c)
			result, err := rules.Simulate(rules.SimulationInput{Definition: c.definition, Selection: validation.Selection, Draw: validation.Draw, Multiplier: validation.Multiplier})
			if err != nil || result.BetPoints != 1 || result.PrizePoints != 10 || !result.Won {
				t.Fatalf("single winning sample: result=%+v err=%v", result, err)
			}
			compound, err := rules.Simulate(rules.SimulationInput{Definition: c.definition, Selection: c.selection, Draw: validation.Draw, Multiplier: c.multiplier})
			if err != nil || compound.CombinationCount != c.combinations || compound.BetPoints != c.expectedPoints || compound.PrizePoints != 10*c.multiplier || !compound.Won {
				t.Fatalf("compound/multiplier sample: result=%+v err=%v", compound, err)
			}
		})
	}
}

func TestBettingMixedModelsCompoundCapacity(t *testing.T) {
	capacitySafety(t)
	path := os.Getenv("LOTTERY_CAPACITY_REPORT")
	if path == "" {
		t.Fatal("LOTTERY_CAPACITY_REPORT must name a fresh report path")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("capacity report path must not already exist")
	}
	const users, brands, rate, seconds, inflight, pool = 500, 2, 500, 30, 500, 20
	f := newCapacityFixture(t, users, brands, pool)
	ctx := context.Background()
	cases := mixedCapacityCases()
	brandIDs := []string{"0199a000-0000-7000-8000-000000000001", "0199a000-0000-7000-8000-000000000002"}
	inputs := make([][][]byte, len(f.Users))
	for brandIndex, brandID := range brandIDs {
		for modelIndex, c := range cases {
			validation := mixedCapacityValidation(c)
			input := capacityRuleDefinition(t, f.DB, brandID, "mixed_"+stringsToCode(c.model), time.Hour, 2*time.Hour, c.definition, validation)
			input.Selection = c.selection
			input.Multiplier = c.multiplier
			for userIndex := range f.Users {
				if userIndex%brands != brandIndex {
					continue
				}
				quote, err := rules.PrepareBet(ctx, c.definition, input.Selection, input.Multiplier)
				if err != nil || quote.CombinationCount != c.combinations || quote.BetPoints != c.expectedPoints {
					t.Fatalf("%s fixed bet calculation mismatch: quote=%+v err=%v", c.model, quote, err)
				}
				user := f.Users[userIndex]
				previewBody, err := json.Marshal(input)
				if err != nil {
					t.Fatal(err)
				}
				response, err := f.request(ctx, user, "/bet-previews", "", previewBody)
				if err != nil {
					t.Fatal(err)
				}
				var preview struct {
					Success bool `json:"success"`
					Data    struct {
						Actor        string `json:"actor_context"`
						Combinations int    `json:"combination_count"`
						Points       string `json:"bet_points"`
					} `json:"data"`
				}
				if response.status != 200 || json.Unmarshal(response.body, &preview) != nil || !preview.Success || preview.Data.Actor == "" || preview.Data.Combinations != c.combinations || preview.Data.Points != strconv.FormatInt(int64(c.expectedPoints), 10) {
					t.Fatalf("%s real preview failed for member %s: status=%d", c.model, user.Member, response.status)
				}
				input.ActorContext = preview.Data.Actor
				body, err := json.Marshal(input)
				if err != nil {
					t.Fatal(err)
				}
				if inputs[userIndex] == nil {
					inputs[userIndex] = make([][]byte, len(cases))
				}
				inputs[userIndex][modelIndex] = body
			}
		}
	}

	var dbVersion string
	if err := f.DB.QueryRow(ctx, `SHOW server_version`).Scan(&dbVersion); err != nil {
		t.Fatal(err)
	}
	conns := make([]*pgxpool.Conn, 0, pool)
	for i := 0; i < pool; i++ {
		conn, err := f.DB.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, conn)
	}
	for _, conn := range conns {
		conn.Release()
	}
	statsCfg := f.DB.Config()
	statsCfg.MaxConns = 2
	statsPool, err := pgxpool.NewWithConfig(ctx, statsCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(statsPool.Close)
	before, err := capacity.SnapshotDB(ctx, statsPool)
	if err != nil {
		t.Fatal(err)
	}
	cpuBefore, cpuErr := pgCapacityCPU(ctx, statsPool)
	workerEnabled := os.Getenv("LOTTERY_CAPACITY_NOTIFICATIONS") != "off"
	workerCtx, workerCancel := context.WithCancel(ctx)
	var workerWG sync.WaitGroup
	var workerErrors atomic.Int64
	if workerEnabled {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			timer := time.NewTicker(100 * time.Millisecond)
			defer timer.Stop()
			for {
				select {
				case <-workerCtx.Done():
					return
				case <-timer.C:
					jobCtx, cancel := context.WithTimeout(workerCtx, 10*time.Second)
					_, err := (notification.Service{DB: f.DB}).Process(jobCtx, 100)
					cancel()
					if err != nil && workerCtx.Err() == nil {
						workerErrors.Add(1)
					}
				}
			}
		}()
	}
	sampleCtx, sampleCancel := context.WithCancel(ctx)
	var sampleWG sync.WaitGroup
	var samples []capacity.DBSnapshot
	var sampleErrors atomic.Int64
	sampleWG.Add(1)
	go func() {
		defer sampleWG.Done()
		timer := time.NewTicker(time.Second)
		defer timer.Stop()
		for {
			select {
			case <-sampleCtx.Done():
				return
			case <-timer.C:
				snapshotCtx, cancel := context.WithTimeout(sampleCtx, 2*time.Second)
				snapshot, err := capacity.SnapshotDB(snapshotCtx, statsPool)
				cancel()
				if err == nil {
					samples = append(samples, snapshot)
				} else if sampleCtx.Err() == nil {
					sampleErrors.Add(1)
				}
			}
		}
	}()

	type receipt struct {
		member, model string
		points        points.Amount
		combinations  int
		multiplier    points.Amount
	}
	knownMu := sync.Mutex{}
	known := map[string]bool{}
	receipts := map[string]receipt{}
	prefix := fmt.Sprintf("mixed-capacity-%d-", time.Now().UnixNano())
	plan := capacity.Plan{Rate: rate, Duration: time.Duration(seconds) * time.Second, MaxInFlight: inflight, RequestTimeout: 10 * time.Second}
	wallStart := time.Now()
	goCPUStart := processCPU()
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)
	report, runErr := capacity.Run(ctx, plan, func(requestCtx context.Context, index int) capacity.Observation {
		user := f.Users[index%len(f.Users)]
		modelIndex := index % len(cases)
		c := cases[modelIndex]
		body := inputs[index%len(f.Users)][modelIndex]
		response, err := f.request(requestCtx, user, "/bet-orders", prefix+strconv.Itoa(index), body)
		if err != nil {
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
			return capacity.Observation{Status: response.status, Err: "invalid_json"}
		}
		if response.status != 201 || !envelope.Success {
			return capacity.Observation{Status: response.status, Code: envelope.Error.Code, Err: "unsuccessful_receipt"}
		}
		if !uuidPattern.MatchString(envelope.Data.ID) || envelope.Data.Member != user.Member || envelope.Data.Points != strconv.FormatInt(int64(c.expectedPoints), 10) {
			return capacity.Observation{Status: response.status, Err: "invalid_receipt"}
		}
		knownMu.Lock()
		known[envelope.Data.ID] = true
		receipts[envelope.Data.ID] = receipt{member: user.Member, model: c.model, points: c.expectedPoints, combinations: c.combinations, multiplier: c.multiplier}
		knownMu.Unlock()
		return capacity.Observation{Status: response.status, OrderID: envelope.Data.ID}
	})
	drainStart := time.Now()
	f.Server.Close()
	serverDrainSeconds := time.Since(drainStart).Seconds()
	elapsed := time.Since(wallStart)
	goCPUSeconds := processCPU() - goCPUStart
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)
	sampleCancel()
	sampleWG.Wait()
	workerCancel()
	workerWG.Wait()
	after, snapshotErr := capacity.SnapshotDB(ctx, statsPool)
	if snapshotErr != nil {
		t.Fatal(snapshotErr)
	}
	cpuAfter, afterCPUErr := pgCapacityCPU(ctx, statsPool)
	var pgCPUSeconds *float64
	sharedPIDs := 0
	if cpuErr == nil && afterCPUErr == nil {
		total := 0.0
		for pid, end := range cpuAfter {
			if start, ok := cpuBefore[pid]; ok && end >= start {
				total += end - start
				sharedPIDs++
			}
		}
		pgCPUSeconds = &total
	}
	integrity, integrityErr := readCapacityIntegrity(ctx, f, known)
	for id, rec := range receipts {
		var member, model, total string
		var combinations int
		var multiplier int64
		err := f.DB.QueryRow(ctx, `SELECT brand_member_id::text,definition_snapshot->'model'->>'model',total_points::text,combination_count,multiplier FROM bet_orders WHERE id=$1`, id).Scan(&member, &model, &total, &combinations, &multiplier)
		if err != nil || member != rec.member || model != rec.model || total != strconv.FormatInt(int64(rec.points), 10) || combinations != rec.combinations || multiplier != int64(rec.multiplier) {
			integrityErr = fmt.Errorf("acknowledged receipt does not match its committed order")
			break
		}
	}
	if len(receipts) != len(known) {
		integrityErr = fmt.Errorf("receipt tracking mismatch")
	}
	for i := range cases {
		var count, stake int64
		if err := f.DB.QueryRow(ctx, `SELECT count(*),coalesce(sum(total_points),0) FROM bet_orders WHERE definition_snapshot->'model'->>'model'=$1`, cases[i].model).Scan(&count, &stake); err != nil {
			integrityErr = err
			continue
		}
		cases[i].orders, cases[i].stakePoints = int(count), stake
	}
	if integrity.Orders != 15000 || integrity.Debits != 15000 || integrity.StakePoints != 1215000 || integrity.DebitPoints != 1215000 || integrity.BalancePoints != 48785000 || integrity.BadOrderLinks != 0 || integrity.BadAccountBalances != 0 || integrity.LateOrders != 0 || integrity.UnknownCommittedOrders != 0 {
		integrityErr = fmt.Errorf("mixed-model financial invariants failed")
	}
	for _, c := range cases {
		if c.orders != 5000 || c.stakePoints != int64(c.expectedPoints)*5000 {
			integrityErr = fmt.Errorf("%s database stake invariant failed", c.model)
		}
	}
	failures := report.Planned - report.Succeeded
	requestChecks := runErr == nil && failures == 0 && report.Succeeded == 15000 && integrity.KnownUniqueReceipts == 15000
	financialChecks := integrityErr == nil
	workerChecks := workerEnabled && workerErrors.Load() == 0
	databaseDelta := capacity.DBCounterDeltas(before, after)
	statusPassed := requestChecks && financialChecks && workerChecks && sampleErrors.Load() == 0 && databaseDelta.Deadlocks == 0 && !databaseDelta.CounterResetDetected
	caseReports := make([]map[string]any, 0, len(cases))
	for _, c := range cases {
		caseReports = append(caseReports, map[string]any{"model": c.model, "combinations": c.combinations, "multiplier": int64(c.multiplier), "expected_points": int64(c.expectedPoints), "orders": c.orders, "stake_points": c.stakePoints})
	}
	artifact := map[string]any{
		"schema_version": 1, "profile": "betting_mixed_models_compound",
		"status":     map[bool]string{true: "passed", false: "failed"}[statusPassed],
		"scope":      "isolated TCP HTTP API + real PostgreSQL transactions; preauthenticated synthetic sessions; no TLS/proxy or production logging",
		"go_version": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH, "logical_cpus": runtime.NumCPU(),
		"users": users, "brands": brands, "pool_max_connections": pool, "notification_worker_enabled": workerEnabled,
		"notification_worker_errors": workerErrors.Load(), "sample_errors": sampleErrors.Load(), "database_version": dbVersion,
		"plan": plan, "scheduled_seconds": seconds, "report": report, "failed_or_dropped": failures,
		"failure_rate_including_dropped": float64(failures) / float64(report.Planned), "server_drain_seconds": serverDrainSeconds,
		"new_order_tps_over_wall": float64(integrity.Orders) / elapsed.Seconds(), "wall_including_server_drain_seconds": elapsed.Seconds(),
		"acknowledgments_within_scheduled_window_per_second": float64(report.SuccessesDuringScheduledInterval) / plan.Duration.Seconds(),
		"integrity": integrity, "cases": caseReports, "database_before": before, "database_after": after,
		"database_samples": samples, "database_delta": databaseDelta,
		"go_api_and_driver_cpu_seconds": goCPUSeconds, "postgres_visible_shared_backend_cpu_seconds": pgCPUSeconds,
		"postgres_cpu_shared_pid_count": sharedPIDs, "postgres_cpu_before_pid_count": len(cpuBefore), "postgres_cpu_after_pid_count": len(cpuAfter),
		"go_heap_alloc_before": memBefore.HeapAlloc, "go_heap_alloc_after": memAfter.HeapAlloc,
		"go_total_alloc_delta": memAfter.TotalAlloc - memBefore.TotalAlloc, "go_gc_count_delta": memAfter.NumGC - memBefore.NumGC,
		"production_capacity_accepted": false, "replica_and_redis_capacity_verified": false,
		"request_checks_passed": requestChecks, "financial_checks_passed": financialChecks, "worker_checks_passed": workerChecks,
		"error_counts": report.ErrorCounts,
	}
	if runErr != nil {
		artifact["run_error"] = "runner_failed"
	}
	if integrityErr != nil {
		artifact["integrity_error"] = "financial_invariant_failed"
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(artifact)
	closeErr := file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	t.Logf("mixed capacity report=%s planned=%d started=%d succeeded=%d dropped=%d orders=%d", path, report.Planned, report.Started, report.Succeeded, report.Dropped, integrity.Orders)
	if runErr != nil {
		t.Fatal(runErr)
	}
	if integrityErr != nil {
		t.Fatal(integrityErr)
	}
	if !statusPassed {
		t.Fatal("mixed-model capacity checks failed")
	}
}

func stringsToCode(model string) string {
	switch model {
	case "DIGITS_0_9":
		return "digits"
	case "X_PLUS_Y":
		return "x_plus_y"
	case "M_SELECT_N":
		return "m_select_n"
	default:
		panic("unexpected capacity model")
	}
}
