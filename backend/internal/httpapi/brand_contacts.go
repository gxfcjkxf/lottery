package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/brandskin"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
	"net/http"
)

func registerBrandContactAdminRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := brandskin.Service{DB: d.Admins.DB}
	handle("GET", "/brand-contacts", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := presentationActor(w, r, d, "view")
		if !ok {
			return
		}
		if r.URL.RawQuery != "" {
			failure(w, r, 400, "REQUEST_INVALID", "此接口不接受查询参数")
			return
		}
		out, err := s.ReadContacts(r.Context(), b)
		if err == nil && !adminReadAudit(w, r, d, a, b, "brand_contacts.view") {
			return
		}
		result, e := presentationResult(out, err)
		outputMutation(w, r, result, e)
	})
	handle("PUT", "/brand-contacts", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := presentationActor(w, r, d, "write")
		if !ok {
			return
		}
		if r.URL.RawQuery != "" {
			failure(w, r, 400, "REQUEST_INVALID", "此接口不接受查询参数")
			return
		}
		var in brandskin.ContactInput
		if !decodeBody(w, r, &in) {
			return
		}
		raw, e := json.Marshal(in)
		if e != nil {
			failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
			return
		}
		var fresh access.Account
		result, e := d.Mutations.ExecuteChecked(r.Context(), b, a.ID, "admin.brand_contacts.update", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(b+":"+string(raw)), func(ctx context.Context, tx pgx.Tx) error {
			var e error
			fresh, e = freshAdmin(ctx, tx, r, d, a, false)
			if errors.Is(e, adminsys.ErrDenied) || errors.Is(e, identity.ErrSession) {
				return identity.ErrSession
			}
			if e != nil {
				return e
			}
			if !brandskin.Allowed(fresh, b, "write") {
				return brandskin.ErrDenied
			}
			return nil
		}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			out, e := s.UpdateContacts(ctx, tx, b, fresh, in, pointMeta(r, fresh))
			return presentationResult(out, e)
		})
		if errors.Is(e, identity.ErrSession) {
			result, e = mutation.Fail(401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录"), nil
		} else if errors.Is(e, brandskin.ErrDenied) {
			result, e = mutation.Fail(403, "PERMISSION_DENIED", "无品牌展示配置权限"), nil
		}
		finishAdminMutation(w, r, d, a, b, "brand_presentation.write", result, e)
	})
}

func registerPublicBrandContacts(mux routeRegistrar, d Dependencies) {
	for _, prefix := range []string{"/api/v1", "/api/v1/b/{brandCode}"} {
		mux.HandleFunc("GET "+prefix+"/brand-contacts", func(w http.ResponseWriter, r *http.Request) {
			brand, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			if d.Admins.DB == nil {
				failure(w, r, 503, "SERVICE_UNAVAILABLE", "联系方式暂不可用")
				return
			}
			out, err := (brandskin.Service{DB: d.Admins.DB}).ReadContacts(r.Context(), brand.ID)
			result, e := presentationResult(out.Config, err)
			outputMutation(w, r, result, e)
		})
	}
}
