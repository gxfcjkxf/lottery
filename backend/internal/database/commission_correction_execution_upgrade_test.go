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

func TestCommissionCorrectionExecution0058To0059DefaultsOffAndPreservesHistory(t *testing.T) {
	ctx := context.Background()
	db := testdb.NewAtVersion(t, 58)
	member, _ := upgradeWallet(t, db, false)
	adminID := ids.New()
	if _, err := db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'correction-execution-upgrade-test')`, adminID, "execution_upgrade_"+adminID[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO sessions(id,token_hash,admin_id,expires_at) VALUES($1,$2,$3,clock_timestamp()+interval '1 hour')`, ids.New(), "execution_upgrade_session_"+adminID, adminID); err != nil {
		t.Fatal(err)
	}
	actor := access.Account{
		ID: adminID, Type: access.AccountAdmin, BrandIDs: []string{upgradeBrand},
		Roles: []access.Role{{BrandID: upgradeBrand, Permissions: []access.Permission{
			{Resource: "reward", Action: "view", Scope: access.ScopeBrand},
			{Resource: "reward", Action: "grant", Scope: access.ScopeBrand},
		}}},
	}
	grantTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	order, err := (rewards.Service{DB: db}).GrantTx(ctx, grantTx, upgradeBrand, actor,
		rewards.GrantInput{MemberID: member, Points: points.Amount(53), Reason: "pre-0059 historical financial fixture"},
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

	brandBootstrap, platformBootstrap := ids.New(), ids.New()
	brandCustom, platformCustom := ids.New(), ids.New()
	for _, role := range []struct {
		id, brand, code string
		bootstrap       bool
	}{
		{brandBootstrap, upgradeBrand, "upgrade_execution_bootstrap_brand", true},
		{platformBootstrap, "", "upgrade_execution_bootstrap_platform", true},
		{brandCustom, upgradeBrand, "upgrade_execution_custom_brand", false},
		{platformCustom, "", "upgrade_execution_custom_platform", false},
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
		t.Fatalf("migrate schema 58 to 59: %v", err)
	}
	if after := rewardMigrationFinancialSnapshot(t, ctx, db); after != financialBefore {
		t.Fatalf("0059 changed wallet/reward/audit/outbox/identity/session history:\nbefore %s\nafter  %s", financialBefore, after)
	}
	assertCorrectionExecutionPoliciesStartDisabled(t, ctx, db)
	assertNoCorrectionExecutionBackfill(t, ctx, db)
	assertCorrectionExecutionPermissions(t, ctx, db, brandBootstrap, platformBootstrap, brandCustom, platformCustom, brandCustomBefore, platformCustomBefore)
	assertCorrectionExecutionSearchPaths(t, ctx, db)

	firstState := rewardMigrationFullSnapshot(t, ctx, db)
	firstFinancial := rewardMigrationFinancialSnapshot(t, ctx, db)
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatalf("repeat migration at schema 59: %v", err)
	}
	if after := rewardMigrationFullSnapshot(t, ctx, db); after != firstState {
		t.Fatalf("repeating migration changed business/permission state:\nafter first %s\nafter repeat %s", firstState, after)
	}
	if after := rewardMigrationFinancialSnapshot(t, ctx, db); after != firstFinancial {
		t.Fatal("repeating migration changed financial history")
	}
	assertCorrectionExecutionPoliciesStartDisabled(t, ctx, db)
	assertNoCorrectionExecutionBackfill(t, ctx, db)
}

func assertCorrectionExecutionPoliciesStartDisabled(t *testing.T, ctx context.Context, db *pgxpool.Pool) {
	t.Helper()
	var brands, policies, revisions, invalidPolicies, invalidRevisions int
	if err := db.QueryRow(ctx, `SELECT
	 (SELECT count(*) FROM brands),
	 (SELECT count(*) FROM brand_commission_correction_policies),
	 (SELECT count(*) FROM commission_correction_policy_revisions),
	 (SELECT count(*) FROM brand_commission_correction_policies WHERE version<>1 OR enabled OR audit_log_id IS NOT NULL),
	 (SELECT count(*) FROM commission_correction_policy_revisions WHERE version<>1 OR enabled OR audit_log_id IS NOT NULL)`).
		Scan(&brands, &policies, &revisions, &invalidPolicies, &invalidRevisions); err != nil {
		t.Fatal(err)
	}
	if brands == 0 || policies != brands || revisions != brands || invalidPolicies != 0 || invalidRevisions != 0 {
		t.Fatalf("correction rollout did not initialize every brand closed: brands=%d policies=%d revisions=%d invalid=(%d,%d)", brands, policies, revisions, invalidPolicies, invalidRevisions)
	}
	var missingBrands, missingRevisions int
	if err := db.QueryRow(ctx, `SELECT
	 (SELECT count(*) FROM brands b LEFT JOIN brand_commission_correction_policies p ON p.brand_id=b.id WHERE p.brand_id IS NULL),
	 (SELECT count(*) FROM brand_commission_correction_policies p LEFT JOIN commission_correction_policy_revisions r ON r.brand_id=p.brand_id AND r.version=p.version WHERE r.brand_id IS NULL)`).Scan(&missingBrands, &missingRevisions); err != nil || missingBrands != 0 || missingRevisions != 0 {
		t.Fatalf("default-off policy history incomplete: missing brands=%d revisions=%d err=%v", missingBrands, missingRevisions, err)
	}
}

