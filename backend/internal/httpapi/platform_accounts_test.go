package httpapi

import (
	"context"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
)

func TestPlatformAccountManagementHTTP(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	role := grantPlatformPermission(t, f, "admin.view.platform", "admin.write.platform", "admin.delete.platform", "role.view.platform", "role.write.platform")
	token := platformAdminToken(t, f)
	mustStatus(t, f.call("GET", "/api/v1/platform/platform-roles", "", token, "", nil), 200)
	input := map[string]any{"username": "new_platform_operator", "password": "new-platform-password-2026", "role_ids": []string{role}, "reason": "Create test platform operator"}
	created := f.call("POST", "/api/v1/platform/platform-accounts", "platform-account-create-key", token, "", input)
	mustStatus(t, created, 201)
	var account adminsys.AdminRecord
	managedData(t, created, &account)
	if !account.SuperAdmin || len(account.BrandIDs) != 0 || account.Version != 1 {
		t.Fatal(account)
	}
	replay := f.call("POST", "/api/v1/platform/platform-accounts", "platform-account-create-key", token, "", input)
	mustStatus(t, replay, 201)
	var replayAccount adminsys.AdminRecord
	managedData(t, replay, &replayAccount)
	if replayAccount.ID != account.ID || replayAccount.AuditLogID != account.AuditLogID {
		t.Fatal("cached receipt changed")
	}
	login := f.call("POST", "/api/v1/platform/auth/login", "new-platform-login-key", "", "", map[string]string{"identifier": account.Username, "password": "new-platform-password-2026"})
	mustStatus(t, login, 200)
	var auth identity.AdminAuthentication
	managedData(t, login, &auth)
	mustStatus(t, f.call("GET", "/api/v1/admin/me", "", auth.AccessToken, managedBrand, nil), 403)
	reset := f.call("POST", "/api/v1/platform/platform-accounts/"+account.ID+"/reset-password", "platform-password-reset-key", token, "", map[string]any{"version": account.Version, "password": "replacement-platform-pass-2026", "reason": "Reset test operator"})
	mustStatus(t, reset, 200)
	mustStatus(t, f.call("GET", "/api/v1/platform/me", "", auth.AccessToken, "", nil), 401)
	updated := f.call("PATCH", "/api/v1/platform/platform-accounts/"+account.ID, "platform-disable-key", token, "", map[string]any{"version": 2, "status": "disabled", "role_ids": []string{role}, "reason": "Disable test operator"})
	mustStatus(t, updated, 200)
	managedData(t, updated, &account)
	if account.Status != "disabled" || account.Version != 3 {
		t.Fatal(account)
	}
	deleted := f.call("POST", "/api/v1/platform/platform-accounts/"+account.ID+"/delete", "platform-account-delete-key", token, "", map[string]any{"version": account.Version})
	mustStatus(t, deleted, 200)
	if got := f.call("GET", "/api/v1/platform/platform-accounts", "", token, "", nil); got.Code != 200 || strings.Contains(got.Body.String(), account.ID) {
		t.Fatalf("deleted platform account remains in list: status=%d body=%s", got.Code, got.Body.String())
	}
	mustStatus(t, f.call("GET", "/api/v1/platform/platform-accounts", "", token, managedBrand, nil), 400)
	// Brand staff account administration reuses the same domain, not platform privileges disguised as brand roles.
	brandRole := f.call("POST", "/api/v1/platform/roles", "platform-brand-role-key", token, managedBrand, map[string]any{"code": "platform_created_support", "name": "Support", "permissions": []string{"user.view.brand"}, "reason": "Create brand support role"})
	mustStatus(t, brandRole, 201)
	var roleRecord adminsys.RoleRecord
	managedData(t, brandRole, &roleRecord)
	staff := f.call("POST", "/api/v1/platform/accounts", "platform-brand-staff-key", token, managedBrand, map[string]any{"username": "platform_created_staff", "password": "new-brand-staff-password-2026", "role_ids": []string{roleRecord.ID}, "reason": "Create brand staff"})
	mustStatus(t, staff, 201)
	var staffRecord adminsys.AdminRecord
	managedData(t, staff, &staffRecord)
	if staffRecord.SuperAdmin || len(staffRecord.BrandIDs) != 1 || staffRecord.BrandIDs[0] != managedBrand {
		t.Fatal(staffRecord)
	}
	// Existing brand session cannot cross into the platform management surface.
	staffLogin := f.call("POST", "/api/v1/admin/auth/login", "staff-entry-login-key", "", managedBrand, map[string]string{"identifier": staffRecord.Username, "password": "new-brand-staff-password-2026"})
	mustStatus(t, staffLogin, 200)
	var staffAuth identity.AdminAuthentication
	managedData(t, staffLogin, &staffAuth)
	mustStatus(t, f.call("POST", "/api/v1/platform/platform-accounts", "wrong-entry-account-key", staffAuth.AccessToken, "", input), 403)
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='admin.write.platform'`, role); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("POST", "/api/v1/platform/platform-accounts", "platform-account-create-key", token, "", input), 403)
}

func TestPlatformBrandRoleDeletionHTTP(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	grant := grantPlatformPermission(t, f, "role.write.platform", "role.delete.platform")
	token := platformAdminToken(t, f)
	created := f.call("POST", "/api/v1/platform/roles", "deletion-create-role", token, managedBrand, map[string]any{
		"code": "deletable_support", "name": "Support", "permissions": []string{"user.view.brand"},
	})
	mustStatus(t, created, 201)
	var role adminsys.RoleRecord
	managedData(t, created, &role)
	path := "/api/v1/platform/roles/" + role.ID + "/delete"
	body := map[string]any{"version": role.Version}
	// A role in another brand cannot be deleted by changing the request scope.
	mustStatus(t, f.call("POST", path, "deletion-other-brand", token, "0199a000-0000-7000-8000-000000000002", body), 404)
	mustStatus(t, f.call("POST", path, "deletion-stale-version", token, managedBrand, map[string]any{"version": role.Version + 1}), 409)
	if _, err := f.pool.Exec(ctx, `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, f.root, role.ID); err != nil {
		t.Fatal(err)
	}
	assigned := f.call("POST", path, "deletion-assigned-role", token, managedBrand, body)
	mustStatus(t, assigned, 409)
	if !strings.Contains(assigned.Body.String(), `"code":"ROLE_ASSIGNED"`) {
		t.Fatal(assigned.Body.String())
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM admin_account_roles WHERE account_id=$1 AND role_id=$2`, f.root, role.ID); err != nil {
		t.Fatal(err)
	}
	deleted := f.call("POST", path, "deletion-original-key", token, managedBrand, body)
	mustStatus(t, deleted, 200)
	var receipt struct {
		AuditID string `json:"audit_log_id"`
	}
	managedData(t, deleted, &receipt)
	if receipt.AuditID == "" {
		t.Fatal("missing deletion audit receipt")
	}
	replayed := f.call("POST", path, "deletion-original-key", token, managedBrand, body)
	mustStatus(t, replayed, 200)
	var replayReceipt struct {
		AuditID string `json:"audit_log_id"`
	}
	managedData(t, replayed, &replayReceipt)
	if replayReceipt != receipt {
		t.Fatal("deletion replay changed the audit receipt")
	}
	var remaining, audits int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM roles WHERE id=$1`, role.ID).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE resource_id=$1 AND action='role.delete'`, role.ID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 || audits != 1 {
		t.Fatalf("remaining roles=%d, deletion audits=%d", remaining, audits)
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='role.delete.platform'`, grant); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("POST", path, "deletion-original-key", token, managedBrand, body), 403)
}

