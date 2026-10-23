package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
)

func registerPlatformUserDelete(handle func(string, string, http.HandlerFunc), d Dependencies) {
	handle("POST", "/global-users/{id}/delete", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := platformAccountActor(w, r, d, "user", "delete")
		if !ok {
			return
		}
		id := r.PathValue("id")
		var body struct{}
		if !uuidPattern.MatchString(id) {
			failure(w, r, 400, "REQUEST_INVALID", "Invalid account ID")
			return
		}
		if !decodeBody(w, r, &body) {
			return
		}
		var fresh access.Account
		result, err := d.Mutations.ExecuteChecked(r.Context(), "", actor.ID, "platform.user.delete", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(id), func(ctx context.Context, tx pgx.Tx) error {
			var err error
			fresh, err = freshAdmin(ctx, tx, r, d, actor, true)
			if err != nil {
				return err
			}
			if !fresh.SuperAdmin || !access.Authorize(fresh, "user", "delete", access.ScopePlatform, "") {
				return adminsys.ErrDenied
			}
			return nil
		}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			deleted, err := d.Admins.DeleteGlobalUser(ctx, tx, fresh, id, requestID(r), meta(r).IP)
			if err != nil {
				return adminResult("", err)
			}
			return mutation.OK(200, deleted), nil
		})
		if errors.Is(err, identity.ErrSession) {
			result, err = mutation.Fail(401, "AUTH_SESSION_REVOKED", "Administrative session changed"), nil
		} else if errors.Is(err, adminsys.ErrDenied) {
			result, err = mutation.Fail(403, "PERMISSION_DENIED", "Account deletion permission required"), nil
		}
		finishAdminMutation(w, r, d, actor, "", "user.delete.platform", result, err)
	})
}
