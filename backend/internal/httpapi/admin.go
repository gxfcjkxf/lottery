package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/tenant"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
	"time"
)

const adminCookie = "lottery_admin"

type platformAdminEntryKey struct{}

func platformAdminEntry(r *http.Request) bool {
	platform, _ := r.Context().Value(platformAdminEntryKey{}).(bool)
	return platform
}

func administrativeCookie(r *http.Request) string {
	if platformAdminEntry(r) {
		return "lottery_platform_admin"
	}
	return adminCookie
}

// Keep only grants owned by the current administrative surface. A misplaced
// platform grant can never widen a brand staff account's scope.
func entryPermissions(a access.Account, platform bool) access.Account {
	scope := access.ScopeBrand
	if platform {
		scope = access.ScopePlatform
	}
	roles := make([]access.Role, 0, len(a.Roles))
	for _, role := range a.Roles {
		grants := []access.Permission{}
		for _, grant := range role.Permissions {
			if grant.Scope == scope {
				grants = append(grants, grant)
			}
		}
		roles = append(roles, access.Role{BrandID: role.BrandID, Permissions: grants})
	}
	a.Roles = roles
	return a
}

func issueAdministrativeCookie(w http.ResponseWriter, r *http.Request, token string, expires time.Time, secure bool) {
	age := int(time.Until(expires).Seconds())
	if token == "" || age <= 0 {
		age = -1
	}
	path := "/api/v1/admin"
	if platformAdminEntry(r) {
		path = "/api/v1/platform"
	}
	http.SetCookie(w, &http.Cookie{Name: administrativeCookie(r), Value: token, Path: path, HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode, Expires: expires, MaxAge: age})
}

func adminReadAudit(w http.ResponseWriter, r *http.Request, d Dependencies, a access.Account, brand, action string) bool {
	tx, err := d.Admins.DB.Begin(r.Context())
	if err == nil {
		defer tx.Rollback(r.Context())
		_, err = audit.Append(r.Context(), tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: action, ResourceType: "query", RequestID: requestID(r), IP: meta(r).IP})
		if err == nil {
			err = tx.Commit(r.Context())
		}
	}
	if err != nil {
		failure(w, r, 503, "SERVICE_UNAVAILABLE", "无法记录查询审计")
		return false
	}
	return true
}

