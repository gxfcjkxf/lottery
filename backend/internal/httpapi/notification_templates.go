package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/notification"
	"github.com/jackc/pgx/v5"
)

func templateActor(w http.ResponseWriter, r *http.Request, d Dependencies, action string) (access.Account, string, bool) {
	a, ok := adminAccount(w, r, d)
	if !ok {
		return a, "", false
	}
	brand := strings.ToLower(r.Header.Get("X-Brand-ID"))
	if !uuidPattern.MatchString(brand) {
		failure(w, r, 400, "REQUEST_INVALID", "必须选择有效品牌")
		return a, brand, false
	}
	if !notification.AllowedTemplate(a, brand, action) {
		rejectAdmin(w, r, d, a, brand, "notification_template."+action)
		return a, brand, false
	}
	return a, brand, true
}
func templateResult(v any, e error) (mutation.Result, error) {
	switch {
	case errors.Is(e, notification.ErrInvalid):
		return mutation.Fail(400, "NOTIFICATION_TEMPLATE_INPUT_INVALID", "通知模板格式或占位符不正确"), nil
	case errors.Is(e, notification.ErrNotFound):
		return mutation.Fail(404, "NOTIFICATION_TEMPLATE_NOT_FOUND", "当前品牌模板不存在"), nil
	case errors.Is(e, notification.ErrDenied):
		return mutation.Fail(403, "PERMISSION_DENIED", "无通知模板权限"), nil
	case errors.Is(e, notification.ErrTemplateVersion):
		return mutation.Fail(409, "NOTIFICATION_TEMPLATE_VERSION_CONFLICT", "模板版本已变化，请重新读取核对"), nil
	case errors.Is(e, notification.ErrState):
		return mutation.Fail(409, "NOTIFICATION_TEMPLATE_STATE_CONFLICT", "停用品牌模板仅可读取"), nil
	case e != nil:
		return mutation.Result{}, e
	default:
		return mutation.OK(200, v), nil
	}
}
func templateFresh(ctx context.Context, tx pgx.Tx, r *http.Request, d Dependencies, a access.Account, brand, action string) (access.Account, error) {
	fresh, e := freshAdmin(ctx, tx, r, d, a, false)
	if errors.Is(e, identity.ErrSession) || errors.Is(e, adminsys.ErrDenied) {
		return fresh, identity.ErrSession
	}
	if e != nil {
		return fresh, e
	}
	if !notification.AllowedTemplate(fresh, brand, action) {
		return fresh, notification.ErrDenied
	}
	return fresh, nil
}
func templateRead(w http.ResponseWriter, r *http.Request, d Dependencies, a access.Account, brand, action string, query map[string]any, run func(pgx.Tx) (any, error)) {
	tx, e := d.Admins.DB.Begin(r.Context())
	if e != nil {
		outputMutation(w, r, mutation.Result{}, e)
		return
	}
	defer tx.Rollback(r.Context())
	_, e = templateFresh(r.Context(), tx, r, d, a, brand, "view")
	var out any
	if e == nil {
		out, e = run(tx)
	}
	if e == nil {
		_, e = templateFresh(r.Context(), tx, r, d, a, brand, "view")
	}
	if e == nil {
		_, e = audit.Append(r.Context(), tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: action, ResourceType: "notification_template", ResourceID: brand, RequestID: requestID(r), IP: meta(r).IP, After: query})
	}
	if e == nil {
		e = tx.Commit(r.Context())
	}
	if errors.Is(e, identity.ErrSession) {
		outputMutation(w, r, mutation.Fail(401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录"), nil)
		return
	}
	result, e := templateResult(out, e)
	outputMutation(w, r, result, e)
}
func registerNotificationTemplateRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := notification.Service{DB: d.Admins.DB}
	handle("GET", "/notification-templates", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := templateActor(w, r, d, "view")
		if !ok {
			return
		}
		if r.URL.RawQuery != "" {
			failure(w, r, 400, "REQUEST_INVALID", "模板列表不接受查询参数")
			return
		}
		templateRead(w, r, d, a, brand, "notification.template.list", nil, func(tx pgx.Tx) (any, error) {
			items, e := s.TemplatesTx(r.Context(), tx, brand)
			return map[string]any{"items": items}, e
		})
	})
	handle("GET", "/notification-templates/{key}/history", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := templateActor(w, r, d, "view")
		if !ok {
			return
		}
		key := r.PathValue("key")
		limit, offset, ok := brandOperationPage(r)
		if !ok || !notification.ValidTemplateKey(key) {
			failure(w, r, 400, "NOTIFICATION_TEMPLATE_INPUT_INVALID", "模板标识或分页参数不正确")
			return
		}
		templateRead(w, r, d, a, brand, "notification.template.history", map[string]any{"key": key, "limit": limit, "offset": offset}, func(tx pgx.Tx) (any, error) {
			items, e := s.TemplateHistoryTx(r.Context(), tx, brand, key, limit, offset)
			return map[string]any{"items": items}, e
		})
	})
	handle("PUT", "/notification-templates/{key}", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := templateActor(w, r, d, "write")
		if !ok {
			return
		}
		key := r.PathValue("key")
		if r.URL.RawQuery != "" || !notification.ValidTemplateKey(key) {
			failure(w, r, 400, "NOTIFICATION_TEMPLATE_INPUT_INVALID", "模板标识或查询参数不正确")
			return
		}
		var in notification.UpdateInput
		if !decodeBody(w, r, &in) {
			return
		}
		if e := notification.ValidateContent(key, in.Content); e != nil {
			result, e := templateResult(nil, e)
			outputMutation(w, r, result, e)
			return
		}
		raw, e := json.Marshal(in)
		if e != nil {
			failure(w, r, 400, "NOTIFICATION_TEMPLATE_INPUT_INVALID", "请求格式不正确")
			return
		}
		var fresh access.Account
		result, e := d.Mutations.ExecuteChecked(r.Context(), brand, a.ID, "notification.template.update", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(brand+":"+key+":"+string(raw)), func(ctx context.Context, tx pgx.Tx) error {
			var e error
			fresh, e = templateFresh(ctx, tx, r, d, a, brand, "write")
			if e != nil {
				return e
			}
			var state string
			e = tx.QueryRow(ctx, `SELECT status FROM brands WHERE id=$1 FOR SHARE`, brand).Scan(&state)
			if errors.Is(e, pgx.ErrNoRows) {
				return notification.ErrNotFound
			}
			if e != nil {
				return e
			}
			if state == "disabled" {
				return notification.ErrState
			}
			return nil
		}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			v, e := s.UpdateTemplate(ctx, tx, brand, key, fresh, in, pointMeta(r, fresh))
			return templateResult(v, e)
		})
		if errors.Is(e, identity.ErrSession) {
			result, e = mutation.Fail(401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录"), nil
		} else if errors.Is(e, notification.ErrDenied) || errors.Is(e, notification.ErrState) || errors.Is(e, notification.ErrNotFound) {
			result, e = templateResult(nil, e)
		}
		finishAdminMutation(w, r, d, a, brand, "notification.template.update", result, e)
	})
}
