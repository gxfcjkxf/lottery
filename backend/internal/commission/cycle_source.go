package commission

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// CycleCursor is a keyset position in the immutable bet-placement ordering.
// It is internal worker state, not a public or financial authorization token.
type CycleCursor struct {
	PlacedAt time.Time
	OrderID  string
}

type CycleOrder struct {
	OrderID  string
	PlacedAt time.Time
	Resolution
}

type CyclePage struct {
	Items []CycleOrder
	Next  *CycleCursor
}

// CyclePageTx enumerates bets by placement time in one configured cycle,
// including non-final and excluded bets. In particular a late settlement never
// moves a bet to another cycle, and not_final is not a zero commission.
//
// The caller must supply the calendar saved for that cycle, not today's policy.
// This reader does not grant payout permission, create financial records, or
// certify that the whole cycle is ready. FinalOrderTx retains each period lock
// until the caller ends its transaction. On any error no partial page is usable;
// the caller must roll back. Later finalization/correction requires another pass
// from the beginning; a cursor is not a completion marker.
func (s Source) CyclePageTx(ctx context.Context, tx pgx.Tx, brand string, calendar Calendar, window Window, cursor *CycleCursor, limit int) (CyclePage, error) {
	if tx == nil || !uuid.MatchString(brand) || limit < 1 || limit > 100 {
		return CyclePage{}, ErrInvalid
	}
	expected, err := calendar.WindowAt(window.From)
	if err != nil {
		return CyclePage{}, err
	}
	if !expected.From.Equal(window.From) || !expected.To.Equal(window.To) {
		return CyclePage{}, ErrInvalid
	}
	var brandExists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM brands WHERE id=$1)`, brand).Scan(&brandExists); err != nil {
		return CyclePage{}, err
	}
	if !brandExists {
		return CyclePage{}, ErrNotFound
	}
	var at any
	var id any
	if cursor != nil {
		if !uuid.MatchString(cursor.OrderID) || cursor.PlacedAt.Before(window.From) || !cursor.PlacedAt.Before(window.To) {
			return CyclePage{}, ErrInvalid
		}
		var exists bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM bet_orders WHERE brand_id=$1 AND id=$2 AND placed_at=$3)`, brand, cursor.OrderID, cursor.PlacedAt).Scan(&exists)
		if err != nil {
			return CyclePage{}, err
		}
		if !exists {
			return CyclePage{}, ErrInvalid
		}
		at, id = cursor.PlacedAt, cursor.OrderID
	}
	rows, err := tx.Query(ctx, `SELECT id::text,placed_at FROM bet_orders
 WHERE brand_id=$1 AND placed_at >= $2 AND placed_at < $3
 AND ($4::timestamptz IS NULL OR (placed_at,id)>($4::timestamptz,$5::uuid))
 ORDER BY placed_at,id LIMIT $6`, brand, window.From, window.To, at, id, limit+1)
	if err != nil {
		return CyclePage{}, err
	}
	keys := make([]CycleCursor, 0, limit+1)
	for rows.Next() {
		var key CycleCursor
		if err = rows.Scan(&key.OrderID, &key.PlacedAt); err != nil {
			rows.Close()
			return CyclePage{}, err
		}
		keys = append(keys, key)
	}
	err = rows.Err()
	rows.Close() // release the connection before nested witness queries
	if err != nil {
		return CyclePage{}, err
	}
	page := CyclePage{Items: make([]CycleOrder, 0, limit)}
	if len(keys) > limit {
		last := keys[limit-1]
		page.Next = &last
		keys = keys[:limit]
	}
	for _, key := range keys {
		resolution, err := s.FinalOrderTx(ctx, tx, brand, key.OrderID)
		if err != nil {
			return CyclePage{}, err
		}
		if resolution.Fact != nil && !resolution.Fact.PlacedAt.Equal(key.PlacedAt) {
			return CyclePage{}, ErrEvidence
		}
		page.Items = append(page.Items, CycleOrder{OrderID: key.OrderID, PlacedAt: key.PlacedAt, Resolution: resolution})
	}
	return page, nil
}
