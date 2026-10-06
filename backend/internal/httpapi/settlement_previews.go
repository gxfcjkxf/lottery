package httpapi

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
	"net/http"
)

func registerSettlementPreviewRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := betService(d)
	handle("GET", "/bet-orders/{id}/settlement-context", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := betAdminActor(w, r, d, "settlement", "view", false)
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		out, e := s.SettlementContext(r.Context(), brand, id)
		if e == nil && !adminReadAudit(w, r, d, a, brand, "settlement.context.view") {
			return
		}
		result, err := betError(out, e)
		outputMutation(w, r, result, err)
	})
	handle("GET", "/bet-orders/{id}/settlement-previews", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := betAdminActor(w, r, d, "settlement", "view", false)
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
		out, e := s.SettlementPreviews(r.Context(), brand, id, limit, offset)
		if e == nil && !adminReadAudit(w, r, d, a, brand, "settlement.previews.view") {
			return
		}
		result, err := betError(out, e)
		outputMutation(w, r, result, err)
	})
	handle("POST", "/bet-orders/{id}/settlement-previews", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := betAdminActor(w, r, d, "settlement", "preview", true)
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		var in betting.SettlementPreviewInput
		if !decodeBody(w, r, &in) {
			return
		}
		betAdminWrite(w, r, d, a, brand, "settlement", "preview", "admin.settlement.preview", id, in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, e := s.CreateSettlementPreview(ctx, tx, brand, fresh, id, in, bettingMeta(r, fresh))
			result, err := betError(out, e)
			if e == nil {
				result.Status = 201
			}
			return result, err
		})
	})
	handle("GET", "/settlement-previews/{id}", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := betAdminActor(w, r, d, "settlement", "view", false)
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		out, e := s.SettlementPreview(r.Context(), brand, id)
		if e == nil && !adminReadAudit(w, r, d, a, brand, "settlement.preview.view") {
			return
		}
		result, err := betError(out, e)
		outputMutation(w, r, result, err)
	})
	handle("GET", "/settlement-previews/{id}/lines", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := betAdminActor(w, r, d, "settlement", "view", false)
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
		out, e := s.SettlementPreviewLines(r.Context(), brand, id, limit, offset)
		if e == nil && !adminReadAudit(w, r, d, a, brand, "settlement.preview.lines.view") {
			return
		}
		result, err := betError(out, e)
		outputMutation(w, r, result, err)
	})
}
