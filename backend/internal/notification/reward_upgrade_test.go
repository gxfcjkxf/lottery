package notification

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rewards"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var rewardUpgradeKeys = []string{"reward.order.granted", "reward.order.revocation_pending", "reward.order.revoked"}
var latestAddedTemplateKeys = []string{"reward.order.granted", "reward.order.revocation_pending", "reward.order.revoked", "commission.corrected"}

type rewardUpgradeWitness struct {
	actionID string
	kind     string
	payload  []byte
}

func TestRewardNotificationUpgradePreserves0055HistoryAndRejectsTemporarySpoofs(t *testing.T) {
	ctx := context.Background()
	db := testdb.NewAtVersion(t, 55)
	var schema, lastMigration, oldDefaults, oldFunctionOID string
	var defaultCount int
	if err := db.QueryRow(ctx, `SELECT current_schema(),(SELECT max(name) FROM schema_migrations),
 notification_template_defaults()::text,to_regprocedure('notification_template_defaults()')::oid::text,
 (SELECT count(*) FROM jsonb_object_keys(notification_template_defaults()))`).
		Scan(&schema, &lastMigration, &oldDefaults, &oldFunctionOID, &defaultCount); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(lastMigration, "0055_") || defaultCount != 16 {
		t.Fatalf("fixture is not the actual 0055 schema: migration=%s defaults=%d", lastMigration, defaultCount)
	}
	app := pgx.Identifier{schema}.Sanitize()
	preBrand := ids.New()
	if _, err := db.Exec(ctx, `INSERT INTO brands(id,code,name,status) VALUES($1,$2,'Before reward notifications','paused')`, preBrand, "reward_pre_"+preBrand[24:]); err != nil {
		t.Fatal(err)
	}
	var oldBrands []string
	if err := db.QueryRow(ctx, `SELECT array_agg(id::text ORDER BY id) FROM brands`).Scan(&oldBrands); err != nil {
		t.Fatal(err)
	}
	for _, id := range oldBrands {
		rewardUpgradeCounts(t, db, id, 16, 16)
	}

	user, member, account, admin := ids.New(), ids.New(), ids.New(), ids.New()
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO global_users(id,username,password_hash) VALUES($1,$2,'test-only')`, []any{user, "reward_upgrade_member_" + user[24:]}},
		{`INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'operator','1','1')`, []any{member, brand, user}},
		{`INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only')`, []any{admin, "reward_upgrade_admin_" + admin[24:]}},
		{`INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, []any{account, brand, member}},
		{`INSERT INTO point_buckets(brand_id,account_id,source,state,points) SELECT $1,$2,s,t,0
 FROM unnest(ARRAY['recharge','winning','gift']) s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal']) t`, []any{brand, account}},
	} {
		if _, err := db.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	actor := access.Account{ID: admin, Type: access.AccountAdmin, BrandIDs: []string{brand}, Roles: []access.Role{{BrandID: brand, Permissions: []access.Permission{
		{Resource: "reward", Action: "view", Scope: access.ScopeBrand},
		{Resource: "reward", Action: "grant", Scope: access.ScopeBrand},
		{Resource: "reward", Action: "revoke", Scope: access.ScopeBrand},
		{Resource: "reward", Action: "retry", Scope: access.ScopeBrand},
		{Resource: "notification_template", Action: "write", Scope: access.ScopeBrand},
	}}}}
	meta := func() points.Metadata {
		return points.Metadata{ActorType: "admin", ActorID: admin, RequestID: ids.New(), IP: "127.0.0.1"}
	}
	rs, ns, ps := rewards.Service{DB: db}, Service{DB: db}, points.Store{DB: db}
	grant := func(amount points.Amount) rewards.Order {
		tx := rewardUpgradeTx(t, db)
		order, err := rs.GrantTx(ctx, tx, brand, actor, rewards.GrantInput{MemberID: member, Points: amount, Reason: "Explicit reward upgrade grant"}, meta())
		if err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return order
	}
	revoke := func(order rewards.Order, state string) rewards.Order {
		tx := rewardUpgradeTx(t, db)
		out, err := rs.RevokeTx(ctx, tx, brand, order.ID, actor, rewards.ActionInput{Version: order.Version, Reason: "Explicit reward upgrade revocation"}, meta())
		if err != nil || out.State != state || out.Version != 2 {
			t.Fatalf("revoke: order=%+v want=%s/2 err=%v", out, state, err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return out
	}
	granted := grant(20)
	revoked := revoke(grant(30), "revoked")
	pending := grant(40)
	allocation := []points.Allocation{{Source: "gift", State: "available", Points: 60}}
	delta, err := points.AllocationDelta(allocation, "available", "manual_frozen")
	if err != nil {
		t.Fatal(err)
	}
	tx := rewardUpgradeTx(t, db)
	if _, err = ps.Post(ctx, tx, points.Change{BrandID: brand, MemberID: member, EntryType: "freeze", ReferenceType: "manual",
		OperationKey: "reward-upgrade-freeze:" + ids.New(), Reason: "Hold available gift points before explicit revocation",
		ActorType: "admin", ActorID: admin, RequestID: ids.New(), Delta: delta, Allocation: allocation}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	pending = revoke(pending, "revocation_pending")
	oldOrderIDs := []string{granted.ID, revoked.ID, pending.ID}

	custom := Content{En: Copy{Title: "Existing custom welcome", Body: "Preserve this approved welcome."}, ZhCN: Copy{Title: "原有欢迎文案", Body: "保留已批准的欢迎文案。"}}
	tx = rewardUpgradeTx(t, db)
	if _, err = ns.UpdateTemplate(ctx, tx, brand, "member.joined", actor, UpdateInput{Version: 1, Content: custom, Reason: "Customize before reward notification upgrade"}, meta()); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	emit(t, ns, brand, member)
	if done, err := ns.Process(ctx, 20); err != nil || done != 1 {
		t.Fatalf("existing inbox delivery: done=%d err=%v", done, err)
	}
	emit(t, ns, brand, member) // Preserve an existing pending delivery as well.

	var witnesses []rewardUpgradeWitness
	rows, err := db.Query(ctx, `SELECT a.id::text,'reward.order.'||a.state_after,
 jsonb_build_object('member_id',o.member_id::text,'resource_id',o.id::text,'points',o.points::text,
 'action_id',a.id::text,'version',a.version,'audit_log_id',a.audit_log_id::text)
 FROM reward_order_actions a JOIN reward_orders o ON o.brand_id=a.brand_id AND o.id=a.order_id
 WHERE a.brand_id=$1 AND o.id=ANY($2::uuid[]) ORDER BY o.id,a.version`, brand, oldOrderIDs)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var w rewardUpgradeWitness
		if err = rows.Scan(&w.actionID, &w.kind, &w.payload); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		witnesses = append(witnesses, w)
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(witnesses) != 5 {
		t.Fatalf("0055 actual action witnesses=%d err=%v", len(witnesses), err)
	}
	rewardUpgradeNoBackfill(t, db, oldOrderIDs)
	before := rewardUpgradeFingerprint(t, db, false)
	if err = database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	rewardUpgradeSame(t, before, rewardUpgradeFingerprint(t, db, true), "0056 changed existing facts")
	var defaultsAfter, functionOIDAfter string
	if err = db.QueryRow(ctx, `SELECT (notification_template_defaults()-$1::text[]-'commission.corrected')::text,
 to_regprocedure('notification_template_defaults()')::oid::text,
 (SELECT count(*) FROM jsonb_object_keys(notification_template_defaults()))`, rewardUpgradeKeys).
		Scan(&defaultsAfter, &functionOIDAfter, &defaultCount); err != nil || defaultsAfter != oldDefaults || functionOIDAfter != oldFunctionOID || defaultCount != 20 {
		t.Fatalf("0056 changed prior defaults/OID or failed to add three: count=%d OID=%s/%s defaultsEqual=%t err=%v", defaultCount, oldFunctionOID, functionOIDAfter, defaultsAfter == oldDefaults, err)
	}
	for _, id := range oldBrands {
		oldRevisions := 16
		if id == brand {
			oldRevisions++
		}
		rewardUpgradeCounts(t, db, id, 20, oldRevisions+4)
		var templates, revisions int
		if err = db.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM notification_templates WHERE brand_id=$1 AND template_key=ANY($2::text[]) AND version=1 AND content=notification_template_defaults()->template_key),
 (SELECT count(*) FROM notification_template_revisions WHERE brand_id=$1 AND template_key=ANY($2::text[]) AND version=1 AND content=notification_template_defaults()->template_key
 AND changed_by IS NULL AND audit_log_id IS NULL AND reason='Initial in-app notification template')`, id, rewardUpgradeKeys).Scan(&templates, &revisions); err != nil || templates != 3 || revisions != 3 {
			t.Fatalf("brand %s did not gain exactly three initial defaults/revisions: %d/%d err=%v", id, templates, revisions, err)
		}
	}
	copy, err := ns.Template(ctx, brand, "member.joined")
	history, historyErr := ns.TemplateHistory(ctx, brand, "member.joined", 20, 0)
	if err != nil || historyErr != nil || copy.Version != 2 || copy.Content != custom || len(history) != 2 || history[0].Content != custom {
		t.Fatalf("custom copy/history changed: copy=%+v history=%+v err=%v/%v", copy, history, err, historyErr)
	}
	rewardUpgradeNoBackfill(t, db, oldOrderIDs)
	rewardUpgradePinnedFunctions(t, db, schema)
	after := rewardUpgradeFingerprint(t, db, false)
	if err = database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	rewardUpgradeSame(t, after, rewardUpgradeFingerprint(t, db, false), "repeat migration changed facts")

	newBrand := ids.New()
	if _, err = db.Exec(ctx, `INSERT INTO brands(id,code,name,status) VALUES($1,$2,'After reward notifications','paused')`, newBrand, "reward_new_"+newBrand[24:]); err != nil {
		t.Fatal(err)
	}
	rewardUpgradeCounts(t, db, newBrand, 20, 20)
	for _, w := range witnesses {
		tx = rewardUpgradeTx(t, db)
		var valid, oldAction bool
		if err = tx.QueryRow(ctx, "SELECT "+app+`.valid_reward_notification_event($1,$2,$3,$4),
 (SELECT creation_xid<>pg_current_xact_id() FROM `+app+`.reward_order_actions WHERE id=$3)`, brand, w.kind, w.actionID, w.payload).Scan(&valid, &oldAction); err != nil || !valid || !oldAction {
			t.Fatalf("real historic %s witness invalid: valid=%t oldAction=%t err=%v", w.kind, valid, oldAction, err)
		}
		input, _, err := parseRewardEvent(w.payload)
		if err != nil {
			t.Fatal(err)
		}
		gotMember, public, err := validateEvent(ctx, tx, brand, w.kind, w.actionID, w.payload)
		if err != nil || gotMember != member || public.ResourceID != input.ResourceID || public.Points == nil || *public.Points != input.Points {
			t.Fatalf("consumer rejected historic witness: %s member=%s payload=%+v err=%v", w.kind, gotMember, public, err)
		}
		rewardUpgradeReject(t, tx, "INSERT INTO "+app+`.outbox_events(id,brand_id,event_type,aggregate_id,payload) VALUES($1,$2,$3,$4,$5)`, "current business action", ids.New(), brand, w.kind, w.actionID, w.payload)
		if err = tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
	}
	rewardUpgradeNoBackfill(t, db, oldOrderIDs)

	newOrder := grant(10)
	var newEvent string
	var events, deliveries, actions int
	if err = db.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM outbox_events WHERE event_type=ANY($1::text[])),
 (SELECT count(*) FROM notification_deliveries d JOIN outbox_events e ON e.id=d.event_id WHERE e.event_type=ANY($1::text[]) AND d.status='pending'),
 (SELECT count(*) FROM reward_order_actions WHERE order_id=$2),
 (SELECT id::text FROM outbox_events WHERE event_type='reward.order.granted' AND payload->>'resource_id'=$2::uuid::text)`, rewardUpgradeKeys, newOrder.ID).
		Scan(&events, &deliveries, &actions, &newEvent); err != nil || events != 1 || deliveries != 1 || actions != 1 {
		t.Fatalf("new normal grant did not produce one action/event/delivery: %d/%d/%d err=%v", actions, events, deliveries, err)
	}
	fundsBeforeDelivery := rewardUpgradeFingerprint(t, db, false)
	if done, err := ns.Process(ctx, 20); err != nil || done != 2 {
		t.Fatalf("post-upgrade delivery: done=%d err=%v", done, err)
	}
	fundsAfterDelivery := rewardUpgradeFingerprint(t, db, false)
	for _, table := range []string{"brands", "global_users", "brand_members", "admin_accounts", "point_accounts", "point_buckets", "point_ledger_entries", "audit_logs", "reward_orders", "reward_order_actions", "notification_templates", "notification_template_revisions", "outbox_events"} {
		if fundsAfterDelivery[table] != fundsBeforeDelivery[table] {
			t.Errorf("notification delivery changed %s", table)
		}
	}
	var inbox, sent int
	if err = db.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM notifications WHERE event_type=ANY($1::text[]) AND event_id=$2 AND payload=jsonb_build_object('resource_id',$3::uuid::text,'points','10') AND content IS NOT NULL),
 (SELECT count(*) FROM notification_deliveries WHERE event_id=$2 AND status='sent' AND attempt_count=1)`, rewardUpgradeKeys, newEvent, newOrder.ID).Scan(&inbox, &sent); err != nil || inbox != 1 || sent != 1 {
		t.Fatalf("new grant notification history=%d sent=%d err=%v", inbox, sent, err)
	}
	rewardUpgradeNoBackfill(t, db, oldOrderIDs)

	t.Run("temporary_witnesses_and_functions_cannot_authorize_real_mutations", func(t *testing.T) {
		before := rewardUpgradeFingerprint(t, db, false)
		rewardUpgradeTemporarySpoofs(t, db, app, granted.ID, member, newEvent)
		rewardUpgradeSame(t, before, rewardUpgradeFingerprint(t, db, false), "temporary spoof probes changed real facts")
	})

	// An operator may explicitly resume a genuine 0055 pending order after
	// upgrade. Only this NEW action gets an event; its old grant/pending actions
	// must still not be backfilled or inferred from the newly released funds.
	var originalHold string
	if err = db.QueryRow(ctx, `SELECT id::text FROM point_ledger_entries WHERE brand_id=$1 AND operation_key LIKE 'reward-upgrade-freeze:%'`, brand).Scan(&originalHold); err != nil {
		t.Fatal(err)
	}
	tx = rewardUpgradeTx(t, db)
	if _, err = ps.Reverse(ctx, tx, brand, member, originalHold, "reward-upgrade-unfreeze:"+ids.New(), "Explicit release of original old hold", meta()); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	rewardUpgradeNoBackfill(t, db, oldOrderIDs)
	tx = rewardUpgradeTx(t, db)
	resumed, err := rs.RetryRevocationTx(ctx, tx, brand, pending.ID, actor, rewards.ActionInput{Version: pending.Version, Reason: "Explicit operator resumes genuine old pending reward"}, meta())
	if err != nil || resumed.State != "revoked" || resumed.Version != 3 {
		t.Fatalf("old pending reward resume=%+v err=%v", resumed, err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if done, err := ns.Process(ctx, 20); err != nil || done != 1 {
		t.Fatalf("old reward new action delivery=%d err=%v", done, err)
	}
	var oldGrantOrPending, newReversal int
	if err = db.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM notifications WHERE event_type IN('reward.order.granted','reward.order.revocation_pending') AND payload->>'resource_id'=$1::uuid::text),
 (SELECT count(*) FROM notifications WHERE event_type='reward.order.revoked' AND payload=jsonb_build_object('resource_id',$1::uuid::text,'points','40'))`, pending.ID).Scan(&oldGrantOrPending, &newReversal); err != nil || oldGrantOrPending != 0 || newReversal != 1 {
		t.Fatalf("old actions backfilled=%d new explicit reversal=%d err=%v", oldGrantOrPending, newReversal, err)
	}
}

