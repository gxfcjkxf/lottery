package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/authcrypto"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
)

func TestAdministrativeEntriesRejectOtherAccountTypeAndSeparateCookies(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	id, role := ids.New(), ids.New()
	hash, err := authcrypto.HashPassword("platform-entry-test-password-2026")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash,is_super_admin) VALUES($1,'platform_entry',$2,true)`, id, hash); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `INSERT INTO roles(id,code,name,is_bootstrap) VALUES($1,$2,'Platform reader',true)`, role, "entry_"+role); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, id, role); err != nil {
		t.Fatal(err)
	}
	for _, permission := range []string{"brand.view.platform", "user.view.platform"} {
		if _, err = f.pool.Exec(ctx, `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, permission); err != nil {
			t.Fatal(err)
		}
		if _, err = f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2)`, role, permission); err != nil {
			t.Fatal(err)
		}
	}
	wrongPlatform := f.call("POST", "/api/v1/platform/auth/login", "entry-wrong-platform", "", "", map[string]string{"identifier": "managed_root", "password": "root-test-password-2026"})
	mustStatus(t, wrongPlatform, 403)
	wrongBrand := f.call("POST", "/api/v1/admin/auth/login", "entry-wrong-brand", "", "", map[string]string{"identifier": "platform_entry", "password": "platform-entry-test-password-2026"})
	mustStatus(t, wrongBrand, 403)
	var sessions int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE admin_id=$1`, id).Scan(&sessions); err != nil || sessions != 0 {
		t.Fatal("wrong entry created a session", sessions, err)
	}
	login := f.call("POST", "/api/v1/platform/auth/login", "entry-platform-login", "", "", map[string]string{"identifier": "platform_entry", "password": "platform-entry-test-password-2026"})
	mustStatus(t, login, 200)
	var auth identity.AdminAuthentication
	managedData(t, login, &auth)
	cookies := login.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "lottery_platform_admin" || cookies[0].Path != "/api/v1/platform" || !cookies[0].HttpOnly {
		t.Fatal("platform cookie boundary", cookies)
	}
	mustStatus(t, f.call("GET", "/api/v1/admin/me", "", auth.AccessToken, "", nil), 403)
	mustStatus(t, f.call("GET", "/api/v1/platform/me", "", f.token, "", nil), 403)
	brands := f.call("GET", "/api/v1/platform/brands", "", auth.AccessToken, "", nil)
	mustStatus(t, brands, 200)
	var listed struct {
		Items []json.RawMessage `json:"items"`
	}
	managedData(t, brands, &listed)
	if len(listed.Items) != 2 {
		t.Fatal("platform did not see both brands")
	}
	mustStatus(t, f.call("GET", "/api/v1/platform/users", "", auth.AccessToken, "0199a000-0000-7000-8000-000000000002", nil), 200)
	for _, path := range []string{"/users/" + ids.New(), "/withdrawals/" + ids.New() + "/approve", "/games", "/rules/" + ids.New() + "/review"} {
		mustStatus(t, f.call("POST", "/api/v1/platform"+path, "entry-forbidden-write", auth.AccessToken, managedBrand, map[string]any{}), 404)
	}
	request := httptest.NewRequest("GET", "http://localhost/api/v1/admin/me", nil)
	request.AddCookie(cookies[0])
	w := httptest.NewRecorder()
	f.http.ServeHTTP(w, request)
	mustStatus(t, w, 401)
	brandLogin := f.call("POST", "/api/v1/admin/auth/login", "entry-brand-login", "", "", map[string]string{"identifier": "managed_root", "password": "root-test-password-2026"})
	mustStatus(t, brandLogin, 200)
	brandCookies := brandLogin.Result().Cookies()
	if len(brandCookies) != 1 || brandCookies[0].Name != adminCookie || brandCookies[0].Path != "/api/v1/admin" {
		t.Fatal("brand cookie boundary", brandCookies)
	}
	mustStatus(t, f.call("POST", "/api/v1/platform/auth/logout", "entry-wrong-logout", f.token, "", map[string]any{}), 403)
	mustStatus(t, f.call("GET", "/api/v1/admin/me", "", f.token, "", nil), 200)
}

func TestAdministrativeEntryScopesDoNotInheritMisplacedGrants(t *testing.T) {
	account := access.Account{ID: "staff", Type: access.AccountAdmin, BrandIDs: []string{"aurora"}, Roles: []access.Role{{Permissions: []access.Permission{
		{Resource: "user", Action: "view", Scope: access.ScopePlatform},
		{Resource: "user", Action: "view", Scope: access.ScopeBrand},
	}}}}
	brand := entryPermissions(account, false)
	if access.Authorize(brand, "user", "view", access.ScopePlatform, "") || access.Authorize(brand, "user", "view", access.ScopeBrand, "harbor") {
		t.Fatal("brand account escaped its scope")
	}
	if !access.Authorize(brand, "user", "view", access.ScopeBrand, "aurora") {
		t.Fatal("brand grant lost")
	}
	account.SuperAdmin = true
	platform := entryPermissions(account, true)
	if !access.Authorize(platform, "user", "view", access.ScopePlatform, "") || access.Authorize(platform, "user", "view", access.ScopeBrand, "aurora") {
		t.Fatal("platform entry accepted brand grants")
	}
}
