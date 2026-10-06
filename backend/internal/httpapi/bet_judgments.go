package httpapi

import (
	"context"
	"net/http"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
)

func registerBetJudgmentRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := betService(d)
	handle("GET", "/bet-orders/{id}/judgment", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := betAdminActor(w, r, d, "bet", "view", false)
		if !ok {
			return
		}
		out, err := s.Judgment(r.Context(), brand, r.PathValue("id"))
		if err == nil && !adminReadAudit(w, r, d, a, brand, "bet.judgment.view") {
			return
		}
		result, e := betError(map[string]any{"judgment": out}, err)
		outputMutation(w, r, result, e)
	})
	handle("POST", "/bet-orders/{id}/judge-cancel", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := betAdminActor(w, r, d, "bet", "judge_cancel", true)
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		var in betting.JudgeCancelInput
		if !decodeBody(w, r, &in) {
			return
		}
		betAdminWrite(w, r, d, a, brand, "bet", "judge_cancel", "admin.bet.judge_cancel", id, in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, err := s.JudgeCancel(ctx, tx, brand, fresh, id, in, bettingMeta(r, fresh))
			return betError(out, err)
		})
	})
}
