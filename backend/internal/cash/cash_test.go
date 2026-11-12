package cash

import (
	"context"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/gxfcjkxf/lottery/backend/internal/withdrawal"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"sync"
	"testing"
)

func TestExactCurrencyAndExchange(t *testing.T) {
	for _, c := range []struct {
		value, currency string
		valid           bool
	}{{"0.01", "CNY", true}, {"0.001", "USD", false}, {"0.000001", "USDT", true}, {"1e3", "USDT", false}, {"-1", "THB", false}, {"1", "BTC", false}} {
		n, e := Parse(c.value, c.currency)
		if (e == nil) != c.valid {
			t.Fatalf("%+v: %v", c, e)
		}
		if e == nil && Format(n) != c.value {
			t.Fatalf("roundtrip %s", Format(n))
		}
	}
	for _, c := range []struct {
		amount   int64
		rate     string
		to       bool
		currency string
		want     int64
	}{{1000000, "7", true, "USDT", 700}, {100, "7", false, "USDT", 142857}, {100, "7", false, "CNY", 140000}, {1234567, "3.333333", true, "USDT", 411}} {
		got, e := Convert(c.amount, c.rate, c.to, c.currency)
		if e != nil || got != c.want {
			t.Fatalf("%+v got %d %v", c, got, e)
		}
	}
	if !ValidTRON("TLa2f6VPqDgRE67v1736s7bJ8Ray5wYjU7") || ValidTRON("TLa2f6VPqDgRE67v1736s7bJ8Ray5wYjU8") {
		t.Fatal("TRON checksum")
	}
}

type fixture struct {
	db                  *pgxpool.Pool
	s                   Service
	brand, member, user string
	meta                points.Metadata
	merchant            Destination
}

