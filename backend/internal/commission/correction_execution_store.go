package commission

import (
	"context"
	"errors"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

type correctionExecutionRow struct {
	ID, Brand, Cycle, Plan, Run, State, Mode string
	Version, PlanVersion, Epoch              int64
	Approval                                 *string
}

func lockCorrectionExecution(ctx context.Context, tx pgx.Tx, brand, id string) (correctionExecutionRow, error) {
	var x correctionExecutionRow
	if tx == nil || !canonicalUUID(brand) || !canonicalUUID(id) {
		return x, ErrInvalid
	}
	var cycle string
	err := tx.QueryRow(ctx, `SELECT cycle_id::text FROM commission_correction_executions WHERE brand_id=$1 AND id=$2`, brand, id).Scan(&cycle)
	if errors.Is(err, pgx.ErrNoRows) {
		return x, ErrNotFound
	}
	if err != nil {
		return x, err
	}
	if _, err = cycleLock(ctx, tx, brand, cycle); err != nil {
		return x, err
	}
	err = tx.QueryRow(ctx, `SELECT id::text,brand_id::text,cycle_id::text,plan_id::text,run_id::text,state,payout_mode,version,plan_version,evidence_epoch,approval_audit_log_id::text
 FROM commission_correction_executions WHERE brand_id=$1 AND id=$2 FOR UPDATE NOWAIT`, brand, id).Scan(&x.ID, &x.Brand, &x.Cycle, &x.Plan, &x.Run, &x.State, &x.Mode, &x.Version, &x.PlanVersion, &x.Epoch, &x.Approval)
	return x, paymentDBError(err)
}

func correctionExecutionGate(ctx context.Context, tx pgx.Tx, brand string) error {
	if err := paymentBrand(ctx, tx, brand); err != nil {
		return err
	}
	p, err := lockPaymentPolicy(ctx, tx, brand)
	if err != nil {
		return err
	}
	if !p.Enabled {
		return ErrCorrectionExecutionState
	}
	var enabled bool
	err = tx.QueryRow(ctx, `SELECT enabled FROM brand_commission_correction_policies WHERE brand_id=$1 FOR SHARE NOWAIT`, brand).Scan(&enabled)
	if err != nil {
		return paymentDBError(err)
	}
	if !enabled {
		return ErrCorrectionExecutionState
	}
	return nil
}

func correctionExecutionActor(ctx context.Context, tx pgx.Tx, brand string, a access.Account, action string, meta points.Metadata) error {
	if tx == nil || !canonicalUUID(a.ID) || !canonicalUUID(brand) || a.Type != access.AccountAdmin || meta.ActorType != "admin" || meta.ActorID != a.ID || meta.RequestID == "" || len(meta.RequestID) > 80 {
		return ErrInvalid
	}
	if !AllowedCorrectionExecution(a, brand, action) {
		return ErrDenied
	}
	var super bool
	var status string
	err := tx.QueryRow(ctx, `SELECT status,is_super_admin FROM admin_accounts WHERE id=$1 FOR SHARE NOWAIT`, a.ID).Scan(&status, &super)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && (super || status != "active") {
		return ErrDenied
	}
	return paymentDBError(err)
}

func (s Service) UpdateCorrectionExecutionPolicyTx(ctx context.Context, tx pgx.Tx, brand string, a access.Account, in PaymentPolicyInput, meta points.Metadata) (CorrectionExecutionPolicy, error) {
	if tx == nil || in.Validate() != nil {
		return CorrectionExecutionPolicy{}, ErrInvalid
	}
	if err := correctionExecutionActor(ctx, tx, brand, a, "policy_write", meta); err != nil {
		return CorrectionExecutionPolicy{}, err
	}
	if err := paymentBrand(ctx, tx, brand); err != nil {
		return CorrectionExecutionPolicy{}, err
	}
	var version int64
	var enabled bool
	err := tx.QueryRow(ctx, `SELECT version,enabled FROM brand_commission_correction_policies WHERE brand_id=$1 FOR UPDATE NOWAIT`, brand).Scan(&version, &enabled)
	if err != nil {
		return CorrectionExecutionPolicy{}, paymentDBError(err)
	}
	if version != in.Version || version >= maxCycleVersion {
		return CorrectionExecutionPolicy{}, ErrCorrectionExecutionVersion
	}
	log, err := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "commission.correction_policy.update", ResourceType: "commission_correction_policy", ResourceID: brand,
		Reason: in.Reason, RequestID: meta.RequestID, IP: meta.IP, Before: map[string]any{"version": version, "enabled": enabled}, After: map[string]any{"version": version + 1, "enabled": in.Enabled}})
	if err != nil {
		return CorrectionExecutionPolicy{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE brand_commission_correction_policies SET version=version+1,enabled=$2,audit_log_id=$3 WHERE brand_id=$1`, brand, in.Enabled, log); err != nil {
		return CorrectionExecutionPolicy{}, paymentDBError(err)
	}
	return s.CorrectionExecutionPolicyTx(ctx, tx, brand)
}

func correctionExecutionStep(ctx context.Context, tx pgx.Tx, x correctionExecutionRow, state, operation, reason string, target, paused, code *string, meta points.Metadata) (string, error) {
	version := x.Version + 1
	var before any = map[string]any{"version": x.Version, "state": x.State}
	var from *string = &x.State
	if operation == "create" {
		version, before, from = 1, nil, nil
	} else if x.Version >= maxCycleVersion {
		return "", ErrCorrectionExecutionVersion
	}
	log, err := audit.Append(ctx, tx, audit.Record{BrandID: x.Brand, ActorType: meta.ActorType, ActorID: meta.ActorID,
		Action: "commission.correction_execution." + operation, ResourceType: "commission_correction_execution", ResourceID: x.ID, Reason: reason, RequestID: meta.RequestID, IP: meta.IP,
		Before: before, After: map[string]any{"version": version, "state": state, "target_id": target, "paused_plan_target_id": paused, "error_code": code}})
	if err != nil {
		return "", err
	}
	if operation != "create" {
		_, err = tx.Exec(ctx, `INSERT INTO commission_correction_execution_steps(id,brand_id,execution_id,version,from_state,to_state,operation,target_id,paused_plan_target_id,error_code,actor_type,actor_id,reason,request_id,audit_log_id)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NULLIF($12,'')::uuid,$13,$14,$15)`, ids.New(), x.Brand, x.ID, version, from, state, operation, target, paused, code, meta.ActorType, meta.ActorID, reason, meta.RequestID, log)
	}
	return log, paymentDBError(err)
}

func saveCorrectionExecution(ctx context.Context, tx pgx.Tx, x correctionExecutionRow, state string, paused, code *string, log string, approver ...string) error {
	var err error
	if len(approver) == 1 {
		_, err = tx.Exec(ctx, `UPDATE commission_correction_executions SET version=version+1,state=$3,last_audit_log_id=$4,paused_plan_target_id=$5,last_error_code=$6,
 next_work_at=clock_timestamp(),approved_by=$7,approval_actor_type='admin',approval_audit_log_id=$4 WHERE brand_id=$1 AND id=$2`, x.Brand, x.ID, state, log, paused, code, approver[0])
	} else {
		_, err = tx.Exec(ctx, `UPDATE commission_correction_executions SET version=version+1,state=$3,last_audit_log_id=$4,paused_plan_target_id=$5,last_error_code=$6,next_work_at=clock_timestamp() WHERE brand_id=$1 AND id=$2`, x.Brand, x.ID, state, log, paused, code)
	}
	return paymentDBError(err)
}

func correctionExecutionEvidence(ctx context.Context, tx pgx.Tx, x correctionExecutionRow) (bool, error) {
	var valid bool
	err := tx.QueryRow(ctx, `SELECT commission_correction_execution_current($1,$2)`, x.Brand, x.ID).Scan(&valid)
	return valid, err
}

func correctionExecutionSources(ctx context.Context, tx pgx.Tx, x correctionExecutionRow) error {
	var valid bool
	err := tx.QueryRow(ctx, `SELECT commission_correction_source_valid($1,p.payment_id,$3) AND commission_correction_heads_valid($1,$4) FROM commission_correction_plans p WHERE p.brand_id=$1 AND p.id=$2`, x.Brand, x.Plan, x.Run, x.Cycle).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return ErrCorrectionExecutionEvidence
	}
	return nil
}

func (s Service) actCorrectionExecution(ctx context.Context, tx pgx.Tx, brand, id string, a access.Account, in RetryCycleInput, meta points.Metadata, action string) (CorrectionExecution, error) {
	if tx == nil || !canonicalUUID(id) || in.Validate() != nil {
		return CorrectionExecution{}, ErrInvalid
	}
	if err := correctionExecutionActor(ctx, tx, brand, a, action, meta); err != nil {
		return CorrectionExecution{}, err
	}
	if err := correctionExecutionGate(ctx, tx, brand); err != nil {
		return CorrectionExecution{}, err
	}
	x, err := lockCorrectionExecution(ctx, tx, brand, id)
	if err != nil {
		return CorrectionExecution{}, err
	}
	if x.Version != in.Version || x.Version >= maxCycleVersion {
		return CorrectionExecution{}, ErrCorrectionExecutionVersion
	}
	if action == "approve" && (x.State != "awaiting_approval" || x.Approval != nil || x.Mode != "manual" && x.Mode != "mixed") ||
		action == "continue" && (x.State != "paused" || x.Approval == nil) || action == "retry" && (x.State != "failed" || x.Approval == nil) {
		return CorrectionExecution{}, ErrCorrectionExecutionState
	}
	valid, err := correctionExecutionEvidence(ctx, tx, x)
	if err != nil {
		return CorrectionExecution{}, err
	}
	if !valid {
		return CorrectionExecution{}, ErrCorrectionExecutionEvidence
	}
	if err = correctionExecutionSources(ctx, tx, x); err != nil {
		return CorrectionExecution{}, err
	}
	var held bool
	err = tx.QueryRow(ctx, `SELECT active FROM commission_correction_cycle_holds WHERE brand_id=$1 AND cycle_id=$2 FOR UPDATE NOWAIT`, brand, x.Cycle).Scan(&held)
	if err != nil {
		return CorrectionExecution{}, paymentDBError(err)
	}
	state := "applying"
	var code *string
	if held && action != "continue" {
		if action != "approve" {
			return CorrectionExecution{}, ErrCorrectionExecutionState
		}
		state = "paused"
		v := "COMMISSION_CORRECTION_CYCLE_HELD"
		code = &v
	}
	if action == "continue" && !held {
		return CorrectionExecution{}, ErrCorrectionExecutionState
	}
	log, err := correctionExecutionStep(ctx, tx, x, state, action, in.Reason, nil, nil, code, meta)
	if err != nil {
		return CorrectionExecution{}, err
	}
	if action == "continue" {
		_, err = tx.Exec(ctx, `UPDATE commission_correction_cycle_holds SET version=version+1,active=false,audit_log_id=$3 WHERE brand_id=$1 AND cycle_id=$2`, brand, x.Cycle, log)
		if err != nil {
			return CorrectionExecution{}, paymentDBError(err)
		}
	}
	if action == "approve" {
		err = saveCorrectionExecution(ctx, tx, x, state, nil, code, log, a.ID)
	} else {
		err = saveCorrectionExecution(ctx, tx, x, state, nil, code, log)
	}
	if err != nil {
		return CorrectionExecution{}, err
	}
	return s.CorrectionExecutionTx(ctx, tx, brand, id)
}

func (s Service) ApproveCorrectionExecutionTx(ctx context.Context, tx pgx.Tx, brand, id string, a access.Account, in RetryCycleInput, meta points.Metadata) (CorrectionExecution, error) {
	return s.actCorrectionExecution(ctx, tx, brand, id, a, in, meta, "approve")
}
func (s Service) ContinueCorrectionExecutionTx(ctx context.Context, tx pgx.Tx, brand, id string, a access.Account, in RetryCycleInput, meta points.Metadata) (CorrectionExecution, error) {
	return s.actCorrectionExecution(ctx, tx, brand, id, a, in, meta, "continue")
}
func (s Service) RetryCorrectionExecutionTx(ctx context.Context, tx pgx.Tx, brand, id string, a access.Account, in RetryCycleInput, meta points.Metadata) (CorrectionExecution, error) {
	return s.actCorrectionExecution(ctx, tx, brand, id, a, in, meta, "retry")
}
