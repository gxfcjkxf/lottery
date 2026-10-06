package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/betting"
)

func publicDrawFilter(w http.ResponseWriter, r *http.Request) (betting.PublicDrawFilter, bool) {
	limit, offset, ok := pageParams(r)
	if !ok {
		failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
		return betting.PublicDrawFilter{}, false
	}
	for _, key := range []string{"game_id", "period_no", "limit", "offset"} {
		if len(r.URL.Query()[key]) > 1 {
			failure(w, r, 400, "REQUEST_INVALID", "查询参数不能重复")
			return betting.PublicDrawFilter{}, false
		}
	}
	return betting.PublicDrawFilter{GameID: r.URL.Query().Get("game_id"), PeriodNo: r.URL.Query().Get("period_no"), Limit: limit, Offset: offset}, true
}
func registerPublicDrawRoutes(mux *http.ServeMux, d Dependencies) {
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
		handle("/draw-results", func(w http.ResponseWriter, r *http.Request) {
			brand, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			f, ok := publicDrawFilter(w, r)
			if !ok {
				return
			}
			out, err := betService(d).PublicDraws(r.Context(), brand.ID, f)
			result, e := betError(out, err)
			outputMutation(w, r, result, e)
		})
		handle("/draw-results/{id}", func(w http.ResponseWriter, r *http.Request) {
			brand, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			out, err := betService(d).PublicDraw(r.Context(), brand.ID, r.PathValue("id"))
			result, e := betError(out, err)
			outputMutation(w, r, result, e)
		})
		handle("/games/{id}/periods", func(w http.ResponseWriter, r *http.Request) {
			brand, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			f, ok := publicDrawFilter(w, r)
			if !ok {
				return
			}
			out, err := betService(d).PublicPeriods(r.Context(), brand.ID, r.PathValue("id"), f)
			result, e := betError(out, err)
			outputMutation(w, r, result, e)
		})
	}
}
