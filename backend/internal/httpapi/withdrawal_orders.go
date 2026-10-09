package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/withdrawal"
	"github.com/jackc/pgx/v5"
)

var errWithdrawalActor = errors.New("withdrawal confirmation account changed")

func withdrawalOrdersService(d Dependencies) withdrawal.OrderService {
	return withdrawal.OrderService{DB: d.Admins.DB, Points: points.Store{DB: d.Admins.DB}, Eligibility: d.WithdrawalEligibility}
}
func withdrawalOrderResult(out any, err error) (mutation.Result, error) {
	switch {
	case err == nil:
		return mutation.OK(200, out), nil
	case errors.Is(err, identity.ErrSession):
		return mutation.Fail(401, "AUTH_SESSION_REVOKED", "登录状态已变化，请重新登录"), nil
	case errors.Is(err, errWithdrawalActor):
		return mutation.Fail(403, "WITHDRAWAL_CONFIRMATION_ACCOUNT_CHANGED", "申请确认账户已变化，请重新读取当前账号的提现入口"), nil
	case errors.Is(err, withdrawal.ErrInvalid):
		return mutation.Fail(400, "WITHDRAWAL_INPUT_INVALID", "提现金额、来源分配或操作参数不正确"), nil
	case errors.Is(err, withdrawal.ErrDenied):
		return mutation.Fail(403, "PERMISSION_DENIED", "当前账号无此品牌提现操作权限"), nil
	case errors.Is(err, withdrawal.ErrNotFound):
		return mutation.Fail(404, "WITHDRAWAL_NOT_FOUND", "当前品牌未找到该提现记录"), nil
	case errors.Is(err, withdrawal.ErrVersion):
		return mutation.Fail(409, "WITHDRAWAL_VERSION_CONFLICT", "提现记录版本已变化，请重新读取核对"), nil
	case errors.Is(err, withdrawal.ErrOrderState):
		return mutation.Fail(409, "WITHDRAWAL_STATE_CONFLICT", "当前提现状态不允许此操作"), nil
	case errors.Is(err, withdrawal.ErrActiveOrder):
		return mutation.Fail(409, "WITHDRAWAL_ACTIVE_ORDER", "同一品牌已有一笔进行中的提现申请"), nil
	case errors.Is(err, withdrawal.ErrEligibilityNotConfigured):
		return mutation.Fail(409, "WITHDRAWAL_ELIGIBILITY_NOT_CONFIGURED", "提现资格口径尚未配置，暂不接受申请；积分未占用"), nil
	case errors.Is(err, withdrawal.ErrTurnoverBusy):
		return mutation.Fail(503, "WITHDRAWAL_TURNOVER_BUSY", "相关期次正在结算或更正，请稍后重试；积分未占用"), nil
	case errors.Is(err, withdrawal.ErrTurnoverEvidence):
		return mutation.Fail(409, "WITHDRAWAL_TURNOVER_EVIDENCE_INVALID", "历史流水证据不完整，暂不能申请，请联系运营人员；积分未占用"), nil
	case errors.Is(err, withdrawal.ErrIneligible):
		return mutation.Fail(409, "WITHDRAWAL_INELIGIBLE", "当前配置或资格检查不允许提现，积分未占用"), nil
	default:
		return pointResult(out, err)
	}
}
func withdrawalOrdersUnavailable(w http.ResponseWriter, r *http.Request, d Dependencies) bool {
	if d.Identity == nil || d.Mutations == nil || d.Admins.DB == nil {
		failure(w, r, 503, "WITHDRAWAL_UNAVAILABLE", "提现服务尚未配置")
		return true
	}
	return false
}
func withdrawalNoQuery(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		failure(w, r, 400, "REQUEST_INVALID", "此提现接口不接受查询参数")
		return false
	}
	return true
}

type withdrawalQuery struct {
	Limit, Offset int
	Member, State string
}

