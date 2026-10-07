package httpapi

import (
	"context"
	"net/http"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
)

const commissionDiscoveryPath = "/commission-discovery"

func registerCommissionDiscoveryRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := commission.Service{DB: d.Admins.DB}
	handle("GET", commissionDiscoveryPath, func(w http.ResponseWriter, r *http.Request) {
		brand, ok := commissionCycleBrand(w, r)
		if !ok {
			return
		}
		limit, offset, ok := commissionCyclePage(r)
		if !ok {
			failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
			return
		}
		initial, ok := adminAccount(w, r, d)
		if !ok {
			return
		}
		out, err := commissionCycleRead(w, r, d, initial, brand, "view", "commission.discovery.list", func(tx pgx.Tx) (any, error) {
			return s.DiscoveriesTx(r.Context(), tx, brand, limit, offset)
		})
		if commissionCycleReadFailure(w, r, err) {
			return
		}
		result, err := commissionCycleResult(out, err)
		outputMutation(w, r, result, err)
	})

	handle("POST", commissionDiscoveryPath+"/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		brand, ok := commissionCycleBrand(w, r)
		if !ok {
			return
		}
		id := r.PathValue("id")
		if !cycleCanonicalID(id) {
			failure(w, r, 400, "REQUEST_INVALID", "发现记录编号不正确")
			return
		}
		if r.URL.RawQuery != "" {
			failure(w, r, 400, "REQUEST_INVALID", "此写接口不接受查询参数")
			return
		}
		var in commission.RetryCycleInput
		raw, ok := decodeCommissionCycleBody(w, r, &in)
		if !ok {
			return
		}
		initial, ok := adminAccount(w, r, d)
		if !ok {
			return
		}
		var fresh access.Account
		result, err := d.Mutations.ExecuteChecked(r.Context(), brand, initial.ID, "admin.commission_discovery.retry", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(id+":"+string(raw)), func(ctx context.Context, tx pgx.Tx) error {
			return commissionCycleWriteCheck(ctx, tx, r, d, initial, brand, "retry", &fresh)
		}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			discovery, err := s.RetryDiscoveryTx(ctx, tx, brand, id, fresh, in, pointMeta(r, fresh))
			if err != nil {
				return mutation.Result{}, err
			}
			return commissionCycleResult(discovery, nil)
		})
		commissionCycleWriteFailure(w, r, d, initial, brand, "discovery.retry", result, err)
	})

}
