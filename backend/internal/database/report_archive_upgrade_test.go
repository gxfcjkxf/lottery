package database_test

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func archiveLegacyTableHashes(t *testing.T, db *pgxpool.Pool, tables []string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, name := range tables {
		var digest string
		query := `SELECT md5(coalesce(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text),'[]'::jsonb)::text) FROM ` + pgx.Identifier{name}.Sanitize() + ` r`
		if name == "permissions" {
			query += ` WHERE r.key NOT IN('report_archive.create.brand','report_archive.download.brand','report_archive.download.platform','report_archive.view.brand','report_archive.view.platform','report_archive_policy.write.brand','report_archive_task.retry.brand','report_attribution.view.brand','report_attribution.export.brand','report_attribution.view.platform','report_attribution.export.platform','audit.export.brand','audit.export.platform')`
		} else if name == "role_permissions" {
			query += ` WHERE r.permission_key NOT IN('report_archive.create.brand','report_archive.download.brand','report_archive.download.platform','report_archive.view.brand','report_archive.view.platform','report_archive_policy.write.brand','report_archive_task.retry.brand','report_attribution.view.brand','report_attribution.export.brand','report_attribution.view.platform','report_attribution.export.platform','audit.export.brand','audit.export.platform')`
		} else if name == "notification_templates" || name == "notification_template_revisions" {
			query += ` WHERE r.template_key NOT IN('draw.result.published','draw.result.corrected')`
		} else if name == "report_archives" {
			query = `SELECT md5(coalesce(jsonb_agg((to_jsonb(r)-'automatic_task_id'-'automatic_policy_version') ORDER BY (to_jsonb(r)-'automatic_task_id'-'automatic_policy_version')::text),'[]'::jsonb)::text) FROM report_archives r`
		}
		if err := db.QueryRow(context.Background(), query).Scan(&digest); err != nil {
			t.Fatal(name, err)
		}
		out[name] = digest
	}
	return out
}

func TestReportArchive0063ToLatestPreservesEveryLegacyTable(t *testing.T) {
	db := testdb.NewAtVersion(t, 63)
	ctx := context.Background()
	member, _ := upgradeWallet(t, db, false)
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var delta points.Balance
	delta[0][0] = 37
	if _, err = (points.Store{DB: db}).Post(ctx, tx, points.Change{BrandID: upgradeBrand, MemberID: member, EntryType: "adjustment", ReferenceType: "archive_upgrade", OperationKey: "archive-upgrade:" + ids.New(), Reason: "actual pre-upgrade ledger posting", ActorType: "system", RequestID: ids.New(), Delta: delta, Allocation: []points.Allocation{{Source: "recharge", State: "available", Points: 37}}}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	permissionRoles := createArchivePermissionUpgradeRoles(t, ctx, db)
	rows, err := db.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname=current_schema() AND tablename<>'schema_migrations' ORDER BY tablename`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(tables) < 60 {
		t.Fatal("upgrade table inventory incomplete", len(tables))
	}
	before := archiveLegacyTableHashes(t, db, tables)
	if err = database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	assertArchivePermissionUpgrade(t, ctx, db, permissionRoles)
	if after := archiveLegacyTableHashes(t, db, tables); !reflect.DeepEqual(before, after) {
		t.Fatal("archive migrations rewrote legacy data outside the separately asserted 0066 permission additions", before, after)
	}
	var archiveCount, migrationCount int
	if err = db.QueryRow(ctx, `SELECT (SELECT count(*) FROM report_archives),(SELECT count(*) FROM schema_migrations WHERE name IN('0064_report_archive_capture.up.sql','0065_report_archive_versions.up.sql','0066_report_archive_permissions.up.sql','0067_report_archive_automatic.up.sql'))`).Scan(&archiveCount, &migrationCount); err != nil || archiveCount != 0 || migrationCount != 4 {
		t.Fatal(archiveCount, migrationCount, err)
	}
	var hardened int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=current_schema() AND p.proname IN('report_archive_capture','report_archive_period_boundary','guard_report_archive_version') AND p.proconfig=ARRAY['search_path=pg_catalog, '||current_schema()||', pg_temp']`).Scan(&hardened); err != nil || hardened != 3 {
		t.Fatal("archive helpers not pinned", hardened, err)
	}
	if err = database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	assertArchivePermissionUpgrade(t, ctx, db, permissionRoles)
	if after := archiveLegacyTableHashes(t, db, tables); !reflect.DeepEqual(before, after) {
		t.Fatal("repeated upgrade changed legacy rows")
	}
	t.Logf("preserved %d legacy tables, including genuine ledger/bucket/audit records", len(tables))
}

