package commission

import (
	"context"
	"errors"
	"strconv"

	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

// ProcessCorrectionExecutions commits at most one beneficiary difference per
// step. Paused/failed jobs never resume on funding, a new draw, or a retry timer.
func (s Service) ProcessCorrectionExecutions(ctx context.Context, maxSteps int) (int, error) {
	if s.DB == nil || maxSteps < 1 || maxSteps > 100 {
		return 0, ErrInvalid
	}
	committed := 0
	for i := 0; i < maxSteps; i++ {
		tx, err := s.DB.Begin(ctx)
		if err != nil {
			return committed, err
		}
		var brand, cycle string
		var execution, plan *string
		err = tx.QueryRow(ctx, `SELECT c.brand_id::text,c.id::text,x.id::text,p.id::text
 FROM commission_cycles c
 LEFT JOIN commission_correction_executions x ON x.cycle_id=c.id AND x.state<>'stale'
 LEFT JOIN commission_correction_plans p ON p.cycle_id=c.id AND p.state='ready'
 JOIN brands b ON b.id=c.brand_id
 JOIN brand_commission_payment_policies oldgate ON oldgate.brand_id=c.brand_id
 JOIN brand_commission_correction_policies gate ON gate.brand_id=c.brand_id
 WHERE (x.id IS NOT NULL AND NOT commission_correction_execution_current(c.brand_id,x.id)) OR
 (gate.enabled AND oldgate.enabled AND b.status<>'disabled' AND
  ((x.state='applying' AND x.next_work_at<=clock_timestamp()) OR
   (x.id IS NULL AND p.id IS NOT NULL AND c.state='ready' AND NOT EXISTS(SELECT 1 FROM commission_correction_executions prev WHERE prev.plan_id=p.id))))
 ORDER BY coalesce(x.next_work_at,p.updated_at,c.updated_at),c.id
 FOR UPDATE OF c SKIP LOCKED LIMIT 1`).Scan(&brand, &cycle, &execution, &plan)
		if errors.Is(err, pgx.ErrNoRows) {
			_ = tx.Rollback(ctx)
			break
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return committed, paymentDBError(err)
		}
		var x correctionExecutionRow
		if execution != nil {
			x, err = lockCorrectionExecution(ctx, tx, brand, *execution)
		}
		if err == nil {
			if x.ID != "" {
				err = s.correctionExecutionWork(ctx, tx, x)
			} else if plan != nil {
				err = createCorrectionExecution(ctx, tx, brand, *plan)
			} else {
				err = ErrCorrectionExecutionEvidence
			}
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
				if x.ID != "" {
					if _, e := s.DB.Exec(ctx, `UPDATE commission_correction_executions SET next_work_at=clock_timestamp()+interval '1 second' WHERE id=$1 AND version=$2`, x.ID, x.Version); e != nil {
						return committed, e
					}
				}
				continue
			}
			if x.ID == "" || x.State != "applying" || ctx.Err() != nil {
				return committed, err
			}
			if e := s.failCorrectionExecution(ctx, x); e != nil {
				return committed, e
			}
		}
		committed++
	}
	return committed, nil
}

