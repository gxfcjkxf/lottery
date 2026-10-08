package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rewards"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const rewardOrderPath = "/reward-orders"

var rewardPageInteger = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)

var errRewardActorContext = errors.New("reward actor context changed")

func rewardResult(out any, err error) (mutation.Result, error) {
	switch {
	case err == nil:
		return mutation.OK(200, out), nil
	case errors.Is(err, rewards.ErrInvalid):
		return mutation.Fail(400, "REWARD_INPUT_INVALID", "奖励请求参数不正确"), nil
	case errors.Is(err, rewards.ErrDenied):
		return mutation.Fail(403, "PERMISSION_DENIED", "无奖励操作权限"), nil
	case errors.Is(err, rewards.ErrNotFound):
		return mutation.Fail(404, "REWARD_NOT_FOUND", "当前品牌未找到该奖励订单或成员"), nil
	case errors.Is(err, rewards.ErrState):
		return mutation.Fail(409, "REWARD_STATE_CONFLICT", "当前奖励订单或品牌状态不允许此操作"), nil
	case errors.Is(err, rewards.ErrVersion):
		return mutation.Fail(409, "REWARD_VERSION_CONFLICT", "奖励订单版本已变化，请重新读取"), nil
	case errors.Is(err, rewards.ErrBusy):
		return mutation.Fail(503, "REWARD_BUSY", "奖励数据正在变化，请稍后重试"), nil
	case errors.Is(err, points.ErrPolicyLimit):
		return mutation.Fail(409, "REWARD_POLICY_LIMIT", "奖励积分超出当前积分规则限制"), nil
	default:
		// Corrupt balances, integer overflow and database failures are deliberately
		// kept opaque at the HTTP boundary.
		return mutation.Result{}, err
	}
}

func rewardBrand(w http.ResponseWriter, r *http.Request) (string, bool) {
	brand := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Brand-ID")))
	if !cycleCanonicalID(brand) {
		failure(w, r, 400, "REQUEST_INVALID", "必须选择有效品牌")
		return "", false
	}
	return brand, true
}

func rewardPage(r *http.Request) (int, int, bool) {
	limit, offset := 20, 0
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || r.URL.ForceQuery {
		return 0, 0, false
	}
	for key, list := range values {
		if len(list) != 1 || (key != "limit" && key != "offset") || !rewardPageInteger.MatchString(list[0]) {
			return 0, 0, false
		}
		value, parseErr := strconv.Atoi(list[0])
		if parseErr != nil {
			return 0, 0, false
		}
		if key == "limit" {
			limit = value
		} else {
			offset = value
		}
	}
	return limit, offset, limit >= 1 && limit <= 100 && offset >= 0 && offset <= 1_000_000
}

func rewardGetHasNoBody(w http.ResponseWriter, r *http.Request) bool {
	if len(r.TransferEncoding) != 0 || r.ContentLength > 0 {
		failure(w, r, 400, "REWARD_INPUT_INVALID", "奖励查询不接受请求正文")
		return false
	}
	if r.Body == nil || r.Body == http.NoBody {
		return true
	}
	probe, err := io.ReadAll(io.LimitReader(r.Body, 1))
	if len(probe) != 0 || err != nil && !errors.Is(err, io.EOF) {
		failure(w, r, 400, "REWARD_INPUT_INVALID", "奖励查询不接受请求正文")
		return false
	}
	return true
}

func decodeRewardBody(w http.ResponseWriter, r *http.Request, dst any) ([]byte, bool) {
	if !safeOrigin(r) {
		failure(w, r, 403, "CSRF_REJECTED", "请求来源不可信")
		return nil, false
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		failure(w, r, 415, "CONTENT_TYPE_INVALID", "需要 application/json")
		return nil, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	raw, err := io.ReadAll(r.Body)
	if err != nil || !utf8.Valid(raw) || json.Unmarshal(raw, dst) != nil {
		failure(w, r, 400, "REWARD_INPUT_INVALID", "奖励请求参数不正确")
		return nil, false
	}
	return raw, true
}

func rewardDBError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "55P03" || pgErr.Code == "40P01" || pgErr.Code == "40001") {
		return rewards.ErrBusy
	}
	return err
}

