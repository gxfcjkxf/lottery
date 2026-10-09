package commission

import (
	"context"
	"errors"
	"strconv"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

type correctionPlanRow struct {
	ID, Brand, Cycle, Payment, Run, State string
	Version, Epoch, Count, Planned        int64
	Cursor                                *string
	ErrorCode                             *string
}

func lockCorrectionPlan(ctx context.Context, tx pgx.Tx, brand, id string) (correctionPlanRow, error) {
	var p correctionPlanRow
	if tx == nil || !canonicalUUID(brand) || !canonicalUUID(id) {
		return p, ErrInvalid
	}
	var cycle string
	err := tx.QueryRow(ctx, `SELECT cycle_id::text FROM commission_correction_plans WHERE brand_id=$1 AND id=$2`, brand, id).Scan(&cycle)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	if err != nil {
		return p, err
	}
	if _, err = cycleLock(ctx, tx, brand, cycle); err != nil {
		return p, err
	}
	err = tx.QueryRow(ctx, `SELECT id::text,brand_id::text,cycle_id::text,payment_id::text,run_id::text,state,version,evidence_epoch,target_count,planned_count,cursor_agent_id::text,last_error_code
 FROM commission_correction_plans WHERE brand_id=$1 AND id=$2 FOR UPDATE NOWAIT`, brand, id).Scan(&p.ID, &p.Brand, &p.Cycle, &p.Payment, &p.Run, &p.State, &p.Version, &p.Epoch, &p.Count, &p.Planned, &p.Cursor, &p.ErrorCode)
	return p, paymentDBError(err)
}

func correctionPlanEvidence(ctx context.Context, tx pgx.Tx, p correctionPlanRow) (bool, error) {
	var valid bool
	// Generation invalidation and damaged prior financial sources are distinct:
	// only changed replacement evidence stales a plan. Source validation is
	// enforced when creating/advancing it; a source failure must remain failed.
	err := tx.QueryRow(ctx, `SELECT commission_payment_evidence_current($1,$2,$3,$4)`, p.Brand, p.Cycle, p.Run, p.Epoch).Scan(&valid)
	return valid, err
}

func correctionPlanStep(ctx context.Context, tx pgx.Tx, p correctionPlanRow, state, operation, reason string, planned int64, cursor, code *string, meta points.Metadata) (string, error) {
	version := p.Version + 1
	var before any = map[string]any{"version": p.Version, "state": p.State}
	var from *string = &p.State
	if operation == "create" {
		version, before, from = 1, nil, nil
	} else if p.Version >= maxCycleVersion {
		return "", ErrCorrectionPlanVersion
	}
	log, err := audit.Append(ctx, tx, audit.Record{BrandID: p.Brand, ActorType: meta.ActorType, ActorID: meta.ActorID, Action: "commission.correction_plan." + operation,
		ResourceType: "commission_correction_plan", ResourceID: p.ID, Reason: reason, RequestID: meta.RequestID, IP: meta.IP,
		Before: before, After: map[string]any{"version": version, "state": state, "planned_count": strconv.FormatInt(planned, 10), "cursor_agent_id": cursor, "error_code": code}})
	if err != nil {
		return "", err
	}
	if operation != "create" {
		_, err = tx.Exec(ctx, `INSERT INTO commission_correction_plan_steps(id,brand_id,plan_id,version,from_state,to_state,operation,planned_count,cursor_agent_id,error_code,actor_type,actor_id,reason,request_id,audit_log_id)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NULLIF($12,'')::uuid,$13,$14,$15)`, ids.New(), p.Brand, p.ID, version, from, state, operation, planned, cursor, code, meta.ActorType, meta.ActorID, reason, meta.RequestID, log)
	}
	return log, paymentDBError(err)
}

func saveCorrectionPlan(ctx context.Context, tx pgx.Tx, p correctionPlanRow, state string, planned int64, cursor, code *string, log string) error {
	_, err := tx.Exec(ctx, `UPDATE commission_correction_plans SET version=version+1,state=$3,planned_count=$4,cursor_agent_id=$5,last_error_code=$6,last_audit_log_id=$7,next_work_at=clock_timestamp()
 WHERE brand_id=$1 AND id=$2`, p.Brand, p.ID, state, planned, cursor, code, log)
	return paymentDBError(err)
}

func (s Service) RetryCorrectionPlanTx(ctx context.Context, tx pgx.Tx, brand, id string, a access.Account, in RetryCycleInput, meta points.Metadata) (CorrectionPlan, error) {
	if tx == nil || !canonicalUUID(id) || !canonicalUUID(a.ID) || in.Validate() != nil || meta.ActorType != "admin" || meta.ActorID != a.ID || meta.RequestID == "" || len(meta.RequestID) > 80 {
		return CorrectionPlan{}, ErrInvalid
	}
	if !AllowedCorrectionPlan(a, brand, "retry") {
		return CorrectionPlan{}, ErrDenied
	}
	if err := paymentBrand(ctx, tx, brand); err != nil {
		return CorrectionPlan{}, err
	}
	var actualSuper bool
	var status string
	err := tx.QueryRow(ctx, `SELECT status,is_super_admin FROM admin_accounts WHERE id=$1 FOR SHARE NOWAIT`, a.ID).Scan(&status, &actualSuper)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && (actualSuper || status != "active") {
		return CorrectionPlan{}, ErrDenied
	}
	if err != nil {
		return CorrectionPlan{}, paymentDBError(err)
	}
	gate, err := lockPaymentPolicy(ctx, tx, brand)
	if err != nil {
		return CorrectionPlan{}, err
	}
	if !gate.Enabled {
		return CorrectionPlan{}, ErrCorrectionPlanState
	}
	p, err := lockCorrectionPlan(ctx, tx, brand, id)
	if err != nil {
		return CorrectionPlan{}, err
	}
	if p.Version != in.Version {
		return CorrectionPlan{}, ErrCorrectionPlanVersion
	}
	historical := p.State == "blocked" && p.ErrorCode != nil && *p.ErrorCode == "COMMISSION_CORRECTION_MANUAL_POLICY_UNRESOLVED" && p.Planned == 0 && p.Cursor == nil
	if p.State != "failed" && !historical {
		return CorrectionPlan{}, ErrCorrectionPlanState
	}
	valid, err := correctionPlanEvidence(ctx, tx, p)
	if err != nil {
		return CorrectionPlan{}, err
	}
	if !valid {
		return CorrectionPlan{}, ErrCorrectionPlanEvidence
	}
	var sourcesValid bool
	if err = tx.QueryRow(ctx, `SELECT commission_correction_source_valid($1,$2,$3)`, p.Brand, p.Payment, p.Run).Scan(&sourcesValid); err != nil {
		return CorrectionPlan{}, err
	}
	if !sourcesValid {
		return CorrectionPlan{}, ErrCorrectionPlanEvidence
	}
	if historical {
		// A historical blocked plan froze these facts before the policy choice.
		// Do not silently rewrite that basis if actual financial heads changed.
		var exact bool
		err = tx.QueryRow(ctx, `SELECT p.credit_points IS NULL AND p.debit_points IS NULL AND p.net_points IS NULL
 AND p.before_points=x.before_points AND p.calculated_points=x.after_points AND p.target_count=x.n
 AND NOT EXISTS(SELECT 1 FROM commission_correction_plan_targets WHERE plan_id=p.id)
 FROM commission_correction_plans p CROSS JOIN LATERAL (
 SELECT count(*) AS n,coalesce(sum(points_before::numeric),0) AS before_points,coalesce(sum(points_after::numeric),0) AS after_points
 FROM commission_correction_candidates(p.brand_id,p.payment_id,p.run_id)) x WHERE p.brand_id=$1 AND p.id=$2`, brand, id).Scan(&exact)
		if err != nil {
			return CorrectionPlan{}, err
		}
		if !exact {
			return CorrectionPlan{}, ErrCorrectionPlanEvidence
		}
	}
	log, err := correctionPlanStep(ctx, tx, p, "planning", "retry", in.Reason, p.Planned, p.Cursor, nil, meta)
	if err != nil {
		return CorrectionPlan{}, err
	}
	if historical {
		_, err = tx.Exec(ctx, `UPDATE commission_correction_plans p SET version=version+1,state='planning',last_error_code=NULL,last_audit_log_id=$3,next_work_at=clock_timestamp(),
 credit_points=x.credit,debit_points=x.debit,net_points=x.net FROM (
 SELECT coalesce(sum(greatest(delta_points,0)::numeric),0) AS credit,coalesce(sum(greatest(-delta_points,0)::numeric),0) AS debit,coalesce(sum(delta_points::numeric),0) AS net
 FROM commission_correction_candidates($1,$4,$5)) x WHERE p.brand_id=$1 AND p.id=$2`, brand, id, log, p.Payment, p.Run)
		err = paymentDBError(err)
	} else {
		err = saveCorrectionPlan(ctx, tx, p, "planning", p.Planned, p.Cursor, nil, log)
	}
	if err != nil {
		return CorrectionPlan{}, err
	}
	return s.CorrectionPlanTx(ctx, tx, brand, id)
}
