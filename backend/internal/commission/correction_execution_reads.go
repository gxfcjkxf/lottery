package commission

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Keep this projection deliberately closed: execution reads expose operational
// state and exact point deltas, never wallet snapshots or plan policy material.
const correctionExecutionJSON = `jsonb_build_object(
 'id',x.id::text,'brand_id',x.brand_id::text,'cycle_id',x.cycle_id::text,
 'plan_id',x.plan_id::text,'run_id',x.run_id::text,'payout_mode',x.payout_mode,
 'state',x.state,'version',x.version,'plan_version',x.plan_version,
	'evidence_epoch',x.evidence_epoch::text,'credit_points',x.credit_points::text,
	'debit_points',x.debit_points::text,'net_points',x.net_points::text,
	'target_count',x.target_count::text,
	'applied_count',coalesce(targets.applied_count,'0'),
	'applied_credit_points',coalesce(targets.applied_credit_points,'0'),
	'applied_debit_points',coalesce(targets.applied_debit_points,'0'),
 'approval_actor_type',x.approval_actor_type,'approved_by',x.approved_by::text,
 'approval_audit_log_id',x.approval_audit_log_id::text,
 'creation_audit_log_id',x.creation_audit_log_id::text,
	'last_audit_log_id',x.last_audit_log_id::text,'last_error_code',x.last_error_code,
	'paused_plan_target_id',x.paused_plan_target_id::text,
	'cycle_hold_active',coalesce(h.active,false),'created_at',x.created_at,'updated_at',x.updated_at)`

const correctionExecutionTargetJSON = `jsonb_build_object(
 'id',t.id::text,'brand_id',t.brand_id::text,'execution_id',t.execution_id::text,
 'plan_target_id',t.plan_target_id::text,'agent_id',t.agent_id::text,
 'member_id',t.member_id::text,'points_before',t.points_before::text,
 'points_after',t.points_after::text,'delta_points',t.delta_points::text,
 'state',t.state,'ledger_entry_id',t.ledger_entry_id::text,
 'audit_log_id',t.audit_log_id::text,'financial_version',t.financial_version,
 'created_at',t.created_at,'applied_at',t.applied_at)`

