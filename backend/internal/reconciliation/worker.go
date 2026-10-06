package reconciliation

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

func (s Service) Process(ctx context.Context, limit int) (int, error) {
	if s.DB == nil || limit < 1 || limit > 100 {
		return 0, ErrInvalid
	}
	n := 0
	for i := 0; i < limit; i++ {
		worked, e := s.processOne(ctx)
		if e != nil {
			return n, e
		}
		if !worked {
			return n, nil
		}
		n++
	}
	return n, nil
}
func (s Service) processOne(ctx context.Context) (worked bool, resultErr error) {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return false, e
	}
	defer tx.Rollback(ctx)
	var job, brand string
	e = tx.QueryRow(ctx, `SELECT id::text,brand_id::text FROM point_reconciliation_jobs j WHERE state IN('pending','running') AND (
 EXISTS(SELECT 1 FROM point_reconciliation_targets t WHERE t.job_id=j.id AND t.state='pending' AND t.next_check_at<=clock_timestamp()) OR NOT EXISTS(SELECT 1 FROM point_reconciliation_targets t WHERE t.job_id=j.id AND t.state='pending'))
 ORDER BY last_step_at,id LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&job, &brand)
	if errors.Is(e, pgx.ErrNoRows) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	var target, account, member string
	var attempt int
	e = tx.QueryRow(ctx, `SELECT id::text,account_id::text,member_id::text,attempt_count FROM point_reconciliation_targets WHERE job_id=$1 AND state='pending' AND next_check_at<=clock_timestamp() ORDER BY next_check_at,id LIMIT 1 FOR UPDATE`, job).Scan(&target, &account, &member, &attempt)
	if errors.Is(e, pgx.ErrNoRows) {
		var pending bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM point_reconciliation_targets WHERE job_id=$1 AND state='pending')`, job).Scan(&pending); e != nil {
			return false, e
		}
		if pending {
			return false, nil
		}
		if _, e = tx.Exec(ctx, `UPDATE point_reconciliation_jobs SET state='completed',version=version+1,started_at=COALESCE(started_at,clock_timestamp()),completed_at=clock_timestamp(),last_step_at=clock_timestamp() WHERE id=$1`, job); e != nil {
			return false, e
		}
		return true, tx.Commit(ctx)
	}
	if e != nil {
		return false, e
	}
	// Every known-target technical failure is recorded only after the failed
	// transaction rolls back. A lost commit acknowledgment is resolved by the
	// new transaction's target-state check, never by overwriting a checked result.
	defer func() {
		if resultErr == nil || ctx.Err() != nil {
			return
		}
		cleanup, stop := context.WithTimeout(context.Background(), 2*time.Second)
		_ = tx.Rollback(cleanup)
		stop()
		failureCtx, stopFailure := context.WithTimeout(context.Background(), 2*time.Second)
		defer stopFailure()
		if failureErr := s.recordFailure(failureCtx, brand, job, target, attempt); failureErr != nil {
			resultErr = failureErr
			worked = false
		} else {
			resultErr = nil
			worked = true
		}
	}()
	// Never wait behind a hot account while holding the job lock: defer it and
	// let later targets/other brands progress. All writes lock account first.
	var locked string
	e = tx.QueryRow(ctx, `SELECT id::text FROM point_accounts WHERE brand_id=$1 AND id=$2 FOR SHARE SKIP LOCKED`, brand, account).Scan(&locked)
	if errors.Is(e, pgx.ErrNoRows) {
		_, e = tx.Exec(ctx, `UPDATE point_reconciliation_targets SET next_check_at=clock_timestamp()+interval '1 second' WHERE id=$1`, target)
		if e != nil {
			return false, e
		}
		if _, e = tx.Exec(ctx, `UPDATE point_reconciliation_jobs SET state='running',version=version+1,started_at=COALESCE(started_at,clock_timestamp()),last_step_at=clock_timestamp() WHERE id=$1`, job); e != nil {
			return false, e
		}
		return true, tx.Commit(ctx)
	}
	if e != nil {
		return false, e
	}
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	preview, e := (points.Store{DB: s.DB}).PreviewRepairTx(checkCtx, tx, brand, member)
	cancel()
	if e != nil {
		return false, e
	}
	outcome := "corrupt"
	if preview.Consistent {
		outcome = "consistent"
	} else if preview.Repairable {
		outcome = "repairable"
	}
	var at time.Time
	if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at); e != nil {
		return false, e
	}
	auditID, e := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "system", Action: "wallet.reconciliation.checked", ResourceType: "wallet_reconciliation_target", ResourceID: target, RequestID: ids.New(), After: map[string]any{"job_id": job, "account_id": account, "outcome": outcome, "preview": preview, "checked_at": at.UTC()}})
	if e != nil {
		return false, e
	}
	raw, e := json.Marshal(preview)
	if e != nil {
		return false, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO point_reconciliation_results(target_id,brand_id,job_id,outcome,preview,checked_at,audit_log_id) VALUES($1,$2,$3,$4,$5,$6,$7)`, target, brand, job, outcome, raw, at, auditID); e != nil {
		return false, e
	}
	if _, e = tx.Exec(ctx, `UPDATE point_reconciliation_targets SET state='checked',attempt_count=attempt_count+1 WHERE id=$1`, target); e != nil {
		return false, e
	}
	if _, e = tx.Exec(ctx, `UPDATE point_reconciliation_jobs SET state=CASE WHEN EXISTS(SELECT 1 FROM point_reconciliation_targets WHERE job_id=$1 AND state='pending') THEN 'running' ELSE 'completed' END,
 version=version+1,started_at=COALESCE(started_at,clock_timestamp()),last_step_at=clock_timestamp(),completed_at=CASE WHEN NOT EXISTS(SELECT 1 FROM point_reconciliation_targets WHERE job_id=$1 AND state='pending') THEN clock_timestamp() END WHERE id=$1`, job); e != nil {
		return false, e
	}
	return true, tx.Commit(ctx)
}
func (s Service) recordFailure(ctx context.Context, brand, job, target string, attempt int) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var state string
	e = tx.QueryRow(ctx, `SELECT state FROM point_reconciliation_jobs WHERE brand_id=$1 AND id=$2 FOR UPDATE`, brand, job).Scan(&state)
	if e != nil {
		return e
	}
	if state != "pending" && state != "running" {
		return nil
	}
	var current int
	e = tx.QueryRow(ctx, `SELECT state,attempt_count FROM point_reconciliation_targets WHERE brand_id=$1 AND job_id=$2 AND id=$3 FOR UPDATE`, brand, job, target).Scan(&state, &current)
	if e != nil {
		return e
	}
	if state != "pending" || attempt != current {
		return nil
	}
	id := ids.New()
	auditID, e := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "system", Action: "wallet.reconciliation.failed", ResourceType: "wallet_reconciliation_target", ResourceID: target, RequestID: ids.New(), After: map[string]any{"job_id": job, "error_code": "CHECK_FAILED", "attempt_count": attempt + 1, "failure_id": id}})
	if e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO point_reconciliation_failures(id,brand_id,job_id,target_id,attempt_count,error_code,audit_log_id) VALUES($1,$2,$3,$4,$5,'CHECK_FAILED',$6)`, id, brand, job, target, attempt+1, auditID); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `UPDATE point_reconciliation_targets SET state='failed',attempt_count=attempt_count+1 WHERE id=$1`, target); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `UPDATE point_reconciliation_jobs SET state='failed',version=version+1,last_failure_id=$2 WHERE id=$1`, job, id); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
