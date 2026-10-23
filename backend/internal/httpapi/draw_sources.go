package httpapi

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/jackc/pgx/v5"
	"net/http"
)

func registerDrawSourceRoutes(register func(string, string, http.HandlerFunc), d Dependencies) {
	registerDrawSourcePreview(register, d)
	s := platformlottery.Service{DB: d.Admins.DB}
	register("GET", "/platform-games/{id}/draw-sources", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := platformLotteryActor(w, r, d, "view")
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		out, e := s.DrawSources(r.Context(), id)
		if e == nil && !adminReadAudit(w, r, d, a, b, "platform_game.draw_source.view") {
			return
		}
		result, e := platformLotteryResult(200, out, e)
		outputMutation(w, r, result, e)
	})
	register("POST", "/platform-games/{id}/draw-sources", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := platformLotteryActor(w, r, d, "write")
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		var in platformlottery.DrawSourceInput
		if !decodeBody(w, r, &in) {
			return
		}
		if in.ID != "" && !uuidPattern.MatchString(in.ID) {
			failure(w, r, 400, "REQUEST_INVALID", "Invalid source identifier")
			return
		}
		platformLotteryWrite(w, r, d, a, b, "write", "platform.draw_source.save", in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, e := s.SaveDrawSource(ctx, tx, fresh, id, in, pointMeta(r, fresh))
			return platformLotteryResult(200, out, e)
		})
	})
	register("POST", "/platform-games/{id}/draw-sources/{sourceId}/activate", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := platformLotteryActor(w, r, d, "write")
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		source, ok := ruleID(w, r, "sourceId")
		if !ok {
			return
		}
		var in platformlottery.DrawSourceActivation
		if !decodeBody(w, r, &in) {
			return
		}
		platformLotteryWrite(w, r, d, a, b, "write", "platform.draw_source.activate", in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			list, e := s.DrawSources(ctx, id)
			if e != nil {
				return platformLotteryResult(200, nil, e)
			}
			found := false
			for _, item := range list.Items {
				if item.ID != source || item.Revision != in.Revision {
					continue
				}
				found = true
				sources, e := platformlottery.LoadAutomationSources()
				if e != nil {
					return mutation.Result{}, e
				}
				if item.Plan.Enabled && !sources.Allows(item.Plan.SourceURL) {
					return mutation.Fail(400, "AUTOMATION_SOURCE_NOT_ALLOWED", "Draw source origin is not configured on this server"), nil
				}
			}
			if !found {
				return platformLotteryResult(200, nil, platformlottery.ErrVersionConflict)
			}
			out, e := s.ActivateDrawSource(ctx, tx, fresh, id, source, in, pointMeta(r, fresh))
			return platformLotteryResult(200, out, e)
		})
	})
}