func TestPlatformRoleCreationHTTP(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	grant := grantPlatformPermission(t, f, "role.view.platform", "role.write.platform", "admin.view.platform", "admin.write.platform", "brand.view.platform", "platform_settings.view.platform")
	token := platformAdminToken(t, f)
	path := "/api/v1/platform/platform-roles"
	input := map[string]any{"code": "platform_reader_custom", "name": "Custom platform reader", "status": "active", "permissions": []string{"brand.view.platform", "platform_settings.view.platform"}}
	mustStatus(t, f.call("POST", path, "platform-role-no-session", "", "", input), 401)
	mustStatus(t, f.call("POST", path, "platform-role-brand-header", token, managedBrand, input), 400)
	for _, permission := range []string{"user.view.brand", "secret.write.platform", "invalid"} {
		bad := map[string]any{"code": "bad_platform_role", "name": "Invalid role", "permissions": []string{permission}}
		want := 400
		if permission == "secret.write.platform" {
			want = 403
		}
		mustStatus(t, f.call("POST", path, "platform-role-bad-"+permission, token, "", bad), want)
	}
	created := f.call("POST", path, "platform-role-create-key", token, "", input)
	mustStatus(t, created, 201)
	var role adminsys.RoleRecord
	managedData(t, created, &role)
	if role.BrandID != "" || role.IsBootstrap || role.Version != 1 || role.AuditLogID == "" || len(role.Permissions) != 2 || role.Permissions[0] != "brand.view.platform" {
		t.Fatal(role)
	}
	replay := f.call("POST", path, "platform-role-create-key", token, "", input)
	mustStatus(t, replay, 201)
	var replayRole adminsys.RoleRecord
	managedData(t, replay, &replayRole)
	if replayRole.ID != role.ID || replayRole.AuditLogID != role.AuditLogID {
		t.Fatal("role replay changed")
	}
	mustStatus(t, f.call("POST", path, "platform-role-duplicate-code", token, "", input), 409)
	changed := map[string]any{"code": "platform_reader_changed", "name": "Changed", "permissions": []string{"brand.view.platform"}}
	mustStatus(t, f.call("POST", path, "platform-role-create-key", token, "", changed), 409)
	var count int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE resource_id=$1 AND action='platform.role.create'`, role.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("audit count %d: %v", count, err)
	}
	// The role is immediately assignable to platform accounts through the existing API.
	accountInput := map[string]any{"username": "custom_role_reader", "password": "custom-reader-pass-2026", "role_ids": []string{role.ID}}
	accountResponse := f.call("POST", "/api/v1/platform/platform-accounts", "custom-role-account-key", token, "", accountInput)
	mustStatus(t, accountResponse, 201)
	var account adminsys.AdminRecord
	managedData(t, accountResponse, &account)
	if !account.SuperAdmin || len(account.BrandIDs) != 0 || len(account.RoleIDs) != 1 || account.RoleIDs[0] != role.ID {
		t.Fatal(account)
	}
	login := f.call("POST", "/api/v1/platform/auth/login", "custom-reader-login", "", "", map[string]string{"identifier": account.Username, "password": "custom-reader-pass-2026"})
	mustStatus(t, login, 200)
	var auth identity.AdminAuthentication
	managedData(t, login, &auth)
	mustStatus(t, f.call("GET", "/api/v1/platform/brands", "", auth.AccessToken, "", nil), 200)
	mustStatus(t, f.call("GET", "/api/v1/platform/platform-settings", "", auth.AccessToken, "", nil), 200)
	mustStatus(t, f.call("POST", path, "custom-reader-denied", auth.AccessToken, "", changed), 403)
	// Cached creation receipts still require current platform role write permission.
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='role.write.platform'`, grant); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("POST", path, "platform-role-create-key", token, "", input), 403)
}
