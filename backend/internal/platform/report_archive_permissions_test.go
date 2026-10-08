package platform_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/reportarchive"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestReportArchive0065To0066PermissionsPreserveRolesArchivesAndMoney(t *testing.T) {
	ctx := context.Background()
	db := testdb.NewAtVersion(t, 65)
	brand := "0199a000-0000-7000-8000-000000000001"
	brandBootstrap, brandCustom := ids.New(), ids.New()
	platformBootstrap, platformCustom := ids.New(), ids.New()
	superID := ids.New()
	for _, role := range []struct {
		id, brandID, code string
		bootstrap         bool
	}{
		{brandBootstrap, brand, "archive_bootstrap_brand", true},
		{brandCustom, brand, "archive_custom_brand", false},
		{platformBootstrap, "", "archive_bootstrap_platform", true},
		{platformCustom, "", "archive_custom_platform", false},
	} {
		if _, err := db.Exec(ctx, `INSERT INTO roles(id,brand_id,code,name,is_bootstrap) VALUES($1,NULLIF($2,'')::uuid,$3,$3,$4)`, role.id, role.brandID, role.code, role.bootstrap); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash,is_super_admin) VALUES($1,$2,'archive-permission-upgrade',true)`, superID, "archive_perm_"+superID[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, superID, platformBootstrap); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO permissions(key) VALUES('report_commission.view.brand'),('report_commission.view.platform') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,'report_commission.view.brand'),($2,'report_commission.view.brand'),($3,'report_commission.view.platform'),($4,'report_commission.view.platform')`, brandBootstrap, brandCustom, platformBootstrap, platformCustom); err != nil {
		t.Fatal(err)
	}
	archiveAdmin := ids.New()
	if _, err := db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'archive-permission-fixture')`, archiveAdmin, "archive_fixture_"+archiveAdmin[:8]); err != nil {
		t.Fatal(err)
	}
	if err := reportArchiveCreateFixture(t, ctx, db, brand, archiveAdmin); err != nil {
		t.Fatal(err)
	}
	if err := reportArchivePostLedgerFixture(t, ctx, db, brand); err != nil {
		t.Fatal(err)
	}
	oldPermissions := reportArchivePermissionRoleSnapshot(t, ctx, db)
	archiveBefore := reportArchiveArchiveSnapshot(t, ctx, db)
	moneyBefore := reportArchiveMoneySnapshot(t, ctx, db)

	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if got := reportArchiveArchiveSnapshot(t, ctx, db); got != archiveBefore {
		t.Fatalf("0066 changed archived reports: before=%s after=%s", archiveBefore, got)
	}
	if got := reportArchiveMoneySnapshot(t, ctx, db); got != moneyBefore {
		t.Fatalf("0066 changed financial records: before=%s after=%s", moneyBefore, got)
	}
	for roleID, want := range oldPermissions {
		var got []string
		if err := db.QueryRow(ctx, `SELECT coalesce(array_agg(permission_key ORDER BY permission_key),'{}') FROM role_permissions WHERE role_id=$1 AND permission_key NOT LIKE 'report_archive.%'`, roleID).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("0066 changed existing role grants for %s: got=%v want=%v", roleID, got, want)
		}
	}
	for _, tc := range []struct {
		id   string
		want []string
	}{
		{brandBootstrap, []string{"report_archive.create.brand", "report_archive.download.brand", "report_archive.view.brand"}},
		{brandCustom, nil},
		{platformBootstrap, []string{"report_archive.download.platform", "report_archive.view.platform"}},
		{platformCustom, nil},
	} {
		var got []string
		if err := db.QueryRow(ctx, `SELECT coalesce(array_agg(permission_key ORDER BY permission_key),'{}') FROM role_permissions WHERE role_id=$1 AND permission_key LIKE 'report_archive.%'`, tc.id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if len(tc.want) == 0 {
			tc.want = []string{}
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("role %s archive grants=%v, want %v", tc.id, got, tc.want)
		}
	}
	var registered int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM permissions WHERE key IN('report_archive.view.brand','report_archive.create.brand','report_archive.download.brand','report_archive.view.platform','report_archive.download.platform')`).Scan(&registered); err != nil || registered != 5 {
		t.Fatalf("registered archive permissions=%d err=%v", registered, err)
	}
	permissionsBeforeRepeat := reportArchivePermissionState(t, ctx, db)
	archiveBeforeRepeat := reportArchiveArchiveSnapshot(t, ctx, db)
	moneyBeforeRepeat := reportArchiveMoneySnapshot(t, ctx, db)
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatalf("repeat 0066 migration: %v", err)
	}
	if got := reportArchivePermissionState(t, ctx, db); !reflect.DeepEqual(got, permissionsBeforeRepeat) {
		t.Fatalf("repeat migration changed permission or grant state: before=%v after=%v", permissionsBeforeRepeat, got)
	}
	if got := reportArchiveArchiveSnapshot(t, ctx, db); got != archiveBeforeRepeat {
		t.Fatal("repeat migration changed retained archive data")
	}
	if got := reportArchiveMoneySnapshot(t, ctx, db); got != moneyBeforeRepeat {
		t.Fatal("repeat migration changed financial records")
	}
}

