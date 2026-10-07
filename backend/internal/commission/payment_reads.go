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

func (s Service) PaymentsTx(ctx context.Context, tx pgx.Tx, brand string, limit, offset int) (PaymentPage, error) {
	out := PaymentPage{BrandID: brand, Items: []Payment{}, Limit: limit, Offset: offset}
	if tx == nil || !validPaymentPage(brand, limit, offset) {
		return out, ErrInvalid
	}
	var raw []byte
	err := tx.QueryRow(ctx, `WITH page AS (
	 SELECT p.* FROM commission_payments p WHERE p.brand_id=$1 ORDER BY p.created_at DESC,p.id DESC LIMIT $2 OFFSET $3
	), contents AS (
	 SELECT `+paymentReadJSON+` data,p.created_at,p.id FROM page p
	)
	SELECT (SELECT count(*)::text FROM commission_payments WHERE brand_id=$1),
	 coalesce((SELECT jsonb_agg(data ORDER BY created_at DESC,id DESC) FROM contents),'[]'::jsonb)`, brand, limit, offset).
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
