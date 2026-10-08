package httpapi

import (
	"context"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/workbench"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
)

func workbenchRead(r *http.Request, d Dependencies, brand string) (workbench.Snapshot, error) {
	var empty workbench.Snapshot
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	tx, err := d.Admins.DB.Begin(ctx)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	id, err := d.Identity.AdminAuthenticateTx(ctx, tx, requestToken(r, adminCookie))
	if errors.Is(err, identity.ErrSession) || errors.Is(err, pgx.ErrNoRows) {
		return empty, identity.ErrSession
	}
	if err != nil {
		return empty, err
	}
	actor, err := freshAdmin(ctx, tx, r, d, access.Account{ID: id}, false)
	if errors.Is(err, adminsys.ErrDenied) || errors.Is(err, identity.ErrSession) {
		return empty, identity.ErrSession
	}
	if err != nil {
		return empty, err
	}
	if !workbench.Allowed(actor, brand) {
		_, err = audit.Append(ctx, tx, audit.Record{ActorType: "admin", ActorID: actor.ID, Action: "access.denied", ResourceType: "permission", Reason: "administrative permission denied", RequestID: requestID(r), IP: meta(r).IP, After: map[string]string{"attempted_brand": brand, "permission": "workbench.view"}})
		if err == nil {
			err = tx.Commit(ctx)
		}
		if err != nil {
			return empty, err
		}
		return empty, workbench.ErrDenied
	}
	// A failed/cancelled SQL statement must not silently turn into a zero-valued
	// section. Keep the audit transaction usable and record failure separately.
	if _, err = tx.Exec(ctx, `SAVEPOINT workbench_snapshot`); err != nil {
		return empty, err
	}
	queryCtx, stop := context.WithTimeout(ctx, 2*time.Second)
	out, queryErr := (workbench.Service{}).ReadTx(queryCtx, tx, actor, brand)
	stop()
	if queryErr != nil {
		if _, err = tx.Exec(ctx, `ROLLBACK TO SAVEPOINT workbench_snapshot`); err != nil {
			return empty, err
		}
	}
	if _, err = tx.Exec(ctx, `RELEASE SAVEPOINT workbench_snapshot`); err != nil {
		return empty, err
	}
	fresh, err := freshAdmin(ctx, tx, r, d, access.Account{ID: id}, false)
	if errors.Is(err, adminsys.ErrDenied) || errors.Is(err, identity.ErrSession) {
		return empty, identity.ErrSession
	}
	if err != nil {
		return empty, err
	}
	if !workbench.Allowed(fresh, brand) {
		_, err = audit.Append(ctx, tx, audit.Record{ActorType: "admin", ActorID: fresh.ID, Action: "access.denied", ResourceType: "permission", Reason: "administrative permission denied", RequestID: requestID(r), IP: meta(r).IP, After: map[string]string{"attempted_brand": brand, "permission": "workbench.view"}})
		if err == nil {
			err = tx.Commit(ctx)
		}
		if err != nil {
			return empty, err
		}
		return empty, workbench.ErrDenied
	}
	action := "workbench.view"
	if queryErr != nil {
		action = "workbench.read_failed"
	}
	record := audit.Record{ActorType: "admin", ActorID: fresh.ID, Action: action, ResourceType: "query", RequestID: requestID(r), IP: meta(r).IP, After: map[string]any{"attempted_brand": brand, "success": queryErr == nil}}
	if queryErr == nil {
		record.BrandID = brand
	}
	if _, err = audit.Append(ctx, tx, record); err != nil {
		return empty, err
	}
	final, err := freshAdmin(ctx, tx, r, d, access.Account{ID: id}, false)
	if errors.Is(err, adminsys.ErrDenied) || errors.Is(err, identity.ErrSession) || err == nil && !workbench.Allowed(final, brand) {
		return empty, identity.ErrSession
	}
	if err != nil {
		return empty, err
	}
	if err = tx.Commit(ctx); err != nil {
		return empty, err
	}
	if queryErr != nil {
		return empty, queryErr
	}
	return out, nil
}
func registerWorkbenchRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	handle("GET", "/workbench", func(w http.ResponseWriter, r *http.Request) {
		brand := strings.ToLower(r.Header.Get("X-Brand-ID"))
		if !uuidPattern.MatchString(brand) {
			failure(w, r, 400, "REQUEST_INVALID", "必须选择有效品牌")
			return
		}
		if r.URL.RawQuery != "" || r.URL.ForceQuery {
			failure(w, r, 400, "REQUEST_INVALID", "工作台不接受查询参数")
			return
		}
		out, err := workbenchRead(r, d, brand)
		switch {
		case errors.Is(err, identity.ErrSession):
			failure(w, r, 401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录")
		case errors.Is(err, workbench.ErrDenied):
			failure(w, r, 403, "PERMISSION_DENIED", "无此品牌工作台查看权限")
		case errors.Is(err, workbench.ErrNotFound):
			failure(w, r, 404, "WORKBENCH_NOT_FOUND", "未找到对应品牌")
		case err != nil:
			failure(w, r, 503, "WORKBENCH_UNAVAILABLE", "工作台暂不可用，请重新读取")
		default:
			respond(w, r, 200, out)
		}
	})
}
