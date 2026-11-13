package cash

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

func TestMemberDestinationCRUDAndScope(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	initial := Destination{Currency: "CNY", Channel: "bank", Holder: "Recipient", Institution: "Bank", Number: "123456", Enabled: true}
	save := func(d Destination, brand, member string) (Destination, error) {
		return transact(t, f, func(tx pgx.Tx) (Destination, error) { return SaveDestination(ctx, tx, brand, member, d, f.meta) })
	}
	remove := func(d Destination, brand, member string) (Destination, error) {
		return transact(t, f, func(tx pgx.Tx) (Destination, error) {
			return DeleteDestination(ctx, tx, brand, member, d.ID, d.Version, f.meta)
		})
	}
	invalidVersion := initial
	invalidVersion.Version = 2
	if _, e := save(invalidVersion, f.brand, f.member); !errors.Is(e, ErrInvalid) {
		t.Fatalf("new binding version: %v", e)
	}
	d, e := save(initial, f.brand, f.member)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = save(initial, f.brand, f.member); !errors.Is(e, ErrDestinationExists) {
		t.Fatalf("duplicate binding: %v", e)
	}
	disabled := d
	disabled.Enabled = false
	if _, e = save(disabled, f.brand, f.member); !errors.Is(e, ErrInvalid) {
		t.Fatalf("member status editing: %v", e)
	}
	wrongCurrency := d
	wrongCurrency.Currency = "USD"
	if _, e = save(wrongCurrency, f.brand, f.member); !errors.Is(e, ErrConflict) {
		t.Fatalf("currency editing: %v", e)
	}
	for _, scope := range [][2]string{{f.brand, ids.New()}, {ids.New(), f.member}} {
		if _, e = save(d, scope[0], scope[1]); !errors.Is(e, ErrConflict) {
			t.Fatalf("cross-scope update: %v", e)
		}
		if _, e = remove(d, scope[0], scope[1]); !errors.Is(e, ErrConflict) {
			t.Fatalf("cross-scope delete: %v", e)
		}
	}
	stale := d
	d.Number = "654321"
	d, e = save(d, f.brand, f.member)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = save(stale, f.brand, f.member); !errors.Is(e, ErrConflict) {
		t.Fatalf("stale edit: %v", e)
	}
	if _, e = remove(stale, f.brand, f.member); !errors.Is(e, ErrConflict) {
		t.Fatalf("stale delete: %v", e)
	}
	// One binding for each supported currency, including a real checksum-valid TRON address.
	for _, c := range []string{"USD", "THB", "USDT"} {
		next := initial
		next.Currency = c
		if c == "USDT" {
			next.Channel = "TRON/TRC20"
			next.Number = "TLa2f6VPqDgRE67v1736s7bJ8Ray5wYjU7"
			next.Holder = ""
			next.Institution = ""
		}
		if _, e = save(next, f.brand, f.member); e != nil {
			t.Fatal(c, e)
		}
	}
	list, e := transact(t, f, func(tx pgx.Tx) ([]Destination, error) { return Destinations(ctx, tx, f.brand, f.member) })
	if e != nil || len(list) != 4 {
		t.Fatal(list, e)
	}
	deleted, e := remove(d, f.brand, f.member)
	if e != nil || deleted.Enabled || deleted.Version != d.Version+1 {
		t.Fatal(deleted, e)
	}
	if _, e = save(d, f.brand, f.member); !errors.Is(e, ErrConflict) {
		t.Fatalf("deleted binding revived: %v", e)
	}
	list, e = transact(t, f, func(tx pgx.Tx) ([]Destination, error) { return Destinations(ctx, tx, f.brand, f.member) })
	if e != nil || len(list) != 3 {
		t.Fatal(list, e)
	}
	replacement, e := save(initial, f.brand, f.member)
	if e != nil || replacement.ID == d.ID {
		t.Fatal(replacement, e)
	}
	var retained int
	if e = f.db.QueryRow(ctx, `SELECT count(*) FROM cash_destinations WHERE id=$1 AND data->>'deleted'='true'`, d.ID).Scan(&retained); e != nil || retained != 1 {
		t.Fatal(retained, e)
	}
	// Brand-owned collection accounts remain a separate pool, without the member constraint.
	if _, e = save(initial, f.brand, ""); e != nil {
		t.Fatal(e)
	}
}

