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
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRewardReports0056To0057GrantsOnlyBootstrapAndPreservesFinance(t *testing.T) {
	db := testdb.NewAtVersion(t, 56)
	ctx := context.Background()
	brandBootstrap, platformBootstrap := ids.New(), ids.New()
	brandCustom, platformCustom := ids.New(), ids.New()
	for _, role := range []struct {
		id, brand, code string
		bootstrap       bool
	}{
		{brandBootstrap, upgradeBrand, "upgrade_reward_bootstrap_brand", true},
		{platformBootstrap, "", "upgrade_reward_bootstrap_platform", true},
		{brandCustom, upgradeBrand, "upgrade_reward_custom_brand", false},
		{platformCustom, "", "upgrade_reward_custom_platform", false},
	} {
		if _, err := db.Exec(ctx, `INSERT INTO roles(id,brand_id,code,name,is_bootstrap) VALUES($1,NULLIF($2,'')::uuid,$3,$3,$4)`, role.id, role.brand, role.code, role.bootstrap); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,'reward.view.brand'),($2,'reward.view.platform')`, brandCustom, platformCustom); err != nil {
		t.Fatal(err)
	}
	createRewardReportUpgradeHistory(t, ctx, db)
	financialBefore := rewardMigrationFinancialSnapshot(t, ctx, db)
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	financialAfter := rewardMigrationFinancialSnapshot(t, ctx, db)
	if financialAfter != financialBefore {
		t.Fatalf("0057 changed financial state:\nbefore %s\nafter  %s", financialBefore, financialAfter)
	}
	stateAfterFirstMigration := rewardMigrationFullSnapshot(t, ctx, db)
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatalf("repeat migration failed: %v", err)
	}
	stateAfterRepeatMigration := rewardMigrationFullSnapshot(t, ctx, db)
	if stateAfterRepeatMigration != stateAfterFirstMigration {
		t.Fatalf("repeating 0057 changed migration/business state:\nafter first %s\nafter repeat %s", stateAfterFirstMigration, stateAfterRepeatMigration)
	}
	for _, roleID := range []string{brandCustom, platformCustom} {
		var grants []string
		if err := db.QueryRow(ctx, `SELECT coalesce(array_agg(permission_key ORDER BY permission_key),'{}') FROM role_permissions WHERE role_id=$1 AND permission_key LIKE 'report_reward.%'`, roleID).Scan(&grants); err != nil {
			t.Fatal(err)
		}
		if len(grants) != 0 {
			t.Fatalf("0057 auto-authorized custom role %s: %v", roleID, grants)
		}
	}
	for _, tc := range []struct {
		id, expected string
	}{
		{brandBootstrap, "report_reward.export.brand,report_reward.view.brand"},
		{platformBootstrap, "report_reward.export.platform,report_reward.view.platform"},
	} {
		var grants string
		if err := db.QueryRow(ctx, `SELECT coalesce(string_agg(permission_key,',' ORDER BY permission_key),'') FROM role_permissions WHERE role_id=$1 AND permission_key LIKE 'report_reward.%'`, tc.id).Scan(&grants); err != nil {
			t.Fatal(err)
		}
		if grants != tc.expected {
			t.Fatalf("bootstrap role %s grants=%q want=%q", tc.id, grants, tc.expected)
		}
	}
	var rights int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM permissions WHERE key IN('report_reward.view.brand','report_reward.view.platform','report_reward.export.brand','report_reward.export.platform')`).Scan(&rights); err != nil || rights != 4 {
		t.Fatalf("0057 report permission definitions=%d err=%v", rights, err)
	}
}

