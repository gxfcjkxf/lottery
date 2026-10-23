package httpapi

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/jackc/pgx/v5"
	"net/http"
)

func registerPlatformAutomationRoutes(register func(string, string, http.HandlerFunc), d Dependencies, platform bool) {
	s := platformlottery.Service{DB: d.Admins.DB}
	if platform {
		registerDrawSourceRoutes(register, d)
	}
	register("GET", "/platform-games/{id}/automation", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := platformLotteryActor(w, r, d, "view")
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		out, e := s.AutomationConfig(r.Context(), b, id)
		if e == nil {
			sources, err := platformlottery.LoadAutomationSources()
			if err != nil {
				e = err
			} else {
				out.AllowedOrigins = sources.Origins
			}
		}
		if e == nil && !adminReadAudit(w, r, d, a, b, "platform_game.automation.view") {
			return
		}
		result, e := platformLotteryResult(200, out, e)
		outputMutation(w, r, result, e)
	})
	if platform {
		return
	}
	register("POST", "/platform-games/{id}/automation", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := platformLotteryActor(w, r, d, "sell")
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		var in platformlottery.BrandAutomationInput
		if !decodeBody(w, r, &in) {
			return
		}
		platformLotteryWrite(w, r, d, a, b, "sell", "platform.automation.sales", in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, e := s.WriteBrandAutomation(ctx, tx, fresh, b, id, in, pointMeta(r, fresh))
			return platformLotteryResult(200, out, e)
		})
	})
}
