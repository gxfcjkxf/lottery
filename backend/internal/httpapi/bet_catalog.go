package httpapi

import (
	"context"
	"net/http"
	"time"
)

func registerBetCatalogRoutes(mux *http.ServeMux, d Dependencies) {
	for _, prefix := range []string{"/api/v1", "/api/v1/b/{brandCode}"} {
		handle := func(path string, fn http.HandlerFunc) {
			mux.HandleFunc("GET "+prefix+path, func(w http.ResponseWriter, r *http.Request) {
				if d.Admins.DB == nil {
					failure(w, r, 503, "BETTING_UNAVAILABLE", "彩种服务尚未配置")
					return
				}
				ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
				defer cancel()
				fn(w, r.WithContext(ctx))
			})
		}
		handle("/games", func(w http.ResponseWriter, r *http.Request) {
			brand, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			limit, offset, ok := pageParams(r)
			if !ok {
				failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
				return
			}
			items, err := betService(d).Games(r.Context(), brand.ID, limit, offset)
			result, e := betError(map[string]any{"items": items, "limit": limit, "offset": offset}, err)
			outputMutation(w, r, result, e)
		})
		for _, suffix := range []string{"", "/plays", "/periods/current"} {
			handle("/games/{id}"+suffix, func(w http.ResponseWriter, r *http.Request) {
				brand, ok := resolveBrand(w, r, d)
				if !ok {
					return
				}
				out, err := betService(d).Catalog(r.Context(), brand.ID, r.PathValue("id"))
				var data any = out
				if err == nil {
					switch suffix {
					case "/plays":
						data = map[string]any{"items": out.Plays}
					case "/periods/current":
						data = map[string]any{"period": out.Period, "server_time": out.ServerTime, "brand_status": out.BrandStatus}
					}
				}
				result, e := betError(data, err)
				outputMutation(w, r, result, e)
			})
		}
	}
}