func createRewardReportUpgradeHistory(t *testing.T, ctx context.Context, db *pgxpool.Pool) {
	t.Helper()
	member, _ := upgradeWallet(t, db, false)
	adminID := ids.New()
	if _, err := db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'reward-upgrade-test-only')`, adminID, "reward_upgrade_"+adminID[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO sessions(id,token_hash,admin_id,expires_at) VALUES($1,$2,$3,clock_timestamp()+interval '1 hour')`, ids.New(), "reward-upgrade-session-"+adminID, adminID); err != nil {
		t.Fatal(err)
	}
	actor := access.Account{
		ID: adminID, Type: access.AccountAdmin, BrandIDs: []string{upgradeBrand},
		Roles: []access.Role{{BrandID: upgradeBrand, Permissions: []access.Permission{
			{Resource: "reward", Action: "view", Scope: access.ScopeBrand},
			{Resource: "reward", Action: "grant", Scope: access.ScopeBrand},
			{Resource: "reward", Action: "revoke", Scope: access.ScopeBrand},
		}}},
	}
	service := rewards.Service{DB: db}
	grant := func(pointsAmount int64) rewards.Order {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		order, err := service.GrantTx(ctx, tx, upgradeBrand, actor, rewards.GrantInput{MemberID: member, Points: points.Amount(pointsAmount), Reason: "pre-0057 upgrade grant"}, points.Metadata{ActorType: "admin", ActorID: adminID, RequestID: ids.New(), IP: "127.0.0.1"})
		if err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("create pre-0057 grant: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return order
	}
	revoke := func(order rewards.Order) rewards.Order {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		updated, err := service.RevokeTx(ctx, tx, upgradeBrand, order.ID, actor, rewards.ActionInput{Version: order.Version, Reason: "pre-0057 upgrade revoke"}, points.Metadata{ActorType: "admin", ActorID: adminID, RequestID: ids.New(), IP: "127.0.0.1"})
		if err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("create pre-0057 revoke: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return updated
	}
	grant(11) // Remains granted after the migration.
	fullyRevoked := revoke(grant(13))
	if fullyRevoked.State != "revoked" {
		t.Fatalf("pre-0057 full revoke state=%s", fullyRevoked.State)
	}
	pending := grant(17)
	store := points.Store{DB: db}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wallet, err := store.LockedSnapshot(ctx, tx, upgradeBrand, member)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	allocation, err := wallet.BySource.Allocate(wallet.GiftPoints, "available")
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	delta, err := points.AllocationDelta(allocation, "available", "manual_frozen")
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if _, err = store.Post(ctx, tx, points.Change{BrandID: upgradeBrand, MemberID: member, EntryType: "freeze", ReferenceType: "wallet_freeze",
		OperationKey: "reward-upgrade-freeze:" + ids.New(), Reason: "freeze before legacy upgrade", ActorType: "admin", ActorID: adminID,
		RequestID: ids.New(), Delta: delta, Allocation: allocation}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("freeze pre-0057 wallet: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	pending = revoke(pending)
	if pending.State != "revocation_pending" {
		t.Fatalf("pre-0057 pending revoke state=%s", pending.State)
	}
	var states []string
	if err := db.QueryRow(ctx, `SELECT array_agg(state ORDER BY state) FROM reward_orders WHERE brand_id=$1`, upgradeBrand).Scan(&states); err != nil {
		t.Fatal(err)
	}
	if len(states) != 3 || states[0] != "granted" || states[1] != "revocation_pending" || states[2] != "revoked" {
		t.Fatalf("pre-0057 reward cohort states=%v", states)
	}
}

func rewardMigrationFinancialSnapshot(t *testing.T, ctx context.Context, db *pgxpool.Pool) string {
	t.Helper()
	var snapshot string
	query := `SELECT jsonb_build_object(
	'global_users',(SELECT coalesce(jsonb_agg(to_jsonb(u) ORDER BY id),'[]'::jsonb) FROM global_users u),
	'brand_members',(SELECT coalesce(jsonb_agg(to_jsonb(m) ORDER BY id),'[]'::jsonb) FROM brand_members m),
	'admin_accounts',(SELECT coalesce(jsonb_agg(to_jsonb(a) ORDER BY id),'[]'::jsonb) FROM admin_accounts a),
	'sessions',(SELECT coalesce(jsonb_agg(to_jsonb(s) ORDER BY id),'[]'::jsonb) FROM sessions s),
	'point_accounts',(SELECT coalesce(jsonb_agg(to_jsonb(a) ORDER BY id),'[]'::jsonb) FROM point_accounts a),
	'point_buckets',(SELECT coalesce(jsonb_agg(to_jsonb(b) ORDER BY account_id,source,state),'[]'::jsonb) FROM point_buckets b),
	'point_ledger_entries',(SELECT coalesce(jsonb_agg(to_jsonb(l) ORDER BY id),'[]'::jsonb) FROM point_ledger_entries l),
	'reward_orders',(SELECT coalesce(jsonb_agg(to_jsonb(o) ORDER BY id),'[]'::jsonb) FROM reward_orders o),
	'reward_order_actions',(SELECT coalesce(jsonb_agg(to_jsonb(a) ORDER BY id),'[]'::jsonb) FROM reward_order_actions a),
	'audit_logs',(SELECT coalesce(jsonb_agg(to_jsonb(a) ORDER BY id),'[]'::jsonb) FROM audit_logs a),
	'outbox_events',(SELECT coalesce(jsonb_agg(to_jsonb(e) ORDER BY id),'[]'::jsonb) FROM outbox_events e),
	'notification_deliveries',(SELECT coalesce(jsonb_agg(to_jsonb(d) ORDER BY event_id),'[]'::jsonb) FROM notification_deliveries d)
)::text`
	if err := db.QueryRow(ctx, query).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func rewardMigrationFullSnapshot(t *testing.T, ctx context.Context, db *pgxpool.Pool) string {
	t.Helper()
	var snapshot string
	query := `SELECT jsonb_build_object(
 'schema_migrations',(SELECT coalesce(jsonb_agg(to_jsonb(m) ORDER BY name),'[]'::jsonb) FROM schema_migrations m),
 'permissions',(SELECT coalesce(jsonb_agg(to_jsonb(p) ORDER BY key),'[]'::jsonb) FROM permissions p),
 'roles',(SELECT coalesce(jsonb_agg(to_jsonb(r) ORDER BY id),'[]'::jsonb) FROM roles r),
 'role_permissions',(SELECT coalesce(jsonb_agg(to_jsonb(rp) ORDER BY role_id,permission_key),'[]'::jsonb) FROM role_permissions rp),
 'admin_accounts',(SELECT coalesce(jsonb_agg(to_jsonb(a) ORDER BY id),'[]'::jsonb) FROM admin_accounts a),
 'global_users',(SELECT coalesce(jsonb_agg(to_jsonb(u) ORDER BY id),'[]'::jsonb) FROM global_users u),
 'brand_members',(SELECT coalesce(jsonb_agg(to_jsonb(m) ORDER BY id),'[]'::jsonb) FROM brand_members m),
 'sessions',(SELECT coalesce(jsonb_agg(to_jsonb(s) ORDER BY id),'[]'::jsonb) FROM sessions s),
 'point_accounts',(SELECT coalesce(jsonb_agg(to_jsonb(a) ORDER BY id),'[]'::jsonb) FROM point_accounts a),
 'point_buckets',(SELECT coalesce(jsonb_agg(to_jsonb(b) ORDER BY account_id,source,state),'[]'::jsonb) FROM point_buckets b),
 'point_ledger_entries',(SELECT coalesce(jsonb_agg(to_jsonb(l) ORDER BY id),'[]'::jsonb) FROM point_ledger_entries l),
 'reward_orders',(SELECT coalesce(jsonb_agg(to_jsonb(o) ORDER BY id),'[]'::jsonb) FROM reward_orders o),
 'reward_order_actions',(SELECT coalesce(jsonb_agg(to_jsonb(a) ORDER BY id),'[]'::jsonb) FROM reward_order_actions a),
 'audit_logs',(SELECT coalesce(jsonb_agg(to_jsonb(a) ORDER BY id),'[]'::jsonb) FROM audit_logs a),
 'outbox_events',(SELECT coalesce(jsonb_agg(to_jsonb(e) ORDER BY id),'[]'::jsonb) FROM outbox_events e),
 'notification_deliveries',(SELECT coalesce(jsonb_agg(to_jsonb(d) ORDER BY event_id),'[]'::jsonb) FROM notification_deliveries d)
)::text`
	if err := db.QueryRow(ctx, query).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	return snapshot
}
