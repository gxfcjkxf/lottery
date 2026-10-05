package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/tenant"
	"github.com/jackc/pgx/v5"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func userCookieName(brand string) string { return "lottery_user_" + strings.ReplaceAll(brand, "-", "") }
func requestToken(r *http.Request, cookieName string) string {
	if header := r.Header.Get("Authorization"); header != "" {
		parts := strings.Fields(header)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return parts[1]
		}
		return ""
	}
	if cookie, err := r.Cookie(cookieName); err == nil {
		return cookie.Value
	}
	return ""
}
func safeOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return r.Header.Get("Cookie") == ""
	}
	u, err := url.Parse(origin)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	// HTTPS reverse proxies must use a configured same-origin public host and
	// preserve it. Do not trust X-Forwarded-Host or arbitrary forwarded schemes.
	return strings.EqualFold(u.Host, r.Host) && (u.Scheme == scheme || (u.Scheme == "https" && scheme == "http"))
}
func decodeBody(w http.ResponseWriter, r *http.Request, destination any) bool {
	if !safeOrigin(r) {
		failure(w, r, 403, "CSRF_REJECTED", "请求来源不可信")
		return false
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		failure(w, r, 415, "CONTENT_TYPE_INVALID", "需要 application/json")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(destination); err != nil {
		failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
		return false
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		failure(w, r, 400, "REQUEST_INVALID", "请求只能包含一个 JSON 对象")
		return false
	}
	return true
}
func resolveBrand(w http.ResponseWriter, r *http.Request, d Dependencies) (tenant.Brand, bool) {
	b, err := d.Brands.Resolve(r.Context(), r.Host, r.PathValue("brandCode"))
	if errors.Is(err, tenant.ErrNotFound) {
		failure(w, r, 404, "BRAND_NOT_FOUND", "当前入口没有可用品牌")
		return b, false
	}
	if err != nil {
		failure(w, r, 503, "SERVICE_UNAVAILABLE", "暂时无法加载品牌")
		return b, false
	}
	return b, true
}
func meta(r *http.Request) identity.Metadata {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	if trusted, ok := r.Context().Value(clientIPKey{}).(string); ok {
		ip = trusted
	}
	return identity.Metadata{RequestID: requestID(r), IP: ip, Domain: tenant.NormalizeHost(r.Host)}
}
func outputMutation(w http.ResponseWriter, r *http.Request, result mutation.Result, err error) {
	if err != nil {
		failure(w, r, 503, "SERVICE_UNAVAILABLE", "服务暂时不可用，请稍后重试")
		return
	}
	if result.Error != nil {
		failure(w, r, result.Status, result.Error.Code, result.Error.Message)
		return
	}
	respond(w, r, result.Status, result.Data)
}
func issueCookie(w http.ResponseWriter, r *http.Request, name, token string, expires time.Time, secure bool) {
	age := int(time.Until(expires).Seconds())
	if token == "" || age <= 0 {
		age = -1
	}
	http.SetCookie(w, &http.Cookie{Name: name, Value: token, Path: "/api/v1", HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode, Expires: expires, MaxAge: age})
}
func registerAuthRoutes(mux *http.ServeMux, d Dependencies) {
	for _, prefix := range []string{"/api/v1", "/api/v1/b/{brandCode}"} {
		handle := func(method, path string, fn http.HandlerFunc) {
			mux.HandleFunc(method+" "+prefix+path, func(w http.ResponseWriter, r *http.Request) {
				if d.Identity == nil || d.Mutations == nil {
					failure(w, r, 503, "AUTH_UNAVAILABLE", "认证尚未配置")
					return
				}
				ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
				defer cancel()
				fn(w, r.WithContext(ctx))
			})
		}
		handle("GET", "/me", func(w http.ResponseWriter, r *http.Request) {
			b, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			v, err := d.Identity.Authenticate(r.Context(), b.ID, requestToken(r, userCookieName(b.ID)))
			if err == identity.ErrSession {
				failure(w, r, 401, "AUTH_SESSION_REVOKED", "请重新登录")
				return
			}
			if err != nil {
				failure(w, r, 503, "SERVICE_UNAVAILABLE", "服务暂不可用")
				return
			}
			respond(w, r, 200, v.View)
		})
		for _, kind := range []string{"register", "login"} {
			handle("POST", "/auth/"+kind, func(w http.ResponseWriter, r *http.Request) {
				b, ok := resolveBrand(w, r, d)
				if !ok {
					return
				}
				var input any
				var identifier, captchaID, captchaAnswer string
				if kind == "register" {
					in := identity.RegisterInput{}
					if !decodeBody(w, r, &in) {
						return
					}
					input = in
					identifier = strings.ToLower(strings.TrimSpace(in.Username)) + ":" + strings.TrimSpace(in.Phone)
					captchaID = in.CaptchaID
					captchaAnswer = in.CaptchaAnswer
				} else {
					in := identity.LoginInput{}
					if !decodeBody(w, r, &in) {
						return
					}
					input = in
					identifier = strings.ToLower(strings.TrimSpace(in.Identifier))
					captchaID = in.CaptchaID
					captchaAnswer = in.CaptchaAnswer
				}
				encoded, _ := json.Marshal(input)
				result, err := d.Mutations.Execute(r.Context(), b.ID, "anonymous", "auth."+kind, r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(string(encoded)), func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
					allowed, err := d.Identity.AllowAttempt(ctx, d.Mutations, b.ID, identifier, meta(r))
					if err != nil {
						return mutation.Result{}, err
					}
					if !allowed {
						return mutation.Fail(429, "AUTH_RATE_LIMITED", "尝试次数过多，请稍后再试"), nil
					}
					cfg, err := d.Identity.Settings(ctx, b.ID)
					if err != nil {
						return mutation.Result{}, err
					}
					if cfg.CaptchaEnabled {
						valid, err := d.Identity.Consume(ctx, d.Mutations, b.ID, "captcha", captchaID, captchaAnswer)
						if err != nil {
							return mutation.Result{}, err
						}
						if !valid {
							return mutation.Fail(400, "CAPTCHA_INVALID", "验证码错误或已失效"), nil
						}
					}
					if kind == "register" {
						return d.Identity.Register(ctx, tx, b.ID, input.(identity.RegisterInput), meta(r))
					}
					return d.Identity.Login(ctx, tx, b.ID, input.(identity.LoginInput), meta(r))
				})
				if err == nil && result.Error == nil {
					var auth identity.Authentication
					if json.Unmarshal(result.Data, &auth) == nil {
						issueCookie(w, r, userCookieName(b.ID), auth.AccessToken, auth.ExpiresAt, d.SecureCookies)
					}
				}
				outputMutation(w, r, result, err)
			})
		}
		handle("POST", "/auth/logout", func(w http.ResponseWriter, r *http.Request) {
			b, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			var body struct{}
			if !decodeBody(w, r, &body) {
				return
			}
			token := requestToken(r, userCookieName(b.ID))
			result, err := d.Mutations.Execute(r.Context(), b.ID, "session:"+token, "auth.logout", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint("logout"), func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
				return d.Identity.Logout(ctx, tx, b.ID, token, meta(r))
			})
			if err == nil && result.Error == nil {
				issueCookie(w, r, userCookieName(b.ID), "", time.Unix(1, 0), d.SecureCookies)
			}
			outputMutation(w, r, result, err)
		})
		handle("PATCH", "/me/profile", func(w http.ResponseWriter, r *http.Request) {
			b, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			var in identity.ProfileInput
			if !decodeBody(w, r, &in) {
				return
			}
			token := requestToken(r, userCookieName(b.ID))
			v, err := d.Identity.Authenticate(r.Context(), b.ID, token)
			if err != nil {
				failure(w, r, 401, "AUTH_SESSION_REVOKED", "请重新登录")
				return
			}
			encoded, _ := json.Marshal(in)
			result, err := d.Mutations.Execute(r.Context(), b.ID, v.User.ID, "user.profile_fill", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(string(encoded)), func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
				// Revalidate immediately before the write; revoked sessions never regain access.
				current, err := d.Identity.Authenticate(ctx, b.ID, token)
				if err != nil || current.User.ID != v.User.ID {
					return mutation.Fail(401, "AUTH_SESSION_REVOKED", "请重新登录"), nil
				}
				return d.Identity.FillProfile(ctx, tx, b.ID, v.User.ID, in, meta(r))
			})
			outputMutation(w, r, result, err)
		})
		handle("GET", "/auth/challenge", func(w http.ResponseWriter, r *http.Request) {
			b, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			allowed, err := d.Identity.AllowAttempt(r.Context(), d.Mutations, b.ID, "", meta(r))
			if err != nil || !allowed {
				failure(w, r, 429, "AUTH_RATE_LIMITED", "请求过于频繁")
				return
			}
			challenge, err := d.Identity.Challenge(r.Context(), d.Mutations, b.ID, "captcha")
			if err != nil {
				failure(w, r, 503, "SERVICE_UNAVAILABLE", "验证码暂不可用")
				return
			}
			respond(w, r, 200, challenge)
		})
	}
}
