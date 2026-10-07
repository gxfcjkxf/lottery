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

func withdrawalReportAllowed(a access.Account, brand, action string) bool {
	return access.Authorize(a, "report_withdrawal", action, access.ScopeBrand, brand) ||
		access.Authorize(a, "report_withdrawal", action, access.ScopePlatform, "")
}

func withdrawalExportQuery(r *http.Request) (reporting.Query, error) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return reporting.Query{}, reporting.ErrInvalid
	}
	for key, list := range values {
		if len(list) != 1 || key == "limit" || key == "offset" || key == "game_id" {
			return reporting.Query{}, reporting.ErrInvalid
		}
		switch key {
		case "from", "to", "group_by", "member_id":
		default:
			return reporting.Query{}, reporting.ErrInvalid
		}
	}
	return reportQuery(r, "withdrawal")
}

func registerWithdrawalReportRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	service := reporting.Service{DB: d.Admins.DB}
	handle("GET", "/reports/withdrawal", func(w http.ResponseWriter, r *http.Request) {
		q, err := reportQuery(r, "withdrawal")
		if err != nil {
			failure(w, r, 400, "REPORT_QUERY_INVALID", "提现报表筛选无效；时间范围最多93天且不支持彩种筛选")
			return
		}
		withdrawalReportRead(w, r, d, "view", q, func(ctx context.Context, tx pgx.Tx, brand string) (any, error) {
			return service.WithdrawalRead(ctx, tx, brand, q)
		}, false)
	})
	handle("GET", "/reports/withdrawal/export", func(w http.ResponseWriter, r *http.Request) {
		q, err := withdrawalExportQuery(r)
		if err != nil {
			failure(w, r, 400, "REPORT_QUERY_INVALID", "提现报表导出筛选无效；不接受分页参数，时间范围最多93天")
			return
		}
		withdrawalReportRead(w, r, d, "export", q, func(ctx context.Context, tx pgx.Tx, brand string) (any, error) {
			return service.WithdrawalExport(ctx, tx, brand, q)
		}, true)
	})
}

