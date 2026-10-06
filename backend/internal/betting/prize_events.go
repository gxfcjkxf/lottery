package betting

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

// Queue after the wallet and immutable target witness are written, in the same
// transaction. points is the actual prize, never the original stake. Notification
// consumers verify this immutable evidence even after the order is reset.
func appendPrizeEvent(ctx context.Context, tx pgx.Tx, o Order, kind, calc, entry, job, correction, original string, amount points.Amount) error {
	if amount <= 0 {
		return ErrInvalid
	}
	raw, e := json.Marshal(map[string]any{"member_id": o.MemberID, "order_id": o.ID, "period_id": o.PeriodID, "points": amount, "calculation_id": calc, "payout_entry_id": entry, "job_id": job, "correction_id": correction, "original_payout_entry_id": original})
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO outbox_events(id,brand_id,event_type,aggregate_id,payload) VALUES($1,$2,$3,$4,$5)`, ids.New(), o.BrandID, kind, o.ID, raw)
	return e
}