func withdrawalListQuery(r *http.Request, admin bool) (withdrawalQuery, bool) {
	out := withdrawalQuery{Limit: 20}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || r.URL.ForceQuery {
		return out, false
	}
	for key, values := range q {
		if len(values) != 1 || values[0] == "" {
			return out, false
		}
		switch key {
		case "limit":
			out.Limit, err = strconv.Atoi(values[0])
		case "offset":
			out.Offset, err = strconv.Atoi(values[0])
		case "state":
			out.State = values[0]
		case "member_id":
			if !admin {
				return out, false
			}
			out.Member = strings.ToLower(values[0])
			if !uuidPattern.MatchString(out.Member) {
				return out, false
			}
		default:
			return out, false
		}
		if err != nil {
			return out, false
		}
	}
	switch out.State {
	case "", "reviewing", "processing", "paid", "rejected", "failed", "cancelled":
	default:
		return out, false
	}
	return out, out.Limit >= 1 && out.Limit <= 100 && out.Offset >= 0 && out.Offset <= 1000000
}
func withdrawalActorContext(d Dependencies, brand string, s identity.Session) string {
	return d.Mutations.Fingerprint("withdrawal-actor-v1:" + brand + ":" + s.User.ID + ":" + s.Member.ID)
}
func withdrawalUserRead(r *http.Request, d Dependencies, brand string, query func(pgx.Tx, identity.Session) (any, error)) (any, error) {
	tx, err := d.Admins.DB.Begin(r.Context())
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(r.Context())
	session, err := d.Identity.AuthenticateTx(r.Context(), tx, brand, requestToken(r, userCookieName(brand)))
	if err != nil {
		return nil, err
	}
	out, err := query(tx, session)
	if err != nil {
		return nil, err
	}
	if _, err = d.Identity.AuthenticateTx(r.Context(), tx, brand, requestToken(r, userCookieName(brand))); err != nil {
		return nil, err
	}
	return out, tx.Commit(r.Context())
}
func registerWithdrawalUserRoutes(mux routeRegistrar, d Dependencies) {
	s := withdrawalOrdersService(d)
	for _, prefix := range []string{"/api/v1", "/api/v1/b/{brandCode}"} {
		handle := func(method, path string, fn http.HandlerFunc) {
			mux.HandleFunc(method+" "+prefix+path, func(w http.ResponseWriter, r *http.Request) {
				ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
				defer cancel()
				fn(w, r.WithContext(ctx))
			})
		}
		for _, path := range []string{"/withdrawal-availability", "/withdrawal-qualification", "/withdrawals", "/withdrawals/{id}", "/withdrawals/{id}/history"} {
			handle("GET", path, func(w http.ResponseWriter, r *http.Request) {
				if withdrawalOrdersUnavailable(w, r, d) {
					return
				}
				brand, ok := resolveBrand(w, r, d)
				if !ok {
					return
				}
				var q withdrawalQuery
				if path == "/withdrawals" {
					q, ok = withdrawalListQuery(r, false)
					if !ok {
						failure(w, r, 400, "REQUEST_INVALID", "提现列表参数不正确")
						return
					}
				} else if !withdrawalNoQuery(w, r) {
					return
				}
				if path == "/withdrawal-qualification" && (r.ContentLength != 0 || len(r.TransferEncoding) != 0) {
					failure(w, r, 400, "REQUEST_INVALID", "资格预览不接受请求正文")
					return
				}
				id := strings.ToLower(r.PathValue("id"))
				if strings.Contains(path, "{id}") && !uuidPattern.MatchString(id) {
					failure(w, r, 400, "REQUEST_INVALID", "提现编号不正确")
					return
				}
				out, err := withdrawalUserRead(r, d, brand.ID, func(tx pgx.Tx, session identity.Session) (any, error) {
					switch path {
					case "/withdrawal-availability":
						v, e := s.AvailabilityTx(r.Context(), tx, brand.ID, session.Member.ID)
						v.ActorContext = withdrawalActorContext(d, brand.ID, session)
						return v, e
					case "/withdrawal-qualification":
						return s.QualificationTx(r.Context(), tx, brand.ID, session.Member.ID)
					case "/withdrawals":
						v, e := s.ListViewTx(r.Context(), tx, brand.ID, session.Member.ID, q.State, q.Limit, q.Offset)
						for i := range v.Items {
							v.Items[i] = withdrawal.ToUserOrderView(v.Items[i])
						}
						return v, e
					case "/withdrawals/{id}":
						return s.ReadViewTx(r.Context(), tx, brand.ID, id, session.Member.ID)
					default:
						return s.HistoryViewTx(r.Context(), tx, brand.ID, id, session.Member.ID)
					}
				})
				result, e := withdrawalOrderResult(out, err)
				outputMutation(w, r, result, e)
			})
		}
		handle("POST", "/withdrawals", func(w http.ResponseWriter, r *http.Request) {
			if withdrawalOrdersUnavailable(w, r, d) || !withdrawalNoQuery(w, r) {
				return
			}
			brand, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			var in withdrawal.CreateRequest
			if !decodeBody(w, r, &in) {
				return
			}
			initial, ok := currentBetSession(w, r, d, brand.ID)
			if !ok {
				return
			}
			encoded, _ := json.Marshal(in)
			expected := r.Header.Get("X-Withdrawal-Actor-Context")
			var fresh identity.Session
			result, err := d.Mutations.ExecuteChecked(r.Context(), brand.ID, "user:"+initial.User.ID+":"+initial.Member.ID, "withdrawal.create", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(string(encoded)), func(ctx context.Context, tx pgx.Tx) error {
				v, e := d.Identity.AuthenticateTx(ctx, tx, brand.ID, requestToken(r, userCookieName(brand.ID)))
				if e != nil {
					return e
				}
				if v.User.ID != initial.User.ID || v.Member.ID != initial.Member.ID {
					return identity.ErrSession
				}
				if expected == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(withdrawalActorContext(d, brand.ID, v))) != 1 {
					return errWithdrawalActor
				}
				fresh = v
				return nil
			}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
				o, e := s.Create(ctx, tx, brand.ID, fresh.Member.ID, withdrawal.OrderInput{Points: in.Points, SourceAllocation: in.SourceAllocation, ClientKey: r.Header.Get("Idempotency-Key")}, points.Metadata{ActorType: "user", ActorID: fresh.User.ID, RequestID: requestID(r), IP: meta(r).IP})
				// A transient period lock conflict must abort the engine transaction,
				// not become a cached terminal 503 under this request's original key.
				if errors.Is(e, withdrawal.ErrTurnoverBusy) {
					return mutation.Result{}, e
				}
				if e != nil {
					return withdrawalOrderResult(nil, e)
				}
				// Wallet/qualification waits can outlive an otherwise valid session.
				// Reject inside the business savepoint so no reservation can commit.
				if _, e = d.Identity.AuthenticateTx(ctx, tx, brand.ID, requestToken(r, userCookieName(brand.ID))); e != nil {
					return withdrawalOrderResult(nil, e)
				}
				return mutation.OK(201, withdrawal.ToUserOrderView(withdrawal.ToOrderView(o))), nil
			})
			if errors.Is(err, identity.ErrSession) || errors.Is(err, errWithdrawalActor) || errors.Is(err, withdrawal.ErrTurnoverBusy) {
				result, err = withdrawalOrderResult(nil, err)
			}
			outputMutation(w, r, result, err)
		})
	}
}

