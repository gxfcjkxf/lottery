// Package capacity contains read-only samplers used by controlled capacity runs.
package capacity

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBSnapshot contains database counters and operational queue counts. It does
// not include session identities, SQL text, connection strings, or credentials.
type DBSnapshot struct {
	TimestampUTC      time.Time              `json:"timestampUTC"`
	Counters          DBCounters             `json:"counters"`
	TotalConnections  int64                  `json:"totalConnections"`
	ActiveConnections int64                  `json:"activeConnections"`
	LockWaiters       int64                  `json:"lockWaiters"`
	ConnectionWaits   map[string]int64       `json:"connectionWaitsByWaitEventType"`
	Queues            map[string]QueueCounts `json:"queues"`
	Replication       ReplicationSnapshot    `json:"replication"`
}

// DBCounters are PostgreSQL's per-database cumulative statistics.
type DBCounters struct {
	XactCommit   int64 `json:"xactCommit"`
	XactRollback int64 `json:"xactRollback"`
	BlksHit      int64 `json:"blksHit"`
	BlksRead     int64 `json:"blksRead"`
	Deadlocks    int64 `json:"deadlocks"`
}

// DBCounterDelta is the nonnegative change between two samples. If a counter
// reset is detected, current-value fallbacks and ordinary differences are
// lower bounds on the true sample-window deltas; SharedCacheHitRatio is then
// unavailable. The ratio is also unavailable when no blocks changed.
type DBCounterDelta struct {
	XactCommit           int64    `json:"xactCommit"`
	XactRollback         int64    `json:"xactRollback"`
	BlksHit              int64    `json:"blksHit"`
	BlksRead             int64    `json:"blksRead"`
	Deadlocks            int64    `json:"deadlocks"`
	CounterResetDetected bool     `json:"counterResetDetected"`
	SharedCacheHitRatio  *float64 `json:"sharedCacheHitRatio"`
}

// QueueCounts groups known durable work states without reading job payloads.
type QueueCounts struct {
	Pending   int64 `json:"pending"`
	Retry     int64 `json:"retry"`
	Sent      int64 `json:"sent"`
	Failed    int64 `json:"failed"`
	Completed int64 `json:"completed"`
}

// ReplicationSnapshot reports lag only. A NULL lag is unavailable; an empty
// replica list means the server has no visible streaming replicas.
type ReplicationSnapshot struct {
	Available bool             `json:"available"`
	Replicas  []ReplicationLag `json:"replicas"`
}

type ReplicationLag struct {
	WriteSeconds  *float64 `json:"writeSeconds"`
	FlushSeconds  *float64 `json:"flushSeconds"`
	ReplaySeconds *float64 `json:"replaySeconds"`
}

// DBCounterDeltas calculates deltas between snapshots. A decrease in any
// counter flags a reset, uses current values for decreased counters, and makes
// the cache-hit ratio unavailable because the sample-window ratio is unknown.
func DBCounterDeltas(previous, current DBSnapshot) DBCounterDelta {
	resetDetected := countersDecreased(previous.Counters, current.Counters)
	delta := DBCounterDelta{
		XactCommit:           counterDelta(previous.Counters.XactCommit, current.Counters.XactCommit),
		XactRollback:         counterDelta(previous.Counters.XactRollback, current.Counters.XactRollback),
		BlksHit:              counterDelta(previous.Counters.BlksHit, current.Counters.BlksHit),
		BlksRead:             counterDelta(previous.Counters.BlksRead, current.Counters.BlksRead),
		Deadlocks:            counterDelta(previous.Counters.Deadlocks, current.Counters.Deadlocks),
		CounterResetDetected: resetDetected,
	}
	blockTotal := float64(delta.BlksHit) + float64(delta.BlksRead)
	if !resetDetected && blockTotal > 0 {
		ratio := float64(delta.BlksHit) / blockTotal
		delta.SharedCacheHitRatio = &ratio
	}
	return delta
}

func countersDecreased(previous, current DBCounters) bool {
	return current.XactCommit < previous.XactCommit ||
		current.XactRollback < previous.XactRollback ||
		current.BlksHit < previous.BlksHit ||
		current.BlksRead < previous.BlksRead ||
		current.Deadlocks < previous.Deadlocks
}

func counterDelta(previous, current int64) int64 {
	if current < previous {
		return current
	}
	return current - previous
}

