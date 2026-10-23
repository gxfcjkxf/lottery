package httpapi

import (
	"net/http"
	"strings"
)

type sessionIdentityKey struct{}

// Preserve the replacement reason for every endpoint rejecting an old session.
func sessionFailure(r *http.Request, code, message string) (string, string) {
	d, ok := r.Context().Value(sessionIdentityKey{}).(Dependencies)
	if !ok || d.Identity == nil {
		return code, message
	}
	var token, brand string
	admin := strings.HasPrefix(r.URL.Path, "/api/v1/admin/") || strings.HasPrefix(r.URL.Path, "/api/v1/platform/")
	if admin {
		token = requestToken(r, administrativeCookie(r))
	} else {
		b, err := d.Brands.Resolve(r.Context(), r.Host, r.PathValue("brandCode"))
		if err != nil {
			return code, message
		}
		brand = b.ID
		token = requestToken(r, userCookieName(brand))
	}
	replaced, err := d.Identity.SessionReplaced(r.Context(), token, brand, admin)
	if err != nil {
		return "SERVICE_UNAVAILABLE", "服务暂时不可用，请稍后重试"
	}
	if replaced {
		return "AUTH_SESSION_REPLACED", "账号有新的登录，当前会话已失效，请重新登录"
	}
	return code, message
}