func setup(t *testing.T) fixture {
	t.Helper()
	p := testdb.NewUnseeded(t)
	ctx := context.Background()
	f := fixture{db: p, brand: ids.New(), member: ids.New(), user: ids.New()}
	f.meta = points.Metadata{ActorType: "user", ActorID: f.user, RequestID: ids.New()}
	store := points.Store{DB: p}
	f.s = Service{Points: store, Withdrawal: withdrawal.OrderService{DB: p, Points: store, Eligibility: withdrawal.TurnoverChecker{}}}
	for _, q := range []struct {
		sql  string
		args []any
	}{{`INSERT INTO brands(id,code,name,status) VALUES($1,$2,'Cash isolated test','active')`, []any{f.brand, "cash_test_" + f.brand}}, {`INSERT INTO global_users(id,username,password_hash) VALUES($1,$2,'test-only')`, []any{f.user, "cash_" + f.user}}, {`INSERT INTO brand_members(id,brand_id,global_user_id,join_method) VALUES($1,$2,$3,'domain')`, []any{f.member, f.brand, f.user}}, {`INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, []any{ids.New(), f.brand, f.member}}, {`INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT $1,id,s,t FROM point_accounts CROSS JOIN unnest(ARRAY['recharge','winning','gift','commission']) s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal']) t WHERE brand_id=$1 ON CONFLICT DO NOTHING`, []any{f.brand}}, {`INSERT INTO brand_recharge_quotas(brand_id,available) VALUES($1,10000)`, []any{f.brand}}} {
		if _, e := p.Exec(ctx, q.sql, q.args...); e != nil {
			t.Fatal(e)
		}
	}
	tx, e := p.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	config := EmptyConfig()
	for i := range config.Rules {
		r := &config.Rules[i]
		r.Rate = "7"
		r.Enabled = true
		r.DepositMin = "1"
		r.DepositMax = "10000"
		r.WithdrawalMin = "1"
		r.WithdrawalMax = "10000"
		r.Fee = "0"
		r.Threshold = "1"
	}
	// System metadata is fixture-only and never exposed through the API.
	meta := points.Metadata{ActorType: "system", RequestID: ids.New()}
	if _, e = SaveConfig(ctx, tx, "platform", config, meta); e != nil {
		t.Fatal(e)
	}
	if _, e = SaveConfig(ctx, tx, f.brand, config, meta); e != nil {
		t.Fatal(e)
	}
	d := Destination{Currency: "CNY", Channel: "bank", Holder: "Test", Institution: "Test bank", Number: "123456", Enabled: true}
	f.merchant, e = SaveDestination(ctx, tx, f.brand, "", d, meta)
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	return f
}
func transact[T any](t *testing.T, f fixture, fn func(pgx.Tx) (T, error)) (T, error) {
	t.Helper()
	tx, e := f.db.Begin(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(context.Background())
	v, e := fn(tx)
	if e == nil {
		e = tx.Commit(context.Background())
	}
	return v, e
}
func TestDepositExchangeAndWithdrawalIsolation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	enablePointConversion(t, f)
	o, e := transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Deposit(ctx, tx, f.brand, f.member, Input{Currency: "CNY", Amount: "10"}, f.meta)
	})
	if e != nil {
		t.Fatal(e)
	}
	admin := points.Metadata{ActorType: "admin", RequestID: ids.New()}
	o, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Advance(ctx, tx, f.brand, "", o.ID, "confirm", Input{Version: 1, Amount: "10", Reference: "bank-reference-1"}, admin)
	})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Advance(ctx, tx, f.brand, "", o.ID, "confirm", Input{Version: 1, Amount: "10", Reference: "bank-reference-1"}, admin)
	}); !errors.Is(e, ErrConflict) {
		t.Fatalf("duplicate confirmation %v", e)
	}
	d, e := transact(t, f, func(tx pgx.Tx) (Destination, error) {
		return SaveDestination(ctx, tx, f.brand, f.member, Destination{Currency: "CNY", Channel: "bank", Holder: "Different name allowed", Institution: "Bank", Number: "654321", Enabled: true}, f.meta)
	})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Withdraw(ctx, tx, f.brand, f.member, Input{Currency: "CNY", Amount: "10", DestinationID: d.ID, DestinationVersion: d.Version, Fee: "0"}, f.meta)
	}); !errors.Is(e, ErrInsufficient) {
		t.Fatalf("unconverted withdrawal %v", e)
	}
	if _, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Exchange(ctx, tx, f.brand, f.member, Input{Currency: "CNY", Amount: "10", Direction: "to_points", Rate: "6"}, f.meta)
	}); !errors.Is(e, ErrRate) {
		t.Fatalf("stale rate %v", e)
	}
	_, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Exchange(ctx, tx, f.brand, f.member, Input{Currency: "CNY", Amount: "10", Direction: "to_points", Rate: "7"}, f.meta)
	})
	if e != nil {
		t.Fatal(e)
	}
	wallet, e := f.s.Points.Read(ctx, f.brand, f.member)
	if e != nil || wallet.AvailablePoints != 7000 {
		t.Fatal(wallet, e)
	}
	if _, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Exchange(ctx, tx, f.brand, f.member, Input{Currency: "CNY", Amount: "70", Direction: "from_points", Rate: "7"}, f.meta)
	}); e == nil {
		t.Fatal("unqualified points converted")
	}
	if _, e = transact(t, f, func(tx pgx.Tx) (Order, error) { return ReadOrder(ctx, tx, ids.New(), f.member, o.ID) }); !errors.Is(e, ErrConflict) {
		t.Fatalf("cross brand read %v", e)
	}
	var quota points.Amount
	if e = f.db.QueryRow(ctx, `SELECT available FROM brand_recharge_quotas WHERE brand_id=$1`, f.brand).Scan(&quota); e != nil || quota != 993000 {
		t.Fatalf("quota %s %v", quota.String(), e)
	}
}
func TestConcurrentExchangeCannotOverspend(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	o, e := transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Deposit(ctx, tx, f.brand, f.member, Input{Currency: "CNY", Amount: "10"}, f.meta)
	})
	if e != nil {
		t.Fatal(e)
	}
	_, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Advance(ctx, tx, f.brand, "", o.ID, "confirm", Input{Version: 1, Amount: "10", Reference: "concurrent-test"}, points.Metadata{ActorType: "admin", RequestID: ids.New()})
	})
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, e := f.db.Begin(ctx)
			if e != nil {
				errs <- e
				return
			}
			defer tx.Rollback(ctx)
			_, e = f.s.Exchange(ctx, tx, f.brand, f.member, Input{Currency: "CNY", Amount: "10", Direction: "to_points", Rate: "7"}, f.meta)
			if e == nil {
				e = tx.Commit(ctx)
			}
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	success := 0
	for e := range errs {
		if e == nil {
			success++
		} else if !errors.Is(e, ErrInsufficient) {
			t.Fatal(e)
		}
	}
	if success != 1 {
		t.Fatalf("success %d", success)
	}
}

