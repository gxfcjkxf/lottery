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

var rewardReportTimestamp = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]{1,9})?(Z|[+-][0-9]{2}:[0-9]{2})$`)

func rewardReportQuery(r *http.Request, kind string, exporting bool) (reporting.RewardQuery, error) {
	q := reporting.RewardQuery{Limit: 20}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || r.URL.ForceQuery {
		return q, reporting.ErrInvalid
	}
	for key, list := range values {
		if len(list) != 1 || list[0] == "" {
			return q, reporting.ErrInvalid
		}
		switch key {
		case "from", "to", "group_by", "member_id", "order_id":
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
	if !rewardReportTimestamp.MatchString(values.Get("from")) || !rewardReportTimestamp.MatchString(values.Get("to")) {
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
			n, e := strconv.Atoi(raw[0])
			if e != nil {
				return q, reporting.ErrInvalid
			}
			if key == "limit" {
				q.Limit = n
			} else {
				q.Offset = n
			}
		}
	}
	for name, target := range map[string]**string{"member_id": &q.MemberID, "order_id": &q.OrderID} {
		if raw, ok := values[name]; ok {
			id := strings.ToLower(raw[0])
			*target = &id
		}
	}
	return q, q.Validate(kind)
}

func registerRewardReportRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	service := reporting.Service{DB: d.Admins.DB}
	for _, kind := range []string{"rewards", "reward_orders"} {
		path := "/reports/rewards"
		if kind == "reward_orders" {
			path = "/reports/reward-orders"
		}
		for _, exporting := range []bool{false, true} {
			target := path
			if exporting {
				target += ".csv"
			}
			handle("GET", target, func(w http.ResponseWriter, r *http.Request) {
				if !rewardGetHasNoBody(w, r) {
					return
				}
				q, err := rewardReportQuery(r, kind, exporting)
				if err != nil {
					failure(w, r, 400, "REPORT_QUERY_INVALID", "奖励报表筛选无效；窗口最多93天，导出不接受分页参数")
					return
				}
				rewardReportRead(w, r, d, kind, q, exporting, func(ctx context.Context, tx pgx.Tx, brand string) (any, error) {
					if kind == "rewards" {
						if exporting {
							return service.RewardExport(ctx, tx, brand, q)
						}
						return service.RewardRead(ctx, tx, brand, q)
					}
					if exporting {
						return service.RewardOrdersExport(ctx, tx, brand, q)
					}
					return service.RewardOrdersRead(ctx, tx, brand, q)
				})
			})
		}
	}
}

func rewardReportAllowed(a access.Account, brand, action string) bool {
	return access.Authorize(a, "report_reward", action, access.ScopeBrand, brand) || access.Authorize(a, "report_reward", action, access.ScopePlatform, "")
}

func rewardReportRead(w http.ResponseWriter, r *http.Request, d Dependencies, kind string, q reporting.RewardQuery, exporting bool, query func(context.Context, pgx.Tx, string) (any, error)) {
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
		rewardReportFailure(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	check := func() (access.Account, string, error) {
		a, e := freshAdmin(ctx, tx, r, d, actor, false)
		if e != nil {
			return a, "view", e
		}
		if !rewardReportAllowed(a, brand, "view") {
			return a, "view", adminsys.ErrDenied
		}
		if exporting && !rewardReportAllowed(a, brand, "export") {
			return a, "export", adminsys.ErrDenied
		}
		return a, "", nil
	}
	fresh, denied, err := check()
	if errors.Is(err, adminsys.ErrDenied) {
		_, err = audit.Append(ctx, tx, audit.Record{ActorType: "admin", ActorID: actor.ID, Action: "access.denied", ResourceType: "permission", Reason: "administrative permission denied", RequestID: requestID(r), IP: meta(r).IP, After: map[string]string{"attempted_brand": brand, "permission": "report_reward." + denied}})
		if err == nil {
			err = tx.Commit(ctx)
		}
		if err != nil {
			rewardReportFailure(w, r, err)
			return
		}
		failure(w, r, 403, "PERMISSION_DENIED", "无奖励报表对应查看或导出权限")
		return
	}
	if rewardReportAuthFailed(err) {
		failure(w, r, 401, "AUTH_SESSION_REVOKED", "当前管理授权已变化，请重新登录")
		return
	}
	if err != nil {
		rewardReportFailure(w, r, err)
		return
	}
	// One SQL statement produces totals/groups/count/timezone at one snapshot.
	// Keep fresh authorization checks in READ COMMITTED, including AFTER the
	// audit wait; successful computation alone never authorizes data release.
	if _, err = tx.Exec(ctx, `SAVEPOINT reward_report_data`); err != nil {
		rewardReportFailure(w, r, err)
		return
	}
	data, queryErr := query(ctx, tx, brand)
	if queryErr != nil {
		if _, err = tx.Exec(ctx, `ROLLBACK TO SAVEPOINT reward_report_data`); err != nil {
			rewardReportFailure(w, r, err)
			return
		}
	}
	if _, err = tx.Exec(ctx, `RELEASE SAVEPOINT reward_report_data`); err != nil {
		rewardReportFailure(w, r, err)
		return
	}
	if _, _, err = check(); rewardReportAuthFailed(err) {
		failure(w, r, 401, "AUTH_SESSION_REVOKED", "当前管理授权已变化，请重新登录")
		return
	} else if err != nil {
		rewardReportFailure(w, r, err)
		return
	}
	action, resource := "report."+kind+".view", "query"
	if exporting {
		action, resource = "report."+kind+".export", "report_export"
	}
	after := map[string]any{"kind": kind, "query": q, "outcome": "ready"}
	var body []byte
	var snapshot time.Time
	var groups, timezone, digest, filename string
	if queryErr == nil {
		switch report := data.(type) {
		case reporting.RewardReport:
			snapshot, groups, timezone = report.SnapshotAt, report.TotalGroups, report.Timezone
			if exporting {
				body, queryErr = reporting.RewardCSV(report)
			}
		case reporting.RewardOrderReport:
			snapshot, groups, timezone = report.SnapshotAt, report.TotalGroups, report.Timezone
			if exporting {
				body, queryErr = reporting.RewardOrdersCSV(report)
			}
		default:
			queryErr = fmt.Errorf("unexpected reward report type")
		}
	}
	if queryErr != nil {
		after["outcome"] = "unavailable"
		if errors.Is(queryErr, reporting.ErrNotFound) {
			after["outcome"] = "scope_not_found"
		} else if errors.Is(queryErr, reporting.ErrExportTooLarge) {
			after["outcome"] = "too_large"
		}
	} else {
		after["snapshot_at"], after["timezone"], after["group_count"] = snapshot.UTC(), timezone, groups
		if exporting {
			sum := sha256.Sum256(body)
			digest = hex.EncodeToString(sum[:])
			after["format"], after["format_version"], after["byte_count"], after["sha256"] = "csv", 1, len(body), digest
			filename = fmt.Sprintf("lottery-%s-%s-%s.csv", kind, brand, snapshot.UTC().Format("20060102T150405Z"))
		}
	}
	auditID, err := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: fresh.ID, Action: action, ResourceType: resource, RequestID: requestID(r), IP: meta(r).IP, After: after})
	if err != nil {
		rewardReportFailure(w, r, err)
		return
	}
	if _, _, err = check(); rewardReportAuthFailed(err) {
		failure(w, r, 401, "AUTH_SESSION_REVOKED", "当前管理授权已变化，请重新登录")
		return
	} else if err != nil {
		rewardReportFailure(w, r, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		rewardReportFailure(w, r, err)
		return
	}
	if queryErr != nil {
		rewardReportFailure(w, r, queryErr)
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
	for key, value := range map[string]string{"X-Report-Brand-ID": brand, "X-Report-Kind": kind, "X-Report-Snapshot-At": snapshot.UTC().Format(time.RFC3339Nano), "X-Report-Timezone": timezone, "X-Report-Group-Count": groups, "X-Report-Byte-Count": strconv.Itoa(len(body)), "X-Report-SHA256": digest, "X-Report-Format-Version": "1", "X-Report-Audit-ID": auditID, "X-Report-From": q.From.UTC().Format(time.RFC3339Nano), "X-Report-To": q.To.UTC().Format(time.RFC3339Nano), "X-Report-Group-By": q.GroupBy} {
		w.Header().Set(key, value)
	}
	if q.MemberID != nil {
		w.Header().Set("X-Report-Member-ID", *q.MemberID)
	}
	if q.OrderID != nil {
		w.Header().Set("X-Report-Order-ID", *q.OrderID)
	}
	w.WriteHeader(200)
	_, _ = w.Write(body)
}
func rewardReportAuthFailed(err error) bool {
	return errors.Is(err, identity.ErrSession) || errors.Is(err, adminsys.ErrNotFound) || errors.Is(err, adminsys.ErrDenied)
}
func rewardReportFailure(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, reporting.ErrNotFound):
		failure(w, r, 404, "REPORT_SCOPE_NOT_FOUND", "当前品牌未找到奖励筛选资源")
	case errors.Is(err, reporting.ErrExportTooLarge):
		failure(w, r, 413, "REPORT_EXPORT_TOO_LARGE", "导出超过10000分组或4MiB，请缩小范围；未生成截断文件")
	default:
		failure(w, r, 503, "REPORT_UNAVAILABLE", "无法完整生成或审计奖励报表，请稍后重试")
	}
}
