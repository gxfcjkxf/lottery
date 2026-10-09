package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

func commissionAnalysisAllowed(a access.Account, brand, resource, action string) bool {
	return access.Authorize(a, resource, action, access.ScopeBrand, brand) ||
		access.Authorize(a, resource, action, access.ScopePlatform, "")
}

func commissionAnalysisQuery(r *http.Request, exporting bool) (reporting.CommissionAnalysisQuery, error) {
	q := reporting.CommissionAnalysisQuery{Limit: 20}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || r.URL.ForceQuery {
		return q, reporting.ErrInvalid
	}
	for key, list := range values {
		if len(list) != 1 || list[0] == "" {
			return q, reporting.ErrInvalid
		}
		switch key {
		case "from", "to", "group_by", "agent_id", "member_id", "cycle_id":
		case "limit", "offset":
			if exporting {
				return q, reporting.ErrInvalid
			}
		default:
			return q, reporting.ErrInvalid
		}
	}
	for _, key := range []string{"from", "to", "group_by"} {
		if _, ok := values[key]; !ok {
			return q, reporting.ErrInvalid
		}
	}
	q.From, err = time.Parse(time.RFC3339Nano, values.Get("from"))
	if err != nil {
		return q, reporting.ErrInvalid
	}
	q.To, err = time.Parse(time.RFC3339Nano, values.Get("to"))
	if err != nil {
		return q, reporting.ErrInvalid
	}
	q.From, q.To = q.From.UTC(), q.To.UTC()
	q.GroupBy = values.Get("group_by")
	for _, key := range []string{"limit", "offset"} {
		if raw, ok := values[key]; ok {
			if !commissionReportInteger.MatchString(raw[0]) {
				return q, reporting.ErrInvalid
			}
			n, parseErr := strconv.Atoi(raw[0])
			if parseErr != nil {
				return q, reporting.ErrInvalid
			}
			if key == "limit" {
				q.Limit = n
			} else {
				q.Offset = n
			}
		}
	}
	for key, target := range map[string]**string{"agent_id": &q.AgentID, "member_id": &q.MemberID, "cycle_id": &q.CycleID} {
		if raw, ok := values[key]; ok {
			if !uuidPattern.MatchString(raw[0]) || strings.ToLower(raw[0]) != raw[0] {
				return q, reporting.ErrInvalid
			}
			value := raw[0]
			*target = &value
		}
	}
	if err = q.Validate(); err != nil || exporting && q.Offset != 0 {
		return q, reporting.ErrInvalid
	}
	return q, nil
}

func commissionAnalysisGetHasNoBody(w http.ResponseWriter, r *http.Request) bool {
	if len(r.TransferEncoding) != 0 || r.ContentLength > 0 {
		failure(w, r, 400, "REPORT_QUERY_INVALID", "佣金周期分析查询不接受请求正文")
		return false
	}
	if r.Body == nil || r.Body == http.NoBody {
		return true
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1))
	if len(body) != 0 || err != nil && !errors.Is(err, io.EOF) {
		failure(w, r, 400, "REPORT_QUERY_INVALID", "佣金周期分析查询不接受请求正文")
		return false
	}
	return true
}

func registerCommissionAnalysisReportRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	service := reporting.Service{DB: d.Admins.DB}
	for _, exporting := range []bool{false, true} {
		path := "/reports/commission-analysis"
		if exporting {
			path += ".csv"
		}
		handle("GET", path, func(w http.ResponseWriter, r *http.Request) {
			if !commissionAnalysisGetHasNoBody(w, r) {
				return
			}
			q, err := commissionAnalysisQuery(r, exporting)
			if err != nil {
				failure(w, r, 400, "REPORT_QUERY_INVALID", "佣金周期分析筛选无效；周期窗口最多93天，导出不接受分页参数")
				return
			}
			commissionAnalysisReportRead(w, r, d, service, q, exporting)
		})
	}
}

