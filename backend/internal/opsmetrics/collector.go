// Package opsmetrics provides a read-only, cached Prometheus view of bounded
// operational state. Refresh is the only code in this package that talks to
// PostgreSQL; Collect only emits the last complete snapshot.
// Database-wide metrics appear in both API and worker processes when both
// expose this collector; dashboards should aggregate with max by database and
// kind to avoid counting the same database state twice.
package opsmetrics

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	refreshInterval = 30 * time.Second
	refreshTimeout  = 2 * time.Second
	staleAfter      = 75 * time.Second
	maxReplicas     = 8
)

var workKinds = []string{
	"notification", "settlement", "period_refund", "draw_correction",
	"commission_cycle", "commission_payment", "commission_correction",
	"report_archive", "withdrawal",
}

// Source semantics: notification uses due notification_deliveries; settlement
// uses settlement_jobs and failed settlement_targets; period_refund uses
// processing period_cancellations with pending/failed targets;
// draw_correction uses reversing work and resettling work whose linked
// settlement job is itself worker-actionable; commission_cycle uses
// due worker states of commission_cycles; commission_payment uses pending targets only
// while paying; commission_correction uses pending targets only while applying
// (awaiting approval and paused insufficient-funds executions are excluded);
// report_archive uses pending/failed automatic tasks; withdrawal processing
// is a manually operated queue (there is no auto-payment worker), reviewing
// orders await an operator, and failed is a terminal business disposition.

type sourceValue struct {
	value float64
	valid bool
}

type snapshot struct {
	completedAt             time.Time
	duration                time.Duration
	workPending             map[string]float64
	workOldest              map[string]float64
	workFailed              map[string]float64
	discrepancy             sourceValue
	reconciliationObserved  float64
	replicationObserved     float64
	replicationStatsVisible float64
	replicationLag          sourceValue
	pools                   []poolSnapshot
}

type poolSnapshot struct {
	node                       string
	acquired, idle, total, max float64
}

// Collector implements prometheus.Collector. Pools may contain at most eight
// replicas; extra entries are ignored so the exported node label stays bounded.
type Collector struct {
	primary      *pgxpool.Pool
	replicas     []*pgxpool.Pool
	startOnce    sync.Once
	refreshMu    sync.Mutex
	mu           sync.RWMutex
	last         snapshot
	hasSnapshot  bool
	lastAttempt  time.Time
	lastDuration time.Duration
	lastSuccess  bool
	databaseUp   bool

	databaseUpDesc, snapshotSuccessDesc, snapshotTimestampDesc, snapshotDurationDesc *prometheus.Desc
	workPendingDesc, workOldestDesc, workFailedDesc, reconciliationDesc              *prometheus.Desc
	reconciliationObservedDesc                                                       *prometheus.Desc
	replicationObservedDesc, replicationLagDesc                                      *prometheus.Desc
	replicationStatsVisibleDesc                                                      *prometheus.Desc
	poolAcquiredDesc, poolIdleDesc, poolTotalDesc, poolMaxDesc                       *prometheus.Desc
}

