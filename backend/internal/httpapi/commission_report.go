package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/reporting"
	"github.com/jackc/pgx/v5"
)

var commissionReportInteger = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)

func commissionReportAllowed(a access.Account, brand, action string) bool {
	return access.Authorize(a, "report_commission", action, access.ScopeBrand, brand) ||
		access.Authorize(a, "report_commission", action, access.ScopePlatform, "")
}

func commissionReportQuery(r *http.Request, exporting bool) (reporting.CommissionQuery, error) {
	out := reporting.CommissionQuery{Limit: 20}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || r.URL.ForceQuery {
		return out, reporting.ErrInvalid
	}
	for key, list := range values {
		if len(list) != 1 || list[0] == "" {
			return out, reporting.ErrInvalid
		}
		switch key {
		case "from", "to", "group_by", "agent_id", "member_id", "cycle_id":
		case "limit", "offset":
			if exporting {
				return out, reporting.ErrInvalid
			}
		default:
			return out, reporting.ErrInvalid
		}
	}
	if _, ok := values["from"]; !ok {
		return out, reporting.ErrInvalid
	}
	if _, ok := values["to"]; !ok {
		return out, reporting.ErrInvalid
	}
	if _, ok := values["group_by"]; !ok {
		return out, reporting.ErrInvalid
	}
	out.From, err = time.Parse(time.RFC3339Nano, values.Get("from"))
	if err != nil {
		return out, reporting.ErrInvalid
	}
	out.To, err = time.Parse(time.RFC3339Nano, values.Get("to"))
	if err != nil {
		return out, reporting.ErrInvalid
	}
	out.From, out.To = out.From.UTC(), out.To.UTC()
	out.GroupBy = values.Get("group_by")
	for name := range map[string]bool{"limit": true, "offset": true} {
		if value, ok := values[name]; ok {
			if !commissionReportInteger.MatchString(value[0]) {
				return out, reporting.ErrInvalid
			}
			n, parseErr := strconv.Atoi(value[0])
			if parseErr != nil {
				return out, reporting.ErrInvalid
			}
			if name == "limit" {
				out.Limit = n
			} else {
				out.Offset = n
			}
		}
	}
	for name, target := range map[string]**string{"agent_id": &out.AgentID, "member_id": &out.MemberID, "cycle_id": &out.CycleID} {
		if value, ok := values[name]; ok {
			id := strings.ToLower(value[0])
			*target = &id
		}
	}
	if err = out.Validate(); err != nil || exporting && out.Offset != 0 {
		return out, reporting.ErrInvalid
	}
	return out, nil
}

func registerCommissionReportRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	service := reporting.Service{DB: d.Admins.DB}
	handle("GET", "/reports/commission", func(w http.ResponseWriter, r *http.Request) {
		q, err := commissionReportQuery(r, false)
		if err != nil {
			failure(w, r, 400, "REPORT_QUERY_INVALID", "佣金报表筛选无效；时间范围最多93天")
			return
		}
		commissionReportRead(w, r, d, "view", q, false, func(ctx context.Context, tx pgx.Tx, brand string) (any, error) {
			return service.CommissionRead(ctx, tx, brand, q)
		})
	})
	handle("GET", "/reports/commission.csv", func(w http.ResponseWriter, r *http.Request) {
		q, err := commissionReportQuery(r, true)
		if err != nil {
			failure(w, r, 400, "REPORT_QUERY_INVALID", "佣金报表导出筛选无效；不接受分页参数，时间范围最多93天")
			return
		}
		commissionReportRead(w, r, d, "export", q, true, func(ctx context.Context, tx pgx.Tx, brand string) (any, error) {
			return service.CommissionExport(ctx, tx, brand, q)
		})
	})
}

