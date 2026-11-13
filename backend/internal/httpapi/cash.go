package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/cash"
	"github.com/gxfcjkxf/lottery/backend/internal/finance"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/withdrawal"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"net/http"
	"net/url"
	"strings"
)

var errCashActor = errors.New("currency confirmation account changed")

func cashActorContext(d Dependencies, brand string, v identity.Session) string {
	return d.Mutations.Fingerprint("cash-actor-v1:" + brand + ":" + v.User.ID + ":" + v.Member.ID)
}
func cashService(d Dependencies) cash.Service {
	return cash.Service{Points: points.Store{DB: d.Admins.DB}, Withdrawal: withdrawalOrdersService(d)}
}
func cashResult(out any, e error) (mutation.Result, error) {
	switch {
	case errors.Is(e, errCashActor):
		return mutation.Fail(403, "AUTH_ACTOR_CONTEXT_CHANGED", "The signed-in account changed; reload the wallet"), nil
	case e == nil:
		return mutation.OK(200, out), nil
	case errors.Is(e, cash.ErrInvalid):
		return mutation.Fail(400, "CASH_INPUT_INVALID", "Invalid currency amount or settings"), nil
	case errors.Is(e, cash.ErrDestinationExists):
		return mutation.Fail(409, "CASH_ACCOUNT_EXISTS", "A payment account is already bound for this currency"), nil
	case errors.Is(e, cash.ErrAccount):
		return mutation.Fail(422, "CASH_ACCOUNT_INCOMPLETE", "Complete the payment account details"), nil
	case errors.Is(e, cash.ErrDepositPending):
		return mutation.Fail(409, "CASH_DEPOSIT_PENDING", "A deposit is already awaiting payment or verification"), nil
	case errors.Is(e, cash.ErrConflict):
		return mutation.Fail(409, "CASH_STATE_CHANGED", "Payment data or status changed"), nil
	case errors.Is(e, cash.ErrUnavailable):
		return mutation.Fail(409, "CASH_NOT_CONFIGURED", "Currency payments are not configured"), nil
	case errors.Is(e, cash.ErrInsufficient):
		return mutation.Fail(422, "CASH_INSUFFICIENT", "Insufficient eligible currency balance"), nil
	case errors.Is(e, cash.ErrLimit):
		return mutation.Fail(422, "CASH_LIMIT_EXCEEDED", "Currency amount exceeds the configured limits"), nil
	case errors.Is(e, cash.ErrRate):
		return mutation.Fail(409, "CASH_RATE_CHANGED", "Exchange rate changed; review the current rate"), nil
	case errors.Is(e, finance.ErrRechargeQuota):
		return mutation.Fail(422, "RECHARGE_QUOTA_INSUFFICIENT", "Insufficient brand recharge quota"), nil
	case errors.Is(e, adminsys.ErrDenied):
		return mutation.Fail(403, "PERMISSION_DENIED", "Permission denied"), nil
	case errors.Is(e, withdrawal.ErrTurnoverBusy):
		return mutation.Result{}, e
	}
	var pe *pgconn.PgError
	if errors.As(e, &pe) && pe.Code == "23505" {
		return mutation.Fail(409, "CASH_STATE_CHANGED", "Payment reference or operation already exists"), nil
	}
	return withdrawalOrderResult(out, e)
}
func cashOutput(w http.ResponseWriter, r *http.Request, out any, e error) {
	v, err := cashResult(out, e)
	outputMutation(w, r, v, err)
}
func cashReadParameters(r *http.Request, list bool) bool {
	if r.URL.ForceQuery || !rechargeUserNoBody(r) {
		return false
	}
	if !list {
		return r.URL.RawQuery == ""
	}
	query, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil {
		return false
	}
	for key, values := range query {
		if len(values) != 1 || values[0] == "" {
			return false
		}
		switch key {
		case "limit", "offset":
			if !canonicalRechargePageNumber.MatchString(values[0]) {
				return false
			}
		case "kind":
			switch values[0] {
			case "deposit", "withdrawal", "to_points", "from_points":
			default:
				return false
			}
		default:
			return false
		}
	}
	_, _, ok := pageParams(r)
	return ok
}
func registerCashUser(mux routeRegistrar, d Dependencies) {
	s := cashService(d)
	for _, prefix := range []string{"/api/v1", "/api/v1/b/{brandCode}"} {
		for _, path := range []string{"/cash", "/cash/orders"} {
			mux.HandleFunc("GET "+prefix+path, func(w http.ResponseWriter, r *http.Request) {
				if !cashReadParameters(r, path == "/cash/orders") {
					cashOutput(w, r, nil, cash.ErrInvalid)
					return
				}
				if withdrawalOrdersUnavailable(w, r, d) {
					return
				}
				b, ok := resolveBrand(w, r, d)
				if !ok {
					return
				}
				out, e := withdrawalUserRead(r, d, b.ID, func(tx pgx.Tx, v identity.Session) (any, error) {
					if path == "/cash/orders" {
						limit, offset, ok := pageParams(r)
						if !ok {
							return nil, cash.ErrInvalid
						}
						return cash.List(r.Context(), tx, b.ID, v.Member.ID, r.URL.Query().Get("kind"), limit, offset)
					}
					if r.URL.RawQuery != "" {
						return nil, cash.ErrInvalid
					}
					balances, e := cash.Balances(r.Context(), tx, b.ID, v.Member.ID)
					if e != nil {
						return nil, e
					}
					dest, e := cash.Destinations(r.Context(), tx, b.ID, v.Member.ID)
					if e != nil {
						return nil, e
					}
					platform, e := cash.ReadConfig(r.Context(), tx, "platform")
					if e != nil {
						return nil, e
					}
					brand, e := cash.ReadBrandConfig(r.Context(), tx, b.ID)
					if e != nil {
						return nil, e
					}
					return map[string]any{"actor_context": cashActorContext(d, b.ID, v), "auto_usdt": true, "brand_id": b.ID, "member_id": v.Member.ID, "balances": balances, "destinations": dest, "platform": platform, "brand": brand}, nil
				})
				cashOutput(w, r, out, e)
			})
		}
		for _, path := range []string{"/cash/deposits", "/cash/exchanges", "/cash/withdrawals", "/cash/destinations", "/cash/destinations/delete", "/cash/orders/{id}/{action}"} {
			mux.HandleFunc("POST "+prefix+path, func(w http.ResponseWriter, r *http.Request) {
				if withdrawalOrdersUnavailable(w, r, d) || !withdrawalNoQuery(w, r) {
					return
				}
				b, ok := resolveBrand(w, r, d)
				if !ok {
					return
				}
				initial, ok := currentBetSession(w, r, d, b.ID)
				if !ok {
					return
				}
				var in cash.Input
				var dest cash.Destination
				var deletion struct {
					ID      string `json:"id"`
					Version int64  `json:"version"`
				}
				var body any
				if path == "/cash/destinations" {
					if !decodeBody(w, r, &dest) {
						return
					}
					body = dest
				} else if path == "/cash/destinations/delete" {
					if !decodeBody(w, r, &deletion) {
						return
					}
					if !uuidPattern.MatchString(deletion.ID) {
						cashOutput(w, r, nil, cash.ErrInvalid)
						return
					}
					body = deletion
				} else {
					if !decodeBody(w, r, &in) {
						return
					}
					body = in
				}
				id, action := r.PathValue("id"), r.PathValue("action")
				if id != "" && !uuidPattern.MatchString(id) {
					cashOutput(w, r, nil, cash.ErrInvalid)
					return
				}
				if dest.ID != "" && !uuidPattern.MatchString(dest.ID) {
					cashOutput(w, r, nil, cash.ErrInvalid)
					return
				}
				raw, _ := json.Marshal(body)
				var fresh identity.Session
				result, e := d.Mutations.ExecuteChecked(r.Context(), b.ID, "user:"+initial.User.ID+":"+initial.Member.ID, "cash."+path+":"+id+":"+action, r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(string(raw)), func(ctx context.Context, tx pgx.Tx) error {
					v, e := d.Identity.AuthenticateTx(ctx, tx, b.ID, requestToken(r, userCookieName(b.ID)))
					if e != nil {
						return e
					}
					if v.User.ID != initial.User.ID || v.Member.ID != initial.Member.ID {
						return identity.ErrSession
					}
					expected := r.Header.Get("X-Cash-Actor-Context")
					if expected == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(cashActorContext(d, b.ID, v))) != 1 {
						return errCashActor
					}
					fresh = v
					return nil
				}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
					meta := points.Metadata{ActorType: "user", ActorID: fresh.User.ID, RequestID: requestID(r), IP: meta(r).IP}
					var out any
					var err error
					switch path {
					case "/cash/deposits":
						out, err = s.Deposit(ctx, tx, b.ID, fresh.Member.ID, in, meta)
					case "/cash/exchanges":
						out, err = s.Exchange(ctx, tx, b.ID, fresh.Member.ID, in, meta)
					case "/cash/withdrawals":
						out, err = s.Withdraw(ctx, tx, b.ID, fresh.Member.ID, in, meta)
					case "/cash/destinations/delete":
						out, err = cash.DeleteDestination(ctx, tx, b.ID, fresh.Member.ID, deletion.ID, deletion.Version, meta)
					case "/cash/destinations":
						out, err = cash.SaveDestination(ctx, tx, b.ID, fresh.Member.ID, dest, meta)
					default:
						out, err = s.Advance(ctx, tx, b.ID, fresh.Member.ID, id, action, in, meta)
					}
					if err == nil {
						_, err = d.Identity.AuthenticateTx(ctx, tx, b.ID, requestToken(r, userCookieName(b.ID)))
					}
					return cashResult(out, err)
				})
				if errors.Is(e, identity.ErrSession) || errors.Is(e, errCashActor) {
					result, e = cashResult(nil, e)
				}
				outputMutation(w, r, result, e)
			})
		}
	}
}
func registerCashAdmin(handle func(string, string, http.HandlerFunc), d Dependencies, platform bool) {
	s := cashService(d)
	handle("GET", "/cash/balances/{memberID}", func(w http.ResponseWriter, r *http.Request) {
		if !cashReadParameters(r, false) {
			cashOutput(w, r, nil, cash.ErrInvalid)
			return
		}
		a, brand, ok := pointAdminActor(w, r, d, "wallet", "view", false)
		if !ok {
			return
		}
		member := r.PathValue("memberID")
		if !uuidPattern.MatchString(member) {
			cashOutput(w, r, nil, cash.ErrInvalid)
			return
		}
		tx, e := d.Admins.DB.Begin(r.Context())
		if e != nil {
			cashOutput(w, r, nil, e)
			return
		}
		defer tx.Rollback(r.Context())
		out, e := cash.Balances(r.Context(), tx, brand, member)
		if e == nil {
			e = tx.Commit(r.Context())
		}
		if e == nil && !adminReadAudit(w, r, d, a, brand, "cash.balances.view") {
			return
		}
		cashOutput(w, r, out, e)
	})
	// Platform controls rates and permitted limits; brands control collection
	// accounts, allocation and their own operating limits. Platform cannot pay.
	for _, path := range []string{"/cash/config", "/cash/orders", "/cash/destinations"} {
		handle("GET", path, func(w http.ResponseWriter, r *http.Request) {
			if !cashReadParameters(r, path == "/cash/orders") {
				cashOutput(w, r, nil, cash.ErrInvalid)
				return
			}
			var a access.Account
			var brand string
			var ok bool
			resource := "point_policy"
			if path == "/cash/orders" {
				resource = "recharge"
				if r.URL.Query().Get("kind") == "withdrawal" {
					resource = "withdrawal"
				}
			}
			if platform && path == "/cash/config" {
				a, ok = platformAccountActor(w, r, d, "platform_settings", "view")
			} else {
				a, brand, ok = pointAdminActor(w, r, d, resource, "view", false)
			}
			if !ok {
				return
			}
			tx, e := d.Admins.DB.Begin(r.Context())
			if e != nil {
				cashOutput(w, r, nil, e)
				return
			}
			defer tx.Rollback(r.Context())
			var out any
			switch path {
			case "/cash/config":
				scope := brand
				if platform {
					scope = "platform"
				}
				var c cash.Config
				if platform {
					c, e = cash.ReadConfig(r.Context(), tx, scope)
				} else {
					c, e = cash.ReadBrandConfig(r.Context(), tx, scope)
				}
				out = c
			case "/cash/destinations":
				out, e = cash.Destinations(r.Context(), tx, brand, "")
			default:
				kind := r.URL.Query().Get("kind")
				if kind != "deposit" && kind != "withdrawal" && kind != "to_points" && kind != "from_points" {
					e = cash.ErrInvalid
					break
				}
				limit, offset, ok := pageParams(r)
				if !ok {
					e = cash.ErrInvalid
					break
				}
				out, e = cash.List(r.Context(), tx, brand, "", kind, limit, offset)
			}
			if e == nil {
				e = tx.Commit(r.Context())
			}
			if e == nil && !adminReadAudit(w, r, d, a, brand, "cash.view") {
				return
			}
			cashOutput(w, r, out, e)
		})
	}
	for _, route := range []struct{ method, path string }{{"PUT", "/cash/config"}, {"POST", "/cash/destinations"}, {"POST", "/cash/orders/{id}/{action}"}} {
		if platform && route.path != "/cash/config" {
			continue
		}
		handle(route.method, route.path, func(w http.ResponseWriter, r *http.Request) {
			if !withdrawalNoQuery(w, r) {
				return
			}
			var a access.Account
			var brand string
			var ok bool
			resource, permission := "point_policy", "write"
			action, id := r.PathValue("action"), r.PathValue("id")
			if route.path == "/cash/orders/{id}/{action}" {
				if !uuidPattern.MatchString(id) {
					cashOutput(w, r, nil, cash.ErrInvalid)
					return
				}
				var kind string
				e := d.Admins.DB.QueryRow(r.Context(), `SELECT kind FROM cash_orders WHERE id=$1`, id).Scan(&kind)
				if e != nil {
					cashOutput(w, r, nil, cash.ErrConflict)
					return
				}
				if kind == "deposit" {
					resource = "recharge"
					permission = "confirm"
					if action == "reject" {
						permission = "cancel"
					}
				} else if kind == "withdrawal" {
					resource = "withdrawal"
					permission = action
					if action == "paid" {
						permission = "mark_paid"
					}
				} else {
					cashOutput(w, r, nil, cash.ErrInvalid)
					return
				}
			}
			if platform {
				a, ok = platformAccountActor(w, r, d, "platform_settings", "write")
			} else {
				a, brand, ok = pointAdminActor(w, r, d, resource, permission, true)
			}
			if !ok {
				return
			}
			var cfg cash.Config
			var dest cash.Destination
			var in cash.Input
			var body any
			switch route.path {
			case "/cash/config":
				if !decodeBody(w, r, &cfg) {
					return
				}
				body = cfg
			case "/cash/destinations":
				if !decodeBody(w, r, &dest) {
					return
				}
				if dest.ID != "" && !uuidPattern.MatchString(dest.ID) {
					cashOutput(w, r, nil, cash.ErrInvalid)
					return
				}
				body = dest
			default:
				if !decodeBody(w, r, &in) {
					return
				}
				body = in
			}
			raw, _ := json.Marshal(body)
			var fresh access.Account
			result, e := d.Mutations.ExecuteChecked(r.Context(), brand, a.ID, "cash.admin."+route.path+":"+id+":"+action, r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(string(raw)), func(ctx context.Context, tx pgx.Tx) error {
				v, e := freshAdmin(ctx, tx, r, d, a, platform)
				if e != nil {
					return e
				}
				scope := access.ScopeBrand
				target := brand
				res, act := resource, permission
				if platform {
					scope = access.ScopePlatform
					target = ""
					res = "platform_settings"
					act = "write"
				}
				if v.SuperAdmin != platform || !access.Authorize(v, res, act, scope, target) {
					return adminsys.ErrDenied
				}
				fresh = v
				return nil
			}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
				var out any
				var e error
				switch route.path {
				case "/cash/config":
					scope := brand
					if platform {
						scope = "platform"
					}
					out, e = cash.SaveConfig(ctx, tx, scope, cfg, pointMeta(r, fresh))
				case "/cash/destinations":
					out, e = cash.SaveDestination(ctx, tx, brand, "", dest, pointMeta(r, fresh))
				default:
					out, e = s.Advance(ctx, tx, brand, "", id, action, in, pointMeta(r, fresh))
				}
				return cashResult(out, e)
			})
			if errors.Is(e, adminsys.ErrDenied) || errors.Is(e, identity.ErrSession) {
				result, e = cashResult(nil, e)
			}
			finishAdminMutation(w, r, d, a, brand, strings.Join([]string{resource, permission}, "."), result, e)
		})
	}
}
