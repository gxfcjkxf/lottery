package reconciliation

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

const jobJSON = `jsonb_build_object('check_scope',j.check_scope,'id',j.id::text,'brand_id',j.brand_id::text,'state',j.state,'version',j.version,
 'target_count',j.target_count::text,'checked_count',s.checked::text,'consistent_count',s.consistent::text,
 'repairable_count',s.repairable::text,'corrupt_count',s.corrupt::text,'failed_count',s.failed::text,'pending_count',s.pending::text,
 'created_by',j.created_by::text,'reason',j.reason,'created_at',j.created_at,'started_at',j.started_at,'completed_at',j.completed_at,
 'last_error_code',CASE WHEN j.state='failed' THEN 'CHECK_FAILED' END,'can_retry',j.state='failed','creation_audit_log_id',j.creation_audit_log_id::text)`
const jobStats = ` LEFT JOIN LATERAL (
 SELECT count(*) FILTER(WHERE t.state='checked') checked,count(*) FILTER(WHERE t.state='pending') pending,
 count(*) FILTER(WHERE t.state='failed') failed,count(*) FILTER(WHERE r.outcome='consistent') consistent,
 count(*) FILTER(WHERE r.outcome='repairable') repairable,count(*) FILTER(WHERE r.outcome='corrupt') corrupt
 FROM point_reconciliation_targets t LEFT JOIN point_reconciliation_results r ON r.target_id=t.id WHERE t.job_id=j.id
 ) s ON true `

