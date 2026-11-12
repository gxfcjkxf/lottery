package cash

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

func usdtOrder(t *testing.T, f fixture, amount string) Order {
	t.Helper()
	ctx := context.Background()
	_, e := transact(t, f, func(tx pgx.Tx) (Destination, error) {
		return SaveDestination(ctx, tx, f.brand, "", Destination{Currency: "USDT", Channel: "TRON/TRC20", Number: "TLa2f6VPqDgRE67v1736s7bJ8Ray5wYjU7", Enabled: true}, points.Metadata{ActorType: "system"})
	})
	if e != nil {
		t.Fatal(e)
	}
	o, e := transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Deposit(ctx, tx, f.brand, f.member, Input{Currency: "USDT", Amount: amount}, f.meta)
	})
	if e != nil {
		t.Fatal(e)
	}
	return o
}
func transferFor(t *testing.T, f fixture, o Order) USDTTransfer {
	t.Helper()
	var at time.Time
	if e := f.db.QueryRow(context.Background(), `SELECT created_at+interval '1 second' FROM cash_orders WHERE id=$1`, o.ID).Scan(&at); e != nil {
		t.Fatal(e)
	}
	a, _ := Parse(o.Amount, "USDT")
	fee, _ := Parse(o.Fee, "USDT")
	return USDTTransfer{Transaction: strings.Repeat("a", 64), Index: 2, Sender: o.Destination.Number, Address: o.Destination.Number, Amount: a + fee, Time: at}
}
func TestUniqueUSDTConcurrentCreditAndFee(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	o := usdtOrder(t, f, "100")
	transfer := transferFor(t, f, o)
	if !o.Automatic || o.Amount != "100" || o.Fee != "0.000001" {
		t.Fatal(o)
	}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	credited := make(chan bool, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, e := f.db.Begin(ctx)
			if e != nil {
				results <- e
				return
			}
			defer tx.Rollback(ctx)
			ok, e := f.s.CreditUSDT(ctx, tx, transfer)
			if e == nil {
				e = tx.Commit(ctx)
			}
			results <- e
			credited <- ok
		}()
	}
	wg.Wait()
	close(results)
	close(credited)
	for e := range results {
		if e != nil {
			t.Fatal(e)
		}
	}
	n := 0
	for ok := range credited {
		if ok {
			n++
		}
	}
	if n != 1 {
		t.Fatal("credit count", n)
	}
	var balance, received int64
	var count int
	if e := f.db.QueryRow(ctx, `SELECT deposited FROM cash_balances WHERE brand_id=$1 AND member_id=$2 AND currency='USDT'`, f.brand, f.member).Scan(&balance); e != nil || balance != 100000000 {
		t.Fatal(balance, e)
	}
	if e := f.db.QueryRow(ctx, `SELECT count(*),max(received) FROM cash_usdt_receipts WHERE order_id=$1`, o.ID).Scan(&count, &received); e != nil || count != 1 || received != 100000001 {
		t.Fatal(count, received, e)
	}
	// A subsequent order cannot reuse the identity, even after settlement.
	next, e := transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Deposit(ctx, tx, f.brand, f.member, Input{Currency: "USDT", Amount: "100"}, f.meta)
	})
	if e != nil || next.Fee != "0.000002" {
		t.Fatal(next, e)
	}
}
func TestUniqueUSDTRejectsUnmatchedAndCancelledPayments(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	o := usdtOrder(t, f, "100")
	valid := transferFor(t, f, o)
	cases := []USDTTransfer{valid, valid, valid}
	cases[0].Amount--
	cases[1].Time = valid.Time.Add(-time.Hour)
	cases[2].Time = valid.Time.Add(time.Hour)
	for i, v := range cases {
		ok, e := transact(t, f, func(tx pgx.Tx) (bool, error) { return f.s.CreditUSDT(ctx, tx, v) })
		if e != nil || ok {
			t.Fatal(i, ok, e)
		}
	}
	_, e := transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Advance(ctx, tx, f.brand, f.member, o.ID, "cancel", Input{Version: 1}, f.meta)
	})
	if e != nil {
		t.Fatal(e)
	}
	next, e := transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Deposit(ctx, tx, f.brand, f.member, Input{Currency: "USDT", Amount: "100"}, f.meta)
	})
	if e != nil || next.Fee == o.Fee {
		t.Fatal(next, e)
	}
	ok, e := transact(t, f, func(tx pgx.Tx) (bool, error) { return f.s.CreditUSDT(ctx, tx, valid) })
	if e != nil || ok {
		t.Fatal("late cancelled payment", ok, e)
	}
}
func TestUniqueUSDTGlobalAddressIdentity(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	o := usdtOrder(t, f, "100")
	// Global reservations do not depend on the requesting brand's rotation lock.
	var fee int64
	e := f.db.QueryRow(ctx, `SELECT payable-100000000 FROM cash_usdt_amounts WHERE order_id=$1`, o.ID).Scan(&fee)
	if e != nil || fee != 1 {
		t.Fatal(fee, e)
	}
	got, e := transact(t, f, func(tx pgx.Tx) (int64, error) { return reserveUniqueAmount(ctx, tx, o.Destination.Number, 100000000) })
	if e != nil || got != 2 {
		t.Fatal(got, e)
	}
	// Receipt hash consumption by manual review must block automatic replay.
	_, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Advance(ctx, tx, f.brand, "", o.ID, "confirm", Input{Version: 1, Amount: "100.000001", Reference: strings.Repeat("a", 64)}, points.Metadata{ActorType: "admin"})
	})
	if e != nil {
		t.Fatal(e)
	}
	ok, e := transact(t, f, func(tx pgx.Tx) (bool, error) { return f.s.CreditUSDT(ctx, tx, transferFor(t, f, o)) })
	if e != nil || ok {
		t.Fatal(ok, e)
	}
}
func TestTRONReceiptContractAndEventVerification(t *testing.T) {
	r := tronReceipt{ID: strings.Repeat("b", 64), Timestamp: time.Now().UnixMilli()}
	// USDT contract's own address gives a known canonical hex/base58 pair.
	to, e := tronAddress(strings.Repeat("0", 24) + usdtHex)
	if e != nil || to != USDTContract {
		t.Fatal(to, e)
	}
	log := struct {
		Address string   `json:"address"`
		Topics  []string `json:"topics"`
		Data    string   `json:"data"`
	}{usdtHex, []string{transferTopic, strings.Repeat("0", 24) + usdtHex, strings.Repeat("0", 24) + usdtHex}, fmt.Sprintf("%064x", 100000001)}
	r.Logs = append(r.Logs, log)
	transfers, e := r.transfers(USDTContract)
	if e != nil || len(transfers) != 1 || transfers[0].Amount != 100000001 || transfers[0].Index != 0 {
		t.Fatal(transfers, e)
	}
	r.Logs[0].Address = strings.Repeat("1", 40)
	transfers, e = r.transfers(USDTContract)
	if e != nil || len(transfers) != 0 {
		t.Fatal("fake token", transfers, e)
	}
}

