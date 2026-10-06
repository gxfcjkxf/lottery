package httpapi

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
	"net/http"
)

func registerSettlementJobRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := betService(d)
	handle("GET", "/settlement-policy", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := betAdminActor(w, r, d, "settlement_policy", "view", false)
		if !ok {
			return
		}
		out, e := s.SettlementPolicy(r.Context(), b)
		if e == nil && !adminReadAudit(w, r, d, a, b, "settlement_policy.view") {
			return
		}
		result, err := betError(out, e)
		outputMutation(w, r, result, err)
	})
	handle("PUT", "/settlement-policy", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := betAdminActor(w, r, d, "settlement_policy", "write", true)
		if !ok {
			return
		}
		var in betting.SettlementPolicyInput
		if !decodeBody(w, r, &in) {
			return
		}
		betAdminWrite(w, r, d, a, b, "settlement_policy", "write", "admin.settlement_policy.write", b, in, func(ctx context.Context, tx pgx.Tx, f access.Account) (mutation.Result, error) {
			out, e := s.SaveSettlementPolicy(ctx, tx, b, f, in, bettingMeta(r, f))
			return betError(out, e)
		})
	})
	handle("GET", "/periods/{id}/settlement-context", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := betAdminActor(w, r, d, "settlement", "view", false)
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		out, e := s.PeriodSettlementContext(r.Context(), b, id)
		if e == nil && !adminReadAudit(w, r, d, a, b, "settlement.context.view") {
			return
		}
		result, err := betError(out, e)
		outputMutation(w, r, result, err)
	})
	handle("GET", "/periods/{id}/settlement", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := betAdminActor(w, r, d, "settlement", "view", false)
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		out, e := s.PeriodSettlementJob(r.Context(), b, id)
		if e == nil && !adminReadAudit(w, r, d, a, b, "settlement.view") {
			return
		}
		result, err := betError(map[string]any{"settlement": out}, e)
		outputMutation(w, r, result, err)
	})
	handle("POST", "/periods/{id}/settle", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := betAdminActor(w, r, d, "settlement", "run", true)
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		var in betting.SettlementStartInput
		if !decodeBody(w, r, &in) {
			return
		}
		betAdminWrite(w, r, d, a, b, "settlement", "run", "admin.settlement.run", id, in, func(ctx context.Context, tx pgx.Tx, f access.Account) (mutation.Result, error) {
			out, e := s.StartSettlement(ctx, tx, b, f, id, in, bettingMeta(r, f))
			result, err := betError(out, e)
			if e == nil {
				result.Status = 201
			}
			return result, err
		})
	})
	handle("GET", "/settlement-jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := betAdminActor(w, r, d, "settlement", "view", false)
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		out, e := s.SettlementJob(r.Context(), b, id)
		if e == nil && !adminReadAudit(w, r, d, a, b, "settlement.job.view") {
			return
		}
		result, err := betError(out, e)
		outputMutation(w, r, result, err)
	})
	handle("GET", "/settlement-jobs/{id}/targets", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := betAdminActor(w, r, d, "settlement", "view", false)
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		limit, offset, ok := pageParams(r)
		if !ok {
			failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
			return
		}
		out, e := s.SettlementTargets(r.Context(), b, id, limit, offset)
		if e == nil && !adminReadAudit(w, r, d, a, b, "settlement.targets.view") {
			return
		}
		result, err := betError(out, e)
		outputMutation(w, r, result, err)
	})
	for _, action := range []string{"approve", "retry"} {
		handle("POST", "/settlement-jobs/{id}/"+action, func(w http.ResponseWriter, r *http.Request) {
			a, b, ok := betAdminActor(w, r, d, "settlement", action, true)
			if !ok {
				return
			}
			id, ok := ruleID(w, r, "id")
			if !ok {
				return
			}
			var in betting.SettlementActionInput
			if !decodeBody(w, r, &in) {
				return
			}
			betAdminWrite(w, r, d, a, b, "settlement", action, "admin.settlement."+action, id, in, func(ctx context.Context, tx pgx.Tx, f access.Account) (mutation.Result, error) {
				out, e := s.ActOnSettlement(ctx, tx, b, f, id, action, in, bettingMeta(r, f))
				return betError(out, e)
			})
		})
	}
}
