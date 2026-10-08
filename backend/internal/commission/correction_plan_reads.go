package commission

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

const correctionPlanJSON = `jsonb_build_object('id',p.id::text,'brand_id',p.brand_id::text,'cycle_id',p.cycle_id::text,
 'payment_id',p.payment_id::text,'run_id',p.run_id::text,'state',p.state,'payout_mode',p.payout_mode,'version',p.version,
 'evidence_epoch',p.evidence_epoch::text,'before_points',p.before_points::text,'calculated_points',p.calculated_points::text,
 'credit_points',p.credit_points::text,'debit_points',p.debit_points::text,'net_points',p.net_points::text,
 'target_count',p.target_count::text,'planned_count',p.planned_count::text,'creation_audit_log_id',p.creation_audit_log_id::text,
 'last_audit_log_id',p.last_audit_log_id::text,'last_error_code',p.last_error_code,'created_at',p.created_at,'updated_at',p.updated_at)`

func (s Service) CorrectionPlanTx(ctx context.Context, tx pgx.Tx, brand, id string) (CorrectionPlan, error) {
	var out CorrectionPlan
	if tx == nil || !canonicalUUID(brand) || !canonicalUUID(id) {
		return out, ErrInvalid
	}
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT `+correctionPlanJSON+` FROM commission_correction_plans p WHERE p.brand_id=$1 AND p.id=$2`, brand, id).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(raw, &out)
	out.CreatedAt, out.UpdatedAt = out.CreatedAt.UTC(), out.UpdatedAt.UTC()
	return out, err
}

func (s Service) CorrectionPlansTx(ctx context.Context, tx pgx.Tx, brand string, limit, offset int) (CorrectionPlanPage, error) {
	out := CorrectionPlanPage{BrandID: brand, Items: []CorrectionPlan{}, Limit: limit, Offset: offset}
	if tx == nil || !validCyclePage(brand, limit, offset) {
		return out, ErrInvalid
	}
	var raw []byte
	err := tx.QueryRow(ctx, `WITH page AS (SELECT * FROM commission_correction_plans WHERE brand_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2 OFFSET $3)
 SELECT (SELECT count(*)::text FROM commission_correction_plans WHERE brand_id=$1),
 coalesce((SELECT jsonb_agg(`+correctionPlanJSON+` ORDER BY p.created_at DESC,p.id DESC) FROM page p),'[]'::jsonb)`, brand, limit, offset).Scan(&out.TotalCount, &raw)
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(raw, &out.Items); err != nil {
		return out, err
	}
	for i := range out.Items {
		out.Items[i].CreatedAt, out.Items[i].UpdatedAt = out.Items[i].CreatedAt.UTC(), out.Items[i].UpdatedAt.UTC()
	}
	return out, nil
}

const correctionPlanTargetJSON = `jsonb_build_object('id',t.id::text,'brand_id',t.brand_id::text,'plan_id',t.plan_id::text,
 'agent_id',t.agent_id::text,'member_id',t.member_id::text,'original_target_id',t.original_target_id::text,'earning_id',t.earning_id::text,
 'adjustment_version',t.adjustment_version,'previous_correction_target_id',t.previous_correction_target_id::text,'financial_version',t.financial_version,'points_before',t.points_before::text,'points_after',t.points_after::text,'delta_points',t.delta_points::text,
 'creation_audit_log_id',t.creation_audit_log_id::text,'created_at',t.created_at)`

func (s Service) CorrectionPlanTargetsTx(ctx context.Context, tx pgx.Tx, brand, id string, limit, offset int) (CorrectionPlanTargetPage, error) {
	out := CorrectionPlanTargetPage{BrandID: brand, PlanID: id, Items: []CorrectionPlanTarget{}, Limit: limit, Offset: offset}
	if tx == nil || !validCyclePage(brand, limit, offset) || !canonicalUUID(id) {
		return out, ErrInvalid
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM commission_correction_plans WHERE brand_id=$1 AND id=$2)`, brand, id).Scan(&exists); err != nil {
		return out, err
	}
	if !exists {
		return out, ErrNotFound
	}
	var raw []byte
	err := tx.QueryRow(ctx, `WITH page AS (SELECT * FROM commission_correction_plan_targets WHERE brand_id=$1 AND plan_id=$2 ORDER BY agent_id LIMIT $3 OFFSET $4)
 SELECT (SELECT count(*)::text FROM commission_correction_plan_targets WHERE brand_id=$1 AND plan_id=$2),
 coalesce((SELECT jsonb_agg(`+correctionPlanTargetJSON+` ORDER BY t.agent_id) FROM page t),'[]'::jsonb)`, brand, id, limit, offset).Scan(&out.TotalCount, &raw)
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(raw, &out.Items); err != nil {
		return out, err
	}
	for i := range out.Items {
		out.Items[i].CreatedAt = out.Items[i].CreatedAt.UTC()
	}
	return out, nil
}
