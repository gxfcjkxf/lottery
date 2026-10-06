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
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"net/http"
	"net/url"
)

func agentResult(out any, e error) (mutation.Result, error) {
	switch {
	case e == nil:
		return mutation.OK(200, out), nil
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
		} else if key != "limit" && key != "offset" {
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
	for _, path := range []string{"/agent-policy/history", "/agents/{id}/history"} {
		handle("GET", path, func(w http.ResponseWriter, r *http.Request) {
			resource := "agent_policy"
			var id *string
			if path != "/agent-policy/history" {
				resource = "agent"
				v := r.PathValue("id")
				if !uuidPattern.MatchString(v) {
					failure(w, r, 400, "REQUEST_INVALID", "代理编号不正确")
					return
				}
				id = &v
			}
			a, b, ok := pointAdminActor(w, r, d, resource, "view", false)
			if !ok {
				return
			}
			limit, offset, _, ok := agentPage(r, false)
			if !ok {
				failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
				return
			}
			out, e := s.History(r.Context(), b, id, limit, offset)
			if e == nil && !adminReadAudit(w, r, d, a, b, "agent.history.view") {
				return
			}
			res, e := agentResult(out, e)
			outputMutation(w, r, res, e)
		})
	}
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
func registerAgentUserRoutes(mux *http.ServeMux, d Dependencies) {
	s := agency.Service{DB: d.Admins.DB}
	for _, prefix := range []string{"/api/v1", "/api/v1/b/{brandCode}"} {
		mux.HandleFunc("GET "+prefix+"/agent/me", func(w http.ResponseWriter, r *http.Request) {
			if bettingUnavailable(w, r, d) {
				return
			}
			b, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			v, ok := currentBetSession(w, r, d, b.ID)
			if !ok {
				return
			}
			out, e := s.Me(r.Context(), b.ID, v.Member.ID)
			res, e := agentResult(out, e)
			outputMutation(w, r, res, e)
		})
		mux.HandleFunc("GET "+prefix+"/agent/children", func(w http.ResponseWriter, r *http.Request) {
			if bettingUnavailable(w, r, d) {
				return
			}
			b, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			v, ok := currentBetSession(w, r, d, b.ID)
			if !ok {
				return
			}
			limit, offset, _, ok := agentPage(r, false)
			if !ok {
				failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
				return
			}
			own, e := s.Me(r.Context(), b.ID, v.Member.ID)
			var out agency.Tree
			if e == nil {
				out, e = s.Tree(r.Context(), b.ID, &own.ID, limit, offset)
			}
			res, e := agentResult(out, e)
			outputMutation(w, r, res, e)
		})
		mux.HandleFunc("PUT "+prefix+"/agent/children/{id}/config", func(w http.ResponseWriter, r *http.Request) {
			if bettingUnavailable(w, r, d) {
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
			id := r.PathValue("id")
			if !uuidPattern.MatchString(id) {
				failure(w, r, 400, "REQUEST_INVALID", "代理编号不正确")
				return
			}
			var in agency.ChildInput
			if !decodeBody(w, r, &in) {
				return
			}
			raw, e := json.Marshal(in)
			if e != nil {
				failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
				return
			}
			var fresh identity.Session
			res, e := d.Mutations.ExecuteChecked(r.Context(), b.ID, "agent-user:"+initial.User.ID+":"+initial.Member.ID, "agent.child.update", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(id+":"+string(raw)), func(ctx context.Context, tx pgx.Tx) error {
				var e error
				fresh, e = d.Identity.AuthenticateTx(ctx, tx, b.ID, requestToken(r, userCookieName(b.ID)))
				if e != nil {
					return e
				}
				if fresh.User.ID != initial.User.ID || fresh.Member.ID != initial.Member.ID {
					return identity.ErrSession
				}
				_, e = s.CheckUserWrite(ctx, tx, b.ID, fresh, id)
				return e
			}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
				out, e := s.UpdateChild(ctx, tx, b.ID, fresh, id, in, points.Metadata{RequestID: requestID(r), IP: meta(r).IP})
				return agentResult(out, e)
			})
			if errors.Is(e, identity.ErrSession) {
				res, e = mutation.Fail(401, "AUTH_SESSION_REVOKED", "登录状态已变化，请重新登录"), nil
			} else if errors.Is(e, agency.ErrDenied) {
				res, e = mutation.Fail(403, "AGENT_DENIED", "无权配置此直属下级或代理权限已停用"), nil
			}
			outputMutation(w, r, res, e)
		})
	}
}
