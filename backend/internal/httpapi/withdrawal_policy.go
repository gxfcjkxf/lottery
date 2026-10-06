package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/withdrawal"
	"github.com/jackc/pgx/v5"
)

func withdrawalPolicyWrite(w http.ResponseWriter, r *http.Request, d Dependencies, a access.Account, brand, operation, id string, body any, run managementAction) {
	encoded, e := json.Marshal(body)
	if e != nil {
		failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
		return
	}
	var fresh access.Account
	result, e := d.Mutations.ExecuteChecked(r.Context(), brand, a.ID, operation, r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(id+":"+string(encoded)), func(ctx context.Context, tx pgx.Tx) error {
		var e error
		fresh, e = freshAdmin(ctx, tx, r, d, a, false)
		if errors.Is(e, adminsys.ErrDenied) || errors.Is(e, identity.ErrSession) {
			return identity.ErrSession
		}
		if e != nil {
			return e
		}
		if fresh.SuperAdmin || !access.Authorize(fresh, "withdrawal_policy", "write", access.ScopeBrand, brand) {
			return withdrawal.ErrDenied
		}
		return nil
	}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) { return run(ctx, tx, fresh) })
	if errors.Is(e, identity.ErrSession) {
		result, e = mutation.Fail(401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录"), nil
	} else if errors.Is(e, withdrawal.ErrDenied) {
		result, e = mutation.Fail(403, "PERMISSION_DENIED", "无提现规则写权限"), nil
	}
	finishAdminMutation(w, r, d, a, brand, "withdrawal_policy.write", result, e)
}

func withdrawalPolicyResult(out any, err error) (mutation.Result, error) {
	switch {
	case errors.Is(err, withdrawal.ErrInvalid):
		return mutation.Fail(400, "WITHDRAWAL_POLICY_INPUT_INVALID", "提现规则配置或查询参数不正确"), nil
	case errors.Is(err, withdrawal.ErrNotFound):
		return mutation.Fail(404, "WITHDRAWAL_POLICY_NOT_FOUND", "当前品牌中未找到提现规则"), nil
	case errors.Is(err, withdrawal.ErrVersion):
		return mutation.Fail(409, "WITHDRAWAL_POLICY_VERSION_CONFLICT", "提现规则版本已变化，请重新读取并核对"), nil
	case errors.Is(err, withdrawal.ErrDenied):
		return mutation.Fail(403, "PERMISSION_DENIED", "无提现规则写权限"), nil
	case err != nil:
		return mutation.Result{}, err
	default:
		return mutation.OK(200, out), nil
	}
}
func registerWithdrawalPolicyRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := withdrawal.Service{DB: d.Admins.DB}
	for _, base := range []string{"/withdrawal-policy", "/games/{id}/withdrawal-policy"} {
		isGame := base != "/withdrawal-policy"
		handle("GET", base, func(w http.ResponseWriter, r *http.Request) {
			a, brand, ok := pointAdminActor(w, r, d, "withdrawal_policy", "view", false)
			if !ok {
				return
			}
			var out any
			var err error
			if isGame {
				out, err = s.GamePolicy(r.Context(), brand, r.PathValue("id"))
			} else {
				out, err = s.BrandPolicy(r.Context(), brand)
			}
			if err == nil && !adminReadAudit(w, r, d, a, brand, "withdrawal_policy.view") {
				return
			}
			result, e := withdrawalPolicyResult(out, err)
			outputMutation(w, r, result, e)
		})
		handle("GET", base+"/history", func(w http.ResponseWriter, r *http.Request) {
			a, brand, ok := pointAdminActor(w, r, d, "withdrawal_policy", "view", false)
			if !ok {
				return
			}
			limit, offset, ok := pageParams(r)
			if !ok {
				failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
				return
			}
			game := ""
			if isGame {
				game = r.PathValue("id")
			}
			out, err := s.History(r.Context(), brand, game, limit, offset)
			if err == nil && !adminReadAudit(w, r, d, a, brand, "withdrawal_policy.history.view") {
				return
			}
			result, e := withdrawalPolicyResult(map[string]any{"items": out, "limit": limit, "offset": offset}, err)
			outputMutation(w, r, result, e)
		})
		handle("PUT", base, func(w http.ResponseWriter, r *http.Request) {
			a, brand, ok := pointAdminActor(w, r, d, "withdrawal_policy", "write", true)
			if !ok {
				return
			}
			if isGame {
				game, ok := ruleID(w, r, "id")
				if !ok {
					return
				}
				var in withdrawal.GameInput
				if !decodeBody(w, r, &in) {
					return
				}
				withdrawalPolicyWrite(w, r, d, a, brand, "admin.withdrawal_policy.game.update", game, in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
					out, err := s.UpdateGame(ctx, tx, brand, game, fresh, in, pointMeta(r, fresh))
					return withdrawalPolicyResult(out, err)
				})
			} else {
				var in withdrawal.BrandInput
				if !decodeBody(w, r, &in) {
					return
				}
				withdrawalPolicyWrite(w, r, d, a, brand, "admin.withdrawal_policy.brand.update", brand, in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
					out, err := s.UpdateBrand(ctx, tx, brand, fresh, in, pointMeta(r, fresh))
					return withdrawalPolicyResult(out, err)
				})
			}
		})
	}
}
