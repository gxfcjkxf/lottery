package notification

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/events"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/gxfcjkxf/lottery/backend/migrations"
	"sync"
	"testing"
)

const brand = "0199a000-0000-7000-8000-000000000001"
const other = "0199a000-0000-7000-8000-000000000002"

func fixture(t *testing.T) (Service, string, string) {
	t.Helper()
	db := testdb.New(t)
	ctx := context.Background()
	user, m1, m2 := ids.New(), ids.New(), ids.New()
	if _, e := db.Exec(ctx, `INSERT INTO global_users(id,username) VALUES($1,$2)`, user, "notify_"+user); e != nil {
		t.Fatal(e)
	}
	for i, m := range []string{m1, m2} {
		b := brand
		if i == 1 {
			b = other
		}
		if _, e := db.Exec(ctx, `INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'domain','dev-1','dev-1')`, m, b, user); e != nil {
			t.Fatal(e)
		}
	}
	return Service{DB: db}, m1, m2
}
func emit(t *testing.T, s Service, b, m string) string {
	t.Helper()
	ctx := context.Background()
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if e = events.Append(ctx, tx, b, "member.joined", m, m, nil); e != nil {
		t.Fatal(e)
	}
	var id string
	if e = tx.QueryRow(ctx, `SELECT id::text FROM outbox_events WHERE brand_id=$1 ORDER BY created_at DESC,id DESC LIMIT 1`, b).Scan(&id); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	return id
}
func TestDeliveryConcurrentDedupIsolationAndImmutableContent(t *testing.T) {
	s, m1, m2 := fixture(t)
	ctx := context.Background()
	e1 := emit(t, s, brand, m1)
	emit(t, s, other, m2)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := s.Process(ctx, 20); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	p, e := s.List(ctx, brand, m1, 20, 0)
	if e != nil || len(p.Items) != 1 || p.UnreadCount != "1" || p.Items[0].TemplateKey != "member.joined" {
		t.Fatal(p, e)
	}
	if _, e = s.Process(ctx, 20); e != nil {
		t.Fatal(e)
	}
	var n int
	if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM consumed_events WHERE consumer=$1`, Consumer).Scan(&n); e != nil || n != 2 {
		t.Fatal(n, e)
	}
	var published bool
	s.DB.QueryRow(ctx, `SELECT published_at IS NOT NULL FROM outbox_events WHERE id=$1`, e1).Scan(&published)
	if published {
		t.Fatal("stole broker publisher state")
	}
	if _, e = s.DB.Exec(ctx, `UPDATE notifications SET payload='{}'::jsonb WHERE id=$1`, p.Items[0].ID); e == nil {
		t.Fatal("mutable content")
	}
	if _, e = s.DB.Exec(ctx, `DELETE FROM notifications WHERE id=$1`, p.Items[0].ID); e == nil {
		t.Fatal("deleted evidence")
	}
	foreign, e := s.List(ctx, other, m2, 20, 0)
	if e != nil || len(foreign.Items) != 1 {
		t.Fatal(foreign, e)
	}
	tx, _ := s.DB.Begin(ctx)
	_, e = s.MarkRead(ctx, tx, brand, m1, []string{p.Items[0].ID, foreign.Items[0].ID})
	tx.Rollback(ctx)
	if e != ErrNotFound {
		t.Fatal("partial cross-scope mark accepted", e)
	}
	// New arrivals are not in the captured explicit-ID request.
	emit(t, s, brand, m1)
	s.Process(ctx, 20)
	tx, _ = s.DB.Begin(ctx)
	r, e := s.MarkRead(ctx, tx, brand, m1, []string{p.Items[0].ID})
	if e != nil {
		tx.Rollback(ctx)
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if r.Changed != 1 || r.UnreadCount != "1" {
		t.Fatal(r)
	}
	tx, _ = s.DB.Begin(ctx)
	r, e = s.MarkRead(ctx, tx, brand, m1, []string{p.Items[0].ID})
	if e != nil {
		t.Fatal(e)
	}
	tx.Commit(ctx)
	if r.Changed != 0 {
		t.Fatal(r)
	}
}
func TestPoisonDeliveryDoesNotBlockAndRetryKeepsEvidence(t *testing.T) {
	s, m, _ := fixture(t)
	ctx := context.Background()
	bad := ids.New()
	raw, _ := json.Marshal(map[string]any{"member_id": m, "resource_id": m, "points": "unexpected"})
	if _, e := s.DB.Exec(ctx, `INSERT INTO outbox_events(id,brand_id,event_type,aggregate_id,payload) VALUES($1,$2,'member.joined',$3,$4)`, bad, brand, m, raw); e != nil {
		t.Fatal(e)
	}
	emit(t, s, brand, m)
	if _, e := s.Process(ctx, 20); e != nil {
		t.Fatal(e)
	}
	var state, last string
	var attempts int
	if e := s.DB.QueryRow(ctx, `SELECT status,attempt_count,last_error FROM notification_deliveries WHERE event_id=$1`, bad).Scan(&state, &attempts, &last); e != nil || state != "failed" || attempts != 1 || last != "INVALID_EVENT" {
		t.Fatal(state, attempts, last, e)
	}
	p, e := s.List(ctx, brand, m, 20, 0)
	if e != nil || len(p.Items) != 1 {
		t.Fatal(p, e)
	}
	var c int
	s.DB.QueryRow(ctx, `SELECT count(*) FROM consumed_events WHERE event_id=$1`, bad).Scan(&c)
	if c != 0 {
		t.Fatal("poison acknowledged")
	}
}
func TestBusinessRollbackDoesNotNotify(t *testing.T) {
	s, m, _ := fixture(t)
	ctx := context.Background()
	tx, _ := s.DB.Begin(ctx)
	if e := events.Append(ctx, tx, brand, "member.joined", m, m, nil); e != nil {
		t.Fatal(e)
	}
	tx.Rollback(ctx)
	s.Process(ctx, 20)
	p, e := s.List(ctx, brand, m, 20, 0)
	if e != nil || len(p.Items) != 0 || p.UnreadCount != "0" {
		t.Fatal(p, e)
	}
}

func TestAcknowledgementFailureRollsBackInboxThenRecoversWithAuditedRetry(t *testing.T) {
	s, m, _ := fixture(t)
	ctx := context.Background()
	event := emit(t, s, brand, m)
	if _, e := s.DB.Exec(ctx, `CREATE FUNCTION reject_inbox_ack() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test ack failure'; END $$;CREATE TRIGGER reject_inbox_ack BEFORE INSERT ON consumed_events FOR EACH ROW EXECUTE FUNCTION reject_inbox_ack()`); e != nil {
		t.Fatal(e)
	}
	for attempt := 1; attempt <= 5; attempt++ {
		if _, e := s.Process(ctx, 20); e != nil {
			t.Fatal(e)
		}
		var n int
		var state string
		var count int
		if e := s.DB.QueryRow(ctx, `SELECT status,attempt_count,(SELECT count(*) FROM notifications) FROM notification_deliveries WHERE event_id=$1`, event).Scan(&state, &count, &n); e != nil || count != attempt || n != 0 {
			t.Fatal(state, count, n, e)
		}
		if attempt < 5 && state != "pending" || attempt == 5 && state != "failed" {
			t.Fatal(state)
		}
		// Only the isolated test clock is advanced; production backoff is unchanged.
		if _, e := s.DB.Exec(ctx, `UPDATE notification_deliveries SET next_attempt_at=clock_timestamp()-interval '1 second' WHERE event_id=$1`, event); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := s.DB.Exec(ctx, `DROP TRIGGER reject_inbox_ack ON consumed_events`); e != nil {
		t.Fatal(e)
	}
	admin := ids.New()
	if _, e := s.DB.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only')`, admin, "notify_retry_"+admin); e != nil {
		t.Fatal(e)
	}
	a := access.Account{ID: admin, Type: access.AccountAdmin, BrandIDs: []string{brand}, Roles: []access.Role{{BrandID: brand, Permissions: []access.Permission{{Resource: "notification", Action: "retry", Scope: access.ScopeBrand}}}}}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Retry(ctx, tx, brand, a, event, 5, "recovered storage dependency", points.Metadata{RequestID: ids.New()})
	if e != nil {
		tx.Rollback(ctx)
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Process(ctx, 20); e != nil {
		t.Fatal(e)
	}
	page, e := s.List(ctx, brand, m, 20, 0)
	if e != nil || len(page.Items) != 1 {
		t.Fatal(page, e)
	}
	var state string
	var count int
	var last *string
	if e = s.DB.QueryRow(ctx, `SELECT status,attempt_count,last_error FROM notification_deliveries WHERE event_id=$1`, event).Scan(&state, &count, &last); e != nil || state != "sent" || count != 6 || last != nil {
		t.Fatal(state, count, last, e)
	}
}