func enablePointConversion(t *testing.T, f fixture) {
	t.Helper()
	ctx := context.Background()
	adminID := ids.New()
	if _, e := f.db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only')`, adminID, "cash_admin_"+adminID); e != nil {
		t.Fatal(e)
	}
	actor := access.Account{ID: adminID, Type: access.AccountAdmin, BrandIDs: []string{f.brand}, Roles: []access.Role{{BrandID: f.brand, Permissions: []access.Permission{{Resource: "withdrawal_policy", Action: "write", Scope: access.ScopeBrand}}}}}
	config := withdrawal.DefaultBrandConfig()
	config.Enabled = true
	_, e := transact(t, f, func(tx pgx.Tx) (withdrawal.BrandPolicy, error) {
		return (withdrawal.Service{DB: f.db}).UpdateBrand(ctx, tx, f.brand, actor, withdrawal.BrandInput{Version: 1, PointsStep: 1, Config: config}, points.Metadata{ActorType: "admin", ActorID: adminID, RequestID: ids.New()})
	})
	if e != nil {
		t.Fatal(e)
	}
}
func TestEligibleExchangePayoutReleaseAndSnapshot(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	enablePointConversion(t, f)
	_, e := transact(t, f, func(tx pgx.Tx) (points.Entry, error) {
		var delta points.Balance
		delta[1][0] = 7000
		return f.s.Points.Post(ctx, tx, points.Change{BrandID: f.brand, MemberID: f.member, EntryType: "adjustment", ReferenceType: "cash_test", OperationKey: ids.New(), ActorType: "system", RequestID: ids.New(), Delta: delta, Allocation: []points.Allocation{{Source: "winning", State: "available", Points: 7000}}})
	})
	if e != nil {
		t.Fatal(e)
	}
	converted, e := transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Exchange(ctx, tx, f.brand, f.member, Input{Currency: "CNY", Amount: "70", Direction: "from_points", Rate: "7"}, f.meta)
	})
	if e != nil || converted.Amount != "10" {
		t.Fatal(converted, e)
	}
	d, e := transact(t, f, func(tx pgx.Tx) (Destination, error) {
		return SaveDestination(ctx, tx, f.brand, f.member, Destination{Currency: "CNY", Channel: "bank", Holder: "Unverified recipient", Institution: "Bank", Number: "123", Enabled: true}, f.meta)
	})
	if e != nil {
		t.Fatal(e)
	}
	o, e := transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Withdraw(ctx, tx, f.brand, f.member, Input{Currency: "CNY", Amount: "10", DestinationID: d.ID, DestinationVersion: d.Version, Fee: "0"}, f.meta)
	})
	if e != nil {
		t.Fatal(e)
	}
	d.Number = "456"
	if d, e = transact(t, f, func(tx pgx.Tx) (Destination, error) { return SaveDestination(ctx, tx, f.brand, f.member, d, f.meta) }); e != nil {
		t.Fatal(e)
	}
	admin := points.Metadata{ActorType: "admin", RequestID: ids.New()}
	o, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Advance(ctx, tx, f.brand, "", o.ID, "approve", Input{Version: 1}, admin)
	})
	if e != nil || o.Destination.Number != "123" {
		t.Fatal(o, e)
	}
	if _, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Advance(ctx, tx, f.brand, f.member, o.ID, "cancel", Input{Version: o.Version}, f.meta)
	}); !errors.Is(e, ErrConflict) {
		t.Fatal("processing cancellation", e)
	}
	o, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Advance(ctx, tx, f.brand, "", o.ID, "fail", Input{Version: o.Version, Proof: "Verified no external payment was made"}, admin)
	})
	if e != nil {
		t.Fatal(e)
	}
	b, e := transact(t, f, func(tx pgx.Tx) ([]Balance, error) { return Balances(ctx, tx, f.brand, f.member) })
	if e != nil || b[0].Withdrawable != "10" || b[0].Reserved != "0" {
		t.Fatal(b, e)
	}
	o, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Withdraw(ctx, tx, f.brand, f.member, Input{Currency: "CNY", Amount: "10", DestinationID: d.ID, DestinationVersion: d.Version, Fee: "0"}, f.meta)
	})
	if e != nil {
		t.Fatal(e)
	}
	o, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Advance(ctx, tx, f.brand, "", o.ID, "approve", Input{Version: o.Version}, admin)
	})
	if e != nil {
		t.Fatal(e)
	}
	o, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Advance(ctx, tx, f.brand, "", o.ID, "paid", Input{Version: o.Version, Reference: "payout-verified-1"}, admin)
	})
	if e != nil || o.State != "paid" {
		t.Fatal(o, e)
	}
	if _, e = f.db.Exec(ctx, `UPDATE cash_orders SET amount=amount+10000 WHERE id=$1`, o.ID); e == nil {
		t.Fatal("immutable payout changed")
	}
}
func TestRotationLateReceiptAndDuplicateReference(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	meta := points.Metadata{ActorType: "admin", RequestID: ids.New()}
	d := f.merchant
	d.ID = ""
	d.Version = 0
	d.Number = "second"
	second, e := transact(t, f, func(tx pgx.Tx) (Destination, error) { return SaveDestination(ctx, tx, f.brand, "", d, meta) })
	if e != nil {
		t.Fatal(e)
	}
	a, e := transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Deposit(ctx, tx, f.brand, f.member, Input{Currency: "CNY", Amount: "3"}, f.meta)
	})
	if e != nil {
		t.Fatal(e)
	}
	_, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Advance(ctx, tx, f.brand, "", a.ID, "confirm", Input{Version: 1, Amount: "4", Reference: "unique-reference"}, meta)
	})
	if e != nil {
		t.Fatal(e)
	}
	b, e := transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Deposit(ctx, tx, f.brand, f.member, Input{Currency: "CNY", Amount: "3"}, f.meta)
	})
	if e != nil || a.Destination.ID == b.Destination.ID || b.Destination.ID != second.ID {
		t.Fatal(a, b, e)
	}
	if _, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Advance(ctx, tx, f.brand, "", b.ID, "confirm", Input{Version: 1, Amount: "3", Reference: "unique-reference"}, meta)
	}); e == nil {
		t.Fatal("duplicate reference credited")
	}
	b, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Advance(ctx, tx, f.brand, f.member, b.ID, "submit", Input{Version: 1, Proof: "bank receipt"}, f.meta)
	})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Advance(ctx, tx, f.brand, "", b.ID, "confirm", Input{Version: b.Version, Amount: "3", Reference: "unique-reference-2"}, meta)
	}); e != nil {
		t.Fatal(e)
	}
	b, e = transact(t, f, func(tx pgx.Tx) (Order, error) { return ReadOrder(ctx, tx, f.brand, f.member, a.ID) })
	if e != nil || b.RequestedAmount != "3" || b.Amount != "4" {
		t.Fatal(b, e)
	}
}

