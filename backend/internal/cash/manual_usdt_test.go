package cash

import (
	"context"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"testing"
)

func TestUniqueUSDTManualReviewCreditsPrincipalOnce(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	meta := points.Metadata{ActorType: "system"}
	d, e := transact(t, f, func(tx pgx.Tx) (Destination, error) {
		return SaveDestination(ctx, tx, f.brand, "", Destination{Currency: "USDT", Channel: "TRON/TRC20", Number: "TLa2f6VPqDgRE67v1736s7bJ8Ray5wYjU7", Enabled: true}, meta)
	})
	if e != nil {
		t.Fatal(e)
	}
	order, e := transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Deposit(ctx, tx, f.brand, f.member, Input{Currency: "USDT", Amount: "10"}, f.meta)
	})
	if e != nil || order.Destination.ID != d.ID || !order.Automatic || order.Fee != "0.000001" {
		t.Fatal(order, e)
	}
	balances, e := transact(t, f, func(tx pgx.Tx) ([]Balance, error) { return Balances(ctx, tx, f.brand, f.member) })
	if e != nil {
		t.Fatal(e)
	}
	for _, b := range balances {
		if b.Currency == "USDT" && b.Deposited != "0" {
			t.Fatal("credited before review", b)
		}
	}
	confirmed, e := transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Advance(ctx, tx, f.brand, "", order.ID, "confirm", Input{Version: order.Version, Amount: "10.000001", Reference: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, meta)
	})
	if e != nil || confirmed.State != "confirmed" {
		t.Fatal(confirmed, e)
	}
	if _, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Advance(ctx, tx, f.brand, "", order.ID, "confirm", Input{Version: order.Version, Amount: "10.000001", Reference: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, meta)
	}); !errors.Is(e, ErrConflict) {
		t.Fatal("duplicate credit", e)
	}
	var credits int
	if e = f.db.QueryRow(ctx, `SELECT count(*) FROM cash_entries WHERE order_id=$1 AND operation='confirm'`, order.ID).Scan(&credits); e != nil || credits != 1 {
		t.Fatal(credits, e)
	}
}