func (s Service) ReadTx(ctx context.Context, tx pgx.Tx, brand, id string) (Job, error) {
	var out Job
	if tx == nil || !uuid.MatchString(brand) || !uuid.MatchString(id) {
		return out, ErrInvalid
	}
	var raw []byte
	e := tx.QueryRow(ctx, `SELECT `+jobJSON+` FROM point_reconciliation_jobs j `+jobStats+` WHERE j.brand_id=$1 AND j.id=$2`, brand, id).Scan(&raw)
	if errors.Is(e, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if e != nil {
		return out, e
	}
	e = json.Unmarshal(raw, &out)
	return out, e
}
func (s Service) ListTx(ctx context.Context, tx pgx.Tx, brand string, limit, offset int) (JobPage, error) {
	out := JobPage{BrandID: brand, Items: []Job{}, Limit: limit, Offset: offset}
	if tx == nil || !validPage(brand, limit, offset) {
		return out, ErrInvalid
	}
	var raw []byte
	e := tx.QueryRow(ctx, `WITH page AS (SELECT j.* FROM point_reconciliation_jobs j WHERE brand_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2 OFFSET $3), contents AS (
 SELECT `+jobJSON+` data,j.created_at,j.id FROM page j `+jobStats+`)
 SELECT (SELECT count(*)::text FROM point_reconciliation_jobs WHERE brand_id=$1),
 COALESCE((SELECT jsonb_agg(data ORDER BY created_at DESC,id DESC) FROM contents),'[]'::jsonb)`, brand, limit, offset).Scan(&out.TotalCount, &raw)
	if e != nil {
		return out, e
	}
	e = json.Unmarshal(raw, &out.Items)
	return out, e
}

const targetJSON = `jsonb_build_object('check_scope',(SELECT check_scope FROM point_reconciliation_jobs WHERE id=t.job_id),'business_preview',r.business_preview,'id',t.id::text,'brand_id',t.brand_id::text,'job_id',t.job_id::text,
 'account_id',t.account_id::text,'member_id',t.member_id::text,'state',t.state,'outcome',r.outcome,'preview',r.preview,
 'attempt_count',t.attempt_count,'error_code',CASE WHEN t.state='failed' THEN 'CHECK_FAILED' END,'checked_at',r.checked_at,'audit_log_id',r.audit_log_id::text)`

func (s Service) TargetsTx(ctx context.Context, tx pgx.Tx, brand, id, filter string, limit, offset int) (TargetPage, error) {
	out := TargetPage{BrandID: brand, JobID: id, Items: []Target{}, Limit: limit, Offset: offset}
	if filter != "" {
		out.Outcome = &filter
	}
	if tx == nil || !validPage(brand, limit, offset) || !uuid.MatchString(id) || !ValidFilter(filter) {
		return out, ErrInvalid
	}
	var exists bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM point_reconciliation_jobs WHERE brand_id=$1 AND id=$2)`, brand, id).Scan(&exists); e != nil {
		return out, e
	}
	if !exists {
		return out, ErrNotFound
	}
	var raw []byte
	e := tx.QueryRow(ctx, `WITH selected AS (
 SELECT t.id FROM point_reconciliation_targets t LEFT JOIN point_reconciliation_results r ON r.target_id=t.id
 WHERE t.brand_id=$1 AND t.job_id=$2 AND ($3='' OR CASE WHEN t.state='checked' THEN r.outcome ELSE t.state END=$3)
 ), page AS (SELECT id FROM selected ORDER BY id LIMIT $4 OFFSET $5), contents AS (
 SELECT `+targetJSON+` data,t.id FROM page p JOIN point_reconciliation_targets t ON t.id=p.id LEFT JOIN point_reconciliation_results r ON r.target_id=t.id
 )
 SELECT (SELECT count(*)::text FROM selected),COALESCE((SELECT jsonb_agg(data ORDER BY id) FROM contents),'[]'::jsonb)`, brand, id, filter, limit, offset).Scan(&out.TotalCount, &raw)
	if e != nil {
		return out, e
	}
	e = json.Unmarshal(raw, &out.Items)
	return out, e
}
func writableBrand(ctx context.Context, tx pgx.Tx, brand string) error {
	var state string
	e := tx.QueryRow(ctx, `SELECT status FROM brands WHERE id=$1 FOR SHARE`, brand).Scan(&state)
	if errors.Is(e, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if e != nil {
		return e
	}
	if state == "disabled" {
		return ErrState
	}
	return nil
}
func (s Service) Create(ctx context.Context, tx pgx.Tx, brand string, a access.Account, reason string, meta points.Metadata) (Job, error) {
	return s.CreateScoped(ctx, tx, brand, a, ScopeWallet, reason, meta)
}
func (s Service) CreateScoped(ctx context.Context, tx pgx.Tx, brand string, a access.Account, scope, reason string, meta points.Metadata) (Job, error) {
	var out Job
	if tx == nil || !uuid.MatchString(brand) || !ValidScope(scope) || !validReason(reason) || !validMeta(a, meta) {
		return out, ErrInvalid
	}
	if !Allowed(a, brand, "run") {
		return out, ErrDenied
	}
	if e := writableBrand(ctx, tx, brand); e != nil {
		return out, e
	}
	// Serialize only maintenance creation for this brand, never all wallet writes.
	if _, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('wallet-reconciliation:'||$1,0))`, brand); e != nil {
		return out, e
	}
	var active bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM point_reconciliation_jobs WHERE brand_id=$1 AND state IN('pending','running'))`, brand).Scan(&active); e != nil {
		return out, e
	}
	if active {
		return out, ErrState
	}
	rows, e := tx.Query(ctx, `SELECT id::text,brand_member_id::text FROM point_accounts WHERE brand_id=$1 ORDER BY id LIMIT $2`, brand, MaxTargets+1)
	if e != nil {
		return out, e
	}
	type capturedAccount struct{ account, member string }
	accounts := []capturedAccount{}
	for rows.Next() {
		var v capturedAccount
		if e = rows.Scan(&v.account, &v.member); e != nil {
			rows.Close()
			return out, e
		}
		accounts = append(accounts, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if len(accounts) > MaxTargets {
		return out, ErrTooLarge
	}
	id := ids.New()
	var at time.Time
	if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at); e != nil {
		return out, e
	}
	auditID, e := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "wallet.reconciliation.create", ResourceType: "wallet_reconciliation_job", ResourceID: id, Reason: reason, RequestID: meta.RequestID, IP: meta.IP, After: map[string]any{"id": id, "brand_id": brand, "state": "pending", "version": 1, "check_scope": scope, "target_count": strconv.Itoa(len(accounts)), "created_at": at.UTC()}})
	if e != nil {
		return out, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO point_reconciliation_jobs(id,brand_id,target_count,created_by,reason,created_at,creation_audit_log_id,check_scope) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, brand, len(accounts), a.ID, reason, at, auditID, scope); e != nil {
		return out, e
	}
	if len(accounts) > 0 {
		data := make([][]any, 0, len(accounts))
		for _, v := range accounts {
			data = append(data, []any{ids.New(), brand, id, v.account, v.member})
		}
		if _, e = tx.CopyFrom(ctx, pgx.Identifier{"point_reconciliation_targets"}, []string{"id", "brand_id", "job_id", "account_id", "member_id"}, pgx.CopyFromRows(data)); e != nil {
			return out, e
		}
	}
	return s.ReadTx(ctx, tx, brand, id)
}
func (s Service) Retry(ctx context.Context, tx pgx.Tx, brand, id string, a access.Account, version int64, reason string, meta points.Metadata) (Job, error) {
	var out Job
	if tx == nil || !uuid.MatchString(brand) || !uuid.MatchString(id) || !validReason(reason) || version < 1 || version >= maxVersion || !validMeta(a, meta) {
		return out, ErrInvalid
	}
	if !Allowed(a, brand, "retry") {
		return out, ErrDenied
	}
	if e := writableBrand(ctx, tx, brand); e != nil {
		return out, e
	}
	if _, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('wallet-reconciliation:'||$1,0))`, brand); e != nil {
		return out, e
	}
	var state string
	var current int64
	var failure *string
	e := tx.QueryRow(ctx, `SELECT state,version,last_failure_id::text FROM point_reconciliation_jobs WHERE brand_id=$1 AND id=$2 FOR UPDATE`, brand, id).Scan(&state, &current, &failure)
	if errors.Is(e, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if e != nil {
		return out, e
	}
	if current != version {
		return out, ErrVersion
	}
	if state != "failed" || failure == nil {
		return out, ErrState
	}
	var active bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM point_reconciliation_jobs WHERE brand_id=$1 AND state IN('pending','running'))`, brand).Scan(&active); e != nil {
		return out, e
	}
	if active {
		return out, ErrState
	}
	auditID, e := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "wallet.reconciliation.retry", ResourceType: "wallet_reconciliation_job", ResourceID: id, Reason: reason, RequestID: meta.RequestID, IP: meta.IP, After: map[string]any{"job_id": id, "version": version + 1, "previous_failure_id": *failure}})
	if e != nil {
		return out, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO point_reconciliation_retries(id,brand_id,job_id,version,previous_failure_id,created_by,reason,audit_log_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, ids.New(), brand, id, version+1, *failure, a.ID, reason, auditID); e != nil {
		return out, e
	}
	if _, e = tx.Exec(ctx, `UPDATE point_reconciliation_targets SET state='pending',next_check_at=clock_timestamp() WHERE brand_id=$1 AND job_id=$2 AND state='failed'`, brand, id); e != nil {
		return out, e
	}
	if _, e = tx.Exec(ctx, `UPDATE point_reconciliation_jobs SET state='pending',version=version+1,last_failure_id=NULL,last_step_at=clock_timestamp() WHERE brand_id=$1 AND id=$2`, brand, id); e != nil {
		return out, e
	}
	return s.ReadTx(ctx, tx, brand, id)
}
