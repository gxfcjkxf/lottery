package httpapi

import (
	"errors"
	"net/http"

	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
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
	handle("PUT", "/commission-policy", func(w http.ResponseWriter, r *http.Request) {
		failure(w, r, 403, "PERMISSION_DENIED", "佣金策略由平台统一管理")
	})
}