type archivePermissionRoles struct {
	brandBootstrap, brandCustom, platformBootstrap, platformCustom string
}

func createArchivePermissionUpgradeRoles(t *testing.T, ctx context.Context, db *pgxpool.Pool) archivePermissionRoles {
	t.Helper()
	roles := archivePermissionRoles{ids.New(), ids.New(), ids.New(), ids.New()}
	for _, role := range []struct {
		id, brand, code string
		bootstrap       bool
	}{
		{roles.brandBootstrap, upgradeBrand, "archive_upgrade_bootstrap_brand", true},
		{roles.brandCustom, upgradeBrand, "archive_upgrade_custom_brand", false},
		{roles.platformBootstrap, "", "archive_upgrade_bootstrap_platform", true},
		{roles.platformCustom, "", "archive_upgrade_custom_platform", false},
	} {
		if _, err := db.Exec(ctx, `INSERT INTO roles(id,brand_id,code,name,is_bootstrap) VALUES($1,NULLIF($2,'')::uuid,$3,$3,$4)`, role.id, role.brand, role.code, role.bootstrap); err != nil {
			t.Fatal(err)
		}
	}
	adminID := ids.New()
	if _, err := db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash,is_super_admin) VALUES($1,$2,'archive-permission-upgrade',true)`, adminID, "archive_upgrade_"+adminID[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, adminID, roles.platformBootstrap); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES
	 ($1,'report_commission.view.brand'),($2,'report_commission.view.brand'),
	 ($3,'report_commission.view.platform'),($4,'report_commission.view.platform')`,
		roles.brandBootstrap, roles.brandCustom, roles.platformBootstrap, roles.platformCustom); err != nil {
		t.Fatal(err)
	}
	return roles
}

func assertArchivePermissionUpgrade(t *testing.T, ctx context.Context, db *pgxpool.Pool, roles archivePermissionRoles) {
	t.Helper()
	var permissions []string
	if err := db.QueryRow(ctx, `SELECT coalesce(array_agg(key ORDER BY key),'{}') FROM permissions WHERE key LIKE 'report_archive.%' OR key IN('report_archive_policy.write.brand','report_archive_task.retry.brand')`).Scan(&permissions); err != nil {
		t.Fatal(err)
	}
	wantPermissions := []string{
		"report_archive.create.brand", "report_archive.download.brand", "report_archive.download.platform",
		"report_archive.view.brand", "report_archive.view.platform",
		"report_archive_policy.write.brand", "report_archive_task.retry.brand",
	}
	if !reflect.DeepEqual(permissions, wantPermissions) {
		t.Fatalf("0066 archive permissions=%v, want exact additions %v", permissions, wantPermissions)
	}
	var grants []string
	if err := db.QueryRow(ctx, `SELECT coalesce(array_agg(role_id::text||':'||permission_key ORDER BY role_id::text,permission_key),'{}') FROM role_permissions WHERE permission_key LIKE 'report_archive.%' OR permission_key IN('report_archive_policy.write.brand','report_archive_task.retry.brand')`).Scan(&grants); err != nil {
		t.Fatal(err)
	}
	wantGrants := []string{
		roles.brandBootstrap + ":report_archive.create.brand",
		roles.brandBootstrap + ":report_archive.download.brand",
		roles.brandBootstrap + ":report_archive.view.brand",
		roles.platformBootstrap + ":report_archive.download.platform",
		roles.platformBootstrap + ":report_archive.view.platform",
		roles.brandBootstrap + ":report_archive_policy.write.brand",
		roles.brandBootstrap + ":report_archive_task.retry.brand",
	}
	sort.Strings(wantGrants)
	if !reflect.DeepEqual(grants, wantGrants) {
		t.Fatalf("0066 archive grants=%v, want exact additions %v", grants, wantGrants)
	}
}
