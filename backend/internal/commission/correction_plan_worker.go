package commission

import (
	"context"
	"errors"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

const correctionPlanCandidateSQL = `SELECT c.brand_id::text,c.id::text,pay.id::text,p.id::text
 FROM commission_cycles c JOIN commission_payments pay ON pay.cycle_id=c.id AND pay.state='blocked' AND pay.last_error_code='COMMISSION_PAYMENT_CORRECTION_REQUIRED'
 JOIN brand_commission_payment_policies gate ON gate.brand_id=c.brand_id JOIN brands b ON b.id=c.brand_id
 LEFT JOIN commission_correction_plans p ON p.cycle_id=c.id AND p.state<>'stale'
 WHERE ($1::uuid IS NULL OR c.id=$1) AND (
  (p.id IS NOT NULL AND (p.evidence_epoch<>c.evidence_epoch OR p.run_id IS DISTINCT FROM c.current_run_id OR c.state<>'ready'))
  OR (gate.enabled AND b.status<>'disabled' AND ((p.state='planning' AND p.next_work_at<=clock_timestamp()) OR
   (p.id IS NULL AND c.state='ready' AND NOT EXISTS(SELECT 1 FROM commission_correction_plans old WHERE old.cycle_id=c.id AND old.run_id=c.current_run_id)))))
 ORDER BY coalesce(p.next_work_at,c.updated_at),c.id FOR UPDATE OF c SKIP LOCKED LIMIT 1`

// ProcessCorrectionPlans prepares bounded, immutable difference pages only.
// It never touches a wallet, approves compensation, or unblocks old payouts.
func (s Service) ProcessCorrectionPlans(ctx context.Context, maxSteps int) (int, error) {
	if s.DB == nil || maxSteps < 1 || maxSteps > 100 {
		return 0, ErrInvalid
	}
	committed := 0
	for i := 0; i < maxSteps; i++ {
		// The post-claim statement must see plans committed after the candidate
		// query snapshot, even when the pool's default isolation is stronger.
		tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		if err != nil {
			return committed, err
		}
		var brand, cycle, payment string
		var planID *string
		err = tx.QueryRow(ctx, correctionPlanCandidateSQL, nil).Scan(&brand, &cycle, &payment, &planID)
		if errors.Is(err, pgx.ErrNoRows) {
			_ = tx.Rollback(ctx)
			break
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return committed, paymentDBError(err)
		}
		// SKIP LOCKED claims c, not the left-joined plan. A plan can commit
		// after the candidate snapshot while c remains unchanged, leaving
		// planID stale (including NULL). Recheck under the cycle lock in a new
		// READ COMMITTED statement before creating or advancing a plan.
		err = tx.QueryRow(ctx, correctionPlanCandidateSQL, cycle).Scan(&brand, &cycle, &payment, &planID)
		if errors.Is(err, pgx.ErrNoRows) {
			_ = tx.Rollback(ctx)
			continue
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return committed, paymentDBError(err)
		}
		c, err := cycleLock(ctx, tx, brand, cycle)
		var p correctionPlanRow
		if err == nil && planID != nil {
			p, err = lockCorrectionPlan(ctx, tx, brand, *planID)
		}
		if err == nil {
			err = s.correctionPlanWork(ctx, tx, c, p, payment)
		}
		if err == nil {
			err = tx.Commit(ctx)
		} else {
			_ = tx.Rollback(ctx)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			err = paymentDBError(err)
			if errors.Is(err, ErrBusy) {
				if p.ID != "" {
					if _, e := s.DB.Exec(ctx, `UPDATE commission_correction_plans SET next_work_at=clock_timestamp()+interval '1 second' WHERE id=$1 AND version=$2`, p.ID, p.Version); e != nil {
						return committed, e
					}
				}
				continue
			}
			if p.ID == "" || p.State != "planning" || ctx.Err() != nil {
				return committed, err
			}
			if e := s.failCorrectionPlan(ctx, p); e != nil {
				return committed, e
			}
		}
		committed++
	}
	return committed, nil
}

func (s Service) correctionPlanWork(ctx context.Context, tx pgx.Tx, c cycleRow, p correctionPlanRow, payment string) error {
	meta := points.Metadata{ActorType: "system", RequestID: ids.New()}
	if p.ID != "" {
		valid, err := correctionPlanEvidence(ctx, tx, p)
		if err != nil {
			return err
		}
		if !valid {
			log, err := correctionPlanStep(ctx, tx, p, "stale", "invalidate", "replacement evidence changed; preserve the old difference plan without financial execution", p.Planned, p.Cursor, nil, meta)
			if err != nil {
				return err
			}
			return saveCorrectionPlan(ctx, tx, p, "stale", p.Planned, p.Cursor, nil, log)
		}
	}
	if err := paymentBrand(ctx, tx, c.Brand); err != nil {
		return err
	}
	gate, err := lockPaymentPolicy(ctx, tx, c.Brand)
	if err != nil {
		return err
	}
	if !gate.Enabled {
		return ErrBusy
	}
	if p.ID == "" {
		return createCorrectionPlan(ctx, tx, c, payment, meta)
	}
	if p.State != "planning" {
		return ErrCorrectionPlanState
	}
	return pageCorrectionPlan(ctx, tx, p, meta)
}

func createCorrectionPlan(ctx context.Context, tx pgx.Tx, c cycleRow, payment string, meta points.Metadata) error {
	if c.RunID == nil || c.State != "ready" {
		return ErrCorrectionPlanEvidence
	}
	original, err := lockPayment(ctx, tx, c.Brand, payment)
	if err != nil {
		return err
	}
	var sourceValid, current bool
	var before, calculated, credit, debit, net, currentMode string
	var count int64
	err = tx.QueryRow(ctx, `SELECT commission_correction_source_valid($1,$2,$3),commission_payment_evidence_current($1,$4,$3,$5),
 coalesce(sum(points_before::numeric),0)::text,coalesce(sum(points_after::numeric),0)::text,
 coalesce(sum(greatest(delta_points,0)::numeric),0)::text,coalesce(sum(greatest(-delta_points,0)::numeric),0)::text,
 coalesce(sum(delta_points::numeric),0)::text,count(*),commission_payment_mode($3)
 FROM commission_correction_candidates($1,$2,$3)`, c.Brand, payment, *c.RunID, c.ID, c.Epoch).Scan(&sourceValid, &current, &before, &calculated, &credit, &debit, &net, &count, &currentMode)
	if err != nil {
		return err
	}
	if !sourceValid || !current {
		return ErrCorrectionPlanEvidence
	}
	if count > 100000 {
		return ErrCorrectionPlanLimit
	}
	mode, err := MergeCorrectionMode(original.Mode, currentMode)
	if err != nil {
		return ErrCorrectionPlanEvidence
	}
	state := "planning"
	var code *string
	var credits, debits, netPoints *string = &credit, &debit, &net
	// A new draw generation supersedes the manual target, not its audit history.
	// Candidates use the actual granted net as before and the new calculation
	// as after: 10 -> manually 12 -> recalculated 8 means a debit of 4.
	p := correctionPlanRow{ID: ids.New(), Brand: c.Brand, Cycle: c.ID, Payment: payment, Run: *c.RunID, State: state, Version: 1, Epoch: c.Epoch, Count: count}
	const reason = "freeze actual prior commission and replacement calculation; preparation does not move points"
	log, err := correctionPlanStep(ctx, tx, p, state, "create", reason, 0, nil, code, meta)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO commission_correction_plans(id,brand_id,cycle_id,payment_id,run_id,evidence_epoch,payout_mode,state,before_points,calculated_points,credit_points,debit_points,net_points,target_count,creation_audit_log_id,last_audit_log_id,last_error_code)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$15,$16)`, p.ID, p.Brand, p.Cycle, p.Payment, p.Run, p.Epoch, mode, state, before, calculated, credits, debits, netPoints, count, log, code)
	if err != nil {
		return paymentDBError(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO commission_correction_plan_steps(id,brand_id,plan_id,version,to_state,operation,planned_count,error_code,actor_type,reason,request_id,audit_log_id)
 VALUES($1,$2,$3,1,$4,'create',0,$5,'system',$6,$7,$8)`, ids.New(), p.Brand, p.ID, state, code, reason, meta.RequestID, log)
	return paymentDBError(err)
}

