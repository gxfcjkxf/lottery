package httpapi

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/authcrypto"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"strings"
	"testing"
)

func TestAdministrativePasswordChangeChecksSessionAndRevokesOldCredentials(t *testing.T) {
	for _, platform := range []bool{false, true} {
		name := "brand"
		if platform {
			name = "platform"
		}
		t.Run(name, func(t *testing.T) {
			f := managedFixture(t)
			ctx := context.Background()
			prefix, username, oldPassword, actorID := "/api/v1/admin", "managed_root", "root-test-password-2026", f.root
			if platform {
				prefix, username, oldPassword, actorID = "/api/v1/platform", "password_platform", "platform-old-pass-2026", ids.New()
				hash, err := authcrypto.HashPassword(oldPassword)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = f.pool.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash,is_super_admin) VALUES($1,$2,$3,true)`, actorID, username, hash); err != nil {
					t.Fatal(err)
				}
			}
			login := f.call("POST", prefix+"/auth/login", "password-login-initial", "", managedBrand, map[string]string{"identifier": username, "password": oldPassword})
			mustStatus(t, login, 200)
			var auth identity.AdminAuthentication
			managedData(t, login, &auth)
			token := auth.AccessToken
			newPassword := "changed-test-pass-2026"
			body := map[string]string{"current_password": oldPassword, "new_password": newPassword}
			result := f.call("POST", prefix+"/me/password", "password-change-001", token, "", body)
			mustStatus(t, result, 200)
			cookies := result.Result().Cookies()
			expectedCookie := adminCookie
			if platform {
				expectedCookie = "lottery_platform_admin"
			}
			if len(cookies) != 1 || cookies[0].Name != expectedCookie || cookies[0].MaxAge >= 0 {
				t.Fatal("changed password must expire the matching cookie", cookies)
			}
			mustStatus(t, f.call("GET", prefix+"/me", "", token, "", nil), 401)
			mustStatus(t, f.call("POST", prefix+"/me/password", "password-change-001", token, "", body), 401)
			mustStatus(t, f.call("POST", prefix+"/auth/login", "password-old-denied", "", managedBrand, map[string]string{"identifier": username, "password": oldPassword}), 401)
			mustStatus(t, f.call("POST", prefix+"/auth/login", "password-new-accepted", "", managedBrand, map[string]string{"identifier": username, "password": newPassword}), 200)
			var count int
			var evidence string
			if err := f.pool.QueryRow(ctx, `SELECT count(*),min(before_json::text||after_json::text) FROM audit_logs WHERE actor_id=$1 AND action='admin.password_change'`, actorID).Scan(&count, &evidence); err != nil {
				t.Fatal(err)
			}
			if count != 1 || !strings.Contains(evidence, "redacted") || strings.Contains(evidence, oldPassword) || strings.Contains(evidence, newPassword) {
				t.Fatal("password audit must be singular and redacted", count, evidence)
			}
		})
	}
}
