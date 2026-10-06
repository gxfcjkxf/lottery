package betting

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

type Exception struct {
	ID           string    `json:"id"`
	BrandID      string    `json:"brand_id"`
	OrderID      string    `json:"order_id"`
	OrderVersion int64     `json:"order_version"`
	MarkedBy     string    `json:"marked_by"`
	Reason       string    `json:"reason"`
	CreatedAt    time.Time `json:"created_at"`
}

func (s Service) Exception(ctx context.Context, brand, id string) (*Exception, error) {
	if _, err := s.Order(ctx, brand, "", id); err != nil {
		return nil, err
	}
	var out Exception
	err := s.DB.QueryRow(ctx, `SELECT id::text,brand_id::text,order_id::text,order_version,marked_by::text,reason,created_at FROM bet_order_exceptions WHERE brand_id=$1 AND order_id=$2`, brand, id).Scan(&out.ID, &out.BrandID, &out.OrderID, &out.OrderVersion, &out.MarkedBy, &out.Reason, &out.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out.CreatedAt = out.CreatedAt.UTC()
	return &out, nil
}

// MarkAbnormal changes only the state and appends evidence. It neither refunds
// the stake nor calculates a prize; ordinary settlement must exclude this state.
func (s Service) MarkAbnormal(ctx context.Context, tx pgx.Tx, brand string, a access.Account, id string, version int64, reason string, meta points.Metadata) (Order, error) {
	if tx == nil || !validIDs(brand, id, a.ID) || !validPolicyReason(reason) {
		return Order{}, ErrInvalid
	}
	if a.SuperAdmin || !access.Authorize(a, "bet", "mark_abnormal", access.ScopeBrand, brand) {
		return Order{}, ErrDenied
	}
	o, err := scanOrder(tx.QueryRow(ctx, `SELECT `+orderFields+` FROM bet_orders WHERE brand_id=$1 AND id=$2`, brand, id))
	if err != nil {
		return o, err
	}
	// Same game -> period -> order ordering as cancellation. No wallet or quota
	// lock is needed because neither money nor exposure changes here.
	if _, _, _, err = s.lockPeriod(ctx, tx, brand, o.PeriodID); err != nil {
		return o, err
	}
	o, err = scanOrder(tx.QueryRow(ctx, `SELECT `+orderFields+` FROM bet_orders WHERE brand_id=$1 AND id=$2 FOR UPDATE`, brand, id))
	if err != nil {
		return o, err
	}
	if version != o.Version || version == math.MaxInt64 {
		return o, ErrVersion
	}
	if o.Status != "placed" {
		return o, ErrState
	}
	reason = strings.TrimSpace(reason)
	evidenceID := ids.New()
	if _, err = tx.Exec(ctx, `INSERT INTO bet_order_exceptions(id,brand_id,order_id,order_version,marked_by,reason) VALUES($1,$2,$3,$4,$5,$6)`, evidenceID, brand, o.ID, version+1, a.ID, reason); err != nil {
		return o, err
	}
	o, err = scanOrder(tx.QueryRow(ctx, `UPDATE bet_orders SET status='abnormal',version=version+1 WHERE brand_id=$1 AND id=$2 RETURNING `+orderFields, brand, o.ID))
	if err != nil {
		return o, err
	}
	if err = appendEvent(ctx, tx, o, "bet.order.abnormal"); err != nil {
		return o, err
	}
	_, err = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "bet.mark_abnormal", ResourceType: "bet_order", ResourceID: o.ID, Reason: reason, RequestID: meta.RequestID, IP: meta.IP, Before: map[string]any{"version": version, "status": "placed"}, After: map[string]any{"version": o.Version, "status": o.Status, "exception_id": evidenceID}})
	return o, err
}
