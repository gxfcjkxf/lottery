package reporting

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"testing"
	"time"
)

const testBrand = "0199a000-0000-7000-8000-000000000001"

func TestReportQueryBoundsAndGroups(t *testing.T) {
	at := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	q := Query{From: at, To: at.Add(24 * time.Hour), GroupBy: "day", Limit: 20}
	if q.Validate("betting") != nil || q.Validate("ledger") != nil {
		t.Fatal(q)
	}
	for _, change := range []func(*Query){func(v *Query) { v.To = v.From }, func(v *Query) { v.To = v.From.Add(93*24*time.Hour + time.Nanosecond) }, func(v *Query) { v.GroupBy = "raw" }, func(v *Query) { v.Limit = 101 }, func(v *Query) { v.Offset = 1000001 }, func(v *Query) { id := "not uuid"; v.MemberID = &id }} {
		bad := q
		change(&bad)
		if bad.Validate("betting") != ErrInvalid {
			t.Fatal(bad)
		}
	}
	g := ids.New()
	q.GameID = &g
	if q.Validate("ledger") != ErrInvalid {
		t.Fatal("ledger silently accepted a game filter")
	}
	q.GameID = nil
	for _, group := range []string{"day", "member", "state"} {
		q.GroupBy = group
		if q.Validate("withdrawal") != nil {
			t.Fatal("withdrawal group rejected", group)
		}
	}
	q.GroupBy = "game"
	if q.Validate("withdrawal") != ErrInvalid {
		t.Fatal("withdrawal accepted unsupported group")
	}
	q.GroupBy = "state"
	q.GameID = &g
	if q.Validate("withdrawal") != ErrInvalid {
		t.Fatal("withdrawal accepted game filter")
	}
}
func TestLedgerReportLargeAggregatesTransfersAndOutsideWindowBalances(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	ps := points.Store{DB: db}
	for i := 0; i < 2; i++ {
		u, m := ids.New(), ids.New()
		if _, e := db.Exec(ctx, `INSERT INTO global_users(id,username) VALUES($1,$2);`, u, "report_"+u); e != nil {
			t.Fatal(e)
		}
		if _, e := db.Exec(ctx, `INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'domain','dev-1','dev-1')`, m, testBrand, u); e != nil {
			t.Fatal(e)
		}
		account := ids.New()
		if _, e := db.Exec(ctx, `INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, account, testBrand, m); e != nil {
			t.Fatal(e)
		}
		if _, e := db.Exec(ctx, `INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT $1,$2,source,state FROM unnest(ARRAY['recharge','winning','gift','commission']) source CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal']) state ON CONFLICT DO NOTHING`, testBrand, account); e != nil {
			t.Fatal(e)
		}
		tx, e := db.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		var delta points.Balance
		delta[0][0] = 9000000000000000000
		_, e = ps.Post(ctx, tx, points.Change{BrandID: testBrand, MemberID: m, EntryType: "adjust", ReferenceType: "test", ReferenceID: m, OperationKey: "report-large:" + m, Reason: "isolated large aggregate fixture", ActorType: "system", RequestID: ids.New(), Delta: delta, Allocation: []points.Allocation{{Source: "recharge", State: "available", Points: delta[0][0]}}})
		if e != nil {
			tx.Rollback(ctx)
			t.Fatal(e)
		}
		if e = tx.Commit(ctx); e != nil {
			t.Fatal(e)
		}
	}
	s := Service{DB: db}
	q := Query{From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour), GroupBy: "entry_type", Limit: 1}
	r, e := s.Ledger(ctx, testBrand, q)
	if e != nil || r.Summary.NetPoints != "18000000000000000000" || r.Balances.AvailablePoints != "18000000000000000000" || r.Balances.AccountCount != "2" || r.Summary.EntryCount != "2" || r.TotalGroups != "1" || len(r.Items) != 1 {
		t.Fatal(r, e)
	}
	q.Offset = 1
	p, e := s.Ledger(ctx, testBrand, q)
	if e != nil || len(p.Items) != 0 || p.Summary != r.Summary || p.TotalGroups != "1" {
		t.Fatal(p, e)
	}
	q.Offset = 0
	q.From = time.Now().Add(24 * time.Hour)
	q.To = q.From.Add(time.Hour)
	empty, e := s.Ledger(ctx, testBrand, q)
	if e != nil || empty.Summary.EntryCount != "0" || empty.Summary.NetPoints != "0" || empty.Balances.TotalPoints != "18000000000000000000" {
		t.Fatal(empty, e)
	}
	foreign := ids.New()
	q.MemberID = &foreign
	if _, e = s.Ledger(ctx, testBrand, q); e != ErrNotFound {
		t.Fatal("foreign member scope not rejected", e)
	}
}