func pageCorrectionPlan(ctx context.Context, tx pgx.Tx, p correctionPlanRow, meta points.Metadata) error {
	rows, err := tx.Query(ctx, `SELECT agent_id::text,member_id::text,original_target_id::text,earning_id::text,adjustment_version,points_before,points_after,delta_points,previous_correction_target_id::text,financial_version
 FROM commission_correction_candidates_v2($1,$2,$3) WHERE ($4::uuid IS NULL OR agent_id>$4) ORDER BY agent_id LIMIT 100`, p.Brand, p.Payment, p.Run, p.Cursor)
	if err != nil {
		return err
	}
	var targets []CorrectionPlanTarget
	for rows.Next() {
		var t CorrectionPlanTarget
		if err = rows.Scan(&t.AgentID, &t.MemberID, &t.OriginalTargetID, &t.EarningID, &t.AdjustmentVersion, &t.PointsBefore, &t.PointsAfter, &t.DeltaPoints, &t.PreviousCorrectionTargetID, &t.FinancialVersion); err != nil {
			rows.Close()
			return err
		}
		if delta, e := ComputeCorrectionDelta(t.PointsBefore, t.PointsAfter); e != nil || delta != t.DeltaPoints {
			rows.Close()
			return ErrCorrectionPlanEvidence
		}
		targets = append(targets, t)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	if len(targets) == 0 {
		if p.Planned != p.Count {
			return ErrCorrectionPlanEvidence
		}
		log, err := correctionPlanStep(ctx, tx, p, "ready", "ready", "all immutable per-agent differences verified; this is not compensation authorization or payment", p.Planned, p.Cursor, nil, meta)
		if err != nil {
			return err
		}
		return saveCorrectionPlan(ctx, tx, p, "ready", p.Planned, p.Cursor, nil, log)
	}
	planned := p.Planned + int64(len(targets))
	if planned > p.Count {
		return ErrCorrectionPlanEvidence
	}
	cursor := targets[len(targets)-1].AgentID
	log, err := correctionPlanStep(ctx, tx, p, "planning", "page", "prepare bounded immutable differences without ledger or notification postings", planned, &cursor, nil, meta)
	if err != nil {
		return err
	}
	for _, t := range targets {
		_, err = tx.Exec(ctx, `INSERT INTO commission_correction_plan_targets(id,brand_id,plan_id,agent_id,member_id,original_target_id,earning_id,adjustment_version,points_before,points_after,delta_points,plan_version,creation_audit_log_id,previous_correction_target_id,financial_version)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, ids.New(), p.Brand, p.ID, t.AgentID, t.MemberID, t.OriginalTargetID, t.EarningID, t.AdjustmentVersion, t.PointsBefore, t.PointsAfter, t.DeltaPoints, p.Version+1, log, t.PreviousCorrectionTargetID, t.FinancialVersion)
		if err != nil {
			return paymentDBError(err)
		}
	}
	return saveCorrectionPlan(ctx, tx, p, "planning", planned, &cursor, nil, log)
}

func (s Service) failCorrectionPlan(ctx context.Context, observed correctionPlanRow) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	p, err := lockCorrectionPlan(ctx, tx, observed.Brand, observed.ID)
	if err != nil {
		return err
	}
	if p.Version != observed.Version || p.State != "planning" {
		return nil
	}
	valid, err := correctionPlanEvidence(ctx, tx, p)
	if err != nil {
		return err
	}
	state, operation, reason := "failed", "fail", "difference preparation failed; explicit operator retry required"
	v := "COMMISSION_CORRECTION_PLAN_FAILED"
	var code *string = &v
	if !valid {
		state, operation, reason, code = "stale", "invalidate", "replacement evidence changed before failure recording", nil
	}
	log, err := correctionPlanStep(ctx, tx, p, state, operation, reason, p.Planned, p.Cursor, code, points.Metadata{ActorType: "system", RequestID: ids.New()})
	if err != nil {
		return err
	}
	if err = saveCorrectionPlan(ctx, tx, p, state, p.Planned, p.Cursor, code, log); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
