package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
)

func permissionsByBrand(a access.Account) map[string][]string {
	out := make(map[string][]string)
	for _, brand := range a.BrandIDs {
		out[brand] = []string{}
		for _, p := range access.UnionPermissions(a.Roles...) {
			if p.Scope == access.ScopeBrand && access.Authorize(a, p.Resource, p.Action, p.Scope, brand) {
				out[brand] = append(out[brand], p.Resource+"."+p.Action+".brand")
			}
		}
	}
	return out
}
func platformPermissions(a access.Account) []string {
	out := []string{}
	for _, p := range access.UnionPermissions(a.Roles...) {
		if p.Scope == access.ScopePlatform && access.Authorize(a, p.Resource, p.Action, p.Scope, "") {
			out = append(out, p.Resource+"."+p.Action+".platform")
		}
	}
	return out
}

// Record only structured attempted permission/context, never bodies, credentials,
// or untrusted resource IDs as foreign keys. Unknown/foreign brands are metadata.
func deniedAdminAudit(r *http.Request, d Dependencies, a access.Account, brand, permission string) error {
	tx, err := d.Admins.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	_, err = audit.Append(r.Context(), tx, audit.Record{
		ActorType: "admin", ActorID: a.ID, Action: "access.denied", ResourceType: "permission",
		Reason: "administrative permission denied", RequestID: requestID(r), IP: meta(r).IP,
		After: map[string]string{"attempted_brand": brand, "permission": permission},
	})
	if err != nil {
		return err
	}
	return tx.Commit(r.Context())
}
func rejectAdmin(w http.ResponseWriter, r *http.Request, d Dependencies, a access.Account, brand, permission string) {
	if err := deniedAdminAudit(r, d, a, brand, permission); err != nil {
		failure(w, r, 503, "SERVICE_UNAVAILABLE", "无法记录权限拒绝")
		return
	}
	failure(w, r, 403, "PERMISSION_DENIED", "无操作权限")
}

func freshAdmin(ctx context.Context, tx pgx.Tx, r *http.Request, d Dependencies, initial access.Account, exclusive bool) (access.Account, error) {
	a, err := d.Admins.LockAdminAccess(ctx, tx, initial.ID, exclusive)
	if errors.Is(err, adminsys.ErrDenied) || errors.Is(err, adminsys.ErrNotFound) {
		return access.Account{}, adminsys.ErrDenied
	}
	if err != nil {
		return a, err
	}
	id, err := d.Identity.AdminAuthenticateTx(ctx, tx, requestToken(r, administrativeCookie(r)))
	if err != nil || id != a.ID {
		if err == nil || errors.Is(err, pgx.ErrNoRows) {
			return access.Account{}, adminsys.ErrDenied
		}
		return access.Account{}, err
	}
	if a.SuperAdmin != platformAdminEntry(r) {
		return access.Account{}, adminsys.ErrDenied
	}
	return entryPermissions(a, platformAdminEntry(r)), nil
}

func finishAdminMutation(w http.ResponseWriter, r *http.Request, d Dependencies, a access.Account, brand, permission string, result mutation.Result, err error) {
	if err == nil && result.Status == 403 {
		if logErr := deniedAdminAudit(r, d, a, brand, permission); logErr != nil {
			failure(w, r, 503, "SERVICE_UNAVAILABLE", "无法记录权限拒绝")
			return
		}
	}
	outputMutation(w, r, result, err)
}
