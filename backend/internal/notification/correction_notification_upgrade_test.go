package notification

import (
	"context"
	"fmt"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCommissionCorrectionNotification0060UpgradePreserves0059History(t *testing.T) {
	db := notificationSchemaBefore(t, "0060_")
	ctx := context.Background()
	var schema, functionOIDBefore, oldDefaults string
	if err := db.QueryRow(ctx, `SELECT current_schema(),to_regprocedure('notification_template_defaults()')::oid::text,notification_template_defaults()::text`).
		Scan(&schema, &functionOIDBefore, &oldDefaults); err != nil {
		t.Fatal(err)
	}
	var oldBrands []string
	if err := db.QueryRow(ctx, `SELECT array_agg(id::text ORDER BY id) FROM brands`).Scan(&oldBrands); err != nil {
		t.Fatal(err)
	}
	if len(oldBrands) == 0 {
		t.Fatal("0059 fixture has no seeded brands")
	}
	for _, id := range oldBrands {
		rewardUpgradeCounts(t, db, id, 19, 19)
	}
	before := correctionNotificationUpgradeFingerprint(t, db)
	var oldOutbox, oldDeliveries, oldInbox int
	if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM outbox_events),(SELECT count(*) FROM notification_deliveries),(SELECT count(*) FROM notifications)`).
		Scan(&oldOutbox, &oldDeliveries, &oldInbox); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	after := correctionNotificationUpgradeFingerprint(t, db)
	for key, value := range before {
		if after[key] != value {
			t.Errorf("0060 changed prior %s", key)
		}
	}
	var defaultsAfter, functionOIDAfter string
	if err := db.QueryRow(ctx, `SELECT notification_template_defaults()::text,to_regprocedure('notification_template_defaults()')::oid::text`).
		Scan(&defaultsAfter, &functionOIDAfter); err != nil {
		t.Fatal(err)
	}
	if functionOIDAfter != functionOIDBefore {
		t.Fatalf("notification_template_defaults OID changed: before=%s after=%s", functionOIDBefore, functionOIDAfter)
	}
	if defaultsAfter != oldDefaults {
		var beforeWithoutNew, afterWithoutNew string
		if err := db.QueryRow(ctx, `SELECT (notification_template_defaults()-'commission.corrected'-'draw.result.published'-'draw.result.corrected')::text`).Scan(&afterWithoutNew); err != nil {
			t.Fatal(err)
		}
		beforeWithoutNew = oldDefaults
		if afterWithoutNew != beforeWithoutNew {
			t.Fatal("0060 changed existing notification defaults")
		}
	}
	for _, id := range oldBrands {
		rewardUpgradeCounts(t, db, id, 22, 22)
		var addedTemplates, addedRevisions int
		if err := db.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM notification_templates WHERE brand_id=$1 AND template_key='commission.corrected' AND version=1 AND content=notification_template_defaults()->'commission.corrected'),
 (SELECT count(*) FROM notification_template_revisions WHERE brand_id=$1 AND template_key='commission.corrected' AND version=1 AND content=notification_template_defaults()->'commission.corrected' AND changed_by IS NULL AND audit_log_id IS NULL AND reason='Initial in-app notification template')`, id).
			Scan(&addedTemplates, &addedRevisions); err != nil || addedTemplates != 1 || addedRevisions != 1 {
			t.Fatalf("brand %s correction default/revision=%d/%d err=%v", id, addedTemplates, addedRevisions, err)
		}
	}
	newBrand := ids.New()
	if _, err := db.Exec(ctx, `INSERT INTO brands(id,code,name,status) VALUES($1,$2,'After 0060','paused')`, newBrand, "after_0060_"+newBrand[:8]); err != nil {
		t.Fatal(err)
	}
	rewardUpgradeCounts(t, db, newBrand, 22, 22)
	var events, deliveries, inbox int
	if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM outbox_events),(SELECT count(*) FROM notification_deliveries),(SELECT count(*) FROM notifications)`).
		Scan(&events, &deliveries, &inbox); err != nil || events != oldOutbox || deliveries != oldDeliveries || inbox != oldInbox {
		t.Fatalf("0060 backfilled notifications: outbox/deliveries/inbox=%d/%d/%d before=%d/%d/%d err=%v", events, deliveries, inbox, oldOutbox, oldDeliveries, oldInbox, err)
	}
	correctionNotificationPinnedFunctions(t, db, schema)
	stable := correctionNotificationUpgradeFingerprint(t, db)
	newDefaultBefore := correctionNotificationUpgradeNewDefaultFingerprint(t, db)
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal("repeat migration", err)
	}
	repeated := correctionNotificationUpgradeFingerprint(t, db)
	newDefaultAfter := correctionNotificationUpgradeNewDefaultFingerprint(t, db)
	for key, value := range stable {
		if got := repeated[key]; got != value {
			t.Errorf("repeat migration changed %s", key)
		}
	}
	if newDefaultAfter != newDefaultBefore {
		t.Error("repeat migration changed the new correction template or revision")
	}
	var migrations int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM schema_migrations WHERE name='0060_commission_correction_notifications.up.sql'`).Scan(&migrations); err != nil || migrations != 1 {
		t.Fatalf("0060 migration rows=%d err=%v", migrations, err)
	}
}

