package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

func betService(d Dependencies) betting.Service { return betting.Service{DB: d.Admins.DB} }

func betError(data any, err error) (mutation.Result, error) {
	switch {
	case err == nil:
		return mutation.OK(200, data), nil
	case errors.Is(err, identity.ErrSession):
		return mutation.Fail(401, "AUTH_SESSION_REVOKED", "登录状态已过期，请重新登录"), nil
	case errors.Is(err, betting.ErrInvalid):
		return mutation.Fail(400, "BET_INPUT_INVALID", "投注请求或配置不正确"), nil
	case errors.Is(err, betting.ErrDenied):
		return mutation.Fail(403, "BET_OPERATION_DENIED", "无权执行此投注操作"), nil
	case errors.Is(err, betting.ErrNotFound):
		return mutation.Fail(404, "BET_RESOURCE_NOT_FOUND", "未找到对应投注记录"), nil
	case errors.Is(err, betting.ErrVersion):
		return mutation.Fail(409, "BET_VERSION_CONFLICT", "投注配置或记录已变化，请刷新后重试"), nil
	case errors.Is(err, betting.ErrClosed):
		return mutation.Fail(409, "BET_PERIOD_CLOSED", "当前投注期不接受此操作"), nil
	case errors.Is(err, betting.ErrLimit):
		return mutation.Fail(409, "BET_LIMIT_EXCEEDED", "投注金额超过当前限额"), nil
	case errors.Is(err, betting.ErrState):
		return mutation.Fail(409, "BET_STATE_CONFLICT", "投注记录已处理或幂等键用于其他请求"), nil
	default:
		if errors.Is(err, rules.ErrInvalid) {
			return mutation.Fail(400, "BET_INPUT_INVALID", "投注选号或倍数不正确"), nil
		}
		if errors.Is(err, rules.ErrLimit) {
			return mutation.Fail(409, "BET_LIMIT_EXCEEDED", "投注组合超过规则计算限额"), nil
		}
		result, mappedErr := pointResult(data, err)
		if mappedErr == nil {
			return result, nil
		}
		return mutation.Result{}, err
	}
}

func bettingUnavailable(w http.ResponseWriter, r *http.Request, d Dependencies) bool {
	if d.Identity == nil || d.Mutations == nil || d.Admins.DB == nil {
		failure(w, r, 503, "BETTING_UNAVAILABLE", "投注服务尚未配置")
		return true
	}
	return false
}

func currentBetSession(w http.ResponseWriter, r *http.Request, d Dependencies, brand string) (identity.Session, bool) {
	session, err := d.Identity.Authenticate(r.Context(), brand, requestToken(r, userCookieName(brand)))
	if errors.Is(err, identity.ErrSession) {
		failure(w, r, 401, "AUTH_SESSION_REVOKED", "请重新登录")
		return identity.Session{}, false
	}
	if err != nil {
		failure(w, r, 503, "SERVICE_UNAVAILABLE", "暂时无法验证登录状态")
		return identity.Session{}, false
	}
	return session, true
}

func betActorContext(d Dependencies, brand string, v identity.Session) string {
	return d.Mutations.Fingerprint("lottery-bet-actor-v1:" + brand + ":" + v.User.ID + ":" + v.Member.ID)
}
func checkedBetWrite(w http.ResponseWriter, r *http.Request, d Dependencies, brand, operation, resourceID, expectedContext string, body any, initial identity.Session, run func(context.Context, pgx.Tx, identity.Session) (mutation.Result, error)) {
	encoded, err := json.Marshal(body)
	if err != nil {
		failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
		return
	}
	actor := "user:" + initial.User.ID + ":" + initial.Member.ID
	fingerprint := d.Mutations.Fingerprint(resourceID + ":" + string(encoded))
	var freshSession identity.Session
	result, err := d.Mutations.ExecuteChecked(r.Context(), brand, actor, operation, r.Header.Get("Idempotency-Key"), fingerprint,
		func(ctx context.Context, tx pgx.Tx) error {
			fresh, err := d.Identity.AuthenticateTx(ctx, tx, brand, requestToken(r, userCookieName(brand)))
			if errors.Is(err, identity.ErrSession) || err == nil && (fresh.User.ID != initial.User.ID || fresh.Member.ID != initial.Member.ID) {
				return identity.ErrSession
			}
			if err == nil {
				if operation == "bet.order.place" && subtle.ConstantTimeCompare([]byte(expectedContext), []byte(betActorContext(d, brand, fresh))) != 1 {
					return betting.ErrDenied
				}
				freshSession = fresh
			}
			return err
		},
		func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			return run(ctx, tx, freshSession)
		})
	if errors.Is(err, identity.ErrSession) {
		failure(w, r, 401, "AUTH_SESSION_REVOKED", "登录状态已变化，请重新登录")
		return
	}
	if errors.Is(err, betting.ErrDenied) {
		failure(w, r, 403, "BET_CONFIRMATION_ACCOUNT_CHANGED", "确认账户已变化，请重新获取投注预览")
		return
	}
	outputMutation(w, r, result, err)
}

