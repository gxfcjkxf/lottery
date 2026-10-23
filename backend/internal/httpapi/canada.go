package httpapi

import (
	"net/http"
)

func registerCanadaAdminRoutes(register func(string, string, http.HandlerFunc), d Dependencies, platform bool) {
	if platform {
		return
	}
	register("GET", "/platform-games/{id}/canada/orders", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := platformLotteryActor(w, r, d, "view")
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		_, offset, ok := pageParams(r)
		if !ok {
			failure(w, r, 400, "REQUEST_INVALID", "Invalid pagination")
			return
		}
		out, e := betService(d).CanadaOrders(r.Context(), b, "", id, offset)
		if e == nil && !adminReadAudit(w, r, d, a, b, "platform_game.view") {
			return
		}
		result, e := betError(map[string]any{"items": out}, e)
		outputMutation(w, r, result, e)
	})
}
func registerCanadaRoutes(mux routeRegistrar, d Dependencies) {
	for _, prefix := range []string{"/api/v1", "/api/v1/b/{brandCode}"} {
		handle := func(method, path string, fn http.HandlerFunc) {
			mux.HandleFunc(method+" "+prefix+"/canada"+path, func(w http.ResponseWriter, r *http.Request) {
				if bettingUnavailable(w, r, d) {
					return
				}
				fn(w, r)
			})
		}
		handle("GET", "/games/{id}/orders", func(w http.ResponseWriter, r *http.Request) {
			b, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			v, ok := currentBetSession(w, r, d, b.ID)
			if !ok {
				return
			}
			id, ok := ruleID(w, r, "id")
			if !ok {
				return
			}
			_, offset, ok := pageParams(r)
			if !ok {
				failure(w, r, 400, "REQUEST_INVALID", "Invalid pagination")
				return
			}
			out, e := betService(d).CanadaOrders(r.Context(), b.ID, v.Member.ID, id, offset)
			result, e := betError(map[string]any{"items": out}, e)
			outputMutation(w, r, result, e)
		})
		handle("GET", "/orders", func(w http.ResponseWriter, r *http.Request) {
			b, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			v, ok := currentBetSession(w, r, d, b.ID)
			if !ok {
				return
			}
			_, offset, ok := pageParams(r)
			if !ok {
				failure(w, r, 400, "REQUEST_INVALID", "Invalid pagination")
				return
			}
			out, e := betService(d).CanadaOrders(r.Context(), b.ID, v.Member.ID, "", offset)
			result, e := betError(map[string]any{"items": out}, e)
			outputMutation(w, r, result, e)
		})
	}
}
