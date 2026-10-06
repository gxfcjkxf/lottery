package httpapi

import (
	"context"
	"net/http"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
)

func registerBetExceptionRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := betService(d)
	handle("GET", "/bet-orders/{id}/exception", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := betAdminActor(w, r, d, "bet", "view", false)
		if !ok {
			return
		}
		out, err := s.Exception(r.Context(), brand, r.PathValue("id"))
		if err == nil && !adminReadAudit(w, r, d, a, brand, "bet.exception.view") {
			return
		}
		result, e := betError(map[string]any{"exception": out}, err)
		outputMutation(w, r, result, e)
	})
	handle("POST", "/bet-orders/{id}/abnormal", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := betAdminActor(w, r, d, "bet", "mark_abnormal", true)
		if !ok {
			return
		}
		id := r.PathValue("id")
		if !uuidPattern.MatchString(id) {
			failure(w, r, 400, "REQUEST_INVALID", "注单编号不正确")
			return
		}
		var in struct {
			Version int64  `json:"version"`
			Reason  string `json:"reason"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		betAdminWrite(w, r, d, a, brand, "bet", "mark_abnormal", "admin.bet.mark_abnormal", id, in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, err := s.MarkAbnormal(ctx, tx, brand, fresh, id, in.Version, in.Reason, bettingMeta(r, fresh))
			return betError(out, err)
		})
	})
}
