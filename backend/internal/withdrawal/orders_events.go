package withdrawal

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/jackc/pgx/v5"
)

// Every newly committed immutable transition carries exactly one minimal inbox
// fact. Historical facts remain valid after a later transition; no private reason,
// policy, qualification, source allocation or payment destination is published.
func appendOrderEvent(ctx context.Context, tx pgx.Tx, o Order) error {
	raw, err := json.Marshal(struct {
		MemberID   string `json:"member_id"`
		ResourceID string `json:"resource_id"`
		Points     string `json:"points"`
		Status     string `json:"status"`
		Version    int64  `json:"version"`
		AuditLogID string `json:"audit_log_id"`
	}{o.MemberID, o.ID, strconv.FormatInt(int64(o.Points), 10), o.State, o.Version, o.AuditLogID})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,brand_id,event_type,aggregate_id,payload) VALUES($1,$2,$3,$4,$5)`, ids.New(), o.BrandID, "withdrawal.order."+o.State, o.ID, raw)
	return err
}
