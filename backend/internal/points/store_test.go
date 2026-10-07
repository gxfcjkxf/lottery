package points

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testBrand = "0199a000-0000-7000-8000-000000000001"

func fixture(t *testing.T) (*pgxpool.Pool, Store, string) {
	t.Helper()
	p := testdb.New(t)
	ctx := context.Background()
	user, member, account := ids.New(), ids.New(), ids.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO global_users(id,username,password_hash) VALUES($1,$2,'test-only-opaque-hash')`, []any{user, "points_" + user}},
		{`INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'domain','dev-1','dev-1')`, []any{member, testBrand, user}},
		{`INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, []any{account, testBrand, member}},
		{`INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT $1,$2,s,t FROM unnest(ARRAY['recharge','winning','gift','commission'])s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal'])t ON CONFLICT(brand_id,account_id,source,state) DO NOTHING`, []any{testBrand, account}},
	} {
		if _, err := p.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	var bucketCount int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM point_buckets WHERE brand_id=$1 AND account_id=$2`, testBrand, account).Scan(&bucketCount); err != nil || bucketCount != 16 {
		t.Fatalf("fixture bucket count=%d err=%v", bucketCount, err)
	}
	return p, Store{DB: p}, member
}
func change(member, key string, delta Balance, allocation []Allocation) Change {
	return Change{BrandID: testBrand, MemberID: member, EntryType: "adjustment", ReferenceType: "test", OperationKey: key, Reason: "isolated ledger test", ActorType: "system", RequestID: ids.New(), Delta: delta, Allocation: allocation}
}
func post(t *testing.T, p *pgxpool.Pool, s Store, c Change) Entry {
	t.Helper()
	ctx := context.Background()
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	e, err := s.Post(ctx, tx, c)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return e
}
func credit(t *testing.T, p *pgxpool.Pool, s Store, member string, source int, amount Amount) Entry {
	var d Balance
	d[source][0] = amount
	return post(t, p, s, change(member, ids.New(), d, []Allocation{{Source: sourceNames[source], State: "available", Points: amount}}))
}
func TestLedgerReconstructReplayAndExactRefund(t *testing.T) {
	p, s, member := fixture(t)
	ctx := context.Background()
	credit(t, p, s, member, 0, 50)
	credit(t, p, s, member, 1, 30)
	credit(t, p, s, member, 2, 20)
	w, err := s.Read(ctx, testBrand, member)
	if err != nil || w.DisplayPoints != 100 || w.Version != 3 {
		t.Fatal(w, err)
	}
	allocation, err := w.BySource.Allocate(75, "available")
	if err != nil {
		t.Fatal(err)
	}
	if len(allocation) != 2 || allocation[0].Source != "recharge" || allocation[0].Points != 50 || allocation[1].Points != 25 {
		t.Fatal(allocation)
	}
	d, err := AllocationDelta(allocation, "available", "")
	if err != nil {
		t.Fatal(err)
	}
	c := change(member, "test-debit", d, allocation)
	c.EntryType = "bet"
	e := post(t, p, s, c)
	if e.Before != w.BySource || e.After[0][0] != 0 || e.After[1][0] != 5 {
		t.Fatal(e)
	}
	c.RequestID = ids.New()
	replay := post(t, p, s, c)
	if replay.ID != e.ID {
		t.Fatal("replay wrote another entry")
	}
	tx, _ := p.Begin(ctx)
	_, err = s.Reverse(ctx, tx, testBrand, member, e.ID, "refund:test-debit", "original-source refund", Metadata{ActorType: "system", RequestID: ids.New()})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	restored, err := s.Read(ctx, testBrand, member)
	if err != nil || restored.BySource != w.BySource || restored.Version != 5 {
		t.Fatal(restored, err)
	}
	report, err := s.Reconcile(ctx, testBrand, member)
	if err != nil || !report.Consistent || report.EntryCount != 5 {
		t.Fatal(report, err)
	}
	tx, _ = p.Begin(ctx)
	_, err = s.Reverse(ctx, tx, testBrand, member, e.ID, "refund:second", "duplicate refund", Metadata{ActorType: "system", RequestID: ids.New()})
	tx.Rollback(ctx)
	if !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	c.Reason = "changed content"
	tx, _ = p.Begin(ctx)
	_, err = s.Post(ctx, tx, c)
	tx.Rollback(ctx)
	if !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	for _, table := range []string{"point_ledger_entries", "audit_logs"} {
		if _, err = p.Exec(ctx, "DELETE FROM "+table); err == nil {
			t.Fatal("append-only protection missing", table)
		}
	}
}

func TestCommissionDebitAndExactRefundAcrossDebitPriority(t *testing.T) {
	p, s, member := fixture(t)
	credit(t, p, s, member, 0, 2)
	credit(t, p, s, member, 1, 3)
	credit(t, p, s, member, 2, 5)
	credit(t, p, s, member, 3, 7)
	ctx := context.Background()
	wallet, err := s.Read(ctx, testBrand, member)
	if err != nil || wallet.CommissionPoints != 7 || wallet.AvailablePoints != 17 {
		t.Fatalf("commission wallet=%+v err=%v", wallet, err)
	}
	allocations, err := wallet.BySource.Allocate(17, "available")
	want := []Allocation{
		{Source: "recharge", State: "available", Points: 2},
		{Source: "winning", State: "available", Points: 3},
		{Source: "commission", State: "available", Points: 7},
		{Source: "gift", State: "available", Points: 5},
	}
	if err != nil || !reflect.DeepEqual(allocations, want) {
		t.Fatalf("commission debit allocations=%+v err=%v want=%+v", allocations, err, want)
	}
	delta, err := AllocationDelta(allocations, "available", "")
	if err != nil {
		t.Fatal(err)
	}
	debit := change(member, "commission-debit-refund", delta, allocations)
	debit.EntryType = "bet"
	entry := post(t, p, s, debit)
	if entry.After[3][0] != 0 {
		t.Fatalf("commission debit left commission balance: %v", entry.After[3])
	}
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Reverse(ctx, tx, testBrand, member, entry.ID, "commission-debit-refund-reversal", "restore original source allocations", Metadata{ActorType: "system", RequestID: ids.New()}); err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	restored, err := s.Read(ctx, testBrand, member)
	if err != nil || restored.BySource != wallet.BySource || restored.CommissionPoints != 7 {
		t.Fatalf("commission refund wallet=%+v err=%v", restored, err)
	}
	reconciled, err := s.Reconcile(ctx, testBrand, member)
	if err != nil || !reconciled.Consistent {
		t.Fatalf("commission refund reconciliation=%+v err=%v", reconciled, err)
	}
}

func TestConcurrentDebitsNeverOverdraw(t *testing.T) {
	p, s, member := fixture(t)
	credit(t, p, s, member, 0, 100)
	var successes atomic.Int64
	var wg sync.WaitGroup
	ctx := context.Background()
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tx, err := p.Begin(ctx)
			if err != nil {
				t.Error(err)
				return
			}
			defer tx.Rollback(ctx)
			var d Balance
			d[0][0] = -10
			c := change(member, fmt.Sprintf("debit:%d", i), d, []Allocation{{Source: "recharge", State: "available", Points: 10}})
			_, err = s.Post(ctx, tx, c)
			if err == nil {
				if err = tx.Commit(ctx); err != nil {
					t.Error(err)
				} else {
					successes.Add(1)
				}
			} else if !errors.Is(err, ErrInsufficient) {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if successes.Load() != 10 {
		t.Fatal(successes.Load())
	}
	w, err := s.Read(ctx, testBrand, member)
	if err != nil || w.AvailablePoints != 0 || w.Version != 11 {
		t.Fatal(w, err)
	}
	r, err := s.Reconcile(ctx, testBrand, member)
	if err != nil || !r.Consistent {
		t.Fatal(r, err)
	}
}
func TestFreezeTransferReversalAndRollback(t *testing.T) {
	p, s, member := fixture(t)
	credit(t, p, s, member, 0, 7)
	credit(t, p, s, member, 2, 8)
	ctx := context.Background()
	w, _ := s.Read(ctx, testBrand, member)
	a, _ := w.BySource.Allocate(10, "available")
	d, _ := AllocationDelta(a, "available", "manual_frozen")
	c := change(member, "freeze:one", d, a)
	c.EntryType = "freeze"
	e := post(t, p, s, c)
	w, err := s.Read(ctx, testBrand, member)
	if err != nil || w.AvailablePoints != 5 || w.ManualFrozenPoints != 10 || w.DisplayPoints != 15 || w.RechargePoints != 0 || w.GiftPoints != 5 {
		t.Fatal(w, err)
	}
	tx, _ := p.Begin(ctx)
	_, err = s.Reverse(ctx, tx, testBrand, member, e.ID, "unfreeze:one", "return original sources", Metadata{ActorType: "system", RequestID: ids.New()})
	if err != nil {
		t.Fatal(err)
	}
	tx.Rollback(ctx)
	still, _ := s.Read(ctx, testBrand, member)
	if still.BySource != w.BySource {
		t.Fatal("rolled-back refund committed")
	}
	tx, _ = p.Begin(ctx)
	_, err = s.Reverse(ctx, tx, testBrand, member, e.ID, "unfreeze:one", "return original sources", Metadata{ActorType: "system", RequestID: ids.New()})
	if err != nil {
		t.Fatal(err)
	}
	tx.Commit(ctx)
	w, _ = s.Read(ctx, testBrand, member)
	if w.BySource[0][0] != 7 || w.BySource[2][0] != 8 || w.FrozenPoints != 0 {
		t.Fatal(w)
	}
	foreign := "0199a000-0000-7000-8000-000000000002"
	tx, _ = p.Begin(ctx)
	_, err = s.Entry(ctx, tx, foreign, member, e.ID)
	tx.Rollback(ctx)
	if !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-brand ledger read", err)
	}
}
func TestOverflowMissingBucketAndOutOfBandChangeFailClosed(t *testing.T) {
	p, s, member := fixture(t)
	ctx := context.Background()
	credit(t, p, s, member, 0, Amount(math.MaxInt64))
	var d Balance
	d[2][0] = 1
	c := change(member, "overflow", d, []Allocation{{Source: "gift", State: "available", Points: 1}})
	tx, _ := p.Begin(ctx)
	_, err := s.Post(ctx, tx, c)
	tx.Rollback(ctx)
	if !errors.Is(err, ErrOverflow) {
		t.Fatal(err)
	}
	w, _ := s.Read(ctx, testBrand, member)
	if _, err = p.Exec(ctx, `UPDATE point_buckets SET points=1 WHERE account_id=$1 AND source='recharge' AND state='available'`, w.AccountID); err != nil {
		t.Fatal(err)
	}
	r, err := s.Reconcile(ctx, testBrand, member)
	if err != nil || r.Consistent {
		t.Fatal(r, err)
	}
	tx, _ = p.Begin(ctx)
	_, err = s.Post(ctx, tx, c)
	tx.Rollback(ctx)
	if !errors.Is(err, ErrCorrupt) {
		t.Fatal(err)
	}
	if _, err = p.Exec(ctx, `DELETE FROM point_buckets WHERE account_id=$1 AND source='commission' AND state='withdrawal'`, w.AccountID); err != nil {
		t.Fatal(err)
	}
	_, err = s.Read(ctx, testBrand, member)
	if !errors.Is(err, ErrCorrupt) {
		t.Fatal(err)
	}
}
