package httpapi

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/jackc/pgx/v5"
	"net/http"
)

func registerPlatformLifecycleRoutes(register func(string, string, http.HandlerFunc), d Dependencies, platform bool) {
	s := platformlottery.Service{DB: d.Admins.DB}
	if platform {
		register("POST", "/platform-games/{id}/lifecycle", func(w http.ResponseWriter, r *http.Request) {
			a, b, ok := platformLotteryActor(w, r, d, "write")
			if !ok {
				return
			}
			id, ok := ruleID(w, r, "id")
			if !ok {
				return
			}
			var in platformlottery.LifecycleInput
			if !decodeBody(w, r, &in) {
				return
			}
			platformLotteryWrite(w, r, d, a, b, "write", "platform.game.lifecycle", in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
				out, e := s.SetLifecycle(ctx, tx, fresh, id, in, pointMeta(r, fresh))
				return platformLotteryResult(200, out, e)
			})
		})
	} else {
		register("POST", "/platform-games/{id}/availability", func(w http.ResponseWriter, r *http.Request) {
			a, b, ok := platformLotteryActor(w, r, d, "sell")
			if !ok {
				return
			}
			id, ok := ruleID(w, r, "id")
			if !ok {
				return
			}
			var in platformlottery.AvailabilityInput
			if !decodeBody(w, r, &in) {
				return
			}
			platformLotteryWrite(w, r, d, a, b, "sell", "brand.platform-game.availability", in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
				out, e := s.SetAvailability(ctx, tx, fresh, b, id, in, pointMeta(r, fresh))
				return platformLotteryResult(200, out, e)
			})
		})
	}
}