func adminAccount(w http.ResponseWriter, r *http.Request, d Dependencies) (access.Account, bool) {
	id, err := d.Identity.AdminAuthenticate(r.Context(), requestToken(r, administrativeCookie(r)))
	if err != nil {
		failure(w, r, 401, "AUTH_SESSION_REVOKED", "请登录管理账号")
		return access.Account{}, false
	}
	account, err := d.Admins.Account(r.Context(), id)
	if err != nil {
		failure(w, r, 401, "AUTH_SESSION_REVOKED", "管理账号不可用")
		return account, false
	}
	if account.SuperAdmin != platformAdminEntry(r) {
		rejectAdmin(w, r, d, account, "", "admin.entry")
		return access.Account{}, false
	}
	return entryPermissions(account, platformAdminEntry(r)), true
}
func permissionNames(a access.Account) []string {
	out := []string{}
	for _, p := range access.UnionPermissions(a.Roles...) {
		out = append(out, p.Resource+"."+p.Action+"."+string(p.Scope))
	}
	return out
}
func pageParams(r *http.Request) (int, int, bool) {
	limit := 50
	offset := 0
	var err error
	if v := r.URL.Query().Get("limit"); v != "" {
		limit, err = strconv.Atoi(v)
		if err != nil {
			return 0, 0, false
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		offset, err = strconv.Atoi(v)
		if err != nil {
			return 0, 0, false
		}
	}
	return limit, offset, limit > 0 && limit <= 100 && offset >= 0 && offset <= 1000000
}
func adminResult(auditID string, err error) (mutation.Result, error) {
	if errors.Is(err, adminsys.ErrCredentialScope) {
		return mutation.Fail(403, "CREDENTIAL_SCOPE_REQUIRED", "共享密码重置需要该用户所有品牌的重置权限"), nil
	}
	if errors.Is(err, adminsys.ErrDenied) {
		return mutation.Fail(403, "PERMISSION_DENIED", "无操作权限"), nil
	}
	if errors.Is(err, adminsys.ErrNotFound) {
		return mutation.Fail(404, "MEMBER_NOT_FOUND", "品牌成员不存在"), nil
	}
	if errors.Is(err, adminsys.ErrInvalid) {
		return mutation.Fail(400, "REQUEST_INVALID", "操作需要有效状态和原因"), nil
	}
	if err != nil {
		return mutation.Result{}, err
	}
	return mutation.OK(200, map[string]string{"audit_log_id": auditID}), nil
}
func registerAdminRoutes(mux routeRegistrar, d Dependencies) {
	registerAdministrationRoutes(mux, d, false)
	registerAdministrationRoutes(mux, d, true)
}

func registerAdministrationRoutes(mux routeRegistrar, d Dependencies, platform bool) {
	prefix := "/api/v1/admin"
	if platform {
		prefix = "/api/v1/platform"
	}
	handle := func(method, path string, fn http.HandlerFunc) {
		if platform && method != "GET" && path != "/auth/login" && path != "/auth/logout" && !(method == "POST" && path == "/brands") && !platformAccountWriteRoute(method, path) {
			return
		}
		mux.HandleFunc(method+" "+prefix+path, func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(context.WithValue(r.Context(), platformAdminEntryKey{}, platform))
			if d.Identity == nil || d.Mutations == nil || d.Admins.DB == nil {
				failure(w, r, 503, "AUTH_UNAVAILABLE", "后台尚未配置")
				return
			}
			if _, ok := resolveAdminEntry(w, r, d); !ok {
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()
			fn(w, r.WithContext(ctx))
		})
	}
	registerRuleSimulationRoutes(handle, d)
	registerRuleBookRoutes(handle, d)
	registerBrandOperationRoutes(handle, d)
	registerBrandPresentationRoutes(handle, d)
	registerBrandDomainRoutes(handle, d)
	registerBrandCreationRoutes(handle, d)
	registerComplianceRoutes(handle, d)
	registerPeriodRoutes(handle, d)
	RegisterDrawRoutes(handle, d)
	handle("POST", "/auth/login", func(w http.ResponseWriter, r *http.Request) {
		b, ok := resolveAdminEntry(w, r, d)
		if !ok {
			return
		}
		var in identity.LoginInput
		if !decodeBody(w, r, &in) {
			return
		}
		encoded, _ := json.Marshal(in)
		run := func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			allowed, err := d.Identity.AllowAttempt(ctx, d.Mutations, b.ID, "admin:"+in.Identifier, meta(r))
			if err != nil {
				return mutation.Result{}, err
			}
			if !allowed {
				return mutation.Fail(429, "AUTH_RATE_LIMITED", "尝试次数过多"), nil
			}
			return d.Identity.AdminLogin(ctx, tx, b.ID, in, meta(r), platform)
		}
		var result mutation.Result
		var err error
		if b.ID == "" {
			operation := "brand.admin.login"
			if platform {
				operation = "platform.admin.login"
			}
			result, err = d.Mutations.ExecuteChecked(r.Context(), "", "admin-anonymous", operation, r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(string(encoded)), func(ctx context.Context, tx pgx.Tx) error { return checkPlatformAdminEntry(ctx, tx, r) }, run)
		} else {
			result, err = d.Mutations.Execute(r.Context(), b.ID, "admin-anonymous", "admin.login", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(string(encoded)), run)
		}
		if errors.Is(err, tenant.ErrNotFound) {
			result, err = mutation.Fail(404, "BRAND_NOT_FOUND", "平台管理入口已停用"), nil
		}
		if err == nil && result.Error == nil {
			var auth identity.AdminAuthentication
			if json.Unmarshal(result.Data, &auth) == nil {
				issueAdministrativeCookie(w, r, auth.AccessToken, auth.ExpiresAt, d.SecureCookies)
			}
		}
		outputMutation(w, r, result, err)
	})
	handle("POST", "/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		b, _ := resolveAdminEntry(w, r, d)
		var in struct{}
		if !decodeBody(w, r, &in) {
			return
		}
		token := requestToken(r, administrativeCookie(r))
		run := func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			return d.Identity.AdminLogout(ctx, tx, b.ID, token, meta(r), platform)
		}
		var result mutation.Result
		var err error
		if b.ID == "" {
			result, err = d.Mutations.ExecuteChecked(r.Context(), "", "admin-session:"+token, "admin.logout", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint("logout"), func(ctx context.Context, tx pgx.Tx) error { return checkPlatformAdminEntry(ctx, tx, r) }, run)
		} else {
			result, err = d.Mutations.Execute(r.Context(), b.ID, "admin-session:"+token, "admin.logout", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint("logout"), run)
		}
		if errors.Is(err, tenant.ErrNotFound) {
			result, err = mutation.Fail(404, "BRAND_NOT_FOUND", "平台管理入口已停用"), nil
		}
		if err == nil && result.Error == nil {
			issueAdministrativeCookie(w, r, "", time.Unix(1, 0), d.SecureCookies)
		}
		outputMutation(w, r, result, err)
	})
	handle("GET", "/me", func(w http.ResponseWriter, r *http.Request) {
		account, ok := adminAccount(w, r, d)
		if !ok {
			return
		}
		brands := account.BrandIDs
		if brands == nil {
			brands = []string{}
		}
		if !adminReadAudit(w, r, d, account, "", "admin.me") {
			return
		}
		respond(w, r, 200, map[string]any{"account": map[string]any{"id": account.ID, "super_admin": account.SuperAdmin, "brand_ids": brands, "permissions": permissionNames(account), "version": account.Version, "permissions_by_brand": permissionsByBrand(account), "platform_permissions": platformPermissions(account)}})
	})
	handle("GET", "/brands", func(w http.ResponseWriter, r *http.Request) {
		a, ok := adminAccount(w, r, d)
		if !ok {
			return
		}
		platform := access.Authorize(a, "brand", "view", access.ScopePlatform, "")
		rows, err := d.Admins.DB.Query(r.Context(), `SELECT id::text,code,name,status FROM brands WHERE $1 OR id IN(SELECT brand_id FROM admin_brand_scopes WHERE account_id=$2) ORDER BY code`, platform, a.ID)
		if err != nil {
			failure(w, r, 503, "SERVICE_UNAVAILABLE", "暂无法查询品牌")
			return
		}
		defer rows.Close()
		items := []map[string]string{}
		for rows.Next() {
			var id, code, name, status string
			if rows.Scan(&id, &code, &name, &status) != nil {
				failure(w, r, 503, "SERVICE_UNAVAILABLE", "查询失败")
				return
			}
			if !platform && !access.Authorize(a, "brand", "view", access.ScopeBrand, id) {
				continue
			}
			items = append(items, map[string]string{"id": id, "code": code, "name": name, "status": status})
		}
		if rows.Err() != nil {
			failure(w, r, 503, "SERVICE_UNAVAILABLE", "查询失败")
			return
		}
		rows.Close()
		if !adminReadAudit(w, r, d, a, "", "brand.view") {
			return
		}
		respond(w, r, 200, map[string]any{"items": items})
	})
	handle("GET", "/users", func(w http.ResponseWriter, r *http.Request) {
		a, ok := adminAccount(w, r, d)
		if !ok {
			return
		}
		brand := r.Header.Get("X-Brand-ID")
		if brand != "" && !uuidPattern.MatchString(brand) {
			failure(w, r, 400, "REQUEST_INVALID", "品牌编号不正确")
			return
		}
		limit, offset, ok := pageParams(r)
		if !ok {
			failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
			return
		}
		items, err := d.Admins.ListMembers(r.Context(), a, brand, limit, offset)
		if err != nil {
			if errors.Is(err, adminsys.ErrDenied) {
				rejectAdmin(w, r, d, a, brand, "user.view")
				return
			}
			result, e := adminResult("", err)
			outputMutation(w, r, result, e)
			return
		}
		if !adminReadAudit(w, r, d, a, brand, "user.view") {
			return
		}
		respond(w, r, 200, map[string]any{"items": items})
	})
	type memberInput struct {
		Status   string `json:"status"`
		Notes    string `json:"notes"`
		Reason   string `json:"reason"`
		Password string `json:"password,omitempty"`
	}
	for _, operation := range []string{"write", "kick", "password_reset"} {
		method, path := "POST", "/users/{id}/kick"
		if operation == "write" {
			method, path = "PATCH", "/users/{id}"
		}
		if operation == "password_reset" {
			path = "/users/{id}/reset-password"
		}
		handle(method, path, func(w http.ResponseWriter, r *http.Request) {
			a, ok := adminAccount(w, r, d)
			if !ok {
				return
			}
			brand, id := r.Header.Get("X-Brand-ID"), r.PathValue("id")
			if !uuidPattern.MatchString(brand) || !uuidPattern.MatchString(id) {
				failure(w, r, 400, "REQUEST_INVALID", "品牌或成员编号不正确")
				return
			}
			var in memberInput
			if !decodeBody(w, r, &in) {
				return
			}
			if len(in.Notes) > 2000 || len(in.Reason) > 500 || len(in.Reason) == 0 {
				failure(w, r, 400, "REQUEST_INVALID", "原因必填且不超过 500 字符，备注不超过 2000 字符")
				return
			}
			if !access.Authorize(a, "user", operation, access.ScopeBrand, brand) {
				rejectAdmin(w, r, d, a, brand, "user."+operation)
				return
			}
			encoded, _ := json.Marshal(in)
			result, err := d.Mutations.Execute(r.Context(), brand, a.ID, "admin.user."+operation, r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(id+":"+string(encoded)), func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
				fresh, err := freshAdmin(ctx, tx, r, d, a, false)
				if errors.Is(err, identity.ErrSession) || errors.Is(err, adminsys.ErrDenied) {
					return mutation.Fail(401, "AUTH_SESSION_REVOKED", "请重新登录"), nil
				}
				if err != nil {
					return mutation.Result{}, err
				}
				var auditID string
				switch operation {
				case "write":
					auditID, err = d.Admins.ChangeMember(ctx, tx, fresh, brand, id, in.Status, in.Notes, in.Reason, requestID(r), meta(r).IP)
				case "kick":
					auditID, err = d.Admins.Kick(ctx, tx, fresh, brand, id, in.Reason, requestID(r), meta(r).IP)
				case "password_reset":
					var hash string
					hash, err = d.Identity.PasswordHash(ctx, in.Password)
					if err != nil {
						return mutation.Fail(400, "PASSWORD_INVALID", "密码须为 10-128 字节"), nil
					}
					auditID, err = d.Admins.ResetPassword(ctx, tx, fresh, brand, id, hash, in.Reason, requestID(r), meta(r).IP)
				}
				return adminResult(auditID, err)
			})
			finishAdminMutation(w, r, d, a, brand, "user."+operation, result, err)
		})
	}
	registerAuditRoutes(handle, d)
	registerCommissionAnalysisReportRoutes(handle, d)
	registerAdminManagementRoutes(handle, d)
	if platform {
		registerPlatformAccountRoutes(handle, d)
	}
	registerPointAdminRoutes(handle, d)
	registerBetAdminRoutes(handle, d)
}
