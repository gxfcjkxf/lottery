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
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
	"time"
)

const adminCookie = "lottery_admin"

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
	id, err := d.Identity.AdminAuthenticate(r.Context(), requestToken(r, adminCookie))
	if err != nil {
		failure(w, r, 401, "AUTH_SESSION_REVOKED", "请登录管理账号")
		return access.Account{}, false
	}
	account, err := d.Admins.Account(r.Context(), id)
	if err != nil {
		failure(w, r, 401, "AUTH_SESSION_REVOKED", "管理账号不可用")
		return account, false
	}
	return account, true
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
func registerAdminRoutes(mux *http.ServeMux, d Dependencies) {
	handle := func(method, path string, fn http.HandlerFunc) {
		mux.HandleFunc(method+" /api/v1/admin"+path, func(w http.ResponseWriter, r *http.Request) {
			if d.Identity == nil || d.Mutations == nil || d.Admins.DB == nil {
				failure(w, r, 503, "AUTH_UNAVAILABLE", "后台尚未配置")
				return
			}
			if _, ok := resolveBrand(w, r, d); !ok {
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()
			fn(w, r.WithContext(ctx))
		})
	}
	registerRuleSimulationRoutes(handle, d)
	registerRuleBookRoutes(handle, d)
	registerPeriodRoutes(handle, d)
	RegisterDrawRoutes(handle, d)
	handle("POST", "/auth/login", func(w http.ResponseWriter, r *http.Request) {
		b, ok := resolveBrand(w, r, d)
		if !ok {
			return
		}
		var in identity.LoginInput
		if !decodeBody(w, r, &in) {
			return
		}
		encoded, _ := json.Marshal(in)
		result, err := d.Mutations.Execute(r.Context(), b.ID, "admin-anonymous", "admin.login", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(string(encoded)), func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			allowed, err := d.Identity.AllowAttempt(ctx, d.Mutations, b.ID, "admin:"+in.Identifier, meta(r))
			if err != nil {
				return mutation.Result{}, err
			}
			if !allowed {
				return mutation.Fail(429, "AUTH_RATE_LIMITED", "尝试次数过多"), nil
			}
			return d.Identity.AdminLogin(ctx, tx, b.ID, in, meta(r))
		})
		if err == nil && result.Error == nil {
			var auth identity.AdminAuthentication
			if json.Unmarshal(result.Data, &auth) == nil {
				issueCookie(w, r, adminCookie, auth.AccessToken, auth.ExpiresAt, d.SecureCookies)
			}
		}
		outputMutation(w, r, result, err)
	})
	handle("POST", "/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		b, _ := resolveBrand(w, r, d)
		var in struct{}
		if !decodeBody(w, r, &in) {
			return
		}
		token := requestToken(r, adminCookie)
		result, err := d.Mutations.Execute(r.Context(), b.ID, "admin-session:"+token, "admin.logout", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint("logout"), func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			return d.Identity.AdminLogout(ctx, tx, b.ID, token, meta(r))
		})
		if err == nil && result.Error == nil {
			issueCookie(w, r, adminCookie, "", time.Unix(1, 0), d.SecureCookies)
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
	handle("GET", "/audit", func(w http.ResponseWriter, r *http.Request) {
		a, ok := adminAccount(w, r, d)
		if !ok {
			return
		}
		brand := r.Header.Get("X-Brand-ID")
		platform := access.Authorize(a, "audit", "view", access.ScopePlatform, "")
		if (brand != "" && !uuidPattern.MatchString(brand)) || (!platform && !access.Authorize(a, "audit", "view", access.ScopeBrand, brand)) {
			rejectAdmin(w, r, d, a, brand, "audit.view")
			return
		}
		limit, offset, ok := pageParams(r)
		if !ok {
			failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
			return
		}
		rows, err := d.Admins.DB.Query(r.Context(), `SELECT id::text,action,actor_type,coalesce(actor_id::text,''),resource_type,coalesce(resource_id::text,''),reason,request_id,created_at,coalesce(ip_address,''),coalesce(before_json,'null'::jsonb),coalesce(after_json,'null'::jsonb) FROM audit_logs WHERE ($1='' OR brand_id=NULLIF($1,'')::uuid) ORDER BY created_at DESC,id DESC LIMIT $2 OFFSET $3`, brand, limit, offset)
		if err != nil {
			failure(w, r, 503, "SERVICE_UNAVAILABLE", "审计查询失败")
			return
		}
		defer rows.Close()
		items := []map[string]any{}
		for rows.Next() {
			var id, action, actorType, actorID, resource, resourceID, reason, req, ip string
			var when time.Time
			var before, after json.RawMessage
			if err = rows.Scan(&id, &action, &actorType, &actorID, &resource, &resourceID, &reason, &req, &when, &ip, &before, &after); err != nil {
				failure(w, r, 503, "SERVICE_UNAVAILABLE", "审计查询失败")
				return
			}
			items = append(items, map[string]any{"id": id, "action": action, "actor_type": actorType, "actor_id": actorID, "resource_type": resource, "resource_id": resourceID, "reason": reason, "request_id": req, "created_at": when, "ip_address": ip, "before_json": before, "after_json": after})
		}
		if rows.Err() != nil {
			failure(w, r, 503, "SERVICE_UNAVAILABLE", "审计查询失败")
			return
		}
		tx, err := d.Admins.DB.Begin(r.Context())
		if err == nil {
			defer tx.Rollback(r.Context())
			_, err = audit.Append(r.Context(), tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: "audit.view", ResourceType: "audit", RequestID: requestID(r), IP: meta(r).IP})
			if err == nil {
				err = tx.Commit(r.Context())
			}
		}
		if err != nil {
			failure(w, r, 503, "SERVICE_UNAVAILABLE", "审计记录失败")
			return
		}
		respond(w, r, 200, map[string]any{"items": items})
	})
	registerAdminManagementRoutes(handle, d)
	registerPointAdminRoutes(handle, d)
	registerBetAdminRoutes(handle, d)
}
