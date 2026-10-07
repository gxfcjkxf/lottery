package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
)

func commissionPolicyResult(out any, err error) (mutation.Result, error) {
	switch {
	case errors.Is(err, commission.ErrInvalid):
		return mutation.Fail(400, "COMMISSION_POLICY_INPUT_INVALID", "佣金规则配置或查询参数不正确"), nil
	case errors.Is(err, commission.ErrNotFound):
		return mutation.Fail(404, "COMMISSION_POLICY_NOT_FOUND", "当前品牌未找到佣金规则"), nil
	case errors.Is(err, commission.ErrVersion):
		return mutation.Fail(409, "COMMISSION_POLICY_VERSION_CONFLICT", "佣金规则版本已变化，请重新读取并核对"), nil
	case errors.Is(err, commission.ErrPolicyEvidence):
		return mutation.Fail(409, "COMMISSION_POLICY_EVIDENCE_CONFLICT", "代理周期规则不匹配或佣金规则证据不可用"), nil
	case errors.Is(err, commission.ErrDenied):
		return mutation.Fail(403, "PERMISSION_DENIED", "无佣金规则写权限"), nil
	case err != nil:
		return mutation.Result{}, err
	default:
		return mutation.OK(200, out), nil
	}
}

func commissionPolicyWrite(w http.ResponseWriter, r *http.Request, d Dependencies, a access.Account, brand string, in commission.PolicyInput, run managementAction) {
	encoded, err := json.Marshal(in)
	if err != nil {
		failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
		return
	}
	var fresh access.Account
	result, err := d.Mutations.ExecuteChecked(r.Context(), brand, a.ID, "admin.commission_policy.update", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(brand+":"+string(encoded)), func(ctx context.Context, tx pgx.Tx) error {
		var e error
		fresh, e = freshAdmin(ctx, tx, r, d, a, false)
		if errors.Is(e, adminsys.ErrDenied) || errors.Is(e, identity.ErrSession) {
			return identity.ErrSession
		}
		if e != nil {
			return e
		}
		if fresh.SuperAdmin || !access.Authorize(fresh, "commission_policy", "write", access.ScopeBrand, brand) {
			return commission.ErrDenied
		}
		return nil
	}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
		return run(ctx, tx, fresh)
	})
	if errors.Is(err, identity.ErrSession) {
		result, err = mutation.Fail(401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录"), nil
	} else if errors.Is(err, commission.ErrDenied) {
		result, err = mutation.Fail(403, "PERMISSION_DENIED", "无佣金规则写权限"), nil
	}
	finishAdminMutation(w, r, d, a, brand, "commission_policy.write", result, err)
}

func registerCommissionPolicyRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := commission.Service{DB: d.Admins.DB}
	handle("GET", "/commission-policy", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := pointAdminActor(w, r, d, "commission_policy", "view", false)
		if !ok {
			return
		}
		out, err := s.Policy(r.Context(), brand)
		if err == nil && !adminReadAudit(w, r, d, a, brand, "commission_policy.view") {
			return
		}
		result, e := commissionPolicyResult(out, err)
		outputMutation(w, r, result, e)
	})
	handle("GET", "/commission-policy/history", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := pointAdminActor(w, r, d, "commission_policy", "view", false)
		if !ok {
			return
		}
		limit, offset, _, ok := agentPage(r, false)
		if !ok {
			failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
			return
		}
		out, err := s.History(r.Context(), brand, limit, offset)
		if err == nil && !adminReadAudit(w, r, d, a, brand, "commission_policy.history.view") {
			return
		}
		result, e := commissionPolicyResult(map[string]any{"items": out, "limit": limit, "offset": offset}, err)
		outputMutation(w, r, result, e)
	})
	handle("PUT", "/commission-policy", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := pointAdminActor(w, r, d, "commission_policy", "write", true)
		if !ok {
			return
		}
		var in commission.PolicyInput
		if !decodeBody(w, r, &in) {
			return
		}
		commissionPolicyWrite(w, r, d, a, brand, in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, err := s.Update(ctx, tx, brand, fresh, in, pointMeta(r, fresh))
			return commissionPolicyResult(out, err)
		})
	})
}
