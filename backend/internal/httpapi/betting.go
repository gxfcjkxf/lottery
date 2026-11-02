package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/compliance"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

func betService(d Dependencies) betting.Service { return betting.Service{DB: d.Admins.DB} }

func betError(data any, err error) (mutation.Result, error) {
	if result, ok := compliance.Rejection(err); ok {
		return result, nil
	}
	switch {
	case err == nil:
		return mutation.OK(200, data), nil
	case errors.Is(err, identity.ErrSession):
		return mutation.Fail(401, "AUTH_SESSION_REVOKED", "登录状态已过期，请重新登录"), nil
	case errors.Is(err, betting.ErrInvalid):
		return mutation.Fail(400, "BET_INPUT_INVALID", "投注请求或配置不正确"), nil
	case errors.Is(err, betting.ErrBrandPaused):
		return mutation.Fail(403, "BRAND_PAUSED", "品牌已暂停新投注；已有订单仍继续处理"), nil
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
				if operation == "configured.place" && subtle.ConstantTimeCompare([]byte(expectedContext), []byte(betActorContext(d, brand, fresh))) != 1 {
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
