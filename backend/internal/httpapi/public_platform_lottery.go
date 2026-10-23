package httpapi

import (
	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"net/http"
)

func registerPublicPlatformLotteryRoutes(mux routeRegistrar, d Dependencies) {
	for _, prefix := range []string{"/api/v1", "/api/v1/b/{brandCode}"} {
		mux.HandleFunc("GET "+prefix+"/platform-games", func(w http.ResponseWriter, r *http.Request) {
			b, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			limit, offset, ok := pageParams(r)
			if !ok {
				failure(w, r, 400, "REQUEST_INVALID", "Invalid pagination")
				return
			}
			s := platformlottery.Service{DB: d.Admins.DB}
			out, e := s.ConnectedGames(r.Context(), b.ID, limit, offset)
			result, e := platformLotteryResult(200, map[string]any{"games": out, "limit": limit, "offset": offset}, e)
			outputMutation(w, r, result, e)
		})
		mux.HandleFunc("GET "+prefix+"/platform-games/{id}", func(w http.ResponseWriter, r *http.Request) {
			b, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			id, ok := ruleID(w, r, "id")
			if !ok {
				return
			}
			limit, offset, ok := pageParams(r)
			if !ok {
				failure(w, r, 400, "REQUEST_INVALID", "Invalid pagination")
				return
			}
			s := platformlottery.Service{DB: d.Admins.DB}
			out, e := s.Detail(r.Context(), b.ID, id, limit, offset)
			result, e := platformLotteryResult(200, out, e)
			outputMutation(w, r, result, e)
		})
	}
}
