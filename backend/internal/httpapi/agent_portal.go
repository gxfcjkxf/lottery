package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

func agentMemberRead(w http.ResponseWriter, r *http.Request, tx pgx.Tx, brand, agent, path string) (any, error) {
	ctx := r.Context()
	scope := "direct"
	if values, exists := r.URL.Query()["scope"]; exists {
		if len(values) != 1 || (values[0] != "direct" && values[0] != "line") {
			return nil, agency.ErrInvalid
		}
		scope = values[0]
	}
	member := r.PathValue("id")
	if member != "" {
		var allowed bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM brand_members m JOIN agent_nodes n ON n.brand_id=m.brand_id AND n.id::text=m.attribution_snapshot->>'agent_id' WHERE m.brand_id=$1 AND m.id=$2 AND (($4='direct' AND n.id=$3::uuid) OR ($4='line' AND $3::uuid=ANY(n.path))))`, brand, member, agent, scope).Scan(&allowed); err != nil {
			return nil, err
		}
		if !allowed {
			return nil, agency.ErrNotFound
		}
	}
	switch path {
	case "members":
		valid := true
		for key, values := range r.URL.Query() {
			if len(values) != 1 || (key != "limit" && key != "offset" && key != "q" && key != "scope") {
				valid = false
			}
		}
		limit, offset, pageValid := pageParams(r)
		valid = valid && pageValid
		if !valid {
			return nil, agency.ErrInvalid
		}
		var raw []byte
		err := tx.QueryRow(ctx, agentMembersSQL, brand, agent, limit, offset, r.URL.Query().Get("q"), scope).Scan(&raw)
		return json.RawMessage(raw), err
	case "members/{id}/bets":
		limit, _, ok := pageParams(r)
		if !ok {
			return nil, agency.ErrInvalid
		}
		return betting.HistoryTx(ctx, tx, brand, member, r.URL.Query().Get("game_id"), r.URL.Query().Get("cursor"), limit)
	default:
		wallet, err := (points.Store{}).Snapshot(ctx, tx, brand, member)
		if err != nil {
			return nil, err
		}
		var pending, net string
		q := `WITH orders AS (` + memberOrderMetricsSQL + `) SELECT coalesce(sum(stake) FILTER(WHERE pending),0)::text,coalesce(sum(net) FILTER(WHERE settled_at IS NOT NULL),0)::text FROM orders`
		if err = tx.QueryRow(ctx, q, brand, member).Scan(&pending, &net); err != nil {
			return nil, err
		}
		var memberInfo json.RawMessage
		if err = tx.QueryRow(ctx, `SELECT jsonb_build_object('id',m.id,'username',u.username,'display_name',m.display_name,'phone',coalesce(u.phone,'')) FROM brand_members m JOIN global_users u ON u.id=m.global_user_id WHERE m.brand_id=$1 AND m.id=$2`, brand, member).Scan(&memberInfo); err != nil {
			return nil, err
		}
		return map[string]any{"member": memberInfo, "wallet": wallet, "unsettled_points": pending, "net_points": net}, nil
	}
}
func agentPortalWrite(w http.ResponseWriter, r *http.Request, d Dependencies, brand string, initial identity.AgentSession, path string) {
	var input any
	switch path {
	case "password":
		input = &identity.AgentPasswordInput{}
	case "members":
		input = &identity.OperatorInput{}
	case "children":
		input = &agency.CreateInput{}
	default:
		input = &agency.ChildInput{}
	}
	if !decodeBody(w, r, input) {
		return
	}
	if path == "password" {
		allowed, e := d.Identity.AllowAttempt(r.Context(), d.Mutations, brand, "agent-password:"+initial.AccountID, meta(r))
		if e != nil {
			failure(w, r, 503, "SERVICE_UNAVAILABLE", "暂时无法操作")
			return
		}
		if !allowed {
			failure(w, r, 429, "AUTH_RATE_LIMITED", "登录尝试过多")
			return
		}
	}
	raw, err := json.Marshal(input)
	if err != nil {
		failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
		return
	}
	var fresh identity.AgentSession
	result, err := d.Mutations.ExecuteChecked(r.Context(), brand, "agent:"+initial.AccountID, "agent.portal."+path, r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(r.PathValue("id")+":"+string(raw)), func(ctx context.Context, tx pgx.Tx) error {
		var e error
		if path == "password" {
			// Match login's account -> session lock order before revalidating.
			var id string
			if e = tx.QueryRow(ctx, `SELECT id::text FROM agent_accounts WHERE brand_id=$1 AND id=$2 FOR UPDATE`, brand, initial.AccountID).Scan(&id); e != nil {
				return e
			}
		}
		fresh, e = d.Identity.AgentAuthenticateTx(ctx, tx, brand, requestToken(r, agentCookieName(brand)))
		if e != nil {
			return e
		}
		if fresh.AccountID != initial.AccountID {
			return identity.ErrSession
		}
		return nil
	}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
		s := agency.Service{DB: d.Admins.DB}
		metadata := points.Metadata{ActorType: "agent", ActorID: fresh.AccountID, RequestID: requestID(r), IP: meta(r).IP}
		switch v := input.(type) {
		case *identity.AgentPasswordInput:
			return d.Identity.AgentChangePassword(ctx, tx, fresh, *v, meta(r))
		case *identity.OperatorInput:
			return d.Identity.AgentCreateMember(ctx, tx, fresh, *v, meta(r))
		case *agency.CreateInput:
			out, e := s.CreateChild(ctx, tx, brand, fresh, *v, metadata)
			res, e := agentResult(out, e)
			if res.Status == 200 {
				res.Status = 201
			}
			return res, e
		default:
			out, e := s.UpdateChild(ctx, tx, brand, fresh, r.PathValue("id"), *input.(*agency.ChildInput), metadata)
			return agentResult(out, e)
		}
	})
	if errors.Is(err, identity.ErrSession) {
		failure(w, r, 401, "AUTH_SESSION_REVOKED", "代理登录已失效")
		return
	}
	if err == nil && result.Status == 200 && path == "password" {
		issueCookie(w, r, agentCookieName(brand), "", time.Now(), d.SecureCookies)
	}
	outputMutation(w, r, result, err)
}
