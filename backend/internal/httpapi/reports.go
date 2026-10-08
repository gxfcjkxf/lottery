package httpapi

import (
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/reporting"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func reportQuery(r *http.Request, kind string) (reporting.Query, error) {
	out := reporting.Query{Limit: 20}
	values, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil {
		return out, reporting.ErrInvalid
	}
	// Reject ambiguous/unknown filters instead of silently broadening a report.
	for key, list := range values {
		if len(list) != 1 {
			return out, reporting.ErrInvalid
		}
		switch key {
		case "from", "to", "group_by", "limit", "offset", "member_id", "game_id":
		default:
			return out, reporting.ErrInvalid
		}
	}
	out.From, e = time.Parse(time.RFC3339Nano, values.Get("from"))
	if e != nil {
		return out, reporting.ErrInvalid
	}
	out.To, e = time.Parse(time.RFC3339Nano, values.Get("to"))
	if e != nil {
		return out, reporting.ErrInvalid
	}
	out.From, out.To = out.From.UTC(), out.To.UTC()
	out.GroupBy = values.Get("group_by")
	for _, name := range []string{"limit", "offset"} {
		if value, exists := values[name]; exists {
			n, e := strconv.Atoi(value[0])
			if e != nil {
				return out, reporting.ErrInvalid
			}
			if name == "limit" {
				out.Limit = n
			} else {
				out.Offset = n
			}
		}
	}
	if value, exists := values["game_id"]; exists {
		out.GameID = &value[0]
	}
	if value, exists := values["member_id"]; exists {
		out.MemberID = &value[0]
	}
	return out, out.Validate(kind)
}
func registerReportAdminRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	registerWithdrawalReportRoutes(handle, d)
	registerCommissionReportRoutes(handle, d)
	registerRewardReportRoutes(handle, d)
	registerReportExportRoutes(handle, d)
	s := reporting.Service{DB: d.Admins.DB}
	for _, kind := range []string{"betting", "ledger"} {
		handle("GET", "/reports/"+kind, func(w http.ResponseWriter, r *http.Request) {
			a, brand, ok := pointAdminActor(w, r, d, "report_"+kind, "view", false)
			if !ok {
				return
			}
			q, e := reportQuery(r, kind)
			if e != nil {
				failure(w, r, 400, "REPORT_QUERY_INVALID", "报表筛选参数不正确，请指定最多93天的起止时间")
				return
			}
			var data any
			if kind == "betting" {
				data, e = s.Betting(r.Context(), brand, q)
			} else {
				data, e = s.Ledger(r.Context(), brand, q)
			}
			if errors.Is(e, reporting.ErrNotFound) {
				failure(w, r, 404, "REPORT_SCOPE_NOT_FOUND", "当前品牌未找到对应报表筛选资源")
				return
			}
			if e != nil {
				failure(w, r, 503, "REPORT_UNAVAILABLE", "报表暂时不可用，请稍后重试")
				return
			}
			if !adminReadAudit(w, r, d, a, brand, "report."+kind+".view") {
				return
			}
			respond(w, r, 200, data)
		})
	}
}
