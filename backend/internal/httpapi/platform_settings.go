package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/platformsettings"
	"github.com/jackc/pgx/v5"
)

func platformSettingsResult(out platformsettings.Settings, err error) (mutation.Result, error) {
	switch {
	case errors.Is(err, platformsettings.ErrInvalid):
		return mutation.Fail(400, "PLATFORM_SETTINGS_INVALID", "平台设置不正确"), nil
	case errors.Is(err, platformsettings.ErrVersion):
		return mutation.Fail(409, "PLATFORM_SETTINGS_VERSION_CONFLICT", "平台设置已被修改，请重新读取后再保存"), nil
	case err != nil:
		return mutation.Result{}, err
	default:
		return mutation.OK(200, out), nil
	}
}

func registerPlatformSettingsRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := platformsettings.Service{DB: d.Admins.DB}
	handle("GET", "/platform-settings", func(w http.ResponseWriter, r *http.Request) {
		a, ok := platformAccountActor(w, r, d, "platform_settings", "view")
		if !ok {
			return
		}
		out, err := s.Read(r.Context())
		if err == nil && !adminReadAudit(w, r, d, a, "", "platform_settings.view.platform") {
			return
		}
		result, e := platformSettingsResult(out, err)
		outputMutation(w, r, result, e)
	})
	handle("PATCH", "/platform-settings", func(w http.ResponseWriter, r *http.Request) {
		a, ok := platformAccountActor(w, r, d, "platform_settings", "write")
		if !ok {
			return
		}
		var in platformsettings.Input
		if !decodeBody(w, r, &in) {
			return
		}
		raw, err := json.Marshal(in)
		if err != nil {
			failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
			return
		}
		var fresh access.Account
		result, err := d.Mutations.ExecuteChecked(r.Context(), "", a.ID, "platform.settings.update", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(string(raw)), func(ctx context.Context, tx pgx.Tx) error {
			freshAccount, e := freshAdmin(ctx, tx, r, d, a, true)
			if errors.Is(e, adminsys.ErrDenied) || errors.Is(e, identity.ErrSession) {
				return identity.ErrSession
			}
			if e != nil {
				return e
			}
			fresh = freshAccount
			if !platformsettings.Allowed(freshAccount, "write") {
				return platformsettings.ErrDenied
			}
			return nil
		}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			out, e := s.Update(ctx, tx, fresh, in, pointMeta(r, fresh))
			return platformSettingsResult(out, e)
		})
		if errors.Is(err, identity.ErrSession) {
			result, err = mutation.Fail(401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录"), nil
		} else if errors.Is(err, platformsettings.ErrDenied) {
			result, err = mutation.Fail(403, "PERMISSION_DENIED", "无平台设置权限"), nil
		}
		finishAdminMutation(w, r, d, a, "", "platform_settings.write.platform", result, err)
	})
}