func TestAmountRotationReleasesCancelledAllocation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	meta := points.Metadata{ActorType: "admin", RequestID: ids.New()}
	cfg := EmptyConfig()
	cfg.Version = 1
	for i := range cfg.Rules {
		r := &cfg.Rules[i]
		r.Enabled = true
		r.Rate = "7"
		r.DepositMin = "1"
		r.DepositMax = "10000"
		r.WithdrawalMin = "1"
		r.WithdrawalMax = "10000"
		r.Mode = "amount"
		r.Threshold = "10"
	}
	if _, e := transact(t, f, func(tx pgx.Tx) (Config, error) { return SaveConfig(ctx, tx, f.brand, cfg, meta) }); e != nil {
		t.Fatal(e)
	}
	d := f.merchant
	d.ID = ""
	d.Version = 0
	d.Number = "second"
	if _, e := transact(t, f, func(tx pgx.Tx) (Destination, error) { return SaveDestination(ctx, tx, f.brand, "", d, meta) }); e != nil {
		t.Fatal(e)
	}
	first, e := transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Deposit(ctx, tx, f.brand, f.member, Input{Currency: "CNY", Amount: "6"}, f.meta)
	})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Advance(ctx, tx, f.brand, "", first.ID, "confirm", Input{Version: 1, Amount: "6", Reference: "volume-test"}, meta)
	}); e != nil {
		t.Fatal(e)
	}
	second, e := transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Deposit(ctx, tx, f.brand, f.member, Input{Currency: "CNY", Amount: "5"}, f.meta)
	})
	if e != nil || second.Destination.ID == first.Destination.ID {
		t.Fatal(first, second, e)
	}
	if _, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Advance(ctx, tx, f.brand, f.member, second.ID, "cancel", Input{Version: 1}, f.meta)
	}); e != nil {
		t.Fatal(e)
	}
	third, e := transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Deposit(ctx, tx, f.brand, f.member, Input{Currency: "CNY", Amount: "5"}, f.meta)
	})
	if e != nil || third.Destination.ID != second.Destination.ID {
		t.Fatal(second, third, e)
	}
}
func TestCashRejectsCorruptBalanceAndExpiredOrderAcceptsLateEvidence(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	o, e := transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Deposit(ctx, tx, f.brand, f.member, Input{Currency: "CNY", Amount: "2"}, f.meta)
	})
	if e != nil {
		t.Fatal(e)
	}
	// Create a separately pinned historical expired order to test late payment.
	late := ids.New()
	if _, e = f.db.Exec(ctx, `INSERT INTO cash_orders(id,brand_id,member_id,kind,currency,amount,requested_amount,state,destination_id,destination,created_at,expires_at) SELECT $2,brand_id,member_id,kind,currency,amount,requested_amount,'expired',destination_id,destination,now()-interval '31 minutes',now()-interval '1 minute' FROM cash_orders WHERE id=$1`, o.ID, late); e != nil {
		t.Fatal(e)
	}
	submitted, e := transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Advance(ctx, tx, f.brand, f.member, late, "submit", Input{Version: 1, Proof: "Verified late bank transfer"}, f.meta)
	})
	if e != nil || submitted.State != "submitted" {
		t.Fatal(submitted, e)
	}
	if _, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Advance(ctx, tx, f.brand, "", late, "confirm", Input{Version: submitted.Version, Amount: "2", Reference: "late-bank-transfer"}, points.Metadata{ActorType: "admin", RequestID: ids.New()})
	}); e != nil {
		t.Fatal(e)
	}
	if _, e = f.db.Exec(ctx, `UPDATE cash_balances SET deposited=deposited+10000 WHERE brand_id=$1 AND member_id=$2 AND currency='CNY'`, f.brand, f.member); e != nil {
		t.Fatal(e)
	}
	if _, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Exchange(ctx, tx, f.brand, f.member, Input{Currency: "CNY", Amount: "1", Direction: "to_points", Rate: "7"}, f.meta)
	}); !errors.Is(e, points.ErrCorrupt) {
		t.Fatal("corrupt balance was accepted", e)
	}
}