// freshRewardAuth holds the same access and brand gates through the business
// transaction. Read operations allow disabled brands; writes allow paused
// brands but reject disabled ones.
func freshRewardAuth(ctx context.Context, tx pgx.Tx, r *http.Request, d Dependencies, initial access.Account, brand, action string) (access.Account, error) {
	fresh, err := freshAdmin(ctx, tx, r, d, initial, false)
	if errors.Is(err, adminsys.ErrDenied) || errors.Is(err, identity.ErrSession) {
		return access.Account{}, identity.ErrSession
	}
	if err != nil {
		return access.Account{}, rewardDBError(err)
	}
	if !rewards.Allowed(fresh, brand, action) {
		return fresh, rewards.ErrDenied
	}
	var state string
	err = tx.QueryRow(ctx, `SELECT status FROM brands WHERE id=$1 FOR SHARE NOWAIT`, brand).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return fresh, rewards.ErrNotFound
	}
	if err != nil {
		return fresh, rewardDBError(err)
	}
	if action != "view" && state == "disabled" {
		return fresh, rewards.ErrState
	}
	return fresh, nil
}

func rewardWriteCheck(ctx context.Context, tx pgx.Tx, r *http.Request, d Dependencies, initial access.Account, brand, action string, fresh *access.Account) error {
	a, err := freshAdmin(ctx, tx, r, d, initial, false)
	if errors.Is(err, adminsys.ErrDenied) || errors.Is(err, identity.ErrSession) {
		return identity.ErrSession
	}
	if err != nil {
		return rewardDBError(err)
	}
	*fresh = a
	actor := r.Header.Get("X-Reward-Actor-ID")
	if !cycleCanonicalID(actor) || actor != a.ID {
		return errRewardActorContext
	}
	if !rewards.Allowed(a, brand, action) {
		return rewards.ErrDenied
	}
	var state string
	err = tx.QueryRow(ctx, `SELECT status FROM brands WHERE id=$1 FOR SHARE NOWAIT`, brand).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return rewards.ErrNotFound
	}
	if err != nil {
		return rewardDBError(err)
	}
	if state == "disabled" {
		return rewards.ErrState
	}
	return nil
}

func rewardDenyRead(ctx context.Context, tx pgx.Tx, r *http.Request, actor access.Account, brand string) error {
	_, err := audit.Append(ctx, tx, audit.Record{ActorType: "admin", ActorID: actor.ID, Action: "access.denied", ResourceType: "permission",
		Reason: "administrative permission denied", RequestID: requestID(r), IP: meta(r).IP,
		After: map[string]string{"attempted_brand": brand, "permission": "reward.view"}})
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		return err
	}
	return rewards.ErrDenied
}

func rewardRead(ctx context.Context, r *http.Request, d Dependencies, initial access.Account, brand, auditAction string, query func(pgx.Tx) (any, error)) (any, error) {
	tx, err := d.Admins.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, rewardDBError(err)
	}
	defer tx.Rollback(ctx)
	fresh, err := freshRewardAuth(ctx, tx, r, d, initial, brand, "view")
	if errors.Is(err, rewards.ErrDenied) {
		return nil, rewardDenyRead(ctx, tx, r, fresh, brand)
	}
	if err != nil {
		return nil, err
	}
	out, queryErr := query(tx)
	fresh, err = freshRewardAuth(ctx, tx, r, d, initial, brand, "view")
	if errors.Is(err, rewards.ErrDenied) {
		return nil, rewardDenyRead(ctx, tx, r, fresh, brand)
	}
	if err != nil {
		return nil, err
	}
	_, err = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: fresh.ID, Action: auditAction,
		ResourceType: "query", RequestID: requestID(r), IP: meta(r).IP})
	if err == nil {
		fresh, err = freshRewardAuth(ctx, tx, r, d, initial, brand, "view")
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		return nil, err
	}
	return out, queryErr
}

