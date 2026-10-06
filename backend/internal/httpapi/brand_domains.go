package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/branddomains"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
)

func domainActor(w http.ResponseWriter, r *http.Request, d Dependencies, action string) (access.Account, string, bool) {
	a, ok := adminAccount(w, r, d)
	if !ok {
		return a, "", false
	}
	b := strings.ToLower(r.Header.Get("X-Brand-ID"))
	if !uuidPattern.MatchString(b) {
		failure(w, r, 400, "REQUEST_INVALID", "必须选择有效品牌")
		return a, b, false
	}
	if !branddomains.Allowed(a, b, action) {
		rejectAdmin(w, r, d, a, b, "brand_domains."+action)
		return a, b, false
	}
	return a, b, true
}
func domainResult(out any, e error, status int) (mutation.Result, error) {
	switch {
	case errors.Is(e, branddomains.ErrInvalid):
		return mutation.Fail(400, "BRAND_DOMAIN_INPUT_INVALID", "域名、主域名状态、版本或原因不正确"), nil
	case errors.Is(e, branddomains.ErrDenied):
		return mutation.Fail(403, "PERMISSION_DENIED", "无品牌域名管理权限"), nil
	case errors.Is(e, branddomains.ErrNotFound):
		return mutation.Fail(404, "BRAND_DOMAIN_NOT_FOUND", "当前品牌未找到域名绑定"), nil
	case errors.Is(e, branddomains.ErrVersion):
		return mutation.Fail(409, "BRAND_DOMAIN_VERSION_CONFLICT", "品牌配置版本已变化，请重新核对"), nil
	case errors.Is(e, branddomains.ErrState):
		return mutation.Fail(409, "BRAND_DOMAIN_STATE_CONFLICT", "停用品牌只读，或绑定状态没有变化"), nil
	case errors.Is(e, branddomains.ErrConflict):
		return mutation.Fail(409, "BRAND_DOMAIN_CONFLICT", "域名已保留或品牌绑定数量已达上限"), nil
	case e != nil:
		return mutation.Result{}, e
	default:
		return mutation.OK(status, out), nil
	}
}
func registerBrandDomainRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := branddomains.Service{DB: d.Admins.DB}
	for _, history := range []bool{false, true} {
		path := "/brand-domains"
		if history {
			path += "/history"
		}
		handle("GET", path, func(w http.ResponseWriter, r *http.Request) {
			a, b, ok := domainActor(w, r, d, "view")
			if !ok {
				return
			}
			var out any
			var e error
			if history {
				limit, offset, ok := brandOperationPage(r)
				if !ok {
					failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
					return
				}
				out, e = auditedHistory(w, r, d, a, b, "brand_domains.history", func(fresh access.Account) bool {
					return branddomains.Allowed(fresh, b, "view")
				}, func(tx pgx.Tx) (any, error) {
					rows, err := s.HistoryTx(r.Context(), tx, b, limit, offset)
					return map[string]any{"items": rows, "limit": limit, "offset": offset}, err
				})
				if historyFailure(w, r, e) {
					return
				}
				result, err := domainResult(out, e, 200)
				outputMutation(w, r, result, err)
				return
			} else {
				if r.URL.RawQuery != "" {
					failure(w, r, 400, "REQUEST_INVALID", "此接口不接受查询参数")
					return
				}
				out, e = s.Read(r.Context(), b)
			}
			if e == nil && !adminReadAudit(w, r, d, a, b, "brand_domains.view") {
				return
			}
			result, err := domainResult(out, e, 200)
			outputMutation(w, r, result, err)
		})
	}
	for _, create := range []bool{true, false} {
		method, path, status := "PATCH", "/brand-domains/{domainID}", 200
		if create {
			method, path, status = "POST", "/brand-domains", 201
		}
		handle(method, path, func(w http.ResponseWriter, r *http.Request) {
			a, b, ok := domainActor(w, r, d, "write")
			if !ok {
				return
			}
			if r.URL.RawQuery != "" {
				failure(w, r, 400, "REQUEST_INVALID", "此接口不接受查询参数")
				return
			}
			var in branddomains.Input
			if !decodeBody(w, r, &in) {
				return
			}
			target := r.PathValue("domainID")
			if create && !branddomains.ValidDomain(in.Domain) || !create && (in.Domain != "" || !uuidPattern.MatchString(target)) {
				failure(w, r, 400, "BRAND_DOMAIN_INPUT_INVALID", "创建仅接受规范域名；已有绑定不可改名")
				return
			}
			raw, e := json.Marshal(in)
			if e != nil {
				failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
				return
			}
			var fresh access.Account
			result, e := d.Mutations.ExecuteChecked(r.Context(), b, a.ID, "brand_domains."+method+":"+target, r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(string(raw)), func(ctx context.Context, tx pgx.Tx) error {
				var e error
				fresh, e = freshAdmin(ctx, tx, r, d, a, false)
				if errors.Is(e, adminsys.ErrDenied) || errors.Is(e, identity.ErrSession) {
					return identity.ErrSession
				}
				if e != nil {
					return e
				}
				if !branddomains.Allowed(fresh, b, "write") {
					return branddomains.ErrDenied
				}
				var state string
				if e = tx.QueryRow(ctx, `SELECT status FROM brands WHERE id=$1 FOR UPDATE`, b).Scan(&state); errors.Is(e, pgx.ErrNoRows) {
					return branddomains.ErrNotFound
				} else if e != nil {
					return e
				}
				if state == "disabled" {
					return branddomains.ErrState
				}
				return nil
			}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
				out, e := s.Change(ctx, tx, b, fresh, target, in, pointMeta(r, fresh))
				return domainResult(out, e, status)
			})
			if errors.Is(e, identity.ErrSession) {
				result, e = mutation.Fail(401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录"), nil
			} else if errors.Is(e, branddomains.ErrDenied) || errors.Is(e, branddomains.ErrState) || errors.Is(e, branddomains.ErrNotFound) {
				result, e = domainResult(nil, e, status)
			}
			finishAdminMutation(w, r, d, a, b, "brand_domains.write", result, e)
		})
	}
}
