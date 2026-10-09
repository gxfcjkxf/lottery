package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/tenant"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

const managedBrand = "0199a000-0000-7000-8000-000000000001"

type managementHTTP struct {
	pool        *pgxpool.Pool
	http        http.Handler
	root, token string
}

func managedFixture(t *testing.T) managementHTTP {
	t.Helper()
	p := testdb.New(t)
	ctx := context.Background()
	users, err := identity.New(p)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := users.PasswordHash(ctx, "root-test-password-2026")
	if err != nil {
		t.Fatal(err)
	}
	root, role := ids.New(), ids.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,'managed_root',$2)`, []any{root, hash}},
		{`INSERT INTO admin_brand_scopes(account_id,brand_id) VALUES($1,$2)`, []any{root, managedBrand}},
		{`INSERT INTO roles(id,brand_id,code,name,is_bootstrap) VALUES($1,$2,'managed_root','Test root',true)`, []any{role, managedBrand}},
		{`INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, []any{root, role}},
	} {
		if _, err := p.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, key := range []string{"user.view.brand", "user.write.brand", "user.kick.brand", "user.password_reset.brand", "user.create.brand", "brand.view.brand", "audit.view.brand", "role.view.brand", "role.write.brand", "admin.view.brand", "admin.write.brand", "auth_config.view.brand", "auth_config.write.brand"} {
		if _, err = p.Exec(ctx, `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, key); err != nil {
			t.Fatal(err)
		}
		if _, err = p.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2)`, role, key); err != nil {
			t.Fatal(err)
		}
	}
	e, err := mutation.New(p, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	f := managementHTTP{pool: p, root: root, http: New(Dependencies{Brands: tenant.Store{DB: p}, Ready: p.Ping, Identity: users, Mutations: e, Admins: adminsys.Store{DB: p}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})}
	r := f.call("POST", "/api/v1/admin/auth/login", "root-login-001", "", managedBrand, map[string]string{"identifier": "managed_root", "password": "root-test-password-2026"})
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	var auth identity.AdminAuthentication
	managedData(t, r, &auth)
	f.token = auth.AccessToken
	return f
}
func (f managementHTTP) call(method, path, key, token, brand string, body any) *httptest.ResponseRecorder {
	encoded, _ := json.Marshal(body)
	r := httptest.NewRequest(method, "http://localhost"+path, bytes.NewReader(encoded))
	r.RemoteAddr = "192.0.2.55:12345"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", key)
	r.Header.Set("X-Brand-ID", brand)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.http.ServeHTTP(w, r)
	return w
}

func grantPlatformPermission(t *testing.T, f managementHTTP, permissions ...string) string {
	t.Helper()
	for _, permission := range permissions {
		if !strings.HasSuffix(permission, ".platform") {
			t.Fatalf("platform fixture requires a platform permission: %s", permission)
		}
	}
	ctx := context.Background()
	role := ids.New()
	if _, err := f.pool.Exec(ctx, `INSERT INTO roles(id,brand_id,code,name) VALUES($1,NULL,$2,'Platform test grant')`, role, "platform_fixture_"+role); err != nil {
		t.Fatal(err)
	}
	for _, permission := range permissions {
		if _, err := f.pool.Exec(ctx, `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, permission); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2)`, role, permission); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, f.root, role); err != nil {
		t.Fatal(err)
	}
	return role
}

func platformAdminToken(t *testing.T, f managementHTTP) string {
	t.Helper()
	response := f.call("POST", "/api/v1/platform/auth/login", "platform-login-"+ids.New(), "", "", map[string]string{
		"identifier": "managed_root",
		"password":   "root-test-password-2026",
	})
	mustStatus(t, response, 200)
	var auth identity.AdminAuthentication
	managedData(t, response, &auth)
	return auth.AccessToken
}

func managedData(t *testing.T, r *httptest.ResponseRecorder, dst any) {
	t.Helper()
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(envelope.Data, dst); err != nil {
		t.Fatal(r.Code, err, r.Body.String())
	}
}

func TestManagementHTTPRolesAccountsReplayRevocationAndAudit(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	roleBody := map[string]any{"code": "member_reader", "name": "Reader", "permissions": []string{"user.view.brand", "brand.view.brand", "admin.view.brand", "role.view.brand"}, "reason": "test role"}
	r := f.call("POST", "/api/v1/admin/roles", "create-role-001", f.token, managedBrand, roleBody)
	if r.Code != 201 {
		t.Fatal(r.Code, r.Body.String())
	}
	var role adminsys.RoleRecord
	managedData(t, r, &role)
	replay := f.call("POST", "/api/v1/admin/roles", "create-role-001", f.token, managedBrand, roleBody)
	var same adminsys.RoleRecord
	managedData(t, replay, &same)
	if replay.Code != 201 || same.ID != role.ID {
		t.Fatal("role replay was not stable")
	}
	changed := map[string]any{"code": "member_reader", "name": "Different", "permissions": roleBody["permissions"], "reason": "test role"}
	if r = f.call("POST", "/api/v1/admin/roles", "create-role-001", f.token, managedBrand, changed); r.Code != 409 {
		t.Fatal("changed idempotency body accepted", r.Code)
	}
	r = f.call("POST", "/api/v1/admin/roles", "create-role-002", f.token, managedBrand, map[string]any{"code": "extra_reader", "name": "Extra reader", "permissions": []string{"user.view.brand", "user.view.brand"}, "reason": "deduplicate"})
	if r.Code != 201 {
		t.Fatal(r.Code, r.Body.String())
	}
	var extra adminsys.RoleRecord
	managedData(t, r, &extra)
	if len(extra.Permissions) != 1 {
		t.Fatal("duplicate permission retained")
	}
	r = f.call("POST", "/api/v1/admin/accounts", "create-staff-001", f.token, managedBrand, map[string]any{"username": "managed_staff", "password": "staff-test-password-2026", "role_ids": []string{role.ID, extra.ID, role.ID}, "reason": "create reader"})
	if r.Code != 201 {
		t.Fatal(r.Code, r.Body.String())
	}
	var staff adminsys.AdminRecord
	managedData(t, r, &staff)
	if len(staff.RoleIDs) != 2 {
		t.Fatal("duplicate role retained")
	}
	staffLogin := func(key string) string {
		r := f.call("POST", "/api/v1/admin/auth/login", key, "", managedBrand, map[string]string{"identifier": "managed_staff", "password": "staff-test-password-2026"})
		if r.Code != 200 {
			t.Fatal(r.Code, r.Body.String())
		}
		var auth identity.AdminAuthentication
		managedData(t, r, &auth)
		return auth.AccessToken
	}
	token := staffLogin("staff-login-001")
	r = f.call("GET", "/api/v1/admin/me", "", token, managedBrand, nil)
	var me struct {
		Account struct {
			Permissions []string            `json:"permissions"`
			ByBrand     map[string][]string `json:"permissions_by_brand"`
		} `json:"account"`
	}
	managedData(t, r, &me)
	count := 0
	for _, p := range me.Account.ByBrand[managedBrand] {
		if p == "user.view.brand" {
			count++
		}
	}
	if count != 1 {
		t.Fatal("role permissions were not union-deduplicated")
	}
	if r = f.call("POST", "/api/v1/admin/accounts", "reader-create-001", token, managedBrand, map[string]any{"username": "forbidden_staff", "password": "not-a-real-password", "role_ids": []string{role.ID}, "reason": "not allowed"}); r.Code != 403 {
		t.Fatal("read-only staff created an admin", r.Code)
	}
	if r = f.call("GET", "/api/v1/admin/roles", "", f.token, "0199a000-0000-7000-8000-000000000002", nil); r.Code != 403 {
		t.Fatal("cross-brand role query accepted", r.Code)
	}
	r = f.call("PATCH", "/api/v1/admin/roles/"+role.ID, "update-role-001", f.token, managedBrand, map[string]any{"version": 1, "name": "Reader revised", "status": "active", "permissions": []string{"brand.view.brand", "role.view.brand", "admin.view.brand"}, "reason": "remove one grant"})
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	if r = f.call("GET", "/api/v1/admin/me", "", token, managedBrand, nil); r.Code != 401 {
		t.Fatal("role change did not revoke old session", r.Code)
	}
	token = staffLogin("staff-login-002")
	r = f.call("GET", "/api/v1/admin/me", "", token, managedBrand, nil)
	managedData(t, r, &me)
	count = 0
	for _, p := range me.Account.ByBrand[managedBrand] {
		if p == "user.view.brand" {
			count++
		}
	}
	if count != 1 {
		t.Fatal("second role grant lost after first role update")
	}
	update := map[string]any{"version": 1, "role_ids": []string{role.ID}, "status": "disabled", "reason": "disable reader"}
	if r = f.call("PATCH", "/api/v1/admin/accounts/"+staff.ID, "stale-staff-001", f.token, managedBrand, update); r.Code != 409 {
		t.Fatal("stale admin version accepted", r.Code, r.Body.String())
	}
	update["version"] = 2
	if r = f.call("PATCH", "/api/v1/admin/accounts/"+staff.ID, "update-staff-001", f.token, managedBrand, update); r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	if r = f.call("GET", "/api/v1/admin/me", "", token, managedBrand, nil); r.Code != 401 {
		t.Fatal("disabled staff remained authenticated")
	}
	var denied, roleAudits int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE action='access.denied'),count(*) FILTER(WHERE action='role.create') FROM audit_logs`).Scan(&denied, &roleAudits); err != nil || denied < 2 || roleAudits != 2 {
		t.Fatal("missing/duplicate audit", denied, roleAudits, err)
	}
	var auditText string
	if err := f.pool.QueryRow(ctx, `SELECT string_agg(coalesce(before_json::text,'')||coalesce(after_json::text,''),' ') FROM audit_logs`).Scan(&auditText); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(auditText, "staff-test-password") || strings.Contains(auditText, "argon2") {
		t.Fatal("credential leaked into audit")
	}
}

func TestManagementHTTPOperatorConsentConfigCASAndSuperRestrictions(t *testing.T) {
	f := managedFixture(t)
	body := map[string]string{"username": "operator_member", "password": "member-test-password-2026", "display_name": "Created member", "notes": "test fixture", "reason": "provision user"}
	r := f.call("POST", "/api/v1/admin/users", "operator-create-001", f.token, managedBrand, body)
	if r.Code != 201 {
		t.Fatal(r.Code, r.Body.String())
	}
	var created struct {
		UserID        string `json:"user_id"`
		MemberID      string `json:"member_id"`
		TermsAccepted bool   `json:"terms_accepted"`
	}
	managedData(t, r, &created)
	if created.TermsAccepted || created.MemberID == "" {
		t.Fatal("operator impersonated consent")
	}
	var sessionCount int
	f.pool.QueryRow(context.Background(), `SELECT count(*) FROM sessions WHERE user_id=$1`, created.UserID).Scan(&sessionCount)
	if sessionCount != 0 {
		t.Fatal("operator-created user received a session")
	}
	login := map[string]string{"identifier": "operator_member", "password": "member-test-password-2026"}
	if r = f.call("POST", "/api/v1/auth/login", "member-login-001", "", managedBrand, login); r.Code != 409 {
		t.Fatal("missing user consent accepted", r.Code, r.Body.String())
	}
	login["privacy_policy_version"] = "dev-1"
	login["service_terms_version"] = "dev-1"
	if r = f.call("POST", "/api/v1/auth/login", "member-login-002", "", managedBrand, login); r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	r = f.call("GET", "/api/v1/admin/auth-settings", "", f.token, managedBrand, nil)
	var config identity.AuthSettingsRecord
	managedData(t, r, &config)
	patch := map[string]any{"version": config.Version, "captcha_enabled": true, "telegram_enabled": false, "telegram_client_id": "", "reason": "enable local captcha"}
	r = f.call("PATCH", "/api/v1/admin/auth-settings", "config-patch-001", f.token, managedBrand, patch)
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	var after identity.AuthSettingsRecord
	managedData(t, r, &after)
	if !after.CaptchaEnabled || after.PrivacyPolicyVersion != "dev-1" || after.ServiceTermsVersion != "dev-1" {
		t.Fatal("config update lost policies", r.Body.String())
	}
	if r = f.call("PATCH", "/api/v1/admin/auth-settings", "config-stale-001", f.token, managedBrand, patch); r.Code != 409 {
		t.Fatal("config CAS missing", r.Code)
	}
	if r = f.call("POST", "/api/v1/auth/login", "member-login-no-captcha", "", managedBrand, login); r.Code != 400 {
		t.Fatal("enabled captcha bypassed", r.Code)
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	body["username"] = "super_forbidden_member"
	if r = f.call("POST", "/api/v1/admin/users", "super-create-001", f.token, managedBrand, body); r.Code != 403 {
		t.Fatal("super admin created a user", r.Code)
	}
}
