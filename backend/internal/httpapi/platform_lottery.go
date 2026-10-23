package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
)

func platformLotteryWriteRoute(method, path string) bool {
	return method == "POST" && strings.HasPrefix(path, "/platform-games")
}
func platformLotteryResult(status int, out any, err error) (mutation.Result, error) {
	switch {
	case errors.Is(err, platformlottery.ErrGameInUse):
		return mutation.Fail(409, "PLATFORM_GAME_IN_USE", "The game has unpublished periods or unsettled orders"), nil
	case errors.Is(err, platformlottery.ErrDenied):
		return mutation.Fail(403, "PERMISSION_DENIED", "Platform lottery permission denied"), nil
	case errors.Is(err, platformlottery.ErrBrowserUnavailable):
		return mutation.Fail(503, "AUTOMATION_BROWSER_UNAVAILABLE", "Draw browser runtime is not configured"), nil
	case errors.Is(err, platformlottery.ErrBrowserRender):
		return mutation.Fail(400, "AUTOMATION_BROWSER_RENDER_FAILED", "Draw browser rendering failed"), nil
	case errors.Is(err, platformlottery.ErrInvalid):
		return mutation.Fail(400, "PLATFORM_GAME_INVALID", "Invalid platform lottery input"), nil
	case errors.Is(err, platformlottery.ErrNotFound):
		return mutation.Fail(404, "RESOURCE_NOT_FOUND", "Platform lottery not found or not connected"), nil
	case errors.Is(err, platformlottery.ErrConflict):
		return mutation.Fail(409, "PLATFORM_GAME_CONFLICT", "Already connected, duplicate record, or draw is not ready"), nil
	case errors.Is(err, platformlottery.ErrModelLocked):
		return mutation.Fail(409, "PLATFORM_GAME_MODEL_LOCKED", "A game with plays or periods cannot change its number model"), nil
	case errors.Is(err, platformlottery.ErrAttributesMissing):
		return mutation.Fail(409, "MARK_SIX_ATTRIBUTES_MISSING", "Publish annual number attributes before creating periods or accepting bets"), nil
	case errors.Is(err, platformlottery.ErrVersionConflict):
		return mutation.Fail(409, "PLATFORM_GAME_VERSION_CONFLICT", "Game version changed; reload before editing"), nil
	case err != nil:
		return mutation.Result{}, err
	default:
		return mutation.OK(status, out), nil
	}
}
func platformLotteryActor(w http.ResponseWriter, r *http.Request, d Dependencies, action string) (access.Account, string, bool) {
	a, ok := adminAccount(w, r, d)
	if !ok {
		return a, "", false
	}
	brand := ""
	if !platformAdminEntry(r) {
		brand = r.Header.Get("X-Brand-ID")
		if !uuidPattern.MatchString(brand) {
			failure(w, r, 400, "REQUEST_INVALID", "A valid brand is required")
			return a, brand, false
		}
	}
	if !platformlottery.Allowed(a, brand, action) {
		rejectAdmin(w, r, d, a, brand, "platform_game."+action)
		return a, brand, false
	}
	return a, brand, true
}
func platformLotteryWrite(w http.ResponseWriter, r *http.Request, d Dependencies, a access.Account, brand, action, operation string, in any, run managementAction) {
	raw, e := json.Marshal(in)
	if e != nil {
		failure(w, r, 400, "REQUEST_INVALID", "Invalid request")
		return
	}
	var fresh access.Account
	result, e := d.Mutations.ExecuteChecked(r.Context(), brand, a.ID, operation, r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(r.URL.Path+":"+string(raw)), func(ctx context.Context, tx pgx.Tx) error {
		current, e := freshAdmin(ctx, tx, r, d, a, true)
		if errors.Is(e, adminsys.ErrDenied) || errors.Is(e, identity.ErrSession) {
			return identity.ErrSession
		}
		if e != nil {
			return e
		}
		fresh = current
		if !platformlottery.Allowed(fresh, brand, action) {
			return platformlottery.ErrDenied
		}
		return nil
	}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) { return run(ctx, tx, fresh) })
	if errors.Is(e, identity.ErrSession) {
		result, e = mutation.Fail(401, "AUTH_SESSION_REVOKED", "Administrative session revoked"), nil
	} else if errors.Is(e, platformlottery.ErrDenied) {
		result, e = mutation.Fail(403, "PERMISSION_DENIED", "Platform lottery permission denied"), nil
	}
	finishAdminMutation(w, r, d, a, brand, "platform_game."+action, result, e)
}
func registerPlatformLotteryRoutes(register func(string, string, http.HandlerFunc), d Dependencies, platform bool) {
	registerConfiguredAdminRoutes(register, d, platform)
	registerConfiguredPreviewRoutes(register, d)
	registerMarkSixAdminRoutes(register, d, platform)
	registerCanadaAdminRoutes(register, d, platform)
	registerPlatformAutomationRoutes(register, d, platform)
	registerPlatformLifecycleRoutes(register, d, platform)
	handle := func(method, path string, handler http.HandlerFunc) {
		if method == "POST" && (strings.HasSuffix(path, "/connect") == platform) {
			return
		}
		register(method, path, handler)
	}
	s := platformlottery.Service{DB: d.Admins.DB}
	handle("GET", "/platform-games", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := platformLotteryActor(w, r, d, "view")
		if !ok {
			return
		}
		limit, offset, ok := pageParams(r)
		if !ok {
			failure(w, r, 400, "REQUEST_INVALID", "Invalid pagination")
			return
		}
		out, e := s.Games(r.Context(), b, limit, offset)
		if e == nil && !adminReadAudit(w, r, d, a, b, "platform_game.view") {
			return
		}
		result, e := platformLotteryResult(200, map[string]any{"games": out, "limit": limit, "offset": offset}, e)
		outputMutation(w, r, result, e)
	})
	handle("GET", "/platform-games/{id}", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := platformLotteryActor(w, r, d, "view")
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
		out, e := s.Detail(r.Context(), b, id, limit, offset)
		if e == nil && !adminReadAudit(w, r, d, a, b, "platform_game.view") {
			return
		}
		result, e := platformLotteryResult(200, out, e)
		outputMutation(w, r, result, e)
	})
	handle("POST", "/platform-games", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := platformLotteryActor(w, r, d, "write")
		if !ok {
			return
		}
		var in platformlottery.GameInput
		if !decodeBody(w, r, &in) {
			return
		}
		platformLotteryWrite(w, r, d, a, b, "write", "platform.game.create", in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, e := s.Create(ctx, tx, fresh, in, pointMeta(r, fresh))
			return platformLotteryResult(201, out, e)
		})
	})
	if platform {
		handle("POST", "/platform-games/{id}/name", func(w http.ResponseWriter, r *http.Request) {
			a, b, ok := platformLotteryActor(w, r, d, "write")
			if !ok {
				return
			}
			id, ok := ruleID(w, r, "id")
			if !ok {
				return
			}
			var in platformlottery.NameInput
			if !decodeBody(w, r, &in) {
				return
			}
			platformLotteryWrite(w, r, d, a, b, "write", "platform.game.name.update", in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
				out, e := s.UpdateName(ctx, tx, fresh, id, in, pointMeta(r, fresh))
				return platformLotteryResult(200, out, e)
			})
		})
	}
	handle("POST", "/platform-games/{id}/model", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := platformLotteryActor(w, r, d, "write")
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		var in platformlottery.ModelInput
		if !decodeBody(w, r, &in) {
			return
		}
		platformLotteryWrite(w, r, d, a, b, "write", "platform.game.model.update", in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, e := s.UpdateModel(ctx, tx, fresh, id, in, pointMeta(r, fresh))
			return platformLotteryResult(200, out, e)
		})
	})
	handle("POST", "/platform-games/{id}/connect", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := platformLotteryActor(w, r, d, "connect")
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		var in struct {
			Reason string `json:"reason"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		platformLotteryWrite(w, r, d, a, b, "connect", "brand.platform-game.connect", in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, e := s.Connect(ctx, tx, fresh, b, id, in.Reason, pointMeta(r, fresh))
			return platformLotteryResult(201, out, e)
		})
	})
	handle("POST", "/platform-games/{id}/plays", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := platformLotteryActor(w, r, d, "write")
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		var in platformlottery.PlayInput
		if !decodeBody(w, r, &in) {
			return
		}
		platformLotteryWrite(w, r, d, a, b, "write", "platform.game.play.create", in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, e := s.CreatePlay(ctx, tx, fresh, id, in, pointMeta(r, fresh))
			return platformLotteryResult(201, out, e)
		})
	})
	handle("POST", "/platform-games/{id}/periods", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := platformLotteryActor(w, r, d, "write")
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		var in platformlottery.PeriodInput
		if !decodeBody(w, r, &in) {
			return
		}
		platformLotteryWrite(w, r, d, a, b, "write", "platform.game.period.create", in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, e := s.CreatePeriod(ctx, tx, fresh, id, in, pointMeta(r, fresh))
			return platformLotteryResult(201, out, e)
		})
	})
	handle("POST", "/platform-games/periods/{id}/result", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := platformLotteryActor(w, r, d, "write")
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		var in struct {
			Result rules.Draw `json:"result"`
			Reason string     `json:"reason"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		platformLotteryWrite(w, r, d, a, b, "write", "platform.game.result.publish", in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, e := s.Publish(ctx, tx, fresh, id, in.Result, in.Reason, pointMeta(r, fresh))
			return platformLotteryResult(201, out, e)
		})
	})
}
