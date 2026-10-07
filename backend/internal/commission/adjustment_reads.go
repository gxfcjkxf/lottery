package commission

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

const commissionTargetReadJSON = `jsonb_build_object(
 'id',t.id::text,'brand_id',t.brand_id::text,'payment_id',t.payment_id::text,'earning_id',t.earning_id::text,
 'agent_id',t.agent_id::text,'member_id',t.member_id::text,'original_points',t.points::text,'state',t.state,
 'ledger_entry_id',t.ledger_entry_id::text,'paid_at',t.paid_at,
 'adjustment_version',h.version,'adjusted_points',h.points::text,'last_adjustment_id',h.last_adjustment_id::text)`

func (s Service) PaymentTargetsTx(ctx context.Context, tx pgx.Tx, brand, payment string, limit, offset int) (TargetPage, error) {
	out := TargetPage{BrandID: brand, PaymentID: payment, Items: []Target{}, Limit: limit, Offset: offset}
	if tx == nil || !validAdjustmentPage(brand, limit, offset) || !cycleCanonicalUUID.MatchString(payment) {
		return out, ErrInvalid
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM commission_payments WHERE brand_id=$1 AND id=$2)`, brand, payment).Scan(&exists); err != nil {
		return out, paymentDBError(err)
	}
	if !exists {
		return out, ErrNotFound
	}
	var raw []byte
	err := tx.QueryRow(ctx, `WITH page AS (
 SELECT t.* FROM commission_payment_targets t WHERE t.brand_id=$1 AND t.payment_id=$2 ORDER BY t.id LIMIT $3 OFFSET $4
), contents AS (
 SELECT `+commissionTargetReadJSON+` data,t.id FROM page t
 LEFT JOIN commission_adjustment_heads h ON h.target_id=t.id AND h.brand_id=t.brand_id
)
SELECT (SELECT count(*)::text FROM commission_payment_targets WHERE brand_id=$1 AND payment_id=$2),
 coalesce((SELECT jsonb_agg(data ORDER BY id) FROM contents),'[]'::jsonb)`, brand, payment, limit, offset).Scan(&out.TotalCount, &raw)
	if err != nil {
		return out, paymentDBError(err)
	}
	if err = json.Unmarshal(raw, &out.Items); err != nil {
		return out, err
	}
	for i := range out.Items {
		if out.Items[i].PaidAt != nil {
			*out.Items[i].PaidAt = out.Items[i].PaidAt.UTC()
		}
	}
	return out, nil
}

const commissionAdjustmentReadJSON = `jsonb_build_object(
 'id',a.id::text,'brand_id',a.brand_id::text,'target_id',a.target_id::text,'payment_id',a.payment_id::text,
 'version',a.version,'points_before',a.points_before::text,'points_after',a.points_after::text,
 'delta_points',a.delta_points::text,'ledger_entry_id',a.ledger_entry_id::text,'audit_log_id',a.audit_log_id::text,
 'created_by',a.created_by::text,'reason',a.reason,'point_policy_version',a.point_policy_version::text,'created_at',a.created_at)`

func (s Service) AdjustmentsTx(ctx context.Context, tx pgx.Tx, brand, target string, limit, offset int) (AdjustmentPage, error) {
	out := AdjustmentPage{BrandID: brand, TargetID: target, Items: []Adjustment{}, Limit: limit, Offset: offset}
	if tx == nil || !validAdjustmentPage(brand, limit, offset) || !cycleCanonicalUUID.MatchString(target) {
		return out, ErrInvalid
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM commission_payment_targets WHERE brand_id=$1 AND id=$2)`, brand, target).Scan(&exists); err != nil {
		return out, paymentDBError(err)
	}
	if !exists {
		return out, ErrNotFound
	}
	var raw []byte
	err := tx.QueryRow(ctx, `WITH page AS (
 SELECT a.* FROM commission_adjustments a WHERE a.brand_id=$1 AND a.target_id=$2 ORDER BY a.version DESC,a.id DESC LIMIT $3 OFFSET $4
), contents AS (
 SELECT `+commissionAdjustmentReadJSON+` data,a.created_at,a.version,a.id FROM page a
)
SELECT (SELECT count(*)::text FROM commission_adjustments WHERE brand_id=$1 AND target_id=$2),
 coalesce((SELECT jsonb_agg(data ORDER BY version DESC,id DESC) FROM contents),'[]'::jsonb)`, brand, target, limit, offset).Scan(&out.TotalCount, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, paymentDBError(err)
	}
	if err = json.Unmarshal(raw, &out.Items); err != nil {
		return out, err
	}
	for i := range out.Items {
		out.Items[i].CreatedAt = out.Items[i].CreatedAt.UTC()
	}
	return out, nil
}