func TestUniqueUSDTSharedAddressAcrossBrands(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	first := usdtOrder(t, f, "100")
	second := f
	second.brand = ids.New()
	second.member = ids.New()
	second.user = ids.New()
	second.meta = points.Metadata{ActorType: "user", ActorID: second.user}
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO brands(id,code,name,status) VALUES($1,$2,'Second isolated brand','active')`, []any{second.brand, "cash_test_" + second.brand}},
		{`INSERT INTO global_users(id,username,password_hash) VALUES($1,$2,'test-only')`, []any{second.user, "cash_" + second.user}},
		{`INSERT INTO brand_members(id,brand_id,global_user_id,join_method) VALUES($1,$2,$3,'domain')`, []any{second.member, second.brand, second.user}},
	} {
		if _, e := f.db.Exec(ctx, q.sql, q.args...); e != nil {
			t.Fatal(e)
		}
	}
	_, e := transact(t, f, func(tx pgx.Tx) (Config, error) {
		c, e := ReadConfig(ctx, tx, f.brand)
		if e != nil {
			return c, e
		}
		c.Version = 0
		return SaveConfig(ctx, tx, second.brand, c, points.Metadata{ActorType: "system"})
	})
	if e != nil {
		t.Fatal(e)
	}
	other := usdtOrder(t, second, "100")
	if other.Fee == first.Fee {
		t.Fatal("shared address reused across brands")
	}
	transfer := transferFor(t, second, other)
	ok, e := transact(t, second, func(tx pgx.Tx) (bool, error) { return second.s.CreditUSDT(ctx, tx, transfer) })
	if e != nil || !ok {
		t.Fatal(ok, e)
	}
	var state string
	if e = f.db.QueryRow(ctx, `SELECT state FROM cash_orders WHERE id=$1 AND brand_id=$2`, first.ID, f.brand).Scan(&state); e != nil || state != "pending" {
		t.Fatal("wrong brand credited", state, e)
	}
	var value int64
	if e = f.db.QueryRow(ctx, `SELECT deposited FROM cash_balances WHERE brand_id=$1 AND member_id=$2 AND currency='USDT'`, second.brand, second.member).Scan(&value); e != nil || value != 100000000 {
		t.Fatal(value, e)
	}
}

func TestUniqueUSDTMultipleLogsAndManualHashConflict(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	o := usdtOrder(t, f, "100")
	transfer := transferFor(t, f, o)
	transfer.Time = time.Now().Add(time.Minute)
	credit := func(v USDTTransfer) {
		t.Helper()
		ok, e := transact(t, f, func(tx pgx.Tx) (bool, error) { return f.s.CreditUSDT(ctx, tx, v) })
		if e != nil || !ok {
			t.Fatal(ok, e)
		}
	}
	credit(transfer)
	next, e := transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Deposit(ctx, tx, f.brand, f.member, Input{Currency: "USDT", Amount: "100"}, f.meta)
	})
	if e != nil {
		t.Fatal(e)
	}
	second := transferFor(t, f, next)
	second.Index = 3
	second.Time = transfer.Time
	credit(second)
	third, e := transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Deposit(ctx, tx, f.brand, f.member, Input{Currency: "USDT", Amount: "100"}, f.meta)
	})
	if e != nil {
		t.Fatal(e)
	}
	_, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Advance(ctx, tx, f.brand, "", third.ID, "confirm", Input{Version: 1, Amount: "100.000003", Reference: transfer.Transaction}, points.Metadata{ActorType: "admin"})
	})
	if e == nil {
		t.Fatal("manual review reused automatic hash")
	}
}

func TestUniqueUSDTDoesNotReuseLegacyRequestedAmount(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	d, e := transact(t, f, func(tx pgx.Tx) (Destination, error) {
		return SaveDestination(ctx, tx, f.brand, "", Destination{Currency: "USDT", Channel: "TRON/TRC20", Number: "TLa2f6VPqDgRE67v1736s7bJ8Ray5wYjU7", Enabled: true}, points.Metadata{ActorType: "system"})
	})
	if e != nil {
		t.Fatal(e)
	}
	legacy := Order{ID: ids.New(), BrandID: f.brand, MemberID: f.member, Kind: "deposit", Currency: "USDT", Amount: "100.000001", Points: "0", Fee: "0", State: "pending", Version: 1, Destination: d}
	_, e = transact(t, f, func(tx pgx.Tx) (Order, error) { return legacy, insert(ctx, tx, legacy, true) })
	if e != nil {
		t.Fatal(e)
	}
	_, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Advance(ctx, tx, f.brand, "", legacy.ID, "confirm", Input{Version: 1, Amount: "100.5", Reference: strings.Repeat("b", 64)}, points.Metadata{ActorType: "admin"})
	})
	if e != nil {
		t.Fatal(e)
	}
	next, e := transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Deposit(ctx, tx, f.brand, f.member, Input{Currency: "USDT", Amount: "100"}, f.meta)
	})
	if e != nil || next.Fee != "0.000002" {
		t.Fatal("reused historical requested amount", next, e)
	}
}
