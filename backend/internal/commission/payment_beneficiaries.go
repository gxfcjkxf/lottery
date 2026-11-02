package commission

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"time"
)

// PaymentBeneficiary uses the saved earning identity even before a worker has
// created a target. TargetID and posting fields describe actual execution only.
type PaymentBeneficiary struct {
	ID             string         `json:"id"`
	BrandID        string         `json:"brand_id"`
	PaymentID      string         `json:"payment_id"`
	AgentID        string         `json:"agent_id"`
	MemberID       string         `json:"member_id"`
	TargetID       *string        `json:"target_id"`
	OriginalPoints points.Amount  `json:"original_points"`
	AdjustedPoints *points.Amount `json:"adjusted_points"`
	State          string         `json:"state"`
	PaidAt         *time.Time     `json:"paid_at"`
}
type PaymentBeneficiaryPage struct {
	BrandID    string               `json:"brand_id"`
	PaymentID  string               `json:"payment_id"`
	Items      []PaymentBeneficiary `json:"items"`
	TotalCount string               `json:"total_count"`
	Limit      int                  `json:"limit"`
	Offset     int                  `json:"offset"`
}

func (s Service) PaymentBeneficiariesTx(ctx context.Context, tx pgx.Tx, brand, payment string, limit, offset int, filter ListFilter) (PaymentBeneficiaryPage, error) {
	out := PaymentBeneficiaryPage{BrandID: brand, PaymentID: payment, Items: []PaymentBeneficiary{}, Limit: limit, Offset: offset}
	if tx == nil || !validPaymentPage(brand, limit, offset) || !cycleCanonicalUUID.MatchString(payment) || filter.Validate("target") != nil {
		return out, ErrInvalid
	}
	if err := filter.validateScope(ctx, tx, brand); err != nil {
		return out, err
	}
	var exists bool
	var raw []byte
	err := tx.QueryRow(ctx, `WITH payment AS (
 SELECT * FROM commission_payments WHERE brand_id=$1 AND id=$2
 ), filtered AS (
 SELECT e.id,e.brand_id,p.id payment_id,e.agent_id,e.member_id,t.id target_id,e.points original_points,h.points adjusted_points,
 CASE WHEN t.id IS NULL THEN 'pending' ELSE t.state END state,t.paid_at
 FROM payment p JOIN commission_earnings e ON e.brand_id=p.brand_id AND e.cycle_id=p.cycle_id AND e.run_id=p.run_id
 LEFT JOIN commission_payment_targets t ON t.brand_id=p.brand_id AND t.payment_id=p.id AND t.earning_id=e.id
 LEFT JOIN commission_adjustment_heads h ON h.brand_id=t.brand_id AND h.target_id=t.id
 WHERE ($5='' OR e.agent_id::text=$5) AND ($6='' OR CASE WHEN t.id IS NULL THEN 'pending' ELSE t.state END=$6)
 ), page AS (SELECT * FROM filtered ORDER BY agent_id,id LIMIT $3 OFFSET $4)
 SELECT EXISTS(SELECT 1 FROM payment),(SELECT count(*)::text FROM filtered),
 coalesce((SELECT jsonb_agg(jsonb_build_object('id',id::text,'brand_id',brand_id::text,'payment_id',payment_id::text,'agent_id',agent_id::text,'member_id',member_id::text,'target_id',target_id::text,'original_points',original_points::text,'adjusted_points',adjusted_points::text,'state',state,'paid_at',paid_at) ORDER BY agent_id,id) FROM page),'[]'::jsonb)`, brand, payment, limit, offset, filter.AgentID, filter.State).Scan(&exists, &out.TotalCount, &raw)
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