func rewardWriteFailure(w http.ResponseWriter, r *http.Request, d Dependencies, initial access.Account, brand, operation string, result mutation.Result, err error) {
	switch {
	case errors.Is(err, errRewardActorContext):
		result, err = mutation.Fail(401, "AUTH_ACTOR_CONTEXT_CHANGED", "管理员身份与确认时不同，请重新登录并核对"), nil
	case errors.Is(err, identity.ErrSession):
		result, err = mutation.Fail(401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录"), nil
	case errors.Is(err, rewards.ErrBusy):
		failure(w, r, 503, "REWARD_BUSY", "奖励数据正在变化，请稍后重试")
		return
	case errors.Is(err, rewards.ErrInvalid), errors.Is(err, rewards.ErrDenied), errors.Is(err, rewards.ErrNotFound),
		errors.Is(err, rewards.ErrState), errors.Is(err, rewards.ErrVersion), errors.Is(err, points.ErrPolicyLimit):
		result, err = rewardResult(nil, err)
	}
	finishAdminMutation(w, r, d, initial, brand, "reward."+operation, result, err)
}

func rewardReadFailure(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case errors.Is(err, identity.ErrSession):
		failure(w, r, 401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录")
	case errors.Is(err, rewards.ErrDenied):
		failure(w, r, 403, "PERMISSION_DENIED", "无奖励查询权限")
	case errors.Is(err, rewards.ErrBusy):
		failure(w, r, 503, "REWARD_BUSY", "奖励数据正在变化，请稍后重试")
	default:
		return false
	}
	return true
}

func registerRewardRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := rewards.Service{DB: d.Admins.DB}

	handle("GET", rewardOrderPath, func(w http.ResponseWriter, r *http.Request) {
		if !rewardGetHasNoBody(w, r) {
			return
		}
		brand, ok := rewardBrand(w, r)
		if !ok {
			return
		}
		limit, offset, ok := rewardPage(r)
		if !ok {
			failure(w, r, 400, "REWARD_INPUT_INVALID", "奖励分页参数不正确")
			return
		}
		initial, ok := adminAccount(w, r, d)
		if !ok {
			return
		}
		out, err := rewardRead(r.Context(), r, d, initial, brand, "reward.order.list", func(tx pgx.Tx) (any, error) {
			return s.OrdersTx(r.Context(), tx, brand, limit, offset)
		})
		if rewardReadFailure(w, r, err) {
			return
		}
		result, err := rewardResult(out, err)
		outputMutation(w, r, result, err)
	})

	handle("POST", rewardOrderPath, func(w http.ResponseWriter, r *http.Request) {
		brand, ok := rewardBrand(w, r)
		if !ok {
			return
		}
		if r.URL.RawQuery != "" || r.URL.ForceQuery {
			failure(w, r, 400, "REWARD_INPUT_INVALID", "奖励写接口不接受查询参数")
			return
		}
		var in rewards.GrantInput
		raw, ok := decodeRewardBody(w, r, &in)
		if !ok {
			return
		}
		initial, ok := adminAccount(w, r, d)
		if !ok {
			return
		}
		var fresh access.Account
		result, err := d.Mutations.ExecuteChecked(r.Context(), brand, initial.ID, "admin.reward_order.grant", r.Header.Get("Idempotency-Key"),
			d.Mutations.Fingerprint(brand+":"+string(raw)), func(ctx context.Context, tx pgx.Tx) error {
				return rewardWriteCheck(ctx, tx, r, d, initial, brand, "grant", &fresh)
			}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
				order, e := s.GrantTx(ctx, tx, brand, fresh, in, pointMeta(r, fresh))
				if errors.Is(e, rewards.ErrBusy) {
					return mutation.Result{}, e
				}
				mapped, e := rewardResult(order, e)
				if e == nil && mapped.Error == nil {
					mapped.Status = 201
				}
				return mapped, e
			})
		rewardWriteFailure(w, r, d, initial, brand, "order.grant", result, err)
	})

	handle("GET", rewardOrderPath+"/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !rewardGetHasNoBody(w, r) {
			return
		}
		brand, ok := rewardBrand(w, r)
		if !ok {
			return
		}
		id := r.PathValue("id")
		if !cycleCanonicalID(id) || r.URL.RawQuery != "" || r.URL.ForceQuery {
			failure(w, r, 400, "REWARD_INPUT_INVALID", "奖励订单编号或查询参数不正确")
			return
		}
		initial, ok := adminAccount(w, r, d)
		if !ok {
			return
		}
		out, err := rewardRead(r.Context(), r, d, initial, brand, "reward.order.read", func(tx pgx.Tx) (any, error) {
			return s.OrderTx(r.Context(), tx, brand, id)
		})
		if rewardReadFailure(w, r, err) {
			return
		}
		result, err := rewardResult(out, err)
		outputMutation(w, r, result, err)
	})

	handle("GET", rewardOrderPath+"/{id}/actions", func(w http.ResponseWriter, r *http.Request) {
		if !rewardGetHasNoBody(w, r) {
			return
		}
		brand, ok := rewardBrand(w, r)
		if !ok {
			return
		}
		id := r.PathValue("id")
		if !cycleCanonicalID(id) {
			failure(w, r, 400, "REWARD_INPUT_INVALID", "奖励订单编号不正确")
			return
		}
		limit, offset, ok := rewardPage(r)
		if !ok {
			failure(w, r, 400, "REWARD_INPUT_INVALID", "奖励分页参数不正确")
			return
		}
		initial, ok := adminAccount(w, r, d)
		if !ok {
			return
		}
		out, err := rewardRead(r.Context(), r, d, initial, brand, "reward.order.actions", func(tx pgx.Tx) (any, error) {
			return s.ActionsTx(r.Context(), tx, brand, id, limit, offset)
		})
		if rewardReadFailure(w, r, err) {
			return
		}
		result, err := rewardResult(out, err)
		outputMutation(w, r, result, err)
	})

	for _, action := range []string{"revoke", "retry-revocation"} {
		action := action
		permission := action
		operation := action
		if action == "retry-revocation" {
			permission = "retry"
			operation = "retry_revocation"
		}
		handle("POST", rewardOrderPath+"/{id}/"+action, func(w http.ResponseWriter, r *http.Request) {
			brand, ok := rewardBrand(w, r)
			if !ok {
				return
			}
			id := r.PathValue("id")
			if !cycleCanonicalID(id) {
				failure(w, r, 400, "REWARD_INPUT_INVALID", "奖励订单编号不正确")
				return
			}
			if r.URL.RawQuery != "" || r.URL.ForceQuery {
				failure(w, r, 400, "REWARD_INPUT_INVALID", "奖励写接口不接受查询参数")
				return
			}
			var in rewards.ActionInput
			raw, ok := decodeRewardBody(w, r, &in)
			if !ok {
				return
			}
			initial, ok := adminAccount(w, r, d)
			if !ok {
				return
			}
			var fresh access.Account
			result, err := d.Mutations.ExecuteChecked(r.Context(), brand, initial.ID, "admin.reward_order."+operation,
				r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(brand+":"+id+":"+string(raw)),
				func(ctx context.Context, tx pgx.Tx) error {
					return rewardWriteCheck(ctx, tx, r, d, initial, brand, permission, &fresh)
				}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
					var order rewards.Order
					var e error
					if action == "revoke" {
						order, e = s.RevokeTx(ctx, tx, brand, id, fresh, in, pointMeta(r, fresh))
					} else {
						order, e = s.RetryRevocationTx(ctx, tx, brand, id, fresh, in, pointMeta(r, fresh))
					}
					if errors.Is(e, rewards.ErrBusy) {
						return mutation.Result{}, e
					}
					return rewardResult(order, e)
				})
			rewardWriteFailure(w, r, d, initial, brand, "order."+operation, result, err)
		})
	}
}
