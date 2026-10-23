package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
	"net/http"
	"net/url"
	"strings"
)

func agentResult(out any, e error) (mutation.Result, error) {
	switch {
	case e == nil:
		return mutation.OK(200, out), nil
	case errors.Is(e, agency.ErrExists):
		return mutation.Fail(409, "IDENTITY_EXISTS", "代理账号已存在"), nil
	case errors.Is(e, agency.ErrInvalid):
		return mutation.Fail(400, "AGENT_INPUT_INVALID", "代理配置或筛选资料不正确"), nil
	case errors.Is(e, agency.ErrNotFound):
		return mutation.Fail(404, "AGENT_NOT_FOUND", "当前品牌未找到对应代理记录"), nil
	case errors.Is(e, agency.ErrDenied):
		return mutation.Fail(403, "AGENT_DENIED", "无权管理此代理或当前代理权限已停用"), nil
	case errors.Is(e, agency.ErrVersion):
		return mutation.Fail(409, "AGENT_VERSION_CONFLICT", "代理、上级或品牌政策版本已变化，请重新核对"), nil
	case errors.Is(e, agency.ErrLimit):
		return mutation.Fail(409, "AGENT_LIMIT_CONFLICT", "代理比例或层级超出上级/品牌上限，或已有下级超出新上限"), nil
	case errors.Is(e, agency.ErrCalendarRequired):
		return mutation.Fail(409, "AGENT_COMMISSION_CALENDAR_REQUIRED", "请先配置佣金日历，再预约代理模式切换"), nil
	case errors.Is(e, agency.ErrModePending):
		return mutation.Fail(409, "AGENT_MODE_PENDING", "整条代理线已有待生效模式，请在生效后再修改"), nil
	case errors.Is(e, agency.ErrState):
		return mutation.Fail(409, "AGENT_STATE_CONFLICT", "代理身份已存在、父级不允许发展、代理管理未启用或配置未变化"), nil
	default:
		return mutation.Result{}, e
	}
}
func agentPage(r *http.Request, parentAllowed bool) (int, int, *string, bool) {
	q, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil {
		return 0, 0, nil, false
	}
	var parent *string
	for key, v := range q {
		if len(v) != 1 {
			return 0, 0, nil, false
		}
		if key == "parent_id" && parentAllowed {
			if !uuidPattern.MatchString(v[0]) {
				return 0, 0, nil, false
			}
			parent = &v[0]
		} else if key != "limit" && key != "offset" && !((key == "q" || key == "online") && (r.URL.Path == "/api/v1/admin/agents" || r.URL.Path == "/api/v1/platform/agents")) {
			return 0, 0, nil, false
		}
	}
	limit, offset, ok := pageParams(r)
	return limit, offset, parent, ok
}
func agentWrite(w http.ResponseWriter, r *http.Request, d Dependencies, a access.Account, brand, resource, op, id string, body any, run func(context.Context, pgx.Tx, access.Account) (mutation.Result, error)) {
	raw, e := json.Marshal(body)
	if e != nil {
		failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
		return
	}
	var fresh access.Account
	s := agency.Service{DB: d.Admins.DB}
	result, e := d.Mutations.ExecuteChecked(r.Context(), brand, a.ID, op, r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(id+":"+string(raw)), func(ctx context.Context, tx pgx.Tx) error {
		var e error
		fresh, e = freshAdmin(ctx, tx, r, d, a, false)
		if errors.Is(e, adminsys.ErrDenied) || errors.Is(e, identity.ErrSession) {
			return identity.ErrSession
		}
		if e != nil {
			return e
		}
		if fresh.SuperAdmin || !access.Authorize(fresh, resource, "write", access.ScopeBrand, brand) {
			return agency.ErrDenied
		}
		_, e = s.LockPolicy(ctx, tx, brand)
		return e
	}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) { return run(ctx, tx, fresh) })
	if errors.Is(e, identity.ErrSession) {
		result, e = mutation.Fail(401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录"), nil
	} else if errors.Is(e, agency.ErrDenied) {
		result, e = mutation.Fail(403, "AGENT_DENIED", "无代理配置写权限"), nil
	}
	finishAdminMutation(w, r, d, a, brand, resource+".write", result, e)
}
func registerAgentAdminRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := agency.Service{DB: d.Admins.DB}
	handle("GET", "/agent-policy", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := pointAdminActor(w, r, d, "agent_policy", "view", false)
		if !ok {
			return
		}
		out, e := s.Policy(r.Context(), b)
		if e == nil && !adminReadAudit(w, r, d, a, b, "agent.policy.view") {
			return
		}
		res, e := agentResult(out, e)
		outputMutation(w, r, res, e)
	})
	handle("PUT", "/agent-policy", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := pointAdminActor(w, r, d, "agent_policy", "write", true)
		if !ok {
			return
		}
		var in agency.PolicyInput
		if !decodeBody(w, r, &in) {
			return
		}
		agentWrite(w, r, d, a, b, "agent_policy", "agent.policy.update", b, in, func(ctx context.Context, tx pgx.Tx, a access.Account) (mutation.Result, error) {
			out, e := s.SavePolicy(ctx, tx, b, a, in, pointMeta(r, a))
			return agentResult(out, e)
		})
	})

	handle("GET", "/agents/{id}/mode-change", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := pointAdminActor(w, r, d, "agent", "view", false)
		if !ok {
			return
		}
		out, e := s.ModeScope(r.Context(), b, r.PathValue("id"))
		if e == nil && !adminReadAudit(w, r, d, a, b, "agent.mode_scope.view") {
			return
		}
		res, e := agentResult(out, e)
		outputMutation(w, r, res, e)
	})
	handle("POST", "/agents/{id}/mode-change", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := pointAdminActor(w, r, d, "agent", "write", true)
		if !ok {
			return
		}
		id := r.PathValue("id")
		var in agency.ModeChangeInput
		if !decodeBody(w, r, &in) {
			return
		}
		agentWrite(w, r, d, a, b, "agent", "agent.mode.schedule", id, in, func(ctx context.Context, tx pgx.Tx, a access.Account) (mutation.Result, error) {
			out, e := s.ScheduleModeChange(ctx, tx, b, a, id, in, pointMeta(r, a))
			return agentResult(out, e)
		})
	})
	handle("GET", "/agents/{id}/history", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !uuidPattern.MatchString(id) {
			failure(w, r, 400, "REQUEST_INVALID", "代理编号不正确")
			return
		}

		a, b, ok := pointAdminActor(w, r, d, "agent", "view", false)
		if !ok {
			return
		}
		limit, offset, _, ok := agentPage(r, false)
		if !ok {
			failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
			return
		}
		out, e := s.History(r.Context(), b, &id, limit, offset)
		if e == nil && !adminReadAudit(w, r, d, a, b, "agent.history.view") {
			return
		}
		res, e := agentResult(out, e)
		outputMutation(w, r, res, e)
	})
	handle("GET", "/agents", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := pointAdminActor(w, r, d, "agent", "view", false)
		if !ok {
			return
		}
		limit, offset, _, ok := agentPage(r, false)
		if !ok {
			failure(w, r, 400, "REQUEST_INVALID", "代理列表筛选参数不正确")
			return
		}
		filter := agency.DirectoryFilter{Search: strings.TrimSpace(r.URL.Query().Get("q")), ViewPoints: access.Authorize(a, "wallet", "view", access.ScopePlatform, "") || access.Authorize(a, "wallet", "view", access.ScopeBrand, b)}
		for key, values := range r.URL.Query() {
			if key != "q" && key != "online" && key != "limit" && key != "offset" {
				failure(w, r, 400, "REQUEST_INVALID", "代理列表筛选参数不正确")
				return
			}
			if key == "online" {
				if values[0] != "true" && values[0] != "false" {
					failure(w, r, 400, "REQUEST_INVALID", "在线筛选参数不正确")
					return
				}
				online := values[0] == "true"
				filter.Online = &online
			}
		}
		out, e := s.List(r.Context(), b, filter, limit, offset)
		if e == nil && !adminReadAudit(w, r, d, a, b, "agent.list.view") {
			return
		}
		res, e := agentResult(out, e)
		outputMutation(w, r, res, e)
	})
	handle("GET", "/agents/tree", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := pointAdminActor(w, r, d, "agent", "view", false)
		if !ok {
			return
		}
		limit, offset, parent, ok := agentPage(r, true)
		if !ok {
			failure(w, r, 400, "REQUEST_INVALID", "代理树筛选参数不正确")
			return
		}
		out, e := s.Tree(r.Context(), b, parent, limit, offset)
		if e == nil && !adminReadAudit(w, r, d, a, b, "agent.tree.view") {
			return
		}
		res, e := agentResult(out, e)
		outputMutation(w, r, res, e)
	})
	handle("GET", "/agents/{id}", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := pointAdminActor(w, r, d, "agent", "view", false)
		if !ok {
			return
		}
		out, e := s.Node(r.Context(), b, r.PathValue("id"))
		if e == nil && !adminReadAudit(w, r, d, a, b, "agent.node.view") {
			return
		}
		res, e := agentResult(out, e)
		outputMutation(w, r, res, e)
	})
	handle("POST", "/agents", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := pointAdminActor(w, r, d, "agent", "write", true)
		if !ok {
			return
		}
		var in agency.CreateInput
		if !decodeBody(w, r, &in) {
			return
		}
		agentWrite(w, r, d, a, b, "agent", "agent.node.create", "", in, func(ctx context.Context, tx pgx.Tx, a access.Account) (mutation.Result, error) {
			out, e := s.Create(ctx, tx, b, a, in, pointMeta(r, a))
			res, e := agentResult(out, e)
			if e == nil && res.Status == 200 {
				res.Status = 201
			}
			return res, e
		})
	})
	handle("PUT", "/agents/{id}", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := pointAdminActor(w, r, d, "agent", "write", true)
		if !ok {
			return
		}
		id := r.PathValue("id")
		var in agency.UpdateInput
		if !decodeBody(w, r, &in) {
			return
		}
		agentWrite(w, r, d, a, b, "agent", "agent.node.update", id, in, func(ctx context.Context, tx pgx.Tx, a access.Account) (mutation.Result, error) {
			out, e := s.Update(ctx, tx, b, a, id, in, pointMeta(r, a))
			return agentResult(out, e)
		})
	})
}
func registerAgentUserRoutes(mux routeRegistrar, d Dependencies) {
	registerIndependentAgentRoutes(mux, d)
}
