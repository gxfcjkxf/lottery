package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
)

// Account and role administration does not enable platform financial or member writes.
func platformAccountWriteRoute(method, path string) bool {
	return method == "POST" && (path == "/accounts" || path == "/accounts/{id}/reset-password" || path == "/roles" || path == "/platform-accounts" || path == "/platform-accounts/{id}/reset-password") ||
		method == "PATCH" && (path == "/accounts/{id}" || path == "/roles/{id}" || path == "/platform-accounts/{id}")
}

func platformAccountActor(w http.ResponseWriter, r *http.Request, d Dependencies, resource, action string) (access.Account, bool) {
	a, ok := adminAccount(w, r, d)
	if !ok {
		return a, false
	}
	if !a.SuperAdmin || !access.Authorize(a, resource, action, access.ScopePlatform, "") {
		rejectAdmin(w, r, d, a, "", resource+"."+action+".platform")
		return a, false
	}
	if r.Header.Get("X-Brand-ID") != "" {
		failure(w, r, 400, "REQUEST_INVALID", "平台管理员账户操作不接受品牌头")
		return a, false
	}
	return a, true
}

func platformAccountWrite(w http.ResponseWriter, r *http.Request, d Dependencies, actor access.Account, operation, id string, body any, run managementAction) {
	raw, err := json.Marshal(body)
	if err != nil {
		failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
		return
	}
	var fresh access.Account
	result, err := d.Mutations.ExecuteChecked(r.Context(), "", actor.ID, operation, r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(id+":"+string(raw)), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		fresh, err = freshAdmin(ctx, tx, r, d, actor, true)
		if errors.Is(err, adminsys.ErrDenied) || errors.Is(err, identity.ErrSession) {
			return identity.ErrSession
		}
		if err != nil {
			return err
		}
		if !fresh.SuperAdmin || !access.Authorize(fresh, "admin", "write", access.ScopePlatform, "") {
			return adminsys.ErrDenied
		}
		return nil
	}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) { return run(ctx, tx, fresh) })
	if errors.Is(err, identity.ErrSession) {
		result, err = mutation.Fail(401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录"), nil
	} else if errors.Is(err, adminsys.ErrDenied) {
		result, err = mutation.Fail(403, "PERMISSION_DENIED", "无平台账号管理权限"), nil
	}
	finishAdminMutation(w, r, d, actor, "", "admin.write.platform", result, err)
}

func registerPlatformAccountRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	for _, kind := range []string{"platform-accounts", "platform-roles"} {
		handle("GET", "/"+kind, func(w http.ResponseWriter, r *http.Request) {
			resource := "admin"
			if kind == "platform-roles" {
				resource = "role"
			}
			actor, ok := platformAccountActor(w, r, d, resource, "view")
			if !ok {
				return
			}
			limit, offset, ok := pageParams(r)
			if !ok {
				failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
				return
			}
			var items any
			var err error
			if kind == "platform-accounts" {
				items, err = d.Admins.ListPlatformAdmins(r.Context(), actor, limit, offset)
			} else {
				items, err = d.Admins.ListPlatformRoles(r.Context(), actor, limit, offset)
			}
			if err != nil {
				managementFailure(w, r, d, actor, "", resource+".view.platform", err)
				return
			}
			if adminReadAudit(w, r, d, actor, "", resource+".view.platform") {
				respond(w, r, 200, map[string]any{"items": items})
			}
		})
	}
	handle("POST", "/platform-accounts", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := platformAccountActor(w, r, d, "admin", "write")
		if !ok {
			return
		}
		var in struct {
			Username string   `json:"username"`
			Password string   `json:"password"`
			RoleIDs  []string `json:"role_ids"`
			Reason   string   `json:"reason"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		if !validRoleIDs(in.RoleIDs) {
			failure(w, r, 400, "REQUEST_INVALID", "至少选择一个平台角色")
			return
		}
		platformAccountWrite(w, r, d, actor, "platform.account.create", "", in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			hash, fail, err := managementPassword(ctx, d, in.Password)
			if err != nil || fail.Error != nil {
				return fail, err
			}
			record, err := d.Admins.CreatePlatformAdmin(ctx, tx, fresh, adminsys.AdminInput{Username: in.Username, RoleIDs: in.RoleIDs, Status: "active", Reason: in.Reason}, hash, meta(r))
			if err != nil {
				return managementError(err)
			}
			return mutation.OK(201, record), nil
		})
	})
	handle("PATCH", "/platform-accounts/{id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := platformAccountActor(w, r, d, "admin", "write")
		if !ok {
			return
		}
		id := r.PathValue("id")
		var in adminsys.AdminInput
		if !decodeBody(w, r, &in) {
			return
		}
		if !uuidPattern.MatchString(id) || !validRoleIDs(in.RoleIDs) || in.Username != "" {
			failure(w, r, 400, "REQUEST_INVALID", "账号和角色编号不正确，用户名不能修改")
			return
		}
		platformAccountWrite(w, r, d, actor, "platform.account.update", id, in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			record, err := d.Admins.UpdatePlatformAdmin(ctx, tx, fresh, id, in, meta(r))
			if err != nil {
				return managementError(err)
			}
			return mutation.OK(200, record), nil
		})
	})
	handle("POST", "/platform-accounts/{id}/reset-password", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := platformAccountActor(w, r, d, "admin", "write")
		if !ok {
			return
		}
		id := r.PathValue("id")
		var in struct {
			Version  int64  `json:"version"`
			Password string `json:"password"`
			Reason   string `json:"reason"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		if !uuidPattern.MatchString(id) || in.Version < 1 {
			failure(w, r, 400, "REQUEST_INVALID", "账号编号或版本不正确")
			return
		}
		platformAccountWrite(w, r, d, actor, "platform.account.password_reset", id, in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			hash, fail, err := managementPassword(ctx, d, in.Password)
			if err != nil || fail.Error != nil {
				return fail, err
			}
			auditID, err := d.Admins.ResetPlatformAdminPassword(ctx, tx, fresh, id, in.Version, hash, in.Reason, meta(r))
			if err != nil {
				return managementError(err)
			}
			return mutation.OK(200, map[string]string{"audit_log_id": auditID}), nil
		})
	})
}
