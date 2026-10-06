package reporting

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newExportMember(t *testing.T, ctx context.Context, db *pgxpool.Pool) string {
	t.Helper()
	user, member, account := ids.New(), ids.New(), ids.New()
	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO global_users(id,username) VALUES($1,$2)`, []any{user, "report_export_" + user}},
		{`INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'domain','dev-1','dev-1')`, []any{member, testBrand, user}},
		{`INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, []any{account, testBrand, member}},
		{`INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT $1,$2,source,state FROM unnest(ARRAY['recharge','winning','gift']) source CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal']) state`, []any{testBrand, account}},
	} {
		if _, err := db.Exec(ctx, stmt.sql, stmt.args...); err != nil {
			t.Fatal(err)
		}
	}
	return member
}

func postLedgerExportEntry(t *testing.T, ctx context.Context, db *pgxpool.Pool, tx pgx.Tx, member, entryType string) points.Entry {
	t.Helper()
	var delta points.Balance
	delta[0][0] = 1
	entry, err := (points.Store{DB: db}).Post(ctx, tx, points.Change{
		BrandID: testBrand, MemberID: member, EntryType: entryType, ReferenceType: "report_export",
		OperationKey: "report-export:" + ids.New(), Reason: "report export fixture credit",
		ActorType: "system", RequestID: ids.New(), Delta: delta,
		Allocation: []points.Allocation{{Source: "recharge", State: "available", Points: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

func postZeroNetLedgerTransfer(t *testing.T, ctx context.Context, db *pgxpool.Pool, tx pgx.Tx, member, entryType, from, to string) {
	t.Helper()
	allocation := []points.Allocation{{Source: "recharge", State: from, Points: 1}}
	delta, err := points.AllocationDelta(allocation, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = (points.Store{DB: db}).Post(ctx, tx, points.Change{
		BrandID: testBrand, MemberID: member, EntryType: entryType, ReferenceType: "report_export",
		OperationKey: "report-export:" + ids.New(), Reason: "zero net report export fixture transfer",
		ActorType: "system", RequestID: ids.New(), Delta: delta, Allocation: allocation,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestLedgerExportReturnsAllGroupsAndHonorsFilters(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	member := newExportMember(t, ctx, db)
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for i := 0; i < 101; i++ {
		postLedgerExportEntry(t, ctx, db, tx, member, fmt.Sprintf("export_type_%03d", i))
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	q := Query{From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour), GroupBy: "entry_type", Limit: 20}
	result, err := (Service{DB: db}).LedgerExport(ctx, mustBegin(t, ctx, db), testBrand, q)
	if err != nil || result.Query != q || result.TotalGroups != "101" || len(result.Items) != 101 || result.Summary.EntryCount != "101" || result.Summary.NetPoints != "101" {
		t.Fatalf("export should contain all groups and totals: groups=%s items=%d summary=%+v err=%v", result.TotalGroups, len(result.Items), result.Summary, err)
	}
	for i, item := range result.Items {
		if item.Key != fmt.Sprintf("export_type_%03d", i) || item.Totals.EntryCount != "1" {
			t.Fatalf("unexpected group at %d: %+v", i, item)
		}
	}

	q.MemberID = &member
	filtered, err := (Service{DB: db}).LedgerExport(ctx, mustBegin(t, ctx, db), testBrand, q)
	if err != nil || filtered.Summary.EntryCount != "101" || len(filtered.Items) != 101 || filtered.Balances.AccountCount != "1" || filtered.Balances.TotalPoints != "101" {
		t.Fatalf("member filter not applied to full export: %+v err=%v", filtered, err)
	}
}

func TestExportUsesCallerTransactionSnapshot(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	member := newExportMember(t, ctx, db)
	seed, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	postLedgerExportEntry(t, ctx, db, seed, member, "snapshot_before")
	if err = seed.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	q := Query{From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour), GroupBy: "entry_type", Limit: 20}
	service := Service{DB: db}
	first, err := service.LedgerExport(ctx, tx, testBrand, q)
	if err != nil || first.Summary.EntryCount != "1" {
		t.Fatalf("initial export: %+v err=%v", first, err)
	}

	writer, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	postLedgerExportEntry(t, ctx, db, writer, member, "snapshot_after")
	if err = writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	inside, err := service.LedgerExport(ctx, tx, testBrand, q)
	if err != nil || inside.Summary.EntryCount != "1" || inside.TotalGroups != "1" || len(inside.Items) != 1 {
		t.Fatalf("export escaped transaction snapshot: %+v err=%v", inside, err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := service.Ledger(ctx, testBrand, q)
	if err != nil || after.Summary.EntryCount != "2" || after.TotalGroups != "2" {
		t.Fatalf("new committed entry not visible to subsequent report: %+v err=%v", after, err)
	}
}

func TestLedgerExportUsesCanonicalASCIIKeyOrder(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	member := newExportMember(t, ctx, db)
	write := mustBegin(t, ctx, db)
	for _, key := range []string{"Aa", ":a", "-a"} {
		postLedgerExportEntry(t, ctx, db, write, member, key)
	}
	if err := write.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	q := Query{From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour), GroupBy: "entry_type", Limit: 20, MemberID: &member}
	r, err := (Service{DB: db}).LedgerExport(ctx, mustBegin(t, ctx, db), testBrand, q)
	if err != nil || len(r.Items) != 3 {
		t.Fatal("canonical key export", err, len(r.Items))
	}
	for i, key := range []string{"-a", ":a", "Aa"} {
		if r.Items[i].Key != key {
			t.Fatalf("key %d: got %q, want %q", i, r.Items[i].Key, key)
		}
	}
	if _, err = LedgerCSV(r); err != nil {
		t.Fatal("valid ordered keys failed CSV encoding", err)
	}
}

func TestLedgerExportRejectsMoreThanGroupLimitFromDatabase(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	member := newExportMember(t, ctx, db)
	seed, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	seedEntry := postLedgerExportEntry(t, ctx, db, seed, member, "export_limit_seed")
	if err = seed.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	from := seedEntry.CreatedAt.Add(time.Second)
	if delay := time.Until(from); delay > 0 {
		time.Sleep(delay)
	}
	write, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer write.Rollback(ctx)
	for i := 0; i < ExportGroupLimit+1; i++ {
		fromState, toState := "available", "manual_frozen"
		if i%2 == 1 {
			fromState, toState = toState, fromState
		}
		postZeroNetLedgerTransfer(t, ctx, db, write, member, fmt.Sprintf("limit_type_%05d", i), fromState, toState)
	}
	if err = write.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	q := Query{From: from, To: time.Now().Add(time.Hour), GroupBy: "entry_type", Limit: 20}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	result, err := (Service{DB: db}).LedgerExport(ctx, tx, testBrand, q)
	if !errors.Is(err, ErrExportTooLarge) {
		t.Fatalf("export did not reject %d database groups: groups=%s returned=%d err=%v", ExportGroupLimit+1, result.TotalGroups, len(result.Items), err)
	}
	if result.TotalGroups != fmt.Sprint(ExportGroupLimit+1) || len(result.Items) != 0 {
		t.Fatalf("oversized export returned partial groups: groups=%s items=%d", result.TotalGroups, len(result.Items))
	}
	ordinary, err := (Service{DB: db}).Ledger(ctx, testBrand, q)
	if err != nil || ordinary.Summary.NetPoints != "0" || ordinary.TotalGroups != fmt.Sprint(ExportGroupLimit+1) || len(ordinary.Items) != q.Limit {
		t.Fatalf("zero-net fixture mismatch: groups=%s summary=%+v err=%v", ordinary.TotalGroups, ordinary.Summary, err)
	}
}

func TestExportValidationAndGroupCountBoundary(t *testing.T) {
	if !groupCountExceeds("10001", ExportGroupLimit) || groupCountExceeds("10000", ExportGroupLimit) || !groupCountExceeds("999999999999999999999999", ExportGroupLimit) {
		t.Fatal("group count comparison did not preserve the 10000 boundary")
	}
	db := testdb.New(t)
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	q := Query{From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour), GroupBy: "day", Limit: 20, Offset: 1}
	if _, err = (Service{DB: db}).BettingExport(ctx, tx, testBrand, q); !errors.Is(err, ErrInvalid) {
		t.Fatalf("betting export accepted offset: %v", err)
	}
	if _, err = (Service{DB: db}).LedgerExport(ctx, nil, testBrand, Query{From: q.From, To: q.To, GroupBy: "day", Limit: 20}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ledger export accepted nil transaction: %v", err)
	}
	q.Offset = 0
	betting, err := (Service{DB: db}).BettingExport(ctx, tx, testBrand, q)
	if err != nil || betting.Query != q || betting.TotalGroups != "0" || len(betting.Items) != 0 || betting.Summary.OrderCount != "0" {
		t.Fatalf("empty betting export projection: %+v err=%v", betting, err)
	}
}

func mustBegin(t *testing.T, ctx context.Context, db *pgxpool.Pool) pgx.Tx {
	t.Helper()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	return tx
}
