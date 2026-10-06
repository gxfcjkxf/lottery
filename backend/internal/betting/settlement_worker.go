package betting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

type settlementFailure struct {
	Job     SettlementJob
	OrderID string
	Cause   error
}

// Each iteration commits one target or a phase transition. Wallets are never
// locked across a batch. SKIP LOCKED and durable targets support multiple workers.
func (s Service) ProcessSettlements(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 100 {
		return 0, ErrInvalid
	}
	n := 0
	var failures error
	for n < limit {
		rows, e := s.DB.Query(ctx, `SELECT id::text,brand_id::text,period_id::text FROM settlement_jobs WHERE state IN ('processing','paying') ORDER BY created_at,id LIMIT $1`, limit)
		if e != nil {
			return n, errors.Join(failures, e)
		}
		var jobs [][3]string
		for rows.Next() {
			var v [3]string
			if e = rows.Scan(&v[0], &v[1], &v[2]); e != nil {
				rows.Close()
				return n, e
			}
			jobs = append(jobs, v)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return n, e
		}
		progress := false
		for _, v := range jobs {
			done, f, e := s.processSettlement(ctx, v[0], v[1], v[2])
			if e != nil {
				if f != nil && ctx.Err() == nil {
					recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
					e = errors.Join(e, s.recordSettlementFailure(recordCtx, *f))
					cancel()
				}
				failures = errors.Join(failures, fmt.Errorf("settlement %s: %w", v[0], e))
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

// Take the period exclusively from the outset, not by upgrading a shared lock.
// This avoids a deadlock with cancellation's period -> job -> wallet ordering.
func lockSettlementPeriod(ctx context.Context, tx pgx.Tx, brand, period string, skip bool) error {
	var game string
	e := tx.QueryRow(ctx, `SELECT game_id::text FROM periods WHERE brand_id=$1 AND id=$2`, brand, period).Scan(&game)
	if e != nil {
		return e
	}
	if e = tx.QueryRow(ctx, `SELECT id::text FROM games WHERE brand_id=$1 AND id=$2 FOR SHARE`, brand, game).Scan(&game); e != nil {
		return e
	}
	clause := ""
	if skip {
		clause = " SKIP LOCKED"
	}
	return tx.QueryRow(ctx, `SELECT id::text FROM periods WHERE brand_id=$1 AND id=$2 FOR UPDATE`+clause, brand, period).Scan(&period)
}
func (s Service) processSettlement(ctx context.Context, id, brand, period string) (done bool, failure *settlementFailure, err error) {
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
	j, e := scanSettlementJob(tx.QueryRow(ctx, `SELECT `+settlementJobFields+` FROM settlement_jobs j WHERE id=$1 AND state IN ('processing','paying') FOR UPDATE SKIP LOCKED`, id), false)
	if errors.Is(e, ErrNotFound) {
		return false, nil, nil
	}
	if e != nil {
		return false, nil, e
	}
	defer func() {
		if failure != nil {
			if err != nil {
				failure.Cause = err
			} else {
				failure = nil
			}
		}
	}()
	var valid bool
	if err = tx.QueryRow(ctx, `SELECT status='settling' AND draw_result_id=$2 FROM periods WHERE id=$1`, period, j.DrawResultID).Scan(&valid); err != nil {
		return
	}
	if !valid {
		return false, nil, ErrState
	}
	targetState := "pending"
	if j.State == "paying" {
		targetState = "ready"
	}
	var orderID, member string
	err = tx.QueryRow(ctx, `SELECT t.order_id::text,o.brand_member_id::text FROM settlement_targets t JOIN bet_orders o ON o.id=t.order_id WHERE t.job_id=$1 AND t.state=$2 ORDER BY t.order_id LIMIT 1 FOR UPDATE OF t`, id, targetState).Scan(&orderID, &member)
	if errors.Is(err, pgx.ErrNoRows) {
		failure = &settlementFailure{Job: j}
		if j.State == "processing" {
			state := "paying"
			if j.Mode == "manual" {
				state = "awaiting_approval"
			}
			_, err = tx.Exec(ctx, `UPDATE settlement_jobs SET state=$2,version=version+1 WHERE id=$1`, id, state)
			j.State = state
			j.Version++
		} else {
			_, err = tx.Exec(ctx, `UPDATE settlement_jobs SET state='completed',completed_at=clock_timestamp(),version=version+1 WHERE id=$1`, id)
			if err == nil {
				_, err = tx.Exec(ctx, `UPDATE periods SET status='settled',version=version+1,state_reason='all settlement targets terminal' WHERE id=$1`, period)
			}
			j.State = "completed"
			j.Version++
		}
		if err == nil {
			err = settlementJobEvent(ctx, tx, j, "settlement."+j.State)
		}
		if err == nil {
			_, err = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "system", Action: "settlement." + j.State, ResourceType: "settlement_job", ResourceID: id, Reason: "durable target phase completed", RequestID: "settlement:" + id, After: map[string]any{"state": j.State, "version": j.Version}})
		}
		if err == nil {
			err = tx.Commit(ctx)
		}
		return err == nil, failure, err
	}
	if err != nil {
		return
	}
	failure = &settlementFailure{Job: j, OrderID: orderID}
	ps := points.Store{DB: s.DB}
	if j.State == "paying" {
		if _, err = ps.LockedSnapshot(ctx, tx, brand, member); err != nil {
			return
		}
	}
	o, err := scanOrder(tx.QueryRow(ctx, `SELECT `+orderFields+` FROM bet_orders WHERE brand_id=$1 AND id=$2 FOR UPDATE`, brand, orderID))
	if err != nil && !errors.Is(err, ErrSnapshot) {
		return
	}
	if errors.Is(err, ErrSnapshot) {
		// Fetch immutable identity independently of the malformed JSON snapshot.
		err = tx.QueryRow(ctx, `SELECT id::text,brand_id::text,period_id::text,brand_member_id::text,status,version FROM bet_orders WHERE id=$1`, orderID).Scan(&o.ID, &o.BrandID, &o.PeriodID, &o.MemberID, &o.Status, &o.Version)
		if err != nil {
			return
		}
		if o.Status != "placed" {
			return false, failure, ErrState
		}
		err = s.classifySettlementAnomaly(ctx, tx, j, o, "ORDER_SNAPSHOT_INVALID")
		if err == nil {
			err = tx.Commit(ctx)
		}
		return err == nil, failure, err
	}
	if o.Status != "placed" {
		if o.Status != "abnormal" && o.Status != "bet_cancelled" && o.Status != "judged_cancelled" {
			return false, failure, ErrState
		}
		err = excludeSettlementTarget(ctx, tx, &j, o.ID)
		if err == nil {
			err = tx.Commit(ctx)
		}
		return err == nil, failure, err
	}
	if j.State == "processing" {
		var raw []byte
		var hash string
		err = tx.QueryRow(ctx, `SELECT result,result_hash FROM draw_results WHERE id=$1`, j.DrawResultID).Scan(&raw, &hash)
		if err != nil {
			return
		}
		var draw rules.Draw
		if err = json.Unmarshal(raw, &draw); err != nil {
			return
		}
		var sim rules.Simulation
		var issue string
		if digest(draw) != hash {
			issue = "DRAW_HASH_MISMATCH"
		} else {
			sim, issue, err = evaluateSettlementSnapshot(ctx, o, draw)
		}
		if err != nil {
			return
		}
		if issue == "" {
			var valid bool
			valid, err = verifySettlementDebit(ctx, tx, o)
			if err != nil {
				return
			}
			if !valid {
				issue = "DEBIT_SNAPSHOT_MISMATCH"
			}
		}
		if issue != "" {
			err = s.classifySettlementAnomaly(ctx, tx, j, o, issue)
		} else {
			calcID := ids.New()
			var blob []byte
			blob, err = json.Marshal(sim)
			if err != nil {
				return
			}
			_, err = tx.Exec(ctx, `INSERT INTO settlement_calculations(id,brand_id,job_id,period_id,order_id,order_version,definition_hash,draw_hash,won,prize_points,calculation) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, calcID, brand, j.ID, period, o.ID, o.Version, o.DefinitionHash, hash, sim.Won, sim.PrizePoints, blob)
			if err == nil {
				_, err = tx.Exec(ctx, `UPDATE settlement_targets SET state='ready',version=version+1,calculation_id=$3 WHERE job_id=$1 AND order_id=$2`, j.ID, o.ID, calcID)
			}
			if err == nil {
				_, err = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "system", Action: "settlement.calculate", ResourceType: "settlement_calculation", ResourceID: calcID, Reason: "purchased snapshot calculated", RequestID: "settlement:" + j.ID, After: map[string]any{"job_id": j.ID, "order_id": o.ID, "won": sim.Won, "prize_points": sim.PrizePoints}})
			}
		}
	} else {
		var calc string
		var won bool
		var prize points.Amount
		var ov int64
		err = tx.QueryRow(ctx, `SELECT c.id::text,c.won,c.prize_points,c.order_version FROM settlement_calculations c JOIN settlement_targets t ON t.calculation_id=c.id WHERE t.job_id=$1 AND t.order_id=$2`, j.ID, o.ID).Scan(&calc, &won, &prize, &ov)
		if err != nil {
			return
		}
		if ov != o.Version {
			return false, failure, ErrVersion
		}
		entryID := ""
		if prize > 0 {
			var delta points.Balance
			delta[1][0] = prize
			var entry points.Entry
			entry, err = ps.Post(ctx, tx, points.Change{BrandID: brand, MemberID: o.MemberID, EntryType: "prize", ReferenceType: "settlement_calculation", ReferenceID: calc, OperationKey: "settlement-payout:" + calc, Reason: "lottery winnings", ActorType: "system", RequestID: "settlement:" + j.ID, Delta: delta, Allocation: []points.Allocation{{Source: "winning", State: "available", Points: prize}}})
			if err != nil {
				return
			}
			entryID = entry.ID
		}
		status := "lost"
		if won {
			status = "won"
		}
		o, err = scanOrder(tx.QueryRow(ctx, `UPDATE bet_orders SET status=$3,version=version+1,settlement_calculation_id=$4,payout_entry_id=NULLIF($5,'')::uuid,prize_points=$6,settled_at=clock_timestamp() WHERE brand_id=$1 AND id=$2 RETURNING `+orderFields, brand, o.ID, status, calc, entryID, prize))
		if err != nil {
			return
		}
		_, err = tx.Exec(ctx, `UPDATE settlement_targets SET state='paid',version=version+1,payout_entry_id=NULLIF($3,'')::uuid WHERE job_id=$1 AND order_id=$2`, j.ID, o.ID, entryID)
		if err == nil {
			err = appendEvent(ctx, tx, o, "bet.order.settled")
		}
		if err == nil {
			_, err = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "system", Action: "settlement.payout", ResourceType: "bet_order", ResourceID: o.ID, Reason: "integer winning points applied", RequestID: "settlement:" + j.ID, After: map[string]any{"job_id": j.ID, "calculation_id": calc, "prize_points": prize, "payout_entry_id": entryID, "status": status}})
		}
	}
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE settlement_jobs SET version=version+1 WHERE id=$1`, j.ID)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	return err == nil, failure, err
}

func (s Service) classifySettlementAnomaly(ctx context.Context, tx pgx.Tx, j SettlementJob, o Order, code string) error {
	_, e := tx.Exec(ctx, `INSERT INTO bet_order_exceptions(id,brand_id,order_id,order_version,marked_by,source,job_id,error_code,reason) VALUES($1,$2,$3,$4,NULL,'system',$5,$6,$7)`, ids.New(), o.BrandID, o.ID, o.Version+1, j.ID, code, "settlement snapshot anomaly: "+code)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE bet_orders SET status='abnormal',version=version+1 WHERE id=$1`, o.ID)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE settlement_targets SET state='excluded',version=version+1,error_code=$3 WHERE job_id=$1 AND order_id=$2`, j.ID, o.ID, code)
	if e != nil {
		return e
	}
	o.Status = "abnormal"
	o.Version++
	if e = appendEvent(ctx, tx, o, "bet.order.abnormal"); e != nil {
		return e
	}
	_, e = audit.Append(ctx, tx, audit.Record{BrandID: j.BrandID, ActorType: "system", Action: "settlement.abnormal", ResourceType: "bet_order", ResourceID: o.ID, Reason: "whole order excluded: " + code, RequestID: "settlement:" + j.ID, After: map[string]any{"job_id": j.ID, "error_code": code, "version": o.Version}})
	return e
}
func settlementFailureCode(e error) string {
	switch {
	case errors.Is(e, points.ErrCorrupt):
		return "LEDGER_CORRUPT"
	case errors.Is(e, points.ErrOverflow):
		return "BALANCE_OVERFLOW"
	case errors.Is(e, points.ErrInsufficient):
		return "BALANCE_INSUFFICIENT"
	case errors.Is(e, ErrVersion):
		return "ORDER_VERSION_CHANGED"
	default:
		return "SETTLEMENT_STORAGE_FAILED"
	}
}
func (s Service) recordSettlementFailure(ctx context.Context, f settlementFailure) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if e = lockSettlementPeriod(ctx, tx, f.Job.BrandID, f.Job.PeriodID, false); e != nil {
		return e
	}
	j, e := lockSettlementJob(ctx, tx, f.Job.BrandID, f.Job.PeriodID)
	if e != nil {
		return e
	}
	if j == nil || j.Version != f.Job.Version || j.State != f.Job.State {
		return nil
	}
	code := settlementFailureCode(f.Cause)
	_, e = tx.Exec(ctx, `INSERT INTO settlement_failures(id,brand_id,job_id,order_id,job_version,phase,error_code) VALUES($1,$2,$3,NULLIF($4,'')::uuid,$5,$6,$7)`, ids.New(), j.BrandID, j.ID, f.OrderID, j.Version, j.State, code)
	if e != nil {
		return e
	}
	if f.OrderID != "" {
		_, e = tx.Exec(ctx, `UPDATE settlement_targets SET state='failed',error_code=$3,version=version+1 WHERE job_id=$1 AND order_id=$2`, j.ID, f.OrderID, code)
		if e != nil {
			return e
		}
	}
	_, e = tx.Exec(ctx, `UPDATE settlement_jobs SET resume_state=state,state='failed',last_error_code=$2,version=version+1 WHERE id=$1`, j.ID, code)
	if e != nil {
		return e
	}
	j.State = "failed"
	j.Version++
	if e = settlementJobEvent(ctx, tx, *j, "settlement.failed"); e != nil {
		return e
	}
	_, e = audit.Append(ctx, tx, audit.Record{BrandID: j.BrandID, ActorType: "system", Action: "settlement.failed", ResourceType: "settlement_job", ResourceID: j.ID, Reason: code, RequestID: "settlement:" + j.ID, After: map[string]any{"order_id": f.OrderID, "code": code, "version": j.Version}})
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}
