package database_test

import (
	"context"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rewards"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCommissionCorrectionPlans0057To0058IsAdditiveAndScoped(t *testing.T) {
	ctx := context.Background()
	db := testdb.NewAtVersion(t, 57)
	member, _ := upgradeWallet(t, db, false)
	adminID := ids.New()
	if _, err := db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'correction-upgrade-test')`, adminID, "correction_upgrade_"+adminID[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO sessions(id,token_hash,admin_id,expires_at) VALUES($1,$2,$3,clock_timestamp()+interval '1 hour')`, ids.New(), "correction_upgrade_session_"+adminID, adminID); err != nil {
		t.Fatal(err)
	}
	actor := access.Account{
		ID: adminID, Type: access.AccountAdmin, BrandIDs: []string{upgradeBrand},
		Roles: []access.Role{{BrandID: upgradeBrand, Permissions: []access.Permission{
			{Resource: "reward", Action: "view", Scope: access.ScopeBrand},
			{Resource: "reward", Action: "grant", Scope: access.ScopeBrand},
		}}},
	}
	service := rewards.Service{DB: db}
	grantTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	order, err := service.GrantTx(ctx, grantTx, upgradeBrand, actor,
		rewards.GrantInput{MemberID: member, Points: points.Amount(37), Reason: "pre-0058 positive wallet fixture"},
		points.Metadata{ActorType: "admin", ActorID: adminID, RequestID: ids.New(), IP: "127.0.0.1"})
	if err != nil {
		_ = grantTx.Rollback(ctx)
		t.Fatalf("create real pre-upgrade reward grant: %v", err)
	}
	if err := grantTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if order.ID == "" {
		t.Fatal("reward service returned an empty order ID")
	}
	var available int64
	if err := db.QueryRow(ctx, `SELECT points FROM point_buckets WHERE brand_id=$1 AND account_id=(SELECT id FROM point_accounts WHERE brand_id=$1 AND brand_member_id=$2) AND source='gift' AND state='available'`, upgradeBrand, member).Scan(&available); err != nil || available != 37 {
		t.Fatalf("service grant did not create positive available gift balance: points=%d err=%v", available, err)
	}
	var immutableEvents int
	if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM point_ledger_entries WHERE reference_type='reward_order' AND reference_id=$1::uuid AND entry_type='reward_grant') + (SELECT count(*) FROM reward_order_actions WHERE order_id=$1::uuid)`, order.ID).Scan(&immutableEvents); err != nil || immutableEvents < 2 {
		t.Fatalf("expected real immutable reward ledger/action witnesses, rows=%d err=%v", immutableEvents, err)
	}

	brandBootstrap, platformBootstrap := ids.New(), ids.New()
	brandCustom, platformCustom := ids.New(), ids.New()
	for _, role := range []struct {
		id, brand, code string
		bootstrap       bool
	}{
		{brandBootstrap, upgradeBrand, "upgrade_correction_bootstrap_brand", true},
		{platformBootstrap, "", "upgrade_correction_bootstrap_platform", true},
		{brandCustom, upgradeBrand, "upgrade_correction_custom_brand", false},
		{platformCustom, "", "upgrade_correction_custom_platform", false},
	} {
		if _, err := db.Exec(ctx, `INSERT INTO roles(id,brand_id,code,name,is_bootstrap) VALUES($1,NULLIF($2,'')::uuid,$3,$3,$4)`, role.id, role.brand, role.code, role.bootstrap); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,'report_commission.view.brand'),($2,'report_commission.view.platform')`, brandCustom, platformCustom); err != nil {
		t.Fatal(err)
	}
	brandCustomBefore := rolePermissionSnapshot(t, ctx, db, brandCustom)
	platformCustomBefore := rolePermissionSnapshot(t, ctx, db, platformCustom)
	financialBefore := rewardMigrationFinancialSnapshot(t, ctx, db)

	if err := database.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate schema 57 to 58: %v", err)
	}
	if financialAfter := rewardMigrationFinancialSnapshot(t, ctx, db); financialAfter != financialBefore {
		t.Fatalf("0058 changed existing wallet/reward/audit/outbox/identity/session data:\nbefore %s\nafter  %s", financialBefore, financialAfter)
	}
	for _, table := range []string{"commission_correction_plans", "commission_correction_plan_steps", "commission_correction_plan_targets"} {
		var count int
		if err := db.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&count); err != nil || count != 0 {
			t.Errorf("new preparation table %s count=%d err=%v; migration must not backfill plans", table, count, err)
		}
	}
	assertCommissionCorrectionSearchPaths(t, ctx, db)
	assertCommissionCorrectionPermissions(t, ctx, db, brandBootstrap, platformBootstrap, brandCustom, platformCustom, brandCustomBefore, platformCustomBefore)
	assertCommissionCorrectionSourceValidationReadOnly(t, ctx, db)

	firstState := rewardMigrationFullSnapshot(t, ctx, db)
	firstFinancial := rewardMigrationFinancialSnapshot(t, ctx, db)
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatalf("repeat migration at schema 58: %v", err)
	}
	if repeated := rewardMigrationFullSnapshot(t, ctx, db); repeated != firstState {
		t.Fatalf("repeating migration changed migration/permission/identity state:\nafter first %s\nafter repeat %s", firstState, repeated)
	}
	if repeated := rewardMigrationFinancialSnapshot(t, ctx, db); repeated != firstFinancial {
		t.Fatal("repeating migration changed financial or immutable event history")
	}
	for _, table := range []string{"commission_correction_plans", "commission_correction_plan_steps", "commission_correction_plan_targets"} {
		var count int
		if err := db.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&count); err != nil || count != 0 {
			t.Errorf("repeat migration changed new preparation table %s count=%d err=%v", table, count, err)
		}
	}
}

