package commission

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"time"
)

type PaymentHistoryRecord struct {
	ID        string    `json:"id"`
	ActorType string    `json:"actor_type"`
	ActorID   *string   `json:"actor_id"`
	Operation string    `json:"operation"`
	State     *string   `json:"state"`
	CreatedAt time.Time `json:"created_at"`
}
type PaymentHistoryPage struct {
	BrandID    string                 `json:"brand_id"`
	PaymentID  string                 `json:"payment_id"`
	Items      []PaymentHistoryRecord `json:"items"`
	TotalCount string                 `json:"total_count"`
	Limit      int                    `json:"limit"`
	Offset     int                    `json:"offset"`
}

func (s Service) PaymentHistoryTx(ctx context.Context, tx pgx.Tx, brand, payment string, limit, offset int) (PaymentHistoryPage, error) {
	out := PaymentHistoryPage{BrandID: brand, PaymentID: payment, Items: []PaymentHistoryRecord{}, Limit: limit, Offset: offset}
	if tx == nil || !validPaymentPage(brand, limit, offset) || !cycleCanonicalUUID.MatchString(payment) {
		return out, ErrInvalid
	}
	var exists bool
	var raw []byte
	err := tx.QueryRow(ctx, `WITH scope AS (SELECT id FROM commission_payments WHERE brand_id=$1 AND id=$2),
 history AS (SELECT l.id,l.actor_type,l.actor_id,substring(l.action from length('commission.payment.')+1) operation,l.after_json->>'state' state,l.created_at
 FROM retention_audit_logs() l JOIN scope s ON s.id=l.resource_id
 WHERE l.brand_id=$1 AND l.resource_type='commission_payment' AND l.action IN ('commission.payment.create','commission.payment.approve','commission.payment.retry','commission.payment.post','commission.payment.complete','commission.payment.invalidate','commission.payment.fail')
 UNION ALL
 SELECT l.id,l.actor_type,l.actor_id,'adjustment' AS operation,NULL::text AS state,l.created_at
 FROM commission_adjustments a JOIN scope s ON s.id=a.payment_id
 JOIN retention_audit_logs() l ON l.id=a.audit_log_id AND l.brand_id=a.brand_id
 WHERE a.brand_id=$1 AND l.resource_type='commission_adjustment' AND l.action='commission.adjustment.create'),
 page AS (SELECT * FROM history ORDER BY created_at DESC,id DESC LIMIT $3 OFFSET $4)
 SELECT EXISTS(SELECT 1 FROM scope),(SELECT count(*)::text FROM history),coalesce((SELECT jsonb_agg(jsonb_build_object('id',id::text,'actor_type',actor_type,'actor_id',actor_id::text,'operation',operation,'state',state,'created_at',created_at) ORDER BY created_at DESC,id DESC) FROM page),'[]'::jsonb)`, brand, payment, limit, offset).Scan(&exists, &out.TotalCount, &raw)
	if err != nil {
		return out, paymentDBError(err)
	}
	if !exists {
		return out, ErrNotFound
	}
	if err = json.Unmarshal(raw, &out.Items); err != nil {
		return out, err
	}
	return out, nil
}
