package notification

import (
	"context"
	"fmt"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/events"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDrawNotificationUpgrade0068PreservesHistoryAndAddsOnlyFutureTemplates(t *testing.T) {
	ctx := context.Background()
	db := notificationSchemaBefore(t, "0069_")
	var lastMigration, defaultsBefore, defaultsOIDBefore, validatorOIDBefore string
	if err := db.QueryRow(ctx, `SELECT (SELECT max(name) FROM schema_migrations),
 notification_template_defaults()::text,to_regprocedure('notification_template_defaults()')::oid::text,
 to_regprocedure('valid_notification_template_content(text,jsonb)')::oid::text`).
		Scan(&lastMigration, &defaultsBefore, &defaultsOIDBefore, &validatorOIDBefore); err != nil {
		t.Fatal(err)
	}
	if lastMigration[:4] != "0068" {
		t.Fatalf("fixture must stop at 0068, got %s", lastMigration)
	}

	var oldBrands []string
	if err := db.QueryRow(ctx, `SELECT array_agg(id::text ORDER BY id) FROM brands`).Scan(&oldBrands); err != nil {
		t.Fatal(err)
	}
	if len(oldBrands) == 0 {
		t.Fatal("0068 fixture has no seeded brands")
	}
	for _, brandID := range oldBrands {
		drawNotificationTemplateCounts(t, db, brandID, 20, 20)
	}

	userID, memberID, accountID, adminID := ids.New(), ids.New(), ids.New(), ids.New()
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO global_users(id,username,password_hash) VALUES($1,$2,'test-only')`, []any{userID, "draw_notice_upgrade_" + userID[24:]}},
		{`INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'operator','1','1')`, []any{memberID, brand, userID}},
		{`INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only')`, []any{adminID, "draw_notice_admin_" + adminID[24:]}},
		{`INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, []any{accountID, brand, memberID}},
		{`INSERT INTO point_buckets(brand_id,account_id,source,state,points) SELECT $1,$2,s,t,0
 FROM unnest(ARRAY['recharge','winning','gift']) s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal']) t`, []any{brand, accountID}},
	} {
		if _, err := db.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}

	// Create one genuine pre-upgrade ledger posting so the financial fingerprint
	// protects nonempty ledger, balance and audit state across the migration.
	var delta points.Balance
	delta[2][0] = 9
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (points.Store{DB: db}).Post(ctx, tx, points.Change{
		BrandID: brand, MemberID: memberID, EntryType: "adjustment", ReferenceType: "draw_notification_upgrade",
		ReferenceID: memberID, OperationKey: "draw-notice-upgrade-ledger:" + memberID,
		Reason: "Preserve a pre-upgrade financial record", ActorType: "system", RequestID: ids.New(), Delta: delta,
		Allocation: []points.Allocation{{Source: "gift", State: "available", Points: 9}},
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// A custom role with no grants must not acquire permissions during this
	// notification-only schema upgrade.
	customRoleID := ids.New()
	if _, err = db.Exec(ctx, `INSERT INTO roles(id,brand_id,code,name,is_bootstrap) VALUES($1,$2,$3,'Draw notice custom role',false)`, customRoleID, brand, "draw_notice_custom_"+customRoleID[24:]); err != nil {
		t.Fatal(err)
	}
	actor := access.Account{ID: adminID, Type: access.AccountAdmin, BrandIDs: []string{brand}, Roles: []access.Role{{BrandID: brand, Permissions: []access.Permission{{Resource: "notification_template", Action: "write", Scope: access.ScopeBrand}}}}}
	custom := Content{En: Copy{Title: "Existing welcome", Body: "Preserve this approved welcome."}, ZhCN: Copy{Title: "原有欢迎文案", Body: "保留已批准的欢迎文案。"}}
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = (Service{DB: db}).UpdateTemplate(ctx, tx, brand, "member.joined", actor,
		UpdateInput{Version: 1, Content: custom, Reason: "Customize before draw notification upgrade"}, templateMeta(actor)); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = events.Append(ctx, tx, brand, "member.joined", memberID, memberID, nil); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if done, err := (Service{DB: db}).Process(ctx, 20); err != nil || done != 1 {
		t.Fatalf("pre-upgrade notification materialization: done=%d err=%v", done, err)
	}

	oldRevisionCounts := make(map[string]int, len(oldBrands))
	for _, brandID := range oldBrands {
		var count int
		if err = db.QueryRow(ctx, `SELECT count(*) FROM notification_template_revisions WHERE brand_id=$1`, brandID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		oldRevisionCounts[brandID] = count
	}
	if oldRevisionCounts[brand] != 21 {
		t.Fatalf("fixture brand should have twenty defaults plus its v2 history, got %d revisions", oldRevisionCounts[brand])
	}

	before := drawNotificationUpgradeFingerprint(t, db)
	permissionsBefore := drawNotificationUpgradeFingerprintValue(t, db, "permissions")
	var roleGrantsBefore string
	if err = db.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(rp) ORDER BY to_jsonb(rp)::text),'[]'::jsonb)::text FROM role_permissions rp WHERE role_id=$1`, customRoleID).Scan(&roleGrantsBefore); err != nil {
		t.Fatal(err)
	}
	var outboxDrawBefore, inboxDrawBefore int
	if err = db.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM outbox_events WHERE event_type IN('draw.result.published','draw.result.corrected')),
	 (SELECT count(*) FROM notifications WHERE event_type IN('draw.result.published','draw.result.corrected'))`).
		Scan(&outboxDrawBefore, &inboxDrawBefore); err != nil {
		t.Fatal(err)
	}

	if err = database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	var defaultsAfter, defaultsOIDAfter, validatorOIDAfter string
	if err = db.QueryRow(ctx, `SELECT notification_template_defaults()::text,
 to_regprocedure('notification_template_defaults()')::oid::text,
 to_regprocedure('valid_notification_template_content(text,jsonb)')::oid::text`).
		Scan(&defaultsAfter, &defaultsOIDAfter, &validatorOIDAfter); err != nil {
		t.Fatal(err)
	}
	if defaultsOIDAfter != defaultsOIDBefore || validatorOIDAfter != validatorOIDBefore {
		t.Fatalf("template function OID changed: defaults %s/%s validator %s/%s", defaultsOIDBefore, defaultsOIDAfter, validatorOIDBefore, validatorOIDAfter)
	}
	var pinnedTemplateFunctions int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
 WHERE n.nspname=current_schema() AND p.proname=ANY(ARRAY['notification_template_defaults','valid_notification_template_content'])
 AND p.proconfig=ARRAY['search_path=pg_catalog, '||current_schema()||', pg_temp']`).Scan(&pinnedTemplateFunctions); err != nil || pinnedTemplateFunctions != 2 {
		t.Fatalf("template function search_path pins=%d want=2 err=%v", pinnedTemplateFunctions, err)
	}
	var oldDefaultsAfter string
	if err = db.QueryRow(ctx, `SELECT (notification_template_defaults()-'draw.result.published'-'draw.result.corrected')::text`).Scan(&oldDefaultsAfter); err != nil || oldDefaultsAfter != defaultsBefore {
		t.Fatalf("existing defaults changed: before=%s after=%s err=%v", defaultsBefore, oldDefaultsAfter, err)
	}
	for _, brandID := range oldBrands {
		drawNotificationTemplateCounts(t, db, brandID, 22, oldRevisionCounts[brandID]+2)
	}
	var drawTemplates, drawRevisions int
	if err = db.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM notification_templates WHERE brand_id=$1 AND template_key IN('draw.result.published','draw.result.corrected') AND version=1 AND content=notification_template_defaults()->template_key),
 (SELECT count(*) FROM notification_template_revisions WHERE brand_id=$1 AND template_key IN('draw.result.published','draw.result.corrected') AND version=1 AND content=notification_template_defaults()->template_key AND changed_by IS NULL AND audit_log_id IS NULL AND reason='Initial in-app notification template')`, brand).Scan(&drawTemplates, &drawRevisions); err != nil || drawTemplates != 2 || drawRevisions != 2 {
		t.Fatalf("draw defaults/revisions=%d/%d err=%v", drawTemplates, drawRevisions, err)
	}
	for name, value := range before {
		if got := drawNotificationUpgradeFingerprintValue(t, db, name); got != value {
			t.Errorf("upgrade changed existing %s", name)
		}
	}
	var roleGrantsAfter string
	if err = db.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(rp) ORDER BY to_jsonb(rp)::text),'[]'::jsonb)::text FROM role_permissions rp WHERE role_id=$1`, customRoleID).Scan(&roleGrantsAfter); err != nil || roleGrantsAfter != roleGrantsBefore {
		t.Fatalf("custom role grants changed: before=%s after=%s err=%v", roleGrantsBefore, roleGrantsAfter, err)
	}
	var outboxDrawAfter, inboxDrawAfter, publicationsAfter, recipientsAfter int
	if err = db.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM outbox_events WHERE event_type IN('draw.result.published','draw.result.corrected')),
 (SELECT count(*) FROM notifications WHERE event_type IN('draw.result.published','draw.result.corrected')),
 (SELECT count(*) FROM draw_notification_publications),
 (SELECT count(*) FROM draw_notification_recipients)`).
		Scan(&outboxDrawAfter, &inboxDrawAfter, &publicationsAfter, &recipientsAfter); err != nil ||
		outboxDrawAfter != outboxDrawBefore || inboxDrawAfter != inboxDrawBefore || publicationsAfter != 0 || recipientsAfter != 0 {
		t.Fatalf("upgrade backfilled draw audiences: outbox/inbox/publications/recipients=%d/%d/%d/%d before=%d/%d err=%v", outboxDrawAfter, inboxDrawAfter, publicationsAfter, recipientsAfter, outboxDrawBefore, inboxDrawBefore, err)
	}

	newBrand := ids.New()
	if _, err = db.Exec(ctx, `INSERT INTO brands(id,code,name,status) VALUES($1,$2,'After draw notification upgrade','paused')`, newBrand, "draw_notice_new_"+newBrand[24:]); err != nil {
		t.Fatal(err)
	}
	drawNotificationTemplateCounts(t, db, newBrand, 22, 22)
	var validDefault bool
	if err = db.QueryRow(ctx, `SELECT valid_notification_template_content('draw.result.published',notification_template_defaults()->'draw.result.published')`).Scan(&validDefault); err != nil || !validDefault {
		t.Fatalf("draw template SQL validator=%t err=%v", validDefault, err)
	}
	permissionsAfter := drawNotificationUpgradeFingerprintValue(t, db, "permissions")
	if permissionsAfter != permissionsBefore {
		t.Fatalf("upgrade changed permission snapshot: before=%s after=%s", permissionsBefore, permissionsAfter)
	}

	stable := drawNotificationUpgradeFingerprint(t, db)
	newDefaultsStable := drawNotificationUpgradeDefaultsFingerprint(t, db)
	defaultsStable, defaultsOIDStable, validatorOIDStable := defaultsAfter, defaultsOIDAfter, validatorOIDAfter
	if err = database.Migrate(ctx, db); err != nil {
		t.Fatal("repeat migration", err)
	}
	for name, value := range stable {
		if got := drawNotificationUpgradeFingerprintValue(t, db, name); got != value {
			t.Errorf("repeat migration changed %s", name)
		}
	}
	if got := drawNotificationUpgradeDefaultsFingerprint(t, db); got != newDefaultsStable {
		t.Error("repeat migration changed draw templates or revisions")
	}
	var defaultsRepeated, defaultsOIDRepeated, validatorOIDRepeated string
	if err = db.QueryRow(ctx, `SELECT notification_template_defaults()::text,
 to_regprocedure('notification_template_defaults()')::oid::text,
 to_regprocedure('valid_notification_template_content(text,jsonb)')::oid::text`).
		Scan(&defaultsRepeated, &defaultsOIDRepeated, &validatorOIDRepeated); err != nil || defaultsRepeated != defaultsStable || defaultsOIDRepeated != defaultsOIDStable || validatorOIDRepeated != validatorOIDStable {
		t.Fatalf("repeat migration changed template functions: defaults_equal=%t OIDs=%s/%s,%s/%s err=%v", defaultsRepeated == defaultsStable, defaultsOIDStable, defaultsOIDRepeated, validatorOIDStable, validatorOIDRepeated, err)
	}
	var migrationCount int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM schema_migrations WHERE name='0069_draw_notification_templates.up.sql'`).Scan(&migrationCount); err != nil || migrationCount != 1 {
		t.Fatalf("0069 migration records=%d err=%v", migrationCount, err)
	}
}

func drawNotificationTemplateCounts(t *testing.T, db *pgxpool.Pool, brandID string, wantTemplates, wantRevisions int) {
	t.Helper()
	var templates, revisions int
	if err := db.QueryRow(context.Background(), `SELECT
 (SELECT count(*) FROM notification_templates WHERE brand_id=$1),
 (SELECT count(*) FROM notification_template_revisions WHERE brand_id=$1)`, brandID).Scan(&templates, &revisions); err != nil || templates != wantTemplates || revisions != wantRevisions {
		t.Fatalf("brand %s templates/revisions=%d/%d want=%d/%d err=%v", brandID, templates, revisions, wantTemplates, wantRevisions, err)
	}
}

func drawNotificationUpgradeFingerprint(t *testing.T, db *pgxpool.Pool) map[string]string {
	t.Helper()
	names := []string{"brands", "point_accounts", "point_buckets", "point_ledger_entries", "audit_logs", "notification_templates", "notification_template_revisions", "outbox_events", "notification_deliveries", "notifications", "consumed_events", "roles", "role_permissions", "permissions"}
	result := make(map[string]string, len(names))
	for _, name := range names {
		result[name] = drawNotificationUpgradeFingerprintValue(t, db, name)
	}
	return result
}

func drawNotificationUpgradeFingerprintValue(t *testing.T, db *pgxpool.Pool, table string) string {
	t.Helper()
	filter := ""
	if table == "notification_templates" || table == "notification_template_revisions" {
		filter = " WHERE template_key NOT IN('draw.result.published','draw.result.corrected')"
	}
	query := fmt.Sprintf(`SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text),'[]'::jsonb)::text FROM %s r%s`, pgx.Identifier{table}.Sanitize(), filter)
	var value string
	if err := db.QueryRow(context.Background(), query).Scan(&value); err != nil {
		t.Fatalf("fingerprint %s: %v", table, err)
	}
	return value
}

func drawNotificationUpgradeDefaultsFingerprint(t *testing.T, db *pgxpool.Pool) string {
	t.Helper()
	var value string
	if err := db.QueryRow(context.Background(), `SELECT jsonb_build_object(
 'templates',(SELECT jsonb_agg(to_jsonb(t) ORDER BY brand_id,template_key) FROM notification_templates t WHERE template_key IN('draw.result.published','draw.result.corrected')),
 'revisions',(SELECT jsonb_agg(to_jsonb(r) ORDER BY brand_id,template_key,version) FROM notification_template_revisions r WHERE template_key IN('draw.result.published','draw.result.corrected')))::text`).Scan(&value); err != nil {
		t.Fatal("draw template fingerprint:", err)
	}
	return value
}
