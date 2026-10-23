package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
	"net/http"
)

func platformSecurityWriteRoute(method, path string) bool {
	return method == "PATCH" && path == "/auth-settings" || method == "PUT" && path == "/compliance-policy" || method == "POST" && path == "/compliance-checks"
}
func authSettingsWrite(w http.ResponseWriter, r *http.Request, d Dependencies, a access.Account, brand string, body identity.AuthSettingsInput, run managementAction) {
	if r.URL.RawQuery != "" {
		failure(w, r, 400, "REQUEST_INVALID", "此接口不接受查询参数")
		return
	}
	raw, _ := json.Marshal(body)
	var fresh access.Account
	result, err := d.Mutations.ExecuteChecked(r.Context(), brand, a.ID, "admin.auth_config.update", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(":"+string(raw)), func(ctx context.Context, tx pgx.Tx) error {
		var e error
		fresh, e = freshAdmin(ctx, tx, r, d, a, false)
		if errors.Is(e, adminsys.ErrDenied) || errors.Is(e, identity.ErrSession) {
			return identity.ErrSession
		}
		if e != nil {
			return e
		}
		if !managementAllowed(fresh, "auth_config", "write", brand) {
			return adminsys.ErrDenied
		}
		var state string
		if e = tx.QueryRow(ctx, `SELECT status FROM brands WHERE id=$1 FOR SHARE`, brand).Scan(&state); e != nil {
			return e
		}
		if state == "disabled" {
			return adminsys.ErrConflict
		}
		return nil
	}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) { return run(ctx, tx, fresh) })
	if errors.Is(err, identity.ErrSession) {
		result, err = mutation.Fail(401, "AUTH_SESSION_REVOKED", "管理会话已变化"), nil
	} else if err != nil {
		result, err = managementError(err)
	}
	finishAdminMutation(w, r, d, a, brand, "auth_config.write", result, err)
}