func TestQueueMigrationRecoversOnlyExistingFactsAndEnqueuesAfterCommit(t *testing.T) {
	s, m, _ := fixture(t)
	ctx := context.Background()
	// Simulate the immediately preceding schema in this disposable test schema.
	if _, e := s.DB.Exec(ctx, `DROP TRIGGER enqueue_in_app_event ON outbox_events;DROP FUNCTION enqueue_in_app_event()`); e != nil {
		t.Fatal(e)
	}
	event := emit(t, s, brand, m)
	if _, e := s.DB.Exec(ctx, `UPDATE outbox_events SET published_at=clock_timestamp() WHERE id=$1`, event); e != nil {
		t.Fatal(e)
	}
	unknown := ids.New()
	if _, e := s.DB.Exec(ctx, `INSERT INTO outbox_events(id,brand_id,event_type,aggregate_id,payload) VALUES($1,$2,'future.prize.requested',$3,'{}')`, unknown, brand, m); e != nil {
		t.Fatal(e)
	}
	sql, e := migrations.Files.ReadFile("0019_notification_outbox_queue.up.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(ctx, string(sql)); e != nil {
		t.Fatal(e)
	}
	emit(t, s, brand, m)
	var n int
	if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM notification_deliveries`).Scan(&n); e != nil || n != 2 {
		t.Fatal(n, e)
	}
	if _, e = s.Process(ctx, 20); e != nil {
		t.Fatal(e)
	}
	page, e := s.List(ctx, brand, m, 20, 0)
	if e != nil || len(page.Items) != 2 {
		t.Fatal(page, e)
	}
	var published bool
	if e = s.DB.QueryRow(ctx, `SELECT published_at IS NOT NULL FROM outbox_events WHERE id=$1`, event).Scan(&published); e != nil || !published {
		t.Fatal(published, e)
	}
}

func TestBootstrapPermissionUpgradeLeavesCustomRolesAndSuperWritesUnchanged(t *testing.T) {
	s, _, _ := fixture(t)
	ctx := context.Background()
	admin, br, custom, platform := ids.New(), ids.New(), ids.New(), ids.New()
	if _, e := s.DB.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash,is_super_admin) VALUES($1,$2,'test-only',true)`, admin, "notification_super_"+admin); e != nil {
		t.Fatal(e)
	}
	for i, r := range []string{br, custom, platform} {
		var b *string
		if i != 2 {
			v := brand
			b = &v
		}
		if _, e := s.DB.Exec(ctx, `INSERT INTO roles(id,brand_id,code,name,is_bootstrap) VALUES($1,$2,$3,'Test notification role',$4)`, r, b, "notify_role_"+r, i != 1); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := s.DB.Exec(ctx, `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, admin, platform); e != nil {
		t.Fatal(e)
	}
	sql, e := migrations.Files.ReadFile("0020_notification_bootstrap_permissions.up.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(ctx, string(sql)); e != nil {
		t.Fatal(e)
	}
	for _, v := range []struct {
		role  string
		count int
	}{{br, 2}, {custom, 0}, {platform, 1}} {
		var n int
		if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM role_permissions WHERE role_id=$1 AND permission_key LIKE 'notification.%'`, v.role).Scan(&n); e != nil || n != v.count {
			t.Fatal(v, n, e)
		}
	}
	var forbidden int
	if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM role_permissions WHERE role_id=$1 AND permission_key='notification.retry.brand'`, platform).Scan(&forbidden); e != nil || forbidden != 0 {
		t.Fatal(forbidden, e)
	}
}
