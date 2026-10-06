package httpapi

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
	"net/http"
)

func registerCorrectionRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := betService(d)
	handle("GET", "/periods/{id}/correction-context", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := betAdminActor(w, r, d, "draw", "view", false)
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		out, e := s.CorrectionContext(r.Context(), b, id)
		if e == nil && !adminReadAudit(w, r, d, a, b, "draw.correction_context.view") {
			return
		}
		result, err := betError(out, e)
		outputMutation(w, r, result, err)
	})
	handle("POST", "/draw-results/{id}/correct", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := betAdminActor(w, r, d, "draw", "correct", true)
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		var in betting.CorrectionInput
		if !decodeBody(w, r, &in) {
			return
		}
		betAdminWriteExtra(w, r, d, a, b, "draw", "correct", "admin.draw.correct", id, in, func(f access.Account) bool {
			return in.PolicyVersion == nil || access.Authorize(f, "settlement", "run", access.ScopeBrand, b)
		}, func(ctx context.Context, tx pgx.Tx, f access.Account) (mutation.Result, error) {
			out, e := s.CreateCorrection(ctx, tx, b, f, id, in, bettingMeta(r, f))
			result, err := betError(out, e)
			if e == nil {
				result.Status = 201
			}
			return result, err
		})
	})
	handle("GET", "/periods/{id}/corrections", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := betAdminActor(w, r, d, "draw", "view", false)
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
		out, e := s.Corrections(r.Context(), b, id, limit, offset)
		if e == nil && !adminReadAudit(w, r, d, a, b, "draw.corrections.view") {
			return
		}
		result, err := betError(out, e)
		outputMutation(w, r, result, err)
	})
	handle("GET", "/corrections/{id}", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := betAdminActor(w, r, d, "draw", "view", false)
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		out, e := s.Correction(r.Context(), b, id)
		if e == nil && !adminReadAudit(w, r, d, a, b, "draw.correction.view") {
			return
		}
		result, err := betError(out, e)
		outputMutation(w, r, result, err)
	})
	handle("GET", "/corrections/{id}/targets", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := betAdminActor(w, r, d, "draw", "view", false)
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
		out, e := s.CorrectionTargets(r.Context(), b, id, limit, offset)
		if e == nil && !adminReadAudit(w, r, d, a, b, "draw.correction_targets.view") {
			return
		}
		result, err := betError(out, e)
		outputMutation(w, r, result, err)
	})
	handle("POST", "/corrections/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := betAdminActor(w, r, d, "draw", "correction_retry", true)
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
		betAdminWriteExtra(w, r, d, a, b, "draw", "correction_retry", "admin.draw.correction_retry", id, in, func(f access.Account) bool { return access.Authorize(f, "settlement", "run", access.ScopeBrand, b) }, func(ctx context.Context, tx pgx.Tx, f access.Account) (mutation.Result, error) {
			out, e := s.RetryCorrection(ctx, tx, b, f, id, in, bettingMeta(r, f))
			return betError(out, e)
		})
	})
}