// New constructs a collector. Refresh is intentionally explicit so the
// process owner can control startup and shutdown with Start(ctx).
func New(primary *pgxpool.Pool, replicas []*pgxpool.Pool) *Collector {
	if len(replicas) > maxReplicas {
		replicas = replicas[:maxReplicas]
	}
	c := &Collector{primary: primary, replicas: append([]*pgxpool.Pool(nil), replicas...)}
	c.databaseUpDesc = prometheus.NewDesc("lottery_database_up", "Whether the primary database ping succeeded.", nil, nil)
	c.snapshotSuccessDesc = prometheus.NewDesc("lottery_ops_snapshot_success", "Whether the latest operational snapshot completed successfully.", nil, nil)
	c.snapshotTimestampDesc = prometheus.NewDesc("lottery_ops_snapshot_timestamp_seconds", "Unix timestamp when the latest operational snapshot completed.", nil, nil)
	c.snapshotDurationDesc = prometheus.NewDesc("lottery_ops_snapshot_duration_seconds", "Duration of the latest operational snapshot attempt.", nil, nil)
	c.workPendingDesc = prometheus.NewDesc("lottery_work_pending", "Pending operational or background work.", []string{"kind"}, nil)
	c.workOldestDesc = prometheus.NewDesc("lottery_work_oldest_pending_age_seconds", "Age of the oldest actionable background work item.", []string{"kind"}, nil)
	c.workFailedDesc = prometheus.NewDesc("lottery_work_failed", "Background work items in explicit technical failure states.", []string{"kind"}, nil)
	c.reconciliationDesc = prometheus.NewDesc("lottery_reconciliation_discrepancies", "Recorded issue count from each brand's latest completed business reconciliation.", nil, nil)
	c.reconciliationObservedDesc = prometheus.NewDesc("lottery_reconciliation_observed", "Whether any completed business reconciliation has been recorded.", nil, nil)
	c.replicationObservedDesc = prometheus.NewDesc("lottery_replication_observed", "Whether live streaming replication lag was observable on the primary.", nil, nil)
	c.replicationStatsVisibleDesc = prometheus.NewDesc("lottery_replication_stats_visible", "Whether the database role can see complete replication statistics; zero means unknown, not healthy.", nil, nil)
	c.replicationLagDesc = prometheus.NewDesc("lottery_replication_lag_bytes", "Maximum current WAL byte distance among live streaming replicas.", nil, nil)
	c.poolAcquiredDesc = prometheus.NewDesc("lottery_db_pool_acquired_connections", "Connections currently acquired from a database pool.", []string{"node"}, nil)
	c.poolIdleDesc = prometheus.NewDesc("lottery_db_pool_idle_connections", "Idle connections currently available in a database pool.", []string{"node"}, nil)
	c.poolTotalDesc = prometheus.NewDesc("lottery_db_pool_total_connections", "Connections currently established in a database pool.", []string{"node"}, nil)
	c.poolMaxDesc = prometheus.NewDesc("lottery_db_pool_max_connections", "Configured maximum connections in a database pool.", []string{"node"}, nil)
	return c
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{c.databaseUpDesc, c.snapshotSuccessDesc, c.snapshotTimestampDesc, c.snapshotDurationDesc,
		c.workPendingDesc, c.workOldestDesc, c.workFailedDesc, c.reconciliationDesc, c.reconciliationObservedDesc,
		c.replicationObservedDesc, c.replicationStatsVisibleDesc, c.replicationLagDesc, c.poolAcquiredDesc, c.poolIdleDesc, c.poolTotalDesc, c.poolMaxDesc} {
		ch <- d
	}
}

// Start performs an immediate refresh and then refreshes every thirty seconds.
// The single loop prevents overlap, and cancellation interrupts an active read.
func (c *Collector) Start(ctx context.Context) {
	if ctx == nil {
		return
	}
	c.startOnce.Do(func() {
		go func() {
			c.Refresh(ctx)
			ticker := time.NewTicker(refreshInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					c.Refresh(ctx)
				}
			}
		}()
	})
}

