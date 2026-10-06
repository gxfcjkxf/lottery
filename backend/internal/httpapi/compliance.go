package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/compliance"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
)

func complianceActor(w http.ResponseWriter, r *http.Request, d Dependencies, resource, action string) (access.Account, string, bool) {
	a, ok := adminAccount(w, r, d)
	if !ok {
		return a, "", false
	}
	brand := strings.ToLower(r.Header.Get("X-Brand-ID"))
	if !uuidPattern.MatchString(brand) {
		failure(w, r, 400, "REQUEST_INVALID", "必须选择有效品牌")
		return a, brand, false
	}
	if !compliance.Allowed(a, brand, resource, action) {
		rejectAdmin(w, r, d, a, brand, resource+"."+action)
		return a, brand, false
	}
	return a, brand, true
}
func complianceResult(out any, e error) (mutation.Result, error) {
	switch {
	case errors.Is(e, compliance.ErrInvalid):
		return mutation.Fail(400, "COMPLIANCE_INPUT_INVALID", "合规配置或检查参数不正确"), nil
	case errors.Is(e, compliance.ErrDenied):
		return mutation.Fail(403, "PERMISSION_DENIED", "无合规配置或检查权限"), nil
	case errors.Is(e, compliance.ErrNotFound):
		return mutation.Fail(404, "COMPLIANCE_NOT_FOUND", "当前品牌合规配置不存在"), nil
	case errors.Is(e, compliance.ErrVersion):
		return mutation.Fail(409, "COMPLIANCE_VERSION_CONFLICT", "合规配置版本已变化，请重新读取核对"), nil
	case errors.Is(e, compliance.ErrState):
		return mutation.Fail(409, "COMPLIANCE_STATE_CONFLICT", "停用品牌合规配置仅可读取"), nil
	case e != nil:
		return mutation.Result{}, e
	default:
		return mutation.OK(200, out), nil
	}
}
func complianceWrite(w http.ResponseWriter, r *http.Request, d Dependencies, a access.Account, brand, resource, action, operation string, body any, run managementAction) {
	if r.URL.RawQuery != "" {
		failure(w, r, 400, "REQUEST_INVALID", "此写接口不接受查询参数")
		return
	}
	raw, e := json.Marshal(body)
	if e != nil {
		failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
		return
	}
	var fresh access.Account
	result, e := d.Mutations.ExecuteChecked(r.Context(), brand, a.ID, operation, r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(brand+":"+string(raw)), func(ctx context.Context, tx pgx.Tx) error {
		var e error
		fresh, e = freshAdmin(ctx, tx, r, d, a, false)
		if errors.Is(e, adminsys.ErrDenied) || errors.Is(e, identity.ErrSession) {
			return identity.ErrSession
		}
		if e != nil {
			return e
		}
		if !compliance.Allowed(fresh, brand, resource, action) {
			return compliance.ErrDenied
		}
		var state string
		if e = tx.QueryRow(ctx, `SELECT status FROM brands WHERE id=$1 FOR SHARE`, brand).Scan(&state); errors.Is(e, pgx.ErrNoRows) {
			return compliance.ErrNotFound
		} else if e != nil {
			return e
		}
		if state == "disabled" {
			return compliance.ErrState
		}
		return nil
	}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) { return run(ctx, tx, fresh) })
	if errors.Is(e, identity.ErrSession) {
		result, e = mutation.Fail(401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录"), nil
	} else if errors.Is(e, compliance.ErrDenied) || errors.Is(e, compliance.ErrState) || errors.Is(e, compliance.ErrNotFound) {
		result, e = complianceResult(nil, e)
	}
	finishAdminMutation(w, r, d, a, brand, resource+"."+action, result, e)
}
func registerComplianceRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := compliance.Service{DB: d.Admins.DB}
	handle("GET", "/compliance-gates", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := complianceActor(w, r, d, "compliance_check", "view")
		if !ok {
			return
		}
		limit, offset, filters, ok := joinCodePage(r, "operation")
		operation := filters["operation"]
		if !ok || operation != "" && operation != "registration" && operation != "betting" {
			failure(w, r, 400, "REQUEST_INVALID", "分页或业务类型不正确")
			return
		}
		out, e := s.Gates(r.Context(), brand, operation, limit, offset)
		if e == nil && !adminReadAudit(w, r, d, a, brand, "compliance_gate.view") {
			return
		}
		result, e := complianceResult(out, e)
		outputMutation(w, r, result, e)
	})
	handle("GET", "/compliance-policy", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := complianceActor(w, r, d, "compliance_policy", "view")
		if !ok {
			return
		}
		if r.URL.RawQuery != "" {
			failure(w, r, 400, "REQUEST_INVALID", "此接口不接受查询参数")
			return
		}
		out, e := s.Read(r.Context(), brand)
		if e == nil && !adminReadAudit(w, r, d, a, brand, "compliance_policy.view") {
			return
		}
		result, e := complianceResult(out, e)
		outputMutation(w, r, result, e)
	})
	handle("GET", "/compliance-policy/history", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := complianceActor(w, r, d, "compliance_policy", "view")
		if !ok {
			return
		}
		limit, offset, ok := brandOperationPage(r)
		if !ok {
			failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
			return
		}
		out, e := s.History(r.Context(), brand, limit, offset)
		if e == nil && !adminReadAudit(w, r, d, a, brand, "compliance_policy.history.view") {
			return
		}
		result, e := complianceResult(out, e)
		outputMutation(w, r, result, e)
	})
	handle("PUT", "/compliance-policy", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := complianceActor(w, r, d, "compliance_policy", "write")
		if !ok {
			return
		}
		var in compliance.Input
		if !decodeBody(w, r, &in) {
			return
		}
		complianceWrite(w, r, d, a, brand, "compliance_policy", "write", "admin.compliance.policy.update", in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, e := s.Update(ctx, tx, brand, fresh, in, pointMeta(r, fresh))
			return complianceResult(out, e)
		})
	})
	handle("POST", "/compliance-checks", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := complianceActor(w, r, d, "compliance_check", "run")
		if !ok {
			return
		}
		var in compliance.CheckInput
		if !decodeBody(w, r, &in) {
			return
		}
		complianceWrite(w, r, d, a, brand, "compliance_check", "run", "admin.compliance.check", in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, e := s.Check(ctx, tx, brand, fresh, in, pointMeta(r, fresh))
			result, e := complianceResult(out, e)
			if e == nil && result.Error == nil {
				result.Status = 201
			}
			return result, e
		})
	})
	handle("GET", "/compliance-checks", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := complianceActor(w, r, d, "compliance_check", "view")
		if !ok {
			return
		}
		limit, offset, filters, ok := joinCodePage(r, "operation")
		if !ok || filters["operation"] != "" && !compliance.ValidOperation(filters["operation"]) {
			failure(w, r, 400, "REQUEST_INVALID", "分页或检查类型不正确")
			return
		}
		out, e := s.Decisions(r.Context(), brand, filters["operation"], limit, offset)
		if e == nil && !adminReadAudit(w, r, d, a, brand, "compliance_check.view") {
			return
		}
		result, e := complianceResult(out, e)
		outputMutation(w, r, result, e)
	})
}