func (s Service) CorrectionExecutionTx(ctx context.Context, tx pgx.Tx, brand, id string) (CorrectionExecution, error) {
	var out CorrectionExecution
	if tx == nil || !canonicalUUID(brand) || !canonicalUUID(id) {
		return out, ErrInvalid
	}
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT `+correctionExecutionJSON+`
 FROM commission_correction_executions x
 LEFT JOIN LATERAL (
   SELECT count(*) FILTER (WHERE t.state='applied')::text AS applied_count,
     coalesce(sum(CASE WHEN t.delta_points>0 THEN t.delta_points::numeric ELSE 0 END),0)::text AS applied_credit_points,
     coalesce(sum(CASE WHEN t.delta_points<0 THEN -t.delta_points::numeric ELSE 0 END),0)::text AS applied_debit_points
   FROM commission_correction_execution_targets t
   WHERE t.brand_id=x.brand_id AND t.execution_id=x.id
 ) targets ON true
 LEFT JOIN commission_correction_cycle_holds h
   ON h.brand_id=x.brand_id AND h.cycle_id=x.cycle_id
 WHERE x.brand_id=$1 AND x.id=$2`, brand, id).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(raw, &out); err != nil {
		return CorrectionExecution{}, err
	}
	out.CreatedAt, out.UpdatedAt = out.CreatedAt.UTC(), out.UpdatedAt.UTC()
	return out, nil
}

func (s Service) CorrectionExecutionsTx(ctx context.Context, tx pgx.Tx, brand string, limit, offset int) (CorrectionExecutionPage, error) {
	out := CorrectionExecutionPage{BrandID: brand, Items: []CorrectionExecution{}, Limit: limit, Offset: offset}
	if tx == nil || !validCyclePage(brand, limit, offset) {
		return out, ErrInvalid
	}
	var raw []byte
	err := tx.QueryRow(ctx, `WITH page AS (
 SELECT x.*, targets.applied_count, targets.applied_credit_points,
   targets.applied_debit_points, h.active AS current_cycle_hold_active
 FROM commission_correction_executions x
 LEFT JOIN LATERAL (
   SELECT count(*) FILTER (WHERE t.state='applied')::text AS applied_count,
     coalesce(sum(CASE WHEN t.delta_points>0 THEN t.delta_points::numeric ELSE 0 END),0)::text AS applied_credit_points,
     coalesce(sum(CASE WHEN t.delta_points<0 THEN -t.delta_points::numeric ELSE 0 END),0)::text AS applied_debit_points
   FROM commission_correction_execution_targets t
   WHERE t.brand_id=x.brand_id AND t.execution_id=x.id
 ) targets ON true
 LEFT JOIN commission_correction_cycle_holds h
   ON h.brand_id=x.brand_id AND h.cycle_id=x.cycle_id
 WHERE x.brand_id=$1
 ORDER BY x.created_at DESC,x.id DESC LIMIT $2 OFFSET $3
), contents AS (
 SELECT `+correctionExecutionJSON+` AS data,x.created_at,x.id FROM page x
 LEFT JOIN LATERAL (SELECT x.applied_count,x.applied_credit_points,x.applied_debit_points) targets ON true
 LEFT JOIN LATERAL (SELECT x.current_cycle_hold_active AS active) h ON true
)
SELECT (SELECT count(*)::text FROM commission_correction_executions WHERE brand_id=$1),
 coalesce((SELECT jsonb_agg(data ORDER BY created_at DESC,id DESC) FROM contents),'[]'::jsonb)`, brand, limit, offset).Scan(&out.TotalCount, &raw)
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(raw, &out.Items); err != nil {
		return CorrectionExecutionPage{BrandID: brand, Items: []CorrectionExecution{}, Limit: limit, Offset: offset}, err
	}
	for i := range out.Items {
		out.Items[i].CreatedAt = out.Items[i].CreatedAt.UTC()
		out.Items[i].UpdatedAt = out.Items[i].UpdatedAt.UTC()
	}
	return out, nil
}

func (s Service) CorrectionExecutionTargetsTx(ctx context.Context, tx pgx.Tx, brand, executionID string, limit, offset int) (CorrectionExecutionTargetPage, error) {
	out := CorrectionExecutionTargetPage{BrandID: brand, ExecutionID: executionID, Items: []CorrectionExecutionTarget{}, Limit: limit, Offset: offset}
	if tx == nil || !validCyclePage(brand, limit, offset) || !canonicalUUID(executionID) {
		return out, ErrInvalid
	}
	var exists bool
	var raw []byte
	err := tx.QueryRow(ctx, `WITH owner AS (
 SELECT 1 FROM commission_correction_executions
 WHERE brand_id=$1 AND id=$2
), page AS (
 SELECT t.* FROM commission_correction_execution_targets t
 WHERE t.brand_id=$1 AND t.execution_id=$2
 ORDER BY t.agent_id,t.id LIMIT $3 OFFSET $4
), contents AS (
 SELECT `+correctionExecutionTargetJSON+` AS data,t.agent_id,t.id FROM page t
)
SELECT EXISTS(SELECT 1 FROM owner),
 (SELECT count(*)::text FROM commission_correction_execution_targets WHERE brand_id=$1 AND execution_id=$2),
 coalesce((SELECT jsonb_agg(data ORDER BY agent_id,id) FROM contents),'[]'::jsonb)`, brand, executionID, limit, offset).Scan(&exists, &out.TotalCount, &raw)
	if err != nil {
		return out, err
	}
	if !exists {
		return out, ErrNotFound
	}
	if err = json.Unmarshal(raw, &out.Items); err != nil {
		return CorrectionExecutionTargetPage{BrandID: brand, ExecutionID: executionID, Items: []CorrectionExecutionTarget{}, Limit: limit, Offset: offset}, err
	}
	for i := range out.Items {
		out.Items[i].CreatedAt = out.Items[i].CreatedAt.UTC()
		if out.Items[i].AppliedAt != nil {
			*out.Items[i].AppliedAt = out.Items[i].AppliedAt.UTC()
		}
	}
	return out, nil
}

func (s Service) CorrectionExecutionPolicyTx(ctx context.Context, tx pgx.Tx, brand string) (CorrectionExecutionPolicy, error) {
	var out CorrectionExecutionPolicy
	if tx == nil || !canonicalUUID(brand) {
		return out, ErrInvalid
	}
	var auditID *string
	err := tx.QueryRow(ctx, `SELECT brand_id::text,version,enabled,audit_log_id::text,updated_at
		FROM brand_commission_correction_policies WHERE brand_id=$1`, brand).
		Scan(&out.BrandID, &out.Version, &out.Enabled, &auditID, &out.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return CorrectionExecutionPolicy{}, ErrNotFound
	}
	if err != nil {
		return CorrectionExecutionPolicy{}, paymentDBError(err)
	}
	if auditID != nil {
		out.AuditLogID = *auditID
	}
	out.UpdatedAt = out.UpdatedAt.UTC()
	return out, nil
}
