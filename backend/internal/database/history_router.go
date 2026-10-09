package database

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HistoryRoute is a closed allowlist. No caller-supplied header or query parameter
// can route current configuration, authorization, financial or gameplay reads.
type HistoryRoute string

const (
	HistoryAudit        HistoryRoute = "audit.view"
	HistoryNotification HistoryRoute = "notification.template.history"
	HistoryCompliance   HistoryRoute = "compliance_policy.history.view"
	HistoryPresentation HistoryRoute = "brand_presentation.history"
	HistoryDomains      HistoryRoute = "brand_domains.history"
	HistoryOperation    HistoryRoute = "brand_operation.history"
)

func (r HistoryRoute) allowed() bool {
	switch r {
	case HistoryAudit, HistoryNotification, HistoryCompliance, HistoryPresentation, HistoryDomains, HistoryOperation:
		return true
	}
	return false
}

type ReadSource struct {
	Replica int    // One-based configured node index; zero means primary.
	Reason  string // Fixed vocabulary, never database errors, addresses or credentials.
}

func (s ReadSource) Name() string {
	if s.Replica > 0 {
		return "replica"
	}
	return "primary"
}

// HistoryRouter requires direct physical-node DSNs (or session-affine pooling),
// never transaction/statement pooling or a proxy which changes nodes mid-session.
// Only side-effect-free, completely buffered queries may be passed to Read.
type HistoryRouter struct {
	replicas []*pgxpool.Pool
	next     atomic.Uint64
}

func NewHistoryRouter(replicas []*pgxpool.Pool) *HistoryRouter {
	return &HistoryRouter{replicas: append([]*pgxpool.Pool(nil), replicas...)}
}

type walFence struct {
	system, database, encoding, schemas, lsn string
	timeline                                 uint64
}

const primaryFenceSQL = `SELECT (pg_catalog.pg_control_system()).system_identifier::text,
 pg_catalog.current_database(),pg_catalog.current_setting('server_encoding'),pg_catalog.current_schemas(true)::text,
 pg_catalog.pg_current_wal_insert_lsn()::text,
 substring(pg_catalog.pg_walfile_name(pg_catalog.pg_current_wal_insert_lsn()),1,8),
 pg_catalog.pg_is_in_recovery()`
const replicaFenceSQL = `SELECT (pg_catalog.pg_control_system()).system_identifier::text,
 pg_catalog.current_database(),pg_catalog.current_setting('server_encoding'),pg_catalog.current_schemas(true)::text,
 pg_catalog.pg_last_wal_replay_lsn()::text,
 (pg_catalog.pg_control_checkpoint()).timeline_id,
 (pg_catalog.pg_control_recovery()).min_recovery_end_timeline,
 pg_catalog.pg_is_in_recovery(),pg_catalog.pg_is_wal_replay_paused()`

func readFence(ctx context.Context, tx pgx.Tx) (walFence, error) {
	var f walFence
	var timeline string
	var recovery bool
	err := tx.QueryRow(ctx, primaryFenceSQL).Scan(&f.system, &f.database, &f.encoding, &f.schemas, &f.lsn, &timeline, &recovery)
	if err != nil {
		return f, err
	}
	f.timeline, err = strconv.ParseUint(timeline, 16, 32)
	if err != nil || recovery || f.encoding != "UTF8" || f.system == "" || f.timeline == 0 {
		return f, errors.New("primary fence unavailable")
	}
	if _, err = parseLSN(f.lsn); err != nil {
		return f, err
	}
	return f, nil
}
func parseLSN(s string) (uint64, error) {
	var sep = -1
	for i, c := range s {
		if c == '/' {
			if sep >= 0 {
				return 0, errors.New("invalid WAL position")
			}
			sep = i
		} else if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return 0, errors.New("invalid WAL position")
		}
	}
	if sep < 1 || sep > 8 || len(s)-sep-1 < 1 || len(s)-sep-1 > 8 {
		return 0, errors.New("invalid WAL position")
	}
	hi, e := strconv.ParseUint(s[:sep], 16, 32)
	if e != nil {
		return 0, errors.New("invalid WAL position")
	}
	lo, e := strconv.ParseUint(s[sep+1:], 16, 32)
	if e != nil {
		return 0, errors.New("invalid WAL position")
	}
	return hi<<32 | lo, nil
}
func eligible(f walFence, system, db, encoding, schemas string, replay *string, timeline, recoveryTimeline uint64, recovery, paused bool) bool {
	if !recovery || paused || replay == nil || system != f.system || db != f.database || encoding != f.encoding || schemas != f.schemas || timeline != f.timeline || recoveryTimeline != f.timeline {
		return false
	}
	want, e := parseLSN(f.lsn)
	if e != nil {
		return false
	}
	got, e := parseLSN(*replay)
	return e == nil && got >= want
}

