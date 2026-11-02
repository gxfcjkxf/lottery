package betting

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

func appendConfiguredEvent(ctx context.Context, tx pgx.Tx, brand, member, order, period, kind, payout string, amount points.Amount) error {
	raw, err := json.Marshal(map[string]any{"member_id": member, "order_id": order, "period_id": period, "points": amount, "payout_entry_id": payout})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,brand_id,event_type,aggregate_id,payload) VALUES($1,$2,$3,$4,$5)`, ids.New(), brand, kind, order, raw)
	return err
}
