package httpapi

import (
	"context"
	"net/http"
	"time"
)

func registerPublicDrawRoutes(mux routeRegistrar, d Dependencies) {
	for _, prefix := range []string{"/api/v1", "/api/v1/b/{brandCode}"} {
		handle := func(path string, fn http.HandlerFunc) {
			mux.HandleFunc("GET "+prefix+path, func(w http.ResponseWriter, r *http.Request) {
				if d.Admins.DB == nil {
					failure(w, r, 503, "DRAWS_UNAVAILABLE", "开奖服务尚未配置")
					return
				}
				ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
				defer cancel()
				fn(w, r.WithContext(ctx))
			})
		}
		handle("/draw-archive", func(w http.ResponseWriter, r *http.Request) {
			brand, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			limit, offset, ok := pageParams(r)
			if !ok {
				failure(w, r, 400, "REQUEST_INVALID", "Invalid pagination")
				return
			}
			for _, key := range []string{"game_id", "kind", "selected", "limit", "offset"} {
				if len(r.URL.Query()[key]) > 1 {
					failure(w, r, 400, "REQUEST_INVALID", "Duplicate parameter")
					return
				}
			}
			out, err := betService(d).DrawArchive(r.Context(), brand.ID, r.URL.Query().Get("game_id"), r.URL.Query().Get("kind"), r.URL.Query().Get("selected"), limit, offset)
			result, e := betError(out, err)
			outputMutation(w, r, result, e)
		})
	}
}
