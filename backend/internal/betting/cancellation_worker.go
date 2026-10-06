package betting

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

type cancellationFailure struct {
	Job     Cancellation
	OrderID string
	Cause   error
}

// ProcessCancellations commits at most limit individual targets. No transaction
// spans multiple wallets, and no in-memory cursor or lease is needed to resume.
func (s Service) ProcessCancellations(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 100 {
		return 0, ErrInvalid
	}
	processed := 0
	var failures error
	for processed < limit {
		rows, err := s.DB.Query(ctx, `SELECT id::text,brand_id::text,period_id::text FROM period_cancellations WHERE state='processing' ORDER BY created_at,id LIMIT $1`, limit)
		if err != nil {
			return processed, errors.Join(failures, err)
		}
		candidates := [][3]string{}
		for rows.Next() {
			var v [3]string
			if err = rows.Scan(&v[0], &v[1], &v[2]); err != nil {
				rows.Close()
				return processed, errors.Join(failures, err)
			}
			candidates = append(candidates, v)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return processed, errors.Join(failures, err)
		}
		progress := false
		for _, v := range candidates {
			done, failure, err := s.processCancellationTarget(ctx, v[0], v[1], v[2])
			if err != nil {
				if failure != nil && ctx.Err() == nil {
					recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
					recordErr := s.recordCancellationFailure(recordCtx, *failure)
					cancel()
					err = errors.Join(err, recordErr)
				}
				failures = errors.Join(failures, fmt.Errorf("cancellation %s: %w", v[0], err))
			}
			if done {
				processed++
				progress = true
			}
			if processed >= limit {
				break
			}
		}
		if !progress {
			break
		}
	}
	return processed, failures
}
func (s Service) processCancellationTarget(ctx context.Context, id, brand, period string) (done bool, failure *cancellationFailure, err error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return false, nil, err
	}
	defer tx.Rollback(ctx)
	_, _, p, err := s.lockPeriod(ctx, tx, brand, period)
	if err != nil {
		return false, nil, err
	}
	c, err := scanCancellation(tx.QueryRow(ctx, `SELECT `+cancellationFields+` FROM period_cancellations c WHERE c.id=$1 AND c.state='processing' FOR UPDATE SKIP LOCKED`, id), false)
	if errors.Is(err, ErrNotFound) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, err
	}
	if p.Status != c.Mode {
		return false, nil, ErrState
	}
	var orderID, member string
	err = tx.QueryRow(ctx, `SELECT t.order_id::text,o.brand_member_id::text FROM period_cancellation_targets t JOIN bet_orders o ON o.id=t.order_id WHERE t.cancellation_id=$1 AND t.state='pending' ORDER BY t.order_id LIMIT 1 FOR UPDATE OF t`, id).Scan(&orderID, &member)
	if errors.Is(err, pgx.ErrNoRows) {
		if err = s.completeCancellation(ctx, tx, c); err != nil {
			return false, nil, err
		}
		return false, nil, tx.Commit(ctx)
	}
	if err != nil {
		return false, nil, err
	}
	failure = &cancellationFailure{Job: c, OrderID: orderID}
	defer func() {
		if err != nil {
			failure.Cause = err
		} else {
			failure = nil
		}
	}()
	ps := points.Store{DB: s.DB}
	if _, err = ps.LockedSnapshot(ctx, tx, brand, member); err != nil {
		return false, failure, err
	}
	o, err := scanOrder(tx.QueryRow(ctx, `SELECT `+orderFields+` FROM bet_orders WHERE brand_id=$1 AND id=$2 FOR UPDATE`, brand, orderID))
	if err != nil {
		return false, failure, err
	}
	targetState := "refunded"
	if o.Status == "bet_cancelled" || o.Status == "judged_cancelled" {
		// Another authorized cancellation may have won before the task got here.
		// Validate existing immutable reversal evidence instead of posting again.
		original, e := ps.Entry(ctx, tx, brand, member, o.DebitEntryID)
		if e != nil {
			return false, failure, e
		}
		refund, e := ps.Entry(ctx, tx, brand, member, o.RefundEntryID)
		if e != nil {
			return false, failure, e
		}
		opposite, e := points.Negate(original.Delta)
		if e != nil {
			return false, failure, e
		}
		if refund.ReversalOf != original.ID || refund.ReferenceID != o.ID || refund.Delta != opposite {
			return false, failure, points.ErrCorrupt
		}
		targetState = "already_refunded"
	} else {
		o, err = s.refundLocked(ctx, tx, o, c.Mode, c.Reason, points.Metadata{ActorType: "system", RequestID: "period-cancel:" + c.ID})
		if err != nil {
			return false, failure, err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE period_cancellation_targets SET state=$3,version=version+1,refund_entry_id=$4 WHERE cancellation_id=$1 AND order_id=$2`, c.ID, o.ID, targetState, o.RefundEntryID); err != nil {
		return false, failure, err
	}
	var pending bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM period_cancellation_targets WHERE cancellation_id=$1 AND state IN ('pending','failed'))`, c.ID).Scan(&pending); err != nil {
		return false, failure, err
	}
	if !pending {
		if err = s.completeCancellation(ctx, tx, c); err != nil {
			return false, failure, err
		}
	}
	err = tx.Commit(ctx)
	return err == nil, failure, err
}
func (s Service) completeCancellation(ctx context.Context, tx pgx.Tx, c Cancellation) error {
	out, err := scanCancellation(tx.QueryRow(ctx, `UPDATE period_cancellations SET state='completed',version=version+1,completed_at=clock_timestamp() WHERE id=$1 RETURNING `+stringsWithoutAlias(cancellationFields), c.ID), false)
	if err != nil {
		return err
	}
	// Summary counts are read only at completion, not for every target refund.
	out, err = scanCancellation(tx.QueryRow(ctx, `SELECT `+cancellationFields+cancellationCounts+` FROM period_cancellations c WHERE c.id=$1`, c.ID), true)
	if err != nil {
		return err
	}
	if err = cancellationEvent(ctx, tx, out, "period.cancellation.completed"); err != nil {
		return err
	}
	_, err = audit.Append(ctx, tx, audit.Record{BrandID: c.BrandID, ActorType: "system", Action: "period.cancel_completed", ResourceType: "period_cancellation", ResourceID: c.ID, Reason: c.Reason, RequestID: "period-cancel:" + c.ID, Before: map[string]any{"state": c.State, "version": c.Version}, After: out})
	return err
}
func cancellationFailureCode(err error) string {
	switch {
	case errors.Is(err, points.ErrCorrupt):
		return "POINTS_BALANCE_CORRUPT"
	case errors.Is(err, points.ErrOverflow):
		return "POINTS_AMOUNT_OVERFLOW"
	case errors.Is(err, points.ErrPolicyLimit):
		return "POINTS_POLICY_LIMIT"
	case errors.Is(err, ErrState):
		return "ORDER_STATE_CONFLICT"
	default:
		return "REFUND_TRANSACTION_FAILED"
	}
}
func (s Service) recordCancellationFailure(ctx context.Context, f cancellationFailure) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, _, _, err = s.lockPeriod(ctx, tx, f.Job.BrandID, f.Job.PeriodID); err != nil {
		return err
	}
	current, err := scanCancellation(tx.QueryRow(ctx, `SELECT `+cancellationFields+` FROM period_cancellations c WHERE c.id=$1 FOR UPDATE`, f.Job.ID), false)
	if err != nil {
		return err
	}
	code := cancellationFailureCode(f.Cause)
	if _, err = tx.Exec(ctx, `INSERT INTO period_cancellation_failures(id,cancellation_id,order_id,job_version,code) VALUES($1,$2,$3,$4,$5)`, ids.New(), f.Job.ID, f.OrderID, f.Job.Version, code); err != nil {
		return err
	}
	var state string
	if err = tx.QueryRow(ctx, `SELECT state FROM period_cancellation_targets WHERE cancellation_id=$1 AND order_id=$2 FOR UPDATE`, f.Job.ID, f.OrderID).Scan(&state); err != nil {
		return err
	}
	// Another worker can finish this order after the failed transaction rolled
	// back. Retain failure history, but never roll a successful task back to failed.
	if current.State == "processing" && current.Version == f.Job.Version && state == "pending" {
		if _, err = tx.Exec(ctx, `UPDATE period_cancellation_targets SET state='failed',version=version+1,error_code=$3 WHERE cancellation_id=$1 AND order_id=$2`, f.Job.ID, f.OrderID, code); err != nil {
			return err
		}
		current, err = scanCancellation(tx.QueryRow(ctx, `UPDATE period_cancellations SET state='failed',version=version+1,last_error_code=$2 WHERE id=$1 RETURNING `+stringsWithoutAlias(cancellationFields), f.Job.ID, code), false)
		if err != nil {
			return err
		}
		current, err = scanCancellation(tx.QueryRow(ctx, `SELECT `+cancellationFields+cancellationCounts+` FROM period_cancellations c WHERE c.id=$1`, f.Job.ID), true)
		if err != nil {
			return err
		}
		if err = cancellationEvent(ctx, tx, current, "period.cancellation.failed"); err != nil {
			return err
		}
	}
	_, err = audit.Append(ctx, tx, audit.Record{BrandID: f.Job.BrandID, ActorType: "system", Action: "period.cancel_refund_failed", ResourceType: "period_cancellation", ResourceID: f.Job.ID, Reason: "refund transaction failed; operator review required", RequestID: "period-cancel:" + f.Job.ID, After: map[string]any{"order_id": f.OrderID, "error_code": code, "observed_version": f.Job.Version, "current_state": current.State}})
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