func TestUSDTSubmitWithoutHashRequiresVerifiedUniqueTransaction(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	admin := points.Metadata{ActorType: "admin", RequestID: ids.New()}
	_, e := transact(t, f, func(tx pgx.Tx) (Destination, error) {
		return SaveDestination(ctx, tx, f.brand, "", Destination{Currency: "USDT", Channel: "TRON/TRC20", Number: "TLa2f6VPqDgRE67v1736s7bJ8Ray5wYjU7", Enabled: true}, admin)
	})
	if e != nil {
		t.Fatal(e)
	}
	create := func() Order {
		t.Helper()
		o, e := transact(t, f, func(tx pgx.Tx) (Order, error) {
			return f.s.Deposit(ctx, tx, f.brand, f.member, Input{Currency: "USDT", Amount: "100.03"}, f.meta)
		})
		if e != nil {
			t.Fatal(e)
		}
		return o
	}
	o := create()
	if _, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Deposit(ctx, tx, f.brand, f.member, Input{Currency: "USDT", Amount: "100.03"}, f.meta)
	}); !errors.Is(e, ErrDepositPending) {
		t.Fatal("missing pending deposit error", e)
	}
	submitted, e := transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Advance(ctx, tx, f.brand, f.member, o.ID, "submit", Input{Version: 1}, f.meta)
	})
	if e != nil || submitted.State != "submitted" || submitted.Reference != "" {
		t.Fatal(submitted, e)
	}
	var credits int
	if e = f.db.QueryRow(ctx, `SELECT count(*) FROM cash_entries WHERE order_id=$1`, o.ID).Scan(&credits); e != nil || credits != 0 {
		t.Fatal("submission credited funds", credits, e)
	}
	for _, ref := range []string{"", "invalid"} {
		if _, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
			return f.s.Advance(ctx, tx, f.brand, "", o.ID, "confirm", Input{Version: 2, Amount: "100.03", Reference: ref}, admin)
		}); !errors.Is(e, ErrInvalid) {
			t.Fatal("confirmation accepted missing or invalid hash", e)
		}
	}
	hash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	firstFee, _ := Parse(o.Fee, "USDT")
	if _, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Advance(ctx, tx, f.brand, "", o.ID, "confirm", Input{Version: 2, Amount: Format(100030000 + firstFee), Reference: hash}, admin)
	}); e != nil {
		t.Fatal(e)
	}
	duplicate := create()
	duplicateFee, _ := Parse(duplicate.Fee, "USDT")
	if _, e = transact(t, f, func(tx pgx.Tx) (Order, error) {
		return f.s.Advance(ctx, tx, f.brand, "", duplicate.ID, "confirm", Input{Version: 1, Amount: Format(100030000 + duplicateFee), Reference: hash}, admin)
	}); e == nil {
		t.Fatal("transaction credited twice")
	}
	var balance int64
	if e = f.db.QueryRow(ctx, `SELECT deposited FROM cash_balances WHERE brand_id=$1 AND member_id=$2 AND currency='USDT'`, f.brand, f.member).Scan(&balance); e != nil || balance != 100030000 {
		t.Fatal("incorrect balance after duplicate", balance, e)
	}
}