func rewardUpgradeTx(t *testing.T, db *pgxpool.Pool) pgx.Tx {
	t.Helper()
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}

// Compare all rows, including timestamps, IDs and financial/audit witnesses.
// The pre-upgrade snapshot is unfiltered; only the three new template keys are
// excluded from the first post-upgrade comparison.
func rewardUpgradeFingerprint(t *testing.T, q rowQuery, excludeNewDefaults bool) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, table := range []struct{ name, order string }{
		{"brands", "id"}, {"global_users", "id"}, {"brand_members", "id"}, {"admin_accounts", "id"},
		{"point_accounts", "id"}, {"point_buckets", "brand_id,account_id,source,state"}, {"point_ledger_entries", "id"},
		{"audit_logs", "id"}, {"reward_orders", "id"}, {"reward_order_actions", "id"}, {"outbox_events", "id"},
		{"notification_deliveries", "event_id"}, {"notifications", "id"}, {"consumed_events", "consumer,event_id"},
		{"notification_templates", "brand_id,template_key"}, {"notification_template_revisions", "brand_id,template_key,version"},
	} {
		filter := ""
		var args []any
		if excludeNewDefaults && (table.name == "notification_templates" || table.name == "notification_template_revisions") {
			filter, args = " WHERE template_key<>ALL($1::text[])", []any{latestAddedTemplateKeys}
		}
		query := fmt.Sprintf(`SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY %s),'[]'::jsonb)::text FROM %s r%s`, table.order, pgx.Identifier{table.name}.Sanitize(), filter)
		var fingerprint string
		if err := q.QueryRow(context.Background(), query, args...).Scan(&fingerprint); err != nil {
			t.Fatalf("fingerprint %s: %v", table.name, err)
		}
		out[table.name] = fingerprint
	}
	return out
}