func createCorrectionExecution(ctx context.Context, tx pgx.Tx, brand, plan string) error {
	p, err := lockCorrectionPlan(ctx, tx, brand, plan)
	if err != nil {
		return err
	}
	if p.State != "ready" {
		return ErrCorrectionExecutionState
	}
	if err = correctionExecutionGate(ctx, tx, brand); err != nil {
		return err
	}
	x := correctionExecutionRow{ID: ids.New(), Brand: brand, Cycle: p.Cycle, Plan: p.ID, Run: p.Run, Version: 1, PlanVersion: p.Version, Epoch: p.Epoch}
	var credit, debit, net string
	err = tx.QueryRow(ctx, `SELECT payout_mode,credit_points::text,debit_points::text,net_points::text FROM commission_correction_plans WHERE brand_id=$1 AND id=$2`, brand, plan).Scan(&x.Mode, &credit, &debit, &net)
	if err != nil {
		return err
	}
	if err = correctionExecutionSources(ctx, tx, x); err != nil {
		return err
	}
	// Initialization has no financial effect, and does not clear an older hold.
	_, err = tx.Exec(ctx, `INSERT INTO commission_correction_cycle_holds(brand_id,cycle_id) VALUES($1,$2) ON CONFLICT(cycle_id) DO NOTHING`, brand, p.Cycle)
	if err != nil {
		return paymentDBError(err)
	}
	var held bool
	err = tx.QueryRow(ctx, `SELECT active FROM commission_correction_cycle_holds WHERE brand_id=$1 AND cycle_id=$2 FOR UPDATE NOWAIT`, brand, p.Cycle).Scan(&held)
	if err != nil {
		return paymentDBError(err)
	}
	x.State = "applying"
	var code *string
	if x.Mode == "manual" || x.Mode == "mixed" {
		x.State = "awaiting_approval"
	} else if held {
		x.State = "paused"
		v := "COMMISSION_CORRECTION_CYCLE_HELD"
		code = &v
	}
	const reason = "register exact current difference plan under explicit financial rollout; manual and mixed require fresh approval"
	meta := points.Metadata{ActorType: "system", RequestID: ids.New()}
	log, err := correctionExecutionStep(ctx, tx, x, x.State, "create", reason, nil, nil, code, meta)
	if err != nil {
		return err
	}
	var approvalType, approvalLog *string
	if x.State != "awaiting_approval" {
		v := "system"
		approvalType = &v
		approvalLog = &log
	}
	_, err = tx.Exec(ctx, `INSERT INTO commission_correction_executions(id,brand_id,cycle_id,plan_id,run_id,plan_version,evidence_epoch,payout_mode,state,credit_points,debit_points,net_points,target_count,approval_actor_type,approval_audit_log_id,last_error_code,creation_audit_log_id,last_audit_log_id)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$17)`, x.ID, brand, x.Cycle, x.Plan, x.Run, x.PlanVersion, x.Epoch, x.Mode, x.State, credit, debit, net, p.Count, approvalType, approvalLog, code, log)
	if err != nil {
		return paymentDBError(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO commission_correction_execution_steps(id,brand_id,execution_id,version,to_state,operation,error_code,actor_type,reason,request_id,audit_log_id)
 VALUES($1,$2,$3,1,$4,'create',$5,'system',$6,$7,$8)`, ids.New(), brand, x.ID, x.State, code, reason, meta.RequestID, log)
	return paymentDBError(err)
}

func (s Service) correctionExecutionWork(ctx context.Context, tx pgx.Tx, x correctionExecutionRow) error {
	meta := points.Metadata{ActorType: "system", RequestID: ids.New()}
	current, err := correctionExecutionEvidence(ctx, tx, x)
	if err != nil {
		return err
	}
	if !current {
		log, e := correctionExecutionStep(ctx, tx, x, "stale", "invalidate", "replacement evidence changed; preserve applied differences and the whole-cycle hold", nil, nil, nil, meta)
		if e != nil {
			return e
		}
		return saveCorrectionExecution(ctx, tx, x, "stale", nil, nil, log)
	}
	if x.State != "applying" {
		return ErrCorrectionExecutionState
	}
	if err = correctionExecutionGate(ctx, tx, x.Brand); err != nil {
		return err
	}
	if err = correctionExecutionSources(ctx, tx, x); err != nil {
		return err
	}
	var t CorrectionPlanTarget
	err = tx.QueryRow(ctx, `SELECT t.id::text,t.agent_id::text,t.member_id::text,t.points_before,t.points_after,t.delta_points,t.financial_version
 FROM commission_correction_plan_targets t WHERE t.brand_id=$1 AND t.plan_id=$2 AND NOT EXISTS
 (SELECT 1 FROM commission_correction_execution_targets done WHERE done.execution_id=$3 AND done.plan_target_id=t.id)
 ORDER BY t.agent_id LIMIT 1`, x.Brand, x.Plan, x.ID).Scan(&t.ID, &t.AgentID, &t.MemberID, &t.PointsBefore, &t.PointsAfter, &t.DeltaPoints, &t.FinancialVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		log, e := correctionExecutionStep(ctx, tx, x, "completed", "complete", "all frozen beneficiary differences committed and verified", nil, nil, nil, meta)
		if e != nil {
			return e
		}
		return saveCorrectionExecution(ctx, tx, x, "completed", nil, nil, log)
	}
	if err != nil {
		return err
	}
	if t.FinancialVersion != nil && *t.FinancialVersion >= maxCycleVersion {
		return ErrCorrectionExecutionVersion
	}
	if t.DeltaPoints != 0 {
		var account string
		err = tx.QueryRow(ctx, `SELECT id::text FROM point_accounts WHERE brand_id=$1 AND brand_member_id=$2 FOR UPDATE NOWAIT`, x.Brand, t.MemberID).Scan(&account)
		if err != nil {
			return paymentDBError(err)
		}
		wallet, e := (points.Store{DB: s.DB}).LockedSnapshot(ctx, tx, x.Brand, t.MemberID)
		if e != nil {
			return e
		}
		// A corrupt wallet is an execution failure, never a legitimate shortage
		// that can be waived by clearing the cycle hold.
		if wallet.Version == 0 {
			if wallet.BySource != (points.Balance{}) {
				return points.ErrCorrupt
			}
		} else {
			var latest string
			if e = tx.QueryRow(ctx, `SELECT id::text FROM point_ledger_entries WHERE brand_id=$1 AND account_id=$2 AND version=$3`, x.Brand, wallet.AccountID, wallet.Version).Scan(&latest); e != nil {
				return points.ErrCorrupt
			}
			entry, e := (points.Store{DB: s.DB}).Entry(ctx, tx, x.Brand, t.MemberID, latest)
			if e != nil || entry.After != wallet.BySource {
				return points.ErrCorrupt
			}
		}
		if t.DeltaPoints < 0 && wallet.CommissionPoints < -t.DeltaPoints {
			code := "COMMISSION_CORRECTION_AVAILABLE_INSUFFICIENT"
			log, e := correctionExecutionStep(ctx, tx, x, "paused", "pause", "commission available points insufficient; stop entire cycle without using frozen or other sources", nil, &t.ID, &code, meta)
			if e != nil {
				return e
			}
			_, e = tx.Exec(ctx, `UPDATE commission_correction_cycle_holds SET version=version+1,active=true,blocked_execution_id=$3,audit_log_id=$4 WHERE brand_id=$1 AND cycle_id=$2`, x.Brand, x.Cycle, x.ID, log)
			if e != nil {
				return paymentDBError(e)
			}
			return saveCorrectionExecution(ctx, tx, x, "paused", &t.ID, &code, log)
		}
	}
	id := ids.New()
	log, err := correctionExecutionStep(ctx, tx, x, "applying", "apply", "one beneficiary difference and actual awarded net committed atomically", &id, nil, nil, meta)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO commission_correction_execution_targets(id,brand_id,cycle_id,execution_id,plan_target_id,agent_id,member_id,points_before,points_after,delta_points,base_financial_version,execution_version)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, id, x.Brand, x.Cycle, x.ID, t.ID, t.AgentID, t.MemberID, t.PointsBefore, t.PointsAfter, t.DeltaPoints, t.FinancialVersion, x.Version)
	if err != nil {
		return paymentDBError(err)
	}
	var ledger *string
	if t.DeltaPoints != 0 {
		var delta points.Balance
		delta[3][0] = t.DeltaPoints
		amount := t.DeltaPoints
		if amount < 0 {
			amount = -amount
		}
		entry, e := (points.Store{DB: s.DB}).Post(ctx, tx, points.Change{BrandID: x.Brand, MemberID: t.MemberID, EntryType: "commission_correction", ReferenceType: "commission_correction_target", ReferenceID: id, OperationKey: "commission-correction:" + id, Reason: "approved difference posted only to commission available points", ActorType: "system", RequestID: meta.RequestID, Delta: delta, Allocation: []points.Allocation{{Source: "commission", State: "available", Points: amount}}})
		if e != nil {
			return paymentDBError(e)
		}
		ledger = &entry.ID
	}
	financialVersion := int64(1)
	if t.FinancialVersion != nil {
		financialVersion = *t.FinancialVersion + 1
	}
	targetLog, err := audit.Append(ctx, tx, audit.Record{BrandID: x.Brand, ActorType: "system", Action: "commission.correction_execution.target", ResourceType: "commission_correction_target", ResourceID: id, Reason: "witness exact difference and resulting actual awarded net; zero differences do not post a ledger", RequestID: meta.RequestID,
		Before: map[string]any{"state": "pending"}, After: map[string]any{"state": "applied", "execution_id": x.ID, "plan_target_id": t.ID, "points_before": strconv.FormatInt(int64(t.PointsBefore), 10), "points_after": strconv.FormatInt(int64(t.PointsAfter), 10), "delta_points": strconv.FormatInt(int64(t.DeltaPoints), 10), "ledger_entry_id": ledger, "financial_version": financialVersion}})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE commission_correction_execution_targets SET state='applied',ledger_entry_id=$2,audit_log_id=$3,financial_version=$4,applied_at=clock_timestamp() WHERE id=$1`, id, ledger, targetLog, financialVersion)
	if err != nil {
		return paymentDBError(err)
	}
	return saveCorrectionExecution(ctx, tx, x, "applying", nil, nil, log)
}

func (s Service) failCorrectionExecution(ctx context.Context, observed correctionExecutionRow) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	x, err := lockCorrectionExecution(ctx, tx, observed.Brand, observed.ID)
	if err != nil {
		return err
	}
	// A possibly committed step wins over an outdated failure observation.
	if x.Version != observed.Version || x.State != "applying" {
		return nil
	}
	current, err := correctionExecutionEvidence(ctx, tx, x)
	if err != nil {
		return err
	}
	state, operation, reason := "failed", "fail", "financial difference failed and rolled back; explicit administrator retry required"
	v := "COMMISSION_CORRECTION_EXECUTION_FAILED"
	var code *string = &v
	if !current {
		state, operation, reason, code = "stale", "invalidate", "replacement evidence changed before failure recording", nil
	}
	log, err := correctionExecutionStep(ctx, tx, x, state, operation, reason, nil, nil, code, points.Metadata{ActorType: "system", RequestID: ids.New()})
	if err != nil {
		return err
	}
	if err = saveCorrectionExecution(ctx, tx, x, state, nil, code, log); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
