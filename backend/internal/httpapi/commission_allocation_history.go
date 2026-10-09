package httpapi

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/jackc/pgx/v5"
)

func commissionAllocationQuery(r *http.Request, filters bool) (commission.AllocationQuery, bool) {
	q := commission.AllocationQuery{Limit: 20}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || r.URL.ForceQuery {
		return q, false
	}
	for key, list := range values {
		if len(list) != 1 || list[0] == "" {
			return q, false
		}
		switch key {
		case "limit", "offset":
			n, e := strconv.Atoi(list[0])
			if e != nil || strconv.Itoa(n) != list[0] {
				return q, false
			}
			if key == "limit" {
				q.Limit = n
			} else {
				q.Offset = n
			}
		case "agent_id", "order_id":
			if !filters || !cycleCanonicalID(list[0]) || strings.ToLower(list[0]) != list[0] {
				return q, false
			}
			value := list[0]
			if key == "agent_id" {
				q.AgentID = &value
			} else {
				q.OrderID = &value
			}
		default:
			return q, false
		}
	}
	return q, q.Limit >= 1 && q.Limit <= 100 && q.Offset >= 0 && q.Offset <= 1000000
}

func registerCommissionAllocationRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := commission.Service{DB: d.Admins.DB}
	for _, kind := range []string{"earnings", "allocations"} {
		handle("GET", "/commission-cycles/{id}/runs/{runID}/"+kind, func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength != 0 || len(r.TransferEncoding) > 0 {
				failure(w, r, 400, "REQUEST_INVALID", "查询接口不接受请求正文")
				return
			}
			brand, ok := commissionCycleBrand(w, r)
			if !ok {
				return
			}
			cycle, run := r.PathValue("id"), r.PathValue("runID")
			if !cycleCanonicalID(cycle) || !cycleCanonicalID(run) {
				failure(w, r, 400, "REQUEST_INVALID", "佣金周期或运行编号不正确")
				return
			}
			q, ok := commissionAllocationQuery(r, kind == "allocations")
			if !ok {
				failure(w, r, 400, "REQUEST_INVALID", "核算明细筛选或分页参数不正确")
				return
			}
			initial, ok := adminAccount(w, r, d)
			if !ok {
				return
			}
			out, err := commissionCycleRead(w, r, d, initial, brand, "view", "commission.cycle.run_"+kind, func(tx pgx.Tx) (any, error) {
				if kind == "earnings" {
					return s.EarningsForRunTx(r.Context(), tx, brand, cycle, run, q.Limit, q.Offset)
				}
				return s.AllocationsTx(r.Context(), tx, brand, cycle, run, q)
			}, map[string]any{"cycle_id": cycle, "run_id": run, "limit": q.Limit, "offset": q.Offset, "agent_id": q.AgentID, "order_id": q.OrderID})
			if commissionCycleReadFailure(w, r, err) {
				return
			}
			result, err := commissionCycleResult(out, err)
			outputMutation(w, r, result, err)
		})
	}
}