func rewardUpgradeSame(t *testing.T, before, after map[string]string, message string) {
	t.Helper()
	if !reflect.DeepEqual(before, after) {
		for name, expected := range before {
			if after[name] != expected {
				t.Errorf("%s: %s", message, name)
			}
		}
		t.FailNow()
	}
}

func rewardUpgradeCounts(t *testing.T, q rowQuery, brandID string, wantTemplates, wantRevisions int) {
	t.Helper()
	var templates, revisions int
	if err := q.QueryRow(context.Background(), `SELECT
 (SELECT count(*) FROM notification_templates WHERE brand_id=$1),
 (SELECT count(*) FROM notification_template_revisions WHERE brand_id=$1)`, brandID).Scan(&templates, &revisions); err != nil || templates != wantTemplates || revisions != wantRevisions {
		t.Fatalf("brand %s templates/revisions=%d/%d want=%d/%d err=%v", brandID, templates, revisions, wantTemplates, wantRevisions, err)
	}
}

func rewardUpgradeNoBackfill(t *testing.T, q rowQuery, oldOrders []string) {
	t.Helper()
	var events, deliveries, inbox int
	if err := q.QueryRow(context.Background(), `SELECT
 (SELECT count(*) FROM outbox_events e JOIN reward_order_actions a ON a.id=e.aggregate_id AND a.brand_id=e.brand_id WHERE a.order_id=ANY($1::uuid[]) AND e.event_type=ANY($2::text[])),
 (SELECT count(*) FROM notification_deliveries d JOIN outbox_events e ON e.id=d.event_id JOIN reward_order_actions a ON a.id=e.aggregate_id AND a.brand_id=e.brand_id WHERE a.order_id=ANY($1::uuid[]) AND e.event_type=ANY($2::text[])),
 (SELECT count(*) FROM notifications WHERE event_type=ANY($2::text[]) AND payload->>'resource_id'=ANY($1::uuid[]::text[]))`, oldOrders, rewardUpgradeKeys).Scan(&events, &deliveries, &inbox); err != nil || events != 0 || deliveries != 0 || inbox != 0 {
		t.Fatalf("old reward history was backfilled: events/deliveries/inbox=%d/%d/%d err=%v", events, deliveries, inbox, err)
	}
}

