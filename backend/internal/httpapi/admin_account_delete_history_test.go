package httpapi

import (
	"context"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
)

func TestPlatformDeletesAdminWithHistoricalReferences(t *testing.T) {
	for _, platform := range []bool{false, true} {
		name := "brand"
		if platform {
			name = "platform"
		}
		t.Run(name, func(t *testing.T) {
			f := managedFixture(t)
			ctx := context.Background()
			if _, err := f.pool.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
				t.Fatal(err)
			}
			globalRole := grantPlatformPermission(t, f, "admin.view.platform", "admin.write.platform", "admin.delete.platform", "role.write.platform", "role.view.platform")
			token := platformAdminToken(t, f)
			path, brand, loginPath, role := "/api/v1/platform/accounts", managedBrand, "/api/v1/admin/auth/login", globalRole
			if platform {
				path = "/api/v1/platform/platform-accounts"
				brand = ""
				loginPath = "/api/v1/platform/auth/login"
			} else {
				w := f.call("POST", "/api/v1/platform/roles", "history-role-create", token, brand, map[string]any{"code": "history_operator", "name": "History operator", "permissions": []string{"user.view.brand"}})
				mustStatus(t, w, 201)
				var r adminsys.RoleRecord
				managedData(t, w, &r)
				role = r.ID
			}
			password := "history-password-2026"
			input := map[string]any{"username": "history_operator", "password": password, "role_ids": []string{role}}
			created := f.call("POST", path, "history-account-create", token, brand, input)
			mustStatus(t, created, 201)
			var target adminsys.AdminRecord
			managedData(t, created, &target)
			login := f.call("POST", loginPath, "history-target-login", "", brand, map[string]string{"identifier": target.Username, "password": password})
			mustStatus(t, login, 200)
			var targetAuth identity.AdminAuthentication
			managedData(t, login, &targetAuth)
			user, member := ids.New(), ids.New()
			if _, err := f.pool.Exec(ctx, `INSERT INTO global_users(id,username) VALUES($1,'history_member')`, user); err != nil {
				t.Fatal(err)
			}
			if _, err := f.pool.Exec(ctx, `INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version,created_by) VALUES($1,$2,$3,'operator','v1','v1',$4)`, member, managedBrand, user, target.ID); err != nil {
				t.Fatal(err)
			}
			var before string
			if err := f.pool.QueryRow(ctx, `SELECT to_jsonb(m)::text FROM brand_members m WHERE id=$1`, member).Scan(&before); err != nil {
				t.Fatal(err)
			}
			endpoint := path + "/" + target.ID
			mustStatus(t, f.call("POST", endpoint+"/delete", "history-stale", token, brand, map[string]int64{"version": target.Version + 1}), 409)
			mustStatus(t, f.call("POST", path+"/"+f.root+"/delete", "history-self", token, brand, map[string]int64{"version": 1}), 403)
			if !platform {
				mustStatus(t, f.call("POST", endpoint+"/delete", "history-cross-brand", token, "0199a000-0000-7000-8000-000000000002", map[string]int64{"version": target.Version}), 403)
			}
			body := map[string]int64{"version": target.Version}
			// Audit failure must roll back credential, session and grant removal.
			if _, err := f.pool.Exec(ctx, `CREATE FUNCTION fail_admin_delete_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='admin.delete' THEN RAISE EXCEPTION 'delete audit failed'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_admin_delete_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION fail_admin_delete_audit()`); err != nil {
				t.Fatal(err)
			}
			mustStatus(t, f.call("POST", endpoint+"/delete", "history-delete", token, brand, body), 503)
			var retainedStatus string
			var retainedSessions, retainedRoles int
			if err := f.pool.QueryRow(ctx, `SELECT status,(SELECT count(*) FROM sessions WHERE admin_id=a.id),(SELECT count(*) FROM admin_account_roles WHERE account_id=a.id) FROM admin_accounts a WHERE id=$1`, target.ID).Scan(&retainedStatus, &retainedSessions, &retainedRoles); err != nil || retainedStatus != "active" || retainedSessions == 0 || retainedRoles == 0 {
				t.Fatal("failed deletion did not roll back", err)
			}
			if _, err := f.pool.Exec(ctx, `DROP TRIGGER fail_admin_delete_audit ON audit_logs; DROP FUNCTION fail_admin_delete_audit()`); err != nil {
				t.Fatal(err)
			}

			deleted := f.call("POST", endpoint+"/delete", "history-delete", token, brand, body)
			mustStatus(t, deleted, 200)
			replay := f.call("POST", endpoint+"/delete", "history-delete", token, brand, body)
			mustStatus(t, replay, 200)
			var first, second map[string]string
			managedData(t, deleted, &first)
			managedData(t, replay, &second)
			if first["audit_log_id"] == "" || first["audit_log_id"] != second["audit_log_id"] {
				t.Fatal("delete replay did not return the original audit")
			}
			var status, hash, after string
			var version int64
			var sessions, roles, scopes, audits int
			if err := f.pool.QueryRow(ctx, `SELECT status,password_hash,version,(SELECT count(*) FROM sessions WHERE admin_id=a.id),(SELECT count(*) FROM admin_account_roles WHERE account_id=a.id),(SELECT count(*) FROM admin_brand_scopes WHERE account_id=a.id),(SELECT count(*) FROM audit_logs WHERE action='admin.delete' AND resource_id=a.id) FROM admin_accounts a WHERE id=$1`, target.ID).Scan(&status, &hash, &version, &sessions, &roles, &scopes, &audits); err != nil {
				t.Fatal(err)
			}
			if status != "deleted" || hash != "" || version != target.Version+1 || sessions != 0 || roles != 0 || scopes != 0 || audits != 1 {
				t.Fatal("account was not fully retired", status, version, sessions, roles, scopes, audits)
			}
			if err := f.pool.QueryRow(ctx, `SELECT to_jsonb(m)::text FROM brand_members m WHERE id=$1`, member).Scan(&after); err != nil || before != after {
				t.Fatal("historical business record changed", err)
			}
			list := f.call("GET", path+"?limit=50&offset=0", "", token, brand, nil)
			mustStatus(t, list, 200)
			var page struct {
				Items []adminsys.AdminRecord `json:"items"`
			}
			managedData(t, list, &page)
			for _, item := range page.Items {
				if item.ID == target.ID {
					t.Fatal("deleted account remains in management list")
				}
			}
			mustStatus(t, f.call("POST", loginPath, "history-login-after-delete", "", brand, map[string]string{"identifier": target.Username, "password": password}), 401)
			me := "/api/v1/admin/me"
			if platform {
				me = "/api/v1/platform/me"
			}
			mustStatus(t, f.call("GET", me, "", targetAuth.AccessToken, brand, nil), 401)
			mustStatus(t, f.call("PATCH", endpoint, "history-reactivate", token, brand, map[string]any{"version": version, "status": "active", "role_ids": []string{role}}), 404)
			mustStatus(t, f.call("POST", endpoint+"/reset-password", "history-reset-after-delete", token, brand, map[string]any{"version": version, "password": password}), 404)
			if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='admin.delete.platform'`, globalRole); err != nil {
				t.Fatal(err)
			}
			mustStatus(t, f.call("POST", endpoint+"/delete", "history-delete", token, brand, body), 403)

		})
	}
}
