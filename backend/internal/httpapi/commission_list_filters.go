package httpapi

import (
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func commissionListFilter(r *http.Request, kind string) (int, int, commission.ListFilter, bool) {
	limit, offset := 20, 0
	var filter commission.ListFilter
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || r.URL.ForceQuery || r.ContentLength != 0 || len(r.TransferEncoding) > 0 {
		return 0, 0, filter, false
	}
	for key, values := range query {
		if len(values) != 1 || values[0] == "" {
			return 0, 0, filter, false
		}
		value := values[0]
		switch key {
		case "limit", "offset":
			n, e := strconv.Atoi(value)
			if e != nil || strconv.Itoa(n) != value {
				return 0, 0, filter, false
			}
			if key == "limit" {
				limit = n
			} else {
				offset = n
			}
		case "from", "to":
			date, e := time.Parse(time.RFC3339Nano, value)
			if e != nil {
				return 0, 0, filter, false
			}
			if key == "from" {
				filter.From = &date
			} else {
				filter.To = &date
			}
		case "state":
			filter.State = value
		case "cycle_id":
			filter.CycleID = value
		case "agent_id":
			filter.AgentID = value
		default:
			return 0, 0, filter, false
		}
	}
	valid := limit >= 1 && limit <= 100 && offset >= 0 && offset <= 1000000 && filter.Validate(kind) == nil
	return limit, offset, filter, valid
}