func reportArchivePermissionState(t *testing.T, ctx context.Context, db *pgxpool.Pool) [2][]string {
	t.Helper()
	var state [2][]string
	if err := db.QueryRow(ctx, `SELECT coalesce(array_agg(key ORDER BY key),'{}') FROM permissions`).Scan(&state[0]); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT coalesce(array_agg(role_id::text||':'||permission_key ORDER BY role_id::text,permission_key),'{}') FROM role_permissions`).Scan(&state[1]); err != nil {
		t.Fatal(err)
	}
	return state
}

func reportArchiveCreateFixture(t *testing.T, ctx context.Context, db *pgxpool.Pool, brand, actorID string) error {
	t.Helper()
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	input := reportarchive.Input{
		Kind: reportarchive.Daily, PeriodKey: time.Now().UTC().AddDate(0, 0, -2).Format("2006-01-02"),
		Reason: "pre-0066 retained archive observation",
	}
	_, err = (reportarchive.Service{DB: db}).CreateTx(ctx, tx, brand, accessArchiveAdmin(actorID, brand), input,
		points.Metadata{ActorType: "admin", ActorID: actorID, RequestID: ids.New()})
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func accessArchiveAdmin(actorID, brand string) access.Account {
	return access.Account{ID: actorID, Type: access.AccountAdmin, BrandIDs: []string{brand}, Roles: []access.Role{{
		BrandID: brand,
		Permissions: []access.Permission{
			{Resource: "report_archive", Action: "view", Scope: access.ScopeBrand},
			{Resource: "report_archive", Action: "create", Scope: access.ScopeBrand},
		},
	}}}
}

func reportArchivePostLedgerFixture(t *testing.T, ctx context.Context, db *pgxpool.Pool, brand string) error {
	t.Helper()
	userID, memberID, accountID := ids.New(), ids.New(), ids.New()
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO global_users(id,username) VALUES($1,$2)`, []any{userID, "archive_perm_user_" + userID[:8]}},
		{`INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'operator','v1','v1')`, []any{memberID, brand, userID}},
		{`INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, []any{accountID, brand, memberID}},
		{`INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT $1,$2,s,st FROM unnest(ARRAY['recharge','winning','gift','commission']::text[]) s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal']::text[]) st ON CONFLICT DO NOTHING`, []any{brand, accountID}},
	} {
		if _, err := db.Exec(ctx, statement.query, statement.args...); err != nil {
			return err
		}
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var delta points.Balance
	delta[0][0] = 29
	if _, err := (points.Store{DB: db}).Post(ctx, tx, points.Change{
		BrandID: brand, MemberID: memberID, EntryType: "adjustment", ReferenceType: "archive_permission_test",
		OperationKey: "archive-permission-fixture:" + ids.New(), Reason: "pre-0066 ledger fixture",
		ActorType: "system", RequestID: ids.New(), Delta: delta,
		Allocation: []points.Allocation{{Source: "recharge", State: "available", Points: 29}},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func reportArchivePermissionRoleSnapshot(t *testing.T, ctx context.Context, db *pgxpool.Pool) map[string][]string {
	t.Helper()
	out := make(map[string][]string)
	rows, err := db.Query(ctx, `SELECT role_id::text,permission_key FROM role_permissions WHERE permission_key NOT LIKE 'report_archive.%' ORDER BY role_id,permission_key`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var role, permission string
		if err := rows.Scan(&role, &permission); err != nil {
			t.Fatal(err)
		}
		out[role] = append(out[role], permission)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func reportArchiveArchiveSnapshot(t *testing.T, ctx context.Context, db *pgxpool.Pool) string {
	t.Helper()
	var snapshot string
	if err := db.QueryRow(ctx, `SELECT coalesce(jsonb_agg(to_jsonb(a) ORDER BY id),'[]'::jsonb)::text FROM report_archives a`).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func reportArchiveMoneySnapshot(t *testing.T, ctx context.Context, db *pgxpool.Pool) string {
	t.Helper()
	var snapshot string
	if err := db.QueryRow(ctx, `SELECT jsonb_build_object(
	 'point_accounts',(SELECT coalesce(jsonb_agg(to_jsonb(r) ORDER BY id),'[]'::jsonb) FROM point_accounts r),
	 'point_buckets',(SELECT coalesce(jsonb_agg(to_jsonb(r) ORDER BY brand_id,account_id,source,state),'[]'::jsonb) FROM point_buckets r),
	 'point_ledger_entries',(SELECT coalesce(jsonb_agg(to_jsonb(r) ORDER BY id),'[]'::jsonb) FROM point_ledger_entries r),
	 'recharge_orders',(SELECT coalesce(jsonb_agg(to_jsonb(r) ORDER BY id),'[]'::jsonb) FROM recharge_orders r),
	 'bet_orders',(SELECT coalesce(jsonb_agg(to_jsonb(r) ORDER BY id),'[]'::jsonb) FROM bet_orders r))::text`).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	return snapshot
}