// SnapshotDB reads PostgreSQL statistics and queue counts from the pool's
// current schema. Optional queue tables are discovered in that explicit schema
// before they are queried, so partial migration states are supported.
func SnapshotDB(ctx context.Context, pool *pgxpool.Pool) (DBSnapshot, error) {
	var snapshot DBSnapshot
	if pool == nil {
		return snapshot, fmt.Errorf("capacity: nil PostgreSQL pool")
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return snapshot, fmt.Errorf("capacity: acquire PostgreSQL connection: %w", err)
	}
	defer conn.Release()

	var schemaFilter string
	if err := conn.QueryRow(ctx, `SELECT current_schema()`).Scan(&schemaFilter); err != nil {
		return snapshot, fmt.Errorf("capacity: read current schema: %w", err)
	}
	if schemaFilter == "" {
		return snapshot, fmt.Errorf("capacity: current schema is empty")
	}

	if err := conn.QueryRow(ctx, `SELECT statement_timestamp(), xact_commit, xact_rollback, blks_hit, blks_read, deadlocks
		FROM pg_stat_database WHERE datname = current_database()`).Scan(
		&snapshot.TimestampUTC,
		&snapshot.Counters.XactCommit,
		&snapshot.Counters.XactRollback,
		&snapshot.Counters.BlksHit,
		&snapshot.Counters.BlksRead,
		&snapshot.Counters.Deadlocks,
	); err != nil {
		return snapshot, fmt.Errorf("capacity: read database statistics: %w", err)
	}
	snapshot.TimestampUTC = snapshot.TimestampUTC.UTC()

	if err := conn.QueryRow(ctx, `SELECT
		count(*),
		count(*) FILTER (WHERE state = 'active'),
		count(*) FILTER (WHERE wait_event_type = 'Lock')
		FROM pg_stat_activity WHERE datname = current_database() AND pid <> pg_backend_pid()`).Scan(
		&snapshot.TotalConnections, &snapshot.ActiveConnections, &snapshot.LockWaiters,
	); err != nil {
		return snapshot, fmt.Errorf("capacity: read connection statistics: %w", err)
	}
	snapshot.ConnectionWaits = make(map[string]int64)
	waitRows, err := conn.Query(ctx, `SELECT wait_event_type, count(*)
		FROM pg_stat_activity
		WHERE datname = current_database() AND pid <> pg_backend_pid() AND wait_event_type IS NOT NULL
		GROUP BY wait_event_type`)
	if err != nil {
		return snapshot, fmt.Errorf("capacity: read connection wait statistics: %w", err)
	}
	for waitRows.Next() {
		var waitEventType string
		var count int64
		if err := waitRows.Scan(&waitEventType, &count); err != nil {
			waitRows.Close()
			return snapshot, fmt.Errorf("capacity: scan connection wait statistics: %w", err)
		}
		snapshot.ConnectionWaits[waitEventType] = count
	}
	if err := waitRows.Err(); err != nil {
		waitRows.Close()
		return snapshot, fmt.Errorf("capacity: read connection wait rows: %w", err)
	}
	waitRows.Close()

	snapshot.Queues = make(map[string]QueueCounts)
	queueSpecs := []queueSpec{
		{name: "notification_deliveries", countSQL: `SELECT
			count(*) FILTER (WHERE status = 'pending' AND attempt_count = 0),
			count(*) FILTER (WHERE status = 'pending' AND attempt_count > 0),
			count(*) FILTER (WHERE status = 'sent'),
			count(*) FILTER (WHERE status = 'failed'), 0
			FROM %s`},
		{name: "period_cancellations", countSQL: `SELECT
			count(*) FILTER (WHERE state = 'processing'), 0, 0,
			count(*) FILTER (WHERE state = 'failed'),
			count(*) FILTER (WHERE state = 'completed') FROM %s`},
		{name: "settlement_jobs", countSQL: `SELECT
			count(*) FILTER (WHERE state IN ('processing','paying','awaiting_approval')), 0, 0,
			count(*) FILTER (WHERE state = 'failed'),
			count(*) FILTER (WHERE state = 'completed') FROM %s`},
		{name: "draw_corrections", countSQL: `SELECT
			count(*) FILTER (WHERE state IN ('reversing','resettling')), 0, 0,
			count(*) FILTER (WHERE state = 'failed'),
			count(*) FILTER (WHERE state = 'completed') FROM %s`},
	}
	for _, spec := range queueSpecs {
		qualified := pgx.Identifier{schemaFilter, spec.name}.Sanitize()
		var exists bool
		if err := conn.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM pg_catalog.pg_class c
			JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = $1 AND c.relname = $2 AND c.relkind IN ('r','p')
		)`, schemaFilter, spec.name).Scan(&exists); err != nil {
			return snapshot, fmt.Errorf("capacity: inspect queue table %s: %w", spec.name, err)
		}
		if !exists {
			continue
		}
		var counts QueueCounts
		if err := conn.QueryRow(ctx, fmt.Sprintf(spec.countSQL, qualified)).Scan(
			&counts.Pending, &counts.Retry, &counts.Sent, &counts.Failed, &counts.Completed,
		); err != nil {
			return snapshot, fmt.Errorf("capacity: count queue %s: %w", spec.name, err)
		}
		snapshot.Queues[spec.name] = counts
	}

	snapshot.Replication.Replicas = make([]ReplicationLag, 0)
	rows, err := conn.Query(ctx, `SELECT
		EXTRACT(EPOCH FROM write_lag)::double precision,
		EXTRACT(EPOCH FROM flush_lag)::double precision,
		EXTRACT(EPOCH FROM replay_lag)::double precision
		FROM pg_stat_replication`)
	if err != nil {
		return snapshot, fmt.Errorf("capacity: read replication lag: %w", err)
	}
	for rows.Next() {
		var write, flush, replay pgtype.Float8
		if err := rows.Scan(&write, &flush, &replay); err != nil {
			rows.Close()
			return snapshot, fmt.Errorf("capacity: scan replication lag: %w", err)
		}
		snapshot.Replication.Replicas = append(snapshot.Replication.Replicas, ReplicationLag{
			WriteSeconds:  nullableFloat(write),
			FlushSeconds:  nullableFloat(flush),
			ReplaySeconds: nullableFloat(replay),
		})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return snapshot, fmt.Errorf("capacity: read replication rows: %w", err)
	}
	rows.Close()
	snapshot.Replication.Available = len(snapshot.Replication.Replicas) > 0

	return snapshot, nil
}

type queueSpec struct {
	name     string
	countSQL string
}

func nullableFloat(value pgtype.Float8) *float64 {
	if !value.Valid {
		return nil
	}
	return &value.Float64
}
