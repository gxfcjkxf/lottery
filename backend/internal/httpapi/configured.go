package httpapi

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"net/http"
)

func registerConfiguredAdminRoutes(register func(string, string, http.HandlerFunc), d Dependencies, platform bool) {
	svc := platformlottery.Service{DB: d.Admins.DB}
	if !platform {
		register("GET", "/platform-games/{id}/configured/orders", func(w http.ResponseWriter, r *http.Request) {
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
				failure(w, r, 400, "REQUEST_INVALID", "Invalid paging")
				return
			}
			if _, e := svc.Detail(r.Context(), b, id, 1, 0); e != nil {
				out, e := platformLotteryResult(200, nil, e)
				outputMutation(w, r, out, e)
				return
			}
			orders, e := betService(d).ConfiguredOrders(r.Context(), b, "", id, offset)
			if e == nil && !adminReadAudit(w, r, d, a, b, "platform_game.view") {
				return
			}
			out, e := configuredResult(map[string]any{"items": orders}, e)
			outputMutation(w, r, out, e)
		})
	}
	register("GET", "/platform-games/{id}/configured/versions", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := platformLotteryActor(w, r, d, "view")
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		rows, e := svc.ConfiguredVersions(r.Context(), b, id)
		if e == nil && !adminReadAudit(w, r, d, a, b, "platform_game.view") {
			return
		}
		result, e := platformLotteryResult(200, map[string]any{"items": rows}, e)
		outputMutation(w, r, result, e)
	})
	register("GET", "/platform-games/{id}/configured/sales", func(w http.ResponseWriter, r *http.Request) {
		_, b, ok := platformLotteryActor(w, r, d, "view")
		if !ok {
			return
		}
		if b == "" {
			failure(w, r, 403, "PERMISSION_DENIED", "Brand entry required")
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		if _, e := svc.Detail(r.Context(), b, id, 1, 0); e != nil {
			out, e := platformLotteryResult(200, nil, e)
			outputMutation(w, r, out, e)
			return
		}
		sales, e := platformlottery.ReadConfiguredSales(r.Context(), d.Admins.DB, b, id, false)
		out, e := platformLotteryResult(200, sales, e)
		outputMutation(w, r, out, e)
	})
	if platform {
		register("POST", "/platform-games/{id}/configured/versions", func(w http.ResponseWriter, r *http.Request) {
			a, b, ok := platformLotteryActor(w, r, d, "write")
			if !ok {
				return
			}
			id, ok := ruleID(w, r, "id")
			if !ok {
				return
			}
			var in platformlottery.ConfiguredDraftInput
			if !decodeBodyLimit(w, r, &in, 4<<20) {
				return
			}
			platformLotteryWrite(w, r, d, a, b, "write", "configured_rule.save", in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
				out, e := svc.SaveConfiguredDraft(ctx, tx, fresh, id, in, pointMeta(r, fresh))
				return platformLotteryResult(200, out, e)
			})
		})
		register("POST", "/platform-games/{id}/configured/versions/{versionId}", func(w http.ResponseWriter, r *http.Request) {
			a, b, ok := platformLotteryActor(w, r, d, "write")
			if !ok {
				return
			}
			id, ok := ruleID(w, r, "id")
			if !ok {
				return
			}
			version, ok := ruleID(w, r, "versionId")
			if !ok {
				return
			}
			var in platformlottery.ConfiguredAction
			if !decodeBody(w, r, &in) {
				return
			}
			platformLotteryWrite(w, r, d, a, b, "write", "configured_rule.action", in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
				out, e := svc.ActConfiguredRule(ctx, tx, fresh, id, version, in, pointMeta(r, fresh))
				return platformLotteryResult(200, out, e)
			})
		})
		return
	}
	register("POST", "/platform-games/{id}/configured/sales", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := platformLotteryActor(w, r, d, "sell")
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		var in platformlottery.ConfiguredSales
		if !decodeBodyLimit(w, r, &in, 4<<20) {
			return
		}
		platformLotteryWrite(w, r, d, a, b, "sell", "configured_sales.write", in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, e := svc.WriteConfiguredSales(ctx, tx, fresh, b, id, in, pointMeta(r, fresh))
			return platformLotteryResult(200, out, e)
		})
	})
}
func registerConfiguredBetRoutes(mux routeRegistrar, d Dependencies) {
	for _, prefix := range []string{"/api/v1/configured", "/api/v1/b/{brandCode}/configured"} {
		handle := func(method, path string, fn http.HandlerFunc) {
			mux.HandleFunc(method+" "+prefix+path, func(w http.ResponseWriter, r *http.Request) {
				if bettingUnavailable(w, r, d) {
					return
				}
				fn(w, r)
			})
		}
		handle("GET", "/games", func(w http.ResponseWriter, r *http.Request) {
			b, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			out, e := betService(d).ConfiguredGames(r.Context(), b.ID)
			result, e := configuredResult(map[string]any{"games": out}, e)
			outputMutation(w, r, result, e)
		})
		handle("GET", "/games/{id}", func(w http.ResponseWriter, r *http.Request) {
			b, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			id, ok := ruleID(w, r, "id")
			if !ok {
				return
			}
			out, e := betService(d).ConfiguredCatalog(r.Context(), b.ID, id)
			result, e := configuredResult(out, e)
			outputMutation(w, r, result, e)
		})
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
				failure(w, r, 400, "REQUEST_INVALID", "Invalid paging")
				return
			}
			out, e := betService(d).ConfiguredOrders(r.Context(), b.ID, v.Member.ID, id, offset)
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
				failure(w, r, 400, "REQUEST_INVALID", "Invalid paging")
				return
			}
			out, e := betService(d).ConfiguredOrders(r.Context(), b.ID, v.Member.ID, "", offset)
			result, e := betError(map[string]any{"items": out}, e)
			outputMutation(w, r, result, e)
		})
		handle("POST", "/preview", func(w http.ResponseWriter, r *http.Request) {
			b, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			var in betting.ConfiguredInput
			if !decodeBodyLimit(w, r, &in, 256<<10) {
				return
			}
			if _, ok = currentBetSession(w, r, d, b.ID); !ok {
				return
			}
			tx, e := d.Admins.DB.Begin(r.Context())
			if e != nil {
				outputMutation(w, r, mutation.Result{}, e)
				return
			}
			defer tx.Rollback(r.Context())
			v, e := d.Identity.AuthenticateTx(r.Context(), tx, b.ID, requestToken(r, userCookieName(b.ID)))
			if e != nil {
				result, e := betError(nil, e)
				outputMutation(w, r, result, e)
				return
			}
			out, e := betService(d).PreviewConfigured(r.Context(), tx, b.ID, v, in, points.Metadata{RequestID: requestID(r), IP: meta(r).IP})
			result, e := configuredResult(struct {
				betting.ConfiguredQuote
				ActorContext string `json:"actor_context"`
			}{out, betActorContext(d, b.ID, v)}, e)
			outputMutation(w, r, result, e)
		})
		handle("POST", "/orders", func(w http.ResponseWriter, r *http.Request) {
			b, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			var in betting.ConfiguredInput
			if !decodeBodyLimit(w, r, &in, 256<<10) {
				return
			}
			v, ok := currentBetSession(w, r, d, b.ID)
			if !ok {
				return
			}
			checkedBetWrite(w, r, d, b.ID, "configured.place", in.PeriodID, in.ActorContext, in, v, func(ctx context.Context, tx pgx.Tx, fresh identity.Session) (mutation.Result, error) {
				out, e := betService(d).PlaceConfigured(ctx, tx, b.ID, fresh, in, points.Metadata{ActorType: "user", ActorID: fresh.User.ID, RequestID: requestID(r), IP: meta(r).IP})
				result, e := configuredResult(out, e)
				if result.Status == 200 {
					result.Status = 201
				}
				return result, e
			})
		})
	}
}

func configuredResult(out any, e error) (mutation.Result, error) {
	if e == platformlottery.ErrAttributesMissing || e == platformlottery.ErrNotFound || e == platformlottery.ErrInvalid || e == platformlottery.ErrVersionConflict {
		return platformLotteryResult(200, out, e)
	}
	return betError(out, e)
}