func correctionNotificationUpgradeFingerprint(t *testing.T, db *pgxpool.Pool) map[string]string {
	t.Helper()
	names := []string{
		"brands", "point_accounts", "point_buckets", "point_ledger_entries", "audit_logs",
		"commission_correction_plans", "commission_correction_plan_steps", "commission_correction_plan_targets",
		"brand_commission_correction_policies", "commission_correction_policy_revisions", "commission_correction_executions",
		"commission_correction_execution_targets", "commission_correction_balance_heads", "commission_correction_cycle_holds", "commission_correction_execution_steps",
		"notification_templates", "notification_template_revisions", "outbox_events", "notification_deliveries", "notifications", "consumed_events",
	}
	result := make(map[string]string, len(names))
	for _, name := range names {
		result[name] = correctionNotificationUpgradeFingerprintValue(t, db, name)
	}
	return result
}

func correctionNotificationUpgradeFingerprintValue(t *testing.T, db *pgxpool.Pool, table string) string {
	t.Helper()
	filter := ""
	if table == "notification_templates" || table == "notification_template_revisions" {
		filter = " WHERE template_key NOT IN('commission.corrected','draw.result.published','draw.result.corrected')"
	}
	query := fmt.Sprintf(`SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text),'[]'::jsonb)::text FROM %s r%s`, pgx.Identifier{table}.Sanitize(), filter)
	var value string
	if err := db.QueryRow(context.Background(), query).Scan(&value); err != nil {
		t.Fatalf("fingerprint %s: %v", table, err)
	}
	return value
}

func correctionNotificationUpgradeNewDefaultFingerprint(t *testing.T, db *pgxpool.Pool) string {
	t.Helper()
	var value string
	if err := db.QueryRow(context.Background(), `SELECT jsonb_build_object(
 'templates',(SELECT jsonb_agg(to_jsonb(t) ORDER BY brand_id) FROM notification_templates t WHERE template_key='commission.corrected'),
 'revisions',(SELECT jsonb_agg(to_jsonb(r) ORDER BY brand_id,version) FROM notification_template_revisions r WHERE template_key='commission.corrected'))::text`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func correctionNotificationPinnedFunctions(t *testing.T, db *pgxpool.Pool, schema string) {
	t.Helper()
	wanted := []string{
		"notification_template_defaults", "enqueue_in_app_event", "valid_commission_correction_notification_event",
		"guard_commission_correction_notification_outbox", "emit_commission_correction_notification",
		"require_commission_correction_notification_commit",
	}
	rows, err := db.Query(context.Background(), `SELECT p.proname,p.proconfig FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=$1 AND p.proname=ANY($2::text[])`, schema, wanted)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var name string
		var config []string
		if err = rows.Scan(&name, &config); err != nil {
			t.Fatal(err)
		}
		pinned := false
		for _, value := range config {
			pinned = pinned || value == "search_path=pg_catalog, "+schema+", pg_temp"
		}
		if !pinned || seen[name] {
			t.Errorf("%s search_path=%v duplicate=%t", name, config, seen[name])
		}
		seen[name] = true
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(seen) != len(wanted) {
		t.Fatalf("0060 pinned functions=%d want=%d", len(seen), len(wanted))
	}
}
