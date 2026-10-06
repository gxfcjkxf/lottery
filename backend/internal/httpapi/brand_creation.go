package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/brandregistry"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
)

func brandCreationResult(out brandregistry.Receipt, err error) (mutation.Result, error) {
	switch {
	case errors.Is(err, brandregistry.ErrInvalid):
		return mutation.Fail(400, "BRAND_CREATE_INPUT_INVALID", "品牌名称、编号、语言、时区或原因不正确"), nil
	case errors.Is(err, brandregistry.ErrDenied):
		return mutation.Fail(403, "PERMISSION_DENIED", "无平台品牌创建权限"), nil
	case errors.Is(err, brandregistry.ErrCodeConflict):
		return mutation.Fail(409, "BRAND_CODE_CONFLICT", "品牌编号已存在，请重新核对"), nil
	case err != nil:
		return mutation.Result{}, err
	default:
		return mutation.OK(201, out), nil
	}
}

func registerBrandCreationRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	handle("POST", "/brands", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := adminAccount(w, r, d)
		if !ok {
			return
		}
		if !brandregistry.Allowed(actor) {
			rejectAdmin(w, r, d, actor, "", "brand.create.platform")
			return
		}
		if r.URL.RawQuery != "" || r.Header.Get("X-Brand-ID") != "" {
			failure(w, r, 400, "REQUEST_INVALID", "创建品牌为平台操作，不接受品牌头或查询参数")
			return
		}
		var in brandregistry.Input
		if !decodeBody(w, r, &in) {
			return
		}
		raw, err := json.Marshal(in)
		if err != nil {
			failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
			return
		}
		var fresh access.Account
		result, err := d.Mutations.ExecuteChecked(r.Context(), "", actor.ID, "admin.brand.create", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint("platform:"+string(raw)), func(ctx context.Context, tx pgx.Tx) error {
			var e error
			fresh, e = freshAdmin(ctx, tx, r, d, actor, false)
			if errors.Is(e, adminsys.ErrDenied) || errors.Is(e, identity.ErrSession) {
				return identity.ErrSession
			}
			if e != nil {
				return e
			}
			if !brandregistry.Allowed(fresh) {
				return brandregistry.ErrDenied
			}
			return nil
		}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			out, e := brandregistry.Create(ctx, tx, fresh, in, pointMeta(r, fresh))
			return brandCreationResult(out, e)
		})
		if errors.Is(err, identity.ErrSession) {
			result, err = mutation.Fail(401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录"), nil
		} else if errors.Is(err, brandregistry.ErrDenied) {
			result, err = mutation.Fail(403, "PERMISSION_DENIED", "无平台品牌创建权限"), nil
		}
		finishAdminMutation(w, r, d, actor, "", "brand.create.platform", result, err)
	})
}