func commissionAnalysisReportRead(w http.ResponseWriter, r *http.Request, d Dependencies, service reporting.Service, q reporting.CommissionAnalysisQuery, exporting bool) {
	brand := strings.ToLower(r.Header.Get("X-Brand-ID"))
	if !uuidPattern.MatchString(brand) {
		failure(w, r, 400, "REQUEST_INVALID", "必须选择有效品牌")
		return
	}
	actor, ok := adminAccount(w, r, d)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	tx, err := d.Admins.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		commissionAnalysisFailure(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	check := func() (access.Account, string, error) {
		fresh, checkErr := freshAdmin(ctx, tx, r, d, actor, false)
		if checkErr != nil {
			return access.Account{}, "view", checkErr
		}
		if !commissionAnalysisAllowed(fresh, brand, "commission", "view") {
			return fresh, "commission.view", adminsys.ErrDenied
		}
		if !commissionAnalysisAllowed(fresh, brand, "report_commission", "view") {
			return fresh, "report_commission.view", adminsys.ErrDenied
		}
		if exporting && !commissionAnalysisAllowed(fresh, brand, "report_commission", "export") {
			return fresh, "report_commission.export", adminsys.ErrDenied
		}
		return fresh, "", nil
	}
	checkFailure := func(permission string, checkErr error) {
		if errors.Is(checkErr, adminsys.ErrDenied) {
			_, auditErr := audit.Append(ctx, tx, audit.Record{ActorType: "admin", ActorID: actor.ID, Action: "access.denied", ResourceType: "permission", Reason: "administrative permission denied", RequestID: requestID(r), IP: meta(r).IP, After: map[string]string{"attempted_brand": brand, "permission": permission}})
			if auditErr == nil {
				auditErr = tx.Commit(ctx)
			}
			if auditErr != nil {
				commissionAnalysisFailure(w, r, auditErr)
				return
			}
			failure(w, r, 403, "PERMISSION_DENIED", "无佣金周期分析对应权限")
			return
		}
		if errors.Is(checkErr, identity.ErrSession) || errors.Is(checkErr, adminsys.ErrNotFound) {
			failure(w, r, 401, "AUTH_SESSION_REVOKED", "当前管理授权已变化，请重新登录")
			return
		}
		commissionAnalysisFailure(w, r, checkErr)
	}
	fresh, denied, err := check()
	if err != nil {
		checkFailure(denied, err)
		return
	}
	if _, err = tx.Exec(ctx, "SAVEPOINT commission_analysis_report_data"); err != nil {
		commissionAnalysisFailure(w, r, err)
		return
	}
	var report reporting.CommissionAnalysisReport
	if exporting {
		report, err = service.CommissionAnalysisExport(ctx, tx, brand, q)
	} else {
		report, err = service.CommissionAnalysisRead(ctx, tx, brand, q)
	}
	queryErr := err
	if queryErr != nil {
		if _, err = tx.Exec(ctx, "ROLLBACK TO SAVEPOINT commission_analysis_report_data"); err != nil {
			commissionAnalysisFailure(w, r, err)
			return
		}
	}
	if _, err = tx.Exec(ctx, "RELEASE SAVEPOINT commission_analysis_report_data"); err != nil {
		commissionAnalysisFailure(w, r, err)
		return
	}
	if _, denied, err = check(); err != nil {
		checkFailure(denied, err)
		return
	}
	action, resource, kind := "report.commission_analysis.view", "query", "commission_analysis"
	if exporting {
		action, resource = "report.commission_analysis.export", "report_export"
	}
	after := map[string]any{"kind": kind, "query": q, "outcome": "ready"}
	var body []byte
	var digest, filename string
	if queryErr != nil {
		after["outcome"] = "failed"
		switch {
		case errors.Is(queryErr, reporting.ErrNotFound):
			after["outcome"] = "scope_not_found"
		case errors.Is(queryErr, reporting.ErrExportTooLarge):
			after["outcome"] = "too_large"
		case errors.Is(queryErr, reporting.ErrAnalysisIntegrity):
			after["outcome"] = "integrity_failed"
		}
	} else {
		after["snapshot_at"], after["timezone"], after["group_count"] = report.SnapshotAt.UTC(), report.Timezone, report.TotalGroups
		if exporting {
			body, queryErr = reporting.CommissionAnalysisCSV(report)
			if queryErr == nil {
				sum := sha256.Sum256(body)
				digest = hex.EncodeToString(sum[:])
				after["format"], after["format_version"], after["byte_count"], after["sha256"] = "csv", 1, len(body), digest
				filename = fmt.Sprintf("lottery-commission-analysis-%s-%s.csv", brand, report.SnapshotAt.UTC().Format("20060102T150405Z"))
			} else {
				after["outcome"] = "failed"
			}
		}
	}
	// Recheck after audit waits and immediately before commit; response data is
	// released only after this transaction has durably committed the audit.
	if _, err = tx.Exec(ctx, "SAVEPOINT commission_analysis_report_audit"); err != nil {
		commissionAnalysisFailure(w, r, err)
		return
	}
	auditID, err := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: fresh.ID, Action: action, ResourceType: resource, RequestID: requestID(r), IP: meta(r).IP, After: after})
	if err != nil {
		commissionAnalysisFailure(w, r, err)
		return
	}
	if _, denied, err = check(); err != nil {
		if errors.Is(err, adminsys.ErrDenied) {
			if _, rollbackErr := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT commission_analysis_report_audit"); rollbackErr != nil {
				commissionAnalysisFailure(w, r, rollbackErr)
				return
			}
			if _, releaseErr := tx.Exec(ctx, "RELEASE SAVEPOINT commission_analysis_report_audit"); releaseErr != nil {
				commissionAnalysisFailure(w, r, releaseErr)
				return
			}
		}
		checkFailure(denied, err)
		return
	}
	if _, err = tx.Exec(ctx, "RELEASE SAVEPOINT commission_analysis_report_audit"); err != nil {
		commissionAnalysisFailure(w, r, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		commissionAnalysisFailure(w, r, err)
		return
	}
	if queryErr != nil {
		commissionAnalysisFailure(w, r, queryErr)
		return
	}
	if !exporting {
		respond(w, r, 200, report)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	for key, value := range map[string]string{
		"X-Report-Brand-ID":       brand,
		"X-Report-Kind":           "commission_analysis",
		"X-Report-Snapshot-At":    report.SnapshotAt.UTC().Format(time.RFC3339Nano),
		"X-Report-Timezone":       report.Timezone,
		"X-Report-Group-Count":    report.TotalGroups,
		"X-Report-Byte-Count":     strconv.Itoa(len(body)),
		"X-Report-SHA256":         digest,
		"X-Report-Format-Version": "1",
		"X-Report-Audit-ID":       auditID,
	} {
		w.Header().Set(key, value)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func commissionAnalysisFailure(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, reporting.ErrAnalysisIntegrity):
		failure(w, r, 409, "COMMISSION_ANALYSIS_INTEGRITY", "佣金周期分析证据不完整，未返回报表")
	case errors.Is(err, reporting.ErrNotFound):
		failure(w, r, 404, "REPORT_SCOPE_NOT_FOUND", "当前品牌未找到佣金周期分析筛选资源")
	case errors.Is(err, reporting.ErrExportTooLarge):
		failure(w, r, 413, "REPORT_EXPORT_TOO_LARGE", "导出超过10000分组或4MiB，未生成截断文件")
	default:
		failure(w, r, 503, "REPORT_UNAVAILABLE", "无法完整生成或审计佣金周期分析，请稍后重试")
	}
}