func registerBetRoutes(mux *http.ServeMux, d Dependencies) {
	for _, prefix := range []string{"/api/v1", "/api/v1/b/{brandCode}"} {
		handle := func(method, path string, fn http.HandlerFunc) {
			mux.HandleFunc(method+" "+prefix+path, func(w http.ResponseWriter, r *http.Request) {
				ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
				defer cancel()
				fn(w, r.WithContext(ctx))
			})
		}
		handle("POST", "/bet-previews", func(w http.ResponseWriter, r *http.Request) {
			if bettingUnavailable(w, r, d) {
				return
			}
			brand, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			var in betting.Input
			if !decodeBody(w, r, &in) {
				return
			}
			_, ok = currentBetSession(w, r, d, brand.ID)
			if !ok {
				return
			}
			tx, err := d.Admins.DB.Begin(r.Context())
			if err != nil {
				failure(w, r, 503, "SERVICE_UNAVAILABLE", "投注服务暂时不可用")
				return
			}
			defer tx.Rollback(r.Context())
			session, err := d.Identity.AuthenticateTx(r.Context(), tx, brand.ID, requestToken(r, userCookieName(brand.ID)))
			if errors.Is(err, identity.ErrSession) {
				failure(w, r, 401, "AUTH_SESSION_REVOKED", "请重新登录")
				return
			}
			if err != nil {
				failure(w, r, 503, "SERVICE_UNAVAILABLE", "暂时无法验证登录状态")
				return
			}
			quote, err := betService(d).Preview(r.Context(), tx, brand.ID, session, in)
			if err != nil {
				result, e := betError(nil, err)
				outputMutation(w, r, result, e)
				return
			}
			respond(w, r, 200, struct {
				betting.Quote
				ActorContext string `json:"actor_context"`
			}{quote, betActorContext(d, brand.ID, session)})
		})
		handle("POST", "/bet-orders", func(w http.ResponseWriter, r *http.Request) {
			if bettingUnavailable(w, r, d) {
				return
			}
			brand, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			var in betting.Input
			if !decodeBody(w, r, &in) {
				return
			}
			session, ok := currentBetSession(w, r, d, brand.ID)
			if !ok {
				return
			}
			checkedBetWrite(w, r, d, brand.ID, "bet.order.place", "", in.ActorContext, in, session, func(ctx context.Context, tx pgx.Tx, fresh identity.Session) (mutation.Result, error) {
				out, err := betService(d).Place(ctx, tx, brand.ID, fresh, in, r.Header.Get("Idempotency-Key"), points.Metadata{ActorType: "user", ActorID: fresh.User.ID, RequestID: requestID(r), IP: meta(r).IP})
				result, e := betError(out, err)
				if err == nil {
					result.Status = 201
				}
				return result, e
			})
		})
		handle("GET", "/bet-orders", func(w http.ResponseWriter, r *http.Request) {
			if bettingUnavailable(w, r, d) {
				return
			}
			brand, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			session, ok := currentBetSession(w, r, d, brand.ID)
			if !ok {
				return
			}
			limit, offset, ok := pageParams(r)
			if !ok {
				failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
				return
			}
			items, err := betService(d).Orders(r.Context(), brand.ID, session.Member.ID, limit, offset)
			result, e := betError(map[string]any{"items": items}, err)
			outputMutation(w, r, result, e)
		})
		handle("GET", "/bet-orders/{id}", func(w http.ResponseWriter, r *http.Request) {
			if bettingUnavailable(w, r, d) {
				return
			}
			brand, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			session, ok := currentBetSession(w, r, d, brand.ID)
			if !ok {
				return
			}
			id := r.PathValue("id")
			if !uuidPattern.MatchString(id) {
				failure(w, r, 400, "REQUEST_INVALID", "投注编号不正确")
				return
			}
			out, err := betService(d).Order(r.Context(), brand.ID, session.Member.ID, id)
			result, e := betError(out, err)
			outputMutation(w, r, result, e)
		})
		handle("POST", "/bet-orders/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
			if bettingUnavailable(w, r, d) {
				return
			}
			brand, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			id := r.PathValue("id")
			if !uuidPattern.MatchString(id) {
				failure(w, r, 400, "REQUEST_INVALID", "投注编号不正确")
				return
			}
			var in struct {
				Version int64  `json:"version"`
				Reason  string `json:"reason"`
			}
			if !decodeBody(w, r, &in) {
				return
			}
			session, ok := currentBetSession(w, r, d, brand.ID)
			if !ok {
				return
			}
			checkedBetWrite(w, r, d, brand.ID, "bet.order.cancel", id, "", in, session, func(ctx context.Context, tx pgx.Tx, fresh identity.Session) (mutation.Result, error) {
				out, err := betService(d).Cancel(ctx, tx, brand.ID, fresh, id, in.Version, in.Reason, points.Metadata{ActorType: "user", ActorID: fresh.User.ID, RequestID: requestID(r), IP: meta(r).IP})
				result, e := betError(out, err)
				return result, e
			})
		})
	}
}

