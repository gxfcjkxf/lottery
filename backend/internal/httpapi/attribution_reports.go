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

var attributionReportTimestamp = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]{1,9})?(Z|[+-][0-9]{2}:[0-9]{2})$`)

func attributionReportQuery(r *http.Request, exporting bool) (reporting.AttributionQuery, error) {
	q := reporting.AttributionQuery{Limit: 20, AgentScope: "direct"}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || r.URL.ForceQuery {
		return q, reporting.ErrInvalid
	}
	for key, list := range values {
		if len(list) != 1 || list[0] == "" {
			return q, reporting.ErrInvalid
		}
		switch key {
		case "from", "to", "group_by", "game_id", "member_id", "agent_id", "agent_scope", "join_method":
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
	if !attributionReportTimestamp.MatchString(values.Get("from")) || !attributionReportTimestamp.MatchString(values.Get("to")) {
		return q, reporting.ErrInvalid
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
	for key, target := range map[string]**string{"game_id": &q.GameID, "member_id": &q.MemberID, "agent_id": &q.AgentID} {
		if raw, ok := values[key]; ok {
			value := strings.ToLower(raw[0])
			*target = &value
		}
	}
	if raw, ok := values["join_method"]; ok {
		value := raw[0]
		q.JoinMethod = &value
	}
	if raw, ok := values["agent_scope"]; ok {
		q.AgentScope = raw[0]
	}
	return q, q.Validate()
}

func attributionReportAllowed(a access.Account, brand, action string) bool {
	return access.Authorize(a, "report_attribution", action, access.ScopeBrand, brand) || access.Authorize(a, "report_attribution", action, access.ScopePlatform, "")
}

func registerAttributionReportRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	service := reporting.Service{DB: d.Admins.DB}
	for _, exporting := range []bool{false, true} {
		path := "/reports/attribution"
		if exporting {
			path += "/export"
		}
		handle("GET", path, func(w http.ResponseWriter, r *http.Request) {
			if !attributionReportGetHasNoBody(w, r) {
				return
			}
			q, err := attributionReportQuery(r, exporting)
			if err != nil {
				failure(w, r, 400, "REPORT_QUERY_INVALID", "归因报表筛选无效；时间范围最多93天，导出不接受分页参数")
				return
			}
			attributionReportRead(w, r, d, service, q, exporting)
		})
	}
}

func attributionReportGetHasNoBody(w http.ResponseWriter, r *http.Request) bool {
	if len(r.TransferEncoding) != 0 || r.ContentLength > 0 {
		failure(w, r, 400, "REPORT_QUERY_INVALID", "归因报表查询不接受请求正文")
		return false
	}
	if r.Body == nil || r.Body == http.NoBody {
		return true
	}
	probe, err := io.ReadAll(io.LimitReader(r.Body, 1))
	if len(probe) != 0 || err != nil && !errors.Is(err, io.EOF) {
		failure(w, r, 400, "REPORT_QUERY_INVALID", "归因报表查询不接受请求正文")
		return false
	}
	return true
}

func attributionReportRead(w http.ResponseWriter, r *http.Request, d Dependencies, service reporting.Service, q reporting.AttributionQuery, exporting bool) {
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
		attributionReportFailure(w, r, err, exporting)
		return
	}
	defer tx.Rollback(ctx)
	check := func() (access.Account, string, error) {
		a, e := freshAdmin(ctx, tx, r, d, actor, false)
		if e != nil {
			return a, "view", e
		}
		if !attributionReportAllowed(a, brand, "view") {
			return a, "view", adminsys.ErrDenied
		}
		if exporting && !attributionReportAllowed(a, brand, "export") {
			return a, "export", adminsys.ErrDenied
		}
		return a, "", nil
	}
	checkFailure := func(permission string, e error) {
		if errors.Is(e, adminsys.ErrDenied) {
			_, auditErr := audit.Append(ctx, tx, audit.Record{ActorType: "admin", ActorID: actor.ID, Action: "access.denied", ResourceType: "permission", Reason: "administrative permission denied", RequestID: requestID(r), IP: meta(r).IP, After: map[string]string{"attempted_brand": brand, "permission": "report_attribution." + permission}})
			if auditErr == nil {
				auditErr = tx.Commit(ctx)
			}
			if auditErr != nil {
				attributionReportFailure(w, r, auditErr, exporting)
				return
			}
			failure(w, r, 403, "PERMISSION_DENIED", "无归因报表对应查看或导出权限")
			return
		}
		if attributionReportAuthFailed(e) {
			failure(w, r, 401, "AUTH_SESSION_REVOKED", "当前管理授权已变化，请重新登录")
			return
		}
		attributionReportFailure(w, r, e, exporting)
	}
	if _, permission, checkErr := check(); checkErr != nil {
		checkFailure(permission, checkErr)
		return
	}
	if _, err = tx.Exec(ctx, `SAVEPOINT attribution_report_data`); err != nil {
		attributionReportFailure(w, r, err, exporting)
		return
	}
	var report reporting.AttributionReport
	if exporting {
		report, err = service.AttributionExport(ctx, tx, brand, q)
	} else {
		report, err = service.AttributionRead(ctx, tx, brand, q)
	}
	queryErr := err
	if queryErr != nil {
		if _, err = tx.Exec(ctx, `ROLLBACK TO SAVEPOINT attribution_report_data`); err != nil {
			attributionReportFailure(w, r, err, exporting)
			return
		}
	}
	if _, err = tx.Exec(ctx, `RELEASE SAVEPOINT attribution_report_data`); err != nil {
		attributionReportFailure(w, r, err, exporting)
		return
	}
	if _, permission, checkErr := check(); checkErr != nil {
		checkFailure(permission, checkErr)
		return
	}
	action, resource := "report.attribution.view", "query"
	if exporting {
		action, resource = "report.attribution.export", "report_export"
	}
	after := map[string]any{"kind": "attribution", "query": q, "outcome": "ready"}
	var body []byte
	var digest, filename string
	if queryErr != nil {
		after["outcome"] = "unavailable"
		if errors.Is(queryErr, reporting.ErrNotFound) {
			after["outcome"] = "scope_not_found"
		} else if errors.Is(queryErr, reporting.ErrExportTooLarge) {
			after["outcome"] = "too_large"
		}
	} else {
		after["snapshot_at"], after["timezone"], after["group_count"] = report.SnapshotAt.UTC(), report.Timezone, report.TotalGroups
		if exporting {
			body, queryErr = reporting.AttributionCSV(report)
			if queryErr == nil {
				sum := sha256.Sum256(body)
				digest = hex.EncodeToString(sum[:])
				after["format"], after["format_version"], after["byte_count"], after["sha256"] = "csv", 1, len(body), digest
				filename = fmt.Sprintf("lottery-attribution-%s-%s.csv", brand, report.SnapshotAt.UTC().Format("20060102T150405Z"))
			} else {
				after["outcome"] = "unavailable"
			}
		}
	}
	auditID, err := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: actor.ID, Action: action, ResourceType: resource, RequestID: requestID(r), IP: meta(r).IP, After: after})
	if err != nil {
		attributionReportFailure(w, r, err, exporting)
		return
	}
	if _, permission, checkErr := check(); checkErr != nil {
		checkFailure(permission, checkErr)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		attributionReportFailure(w, r, err, exporting)
		return
	}
	if queryErr != nil {
		attributionReportFailure(w, r, queryErr, exporting)
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
	for key, value := range map[string]string{"X-Report-Brand-ID": brand, "X-Report-Kind": "attribution", "X-Report-Snapshot-At": report.SnapshotAt.UTC().Format(time.RFC3339Nano), "X-Report-Timezone": report.Timezone, "X-Report-Group-Count": report.TotalGroups, "X-Report-Byte-Count": strconv.Itoa(len(body)), "X-Report-SHA256": digest, "X-Report-Format-Version": "1", "X-Report-Audit-ID": auditID, "X-Report-From": q.From.UTC().Format(time.RFC3339Nano), "X-Report-To": q.To.UTC().Format(time.RFC3339Nano), "X-Report-Group-By": q.GroupBy, "X-Report-Agent-Scope": q.AgentScope} {
		w.Header().Set(key, value)
	}
	if q.GameID != nil {
		w.Header().Set("X-Report-Game-ID", *q.GameID)
	}
	if q.MemberID != nil {
		w.Header().Set("X-Report-Member-ID", *q.MemberID)
	}
	if q.AgentID != nil {
		w.Header().Set("X-Report-Agent-ID", *q.AgentID)
	}
	if q.JoinMethod != nil {
		w.Header().Set("X-Report-Join-Method", *q.JoinMethod)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func attributionReportAuthFailed(err error) bool {
	return errors.Is(err, identity.ErrSession) || errors.Is(err, adminsys.ErrNotFound) || errors.Is(err, adminsys.ErrDenied)
}

func attributionReportFailure(w http.ResponseWriter, r *http.Request, err error, exporting bool) {
	switch {
	case errors.Is(err, reporting.ErrNotFound):
		failure(w, r, 404, "REPORT_SCOPE_NOT_FOUND", "当前品牌未找到归因报表筛选资源")
	case errors.Is(err, reporting.ErrExportTooLarge):
		failure(w, r, 413, "REPORT_EXPORT_TOO_LARGE", "导出超过10000分组或4MiB，请缩小范围；未生成截断文件")
	case exporting:
		failure(w, r, 503, "REPORT_EXPORT_UNAVAILABLE", "无法完整生成或审计归因报表导出，请稍后重试")
	default:
		failure(w, r, 503, "REPORT_UNAVAILABLE", "归因报表暂时不可用，请稍后重试")
	}
}
