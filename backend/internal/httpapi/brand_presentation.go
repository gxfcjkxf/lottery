package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/brandskin"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
)

func presentationActor(w http.ResponseWriter, r *http.Request, d Dependencies, action string) (access.Account, string, bool) {
	a, ok := adminAccount(w, r, d)
	if !ok {
		return a, "", false
	}
	b := strings.ToLower(r.Header.Get("X-Brand-ID"))
	if !uuidPattern.MatchString(b) {
		failure(w, r, 400, "REQUEST_INVALID", "必须选择有效品牌")
		return a, b, false
	}
	if !brandskin.Allowed(a, b, action) {
		rejectAdmin(w, r, d, a, b, "brand_presentation."+action)
		return a, b, false
	}
	return a, b, true
}
func presentationResult(out any, e error) (mutation.Result, error) {
	switch {
	case errors.Is(e, brandskin.ErrInvalid):
		return mutation.Fail(400, "BRAND_PRESENTATION_INPUT_INVALID", "展示配置、语言、素材地址或原因不正确"), nil
	case errors.Is(e, brandskin.ErrDenied):
		return mutation.Fail(403, "PERMISSION_DENIED", "无品牌展示配置权限"), nil
	case errors.Is(e, brandskin.ErrNotFound):
		return mutation.Fail(404, "BRAND_PRESENTATION_NOT_FOUND", "品牌展示配置不存在"), nil
	case errors.Is(e, brandskin.ErrVersion):
		return mutation.Fail(409, "BRAND_PRESENTATION_VERSION_CONFLICT", "品牌配置版本已变化，请重新读取并核对"), nil
	case errors.Is(e, brandskin.ErrState):
		return mutation.Fail(409, "BRAND_PRESENTATION_STATE_CONFLICT", "停用品牌的展示配置只读"), nil
	case e != nil:
		return mutation.Result{}, e
	default:
		return mutation.OK(200, out), nil
	}
}
func registerBrandPresentationRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := brandskin.Service{DB: d.Admins.DB}
	for _, history := range []bool{false, true} {
		path := "/brand-presentation"
		if history {
			path += "/history"
		}
		handle("GET", path, func(w http.ResponseWriter, r *http.Request) {
			a, b, ok := presentationActor(w, r, d, "view")
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
				out, e = auditedHistory(w, r, d, a, b, "brand_presentation.history", func(fresh access.Account) bool {
					return brandskin.Allowed(fresh, b, "view")
				}, func(tx pgx.Tx) (any, error) {
					items, err := s.HistoryTx(r.Context(), tx, b, limit, offset)
					return map[string]any{"items": items, "limit": limit, "offset": offset}, err
				})
				if historyFailure(w, r, e) {
					return
				}
				result, err := presentationResult(out, e)
				outputMutation(w, r, result, err)
				return
			} else {
				if r.URL.RawQuery != "" {
					failure(w, r, 400, "REQUEST_INVALID", "此接口不接受查询参数")
					return
				}
				out, e = s.Read(r.Context(), b)
			}
			if e == nil && !adminReadAudit(w, r, d, a, b, "brand_presentation.view") {
				return
			}
			result, err := presentationResult(out, e)
			outputMutation(w, r, result, err)
		})
	}
	handle("PUT", "/brand-presentation", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := presentationActor(w, r, d, "write")
		if !ok {
			return
		}
		if r.URL.RawQuery != "" {
			failure(w, r, 400, "REQUEST_INVALID", "此接口不接受查询参数")
			return
		}
		var in brandskin.Input
		if !decodeBody(w, r, &in) {
			return
		}
		raw, e := json.Marshal(in)
		if e != nil {
			failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
			return
		}
		var fresh access.Account
		result, e := d.Mutations.ExecuteChecked(r.Context(), b, a.ID, "admin.brand_presentation.update", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(b+":"+string(raw)), func(ctx context.Context, tx pgx.Tx) error {
			var e error
			fresh, e = freshAdmin(ctx, tx, r, d, a, false)
			if errors.Is(e, adminsys.ErrDenied) || errors.Is(e, identity.ErrSession) {
				return identity.ErrSession
			}
			if e != nil {
				return e
			}
			if !brandskin.Allowed(fresh, b, "write") {
				return brandskin.ErrDenied
			}
			return nil
		}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			out, e := s.Update(ctx, tx, b, fresh, in, pointMeta(r, fresh))
			return presentationResult(out, e)
		})
		if errors.Is(e, identity.ErrSession) {
			result, e = mutation.Fail(401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录"), nil
		} else if errors.Is(e, brandskin.ErrDenied) {
			result, e = mutation.Fail(403, "PERMISSION_DENIED", "无品牌展示配置权限"), nil
		}
		finishAdminMutation(w, r, d, a, b, "brand_presentation.write", result, e)
	})
}