func TestConcurrentMemberDestinationCreation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			tx, e := f.db.Begin(ctx)
			if e != nil {
				results <- e
				return
			}
			defer tx.Rollback(ctx)
			_, e = SaveDestination(ctx, tx, f.brand, f.member, Destination{Currency: "CNY", Channel: "bank", Holder: "Recipient", Institution: "Bank", Number: "123456", Enabled: true}, f.meta)
			if e == nil {
				e = tx.Commit(ctx)
			}
			results <- e
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	success, duplicate := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else if errors.Is(e, ErrDestinationExists) {
			duplicate++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || duplicate != 1 {
		t.Fatalf("success=%d duplicate=%d", success, duplicate)
	}
}

func TestDeletedDestinationCannotWithdrawOrChangeOrderSnapshot(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	d, e := transact(t, f, func(tx pgx.Tx) (Destination, error) {
		return SaveDestination(ctx, tx, f.brand, f.member, Destination{Currency: "CNY", Channel: "bank", Holder: "Original", Institution: "Bank", Number: "123456", Enabled: true}, f.meta)
	})
	if e != nil {
		t.Fatal(e)
	}
	enablePointConversion(t, f)
	if _, e = transact(t, f, func(tx pgx.Tx) (points.Entry, error) {
		var delta points.Balance
		delta[1][0] = 7000
		return f.s.Points.Post(ctx, tx, points.Change{BrandID: f.brand, MemberID: f.member, EntryType: "adjustment", ReferenceType: "cash_test", OperationKey: ids.New(), ActorType: "system", RequestID: ids.New(), Delta: delta, Allocation: []points.Allocation{{Source: "winning", State: "available", Points: 7000}}})
	}); e != nil {
		t.Fatal(e)
	}
	if _, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Exchange(ctx, tx, f.brand, f.member, Input{Currency: "CNY", Amount: "70", Direction: "from_points", Rate: "7"}, f.meta)
	}); e != nil {
		t.Fatal(e)
	}
	input := Input{Currency: "CNY", Amount: "10", DestinationID: d.ID, DestinationVersion: d.Version, Fee: "0"}
	stale := input
	d.Number = "654321"
	d, e = transact(t, f, func(tx pgx.Tx) (Destination, error) { return SaveDestination(ctx, tx, f.brand, f.member, d, f.meta) })
	if e != nil {
		t.Fatal(e)
	}
	if _, e = transact(t, f, func(tx pgx.Tx) (Order, error) { return f.s.Withdraw(ctx, tx, f.brand, f.member, stale, f.meta) }); !errors.Is(e, ErrConflict) {
		t.Fatalf("stale withdrawal: %v", e)
	}
	input.DestinationVersion = d.Version
	o, e := transact(t, f, func(tx pgx.Tx) (Order, error) { return f.s.Withdraw(ctx, tx, f.brand, f.member, input, f.meta) })
	if e != nil {
		t.Fatal(e)
	}
	if _, e = transact(t, f, func(tx pgx.Tx) (Destination, error) {
		return DeleteDestination(ctx, tx, f.brand, f.member, d.ID, d.Version, f.meta)
	}); e != nil {
		t.Fatal(e)
	}
	stored, e := transact(t, f, func(tx pgx.Tx) (Order, error) { return ReadOrder(ctx, tx, f.brand, f.member, o.ID) })
	if e != nil || stored.Destination.Number != "654321" || stored.Destination.Version != d.Version {
		t.Fatal(stored, e)
	}
	if _, e = transact(t, f, func(tx pgx.Tx) (Order, error) { return f.s.Withdraw(ctx, tx, f.brand, f.member, input, f.meta) }); !errors.Is(e, ErrAccount) {
		t.Fatalf("deleted withdrawal: %v", e)
	}
	balances, e := transact(t, f, func(tx pgx.Tx) ([]Balance, error) { return Balances(ctx, tx, f.brand, f.member) })
	if e != nil || balances[0].Withdrawable != "0" || balances[0].Reserved != "10" {
		t.Fatal(balances, e)
	}
}