func betAdminActor(w http.ResponseWriter, r *http.Request, d Dependencies, resource, action string, write bool) (access.Account, string, bool) {
	a, brand, ok := managementActor(w, r, d, resource, action)
	if !ok {
		return a, brand, false
	}
	if write && a.SuperAdmin {
		rejectAdmin(w, r, d, a, brand, resource+"."+action)
		return a, brand, false
	}
	return a, brand, true
}

func betAdminWrite(w http.ResponseWriter, r *http.Request, d Dependencies, a access.Account, brand, resource, action, operation, id string, body any, run func(context.Context, pgx.Tx, access.Account) (mutation.Result, error)) {
	betAdminWriteExtra(w, r, d, a, brand, resource, action, operation, id, body, nil, run)
}
func betAdminWriteExtra(w http.ResponseWriter, r *http.Request, d Dependencies, a access.Account, brand, resource, action, operation, id string, body any, extra func(access.Account) bool, run func(context.Context, pgx.Tx, access.Account) (mutation.Result, error)) {
	encoded, err := json.Marshal(body)
	if err != nil {
		failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
		return
	}
	var fresh access.Account
	result, err := d.Mutations.ExecuteChecked(r.Context(), brand, a.ID, operation, r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(id+":"+string(encoded)), func(ctx context.Context, tx pgx.Tx) error {
		var e error
		fresh, e = freshAdmin(ctx, tx, r, d, a, false)
		if errors.Is(e, adminsys.ErrDenied) || errors.Is(e, identity.ErrSession) {
			return identity.ErrSession
		}
		if e != nil {
			return e
		}
		if fresh.SuperAdmin || !access.Authorize(fresh, resource, action, access.ScopeBrand, brand) || (extra != nil && !extra(fresh)) {
			return betting.ErrDenied
		}
		return nil
	}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
		return run(ctx, tx, fresh)
	})
	if errors.Is(err, identity.ErrSession) {
		result, err = mutation.Fail(401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录"), nil
	} else if errors.Is(err, betting.ErrDenied) {
		result, err = mutation.Fail(403, "PERMISSION_DENIED", "无操作权限"), nil
	}
	finishAdminMutation(w, r, d, a, brand, resource+"."+action, result, err)
}

func bettingMeta(r *http.Request, a access.Account) points.Metadata {
	m := meta(r)
	return points.Metadata{ActorType: "admin", ActorID: a.ID, RequestID: m.RequestID, IP: m.IP}
}

func registerBetAdminRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	registerBetExceptionRoutes(handle, d)
	registerBetJudgmentRoutes(handle, d)
	registerSettlementPreviewRoutes(handle, d)
	registerSettlementJobRoutes(handle, d)
	registerCorrectionRoutes(handle, d)
	s := betService(d)
	handle("GET", "/bet-policy", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := betAdminActor(w, r, d, "bet_policy", "view", false)
		if !ok {
			return
		}
		out, err := s.BrandPolicy(r.Context(), brand)
		if err == nil && !adminReadAudit(w, r, d, a, brand, "bet_policy.view") {
			return
		}
		result, e := betError(out, err)
		outputMutation(w, r, result, e)
	})
	handle("PUT", "/bet-policy", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := betAdminActor(w, r, d, "bet_policy", "write", true)
		if !ok {
			return
		}
		var in struct {
			Version int64                     `json:"version"`
			Config  betting.BrandPolicyConfig `json:"config"`
			Reason  string                    `json:"reason"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		betAdminWrite(w, r, d, a, brand, "bet_policy", "write", "admin.bet_policy.write.brand", brand, in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, err := betting.WriteBrandPolicy(ctx, tx, brand, fresh, in.Version, in.Config, in.Reason, bettingMeta(r, fresh))
			return betError(out, err)
		})
	})
	handle("GET", "/games/{id}/bet-policy", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := betAdminActor(w, r, d, "bet_policy", "view", false)
		if !ok {
			return
		}
		id := r.PathValue("id")
		if !uuidPattern.MatchString(id) {
			failure(w, r, 400, "REQUEST_INVALID", "游戏编号不正确")
			return
		}
		out, err := s.GamePolicy(r.Context(), brand, id)
		if err == nil && !adminReadAudit(w, r, d, a, brand, "bet_policy.view") {
			return
		}
		result, e := betError(out, err)
		outputMutation(w, r, result, e)
	})
	handle("PUT", "/games/{id}/bet-policy", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := betAdminActor(w, r, d, "bet_policy", "write", true)
		if !ok {
			return
		}
		id := r.PathValue("id")
		if !uuidPattern.MatchString(id) {
			failure(w, r, 400, "REQUEST_INVALID", "游戏编号不正确")
			return
		}
		var in struct {
			Version int64                    `json:"version"`
			Config  betting.GamePolicyConfig `json:"config"`
			Reason  string                   `json:"reason"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		betAdminWrite(w, r, d, a, brand, "bet_policy", "write", "admin.bet_policy.write.game", id, in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, err := betting.WriteGamePolicy(ctx, tx, brand, id, fresh, in.Version, in.Config, in.Reason, bettingMeta(r, fresh))
			return betError(out, err)
		})
	})
	handle("GET", "/bet-orders", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := betAdminActor(w, r, d, "bet", "view", false)
		if !ok {
			return
		}
		member := r.URL.Query().Get("member_id")
		if member != "" && !uuidPattern.MatchString(member) {
			failure(w, r, 400, "REQUEST_INVALID", "成员编号不正确")
			return
		}
		limit, offset, valid := pageParams(r)
		if !valid {
			failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
			return
		}
		items, err := s.Orders(r.Context(), brand, member, limit, offset)
		if err == nil && !adminReadAudit(w, r, d, a, brand, "bet.view") {
			return
		}
		result, e := betError(map[string]any{"items": items}, err)
		outputMutation(w, r, result, e)
	})
	handle("GET", "/bet-orders/{id}", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := betAdminActor(w, r, d, "bet", "view", false)
		if !ok {
			return
		}
		id := r.PathValue("id")
		if !uuidPattern.MatchString(id) {
			failure(w, r, 400, "REQUEST_INVALID", "投注编号不正确")
			return
		}
		out, err := s.Order(r.Context(), brand, "", id)
		if err == nil && !adminReadAudit(w, r, d, a, brand, "bet.view") {
			return
		}
		result, e := betError(out, err)
		outputMutation(w, r, result, e)
	})
	handle("POST", "/bet-orders/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := betAdminActor(w, r, d, "bet", "cancel", true)
		if !ok {
			return
		}
		id := r.PathValue("id")
		if !uuidPattern.MatchString(id) {
			failure(w, r, 400, "REQUEST_INVALID", "投注编号不正确")
			return
		}
		var in struct {
			Version int64  `json:"version"`
			Reason  string `json:"reason"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		betAdminWrite(w, r, d, a, brand, "bet", "cancel", "admin.bet.cancel", id, in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, err := s.CancelAdmin(ctx, tx, brand, fresh, id, in.Version, in.Reason, bettingMeta(r, fresh))
			return betError(out, err)
		})
	})
}
