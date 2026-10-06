package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/brandops"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
)

func brandOperationActor(w http.ResponseWriter, r *http.Request, d Dependencies, action string) (access.Account, string, bool) {
	a, ok := adminAccount(w, r, d)
	if !ok {
		return a, "", false
	}
	b := strings.ToLower(r.Header.Get("X-Brand-ID"))
	if !uuidPattern.MatchString(b) {
		failure(w, r, 400, "REQUEST_INVALID", "必须选择有效品牌")
		return a, b, false
	}
	if !brandops.Allowed(a, b, action) {
		rejectAdmin(w, r, d, a, b, "brand_operation."+action)
		return a, b, false
	}
	return a, b, true
}
func brandOperationResult(out any, err error) (mutation.Result, error) {
	switch {
	case errors.Is(err, brandops.ErrInvalid):
		return mutation.Fail(400, "BRAND_OPERATION_INPUT_INVALID", "品牌运行状态、版本或原因不正确"), nil
	case errors.Is(err, brandops.ErrDenied):
		return mutation.Fail(403, "PERMISSION_DENIED", "无品牌运行状态操作权限"), nil
	case errors.Is(err, brandops.ErrNotFound):
		return mutation.Fail(404, "BRAND_OPERATION_NOT_FOUND", "品牌不存在"), nil
	case errors.Is(err, brandops.ErrVersion):
		return mutation.Fail(409, "BRAND_OPERATION_VERSION_CONFLICT", "品牌配置版本已变化，请重新读取并核对"), nil
	case errors.Is(err, brandops.ErrState):
		return mutation.Fail(409, "BRAND_OPERATION_STATE_CONFLICT", "当前状态不允许此操作"), nil
	case err != nil:
		return mutation.Result{}, err
	default:
		return mutation.OK(200, out), nil
	}
}
func brandOperationPage(r *http.Request) (int, int, bool) {
	limit, offset := 20, 0
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return 0, 0, false
	}
	for key, values := range query {
		if len(values) != 1 || (key != "limit" && key != "offset") || values[0] == "" {
			return 0, 0, false
		}
		n, err := strconv.Atoi(values[0])
		if err != nil {
			return 0, 0, false
		}
		if key == "limit" {
			limit = n
		} else {
			offset = n
		}
	}
	return limit, offset, limit >= 1 && limit <= 100 && offset >= 0 && offset <= 1000000
}
func registerBrandOperationRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := brandops.Service{DB: d.Admins.DB}
	for _, history := range []bool{false, true} {
		path := "/brand-operation"
		if history {
			path += "/history"
		}
		handle("GET", path, func(w http.ResponseWriter, r *http.Request) {
			a, b, ok := brandOperationActor(w, r, d, "view")
			if !ok {
				return
			}
			var out any
			var err error
			if history {
				limit, offset, ok := brandOperationPage(r)
				if !ok {
					failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
					return
				}
				items, e := s.History(r.Context(), b, limit, offset)
				err = e
				out = map[string]any{"items": items, "limit": limit, "offset": offset}
			} else {
				if r.URL.RawQuery != "" {
					failure(w, r, 400, "REQUEST_INVALID", "此接口不接受查询参数")
					return
				}
				out, err = s.Read(r.Context(), b)
			}
			if err == nil && !adminReadAudit(w, r, d, a, b, "brand_operation.view") {
				return
			}
			result, e := brandOperationResult(out, err)
			outputMutation(w, r, result, e)
		})
	}
	handle("PATCH", "/brand-operation", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := brandOperationActor(w, r, d, "write")
		if !ok {
			return
		}
		var in brandops.Input
		if !decodeBody(w, r, &in) {
			return
		}
		if r.URL.RawQuery != "" {
			failure(w, r, 400, "REQUEST_INVALID", "此接口不接受查询参数")
			return
		}
		raw, err := json.Marshal(in)
		if err != nil {
			failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
			return
		}
		var fresh access.Account
		result, err := d.Mutations.ExecuteChecked(r.Context(), b, a.ID, "admin.brand_operation.update", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(b+":"+string(raw)), func(ctx context.Context, tx pgx.Tx) error {
			var e error
			fresh, e = freshAdmin(ctx, tx, r, d, a, false)
			if errors.Is(e, adminsys.ErrDenied) || errors.Is(e, identity.ErrSession) {
				return identity.ErrSession
			}
			if e != nil {
				return e
			}
			if !brandops.Allowed(fresh, b, "write") {
				return brandops.ErrDenied
			}
			return nil
		}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			out, e := s.Update(ctx, tx, b, fresh, in, pointMeta(r, fresh))
			return brandOperationResult(out, e)
		})
		if errors.Is(err, identity.ErrSession) {
			result, err = mutation.Fail(401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录"), nil
		} else if errors.Is(err, brandops.ErrDenied) {
			result, err = mutation.Fail(403, "PERMISSION_DENIED", "无品牌运行状态操作权限"), nil
		}
		finishAdminMutation(w, r, d, a, b, "brand_operation.write", result, err)
	})
}