func rolePermissionSnapshot(t *testing.T, ctx context.Context, db *pgxpool.Pool, roleID string) []string {
	t.Helper()
	var permissions []string
	if err := db.QueryRow(ctx, `SELECT coalesce(array_agg(permission_key ORDER BY permission_key),'{}') FROM role_permissions WHERE role_id=$1`, roleID).Scan(&permissions); err != nil {
		t.Fatal(err)
	}
	return permissions
}

func assertCommissionCorrectionPermissions(t *testing.T, ctx context.Context, db *pgxpool.Pool, brandBootstrap, platformBootstrap, brandCustom, platformCustom string, brandCustomBefore, platformCustomBefore []string) {
	t.Helper()
	var correctionMigrations int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM schema_migrations WHERE name IN('0058_commission_correction_plans.up.sql','0059_commission_correction_execution.up.sql')`).Scan(&correctionMigrations); err != nil || correctionMigrations != 2 {
		t.Fatalf("expected latest schema with both correction migrations, applied=%d err=%v", correctionMigrations, err)
	}
	allCorrectionKeys := []string{
		"commission_correction.approve.brand",
		"commission_correction.continue.brand",
		"commission_correction.execute_retry.brand",
		"commission_correction.retry.brand",
		"commission_correction_policy.write.brand",
	}
	for _, tc := range []struct {
		id, want string
	}{
		{brandBootstrap, "commission_correction.approve.brand,commission_correction.continue.brand,commission_correction.execute_retry.brand,commission_correction.retry.brand,commission_correction_policy.write.brand"},
		{platformBootstrap, ""},
		{brandCustom, ""},
		{platformCustom, ""},
	} {
		var grants string
		if err := db.QueryRow(ctx, `SELECT coalesce(string_agg(permission_key,',' ORDER BY permission_key),'') FROM role_permissions WHERE role_id=$1 AND permission_key=ANY($2::text[])`, tc.id, allCorrectionKeys).Scan(&grants); err != nil || grants != tc.want {
			t.Errorf("role %s correction grants=%q want=%q err=%v", tc.id, grants, tc.want, err)
		}
	}
	if after := rolePermissionSnapshot(t, ctx, db, brandCustom); !equalStrings(after, brandCustomBefore) {
		t.Errorf("migration expanded custom brand role: before=%v after=%v", brandCustomBefore, after)
	}
	if after := rolePermissionSnapshot(t, ctx, db, platformCustom); !equalStrings(after, platformCustomBefore) {
		t.Errorf("migration expanded custom platform role: before=%v after=%v", platformCustomBefore, after)
	}
	var definitions []string
	if err := db.QueryRow(ctx, `SELECT coalesce(array_agg(key ORDER BY key),'{}') FROM permissions WHERE key=ANY($1::text[])`, allCorrectionKeys).Scan(&definitions); err != nil || !equalStrings(definitions, allCorrectionKeys) {
		t.Errorf("correction permission definitions=%v want=%v err=%v", definitions, allCorrectionKeys, err)
	}
}

func assertCommissionCorrectionSearchPaths(t *testing.T, ctx context.Context, db *pgxpool.Pool) {
	t.Helper()
	var schema string
	if err := db.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	wantPath := "search_path=pg_catalog, " + schema + ", pg_temp"
	functionNames := []string{
		"commission_correction_candidates", "commission_correction_source_valid", "commission_correction_mode",
		"guard_commission_correction_plan_step", "guard_commission_correction_plan", "guard_commission_correction_plan_target",
		"require_commission_correction_plan_commit", "require_commission_correction_plan_step_commit", "require_commission_correction_plan_target_commit",
		"guard_unexecuted_commission_correction_credit",
	}
	rows, err := db.Query(ctx, `SELECT p.proname,p.proconfig FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=$1 AND p.proname=ANY($2::text[])`, schema, functionNames)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	paths := make(map[string]string, len(functionNames))
	for rows.Next() {
		var name string
		var config []string
		if err := rows.Scan(&name, &config); err != nil {
			t.Fatal(err)
		}
		for _, setting := range config {
			if len(setting) >= len("search_path=") && setting[:len("search_path=")] == "search_path=" {
				paths[name] = setting
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, name := range functionNames {
		if got := paths[name]; got != wantPath {
			t.Errorf("%s search_path=%q want=%q", name, got, wantPath)
		}
	}
}

func assertCommissionCorrectionSourceValidationReadOnly(t *testing.T, ctx context.Context, db *pgxpool.Pool) {
	t.Helper()
	unknownPayment, unknownRun := ids.New(), ids.New()
	var valid bool
	if err := db.QueryRow(ctx, `SELECT commission_correction_source_valid($1,$2,$3)`, upgradeBrand, unknownPayment, unknownRun).Scan(&valid); err != nil || valid {
		t.Fatalf("unknown correction source valid=%t err=%v; want false", valid, err)
	}

	// These temporary rows intentionally describe a plausible old paid payment
	// with a missing genuine commission posting. 0058 functions pin their own
	// search_path to the application schema, before pg_temp, so test-only rows
	// cannot become financial evidence. No guard is disabled and no commission
	// ledger credit is inserted.
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for _, statement := range []string{
		`CREATE TEMP TABLE commission_runs (LIKE commission_runs INCLUDING DEFAULTS)`,
		`CREATE TEMP TABLE commission_payments (LIKE commission_payments INCLUDING DEFAULTS)`,
		`CREATE TEMP TABLE commission_payment_targets (LIKE commission_payment_targets INCLUDING DEFAULTS)`,
	} {
		if _, err := tx.Exec(ctx, statement); err != nil {
			t.Fatalf("prepare isolated temporary source-validation fixture: %v", err)
		}
	}
	brand, cycle, currentRun, oldRun, payment, target := upgradeBrand, ids.New(), ids.New(), ids.New(), ids.New(), ids.New()
	if _, err := tx.Exec(ctx, `INSERT INTO pg_temp.commission_runs(id,brand_id,cycle_id,generation,evidence_epoch,state) VALUES($1,$2,$3,1,0,'ready')`, currentRun, brand, cycle); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO pg_temp.commission_runs(id,brand_id,cycle_id,generation,evidence_epoch,state) VALUES($1,$2,$3,1,0,'ready')`, oldRun, brand, cycle); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO pg_temp.commission_payments(id,brand_id,cycle_id,run_id,evidence_epoch,payout_mode,state,total_points,target_count,creation_audit_log_id,last_audit_log_id,last_error_code) VALUES($1,$2,$3,$4,0,'manual','blocked',10,1,$5,$5,'COMMISSION_PAYMENT_CORRECTION_REQUIRED')`, payment, brand, cycle, oldRun, ids.New()); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO pg_temp.commission_payment_targets(id,brand_id,payment_id,earning_id,points,member_id,agent_id,payment_version,state,ledger_entry_id) VALUES($1,$2,$3,$4,10,$5,$6,1,'paid',$7)`, target, brand, payment, ids.New(), ids.New(), ids.New(), ids.New()); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT commission_correction_source_valid($1,$2,$3)`, brand, payment, currentRun).Scan(&valid); err != nil || valid {
		t.Fatalf("pg_temp-only old paid source spoof valid=%t err=%v; want false", valid, err)
	}
	var temporaryRows, realRows int
	var schema string
	if err := tx.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	paymentsTable := pgx.Identifier{schema, "commission_payments"}.Sanitize()
	runsTable := pgx.Identifier{schema, "commission_runs"}.Sanitize()
	targetsTable := pgx.Identifier{schema, "commission_payment_targets"}.Sanitize()
	query := `SELECT (SELECT count(*) FROM pg_temp.commission_payments)+(SELECT count(*) FROM pg_temp.commission_runs)+(SELECT count(*) FROM pg_temp.commission_payment_targets), (` +
		`SELECT count(*) FROM ` + paymentsTable + ` WHERE id=$1)+(SELECT count(*) FROM ` + runsTable + ` WHERE id IN($2,$3))+(SELECT count(*) FROM ` + targetsTable + ` WHERE id=$4)`
	if err := tx.QueryRow(ctx, query, payment, currentRun, oldRun, target).Scan(&temporaryRows, &realRows); err != nil || temporaryRows != 4 || realRows != 0 {
		t.Fatalf("source spoof fixture isolation temp=%d real=%d err=%v", temporaryRows, realRows, err)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
