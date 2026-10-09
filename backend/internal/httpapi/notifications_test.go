package httpapi

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/notification"
	"reflect"
	"strings"
	"testing"
)

func TestNotificationHTTPRealProducersScopeCheckedReplayAndNoMoneyWrites(t *testing.T) {
	f := pointsFixture(t)
	ctx := context.Background()
	s := notification.Service{DB: f.pool}
	mustStatus(t, f.call("GET", "/api/v1/notifications?limit=20", "", "", managedBrand, nil), 401)
	mustStatus(t, f.call("GET", "/api/v1/b/harbor/notifications?limit=20", "", f.userToken, managedBrand, nil), 401)
	mustStatus(t, f.call("GET", "/api/v1/notifications?limit=101", "", f.userToken, managedBrand, nil), 400)
	pending := pointRecharge(t, f, "125", "test confidential proof", "notification-recharge-create")
	if _, e := s.Process(ctx, 20); e != nil {
		t.Fatal(e)
	}
	r := f.call("GET", "/api/v1/notifications?limit=20", "", f.userToken, managedBrand, nil)
	mustStatus(t, r, 200)
	var page notification.Page
	managedData(t, r, &page)
	if len(page.Items) != 1 || page.Items[0].EventType != "member.joined" {
		t.Fatal(page)
	}
	path := "/api/v1/admin/recharges/" + pending.ID + "/confirm"
	body := map[string]any{"version": 1, "reason": "internal bank verification secret"}
	mustStatus(t, f.call("POST", path, "notification-recharge-confirm", f.token, managedBrand, body), 200)
	mustStatus(t, f.call("POST", path, "notification-recharge-confirm", f.token, managedBrand, body), 200)
	if _, e := s.Process(ctx, 20); e != nil {
		t.Fatal(e)
	}
	before := pointWallet(t, f)
	r = f.call("GET", "/api/v1/notifications?limit=20", "", f.userToken, managedBrand, nil)
	mustStatus(t, r, 200)
	managedData(t, r, &page)
	if len(page.Items) != 2 || page.UnreadCount != "2" || page.Items[0].EventType != "recharge.confirmed" || *page.Items[0].Payload.Points != "125" {
		t.Fatal(page)
	}
	for _, secret := range []string{"internal bank", "manual-proof", "confirmed_by", "remark", "password", "access_token"} {
		if strings.Contains(r.Body.String(), secret) {
			t.Fatal("private operator data leaked", secret)
		}
	}
	// A same-brand different user cannot mark someone else's notification.
	input := map[string]string{"username": "other_notify", "password": "notification-password-123", "privacy_policy_version": "dev-1", "service_terms_version": "dev-1"}
	registered := f.call("POST", "/api/v1/auth/register", "notification-user-register", "", managedBrand, input)
	mustStatus(t, registered, 201)
	var auth identity.Authentication
	managedData(t, registered, &auth)
	s.Process(ctx, 20)
	readBody := map[string]any{"ids": []string{page.Items[0].ID, page.Items[1].ID}}
	mustStatus(t, f.call("POST", "/api/v1/notifications/read", "notification-foreign-read", auth.AccessToken, managedBrand, readBody), 404)
	mustStatus(t, pointsCall(f.managementHTTP, "POST", "/api/v1/notifications/read", "notification-csrf-read", f.userToken, managedBrand, "http://evil.example", readBody), 403)
	mustStatus(t, f.call("POST", "/api/v1/notifications/read", "notification-missing-id", f.userToken, managedBrand, map[string]any{"ids": []string{page.Items[0].ID, ids.New()}}), 404)
	mustStatus(t, f.call("POST", "/api/v1/notifications/read", "notification-duplicate-id", f.userToken, managedBrand, map[string]any{"ids": []string{page.Items[0].ID, page.Items[0].ID}}), 400)
	r = f.call("POST", "/api/v1/notifications/read", "notification-page-read", f.userToken, managedBrand, readBody)
	mustStatus(t, r, 200)
	var receipt notification.ReadReceipt
	managedData(t, r, &receipt)
	if receipt.Changed != 2 || receipt.UnreadCount != "0" {
		t.Fatal(receipt)
	}
	replay := f.call("POST", "/api/v1/notifications/read", "notification-page-read", f.userToken, managedBrand, readBody)
	mustStatus(t, replay, 200)
	var again notification.ReadReceipt
	managedData(t, replay, &again)
	if !reflect.DeepEqual(receipt, again) {
		t.Fatal(receipt, again)
	}
	mustStatus(t, f.call("POST", "/api/v1/notifications/read", "notification-page-read", f.userToken, managedBrand, map[string]any{"ids": []string{page.Items[0].ID}}), 409)
	after := pointWallet(t, f)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("inbox changed wallet")
	}
	// Replay must revalidate the session, not serve an old cached success.
	mustStatus(t, f.call("POST", "/api/v1/auth/logout", "notification-logout", f.userToken, managedBrand, map[string]any{}), 200)
	mustStatus(t, f.call("POST", "/api/v1/notifications/read", "notification-page-read", f.userToken, managedBrand, readBody), 401)
	var count int
	if e := f.pool.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries`).Scan(&count); e != nil || count != 1 {
		t.Fatal(count, e)
	}
}
func TestNotificationAdminFailedRetryAuditFreshPermissionsAndSuperReadOnly(t *testing.T) {
	f := pointsFixture(t)
	ctx := context.Background()
	s := notification.Service{DB: f.pool}
	event := ids.New()
	raw, _ := json.Marshal(map[string]any{"member_id": f.memberID, "resource_id": f.memberID, "points": "not-valid"})
	if _, e := f.pool.Exec(ctx, `INSERT INTO outbox_events(id,brand_id,event_type,aggregate_id,payload) VALUES($1,$2,'member.joined',$3,$4)`, event, managedBrand, f.memberID, raw); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Process(ctx, 20); e != nil {
		t.Fatal(e)
	}
	base := "/api/v1/admin/notification-deliveries"
	path := base + "/" + event + "/retry"
	body := map[string]any{"attempt_count": 1, "reason": "operator retries with evidence"}
	mustStatus(t, f.call("GET", base, "", f.token, managedBrand, nil), 403)
	for _, key := range []string{"notification.view.brand", "notification.view.platform", "notification.retry.brand"} {
		if _, e := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,$2 FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root, key); e != nil {
			t.Fatal(e)
		}
	}
	mustStatus(t, f.call("GET", base, "", f.token, managedBrand, nil), 200)
	mustStatus(t, f.call("GET", base, "", f.token, pointsBrandB, nil), 403)
	if _, e := f.pool.Exec(ctx, `CREATE FUNCTION reject_notification_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='notification.retry' THEN RAISE EXCEPTION 'test'; END IF;RETURN NEW;END $$;CREATE TRIGGER reject_notification_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_notification_audit()`); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("POST", path, "notification-retry-audit-fail", f.token, managedBrand, body), 503)
	var state string
	f.pool.QueryRow(ctx, `SELECT status FROM notification_deliveries WHERE event_id=$1`, event).Scan(&state)
	if state != "failed" {
		t.Fatal("audit failure leaked retry", state)
	}
	if _, e := f.pool.Exec(ctx, `DROP TRIGGER reject_notification_audit ON audit_logs`); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("POST", path, "notification-retry-first", f.token, managedBrand, body), 200)
	if _, e := s.Process(ctx, 20); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("POST", path, "notification-retry-first", f.token, managedBrand, body), 200)
	mustStatus(t, f.call("POST", path, "notification-retry-stale", f.token, managedBrand, body), 409)
	var audits int
	f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='notification.retry'`).Scan(&audits)
	if audits != 1 {
		t.Fatal(audits)
	}
	if _, e := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='notification.retry.brand'`, f.root); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("POST", path, "notification-retry-first", f.token, managedBrand, body), 403)
	if _, e := f.pool.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); e != nil {
		t.Fatal(e)
	}
	platformToken := platformAdminToken(t, f.managementHTTP)
	platformBase := strings.Replace(base, "/api/v1/admin", "/api/v1/platform", 1)
	mustStatus(t, f.call("GET", base, "", f.token, managedBrand, nil), 403)
	mustStatus(t, f.call("GET", platformBase, "", platformToken, managedBrand, nil), 403)
	grantPlatformPermission(t, f.managementHTTP, "notification.view.platform")
	mustStatus(t, f.call("GET", platformBase, "", platformToken, managedBrand, nil), 200)
	mustStatus(t, f.call("POST", path, "notification-retry-first", f.token, managedBrand, body), 403)
}