// Refresh collects one consistent, read-only primary snapshot with a hard
// two-second bound. Source errors are retained internally as an unsuccessful
// attempt; no database error text is emitted or exposed as a metric label.
func (c *Collector) Refresh(ctx context.Context) {
	started := time.Now()
	if ctx == nil {
		ctx = context.Background()
	}
	refreshCtx, cancel := context.WithTimeout(ctx, refreshTimeout)
	defer cancel()
	if !c.refreshMu.TryLock() {
		c.mu.Lock()
		c.lastAttempt = time.Now()
		c.lastDuration = time.Since(started)
		c.lastSuccess = false
		c.mu.Unlock()
		return
	}
	defer c.refreshMu.Unlock()
	up := false
	if c.primary != nil && refreshCtx.Err() == nil {
		up = c.primary.Ping(refreshCtx) == nil
	}

	var next snapshot
	var err error
	if up && refreshCtx.Err() == nil {
		next, err = readSnapshot(refreshCtx, c.primary)
	}
	if err == nil && up && refreshCtx.Err() == nil {
		next.pools = poolSnapshots(c.primary, c.replicas)
	}
	duration := time.Since(started)
	completed := time.Now()

	c.mu.Lock()
	c.databaseUp = up
	c.lastAttempt = completed
	c.lastDuration = duration
	c.lastSuccess = err == nil && up && refreshCtx.Err() == nil
	if c.lastSuccess {
		next.completedAt = completed
		next.duration = duration
		c.last = next
		c.hasSnapshot = true
	}
	c.mu.Unlock()
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	now := time.Now()
	last, has := c.last, c.hasSnapshot
	up, success, attemptAt, duration := c.databaseUp, c.lastSuccess, c.lastAttempt, c.lastDuration
	c.mu.RUnlock()

	ch <- prometheus.MustNewConstMetric(c.databaseUpDesc, prometheus.GaugeValue, boolFloat(up))
	ch <- prometheus.MustNewConstMetric(c.snapshotSuccessDesc, prometheus.GaugeValue, boolFloat(success))
	if !attemptAt.IsZero() {
		ch <- prometheus.MustNewConstMetric(c.snapshotTimestampDesc, prometheus.GaugeValue, float64(attemptAt.UnixNano())/1e9)
		ch <- prometheus.MustNewConstMetric(c.snapshotDurationDesc, prometheus.GaugeValue, duration.Seconds())
	}
	if !success || !has || now.Sub(last.completedAt) > staleAfter {
		return
	}
	for _, kind := range workKinds {
		if v, ok := last.workPending[kind]; ok {
			ch <- prometheus.MustNewConstMetric(c.workPendingDesc, prometheus.GaugeValue, v, kind)
		}
		if v, ok := last.workOldest[kind]; ok {
			ch <- prometheus.MustNewConstMetric(c.workOldestDesc, prometheus.GaugeValue, v, kind)
		}
		if v, ok := last.workFailed[kind]; ok {
			ch <- prometheus.MustNewConstMetric(c.workFailedDesc, prometheus.GaugeValue, v, kind)
		}
	}
	if last.discrepancy.valid {
		ch <- prometheus.MustNewConstMetric(c.reconciliationDesc, prometheus.GaugeValue, last.discrepancy.value)
	}
	ch <- prometheus.MustNewConstMetric(c.reconciliationObservedDesc, prometheus.GaugeValue, last.reconciliationObserved)
	ch <- prometheus.MustNewConstMetric(c.replicationObservedDesc, prometheus.GaugeValue, last.replicationObserved)
	ch <- prometheus.MustNewConstMetric(c.replicationStatsVisibleDesc, prometheus.GaugeValue, last.replicationStatsVisible)
	if last.replicationLag.valid {
		ch <- prometheus.MustNewConstMetric(c.replicationLagDesc, prometheus.GaugeValue, last.replicationLag.value)
	}
	for _, p := range last.pools {
		ch <- prometheus.MustNewConstMetric(c.poolAcquiredDesc, prometheus.GaugeValue, p.acquired, p.node)
		ch <- prometheus.MustNewConstMetric(c.poolIdleDesc, prometheus.GaugeValue, p.idle, p.node)
		ch <- prometheus.MustNewConstMetric(c.poolTotalDesc, prometheus.GaugeValue, p.total, p.node)
		ch <- prometheus.MustNewConstMetric(c.poolMaxDesc, prometheus.GaugeValue, p.max, p.node)
	}
}

