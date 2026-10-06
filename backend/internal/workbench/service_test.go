package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"testing"
	"time"
)

const brandA = "0199a000-0000-7000-8000-000000000001"
const brandB = "0199a000-0000-7000-8000-000000000002"

func actor(resources ...string) access.Account {
	a := access.Account{ID: ids.New(), Type: access.AccountAdmin, BrandIDs: []string{brandA}}
	role := access.Role{BrandID: brandA}
	for _, r := range resources {
		role.Permissions = append(role.Permissions, access.Permission{Resource: r, Action: "view", Scope: access.ScopeBrand})
	}
	a.Roles = []access.Role{role}
	return a
}
func snapshot(t *testing.T, db *pgxpool.Pool, a access.Account, brand string) (Snapshot, error) {
	t.Helper()
	ctx := context.Background()
	tx, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	return (Service{}).ReadTx(ctx, tx, a, brand)
}
func TestExplicitPermissionsAndNullSections(t *testing.T) {
	db := testdb.New(t)
	a := actor("brand")
	out, e := snapshot(t, db, a, brandA)
	if e != nil {
		t.Fatal(e)
	}
	if out.Brand.Status != "ready" || out.Brand.Data.Name != "Aurora" || out.Orders.Status != "forbidden" || out.Orders.Data != nil || out.Ledger.Data != nil || out.Withdrawals.Status != "not_implemented" || out.Withdrawals.Data != nil {
		t.Fatal(out)
	}
	raw, e := json.Marshal(out)
	if e != nil {
		t.Fatal(e)
	}
	var decoded map[string]any
	if e = json.Unmarshal(raw, &decoded); e != nil {
		t.Fatal(e)
	}
	if decoded["orders"].(map[string]any)["data"] != nil {
		t.Fatal("forbidden data was serialized")
	}
	a.SuperAdmin = true
	a.Roles = nil
	if _, e = snapshot(t, db, a, brandA); !errors.Is(e, ErrDenied) {
		t.Fatalf("super identity is not permission: %v", e)
	}
	a = actor("report_ledger")
	a.BrandIDs = append(a.BrandIDs, brandB)
	if _, e = snapshot(t, db, a, brandB); !errors.Is(e, ErrDenied) {
		t.Fatalf("cross-brand role leak: %v", e)
	}
	a = access.Account{Type: access.AccountAdmin, SuperAdmin: true, Roles: []access.Role{{Permissions: []access.Permission{{Resource: "brand", Action: "view", Scope: access.ScopePlatform}}}}}
	if out, e = snapshot(t, db, a, brandB); e != nil || out.Brand.Data.Name != "Harbor" {
		t.Fatal(out, e)
	}
}
func TestFullEmptySnapshotIsActualZeroNotUnavailable(t *testing.T) {
	db := testdb.New(t)
	out, e := snapshot(t, db, actor("brand", "period", "bet", "report_betting", "settlement", "recharge", "report_ledger", "wallet", "draw_source"), brandA)
	if e != nil {
		t.Fatal(e)
	}
	if out.Periods.Data.Betting != "0" || out.TodayBets.Data.StakePoints != "0" || out.Settlement.Data.Failed != "0" || out.Recharges.Data.PendingPoints != "0" || out.Ledger.Data.NetPoints != "0" || out.Balances.Data.TotalPoints != "0" || out.Reconciliation.Data.LatestJob != nil || out.Sources.Data.AdapterState != "stub" || out.Sources.Data.LastAttemptAt != nil {
		t.Fatal(out)
	}
	if out.SnapshotAt.Before(out.DayFrom) || out.SnapshotAt.Sub(out.DayFrom) > 25*time.Hour {
		t.Fatal(out.SnapshotAt, out.DayFrom)
	}
}
func fund(t *testing.T, db *pgxpool.Pool, brand string, amount points.Amount) string {
	t.Helper()
	ctx := context.Background()
	user, member, account := ids.New(), ids.New(), ids.New()
	tx, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	queries := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO global_users(id,username) VALUES($1,$2)`, []any{user, "bench_" + user}},
		{`INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'domain','dev-1','dev-1')`, []any{member, brand, user}},
		{`INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, []any{account, brand, member}},
		{`INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT $1,$2,s,v FROM unnest(ARRAY['recharge','winning','gift'])s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal'])v`, []any{brand, account}},
	}
	for _, q := range queries {
		if _, e = tx.Exec(ctx, q.sql, q.args...); e != nil {
			t.Fatal(e)
		}
	}
	var delta points.Balance
	delta[0][0] = amount
	_, e = (points.Store{DB: db}).Post(ctx, tx, points.Change{BrandID: brand, MemberID: member, EntryType: "adjust", ReferenceType: "test", ReferenceID: member, OperationKey: "workbench-fixture:" + member, Reason: "isolated workbench fixture", ActorType: "system", RequestID: ids.New(), Delta: delta, Allocation: []points.Allocation{{Source: "recharge", State: "available", Points: amount}}})
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	return member
}
func TestUnboundedExactSumsBrandIsolationAndTimezone(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	fund(t, db, brandA, 9000000000000000000)
	fund(t, db, brandA, 9000000000000000000)
	fund(t, db, brandB, 71)
	if _, e := db.Exec(ctx, `UPDATE brands SET timezone='Pacific/Kiritimati' WHERE id=$1`, brandA); e != nil {
		t.Fatal(e)
	}
	out, e := snapshot(t, db, actor("report_ledger"), brandA)
	if e != nil {
		t.Fatal(e)
	}
	if out.Ledger.Data.NetPoints != "18000000000000000000" || out.Balances.Data.TotalPoints != "18000000000000000000" || out.Balances.Data.AccountCount != "2" || out.Brand.Data != nil || out.Reconciliation.Data != nil {
		t.Fatal(out)
	}
	zone, e := time.LoadLocation("Pacific/Kiritimati")
	if e != nil {
		t.Fatal(e)
	}
	local := out.DayFrom.In(zone)
	if local.Hour() != 0 || local.Minute() != 0 || out.Timezone != "Pacific/Kiritimati" {
		t.Fatal(local, out.Timezone)
	}
}
func TestForbiddenTablesAreNotReadAndGrantedFailureIsNotZero(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if _, e := db.Exec(ctx, `ALTER TABLE point_buckets RENAME TO owned_hidden_buckets`); e != nil {
		t.Fatal(e)
	}
	out, e := snapshot(t, db, actor("brand"), brandA)
	if e != nil || out.Brand.Data == nil {
		t.Fatal(out, e)
	}
	if _, e = snapshot(t, db, actor("report_ledger"), brandA); e == nil {
		t.Fatal("broken granted table must fail, not report zero")
	}
}
func TestDeniedUnknownBrandDoesNotRevealExistence(t *testing.T) {
	db := testdb.New(t)
	unknown := ids.New()
	if _, e := snapshot(t, db, actor("brand"), unknown); !errors.Is(e, ErrDenied) {
		t.Fatal(e)
	}
	a := access.Account{Type: access.AccountAdmin, Roles: []access.Role{{Permissions: []access.Permission{{Resource: "brand", Action: "view", Scope: access.ScopePlatform}}}}}
	if _, e := snapshot(t, db, a, unknown); !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
	if _, e := snapshot(t, db, a, "invalid"); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
}
