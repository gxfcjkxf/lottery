package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/reporting"
	"github.com/jackc/pgx/v5"
)

func exportAllowed(a access.Account, brand, kind string) bool {
	resource := "report_" + kind
	granted := func(action string) bool {
		return access.Authorize(a, resource, action, access.ScopeBrand, brand) || access.Authorize(a, resource, action, access.ScopePlatform, "")
	}
	return (kind == "betting" || kind == "ledger") && granted("view") && granted("export")
}
func reportExportQuery(r *http.Request, kind string) (reporting.Query, error) {
	values, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil {
		return reporting.Query{}, reporting.ErrInvalid
	}
	for key := range values {
		switch key {
		case "from", "to", "group_by", "member_id", "game_id":
		default:
			return reporting.Query{}, reporting.ErrInvalid
		}
	}
	return reportQuery(r, kind)
}
func exportError(w http.ResponseWriter, r *http.Request, e error) {
	switch {
	case errors.Is(e, reporting.ErrNotFound):
		failure(w, r, 404, "REPORT_SCOPE_NOT_FOUND", "当前品牌未找到筛选资源")
	case errors.Is(e, reporting.ErrExportTooLarge):
		failure(w, r, 413, "REPORT_EXPORT_TOO_LARGE", "导出超过10000分组或4MiB，请缩小筛选范围；未生成截断文件")
	default:
		failure(w, r, 503, "REPORT_EXPORT_UNAVAILABLE", "无法完整生成或审计导出，请稍后重试")
	}
}
func registerReportExportRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	service := reporting.Service{DB: d.Admins.DB}
	for _, kind := range []string{"betting", "ledger"} {
		handle("GET", "/reports/"+kind+"/export", func(w http.ResponseWriter, r *http.Request) {
			actor, ok := adminAccount(w, r, d)
			if !ok {
				return
			}
			brand := strings.ToLower(r.Header.Get("X-Brand-ID"))
			if !uuidPattern.MatchString(brand) {
				failure(w, r, 400, "REQUEST_INVALID", "必须选择有效品牌")
				return
			}
			if !exportAllowed(actor, brand, kind) {
				rejectAdmin(w, r, d, actor, brand, "report_"+kind+".export")
				return
			}
			q, e := reportExportQuery(r, kind)
			if e != nil {
				failure(w, r, 400, "REPORT_QUERY_INVALID", "导出筛选无效，不接受分页参数，时间范围最多93天")
				return
			}
			// READ COMMITTED lets authorization observe changes committed while
			// waiting for its ACL lock. All facts/aggregates/balances still come
			// from exactly one SQL statement snapshot, never stitched pages.
			tx, e := d.Admins.DB.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
			if e != nil {
				exportError(w, r, e)
				return
			}
			defer tx.Rollback(r.Context())
			check := func(ctx context.Context) error {
				a, e := freshAdmin(ctx, tx, r, d, actor, false)
				if e != nil {
					return e
				}
				if !exportAllowed(a, brand, kind) {
					return adminsys.ErrDenied
				}
				return nil
			}
			denied := func(e error) bool {
				if errors.Is(e, identity.ErrSession) || errors.Is(e, adminsys.ErrDenied) || errors.Is(e, adminsys.ErrNotFound) {
					failure(w, r, 401, "AUTH_SESSION_REVOKED", "当前管理授权已变化，请重新登录")
					return true
				}
				return false
			}
			if e = check(r.Context()); e != nil {
				if !denied(e) {
					exportError(w, r, e)
				}
				return
			}
			var body []byte
			var snapshot time.Time
			var groups, timezone string
			if kind == "betting" {
				v, err := service.BettingExport(r.Context(), tx, brand, q)
				e = err
				if e == nil {
					body, e = reporting.BettingCSV(v)
					snapshot, groups, timezone = v.SnapshotAt, v.TotalGroups, v.Timezone
				}
			} else {
				v, err := service.LedgerExport(r.Context(), tx, brand, q)
				e = err
				if e == nil {
					body, e = reporting.LedgerCSV(v)
					snapshot, groups, timezone = v.SnapshotAt, v.TotalGroups, v.Timezone
				}
			}
			if e != nil {
				exportError(w, r, e)
				return
			}
			if e = check(r.Context()); e != nil {
				if !denied(e) {
					exportError(w, r, e)
				}
				return
			}
			sum := sha256.Sum256(body)
			digest := hex.EncodeToString(sum[:])
			filename := fmt.Sprintf("lottery-%s-%s-%s.csv", kind, brand, snapshot.UTC().Format("20060102T150405Z"))
			auditID, e := audit.Append(r.Context(), tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: actor.ID, Action: "report." + kind + ".export", ResourceType: "report_export", RequestID: requestID(r), IP: meta(r).IP, After: map[string]any{"format": "csv", "format_version": 1, "kind": kind, "query": q, "snapshot_at": snapshot.UTC(), "timezone": timezone, "group_count": groups, "byte_count": len(body), "sha256": digest}})
			if e == nil {
				e = tx.Commit(r.Context())
			}
			if e != nil {
				exportError(w, r, e)
				return
			}
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
			w.Header().Set("Content-Length", fmt.Sprint(len(body)))
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Report-Brand-ID", brand)
			w.Header().Set("X-Report-Kind", kind)
			w.Header().Set("X-Report-Snapshot-At", snapshot.UTC().Format(time.RFC3339Nano))
			w.Header().Set("X-Report-Group-Count", groups)
			w.Header().Set("X-Report-SHA256", digest)
			w.Header().Set("X-Report-Format-Version", "1")
			w.Header().Set("X-Report-Audit-ID", auditID)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		})
	}
}
