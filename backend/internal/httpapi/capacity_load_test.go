//go:build capacity && !windows

package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/capacity"
	"github.com/gxfcjkxf/lottery/backend/internal/notification"
	"github.com/jackc/pgx/v5/pgxpool"
)

func capacityInt(t *testing.T, name string, fallback, min, max int) int {
	t.Helper()
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	n, e := strconv.Atoi(raw)
	if e != nil || n < min || n > max {
		t.Fatalf("%s must be between %d and %d", name, min, max)
	}
	return n
}

type capacityIntegrity struct {
	Orders                 int64 `json:"orders"`
	Debits                 int64 `json:"debits"`
	StakePoints            int64 `json:"stake_points"`
	DebitPoints            int64 `json:"debit_points"`
	BalancePoints          int64 `json:"balance_points"`
	BadOrderLinks          int64 `json:"bad_order_links"`
	BadAccountBalances     int64 `json:"bad_account_balances"`
	LateOrders             int64 `json:"late_orders"`
	KnownUniqueReceipts    int   `json:"known_unique_receipts"`
	UnknownCommittedOrders int64 `json:"unknown_committed_orders"`
}

func checkCapacityIntegrity(ctx context.Context, f capacityFixture, known map[string]bool) (capacityIntegrity, error) {
	var out capacityIntegrity
	e := f.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM bet_orders),(SELECT count(*) FROM point_ledger_entries WHERE entry_type='bet'),
 coalesce((SELECT sum(total_points) FROM bet_orders),0),
 coalesce((SELECT -sum((delta_snapshot->'recharge'->>'available')::bigint+(delta_snapshot->'winning'->>'available')::bigint+(delta_snapshot->'gift'->>'available')::bigint) FROM point_ledger_entries WHERE entry_type='bet'),0),
 (SELECT sum(points) FROM point_buckets),
 (SELECT count(*) FROM bet_orders o LEFT JOIN point_ledger_entries l ON l.id=o.debit_entry_id WHERE l.id IS NULL OR l.brand_id<>o.brand_id OR l.account_id<>o.account_id OR l.member_id<>o.brand_member_id OR l.reference_type<>'bet_order' OR l.reference_id<>o.id OR l.entry_type<>'bet'),
 (SELECT count(*) FROM point_accounts a WHERE (SELECT coalesce(sum(points),0) FROM point_buckets b WHERE b.account_id=a.id)<>(SELECT coalesce(sum((l.delta_snapshot->'recharge'->>'available')::bigint+(l.delta_snapshot->'winning'->>'available')::bigint+(l.delta_snapshot->'gift'->>'available')::bigint),0) FROM point_ledger_entries l WHERE l.account_id=a.id))`).Scan(&out.Orders, &out.Debits, &out.StakePoints, &out.DebitPoints, &out.BalancePoints, &out.BadOrderLinks, &out.BadAccountBalances)
	if e != nil {
		return out, e
	}
	if e = f.DB.QueryRow(ctx, `SELECT count(*) FROM bet_orders o JOIN periods p ON p.id=o.period_id WHERE o.placed_at<p.bet_start_at OR o.placed_at>=p.bet_end_at`).Scan(&out.LateOrders); e != nil {
		return out, e
	}
	ids := make([]string, 0, len(known))
	for id := range known {
		ids = append(ids, id)
	}
	var found int
	if len(ids) > 0 {
		if e = f.DB.QueryRow(ctx, `SELECT count(*) FROM bet_orders WHERE id=ANY($1::uuid[])`, ids).Scan(&found); e != nil {
			return out, e
		}
	}
	out.KnownUniqueReceipts = found
	out.UnknownCommittedOrders = out.Orders - int64(found)
	if found != len(known) || out.Orders != out.Debits || out.StakePoints != out.DebitPoints || out.StakePoints != out.Orders || out.BalancePoints != int64(len(f.Users))*int64(capacityFunding)-out.StakePoints || out.BadOrderLinks != 0 || out.BadAccountBalances != 0 || out.LateOrders != 0 {
		return out, fmt.Errorf("capacity financial invariants failed")
	}
	return out, nil
}

type capacityCPUSnapshot map[int]float64

func pgCapacityCPU(ctx context.Context, p *pgxpool.Pool) (capacityCPUSnapshot, error) {
	rows, e := p.Query(ctx, `SELECT pid FROM pg_stat_activity WHERE datname=current_database()`)
	if e != nil {
		return nil, e
	}
	var ids []string
	for rows.Next() {
		var pid int
		if e = rows.Scan(&pid); e != nil {
			rows.Close()
			return nil, e
		}
		ids = append(ids, strconv.Itoa(pid))
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no visible PostgreSQL client backends")
	}
	data, e := exec.CommandContext(ctx, "ps", "-p", strings.Join(ids, ","), "-o", "pid=", "-o", "time=").Output()
	if e != nil {
		return nil, e
	}
	out := capacityCPUSnapshot{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		pid, e := strconv.Atoi(fields[0])
		if e != nil {
			continue
		}
		parts := strings.Split(fields[1], ":")
		total := 0.0
		valid := true
		for _, part := range parts {
			n, e := strconv.ParseFloat(part, 64)
			if e != nil {
				valid = false
				break
			}
			total = total*60 + n
		}
		if valid {
			out[pid] = total
		}
	}
	return out, nil
}
func processCPU() float64 {
	var u syscall.Rusage
	if syscall.Getrusage(syscall.RUSAGE_SELF, &u) != nil {
		return 0
	}
	return float64(u.Utime.Sec+u.Stime.Sec) + float64(u.Utime.Usec+u.Stime.Usec)/1e6
}

func TestBettingCapacity(t *testing.T) {
	capacitySafety(t)
	path := os.Getenv("LOTTERY_CAPACITY_REPORT")
	if path == "" {
		t.Fatal("LOTTERY_CAPACITY_REPORT must name a fresh report path")
	}
	if _, e := os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("capacity report path must not already exist")
	}
	users := capacityInt(t, "LOTTERY_CAPACITY_USERS", 500, 1, 5000)
	brands := capacityInt(t, "LOTTERY_CAPACITY_BRANDS", 1, 1, 2)
	rate := capacityInt(t, "LOTTERY_CAPACITY_RATE", 500, 1, 10000)
	seconds := capacityInt(t, "LOTTERY_CAPACITY_SECONDS", 30, 1, 300)
	inflight := capacityInt(t, "LOTTERY_CAPACITY_INFLIGHT", 500, 1, 10000)
	pool := capacityInt(t, "LOTTERY_CAPACITY_POOL", 20, 4, 100)
	duplicateGroup := capacityInt(t, "LOTTERY_CAPACITY_DUPLICATE_GROUP", 1, 1, 100)
	cutoffSeconds := capacityInt(t, "LOTTERY_CAPACITY_CUTOFF_SECONDS", 0, 0, 300)
	if duplicateGroup > 1 && users != 1 {
		t.Fatal("duplicate profile requires one user so original actor/key/body are identical")
	}
	if rate*seconds > 1000000 {
		t.Fatal("capacity planned requests exceed bounded report limit")
	}
	f := newCapacityFixture(t, users, brands, int32(pool))
	ctx := context.Background()
	var dbVersion string
	if e := f.DB.QueryRow(ctx, `SHOW server_version`).Scan(&dbVersion); e != nil {
		t.Fatal(e)
	}
	var cutoffAt time.Time
	if e := f.DB.QueryRow(ctx, `SELECT min(bet_end_at) FROM periods`).Scan(&cutoffAt); e != nil {
		t.Fatal(e)
	}
	// Warm all configured API pool connections before sampling CPU. This does
	// not change production pool defaults or lower authentication requirements.
	conns := make([]*pgxpool.Conn, 0, pool)
	for i := 0; i < pool; i++ {
		c, e := f.DB.Acquire(ctx)
		if e != nil {
			t.Fatal(e)
		}
		conns = append(conns, c)
	}
	for _, c := range conns {
		c.Release()
	}
	statsCfg := f.DB.Config()
	statsCfg.MaxConns = 2
	statsPool, e := pgxpool.NewWithConfig(ctx, statsCfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(statsPool.Close)
	before, e := capacity.SnapshotDB(ctx, statsPool)
	if e != nil {
		t.Fatal(e)
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
					_, e := (notification.Service{DB: f.DB}).Process(jobCtx, 100)
					cancel()
					if e != nil && workerCtx.Err() == nil {
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
				c, cancel := context.WithTimeout(sampleCtx, 2*time.Second)
				s, e := capacity.SnapshotDB(c, statsPool)
				cancel()
				if e == nil {
					samples = append(samples, s)
				} else if sampleCtx.Err() == nil {
					sampleErrors.Add(1)
				}
			}
		}
	}()
	var idsMu sync.Mutex
	known := map[string]bool{}
	prefix := fmt.Sprintf("capacity-%d-", time.Now().UnixNano())
	goCPUStart := processCPU()
	wallStart := time.Now()
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)
	plan := capacity.Plan{Rate: rate, Duration: time.Duration(seconds) * time.Second, MaxInFlight: inflight, RequestTimeout: 10 * time.Second}
	report, runErr := capacity.Run(ctx, plan, func(requestCtx context.Context, index int) capacity.Observation {
		c := f.Users[index%len(f.Users)]
		key := prefix + strconv.Itoa(index/duplicateGroup)
		r, e := f.request(requestCtx, c, "/bet-orders", key, c.Body)
		if e != nil {
			return capacity.Observation{Status: r.status, Err: "request_failed"}
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
		if json.Unmarshal(r.body, &envelope) != nil {
			return capacity.Observation{Status: r.status, Err: "invalid_json"}
		}
		if r.status != 201 || !envelope.Success {
			return capacity.Observation{Status: r.status, Code: envelope.Error.Code, Err: "unsuccessful_receipt"}
		}
		if !uuidPattern.MatchString(envelope.Data.ID) || envelope.Data.Member != c.Member || envelope.Data.Points != "1" {
			return capacity.Observation{Status: r.status, Err: "invalid_receipt"}
		}
		idsMu.Lock()
		known[envelope.Data.ID] = true
		idsMu.Unlock()
		return capacity.Observation{Status: r.status, OrderID: envelope.Data.ID}
	})
	// A client deadline does not prove the server stopped its transaction.
	// Drain real TCP handlers before reading the final committed ledger.
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
	after, e := capacity.SnapshotDB(ctx, statsPool)
	if e != nil {
		t.Fatal(e)
	}
	cpuAfter, afterCPUError := pgCapacityCPU(ctx, statsPool)
	var pgCPUSeconds *float64
	sharedPIDs := 0
	if cpuErr == nil && afterCPUError == nil {
		total := 0.0
		for pid, end := range cpuAfter {
			if start, ok := cpuBefore[pid]; ok && end >= start {
				total += end - start
				sharedPIDs++
			}
		}
		pgCPUSeconds = &total
	}
	integrity, integrityErr := checkCapacityIntegrity(ctx, f, known)
	if duplicateGroup > 1 {
		maxOrders := int64((report.Planned + duplicateGroup - 1) / duplicateGroup)
		if integrity.Orders > maxOrders || report.Succeeded == report.Planned && integrity.Orders != maxOrders {
			integrityErr = fmt.Errorf("duplicate requests created an unexpected number of orders")
		}
	}
	if cutoffSeconds > 0 && report.CodeCounts["BET_PERIOD_CLOSED"] == 0 {
		integrityErr = fmt.Errorf("cutoff profile did not exercise a closed-period rejection")
	}
	failures := report.Planned - report.Succeeded
	artifact := map[string]any{
		"schema_version": 1, "scope": "isolated TCP HTTP API + real PostgreSQL transactions; preauthenticated synthetic sessions; no TLS/proxy or production logging",
		"go_version": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH, "logical_cpus": runtime.NumCPU(),
		"users": users, "brands": brands, "pool_max_connections": pool, "duplicate_group": duplicateGroup,
		"notification_worker_enabled": workerEnabled, "notification_worker_errors": workerErrors.Load(), "sample_errors": sampleErrors.Load(),
		"database_version": dbVersion, "cutoff_seconds_from_fixture_creation": cutoffSeconds, "earliest_period_cutoff_utc": cutoffAt.UTC(), "load_started_at_utc": wallStart.UTC(),
		"plan": plan, "scheduled_seconds": seconds, "report": report, "server_drain_seconds": serverDrainSeconds,
		"failed_or_dropped": failures, "failure_rate_including_dropped": float64(failures) / float64(report.Planned),
		"new_order_tps_over_wall": float64(integrity.Orders) / elapsed.Seconds(), "wall_including_server_drain_seconds": elapsed.Seconds(),
		"acknowledgments_within_scheduled_window_per_second": float64(report.SuccessesDuringScheduledInterval) / plan.Duration.Seconds(),
		"integrity": integrity, "database_before": before, "database_after": after, "database_samples": samples, "database_delta": capacity.DBCounterDeltas(before, after),
		"go_api_and_driver_cpu_seconds": goCPUSeconds, "go_api_and_driver_cpu_percent_one_core": goCPUSeconds / elapsed.Seconds() * 100,
		"postgres_visible_shared_backend_cpu_seconds": pgCPUSeconds, "postgres_cpu_shared_pid_count": sharedPIDs, "postgres_cpu_before_pid_count": len(cpuBefore), "postgres_cpu_after_pid_count": len(cpuAfter),
		"go_heap_alloc_before": memBefore.HeapAlloc, "go_heap_alloc_after": memAfter.HeapAlloc, "go_total_alloc_delta": memAfter.TotalAlloc - memBefore.TotalAlloc, "go_gc_count_delta": memAfter.NumGC - memBefore.NumGC,
		"replica_and_redis_capacity_verified": false, "production_capacity_accepted": false,
	}
	if runErr != nil {
		artifact["run_error"] = "runner_failed"
	}
	if integrityErr != nil {
		artifact["integrity_error"] = "financial_invariant_failed"
	}
	file, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		t.Fatal(e)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	e = encoder.Encode(artifact)
	closeErr := file.Close()
	if e != nil {
		t.Fatal(e)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	t.Logf("capacity report=%s planned=%d started=%d succeeded=%d dropped=%d new_orders=%d new_order_tps=%.2f p95_ms=%.2f p99_ms=%.2f", path, report.Planned, report.Started, report.Succeeded, report.Dropped, integrity.Orders, float64(integrity.Orders)/elapsed.Seconds(), report.EndToEnd.P95Ms, report.EndToEnd.P99Ms)
	if runErr != nil {
		t.Fatal(runErr)
	}
	if integrityErr != nil {
		t.Fatal(integrityErr)
	}
	// Below-target throughput is a measured capacity result, not a passing SLO.
	// No P95/P99 or sustained-window acceptance target has yet been selected.
}
