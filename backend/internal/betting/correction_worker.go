package betting

import (
	"context"
	"errors"
	"fmt"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"time"
)

type correctionFailure struct {
	Correction Correction
	OrderID    string
	Cause      error
}

func (s Service) ProcessCorrections(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 100 {
		return 0, ErrInvalid
	}
	n := 0
	var failures error
	for n < limit {
		rows, e := s.DB.Query(ctx, `SELECT id::text,brand_id::text,period_id::text FROM draw_corrections WHERE state='reversing' ORDER BY created_at,id LIMIT $1`, limit)
		if e != nil {
			return n, errors.Join(failures, e)
		}
		var candidates [][3]string
		for rows.Next() {
			var v [3]string
			if e = rows.Scan(&v[0], &v[1], &v[2]); e != nil {
				rows.Close()
				return n, e
			}
			candidates = append(candidates, v)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return n, e
		}
		progress := false
		for _, v := range candidates {
			var pending bool
			if e = s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM draw_correction_targets WHERE correction_id=$1 AND state='pending')`, v[0]).Scan(&pending); e != nil {
				return n, e
			}
			var done bool
			var f *correctionFailure
			if pending {
				done, f, e = s.reverseCorrectionTarget(ctx, v[0], v[1], v[2])
			} else {
				done, f, e = s.publishCorrection(ctx, v[0], v[1], v[2])
			}
			if e != nil {
				if f != nil && ctx.Err() == nil {
					recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
					e = errors.Join(e, s.recordCorrectionFailure(recordCtx, *f))
					cancel()
				}
				failures = errors.Join(failures, fmt.Errorf("correction %s: %w", v[0], e))
			}
			if done {
				n++
				progress = true
			}
			if n >= limit {
				break
			}
		}
		if !progress {
			break
		}
	}
	return n, failures
}
func (s Service) reverseCorrectionTarget(ctx context.Context, id, brand, period string) (done bool, failure *correctionFailure, err error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)
	if err = lockSettlementPeriod(ctx, tx, brand, period, true); errors.Is(err, pgx.ErrNoRows) {
		return false, nil, nil
	}
	if err != nil {
		return
	}
	c, e := lockCorrection(ctx, tx, brand, id)
	if e != nil {
		return false, nil, e
	}
	if c.State != "reversing" {
		return false, nil, nil
	}
	var t CorrectionTarget
	var member string
	err = tx.QueryRow(ctx, `SELECT t.order_id::text,o.brand_member_id::text,t.old_order_version,t.old_order_status,t.old_calculation_id::text,t.old_payout_entry_id::text,t.old_prize_points FROM draw_correction_targets t JOIN bet_orders o ON o.id=t.order_id WHERE t.correction_id=$1 AND t.state='pending' ORDER BY t.order_id LIMIT 1 FOR UPDATE OF t`, id).Scan(&t.OrderID, &member, &t.OldOrderVersion, &t.OldOrderStatus, &t.OldCalculationID, &t.OldPayoutEntryID, &t.OldPrizePoints)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil, nil
	}
	if err != nil {
		return
	}
	failure = &correctionFailure{Correction: c, OrderID: t.OrderID}
	defer func() {
		if err != nil {
			failure.Cause = err
		} else {
			failure = nil
		}
	}()
	ps := points.Store{DB: s.DB}
	if t.OldPrizePoints > 0 {
		if _, err = ps.LockedSnapshot(ctx, tx, brand, member); err != nil {
			return
		}
	}
	o, err := scanOrder(tx.QueryRow(ctx, `SELECT `+orderFields+` FROM bet_orders WHERE brand_id=$1 AND id=$2 FOR UPDATE`, brand, t.OrderID))
	if err != nil {
		return
	}
	if o.Status == "abnormal" || o.Status == "bet_cancelled" || o.Status == "judged_cancelled" {
		_, err = tx.Exec(ctx, `UPDATE draw_correction_targets SET state='excluded',version=version+1 WHERE correction_id=$1 AND order_id=$2`, id, o.ID)
	} else if o.Status == "placed" && t.OldOrderStatus == "placed" && o.Version == t.OldOrderVersion {
		_, err = tx.Exec(ctx, `UPDATE draw_correction_targets SET state='unchanged',version=version+1 WHERE correction_id=$1 AND order_id=$2`, id, o.ID)
	} else if (o.Status == "won" || o.Status == "lost") && o.Status == t.OldOrderStatus && o.Version == t.OldOrderVersion && o.PrizePoints == t.OldPrizePoints {
		entryID := ""
		if t.OldPrizePoints > 0 {
			if t.OldPayoutEntryID == nil || o.PayoutEntryID == nil || *t.OldPayoutEntryID != *o.PayoutEntryID {
				return false, failure, points.ErrCorrupt
			}
			original, e := ps.Entry(ctx, tx, brand, member, *t.OldPayoutEntryID)
			if e != nil {
				return false, failure, e
			}
			if original.EntryType != "prize" || original.ReferenceType != "settlement_calculation" || o.SettlementCalculationID == nil || original.ReferenceID != *o.SettlementCalculationID {
				return false, failure, points.ErrCorrupt
			}
			delta, e := points.Negate(original.Delta)
			if e != nil {
				return false, failure, e
			}
			var entry points.Entry
			entry, err = ps.Post(ctx, tx, points.Change{BrandID: brand, MemberID: member, EntryType: "prize_reversal", ReferenceType: "draw_correction", ReferenceID: id, OperationKey: "draw-correction:" + id + ":" + o.ID, Reason: "corrected lottery result reverses original prize", ActorType: "system", RequestID: "correction:" + id, Delta: delta, Allocation: original.Allocation, ReversalOf: original.ID})
			if err != nil {
				return
			}
			entryID = entry.ID
		}
		_, err = tx.Exec(ctx, `UPDATE draw_correction_targets SET state='reversed',version=version+1,reversal_entry_id=NULLIF($3,'')::uuid,reset_order_version=$4 WHERE correction_id=$1 AND order_id=$2`, id, o.ID, entryID, o.Version+1)
		if err != nil {
			return
		}
		before := o
		o, err = scanOrder(tx.QueryRow(ctx, `UPDATE bet_orders SET status='placed',version=version+1,settlement_calculation_id=NULL,payout_entry_id=NULL,prize_points=0,settled_at=NULL WHERE id=$1 RETURNING `+orderFields, o.ID))
		if err != nil {
			return
		}
		if err = appendEvent(ctx, tx, o, "bet.order.settlement_reversed"); err != nil {
			return
		}
		_, err = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "system", Action: "draw.correction.reverse", ResourceType: "bet_order", ResourceID: o.ID, Reason: "original prize reversed before new calculation", RequestID: "correction:" + id, Before: map[string]any{"version": before.Version, "status": before.Status, "calculation_id": before.SettlementCalculationID, "payout_entry_id": before.PayoutEntryID, "prize_points": before.PrizePoints}, After: map[string]any{"correction_id": id, "version": o.Version, "status": o.Status, "reversal_entry_id": entryID}})
	} else {
		return false, failure, ErrVersion
	}
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE draw_corrections SET version=version+1 WHERE id=$1`, id)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	return err == nil, failure, err
}
func (s Service) publishCorrection(ctx context.Context, id, brand, period string) (done bool, failure *correctionFailure, err error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)
	if err = lockCorrectionPeriod(ctx, tx, brand, period); err != nil {
		return
	}
	c, e := lockCorrection(ctx, tx, brand, id)
	if e != nil {
		return false, nil, e
	}
	if c.State != "reversing" {
		return false, nil, nil
	}
	var unresolved bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM draw_correction_targets WHERE correction_id=$1 AND state IN ('pending','failed'))`, id).Scan(&unresolved); err != nil {
		return
	}
	if unresolved {
		return false, nil, nil
	}
	failure = &correctionFailure{Correction: c}
	defer func() {
		if err != nil {
			failure.Cause = err
		} else {
			failure = nil
		}
	}()
	if c.PreviousJobID == nil || c.PolicyVersion == nil || c.Mode == nil {
		return false, failure, ErrState
	}
	var pv, gen int64
	var oldPointer string
	if err = tx.QueryRow(ctx, `SELECT p.version,p.current_settlement_job_id::text,j.generation FROM periods p JOIN settlement_jobs j ON j.id=p.current_settlement_job_id WHERE p.id=$1`, period).Scan(&pv, &oldPointer, &gen); err != nil {
		return
	}
	if oldPointer != *c.PreviousJobID || pv != c.PeriodVersion {
		return false, failure, ErrVersion
	}
	newID := ids.New()
	var count int64
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM bet_orders WHERE brand_id=$1 AND period_id=$2`, brand, period).Scan(&count); err != nil {
		return
	}
	_, err = tx.Exec(ctx, `INSERT INTO settlement_jobs(id,brand_id,game_id,period_id,draw_result_id,period_version,policy_version,mode,target_count,created_by,reason,generation,previous_job_id,correction_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, newID, brand, c.GameID, period, c.DrawResultID, pv+1, *c.PolicyVersion, *c.Mode, count, c.CreatedBy, c.Reason, gen+1, oldPointer, id)
	if err != nil {
		return
	}
	_, err = tx.Exec(ctx, `INSERT INTO settlement_targets(brand_id,job_id,period_id,order_id,state) SELECT brand_id,$1,period_id,id,CASE WHEN status='placed' THEN 'pending' ELSE 'excluded' END FROM bet_orders WHERE brand_id=$2 AND period_id=$3`, newID, brand, period)
	if err != nil {
		return
	}
	_, err = tx.Exec(ctx, `UPDATE draw_corrections SET new_job_id=$2,state='resettling',version=version+1 WHERE id=$1`, id, newID)
	if err != nil {
		return
	}
	_, err = tx.Exec(ctx, `UPDATE periods SET current_settlement_job_id=$2,draw_result_id=$3,version=version+1,state_reason='corrected result published after original reversals' WHERE id=$1`, period, newID, c.DrawResultID)
	if err != nil {
		return
	}
	c.State = "resettling"
	c.Version++
	c.NewJobID = &newID
	if err = correctionEvent(ctx, tx, c, "draw.correction.published"); err != nil {
		return
	}
	_, err = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "system", Action: "draw.correction.publish", ResourceType: "draw_correction", ResourceID: id, Reason: "all original targets reversed or excluded", RequestID: "correction:" + id, After: map[string]any{"draw_result_id": c.DrawResultID, "new_job_id": newID, "generation": gen + 1, "policy_version": c.PolicyVersion}})
	if err == nil {
		err = tx.Commit(ctx)
	}
	return err == nil, failure, err
}
func correctionFailureCode(e error) string {
	if errors.Is(e, points.ErrInsufficient) {
		return "WINNING_AVAILABLE_INSUFFICIENT"
	}
	if errors.Is(e, points.ErrCorrupt) {
		return "LEDGER_CORRUPT"
	}
	if errors.Is(e, ErrVersion) {
		return "ORDER_VERSION_CHANGED"
	}
	return "CORRECTION_STORAGE_FAILED"
}
func (s Service) recordCorrectionFailure(ctx context.Context, f correctionFailure) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if e = lockSettlementPeriod(ctx, tx, f.Correction.BrandID, f.Correction.PeriodID, false); e != nil {
		return e
	}
	c, e := lockCorrection(ctx, tx, f.Correction.BrandID, f.Correction.ID)
	if e != nil {
		return e
	}
	if c.Version != f.Correction.Version || c.State != "reversing" {
		return nil
	}
	code := correctionFailureCode(f.Cause)
	_, e = tx.Exec(ctx, `INSERT INTO draw_correction_failures(id,brand_id,correction_id,order_id,correction_version,error_code) VALUES($1,$2,$3,NULLIF($4,'')::uuid,$5,$6)`, ids.New(), c.BrandID, c.ID, f.OrderID, c.Version, code)
	if e != nil {
		return e
	}
	if f.OrderID != "" {
		_, e = tx.Exec(ctx, `UPDATE draw_correction_targets SET state='failed',version=version+1,error_code=$3 WHERE correction_id=$1 AND order_id=$2`, c.ID, f.OrderID, code)
		if e != nil {
			return e
		}
	}
	_, e = tx.Exec(ctx, `UPDATE draw_corrections SET state='failed',version=version+1,last_error_code=$2 WHERE id=$1`, c.ID, code)
	if e != nil {
		return e
	}
	c.State = "failed"
	c.Version++
	if e = correctionEvent(ctx, tx, c, "draw.correction.failed"); e != nil {
		return e
	}
	_, e = audit.Append(ctx, tx, audit.Record{BrandID: c.BrandID, ActorType: "system", Action: "draw.correction.failed", ResourceType: "draw_correction", ResourceID: c.ID, Reason: code, RequestID: "correction:" + c.ID, After: map[string]any{"order_id": f.OrderID, "code": code}})
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func completeChildCorrection(ctx context.Context, tx pgx.Tx, j SettlementJob) error {
	if j.CorrectionID == nil {
		return nil
	}
	_, e := tx.Exec(ctx, `UPDATE draw_corrections SET state='completed',completed_at=clock_timestamp(),version=version+1 WHERE id=$1 AND new_job_id=$2 AND state='resettling'`, *j.CorrectionID, j.ID)
	if e != nil {
		return e
	}
	c, e := scanCorrection(tx.QueryRow(ctx, `SELECT `+correctionFields+correctionJoin+` WHERE c.id=$1`, *j.CorrectionID), false)
	if e != nil {
		return e
	}
	if e = correctionEvent(ctx, tx, c, "draw.correction.completed"); e != nil {
		return e
	}
	_, e = audit.Append(ctx, tx, audit.Record{BrandID: j.BrandID, ActorType: "system", Action: "draw.correction.completed", ResourceType: "draw_correction", ResourceID: c.ID, Reason: "new settlement generation completed", RequestID: "correction:" + c.ID, After: map[string]any{"new_job_id": j.ID, "generation": j.Generation}})
	return e
}
