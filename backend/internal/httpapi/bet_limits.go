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
	"github.com/jackc/pgx/v5"
	"net/http"
)

func registerBetLimitRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	svc := platformlottery.Service{DB: d.Admins.DB}
	handle("GET", "/bet-limits", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := managementActor(w, r, d, "bet_policy", "view")
		if !ok {
			return
		}
		items, err := svc.BetLimitGames(r.Context(), brand)
		if err == nil && !adminReadAudit(w, r, d, a, brand, "bet_policy.view") {
			return
		}
		result, e := platformLotteryResult(200, struct {
			Items []platformlottery.BetLimitGame `json:"items"`
		}{items}, err)
		outputMutation(w, r, result, e)
	})
	handle("GET", "/platform-games/{id}/bet-limits", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := managementActor(w, r, d, "bet_policy", "view")
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		out, err := svc.BetLimits(r.Context(), brand, id)
		if err == nil && !adminReadAudit(w, r, d, a, brand, "bet_policy.view") {
			return
		}
		result, e := platformLotteryResult(200, out, err)
		outputMutation(w, r, result, e)
	})
	handle("POST", "/platform-games/{id}/bet-limits", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := managementActor(w, r, d, "bet_policy", "write")
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		var in platformlottery.BetLimits
		if !decodeBody(w, r, &in) {
			return
		}
		raw, err := json.Marshal(in)
		if err != nil {
			failure(w, r, 400, "REQUEST_INVALID", "Invalid request")
			return
		}
		var fresh access.Account
		result, err := d.Mutations.ExecuteChecked(r.Context(), brand, a.ID, "admin.bet_limits.write", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(id+":"+string(raw)), func(ctx context.Context, tx pgx.Tx) error {
			var e error
			fresh, e = freshAdmin(ctx, tx, r, d, a, true)
			if e != nil {
				return e
			}
			if !managementAllowed(fresh, "bet_policy", "write", brand) {
				return adminsys.ErrDenied
			}
			return nil
		}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			out, e := svc.WriteBetLimits(ctx, tx, fresh, brand, id, in, pointMeta(r, fresh))
			return platformLotteryResult(200, out, e)
		})
		if errors.Is(err, identity.ErrSession) {
			result = mutation.Fail(401, "AUTH_SESSION_REVOKED", "Session expired")
			err = nil
		}
		if errors.Is(err, adminsys.ErrDenied) {
			result = mutation.Fail(403, "PERMISSION_DENIED", "Permission denied")
			err = nil
		}
		finishAdminMutation(w, r, d, a, brand, "bet_policy.write", result, err)
	})
}