func boolFloat(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

func poolSnapshots(primary *pgxpool.Pool, replicas []*pgxpool.Pool) []poolSnapshot {
	out := make([]poolSnapshot, 0, 1+len(replicas))
	add := func(node string, p *pgxpool.Pool) {
		if p == nil {
			return
		}
		s := p.Stat()
		out = append(out, poolSnapshot{node: node, acquired: float64(s.AcquiredConns()), idle: float64(s.IdleConns()), total: float64(s.TotalConns()), max: float64(s.MaxConns())})
	}
	add("primary", primary)
	for i, p := range replicas {
		add(fmt.Sprintf("replica_%d", i+1), p)
	}
	return out
}

var _ prometheus.Collector = (*Collector)(nil)

var errNoApplicationSchema = errors.New("application schema unavailable")

func readSnapshot(ctx context.Context, pool *pgxpool.Pool) (snapshot, error) {
	out := snapshot{workPending: make(map[string]float64), workOldest: make(map[string]float64), workFailed: make(map[string]float64)}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SET LOCAL statement_timeout = '1800ms'`); err != nil {
		return out, err
	}
	var schema string
	var isPrimary bool
	err = tx.QueryRow(ctx, `SELECT n.nspname, NOT pg_catalog.pg_is_in_recovery()
 FROM pg_catalog.pg_namespace n
 JOIN pg_catalog.pg_class b ON b.relnamespace=n.oid AND b.relname='brands' AND b.relkind IN ('r','p')
 JOIN pg_catalog.pg_class p ON p.relnamespace=n.oid AND p.relname='periods' AND p.relkind IN ('r','p')
 JOIN pg_catalog.pg_class j ON j.relnamespace=n.oid AND j.relname='settlement_jobs' AND j.relkind IN ('r','p')
 JOIN pg_catalog.pg_class m ON m.relnamespace=n.oid AND m.relname='schema_migrations' AND m.relkind IN ('r','p')
 WHERE n.nspname = ANY(pg_catalog.current_schemas(false))
   AND n.nspname <> 'pg_catalog' AND n.nspname <> 'information_schema'
   AND n.nspname NOT LIKE 'pg_temp_%' AND n.nspname NOT LIKE 'pg_toast%'
	ORDER BY coalesce(pg_catalog.array_position(pg_catalog.current_schemas(false), n.nspname), 2147483647), n.oid
 LIMIT 1`).Scan(&schema, &isPrimary)
	if err != nil {
		return out, errNoApplicationSchema
	}
	if !isPrimary {
		return out, errors.New("primary database is in recovery")
	}
	if strings.ContainsRune(schema, 0) {
		return out, errNoApplicationSchema
	}
	app := pgx.Identifier{schema}.Sanitize()
	query := operationalQuery(app)
	rows, err := tx.Query(ctx, query)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var kind string
		var pending, oldest, failed int64
		var discrepancy *float64
		var visible, reconciliationObserved bool
		if err = rows.Scan(&kind, &pending, &oldest, &failed, &discrepancy, &reconciliationObserved, &visible); err != nil {
			rows.Close()
			return out, err
		}
		out.workPending[kind] = float64(pending)
		if oldest < 0 {
			oldest = 0
		}
		out.workOldest[kind] = float64(oldest)
		out.workFailed[kind] = float64(failed)
		if discrepancy != nil {
			out.discrepancy = sourceValue{value: *discrepancy, valid: true}
		}
		out.reconciliationObserved = boolFloat(reconciliationObserved)
		out.replicationStatsVisible = boolFloat(visible)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()
	if out.replicationStatsVisible == 1 {
		var observed bool
		var lag *float64
		err = tx.QueryRow(ctx, `SELECT
	 EXISTS(SELECT 1 FROM pg_catalog.pg_stat_replication r WHERE r.state='streaming' AND r.replay_lsn IS NOT NULL),
	 (SELECT max(pg_catalog.pg_wal_lsn_diff(pg_catalog.pg_current_wal_lsn(),r.replay_lsn))::double precision
	    FROM pg_catalog.pg_stat_replication r WHERE r.state='streaming' AND r.replay_lsn IS NOT NULL)`).Scan(&observed, &lag)
		if err != nil {
			return out, err
		}
		out.replicationObserved = boolFloat(observed)
		if observed && lag != nil {
			out.replicationLag = sourceValue{value: *lag, valid: true}
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return out, err
	}
	return out, nil
}

func operationalQuery(s string) string {
	// Pending is limited to worker owned actions. Manual review, approval,
	// insufficient-funds pauses, and other human gates are deliberately omitted.
	return fmt.Sprintf(`WITH work AS (
 SELECT 'notification'::text kind, count(*) FILTER (WHERE d.status='pending' AND d.next_attempt_at<=clock_timestamp()) pending,
   coalesce(floor(extract(epoch FROM clock_timestamp()-min(e.created_at) FILTER (WHERE d.status='pending' AND d.next_attempt_at<=clock_timestamp()))),-1)::bigint oldest,
   count(*) FILTER (WHERE d.status='failed') failed
 FROM %s.notification_deliveries d JOIN %s.outbox_events e ON e.id=d.event_id
 UNION ALL
 SELECT 'settlement', count(DISTINCT j.id) FILTER (WHERE j.state IN('processing','paying')),
   coalesce(floor(extract(epoch FROM clock_timestamp()-min(j.created_at) FILTER (WHERE j.state IN('processing','paying')))), -1)::bigint,
	   count(DISTINCT j.id) FILTER (WHERE j.state='failed') + count(t.order_id) FILTER (WHERE t.state='failed')
 FROM %s.settlement_jobs j LEFT JOIN %s.settlement_targets t ON t.job_id=j.id
 UNION ALL
 SELECT 'period_refund', count(t.order_id) FILTER (WHERE j.state='processing' AND t.state='pending'),
   coalesce(floor(extract(epoch FROM clock_timestamp()-min(j.created_at) FILTER (WHERE j.state='processing' AND t.state='pending'))),-1)::bigint,
   count(DISTINCT j.id) FILTER (WHERE j.state='failed') + count(t.order_id) FILTER (WHERE t.state='failed')
 FROM %s.period_cancellations j LEFT JOIN %s.period_cancellation_targets t ON t.cancellation_id=j.id
 UNION ALL
	 SELECT 'draw_correction', count(*) FILTER (WHERE j.state='reversing' OR (j.state='resettling' AND s.state IN('processing','paying'))),
	   coalesce(floor(extract(epoch FROM clock_timestamp()-min(j.created_at) FILTER (WHERE j.state='reversing' OR (j.state='resettling' AND s.state IN('processing','paying'))))),-1)::bigint,
	   count(*) FILTER (WHERE j.state='failed')
	 FROM %s.draw_corrections j LEFT JOIN %s.settlement_jobs s ON s.id=j.new_job_id
 UNION ALL
	 SELECT 'commission_cycle', count(*) FILTER (WHERE (j.state IN('enumerating','waiting','calculating','summarizing') AND j.next_work_at<=clock_timestamp()) OR (j.state='ready' AND r.evidence_epoch<>j.evidence_epoch)),
	   coalesce(floor(extract(epoch FROM clock_timestamp()-min(j.created_at) FILTER (WHERE (j.state IN('enumerating','waiting','calculating','summarizing') AND j.next_work_at<=clock_timestamp()) OR (j.state='ready' AND r.evidence_epoch<>j.evidence_epoch)))),-1)::bigint,
	   count(*) FILTER (WHERE j.state='failed')
	 FROM %s.commission_cycles j LEFT JOIN %s.commission_runs r ON r.id=j.current_run_id
 UNION ALL
 SELECT 'commission_payment', count(t.id) FILTER (WHERE j.state='paying' AND t.state='pending' AND j.next_work_at<=clock_timestamp()),
   coalesce(floor(extract(epoch FROM clock_timestamp()-min(j.created_at) FILTER (WHERE j.state='paying' AND t.state='pending' AND j.next_work_at<=clock_timestamp()))),-1)::bigint,
   count(DISTINCT j.id) FILTER (WHERE j.state='failed')
 FROM %s.commission_payments j LEFT JOIN %s.commission_payment_targets t ON t.payment_id=j.id
 UNION ALL
 SELECT 'commission_correction', count(t.id) FILTER (WHERE j.state='applying' AND t.state='pending' AND j.next_work_at<=clock_timestamp()),
   coalesce(floor(extract(epoch FROM clock_timestamp()-min(j.created_at) FILTER (WHERE j.state='applying' AND t.state='pending' AND j.next_work_at<=clock_timestamp()))),-1)::bigint,
   count(DISTINCT j.id) FILTER (WHERE j.state='failed')
 FROM %s.commission_correction_executions j LEFT JOIN %s.commission_correction_execution_targets t ON t.execution_id=j.id
 UNION ALL
 SELECT 'report_archive', count(*) FILTER (WHERE j.state='pending'),
   coalesce(floor(extract(epoch FROM clock_timestamp()-min(j.created_at) FILTER (WHERE j.state='pending'))),-1)::bigint,
   count(*) FILTER (WHERE j.state='failed')
 FROM %s.report_archive_automatic_tasks j
 UNION ALL
 SELECT 'withdrawal', count(*) FILTER (WHERE j.state='processing'),
   coalesce(floor(extract(epoch FROM clock_timestamp()-min(j.created_at) FILTER (WHERE j.state='processing'))),-1)::bigint,
	   0::bigint
	 FROM %s.withdrawal_orders j
), latest_business_reconciliation AS (
 SELECT DISTINCT ON (j.brand_id) j.brand_id,j.id
 FROM %s.point_reconciliation_jobs j
 WHERE j.check_scope='wallet_and_business' AND j.state='completed'
 ORDER BY j.brand_id,j.completed_at DESC,j.id DESC
), reconciliation AS (
 SELECT coalesce(sum((r.business_preview->>'issue_count')::bigint),0)::double precision discrepancies,
   EXISTS(SELECT 1 FROM latest_business_reconciliation) observed
 FROM latest_business_reconciliation latest
 LEFT JOIN %s.point_reconciliation_results r ON r.brand_id=latest.brand_id AND r.job_id=latest.id AND r.business_preview IS NOT NULL
), replication AS (
	 SELECT (current_setting('is_superuser')='on'
      OR pg_catalog.pg_has_role(current_user,'pg_read_all_stats','MEMBER')
      OR pg_catalog.pg_has_role(current_user,'pg_monitor','MEMBER'))
	      AND pg_catalog.has_table_privilege(current_user,'pg_catalog.pg_stat_replication','SELECT') visible
)
SELECT w.kind,w.pending,w.oldest,w.failed,c.discrepancies,c.observed,r.visible
FROM work w CROSS JOIN reconciliation c CROSS JOIN replication r`,
		s, s, s, s, s, s, s, s, s, s, s, s, s, s, s, s, s, s)
}