func rewardUpgradePinnedFunctions(t *testing.T, db *pgxpool.Pool, schema string) {
	t.Helper()
	wanted := []string{"notification_template_defaults", "enqueue_in_app_event", "valid_reward_notification_event", "guard_reward_notification_outbox", "emit_reward_notification", "require_reward_notification_commit", "valid_commission_correction_notification_event", "guard_commission_correction_notification_outbox", "emit_commission_correction_notification", "require_commission_correction_notification_commit"}
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
			t.Errorf("%s does not have exactly one schema-pinned function: config=%v duplicate=%t", name, config, seen[name])
		}
		seen[name] = true
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(seen) != len(wanted) {
		t.Fatalf("0056 pinned functions=%d want=%d", len(seen), len(wanted))
	}
}

func rewardUpgradeReject(t *testing.T, tx pgx.Tx, query, message string, args ...any) {
	t.Helper()
	ctx := context.Background()
	if _, err := tx.Exec(ctx, `SAVEPOINT reward_upgrade_reject`); err != nil {
		t.Fatal(err)
	}
	_, err := tx.Exec(ctx, query, args...)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "P0001" || !strings.Contains(pgErr.Message, message) {
		t.Fatalf("expected business guard rejection %q, got %v", message, err)
	}
	if _, err = tx.Exec(ctx, `ROLLBACK TO SAVEPOINT reward_upgrade_reject; RELEASE SAVEPOINT reward_upgrade_reject`); err != nil {
		t.Fatal(err)
	}
}

