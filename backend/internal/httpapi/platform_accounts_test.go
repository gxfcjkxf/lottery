package httpapi

import (
	"context"
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
	role := grantPlatformPermission(t, f, "admin.view.platform", "admin.write.platform", "role.view.platform", "role.write.platform")
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
	reset := f.call("POST", "/api/v1/platform/platform-accounts/"+account.ID+"/reset-password", "platform-password-reset-key", token, "", map[string]any{"version": account.Version, "password": "replacement-platform-password-2026", "reason": "Reset test operator"})
	mustStatus(t, reset, 200)
	mustStatus(t, f.call("GET", "/api/v1/platform/me", "", auth.AccessToken, "", nil), 401)
	updated := f.call("PATCH", "/api/v1/platform/platform-accounts/"+account.ID, "platform-disable-key", token, "", map[string]any{"version": 2, "status": "disabled", "role_ids": []string{role}, "reason": "Disable test operator"})
	mustStatus(t, updated, 200)
	managedData(t, updated, &account)
	if account.Status != "disabled" || account.Version != 3 {
		t.Fatal(account)
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