func commissionReportRead(w http.ResponseWriter, r *http.Request, d Dependencies, action string, q reporting.CommissionQuery, exporting bool, query func(context.Context, pgx.Tx, string) (any, error)) {
	brand := strings.ToLower(r.Header.Get("X-Brand-ID"))
	if !uuidPattern.MatchString(brand) {
		failure(w, r, 400, "REQUEST_INVALID", "必须选择有效品牌")
		return
	}
	actor, ok := adminAccount(w, r, d)
	if !ok {
		return
	}
	// READ COMMITTED keeps freshAdmin's role/session rechecks current at each
	// statement. The report's summary, groups, count, and timezone are produced
	// together by one SQL statement, so they share one database snapshot while
	// the authorization gate stays locked through the audit commit.
	tx, err := d.Admins.DB.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		commissionReportFailure(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	check := func(requiredAction string) (access.Account, error) {
		fresh, checkErr := freshAdmin(r.Context(), tx, r, d, actor, false)
		if checkErr != nil {
			return access.Account{}, checkErr
		}
		if !commissionReportAllowed(fresh, brand, requiredAction) {
			return fresh, adminsys.ErrDenied
		}
		return fresh, nil
	}
	fresh, err := check(action)
	deniedAction := action
	if err == nil && exporting {
		deniedAction = "view"
		_, err = check("view")
	}
	if errors.Is(err, adminsys.ErrDenied) {
		permission := "report_commission." + deniedAction
		_, auditErr := audit.Append(r.Context(), tx, audit.Record{ActorType: "admin", ActorID: actor.ID, Action: "access.denied", ResourceType: "permission", Reason: "administrative permission denied", RequestID: requestID(r), IP: meta(r).IP, After: map[string]string{"attempted_brand": brand, "permission": permission}})
		if auditErr == nil {
			auditErr = tx.Commit(r.Context())
		}
		if auditErr != nil {
			commissionReportFailure(w, r, auditErr)
			return
		}
		failure(w, r, 403, "PERMISSION_DENIED", "无佣金报表权限")
		return
	}
	if errors.Is(err, identity.ErrSession) || errors.Is(err, adminsys.ErrNotFound) {
		failure(w, r, 401, "AUTH_SESSION_REVOKED", "当前管理授权已变化，请重新登录")
		return
	}
	if err != nil {
		commissionReportFailure(w, r, err)
		return
	}
	data, queryErr := query(r.Context(), tx, brand)
	// Keep the primary transaction through a second role/session check and the
	// audit commit, so a stale grant cannot release report data.
	if _, err = check(action); err == nil && exporting {
		_, err = check("view")
	}
	if errors.Is(err, identity.ErrSession) || errors.Is(err, adminsys.ErrDenied) || errors.Is(err, adminsys.ErrNotFound) {
		failure(w, r, 401, "AUTH_SESSION_REVOKED", "当前管理授权已变化，请重新登录")
		return
	}
	if err != nil {
		commissionReportFailure(w, r, err)
		return
	}
	if queryErr != nil && !errors.Is(queryErr, reporting.ErrNotFound) {
		commissionReportFailure(w, r, queryErr)
		return
	}
	if errors.Is(queryErr, reporting.ErrNotFound) {
		actionName, resourceType := "report.commission.view", "query"
		after := map[string]any{"query": q, "outcome": "scope_not_found"}
		if exporting {
			actionName, resourceType = "report.commission.export", "report_export"
			after["kind"], after["format"] = "commission", "csv"
		}
		_, err = audit.Append(r.Context(), tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: fresh.ID, Action: actionName, ResourceType: resourceType, RequestID: requestID(r), IP: meta(r).IP, After: after})
		if err == nil {
			err = tx.Commit(r.Context())
		}
		if err != nil {
			commissionReportFailure(w, r, err)
			return
		}
		failure(w, r, 404, "REPORT_SCOPE_NOT_FOUND", "当前品牌未找到佣金报表筛选资源")
		return
	}
	var body []byte
	var snapshot time.Time
	var groups, timezone, digest, auditID, filename string
	if exporting {
		report, typeOK := data.(reporting.CommissionReport)
		if !typeOK {
			commissionReportFailure(w, r, fmt.Errorf("unexpected commission report type"))
			return
		}
		body, err = reporting.CommissionCSV(report)
		snapshot, groups, timezone = report.SnapshotAt, report.TotalGroups, report.Timezone
		filename = fmt.Sprintf("lottery-commission-%s-%s.csv", brand, snapshot.UTC().Format("20060102T150405Z"))
		if err != nil {
			commissionReportFailure(w, r, err)
			return
		}
		sum := sha256.Sum256(body)
		digest = hex.EncodeToString(sum[:])
		auditID, err = audit.Append(r.Context(), tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: fresh.ID, Action: "report.commission.export", ResourceType: "report_export", RequestID: requestID(r), IP: meta(r).IP, After: map[string]any{"format": "csv", "format_version": 1, "kind": "commission", "query": q, "snapshot_at": snapshot.UTC(), "timezone": timezone, "group_count": groups, "byte_count": len(body), "sha256": digest}})
	} else {
		auditID, err = audit.Append(r.Context(), tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: fresh.ID, Action: "report.commission.view", ResourceType: "query", RequestID: requestID(r), IP: meta(r).IP, After: map[string]any{"query": q}})
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		commissionReportFailure(w, r, err)
		return
	}
	if !exporting {
		respond(w, r, 200, data)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Report-Brand-ID", brand)
	w.Header().Set("X-Report-Kind", "commission")
	w.Header().Set("X-Report-Snapshot-At", snapshot.UTC().Format(time.RFC3339Nano))
	w.Header().Set("X-Report-Timezone", timezone)
	w.Header().Set("X-Report-Group-Count", groups)
	w.Header().Set("X-Report-Byte-Count", strconv.Itoa(len(body)))
	w.Header().Set("X-Report-SHA256", digest)
	w.Header().Set("X-Report-Format-Version", "1")
	w.Header().Set("X-Report-Audit-ID", auditID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func commissionReportFailure(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, reporting.ErrNotFound):
		failure(w, r, 404, "REPORT_SCOPE_NOT_FOUND", "当前品牌未找到佣金报表筛选资源")
	case errors.Is(err, reporting.ErrExportTooLarge):
		failure(w, r, 413, "REPORT_EXPORT_TOO_LARGE", "导出超过10000分组或4MiB，请缩小筛选范围；未生成截断文件")
	default:
		failure(w, r, 503, "REPORT_UNAVAILABLE", "无法完整生成或审计佣金报表，请稍后重试")
	}
}
