//go:build reportlimit

package reporting

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

// Larger than normal page regressions; invoke explicitly with -tags reportlimit.
// Every financial row is posted through the production ledger, not fabricated
// by weakening immutability or projection/history constraints.
func TestReportExportActualDatabaseGroupBoundary(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	user, member, account := ids.New(), ids.New(), ids.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO global_users(id,username)VALUES($1,'export_limit_member')`, []any{user}},
		{`INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version)VALUES($1,$2,$3,'domain','dev-1','dev-1')`, []any{member, testBrand, user}},
		{`INSERT INTO point_accounts(id,brand_id,brand_member_id)VALUES($1,$2,$3)`, []any{account, testBrand, member}},
		{`INSERT INTO point_buckets(brand_id,account_id,source,state)SELECT $1,$2,source,state FROM unnest(ARRAY['recharge','winning','gift'])source CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal'])state`, []any{testBrand, account}},
	} {
		if _, e := db.Exec(ctx, q.sql, q.args...); e != nil {
			t.Fatal(e)
		}
	}
	tx, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	store := points.Store{DB: db}
	var delta points.Balance
	delta[0][0] = 1
	post := func(i int) {
		t.Helper()
		_, e := store.Post(ctx, tx, points.Change{BrandID: testBrand, MemberID: member, EntryType: fmt.Sprintf("group_%05d", i), ReferenceType: "export_limit_fixture", OperationKey: fmt.Sprintf("export-limit:%05d", i), Reason: "Owned report export boundary fixture", ActorType: "system", RequestID: ids.New(), Delta: delta, Allocation: []points.Allocation{{Source: "recharge", State: "available", Points: 1}}})
		if e != nil {
			t.Fatal(e)
		}
	}
	for i := 0; i < ExportGroupLimit; i++ {
		post(i)
	}
	service := Service{DB: db}
	q := Query{From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour), GroupBy: "entry_type", Limit: 20}
	atLimit, e := service.LedgerExport(ctx, tx, testBrand, q)
	if e != nil || len(atLimit.Items) != ExportGroupLimit || atLimit.TotalGroups != "10000" || atLimit.Summary.EntryCount != "10000" {
		t.Fatal("actual boundary not complete", e, atLimit.TotalGroups, len(atLimit.Items))
	}
	if body, e := LedgerCSV(atLimit); e != nil || len(body) > CSVByteLimit {
		t.Fatal("allowed complete export failed", e, len(body))
	}
	post(ExportGroupLimit)
	if _, e = service.LedgerExport(ctx, tx, testBrand, q); e != ErrExportTooLarge {
		t.Fatal("10001 actual groups not rejected", e)
	}
	// The outer fixture transaction is rolled back normally. No original data or
	// authorization counters are reset to make the boundary pass.
}