func withdrawalReportRead(w http.ResponseWriter, r *http.Request, d Dependencies, action string, q reporting.Query, query func(context.Context, pgx.Tx, string) (any, error), exporting bool) {
	brand := strings.ToLower(r.Header.Get("X-Brand-ID"))
	if !uuidPattern.MatchString(brand) {
		failure(w, r, 400, "REQUEST_INVALID", "必须选择有效品牌")
		return
	}
	actor, ok := adminAccount(w, r, d)
	if !ok {
		return
	}
	tx, err := d.Admins.DB.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		withdrawalReportFailure(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	check := func() (access.Account, error) {
		fresh, e := freshAdmin(r.Context(), tx, r, d, actor, false)
		if e != nil {
			return access.Account{}, e
		}
		if !withdrawalReportAllowed(fresh, brand, action) || exporting && !withdrawalReportAllowed(fresh, brand, "view") {
			return fresh, adminsys.ErrDenied
		}
		return fresh, nil
	}
	fresh, err := check()
	if errors.Is(err, identity.ErrSession) || errors.Is(err, adminsys.ErrDenied) || errors.Is(err, adminsys.ErrNotFound) {
		if errors.Is(err, adminsys.ErrDenied) {
			_, auditErr := audit.Append(r.Context(), tx, audit.Record{ActorType: "admin", ActorID: actor.ID, Action: "access.denied", ResourceType: "permission", Reason: "administrative permission denied", RequestID: requestID(r), IP: meta(r).IP, After: map[string]string{"attempted_brand": brand, "permission": "report_withdrawal." + action}})
			if auditErr == nil {
				auditErr = tx.Commit(r.Context())
			}
			if auditErr != nil {
				withdrawalReportFailure(w, r, auditErr)
				return
			}
			failure(w, r, 403, "PERMISSION_DENIED", "无提现报表权限")
			return
		}
		failure(w, r, 401, "AUTH_SESSION_REVOKED", "当前管理授权已变化，请重新登录")
		return
	}
	if err != nil {
		withdrawalReportFailure(w, r, err)
		return
	}
	data, queryErr := query(r.Context(), tx, brand)
	// Retain the same primary transaction and locks through the second fresh
	// session/role check, audit insert and commit.
	if _, err = check(); err != nil {
		if errors.Is(err, identity.ErrSession) || errors.Is(err, adminsys.ErrDenied) || errors.Is(err, adminsys.ErrNotFound) {
			failure(w, r, 401, "AUTH_SESSION_REVOKED", "当前管理授权已变化，请重新登录")
		} else {
			withdrawalReportFailure(w, r, err)
		}
		return
	}
	if queryErr != nil && !errors.Is(queryErr, reporting.ErrNotFound) {
		withdrawalReportFailure(w, r, queryErr)
		return
	}
	if errors.Is(queryErr, reporting.ErrNotFound) {
		actionName, resourceType := "report.withdrawal.view", "query"
		after := map[string]any{"query": q, "outcome": "scope_not_found"}
		if exporting {
			actionName, resourceType = "report.withdrawal.export", "report_export"
			after["kind"], after["format"] = "withdrawal", "csv"
		}
		_, err = audit.Append(r.Context(), tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: fresh.ID, Action: actionName, ResourceType: resourceType, RequestID: requestID(r), IP: meta(r).IP, After: after})
		if err == nil {
			err = tx.Commit(r.Context())
		}
		if err != nil {
			withdrawalReportFailure(w, r, err)
			return
		}
		failure(w, r, 404, "REPORT_SCOPE_NOT_FOUND", "当前品牌未找到提现报表筛选会员")
		return
	}
	var body []byte
	var snapshot time.Time
	var groups, timezone, digest, auditID, filename string
	if exporting {
		report, ok := data.(reporting.WithdrawalReport)
		if !ok {
			withdrawalReportFailure(w, r, fmt.Errorf("unexpected withdrawal report type"))
			return
		}
		body, err = reporting.WithdrawalCSV(report)
		snapshot, groups, timezone = report.SnapshotAt, report.TotalGroups, report.Timezone
		filename = fmt.Sprintf("lottery-withdrawal-%s-%s.csv", brand, snapshot.UTC().Format("20060102T150405Z"))
		if err != nil {
			exportError(w, r, err)
			return
		}
		sum := sha256.Sum256(body)
		digest = hex.EncodeToString(sum[:])
		auditID, err = audit.Append(r.Context(), tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: fresh.ID, Action: "report.withdrawal.export", ResourceType: "report_export", RequestID: requestID(r), IP: meta(r).IP, After: map[string]any{"format": "csv", "format_version": 1, "kind": "withdrawal", "query": q, "snapshot_at": snapshot.UTC(), "timezone": timezone, "group_count": groups, "byte_count": len(body), "sha256": digest}})
	} else {
		auditID, err = audit.Append(r.Context(), tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: fresh.ID, Action: "report.withdrawal.view", ResourceType: "query", RequestID: requestID(r), IP: meta(r).IP, After: map[string]any{"query": q}})
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		withdrawalReportFailure(w, r, err)
		return
	}
	if exporting {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Report-Brand-ID", brand)
		w.Header().Set("X-Report-Kind", "withdrawal")
		w.Header().Set("X-Report-Snapshot-At", snapshot.UTC().Format(time.RFC3339Nano))
		w.Header().Set("X-Report-Group-Count", groups)
		w.Header().Set("X-Report-SHA256", digest)
		w.Header().Set("X-Report-Format-Version", "1")
		w.Header().Set("X-Report-Audit-ID", auditID)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
		return
	}
	respond(w, r, 200, data)
}

func withdrawalReportFailure(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, reporting.ErrNotFound):
		failure(w, r, 404, "REPORT_SCOPE_NOT_FOUND", "当前品牌未找到提现报表筛选会员")
	case errors.Is(err, reporting.ErrExportTooLarge):
		failure(w, r, 413, "REPORT_EXPORT_TOO_LARGE", "导出超过10000分组或4MiB，请缩小筛选范围；未生成截断文件")
	default:
		failure(w, r, 503, "REPORT_UNAVAILABLE", "无法完整生成或审计提现报表，请稍后重试")
	}
}