// Read samples a WAL fence only after the handler locks and checks primary ACLs.
// Probe errors are isolated by a savepoint so the caller's primary transaction
// remains usable when routing returns an error.
func (h *HistoryRouter) Read(ctx context.Context, primary pgx.Tx, route HistoryRoute, run func(pgx.Tx) (any, error)) (any, ReadSource, error) {
	source := ReadSource{Reason: "not_configured"}
	if err := ctx.Err(); err != nil {
		return nil, source, err
	}
	if h == nil || len(h.replicas) == 0 {
		out, e := run(primary)
		return out, source, e
	}
	if !route.allowed() {
		source.Reason = "primary_only"
		out, e := run(primary)
		return out, source, e
	}
	if err := ctx.Err(); err != nil {
		return nil, source, err
	}
	probe, err := primary.Begin(ctx)
	if err != nil {
		source.Reason = "fence_unavailable"
		return nil, source, errors.New("primary history fence unavailable")
	}
	f, err := readFence(ctx, probe)
	if err != nil {
		_ = probe.Rollback(ctx)
		source.Reason = "fence_unavailable"
		return nil, source, errors.New("primary history fence unavailable")
	}
	if err = probe.Commit(ctx); err != nil {
		source.Reason = "fence_unavailable"
		return nil, source, errors.New("primary history fence unavailable")
	}
	i := int((h.next.Add(1) - 1) % uint64(len(h.replicas)))
	source.Replica = i + 1
	source.Reason = "replica_unavailable"
	pool := h.replicas[i]
	if pool == nil {
		return nil, source, errors.New("configured history replica unavailable")
	}
	nodeCtx, nodeCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	conn, e := pool.Acquire(nodeCtx)
	if e != nil {
		nodeCancel()
		return nil, source, errors.New("configured history replica unavailable")
	}
	var system, db, encoding, schemas string
	var replay *string
	var timeline, recoveryTimeline uint64
	var recovery, paused bool
	e = conn.QueryRow(nodeCtx, replicaFenceSQL).Scan(&system, &db, &encoding, &schemas, &replay, &timeline, &recoveryTimeline, &recovery, &paused)
	nodeCancel()
	if e != nil {
		conn.Release()
		return nil, source, errors.New("configured history replica fence failed")
	}
	if !eligible(f, system, db, encoding, schemas, replay, timeline, recoveryTimeline, recovery, paused) {
		conn.Release()
		source.Reason = "replica_not_wal_fenced"
		return nil, source, errors.New("configured history replica is not WAL-fenced")
	}
	// Establish the repeatable-read snapshot after the replay check on the same
	// held physical connection.
	queryCtx, queryCancel := context.WithTimeout(ctx, 2*time.Second)
	tx, e := conn.BeginTx(queryCtx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		queryCancel()
		conn.Release()
		source.Reason = "replica_query_failed"
		return nil, source, errors.New("configured history replica query failed")
	}
	out, e := run(historyQueryTx{Tx: tx, ctx: queryCtx})
	if e == nil {
		e = tx.Commit(queryCtx)
	}
	cleanup, cleanupCancel := context.WithTimeout(context.Background(), time.Second)
	_ = tx.Rollback(cleanup)
	cleanupCancel()
	queryCancel()
	conn.Release()
	if e != nil {
		source.Reason = "replica_query_failed"
		return nil, source, errors.New("configured history replica query failed")
	}
	return out, ReadSource{Replica: i + 1, Reason: "wal_fenced"}, nil
}

// Bind every data statement to the node's shorter deadline even when a handler
// callback passes its original request context. Transactions remain read-only.
type historyQueryTx struct {
	pgx.Tx
	ctx context.Context
}

func (t historyQueryTx) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	return t.Tx.Query(t.ctx, sql, args...)
}
func (t historyQueryTx) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	return t.Tx.QueryRow(t.ctx, sql, args...)
}
func (t historyQueryTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return t.Tx.Exec(t.ctx, sql, args...)
}