func rewardUpgradeTemporarySpoofs(t *testing.T, db *pgxpool.Pool, app, grantedID, member, newEvent string) {
	t.Helper()
	ctx := context.Background()
	tx := rewardUpgradeTx(t, db)
	for _, table := range []struct{ name, predicate string }{
		{"reward_orders", "id=$1"}, {"reward_order_actions", "order_id=$1 AND version=1"},
		{"audit_logs", "id=(SELECT audit_log_id FROM " + app + ".reward_order_actions WHERE order_id=$1 AND version=1)"},
		{"point_ledger_entries", "id=(SELECT grant_ledger_entry_id FROM " + app + ".reward_orders WHERE id=$1)"},
	} {
		if _, err := tx.Exec(ctx, "CREATE TEMP TABLE "+pgx.Identifier{table.name}.Sanitize()+" AS SELECT * FROM "+app+"."+pgx.Identifier{table.name}.Sanitize()+" WHERE "+table.predicate, grantedID); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"outbox_events", "notification_deliveries"} {
		if _, err := tx.Exec(ctx, "CREATE TEMP TABLE "+pgx.Identifier{name}.Sanitize()+" AS SELECT * FROM "+app+"."+pgx.Identifier{name}.Sanitize()+" WITH NO DATA"); err != nil {
			t.Fatal(err)
		}
	}
	fakeOrder, fakeAction, fakeAudit, fakeLedger := ids.New(), ids.New(), ids.New(), ids.New()
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`UPDATE pg_temp.reward_orders SET id=$1,grant_ledger_entry_id=$2,creation_audit_log_id=$3,last_audit_log_id=$3,creation_xid=pg_current_xact_id()`, []any{fakeOrder, fakeLedger, fakeAudit}},
		{`UPDATE pg_temp.reward_order_actions SET id=$1,order_id=$2,audit_log_id=$3,ledger_entry_id=$4,creation_xid=pg_current_xact_id()`, []any{fakeAction, fakeOrder, fakeAudit, fakeLedger}},
		{`UPDATE pg_temp.audit_logs SET id=$1,resource_id=$2,after_json=jsonb_set(after_json,'{action_id}',to_jsonb($3::text))`, []any{fakeAudit, fakeOrder, fakeAction}},
		{`UPDATE pg_temp.point_ledger_entries SET id=$1,reference_id=$2,operation_key='reward-grant:'||$2::uuid::text`, []any{fakeLedger, fakeOrder}},
		{`INSERT INTO pg_temp.reward_order_actions SELECT * FROM ` + app + `.reward_order_actions WHERE order_id=$1 AND version=1`, []any{grantedID}},
		{`UPDATE pg_temp.reward_order_actions SET creation_xid=pg_current_xact_id()`, nil},
	} {
		if _, err := tx.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	var fakePayload, oldPayload []byte
	var oldAction string
	if err := tx.QueryRow(ctx, `SELECT jsonb_build_object('member_id',$1::text,'resource_id',$2::text,'points','20','action_id',$3::text,'version',1,'audit_log_id',$4::text)`, member, fakeOrder, fakeAction, fakeAudit).Scan(&fakePayload); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT a.id::text,jsonb_build_object('member_id',o.member_id::text,'resource_id',o.id::text,'points',o.points::text,'action_id',a.id::text,'version',a.version,'audit_log_id',a.audit_log_id::text)
 FROM `+app+`.reward_order_actions a JOIN `+app+`.reward_orders o ON o.brand_id=a.brand_id AND o.id=a.order_id WHERE o.id=$1 AND a.version=1`, grantedID).Scan(&oldAction, &oldPayload); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO pg_temp.outbox_events(id,brand_id,event_type,aggregate_id,payload) VALUES($1,$2,'reward.order.granted',$3,$4)`, ids.New(), brand, fakeAction, fakePayload); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `CREATE FUNCTION pg_temp.valid_reward_notification_event(uuid,text,uuid,jsonb) RETURNS boolean LANGUAGE sql AS $$ SELECT true $$;
 CREATE FUNCTION pg_temp.point_zero_snapshot() RETURNS jsonb LANGUAGE sql AS $$ SELECT '{}'::jsonb $$;
 CREATE FUNCTION pg_temp.notification_template_defaults() RETURNS jsonb LANGUAGE sql AS $$ SELECT '{"spoof":true}'::jsonb $$;
 SET LOCAL search_path TO pg_temp, `+app+`, pg_catalog`); err != nil {
		t.Fatal(err)
	}
	var fakeValid, oldValid, spoofValid bool
	var defaults int
	if err := tx.QueryRow(ctx, `SELECT `+app+`.valid_reward_notification_event($1,'reward.order.granted',$2,$3),
 `+app+`.valid_reward_notification_event($1,'reward.order.granted',$4,$5),
 pg_temp.valid_reward_notification_event($1,'reward.order.granted',$2,$3),
		(SELECT count(*) FROM jsonb_object_keys(`+app+`.notification_template_defaults()))`, brand, fakeAction, fakePayload, oldAction, oldPayload).Scan(&fakeValid, &oldValid, &spoofValid, &defaults); err != nil || fakeValid || !oldValid || !spoofValid || defaults != 20 {
		t.Fatalf("temp chain/function displaced real evidence: fake=%t old=%t spoofControl=%t defaults=%d err=%v", fakeValid, oldValid, spoofValid, defaults, err)
	}
	insert := "INSERT INTO " + app + `.outbox_events(id,brand_id,event_type,aggregate_id,payload) VALUES($1,$2,'reward.order.granted',$3,$4)`
	rewardUpgradeReject(t, tx, insert, "current business action", ids.New(), brand, fakeAction, fakePayload)
	rewardUpgradeReject(t, tx, insert, "current business action", ids.New(), brand, oldAction, oldPayload)
	for _, query := range []string{
		"UPDATE " + app + `.outbox_events SET payload=payload||'{"points":"999"}'::jsonb WHERE id=$1`,
		"UPDATE " + app + `.outbox_events SET event_type='member.joined' WHERE id=$1`,
		"DELETE FROM " + app + `.outbox_events WHERE id=$1`,
	} {
		rewardUpgradeReject(t, tx, query, "reward notification event immutable", newEvent)
	}
	// The permitted publisher mutation still validates against the real grant,
	// despite a bogus temp zero-snapshot helper and a positive temp validator.
	if _, err := tx.Exec(ctx, "UPDATE "+app+`.outbox_events SET published_at=clock_timestamp() WHERE id=$1`, newEvent); err != nil {
		t.Fatalf("real publisher update rejected under temp spoofing: %v", err)
	}
	// Invoke the actual emitter and deferred commit function from temporary
	// trigger sites. A counterfeit temp action/event must not authorize either.
	if _, err := tx.Exec(ctx, `UPDATE pg_temp.reward_orders SET grant_ledger_entry_id=NULL;
 CREATE TRIGGER reward_upgrade_emit AFTER UPDATE ON pg_temp.reward_orders FOR EACH ROW EXECUTE FUNCTION `+app+`.emit_reward_notification();
 CREATE CONSTRAINT TRIGGER reward_upgrade_require AFTER INSERT ON pg_temp.reward_order_actions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION `+app+`.require_reward_notification_commit()`); err != nil {
		t.Fatal(err)
	}
	rewardUpgradeReject(t, tx, `UPDATE pg_temp.reward_orders SET grant_ledger_entry_id=$1 WHERE id=$2`, "reward notification requires matching action", fakeLedger, fakeOrder)
	rewardUpgradeReject(t, tx, `INSERT INTO pg_temp.reward_order_actions SELECT * FROM pg_temp.reward_order_actions WHERE order_id=(SELECT id FROM pg_temp.reward_orders);
 SET CONSTRAINTS reward_upgrade_require IMMEDIATE`, "new reward action requires matching immutable notification event")
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
}
