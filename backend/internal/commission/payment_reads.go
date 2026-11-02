package commission

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

func (s Service) PaymentPolicyTx(ctx context.Context, tx pgx.Tx, brand string) (PaymentPolicy, error) {
	var out PaymentPolicy
	if tx == nil || !cycleCanonicalUUID.MatchString(brand) {
		return out, ErrInvalid
	}
	var auditID *string
	err := tx.QueryRow(ctx, `SELECT brand_id::text,version,enabled,audit_log_id::text,updated_at
		FROM brand_commission_payment_policies WHERE brand_id=$1`, brand).
		Scan(&out.BrandID, &out.Version, &out.Enabled, &auditID, &out.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return PaymentPolicy{}, ErrNotFound
	}
	if err != nil {
		return PaymentPolicy{}, paymentDBError(err)
	}
	if auditID != nil {
		out.AuditLogID = *auditID
	}
	out.UpdatedAt = out.UpdatedAt.UTC()
	return out, nil
}

const paymentReadJSON = `jsonb_build_object(
 'id',p.id::text,'brand_id',p.brand_id::text,'cycle_id',p.cycle_id::text,'run_id',p.run_id::text,
 'state',p.state,'payout_mode',p.payout_mode,'version',p.version,'evidence_epoch',p.evidence_epoch::text,
 'total_points',p.total_points::text,'paid_points',coalesce((SELECT sum(t.points)::text FROM commission_payment_targets t WHERE t.brand_id=p.brand_id AND t.payment_id=p.id AND t.state='paid'),'0'),
 'target_count',p.target_count::text,'paid_count',coalesce((SELECT count(*)::text FROM commission_payment_targets t WHERE t.brand_id=p.brand_id AND t.payment_id=p.id AND t.state='paid'),'0'),
 'creation_audit_log_id',p.creation_audit_log_id::text,'last_error_code',p.last_error_code,
 'created_at',p.created_at,'updated_at',p.updated_at)`

func (s Service) PaymentTx(ctx context.Context, tx pgx.Tx, brand, id string) (Payment, error) {
	var out Payment
	if tx == nil || !cycleCanonicalUUID.MatchString(brand) || !cycleCanonicalUUID.MatchString(id) {
		return out, ErrInvalid
	}
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT `+paymentReadJSON+` FROM commission_payments p WHERE p.brand_id=$1 AND p.id=$2`, brand, id).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return Payment{}, ErrNotFound
	}
	if err != nil {
		return Payment{}, paymentDBError(err)
	}
	if err = json.Unmarshal(raw, &out); err != nil {
		return Payment{}, err
	}
	out.CreatedAt = out.CreatedAt.UTC()
	out.UpdatedAt = out.UpdatedAt.UTC()
	return out, nil
}

func (s Service) PaymentsTx(ctx context.Context, tx pgx.Tx, brand string, limit, offset int, filter ListFilter) (PaymentPage, error) {
	out := PaymentPage{BrandID: brand, Items: []Payment{}, Limit: limit, Offset: offset}
	if tx == nil || !validPaymentPage(brand, limit, offset) || filter.Validate("payment") != nil {
		return out, ErrInvalid
	}
	if err := filter.validateScope(ctx, tx, brand); err != nil {
		return out, err
	}
	var raw []byte
	err := tx.QueryRow(ctx, `WITH filtered AS (
 SELECT p.*,c.window_to FROM commission_payments p JOIN commission_cycles c ON c.brand_id=p.brand_id AND c.id=p.cycle_id
 WHERE p.brand_id=$1
 AND ($4::timestamptz IS NULL OR c.window_to >= $4) AND ($5::timestamptz IS NULL OR c.window_to < $5)
 AND ($6='' OR p.state=$6 OR ($6='needs_attention' AND (
 p.state='failed' OR (p.state='blocked' AND NOT EXISTS (
  SELECT 1 FROM commission_correction_plans cp JOIN commission_correction_executions cx ON cx.brand_id=cp.brand_id AND cx.plan_id=cp.id
  WHERE cp.brand_id=p.brand_id AND cp.payment_id=p.id AND cp.state='ready'
   AND c.state='ready' AND cp.run_id=c.current_run_id AND cp.evidence_epoch=c.evidence_epoch AND cx.state='completed'
 )))))
 AND ($7='' OR p.cycle_id::text=$7)
 AND ($8='' OR EXISTS(SELECT 1 FROM commission_earnings e WHERE e.brand_id=p.brand_id AND e.cycle_id=p.cycle_id AND e.run_id=p.run_id AND e.agent_id::text=$8))
), page AS (
 SELECT * FROM filtered ORDER BY window_to DESC,created_at DESC,id DESC LIMIT $2 OFFSET $3
), contents AS (
 SELECT `+paymentReadJSON+` data,p.window_to,p.created_at,p.id FROM page p
)
SELECT (SELECT count(*)::text FROM filtered),
 coalesce((SELECT jsonb_agg(data ORDER BY window_to DESC,created_at DESC,id DESC) FROM contents),'[]'::jsonb)`, brand, limit, offset, filter.From, filter.To, filter.State, filter.CycleID, filter.AgentID).
		Scan(&out.TotalCount, &raw)
	if err != nil {
		return out, paymentDBError(err)
	}
	if err = json.Unmarshal(raw, &out.Items); err != nil {
		return out, err
	}
	for i := range out.Items {
		out.Items[i].CreatedAt = out.Items[i].CreatedAt.UTC()
		out.Items[i].UpdatedAt = out.Items[i].UpdatedAt.UTC()
	}
	return out, nil
}
