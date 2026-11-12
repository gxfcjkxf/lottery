package dataretention_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/dataretention"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/reporting"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
)

func TestArchivePreservesBalancesReplayReversalAndHistory(t *testing.T) {
	db := testdb.NewUnseeded(t)
	ctx := context.Background()
	brand, user, member, account := ids.New(), ids.New(), ids.New(), ids.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO brands(id,code,name,status) VALUES($1,$2,'Retention isolated test','active')`, []any{brand, "retention_" + brand}},
		{`INSERT INTO global_users(id,username) VALUES($1,$2)`, []any{user, "retention_" + user}},
		{`INSERT INTO brand_members(id,brand_id,global_user_id,join_method) VALUES($1,$2,$3,'domain')`, []any{member, brand, user}},
		{`INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, []any{account, brand, member}},
		{`INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT $1,$2,s,t FROM unnest(ARRAY['recharge','winning','gift','commission'])s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal'])t ON CONFLICT DO NOTHING`, []any{brand, account}},
	} {
		if _, err := db.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	ps := points.Store{DB: db}
	var delta points.Balance
	delta[0][0] = 12345
	change := points.Change{BrandID: brand, MemberID: member, EntryType: "adjustment", ReferenceType: "retention_test", OperationKey: ids.New(), ActorType: "system", RequestID: ids.New(), Delta: delta, Allocation: []points.Allocation{{Source: "recharge", State: "available", Points: 12345}}}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	entry, err := ps.Post(ctx, tx, change)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	engine, err := mutation.New(db, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	requestKey := ids.New()
	callbackRuns := 0
	operation := func(context.Context, pgx.Tx) (mutation.Result, error) {
		callbackRuns++
		return mutation.OK(200, map[string]string{"id": entry.ID}), nil
	}
	if _, err = engine.Execute(ctx, brand, user, "retention.test", requestKey, engine.Fingerprint("test"), operation); err != nil {
		t.Fatal(err)
	}
	var zone string
	if err = db.QueryRow(ctx, `SELECT timezone FROM brands WHERE id=$1`, brand).Scan(&zone); err != nil {
		t.Fatal(err)
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(loc)
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	to := from.AddDate(0, 0, 1)
	q := reporting.Query{From: from, To: to, GroupBy: "entry_type", Limit: 20}
	rs := reporting.Service{DB: db}
	before, err := rs.Ledger(ctx, brand, q)
	if err != nil {
		t.Fatal(err)
	}
	task := ids.New()
	if _, err = db.Exec(ctx, `INSERT INTO data_retention_tasks(id,brand_id,policy_version,timezone,execute_at,cutoff_at) VALUES($1,$2,1,$3,$4,$5)`, task, brand, zone, to.AddDate(0, 2, 0), to); err != nil {
		t.Fatal(err)
	}
	s := dataretention.Service{DB: db}
	for i := 0; i < 20; i++ {
		worked, e := s.Process(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if !worked {
			break
		}
		if i == 19 {
			t.Fatal("retention task did not finish")
		}
	}
	var raw, receipts int
	if err = db.QueryRow(ctx, `SELECT (SELECT count(*) FROM point_ledger_entries WHERE brand_id=$1),(SELECT count(*) FROM data_retention_receipts WHERE brand_id=$1 AND family='point_ledger_entries')`, brand).Scan(&raw, &receipts); err != nil || raw != 0 || receipts != 1 {
		t.Fatal(raw, receipts, err)
	}
	after, err := rs.Ledger(ctx, brand, q)
	if err != nil || after.Summary != before.Summary || after.Balances != before.Balances {
		t.Fatal(before, after, err)
	}
	partial := q
	partial.From = partial.From.Add(time.Hour)
	if _, err = rs.Ledger(ctx, brand, partial); !errors.Is(err, reporting.ErrHistoricalRange) {
		t.Fatal("partial history was accepted", err)
	}
	w, err := ps.Read(ctx, brand, member)
	if err != nil || w.AvailablePoints != 12345 || w.Version != 1 {
		t.Fatal(w, err)
	}
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	replayed, err := ps.Post(ctx, tx, change)
	if err != nil || replayed.ID != entry.ID {
		t.Fatal(replayed, err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = ps.Reverse(ctx, tx, brand, member, entry.ID, "retention-reverse:"+entry.ID, "", points.Metadata{ActorType: "system", RequestID: ids.New()}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	w, err = ps.Read(ctx, brand, member)
	if err != nil || w.AvailablePoints != 0 || w.Version != 2 {
		t.Fatal(w, err)
	}
	// A second pass cannot summarize or delete the original transaction twice.
	if worked, err := s.Process(ctx); err != nil || worked {
		t.Fatal(worked, err)
	}
	response, err := engine.Execute(ctx, brand, user, "retention.test", requestKey, engine.Fingerprint("test"), operation)
	if err != nil || response.Status != 409 || response.Error == nil || response.Error.Code != "IDEMPOTENCY_RECEIPT_ARCHIVED" || callbackRuns != 1 {
		t.Fatal("archived request executed again", response, callbackRuns, err)
	}
	if _, err = db.Exec(ctx, `DELETE FROM data_retention_receipts WHERE id=$1`, entry.ID); err == nil {
		t.Fatal("economic receipt was deletable")
	}
	if _, err = db.Exec(ctx, `DELETE FROM retention_keys_point_ledger_entries WHERE id=$1`, entry.ID); err == nil {
		t.Fatal("economic operation key was deletable")
	}
}
