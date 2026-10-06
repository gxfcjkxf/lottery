// Package events writes minimal public business facts to the transactional outbox.
// It never publishes before commit or performs external I/O.
package events

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/jackc/pgx/v5"
)

func Append(ctx context.Context, tx pgx.Tx, brand, kind, resource, member string, points *string) error {
	raw, err := json.Marshal(struct {
		MemberID   string  `json:"member_id"`
		ResourceID string  `json:"resource_id"`
		Points     *string `json:"points"`
	}{member, resource, points})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,brand_id,event_type,aggregate_id,payload) VALUES($1,$2,$3,$4,$5)`, ids.New(), brand, kind, resource, raw)
	return err
}