func withdrawalReadAllowed(a access.Account, brand string) bool {
	return access.Authorize(a, "withdrawal", "view", access.ScopeBrand, brand) || access.Authorize(a, "withdrawal", "view", access.ScopePlatform, "")
}
func withdrawalAdminRead(r *http.Request, d Dependencies, brand, action string, query func(pgx.Tx) (any, error)) (any, error) {
	ctx := r.Context()
	// Authorization must read current grants after acquiring the shared access
	// gate; a repeatable-read snapshot taken before a revocation wait is unsafe.
	tx, err := d.Admins.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	id, err := d.Identity.AdminAuthenticateTx(ctx, tx, requestToken(r, administrativeCookie(r)))
	if err != nil {
		return nil, err
	}
	actor, err := freshAdmin(ctx, tx, r, d, access.Account{ID: id}, false)
	if errors.Is(err, adminsys.ErrDenied) {
		return nil, identity.ErrSession
	}
	if err != nil {
		return nil, err
	}
	if !withdrawalReadAllowed(actor, brand) {
		_, err = audit.Append(ctx, tx, audit.Record{ActorType: "admin", ActorID: actor.ID, Action: "access.denied", ResourceType: "permission", Reason: "administrative permission denied", RequestID: requestID(r), IP: meta(r).IP, After: map[string]string{"attempted_brand": brand, "permission": "withdrawal.view"}})
		if err == nil {
			err = tx.Commit(ctx)
		}
		if err != nil {
			return nil, err
		}
		return nil, withdrawal.ErrDenied
	}
	out, err := query(tx)
	if err != nil {
		return nil, err
	}
	actor, err = freshAdmin(ctx, tx, r, d, access.Account{ID: id}, false)
	if errors.Is(err, adminsys.ErrDenied) {
		return nil, identity.ErrSession
	}
	if err != nil {
		return nil, err
	}
	if !withdrawalReadAllowed(actor, brand) {
		return nil, withdrawal.ErrDenied
	}
	_, err = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: actor.ID, Action: action, ResourceType: "query", RequestID: requestID(r), IP: meta(r).IP})
	if err != nil {
		return nil, err
	}
	return out, tx.Commit(ctx)
}
func registerWithdrawalOrderAdminRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := withdrawalOrdersService(d)
	for _, path := range []string{"/withdrawals", "/withdrawals/{id}", "/withdrawals/{id}/history"} {
		handle("GET", path, func(w http.ResponseWriter, r *http.Request) {
			if withdrawalOrdersUnavailable(w, r, d) {
				return
			}
			brand, ok := reconciliationBrand(w, r)
			if !ok {
				return
			}
			var q withdrawalQuery
			if path == "/withdrawals" {
				q, ok = withdrawalListQuery(r, true)
				if !ok {
					failure(w, r, 400, "REQUEST_INVALID", "提现列表参数不正确")
					return
				}
			} else if !withdrawalNoQuery(w, r) {
				return
			}
			id := strings.ToLower(r.PathValue("id"))
			if strings.Contains(path, "{id}") && !uuidPattern.MatchString(id) {
				failure(w, r, 400, "REQUEST_INVALID", "提现编号不正确")
				return
			}
			out, err := withdrawalAdminRead(r, d, brand, "withdrawal.read", func(tx pgx.Tx) (any, error) {
				switch path {
				case "/withdrawals":
					return s.ListViewTx(r.Context(), tx, brand, q.Member, q.State, q.Limit, q.Offset)
				case "/withdrawals/{id}":
					return s.ReadViewTx(r.Context(), tx, brand, id, "")
				default:
					return s.HistoryViewTx(r.Context(), tx, brand, id, "")
				}
			})
			result, e := withdrawalOrderResult(out, err)
			outputMutation(w, r, result, e)
		})
	}
	for _, pathAction := range []string{"approve", "reject", "cancel", "fail", "mark-paid"} {
		action := strings.ReplaceAll(pathAction, "-", "_")
		handle("POST", "/withdrawals/{id}/"+pathAction, func(w http.ResponseWriter, r *http.Request) {
			if withdrawalOrdersUnavailable(w, r, d) || !withdrawalNoQuery(w, r) {
				return
			}
			brand, ok := reconciliationBrand(w, r)
			if !ok {
				return
			}
			id := strings.ToLower(r.PathValue("id"))
			if !uuidPattern.MatchString(id) {
				failure(w, r, 400, "REQUEST_INVALID", "提现编号不正确")
				return
			}
			var in withdrawal.ActionRequest
			if !decodeBody(w, r, &in) {
				return
			}
			in.Reason = strings.TrimSpace(in.Reason)
			initial, _, ok := pointAdminActor(w, r, d, "withdrawal", action, true)
			if !ok {
				return
			}
			encoded, _ := json.Marshal(in)
			var fresh access.Account
			result, err := d.Mutations.ExecuteChecked(r.Context(), brand, initial.ID, "admin.withdrawal."+action, r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(id+":"+string(encoded)), func(ctx context.Context, tx pgx.Tx) error {
				a, e := freshAdmin(ctx, tx, r, d, initial, false)
				if errors.Is(e, adminsys.ErrDenied) {
					return identity.ErrSession
				}
				if e != nil {
					return e
				}
				if a.SuperAdmin || !access.Authorize(a, "withdrawal", action, access.ScopeBrand, brand) {
					return withdrawal.ErrDenied
				}
				fresh = a
				return nil
			}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
				o, e := s.Advance(ctx, tx, brand, id, action, fresh, withdrawal.ActionInput{Version: in.Version, ClientKey: r.Header.Get("Idempotency-Key"), Reason: in.Reason}, pointMeta(r, fresh))
				if e != nil {
					return withdrawalOrderResult(nil, e)
				}
				if _, e = freshAdmin(ctx, tx, r, d, fresh, false); e != nil {
					if errors.Is(e, adminsys.ErrDenied) {
						e = identity.ErrSession
					}
					return withdrawalOrderResult(nil, e)
				}
				return mutation.OK(200, withdrawal.ToOrderView(o)), nil
			})
			if errors.Is(err, identity.ErrSession) || errors.Is(err, withdrawal.ErrDenied) {
				result, err = withdrawalOrderResult(nil, err)
			}
			finishAdminMutation(w, r, d, initial, brand, "withdrawal."+action, result, err)
		})
	}
}
