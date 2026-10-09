package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/reconciliation"
	"github.com/jackc/pgx/v5"
)

func businessInventoryRead(r *http.Request, d Dependencies, brand string) (reconciliation.InventorySnapshot, error) {
	var empty reconciliation.InventorySnapshot
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
	initial := access.Account{ID: id}
	fresh, err := freshAdmin(ctx, tx, r, d, initial, false)
	if errors.Is(err, adminsys.ErrDenied) || errors.Is(err, identity.ErrSession) {
		return empty, identity.ErrSession
	}
	if err != nil {
		return empty, err
	}
	if !reconciliation.Allowed(fresh, brand, "view") {
		return empty, reconciliationDenyRead(ctx, tx, r, fresh, brand, "wallet.view.brand")
	}
	if _, err = tx.Exec(ctx, "SAVEPOINT brand_business_inventory"); err != nil {
		return empty, err
	}
	queryCtx, stop := context.WithTimeout(ctx, 2*time.Second)
	out, queryErr := (reconciliation.Service{}).InventoryTx(queryCtx, tx, brand)
	stop()
	if queryErr != nil {
		if _, err = tx.Exec(ctx, "ROLLBACK TO SAVEPOINT brand_business_inventory"); err != nil {
			return empty, err
		}
	}
	if _, err = tx.Exec(ctx, "RELEASE SAVEPOINT brand_business_inventory"); err != nil {
		return empty, err
	}
	fresh, err = freshAdmin(ctx, tx, r, d, initial, false)
	if errors.Is(err, adminsys.ErrDenied) || errors.Is(err, identity.ErrSession) {
		return empty, identity.ErrSession
	}
	if err != nil {
		return empty, err
	}
	if !reconciliation.Allowed(fresh, brand, "view") {
		return empty, reconciliationDenyRead(ctx, tx, r, fresh, brand, "wallet.view.brand")
	}
	after := map[string]any{"attempted_brand": brand, "success": queryErr == nil}
	action := "wallet.business_inventory.view"
	if queryErr != nil {
		action = "wallet.business_inventory.read_failed"
	} else {
		after["schema_version"] = out.SchemaVersion
		after["source_row_count"] = out.SourceRowCount
		after["reference_count"] = out.ReferenceCount
		after["issue_count"] = out.IssueCount
		after["fingerprint"] = out.Fingerprint
		after["snapshot_at"] = out.SnapshotAt
	}
	record := audit.Record{ActorType: "admin", ActorID: fresh.ID, Action: action, ResourceType: "query", RequestID: requestID(r), IP: meta(r).IP, After: after}
	if queryErr == nil {
		record.BrandID = brand
	}
	if _, err = audit.Append(ctx, tx, record); err != nil {
		return empty, err
	}
	fresh, err = freshAdmin(ctx, tx, r, d, initial, false)
	if errors.Is(err, adminsys.ErrDenied) || errors.Is(err, identity.ErrSession) {
		return empty, identity.ErrSession
	}
	if err != nil {
		return empty, err
	}
	if !reconciliation.Allowed(fresh, brand, "view") {
		return empty, reconciliation.ErrDenied
	}
	if err = tx.Commit(ctx); err != nil {
		return empty, err
	}
	if queryErr != nil {
		return empty, queryErr
	}
	return out, nil
}

func registerBusinessInventoryRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	handle("GET", "/reconciliations/business-inventory", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" || r.URL.ForceQuery {
			failure(w, r, 400, "REQUEST_INVALID", "业务引用检查不接受查询参数")
			return
		}
		if !reconciliationGetHasNoBody(w, r) {
			return
		}
		brand, ok := reconciliationBrand(w, r)
		if !ok {
			return
		}
		out, err := businessInventoryRead(r, d, brand)
		switch {
		case errors.Is(err, identity.ErrSession):
			failure(w, r, 401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录")
		case errors.Is(err, reconciliation.ErrDenied):
			failure(w, r, 403, "PERMISSION_DENIED", "无此品牌钱包查看权限")
		case errors.Is(err, reconciliation.ErrNotFound):
			failure(w, r, 404, "BUSINESS_INVENTORY_NOT_FOUND", "未找到对应品牌")
		case errors.Is(err, reconciliation.ErrInventoryTooLarge):
			failure(w, r, 413, "BUSINESS_INVENTORY_TOO_LARGE", "完整业务来源超过单次检查上限，未返回部份结论")
		case err != nil:
			failure(w, r, 503, "BUSINESS_INVENTORY_UNAVAILABLE", "品牌业务引用检查暂不可用，请重新读取")
		default:
			respond(w, r, 200, out)
		}
	})
}
