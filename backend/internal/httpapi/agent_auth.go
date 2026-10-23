package httpapi

import (
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"net/http"
	"strings"
	"time"
)

func agentCookieName(brand string) string {
	return "lottery_agent_" + strings.ReplaceAll(brand, "-", "")
}
func registerIndependentAgentRoutes(mux routeRegistrar, d Dependencies) {
	for _, prefix := range []string{"/api/v1", "/api/v1/b/{brandCode}"} {
		mux.HandleFunc("POST "+prefix+"/agent/login", func(w http.ResponseWriter, r *http.Request) {
			if d.Identity == nil || d.Admins.DB == nil || d.Mutations == nil {
				failure(w, r, 503, "SERVICE_UNAVAILABLE", "代理服务不可用")
				return
			}
			b, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			var in identity.AgentLoginInput
			if !decodeBody(w, r, &in) {
				return
			}
			allowed, err := d.Identity.AllowAttempt(r.Context(), d.Mutations, b.ID, "agent:"+in.Username, meta(r))
			if err != nil {
				failure(w, r, 503, "SERVICE_UNAVAILABLE", "暂时无法登录")
				return
			}
			if !allowed {
				failure(w, r, 429, "AUTH_RATE_LIMITED", "登录尝试过多")
				return
			}
			tx, err := d.Admins.DB.Begin(r.Context())
			if err != nil {
				failure(w, r, 503, "SERVICE_UNAVAILABLE", "暂时无法登录")
				return
			}
			defer tx.Rollback(r.Context())
			result, err := d.Identity.AgentLogin(r.Context(), tx, b.ID, in)
			if err == nil && result.Error == nil {
				err = tx.Commit(r.Context())
			}
			if err == nil && result.Error == nil {
				var v identity.AgentAuthentication
				if json.Unmarshal(result.Data, &v) != nil {
					failure(w, r, 503, "SERVICE_UNAVAILABLE", "登录回执无效")
					return
				}
				issueCookie(w, r, agentCookieName(b.ID), v.AccessToken, v.ExpiresAt, d.SecureCookies)
				v.AccessToken = ""
				result.Data, _ = json.Marshal(v)
			}
			outputMutation(w, r, result, err)
		})
		for _, action := range []struct{ method, path string }{{"GET", "me"}, {"GET", "children"}, {"GET", "wallet"}, {"GET", "commission"}, {"GET", "members"}, {"GET", "members/{id}/summary"}, {"GET", "members/{id}/bets"}, {"POST", "members"}, {"POST", "children"}, {"POST", "password"}, {"POST", "logout"}, {"PUT", "children/{id}/config"}} {
			action := action
			mux.HandleFunc(action.method+" "+prefix+"/agent/"+action.path, func(w http.ResponseWriter, r *http.Request) {
				if d.Identity == nil || d.Admins.DB == nil || d.Mutations == nil {
					failure(w, r, 503, "SERVICE_UNAVAILABLE", "代理服务不可用")
					return
				}
				b, ok := resolveBrand(w, r, d)
				if !ok {
					return
				}
				if action.method != "GET" && !safeOrigin(r) {
					failure(w, r, 403, "CSRF_REJECTED", "请求来源不可信")
					return
				}
				tx, err := d.Admins.DB.Begin(r.Context())
				if err != nil {
					failure(w, r, 503, "SERVICE_UNAVAILABLE", "暂时无法读取代理")
					return
				}
				defer tx.Rollback(r.Context())
				v, err := d.Identity.AgentAuthenticateTx(r.Context(), tx, b.ID, requestToken(r, agentCookieName(b.ID)))
				if err != nil {
					if errors.Is(err, identity.ErrSession) {
						failure(w, r, 401, "AUTH_SESSION_REVOKED", "代理登录已失效")
					} else {
						failure(w, r, 503, "SERVICE_UNAVAILABLE", "暂时无法验证代理会话")
					}
					return
				}
				s := agency.Service{DB: d.Admins.DB}
				own, err := s.Me(r.Context(), b.ID, v.AccountID)
				if err != nil {
					res, e := agentResult(nil, err)
					outputMutation(w, r, res, e)
					return
				}
				if action.method != "GET" && action.path != "logout" {
					if err = tx.Commit(r.Context()); err != nil {
						failure(w, r, 503, "SERVICE_UNAVAILABLE", "暂时无法操作")
						return
					}
					agentPortalWrite(w, r, d, b.ID, v, action.path)
					return
				}
				var out any
				switch action.path {
				case "commission":
					limit, offset, valid := pageParams(r)
					if !valid {
						failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
						return
					}
					for key, values := range r.URL.Query() {
						if (key != "limit" && key != "offset") || len(values) != 1 {
							failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
							return
						}
					}
					var raw []byte
					err = tx.QueryRow(r.Context(), `WITH entries AS (
 SELECT id,created_at,entry_type,((delta_snapshot->'commission'->>'available')::numeric+(delta_snapshot->'commission'->>'manual_frozen')::numeric+(delta_snapshot->'commission'->>'system_frozen')::numeric+(delta_snapshot->'commission'->>'withdrawal')::numeric) AS points
 FROM point_ledger_entries WHERE brand_id=$1 AND member_id=$2), postings AS (SELECT * FROM entries WHERE points<>0), page AS (
 SELECT id,created_at,entry_type,points::text FROM postings ORDER BY created_at DESC,id DESC LIMIT $3 OFFSET $4)
 SELECT jsonb_build_object('items',coalesce((SELECT jsonb_agg(to_jsonb(page) ORDER BY created_at DESC,id DESC) FROM page),'[]'::jsonb),'total_count',(SELECT count(*)::text FROM postings),'limit',$3::int,'offset',$4::int,
 'credited_points',(SELECT coalesce(sum(points) FILTER(WHERE points>0),0)::text FROM postings),'debited_points',(SELECT coalesce(-sum(points) FILTER(WHERE points<0),0)::text FROM postings))`, b.ID, v.AccountID, limit, offset).Scan(&raw)
					out = json.RawMessage(raw)
				case "me":
					var code *string
					err = tx.QueryRow(r.Context(), `SELECT (SELECT code FROM join_codes WHERE brand_id=$1 AND agent_id=$2 AND kind='agent' AND join_code_usable(join_codes) ORDER BY created_at,id LIMIT 1)`, b.ID, own.ID).Scan(&code)
					out = map[string]any{"account": v, "agent": own, "code": code}
				case "children":
					limit, offset, _, valid := agentPage(r, false)
					if !valid {
						failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
						return
					}
					out, err = s.Tree(r.Context(), b.ID, &own.ID, limit, offset)
				case "wallet":
					wallet, e := (points.Store{DB: d.Admins.DB}).Snapshot(r.Context(), tx, b.ID, v.AccountID)
					err = e
					out = map[string]any{"agent_account_id": v.AccountID, "account_id": wallet.AccountID, "available_points": wallet.AvailablePoints, "commission_points": wallet.CommissionPoints}
				case "logout":
					_, err = tx.Exec(r.Context(), `UPDATE agent_sessions SET revoked_at=clock_timestamp() WHERE id=$1`, v.ID)
					out = map[string]bool{"logged_out": true}
					if err == nil {
						issueCookie(w, r, agentCookieName(b.ID), "", time.Now(), d.SecureCookies)
					}
				default:
					out, err = agentMemberRead(w, r, tx, b.ID, own.ID, action.path)
				}
				if err == nil && action.path != "logout" {
					_, err = tx.Exec(r.Context(), `UPDATE agent_sessions SET last_seen_at=clock_timestamp() WHERE id=$1`, v.ID)
				}
				if err == nil {
					err = tx.Commit(r.Context())
				}
				res, e := agentResult(out, err)
				outputMutation(w, r, res, e)
			})
		}
	}
}
