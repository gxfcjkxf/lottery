package httpapi

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/notification"
)

func templateHTTPList(t *testing.T, f managementHTTP) []notification.Template {
	t.Helper()
	r := f.call("GET", "/api/v1/admin/notification-templates", "", f.token, managedBrand, nil)
	mustStatus(t, r, 200)
	var v struct {
		Items []notification.Template `json:"items"`
	}
	managedData(t, r, &v)
	return v.Items
}
func templateHTTPRecord(t *testing.T, f managementHTTP, key string) notification.Template {
	t.Helper()
	for _, v := range templateHTTPList(t, f) {
		if v.Key == key {
			return v
		}
	}
	t.Fatal("missing template", key)
	return notification.Template{}
}
func TestNotificationTemplateHTTPCheckedReplayValidationAndAudit(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	base := "/api/v1/admin/notification-templates"
	path := base + "/recharge.confirmed"
	mustStatus(t, f.call("GET", base, "", f.token, managedBrand, nil), 403)
	grantReportPermission(t, f, "notification_template.view.brand")
	if rows := templateHTTPList(t, f); len(rows) != 14 {
		t.Fatal(len(rows))
	}
	for _, q := range []string{"?unexpected=1", "?limit=20"} {
		mustStatus(t, f.call("GET", base+q, "", f.token, managedBrand, nil), 400)
	}
	current := templateHTTPRecord(t, f, "recharge.confirmed")
	in := notification.UpdateInput{Version: current.Version, Content: current.Content, Reason: "update notification wording"}
	in.Content.En.Body = "Recorded {points} points; reference {resource_id}."
	mustStatus(t, f.call("PUT", path, "template-accepted-001", f.token, managedBrand, in), 403)
	grantReportPermission(t, f, "notification_template.write.brand")
	before := f.call("PUT", path, "template-accepted-001", f.token, managedBrand, in)
	mustStatus(t, before, 200)
	var accepted notification.Template
	managedData(t, before, &accepted)
	if accepted.Version != 2 || accepted.AuditLogID == nil || !reflect.DeepEqual(accepted.Content, in.Content) {
		t.Fatal(accepted)
	}
	mustStatus(t, f.call("PUT", path, "template-stale-001", f.token, managedBrand, in), 409)
	changed := in
	changed.Version = 2
	changed.Content.ZhCN.Body = "记录 {points} 积分，业务编号 {resource_id}。"
	mustStatus(t, f.call("PUT", path, "template-new-001", f.token, managedBrand, changed), 200)
	replay := f.call("PUT", path, "template-accepted-001", f.token, managedBrand, in)
	mustStatus(t, replay, 200)
	var old notification.Template
	managedData(t, replay, &old)
	if !reflect.DeepEqual(old, accepted) || templateHTTPRecord(t, f, "recharge.confirmed").Version != 3 {
		t.Fatal("receipt replaced by live projection", old)
	}
	mustStatus(t, f.call("PUT", base+"/bet.order.placed", "template-accepted-001", f.token, managedBrand, in), 409)
	invalid := changed
	invalid.Version = 3
	invalid.Content.En.Body = "{password} {points}"
	mustStatus(t, f.call("PUT", path, "template-invalid-001", f.token, managedBrand, invalid), 400)
	mustStatus(t, f.call("PUT", path+"?x=1", "template-invalid-002", f.token, managedBrand, changed), 400)
	mustStatus(t, f.call("PUT", path, "template-invalid-003", f.token, managedBrand, map[string]any{"version": 3, "content": changed.Content, "reason": "bad", "extra": true}), 400)
	badContent := map[string]any{"en": map[string]any{"title": "Title", "body": "{points}", "private": true}, "zh-CN": changed.Content.ZhCN}
	mustStatus(t, f.call("PUT", path, "template-invalid-004", f.token, managedBrand, map[string]any{"version": 3, "content": badContent, "reason": "bad"}), 400)
	mustStatus(t, f.call("GET", base+"/recharge.confirmed/history?limit=20&offset=0", "", f.token, managedBrand, nil), 200)
	mustStatus(t, f.call("GET", base+"/unknown/history", "", f.token, managedBrand, nil), 400)
	mustStatus(t, f.call("GET", base+"/recharge.confirmed/history?limit=1&limit=2", "", f.token, managedBrand, nil), 400)
	mustStatus(t, f.call("GET", base, "", f.token, pointsBrandB, nil), 403)
	if _, e := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='notification_template.write.brand'`, f.root); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("PUT", path, "template-accepted-001", f.token, managedBrand, in), 403)
	grantReportPermission(t, f, "notification_template.write.brand")
	if _, e := f.pool.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("PUT", path, "template-accepted-001", f.token, managedBrand, in), 403)
	grantReportPermission(t, f, "notification_template.view.platform")
	mustStatus(t, f.call("GET", base, "", f.token, pointsBrandB, nil), 200)
}
func TestNotificationTemplateHTTPAuditFailureAndDisabledReplay(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	base := "/api/v1/admin/notification-templates"
	grantReportPermission(t, f, "notification_template.view.brand")
	grantReportPermission(t, f, "notification_template.write.brand")
	cur := templateHTTPRecord(t, f, "member.joined")
	in := notification.UpdateInput{Version: 1, Content: cur.Content, Reason: "test template audit"}
	in.Content.En.Body = "A new welcome for {resource_id}."
	if _, e := f.pool.Exec(ctx, `CREATE FUNCTION reject_template_http_audit() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN IF NEW.action LIKE 'notification.template.%' THEN RAISE EXCEPTION 'injected';END IF;RETURN NEW;END$$;CREATE TRIGGER reject_template_http_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_template_http_audit()`); e != nil {
		t.Fatal(e)
	}
	r := f.call("GET", base, "", f.token, managedBrand, nil)
	mustStatus(t, r, 503)
	if strings.Contains(r.Body.String(), "Welcome") {
		t.Fatal("unaudited content leaked")
	}
	mustStatus(t, f.call("PUT", base+"/member.joined", "template-audit-fail", f.token, managedBrand, in), 503)
	var version, revisions int
	if e := f.pool.QueryRow(ctx, `SELECT version,(SELECT count(*) FROM notification_template_revisions WHERE brand_id=$1 AND template_key='member.joined') FROM notification_templates WHERE brand_id=$1 AND template_key='member.joined'`, managedBrand).Scan(&version, &revisions); e != nil || version != 1 || revisions != 1 {
		t.Fatal(version, revisions, e)
	}
	if _, e := f.pool.Exec(ctx, `DROP TRIGGER reject_template_http_audit ON audit_logs`); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("PUT", base+"/member.joined", "template-audit-fail", f.token, managedBrand, in), 200)
	if _, e := f.pool.Exec(ctx, `UPDATE brands SET status='disabled' WHERE id=$1`, managedBrand); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("PUT", base+"/member.joined", "template-audit-fail", f.token, managedBrand, in), 409)
	mustStatus(t, f.call("GET", base, "", f.token, managedBrand, nil), 200)
}
func TestNotificationTemplateHTTPRealRegistrationGetsImmutableCurrentCopy(t *testing.T) {
	f := pointsFixture(t)
	ctx := context.Background()
	service := notification.Service{DB: f.pool}
	grantReportPermission(t, f.managementHTTP, "notification_template.view.brand")
	grantReportPermission(t, f.managementHTTP, "notification_template.write.brand")
	if _, e := service.Process(ctx, 20); e != nil {
		t.Fatal(e)
	}
	first, e := service.List(ctx, managedBrand, f.memberID, 20, 0)
	if e != nil || len(first.Items) != 1 || first.Items[0].Content == nil || first.Items[0].TemplateVersion != 1 {
		t.Fatal(first, e)
	}
	beforeWallet := pointWallet(t, f)
	old := templateHTTPRecord(t, f.managementHTTP, "member.joined")
	content := old.Content
	content.En.Title = "Brand welcome"
	content.En.Body = "Hello member {resource_id}."
	content.ZhCN.Title = "品牌欢迎"
	content.ZhCN.Body = "欢迎会员 {resource_id}。"
	in := notification.UpdateInput{Version: 1, Content: content, Reason: "publish bilingual welcome"}
	r := f.call("PUT", "/api/v1/admin/notification-templates/member.joined", "template-welcome-002", f.token, managedBrand, in)
	mustStatus(t, r, 200)
	var updated notification.Template
	managedData(t, r, &updated)
	registered := f.call("POST", "/api/v1/auth/register", "template-user-new-002", "", managedBrand, map[string]any{"username": "template_notification_user", "password": "template-notification-test-only-password", "privacy_policy_version": "dev-1", "service_terms_version": "dev-1"})
	mustStatus(t, registered, 201)
	var auth identity.Authentication
	managedData(t, registered, &auth)
	if _, e = service.Process(ctx, 20); e != nil {
		t.Fatal(e)
	}
	newer, e := service.List(ctx, managedBrand, auth.Member.ID, 20, 0)
	if e != nil || len(newer.Items) != 1 || newer.Items[0].TemplateVersion != 2 || newer.Items[0].Content == nil || !reflect.DeepEqual(*newer.Items[0].Content, content) {
		t.Fatal(newer, e)
	}
	in.Version = 2
	in.Content.En.Title = "Later welcome"
	mustStatus(t, f.call("PUT", "/api/v1/admin/notification-templates/member.joined", "template-welcome-003", f.token, managedBrand, in), 200)
	preserved, e := service.List(ctx, managedBrand, auth.Member.ID, 20, 0)
	if e != nil || !reflect.DeepEqual(preserved.Items, newer.Items) {
		t.Fatal("historical snapshot rewritten", preserved, e)
	}
	original, e := service.List(ctx, managedBrand, f.memberID, 20, 0)
	if e != nil || !reflect.DeepEqual(original.Items, first.Items) {
		t.Fatal("initial notification rewritten", original, e)
	}
	if !reflect.DeepEqual(beforeWallet, pointWallet(t, f)) {
		t.Fatal("template changed existing wallet")
	}
}
