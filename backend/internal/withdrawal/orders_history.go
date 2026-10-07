package withdrawal

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type OrderTransition struct {
	ID         string    `json:"id"`
	BrandID    string    `json:"brand_id"`
	OrderID    string    `json:"order_id"`
	Version    int64     `json:"version"`
	FromState  string    `json:"from_state"`
	ToState    string    `json:"to_state"`
	Reason     string    `json:"reason"`
	ActorType  string    `json:"actor_type"`
	ActorID    *string   `json:"actor_id"`
	AuditLogID string    `json:"audit_log_id"`
	CreatedAt  time.Time `json:"created_at"`
}

// History is an internal scoped query. The eventual HTTP handler must perform
// administrator view or member ownership authorization before calling it.
func (s OrderService) History(ctx context.Context, brand, id string) ([]OrderTransition, error) {
	out := []OrderTransition{}
	if s.DB == nil || !validIDs(brand, id) {
		return out, ErrInvalid
	}
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if _, err = s.orderTx(ctx, tx, brand, id, false); err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, `SELECT id::text,brand_id::text,order_id::text,version,from_state,to_state,reason,actor_type,actor_id::text,audit_log_id::text,created_at FROM withdrawal_order_transitions WHERE brand_id=$1 AND order_id=$2 ORDER BY version`, brand, id)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var v OrderTransition
		if err = rows.Scan(&v.ID, &v.BrandID, &v.OrderID, &v.Version, &v.FromState, &v.ToState, &v.Reason, &v.ActorType, &v.ActorID, &v.AuditLogID, &v.CreatedAt); err != nil {
			return out, err
		}
		v.CreatedAt = v.CreatedAt.UTC()
		out = append(out, v)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}
