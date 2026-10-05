package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/authcrypto"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/tenant"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"net/http/httptest"
	"testing"
)

func TestAdminHTTPAuthorizationKickAndAudit(t *testing.T) {
	p := testdb.New(t)
	ctx := context.Background()
	users, _ := identity.New(p)
	engine, _ := mutation.New(p, make([]byte, 32))
	h := New(Dependencies{Brands: tenant.Store{DB: p}, Ready: p.Ping, Identity: users, Mutations: engine, Admins: adminsys.Store{DB: p}})
	a := "0199a000-0000-7000-8000-000000000001"
	b := "0199a000-0000-7000-8000-000000000002"
	admin, role := ids.New(), ids.New()
	hash, _ := authcrypto.HashPassword("admin-test-password-123")
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,'operator',$2)", []any{admin, hash}},
		{"INSERT INTO roles(id,code,name) VALUES($1,'operator','Operator')", []any{role}},
		{"INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)", []any{admin, role}},
		{"INSERT INTO admin_brand_scopes(account_id,brand_id) VALUES($1,$2)", []any{admin, a}},
	} {
		if _, err := p.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, permission := range []string{"user.view.brand", "user.write.brand", "user.kick.brand", "user.password_reset.brand", "audit.view.brand", "brand.view.brand"} {
		p.Exec(ctx, "INSERT INTO permissions(key) VALUES($1)", permission)
		p.Exec(ctx, "INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2)", role, permission)
	}
	call := func(method, path, key, token, brand string, body any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "http://localhost"+path, bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", key)
		r.Header.Set("X-Brand-ID", brand)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	r := call("POST", "/api/v1/auth/register", "register-test-001", "", a, map[string]string{"username": "member_test", "password": "member-test-password-123", "privacy_policy_version": "dev-1", "service_terms_version": "dev-1"})
	var u struct {
		Data identity.Authentication `json:"data"`
	}
	if r.Code != 201 {
		t.Fatal(r.Body.String())
	}
	json.Unmarshal(r.Body.Bytes(), &u)
	r = call("POST", "/api/v1/admin/auth/login", "admin-login-001", "", a, map[string]string{"identifier": "operator", "password": "admin-test-password-123"})
	if r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	var s struct {
		Data identity.AdminAuthentication `json:"data"`
	}
	json.Unmarshal(r.Body.Bytes(), &s)
	if r := call("GET", "/api/v1/admin/users", "", s.Data.AccessToken, b, nil); r.Code != 403 {
		t.Fatal("cross-brand users", r.Code)
	}
	if r := call("GET", "/api/v1/admin/users", "", u.Data.AccessToken, a, nil); r.Code != 401 {
		t.Fatal("user got admin access", r.Code)
	}
	if r := call("PATCH", "/api/v1/admin/users/"+u.Data.Member.ID, "edit-member-001", s.Data.AccessToken, a, map[string]string{"status": "frozen", "notes": "manual review", "reason": "test reason"}); r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	if r := call("GET", "/api/v1/me", "", u.Data.AccessToken, a, nil); r.Code != 200 {
		t.Fatal("frozen user cannot inspect account")
	}
	if r := call("POST", "/api/v1/admin/users/"+u.Data.Member.ID+"/kick", "kick-member-001", s.Data.AccessToken, a, map[string]string{"reason": "test kick"}); r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	if r := call("GET", "/api/v1/me", "", u.Data.AccessToken, a, nil); r.Code != 401 {
		t.Fatal("kick didn't revoke session")
	}
	r = call("GET", "/api/v1/admin/audit", "", s.Data.AccessToken, a, nil)
	if r.Code != 200 || !bytes.Contains(r.Body.Bytes(), []byte("test kick")) {
		t.Fatal("audit missing", r.Body.String())
	}
	p.Exec(ctx, "UPDATE admin_accounts SET is_super_admin=true WHERE id=$1", admin)
	if r := call("PATCH", "/api/v1/admin/users/"+u.Data.Member.ID, "super-edit-001", s.Data.AccessToken, a, map[string]string{"status": "normal", "reason": "test"}); r.Code != 403 {
		t.Fatal("super admin changed user", r.Code)
	}
}
