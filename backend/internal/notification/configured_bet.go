package notification

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
)

func validateConfiguredBetEvent(ctx context.Context, tx pgx.Tx, brand, kind, order string, raw []byte) (string, Payload, error) {
	var p Payload
	if !exactEventJSONKeys(raw, "member_id", "order_id", "period_id", "points", "payout_entry_id") {
		return "", p, ErrInvalid
	}
	var in struct {
		MemberID string `json:"member_id"`
		OrderID  string `json:"order_id"`
		PeriodID string `json:"period_id"`
		Points   string `json:"points"`
		Payout   string `json:"payout_entry_id"`
	}
	if json.Unmarshal(raw, &in) != nil || in.OrderID != order || !uuid.MatchString(in.MemberID) || !uuid.MatchString(in.PeriodID) || !positive(&in.Points) {
		return "", p, ErrInvalid
	}
	var ok bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM retention_configured_orders() o WHERE o.brand_id=$1 AND o.id=$2 AND o.member_id=$3 AND o.period_id=$4 AND
 (($5='bet.order.placed' AND o.total_points=$6::numeric AND $7='') OR
 ($5='bet.order.won' AND o.status='settled' AND (o.result->>'prize_points')::numeric=$6::numeric AND o.payout_entry_id::text=$7 AND
 EXISTS(SELECT 1 FROM retention_point_ledger_entries() l WHERE l.id=o.payout_entry_id AND l.brand_id=o.brand_id AND l.member_id=o.member_id AND l.entry_type='prize' AND l.reference_type='configured_order' AND l.reference_id=o.id AND l.operation_key='configured-prize:'||o.id::text))))`, brand, order, in.MemberID, in.PeriodID, kind, in.Points, in.Payout).Scan(&ok)
	if err != nil {
		return "", p, err
	}
	if !ok {
		return "", p, ErrInvalid
	}
	p = Payload{ResourceID: order, Points: &in.Points}
	return in.MemberID, p, nil
}