func assertNoCorrectionExecutionBackfill(t *testing.T, ctx context.Context, db *pgxpool.Pool) {
	t.Helper()
	for _, table := range []string{
		"commission_correction_executions", "commission_correction_execution_targets",
		"commission_correction_execution_steps", "commission_correction_balance_heads", "commission_correction_cycle_holds",
	} {
		var count int
		if err := db.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&count); err != nil || count != 0 {
			t.Errorf("0059 backfilled execution/financial table %s: rows=%d err=%v", table, count, err)
		}
	}
}

func assertCorrectionExecutionPermissions(t *testing.T, ctx context.Context, db *pgxpool.Pool, brandBootstrap, platformBootstrap, brandCustom, platformCustom string, brandCustomBefore, platformCustomBefore []string) {
	t.Helper()
	wantKeys := []string{
		"commission_correction.approve.brand", "commission_correction.continue.brand",
		"commission_correction.execute_retry.brand", "commission_correction_policy.write.brand",
	}
	var definitions []string
	if err := db.QueryRow(ctx, `SELECT coalesce(array_agg(key ORDER BY key),'{}') FROM permissions WHERE key=ANY($1::text[])`, wantKeys).Scan(&definitions); err != nil {
		t.Fatal(err)
	}
	if !sameSortedStrings(definitions, wantKeys) {
		t.Errorf("correction execution permission definitions=%v want=%v", definitions, wantKeys)
	}
	for _, tc := range []struct {
		id   string
		want []string
	}{
		{brandBootstrap, wantKeys}, {platformBootstrap, nil}, {brandCustom, nil}, {platformCustom, nil},
	} {
		var grants []string
		if err := db.QueryRow(ctx, `SELECT coalesce(array_agg(permission_key ORDER BY permission_key),'{}') FROM role_permissions WHERE role_id=$1 AND permission_key=ANY($2::text[])`, tc.id, wantKeys).Scan(&grants); err != nil {
			t.Fatal(err)
		}
		if !sameSortedStrings(grants, tc.want) {
			t.Errorf("role %s correction execution permissions=%v want=%v", tc.id, grants, tc.want)
		}
	}
	if after := rolePermissionSnapshot(t, ctx, db, brandCustom); !equalStrings(after, brandCustomBefore) {
		t.Errorf("migration expanded custom brand role: before=%v after=%v", brandCustomBefore, after)
	}
	if after := rolePermissionSnapshot(t, ctx, db, platformCustom); !equalStrings(after, platformCustomBefore) {
		t.Errorf("migration expanded custom platform role: before=%v after=%v", platformCustomBefore, after)
	}
}

func assertCorrectionExecutionSearchPaths(t *testing.T, ctx context.Context, db *pgxpool.Pool) {
	t.Helper()
	var schema string
	if err := db.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	wantPath := "search_path=pg_catalog, " + schema + ", pg_temp"
	functionNames := []string{
		"guard_commission_correction_policy", "capture_commission_correction_policy", "guard_commission_correction_policy_revision",
		"initialize_commission_correction_policy", "commission_correction_candidates_v2", "commission_correction_candidates",
		"commission_correction_heads_valid", "commission_correction_execution_current", "commission_correction_source_valid",
		"guard_commission_correction_plan_target",
		"guard_commission_correction_execution_step", "guard_commission_correction_cycle_hold", "guard_commission_correction_execution",
		"guard_commission_correction_execution_target", "guard_commission_correction_balance_head", "capture_commission_correction_balance_head",
		"guard_unexecuted_commission_correction_credit", "require_commission_correction_execution_commit",
		"require_commission_correction_execution_step_commit", "require_commission_correction_execution_target_commit",
		"require_commission_correction_ledger_commit",
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

func sameSortedStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
