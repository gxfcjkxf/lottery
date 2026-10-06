package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/notification"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

func notificationResult(out any, err error) (mutation.Result, error) {
	switch {
	case errors.Is(err, notification.ErrInvalid):
		return mutation.Fail(400, "NOTIFICATION_INPUT_INVALID", "消息请求格式不正确"), nil
	case errors.Is(err, notification.ErrNotFound):
		return mutation.Fail(404, "NOTIFICATION_NOT_FOUND", "当前账户或品牌没有对应消息"), nil
	case errors.Is(err, notification.ErrState):
		return mutation.Fail(409, "NOTIFICATION_STATE_CONFLICT", "消息投递状态已变化，请刷新后核对"), nil
	case errors.Is(err, notification.ErrDenied):
		return mutation.Fail(403, "PERMISSION_DENIED", "无通知重试权限"), nil
	case err != nil:
		return mutation.Result{}, err
	default:
		return mutation.OK(200, out), nil
	}
}
func registerNotificationRoutes(mux routeRegistrar, d Dependencies) {
	s := notification.Service{DB: d.Admins.DB}
	for _, prefix := range []string{"/api/v1", "/api/v1/b/{brandCode}"} {
		handle := func(method, path string, fn http.HandlerFunc) {
			mux.HandleFunc(method+" "+prefix+path, func(w http.ResponseWriter, r *http.Request) {
				if d.Identity == nil || d.Mutations == nil || d.Admins.DB == nil {
					failure(w, r, 503, "NOTIFICATIONS_UNAVAILABLE", "站内消息尚未配置")
					return
				}
				ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
				defer cancel()
				fn(w, r.WithContext(ctx))
			})
		}
		handle("GET", "/notifications", func(w http.ResponseWriter, r *http.Request) {
			b, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			limit, offset, ok := pageParams(r)
			if !ok {
				failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
				return
			}
			tx, e := d.Admins.DB.Begin(r.Context())
			if e != nil {
				failure(w, r, 503, "SERVICE_UNAVAILABLE", "消息服务暂时不可用")
				return
			}
			defer tx.Rollback(r.Context())
			session, e := d.Identity.AuthenticateTx(r.Context(), tx, b.ID, requestToken(r, userCookieName(b.ID)))
			if errors.Is(e, identity.ErrSession) {
				failure(w, r, 401, "AUTH_SESSION_REVOKED", "请先登录")
				return
			}
			if e != nil {
				failure(w, r, 503, "SERVICE_UNAVAILABLE", "暂时无法验证登录状态")
				return
			}
			out, e := s.ListTx(r.Context(), tx, b.ID, session.Member.ID, limit, offset)
			result, err := notificationResult(out, e)
			outputMutation(w, r, result, err)
		})
		handle("POST", "/notifications/read", func(w http.ResponseWriter, r *http.Request) {
			b, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			var in struct {
				IDs []string `json:"ids"`
			}
			if !decodeBody(w, r, &in) {
				return
			}
			if !notification.ValidIDs(in.IDs) {
				failure(w, r, 400, "NOTIFICATION_INPUT_INVALID", "需要 1-100 个不重复的消息编号")
				return
			}
			session, ok := currentBetSession(w, r, d, b.ID)
			if !ok {
				return
			}
			checkedBetWrite(w, r, d, b.ID, "notification.read", "", "", in, session, func(ctx context.Context, tx pgx.Tx, fresh identity.Session) (mutation.Result, error) {
				out, e := s.MarkRead(ctx, tx, b.ID, fresh.Member.ID, in.IDs)
				return notificationResult(out, e)
			})
		})
	}
}
func registerNotificationAdminRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	registerNotificationTemplateRoutes(handle, d)
	s := notification.Service{DB: d.Admins.DB}
	handle("GET", "/notification-deliveries", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := pointAdminActor(w, r, d, "notification", "view", false)
		if !ok {
			return
		}
		limit, offset, ok := pageParams(r)
		if !ok {
			failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
			return
		}
		out, e := s.Deliveries(r.Context(), brand, limit, offset)
		if e == nil && !adminReadAudit(w, r, d, a, brand, "notification.deliveries.view") {
			return
		}
		result, err := notificationResult(map[string]any{"items": out}, e)
		outputMutation(w, r, result, err)
	})
	handle("POST", "/notification-deliveries/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := pointAdminActor(w, r, d, "notification", "retry", true)
		if !ok {
			return
		}
		id := r.PathValue("id")
		if !uuidPattern.MatchString(id) {
			failure(w, r, 400, "REQUEST_INVALID", "投递编号不正确")
			return
		}
		var in struct {
			AttemptCount int    `json:"attempt_count"`
			Reason       string `json:"reason"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		raw, e := json.Marshal(in)
		if e != nil {
			failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
			return
		}
		var fresh access.Account
		result, e := d.Mutations.ExecuteChecked(r.Context(), brand, a.ID, "notification.retry", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(id+":"+string(raw)), func(ctx context.Context, tx pgx.Tx) error {
			var e error
			fresh, e = freshAdmin(ctx, tx, r, d, a, false)
			if errors.Is(e, adminsys.ErrDenied) || errors.Is(e, identity.ErrSession) {
				return identity.ErrSession
			}
			if e != nil {
				return e
			}
			if fresh.SuperAdmin || !access.Authorize(fresh, "notification", "retry", access.ScopeBrand, brand) {
				return notification.ErrDenied
			}
			return nil
		}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			out, e := s.Retry(ctx, tx, brand, fresh, id, in.AttemptCount, in.Reason, pointMeta(r, fresh))
			return notificationResult(out, e)
		})
		if errors.Is(e, identity.ErrSession) {
			result, e = mutation.Fail(401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录"), nil
		} else if errors.Is(e, notification.ErrDenied) {
			result, e = mutation.Fail(403, "PERMISSION_DENIED", "无通知重试权限"), nil
		}
		finishAdminMutation(w, r, d, a, brand, "notification.retry", result, e)
	})
}
