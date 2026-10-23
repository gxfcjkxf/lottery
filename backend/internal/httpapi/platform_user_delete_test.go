package httpapi

import (
	"context"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
)

func TestPlatformGlobalUserDeleteHTTP(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	user, member := ids.New(), ids.New()
	if _, err := f.pool.Exec(ctx, `INSERT INTO global_users(id,username,password_hash) VALUES($1,'global_delete_test','unused')`, user); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'domain','dev-1','dev-1')`, member, managedBrand, user); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/platform/global-users/" + user + "/delete"
	mustStatus(t, f.call("POST", path, "brand-cannot-delete", f.token, "", map[string]any{}), 403)
	if _, err := f.pool.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	token := platformAdminToken(t, f)
	mustStatus(t, f.call("POST", path, "no-delete-permission", token, "", map[string]any{}), 403)
	role := grantPlatformPermission(t, f, "user.delete.platform")
	mustStatus(t, f.call("POST", path, "brand-header-rejected", token, managedBrand, map[string]any{}), 400)
	mustStatus(t, f.call("POST", path, "unknown-field-rejected", token, "", map[string]any{"brand_id": managedBrand}), 400)
	mustStatus(t, f.call("POST", "/api/v1/platform/users/"+member+"/delete", "old-local-route", token, managedBrand, map[string]any{}), 404)
	r := f.call("POST", path, "global-delete-exact-key", token, "", map[string]any{})
	mustStatus(t, r, 200)
	var receipt adminsys.DeletedUser
	managedData(t, r, &receipt)
	if receipt.ID != user || receipt.AccountName != "global_delete_test" || receipt.AuditLogID == "" {
		t.Fatal(receipt)
	}
	replay := f.call("POST", path, "global-delete-exact-key", token, "", map[string]any{})
	mustStatus(t, replay, 200)
	var repeated adminsys.DeletedUser
	managedData(t, replay, &repeated)
	if repeated != receipt {
		t.Fatal("replay receipt changed", repeated, receipt)
	}
	mustStatus(t, f.call("POST", path, "global-delete-new-key", token, "", map[string]any{}), 404)
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='user.delete.platform'`, role); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("POST", path, "global-delete-exact-key", token, "", map[string]any{}), 403)
	var count int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='user.delete.platform' AND resource_id=$1`, user).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
}
