package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/identity"
)

type quickAmountsInput struct {
	Amounts []string `json:"quick_amounts"`
}
type quickAmountsOutput struct {
	BrandID  string   `json:"brand_id"`
	MemberID string   `json:"member_id"`
	Amounts  []string `json:"quick_amounts"`
}

func parseQuickAmounts(values []string) ([]int64, bool) {
	if values == nil || len(values) > 5 {
		return nil, false
	}
	amounts := make([]int64, len(values))
	for i, value := range values {
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil || n <= 0 || n > 999999 || strconv.FormatInt(n, 10) != value {
			return nil, false
		}
		amounts[i] = n
	}
	return amounts, true
}
func registerBettingPreferences(mux routeRegistrar, d Dependencies) {
	for _, prefix := range []string{"/api/v1", "/api/v1/b/{brandCode}"} {
		for _, method := range []string{"GET", "PUT"} {
			mux.HandleFunc(method+" "+prefix+"/me/betting-preferences", func(w http.ResponseWriter, r *http.Request) {
				if d.Identity == nil || d.Admins.DB == nil {
					failure(w, r, 503, "POINTS_UNAVAILABLE", "Settings unavailable")
					return
				}
				if len(r.URL.Query()) != 0 {
					failure(w, r, 400, "REQUEST_INVALID", "Settings accept no filters")
					return
				}
				var input quickAmountsInput
				var amounts []int64
				if r.Method == "PUT" {
					if !decodeBody(w, r, &input) {
						return
					}
					var valid bool
					amounts, valid = parseQuickAmounts(input.Amounts)
					if !valid {
						failure(w, r, 400, "REQUEST_INVALID", "Invalid quick stake amounts")
						return
					}
				}
				ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
				defer cancel()
				r = r.WithContext(ctx)
				brand, ok := resolveBrand(w, r, d)
				if !ok {
					return
				}
				tx, err := d.Admins.DB.Begin(ctx)
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
				if r.Method == "PUT" {
					err = tx.QueryRow(ctx, `UPDATE brand_members SET betting_quick_amounts=$3 WHERE brand_id=$1 AND id=$2 RETURNING betting_quick_amounts`, brand.ID, session.Member.ID, amounts).Scan(&amounts)
				} else {
					err = tx.QueryRow(ctx, `SELECT betting_quick_amounts FROM brand_members WHERE brand_id=$1 AND id=$2`, brand.ID, session.Member.ID).Scan(&amounts)
				}
				if err != nil {
					pointOutput(w, r, nil, err)
					return
				}
				out := quickAmountsOutput{BrandID: brand.ID, MemberID: session.Member.ID, Amounts: make([]string, len(amounts))}
				for i, n := range amounts {
					out.Amounts[i] = strconv.FormatInt(n, 10)
				}
				if err = tx.Commit(ctx); err != nil {
					pointOutput(w, r, nil, err)
					return
				}
				respond(w, r, 200, out)
			})
		}
	}
}
