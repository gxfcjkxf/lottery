package cash

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestListStateFiltersRowsAndTotal(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	order, err := transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Deposit(ctx, tx, f.brand, f.member, Input{Currency: "CNY", Amount: "10"}, f.meta)
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Advance(ctx, tx, f.brand, f.member, order.ID, "cancel", Input{Version: order.Version}, f.meta)
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		state, total  string
		offset, count int
	}{
		{"", "1", 0, 1}, {"cancelled", "1", 0, 1}, {"pending", "0", 0, 0}, {"cancelled", "1", 1, 0},
	} {
		result, err := transact(t, f, func(tx pgx.Tx) (map[string]any, error) {
			return List(ctx, tx, f.brand, "", "deposit", 1, tc.offset, tc.state)
		})
		if err != nil {
			t.Fatal(err)
		}
		items := result["items"].([]Order)
		if result["total_count"] != tc.total || len(items) != tc.count {
			t.Fatalf("%+v: %+v", tc, result)
		}
		if len(items) > 0 && items[0].State != "cancelled" {
			t.Fatal(items)
		}
	}
	_, err = transact(t, f, func(tx pgx.Tx) (map[string]any, error) {
		return List(ctx, tx, f.brand, "", "deposit", 10, 0, "completed")
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid deposit state: %v", err)
	}
}
