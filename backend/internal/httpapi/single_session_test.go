package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/identity"
)

func TestAdministrativeSingleSessionReplacementReason(t *testing.T) {
	for _, platform := range []bool{false, true} {
		prefix := "/api/v1/admin"
		if platform {
			prefix = "/api/v1/platform"
		}
		t.Run(prefix, func(t *testing.T) {
			f := managedFixture(t)
			if platform {
				if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
					t.Fatal(err)
				}
				grantPlatformPermission(t, f, "brand.view.platform")
			}
			login := func(key string) *httptest.ResponseRecorder {
				return f.call("POST", prefix+"/auth/login", key, "", "", map[string]string{"identifier": "managed_root", "password": "root-test-password-2026"})
			}
			first := login("single-login-first")
			mustStatus(t, first, 200)
			var a identity.AdminAuthentication
			managedData(t, first, &a)

			// Failed credentials do not replace the current session, even with its cookie/token.
			failed := f.call("POST", prefix+"/auth/login", "single-login-invalid", a.AccessToken, "", map[string]string{"identifier": "managed_root", "password": "incorrect-password"})
			mustStatus(t, failed, http.StatusUnauthorized)
			if !strings.Contains(failed.Body.String(), "AUTH_INVALID_CREDENTIALS") {
				t.Fatal("invalid password incorrectly reported replacement", failed.Body.String())
			}
			mustStatus(t, f.call("GET", prefix+"/me", "", a.AccessToken, "", nil), 200)
			next := login("single-login-next")
			mustStatus(t, next, 200)
			var b identity.AdminAuthentication
			managedData(t, next, &b)
			old := f.call("GET", prefix+"/me", "", a.AccessToken, "", nil)
			mustStatus(t, old, 401)
			if !strings.Contains(old.Body.String(), "AUTH_SESSION_REPLACED") {
				t.Fatal(old.Body.String())
			}
			request := httptest.NewRequest("GET", "http://localhost"+prefix+"/me", nil)
			request.AddCookie(first.Result().Cookies()[0])
			w := httptest.NewRecorder()
			f.http.ServeHTTP(w, request)
			if w.Code != 401 || !strings.Contains(w.Body.String(), "AUTH_SESSION_REPLACED") {
				t.Fatal("cookie replacement reason", w.Code, w.Body.String())
			}
			// Replaying a prior login returns its old token, without reviving it or kicking the latest login.
			mustStatus(t, login("single-login-first"), 200)
			mustStatus(t, f.call("GET", prefix+"/me", "", b.AccessToken, "", nil), 200)
			mustStatus(t, f.call("POST", prefix+"/auth/logout", "single-old-logout", a.AccessToken, "", map[string]string{}), 200)
			mustStatus(t, f.call("GET", prefix+"/me", "", b.AccessToken, "", nil), 200)
			mustStatus(t, f.call("POST", prefix+"/auth/logout", "single-next-logout", b.AccessToken, "", map[string]string{}), 200)
			expired := f.call("GET", prefix+"/me", "", b.AccessToken, "", nil)
			mustStatus(t, expired, http.StatusUnauthorized)
			if !strings.Contains(expired.Body.String(), "AUTH_SESSION_REVOKED") {
				t.Fatal("logout incorrectly reported replacement", expired.Body.String())
			}
			// Natural expiry and unknown tokens are ordinary invalid sessions, not new sign-ins.
			third := login("single-login-expiring")
			mustStatus(t, third, 200)
			var c identity.AdminAuthentication
			managedData(t, third, &c)
			if _, err := f.pool.Exec(context.Background(), `UPDATE sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE admin_id=$1 AND revoked_at IS NULL`, f.root); err != nil {
				t.Fatal(err)
			}
			for _, token := range []string{c.AccessToken, strings.Repeat("x", 43), ""} {
				response := f.call("GET", prefix+"/me", "", token, "", nil)
				mustStatus(t, response, http.StatusUnauthorized)
				if !strings.Contains(response.Body.String(), "AUTH_SESSION_REVOKED") || strings.Contains(response.Body.String(), "AUTH_SESSION_REPLACED") {
					t.Fatal("ordinary invalid session incorrectly reported replacement", response.Body.String())
				}
			}

		})
	}
}
