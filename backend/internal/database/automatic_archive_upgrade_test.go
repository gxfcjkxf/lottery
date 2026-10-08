package database_test

import (
	"context"
	"reflect"
	"strings"
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

func TestReportArchive0066ToAutomaticPreservesManualArchivesAndLegacyRows(t *testing.T) {
	ctx := context.Background()
	db := testdb.NewAtVersion(t, 66)
	member, _ := upgradeWallet(t, db, false)

	// Exercise the real ledger posting path before upgrading. Its source rows,
	// allocations and audit evidence are covered by the full table snapshot.
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var delta points.Balance
	delta[0][0] = 37
	if _, err = (points.Store{DB: db}).Post(ctx, tx, points.Change{
		BrandID: upgradeBrand, MemberID: member, EntryType: "adjustment", ReferenceType: "archive_upgrade",
		OperationKey: "automatic-archive-upgrade:" + ids.New(), Reason: "pre-0067 actual ledger posting",
		ActorType: "system", RequestID: ids.New(), Delta: delta,
		Allocation: []points.Allocation{{Source: "recharge", State: "available", Points: 37}},
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	adminID := ids.New()
	if _, err = db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'automatic-archive-upgrade-fixture')`, adminID, "automatic_archive_"+adminID[:8]); err != nil {
		t.Fatal(err)
	}
	period := time.Now().UTC().AddDate(0, 0, -3).Format("2006-01-02")
	archiveTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	manual, err := (reportarchive.Service{DB: db}).CreateTx(ctx, archiveTx, upgradeBrand, access.Account{
		ID: adminID, Type: access.AccountAdmin, BrandIDs: []string{upgradeBrand}, Roles: []access.Role{{
			BrandID: upgradeBrand, Permissions: []access.Permission{
				{Resource: "report_archive", Action: "view", Scope: access.ScopeBrand},
				{Resource: "report_archive", Action: "create", Scope: access.ScopeBrand},
			},
		}},
	}, reportarchive.Input{Kind: reportarchive.Daily, PeriodKey: period, Reason: "manual archive retained across automatic upgrade"},
		points.Metadata{ActorType: "admin", ActorID: adminID, RequestID: ids.New()})
	if err != nil {
		_ = archiveTx.Rollback(ctx)
		t.Fatalf("create actual pre-0067 manual archive: %v", err)
	}
	if err = archiveTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	roles := createArchivePermissionUpgradeRoles(t, ctx, db)
	if _, err = db.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES
	 ($1,'report_archive.create.brand'),($1,'report_archive.download.brand'),($1,'report_archive.view.brand'),
	 ($2,'report_archive.download.platform'),($2,'report_archive.view.platform')`, roles.brandBootstrap, roles.platformBootstrap); err != nil {
		t.Fatal(err)
	}
	tables := archiveUpgradeTableInventory(t, ctx, db)
	if len(tables) < 60 {
		t.Fatalf("upgrade table inventory incomplete: %d", len(tables))
	}
	before := archiveLegacyTableHashes(t, db, tables)
	guardOIDBefore, guardDefinitionBefore := archiveManualGuardDefinition(t, ctx, db)
	var archiveBefore string
	if err = db.QueryRow(ctx, `SELECT jsonb_build_object('row',to_jsonb(r),'payload',r.payload,'payload_sha256',r.payload_sha256)::text FROM report_archives r WHERE id=$1`, manual.ID).Scan(&archiveBefore); err != nil {
		t.Fatal(err)
	}

	if err = database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	guardOIDAfter, guardDefinitionAfter := archiveManualGuardDefinition(t, ctx, db)
	if guardOIDAfter != guardOIDBefore {
		t.Fatalf("0067 replaced the existing manual archive guard OID: before=%d after=%d", guardOIDBefore, guardOIDAfter)
	}
	anchor := ` IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'report archives are immutable'; END IF;`
	automaticBranch := `
 IF NEW.automatic_task_id IS NOT NULL THEN
  PERFORM pg_advisory_xact_lock(hashtextextended('report-archive:'||NEW.brand_id::text||':'||NEW.kind||':'||NEW.period_key,0));
  IF NOT report_archive_automatic_valid(NEW) THEN RAISE EXCEPTION 'automatic archive lacks task and snapshot evidence'; END IF;
  RETURN NEW;
 END IF;`
	calendarAnchor := ` first_at:=report_archive_period_boundary(boundary,NEW.timezone); end_at:=report_archive_period_boundary(next_boundary,NEW.timezone);`
	calendarReplacement := ` first_at:=CASE WHEN NEW.kind='monthly' THEN report_archive_period_end_boundary(boundary,NEW.timezone) ELSE report_archive_period_boundary(boundary,NEW.timezone) END; end_at:=report_archive_period_end_boundary(next_boundary,NEW.timezone);`
	expectedDefinition := strings.Replace(strings.Replace(guardDefinitionBefore, anchor, anchor+automaticBranch, 1), calendarAnchor, calendarReplacement, 1)
	lockAnchor := `hashtextextended('report-archive:'||NEW.brand_id::text`
	expectedDefinition = strings.ReplaceAll(expectedDefinition, lockAnchor, `hashtextextended(TG_TABLE_SCHEMA||':report-archive:'||NEW.brand_id::text`)
	if strings.Count(guardDefinitionBefore, anchor) != 1 || strings.Count(guardDefinitionBefore, calendarAnchor) != 1 || expectedDefinition != guardDefinitionAfter {
		t.Fatal("0067 changed manual archive guard beyond its anchored automatic branch, schema lock and skipped-date end correction")
	}
	if after := archiveLegacyTableHashes(t, db, tables); !reflect.DeepEqual(before, after) {
		t.Fatalf("0067 changed preexisting table rows beyond its nullable archive metadata and exact permission additions: before=%v after=%v", before, after)
	}
	var archiveAfter string
	if err = db.QueryRow(ctx, `SELECT jsonb_build_object('row',to_jsonb(r)-'automatic_task_id'-'automatic_policy_version','payload',r.payload,'payload_sha256',r.payload_sha256)::text FROM report_archives r WHERE id=$1`, manual.ID).Scan(&archiveAfter); err != nil {
		t.Fatal(err)
	}
	if archiveAfter != archiveBefore {
		t.Fatalf("manual archive row, payload, or payload hash changed across 0067: before=%s after=%s", archiveBefore, archiveAfter)
	}
	var metadataIsNull bool
	if err = db.QueryRow(ctx, `SELECT automatic_task_id IS NULL AND automatic_policy_version IS NULL FROM report_archives WHERE id=$1`, manual.ID).Scan(&metadataIsNull); err != nil || !metadataIsNull {
		t.Fatalf("manual archive was backfilled with automatic metadata: null=%v err=%v", metadataIsNull, err)
	}
	assertArchivePermissionUpgrade(t, ctx, db, roles)
	assertAutomaticArchiveUpgradeState(t, ctx, db)

	if err = database.Migrate(ctx, db); err != nil {
		t.Fatalf("repeat 0067 migration: %v", err)
	}
	guardOIDRepeat, guardDefinitionRepeat := archiveManualGuardDefinition(t, ctx, db)
	if guardOIDRepeat != guardOIDBefore || guardDefinitionRepeat != guardDefinitionAfter {
		t.Fatal("repeated migration changed the existing archive guard OID or body")
	}
	if after := archiveLegacyTableHashes(t, db, tables); !reflect.DeepEqual(before, after) {
		t.Fatalf("repeated migration changed preexisting table rows: before=%v after=%v", before, after)
	}
	if err = db.QueryRow(ctx, `SELECT jsonb_build_object('row',to_jsonb(r)-'automatic_task_id'-'automatic_policy_version','payload',r.payload,'payload_sha256',r.payload_sha256)::text FROM report_archives r WHERE id=$1`, manual.ID).Scan(&archiveAfter); err != nil || archiveAfter != archiveBefore {
		t.Fatalf("repeated migration changed manual archive: before=%s after=%s err=%v", archiveBefore, archiveAfter, err)
	}
	assertArchivePermissionUpgrade(t, ctx, db, roles)
	assertAutomaticArchiveUpgradeState(t, ctx, db)

	// A plain brand registry insert must receive an opt-out policy without any
	// task being created as a side effect.
	newBrand := ids.New()
	if _, err = db.Exec(ctx, `INSERT INTO brands(id,code,name,status) VALUES($1,$2,$2,'active')`, newBrand, "automatic_default_"+newBrand[:8]); err != nil {
		t.Fatal(err)
	}
	assertAutomaticArchivePolicy(t, ctx, db, newBrand)
	var taskCount int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM report_archive_automatic_tasks`).Scan(&taskCount); err != nil || taskCount != 0 {
		t.Fatalf("brand registration unexpectedly created automatic tasks: count=%d err=%v", taskCount, err)
	}
}

func archiveUpgradeTableInventory(t *testing.T, ctx context.Context, db *pgxpool.Pool) []string {
	t.Helper()
	rows, err := db.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname=current_schema() AND tablename<>'schema_migrations' ORDER BY tablename`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return tables
}

func assertAutomaticArchiveUpgradeState(t *testing.T, ctx context.Context, db *pgxpool.Pool) {
	t.Helper()
	var brands, policies, disabledPolicies, revisions, cursors, tasks, scheduledCursors int
	if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM brands),(SELECT count(*) FROM brand_report_archive_policies),(SELECT count(*) FROM brand_report_archive_policies WHERE daily_enabled=false AND monthly_enabled=false),(SELECT count(*) FROM report_archive_policy_revisions),(SELECT count(*) FROM report_archive_automatic_cursors),(SELECT count(*) FROM report_archive_automatic_tasks),(SELECT count(*) FROM report_archive_automatic_cursors WHERE due_at IS NOT NULL)`).Scan(&brands, &policies, &disabledPolicies, &revisions, &cursors, &tasks, &scheduledCursors); err != nil {
		t.Fatal(err)
	}
	if brands == 0 || policies != brands || disabledPolicies != brands || revisions != brands || cursors != 0 || scheduledCursors != 0 || tasks != 0 {
		t.Fatalf("automatic archive migration state: brands=%d policies=%d disabled policies=%d revisions=%d cursors=%d with due_at=%d tasks=%d; want one disabled current policy and revision per existing brand, and no cursors or tasks", brands, policies, disabledPolicies, revisions, cursors, scheduledCursors, tasks)
	}
	var invalidVersions, duplicateBrandPolicies int
	if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM brand_report_archive_policies WHERE version<>1),(SELECT count(*)-count(DISTINCT brand_id) FROM brand_report_archive_policies)`).Scan(&invalidVersions, &duplicateBrandPolicies); err != nil {
		t.Fatal(err)
	}
	if invalidVersions != 0 || duplicateBrandPolicies != 0 {
		t.Fatalf("automatic archive policy version invariants: noninitial versions=%d duplicate brands=%d", invalidVersions, duplicateBrandPolicies)
	}
	var exactRevisions int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM brand_report_archive_policies p JOIN report_archive_policy_revisions r ON r.brand_id=p.brand_id AND r.version=p.version WHERE to_jsonb(p)=to_jsonb(r)`).Scan(&exactRevisions); err != nil {
		t.Fatal(err)
	}
	if exactRevisions != brands {
		t.Fatalf("initial policy revisions matching current policies=%d, want %d", exactRevisions, brands)
	}
	// All ten migration functions are pinned to the application schema.
	var functions, pinned int
	if err := db.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE p.proconfig=ARRAY['search_path=pg_catalog, '||current_schema()||', pg_temp']) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=current_schema() AND p.proname IN('guard_report_archive_policy','capture_report_archive_policy','guard_report_archive_policy_revision','initialize_report_archive_policy','report_archive_period_end_boundary','report_archive_cursor_due','guard_report_archive_cursor','guard_report_archive_automatic_task','report_archive_automatic_valid','require_report_archive_automatic_link')`).Scan(&functions, &pinned); err != nil {
		t.Fatal(err)
	}
	if functions != 10 || pinned != 10 {
		t.Fatalf("0067 automatic archive functions pinned=%d/%d, want all ten pinned", pinned, functions)
	}
}

func archiveManualGuardDefinition(t *testing.T, ctx context.Context, db *pgxpool.Pool) (int64, string) {
	t.Helper()
	var oid int64
	var definition string
	if err := db.QueryRow(ctx, `SELECT p.oid::bigint,pg_get_functiondef(p.oid) FROM pg_proc p WHERE p.oid='guard_report_archive_version()'::regprocedure`).Scan(&oid, &definition); err != nil {
		t.Fatal(err)
	}
	return oid, definition
}

func assertAutomaticArchivePolicy(t *testing.T, ctx context.Context, db *pgxpool.Pool, brand string) {
	t.Helper()
	var version int64
	var dailyEnabled, monthlyEnabled bool
	if err := db.QueryRow(ctx, `SELECT version,daily_enabled,monthly_enabled FROM brand_report_archive_policies WHERE brand_id=$1`, brand).Scan(&version, &dailyEnabled, &monthlyEnabled); err != nil {
		t.Fatal(err)
	}
	if version < 1 || dailyEnabled || monthlyEnabled {
		t.Fatalf("new brand automatic archive policy: version=%d daily_enabled=%v monthly_enabled=%v; want a positive version and both periods disabled", version, dailyEnabled, monthlyEnabled)
	}
}
