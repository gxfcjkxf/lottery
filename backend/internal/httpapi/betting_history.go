package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/jackc/pgx/v5"
)

func registerBettingHistory(mux routeRegistrar, d Dependencies) {
	for _, prefix := range []string{"/api/v1", "/api/v1/b/{brandCode}"} {
		mux.HandleFunc("GET "+prefix+"/betting-history", func(w http.ResponseWriter, r *http.Request) {
			if d.Identity == nil || d.Admins.DB == nil {
				failure(w, r, 503, "BETTING_UNAVAILABLE", "History unavailable")
				return
			}
			for key, values := range r.URL.Query() {
				if (key != "game_id" && key != "cursor" && key != "limit") || len(values) != 1 {
					failure(w, r, 400, "REQUEST_INVALID", "Invalid history filters")
					return
				}
			}
			limit, _, ok := pageParams(r)
			if !ok {
				failure(w, r, 400, "REQUEST_INVALID", "Invalid paging")
				return
			}
			game, cursor := r.URL.Query().Get("game_id"), r.URL.Query().Get("cursor")
			if game != "" && !uuidPattern.MatchString(game) {
				failure(w, r, 400, "REQUEST_INVALID", "Invalid game")
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()
			r = r.WithContext(ctx)
			brand, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			tx, err := d.Admins.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
			if err != nil {
				pointOutput(w, r, nil, err)
				return
			}
			defer tx.Rollback(ctx)
			session, err := d.Identity.AuthenticateTx(ctx, tx, brand.ID, requestToken(r, userCookieName(brand.ID)))
			if errors.Is(err, identity.ErrSession) {
				failure(w, r, 401, "AUTH_SESSION_REVOKED", "Session unavailable")
				return
			}
			if err != nil {
				pointOutput(w, r, nil, err)
				return
			}
			page, err := betting.HistoryTx(ctx, tx, brand.ID, session.Member.ID, game, cursor, limit)
			if err == nil {
				err = tx.Commit(ctx)
			}
			result, e := betError(page, err)
			outputMutation(w, r, result, e)
		})
	}
}
