package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
)

func managementAllowed(a access.Account, resource, action, brand string) bool {
	if resource == "user" {
		return access.Authorize(a, resource, action, access.ScopeBrand, brand)
	}
	return adminsys.ManagementAllowed(a, resource, action, brand)
}
func managementActor(w http.ResponseWriter, r *http.Request, d Dependencies, resource, action string) (access.Account, string, bool) {
	a, ok := adminAccount(w, r, d)
	if !ok {
		return a, "", false
	}
	brand := r.Header.Get("X-Brand-ID")
	if !uuidPattern.MatchString(brand) {
		failure(w, r, 400, "REQUEST_INVALID", "必须选择有效品牌")
		return a, brand, false
	}
	if !managementAllowed(a, resource, action, brand) {
		rejectAdmin(w, r, d, a, brand, resource+"."+action)
		return a, brand, false
	}
	return a, brand, true
}
func managementError(err error) (mutation.Result, error) {
	if errors.Is(err, adminsys.ErrConflict) {
		return mutation.Fail(409, "VERSION_CONFLICT", "记录已变化或编号已使用，请刷新后重试"), nil
	}
	if errors.Is(err, adminsys.ErrNotFound) || errors.Is(err, pgx.ErrNoRows) {
		return mutation.Fail(404, "RESOURCE_NOT_FOUND", "管理记录不存在"), nil
	}
	return adminResult("", err)
}
func managementFailure(w http.ResponseWriter, r *http.Request, d Dependencies, a access.Account, brand, permission string, err error) {
	if errors.Is(err, adminsys.ErrDenied) {
		rejectAdmin(w, r, d, a, brand, permission)
		return
	}
	result, e := managementError(err)
	outputMutation(w, r, result, e)
}

type managementAction func(context.Context, pgx.Tx, access.Account) (mutation.Result, error)

func managementWrite(w http.ResponseWriter, r *http.Request, d Dependencies, a access.Account, brand, resource, action, operation, id string, body any, exclusive bool, run managementAction) {
	encoded, err := json.Marshal(body)
	if err != nil {
		failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
		return
	}
	result, err := d.Mutations.Execute(r.Context(), brand, a.ID, operation, r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(id+":"+string(encoded)), func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
		fresh, err := freshAdmin(ctx, tx, r, d, a, exclusive)
		if errors.Is(err, adminsys.ErrDenied) || errors.Is(err, identity.ErrSession) {
			return mutation.Fail(401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录"), nil
		}
		if err != nil {
			return mutation.Result{}, err
		}
		if !managementAllowed(fresh, resource, action, brand) {
			return mutation.Fail(403, "PERMISSION_DENIED", "无操作权限"), nil
		}
		return run(ctx, tx, fresh)
	})
	finishAdminMutation(w, r, d, a, brand, resource+"."+action, result, err)
}

func validRoleIDs(ids []string) bool {
	if len(ids) < 1 || len(ids) > 100 {
		return false
	}
	for _, id := range ids {
		if !uuidPattern.MatchString(id) {
			return false
		}
	}
	return true
}
func managementPassword(ctx context.Context, d Dependencies, password string) (string, mutation.Result, error) {
	if len(password) < 16 || len(password) > 128 {
		return "", mutation.Fail(400, "PASSWORD_INVALID", "管理密码须为 16-128 字节"), nil
	}
	hash, err := d.Identity.PasswordHash(ctx, password)
	if err != nil {
		// Hash-work exhaustion is transient; do not seal it as a final failure.
		return "", mutation.Result{}, err
	}
	return hash, mutation.Result{}, nil
}

func registerAdminManagementRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	for _, kind := range []string{"permissions", "roles", "accounts"} {
		handle("GET", "/"+kind, func(w http.ResponseWriter, r *http.Request) {
			resource := "role"
			if kind == "accounts" {
				resource = "admin"
			}
			a, brand, ok := managementActor(w, r, d, resource, "view")
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
			switch kind {
			case "permissions":
				items, err = d.Admins.ListPermissions(r.Context(), a, brand)
			case "roles":
				items, err = d.Admins.ListRoles(r.Context(), a, brand, limit, offset)
			case "accounts":
				items, err = d.Admins.ListAdmins(r.Context(), a, brand, limit, offset)
			}
			if err != nil {
				managementFailure(w, r, d, a, brand, resource+".view", err)
				return
			}
			if adminReadAudit(w, r, d, a, brand, resource+".view") {
				respond(w, r, 200, map[string]any{"items": items})
			}
		})
	}
	for _, create := range []bool{true, false} {
		method, path := "PATCH", "/roles/{id}"
		if create {
			method, path = "POST", "/roles"
		}
		handle(method, path, func(w http.ResponseWriter, r *http.Request) {
			a, brand, ok := managementActor(w, r, d, "role", "write")
			if !ok {
				return
			}
			id := r.PathValue("id")
			if !create && !uuidPattern.MatchString(id) {
				failure(w, r, 400, "REQUEST_INVALID", "角色编号不正确")
				return
			}
			var in adminsys.RoleInput
			if !decodeBody(w, r, &in) {
				return
			}
			if !create && in.Code != "" {
				failure(w, r, 400, "REQUEST_INVALID", "已有角色编号不能修改")
				return
			}
			managementWrite(w, r, d, a, brand, "role", "write", "admin.role."+method, id, in, true, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
				record, err := d.Admins.WriteRole(ctx, tx, fresh, brand, id, in, meta(r))
				if err != nil {
					return managementError(err)
				}
				status := 200
				if create {
					status = 201
				}
				return mutation.OK(status, record), nil
			})
		})
	}
	handle("POST", "/accounts", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := managementActor(w, r, d, "admin", "write")
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
			failure(w, r, 400, "REQUEST_INVALID", "至少选择一个有效角色")
			return
		}
		managementWrite(w, r, d, a, brand, "admin", "write", "admin.account.create", "", in, true, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			hash, fail, err := managementPassword(ctx, d, in.Password)
			if err != nil || fail.Error != nil {
				return fail, err
			}
			record, err := d.Admins.CreateAdmin(ctx, tx, fresh, brand, adminsys.AdminInput{Username: in.Username, RoleIDs: in.RoleIDs, Status: "active", Reason: in.Reason}, hash, meta(r))
			if err != nil {
				return managementError(err)
			}
			return mutation.OK(201, record), nil
		})
	})
	handle("PATCH", "/accounts/{id}", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := managementActor(w, r, d, "admin", "write")
		if !ok {
			return
		}
		id := r.PathValue("id")
		var in adminsys.AdminInput
		if !decodeBody(w, r, &in) {
			return
		}
		if !uuidPattern.MatchString(id) || !validRoleIDs(in.RoleIDs) || in.Username != "" {
			failure(w, r, 400, "REQUEST_INVALID", "账号、角色编号不正确；账号用户名不可修改")
			return
		}
		managementWrite(w, r, d, a, brand, "admin", "write", "admin.account.update", id, in, true, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			record, err := d.Admins.UpdateAdmin(ctx, tx, fresh, brand, id, in, meta(r))
			if err != nil {
				return managementError(err)
			}
			return mutation.OK(200, record), nil
		})
	})
	handle("POST", "/accounts/{id}/reset-password", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := managementActor(w, r, d, "admin", "write")
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
		managementWrite(w, r, d, a, brand, "admin", "write", "admin.account.password_reset", id, in, true, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			hash, fail, err := managementPassword(ctx, d, in.Password)
			if err != nil || fail.Error != nil {
				return fail, err
			}
			auditID, err := d.Admins.ResetAdminPassword(ctx, tx, fresh, brand, id, in.Version, hash, in.Reason, meta(r))
			if err != nil {
				return managementError(err)
			}
			return mutation.OK(200, map[string]string{"audit_log_id": auditID}), nil
		})
	})
	handle("POST", "/users", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := managementActor(w, r, d, "user", "create")
		if !ok {
			return
		}
		var in identity.OperatorInput
		if !decodeBody(w, r, &in) {
			return
		}
		managementWrite(w, r, d, a, brand, "user", "create", "admin.user.create", "", in, false, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			return d.Identity.OperatorCreate(ctx, tx, brand, fresh.ID, in, meta(r))
		})
	})
	handle("GET", "/auth-settings", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := managementActor(w, r, d, "auth_config", "view")
		if !ok {
			return
		}
		record, err := d.Identity.ReadAuthSettings(r.Context(), brand)
		if err != nil {
			managementFailure(w, r, d, a, brand, "auth_config.view", err)
			return
		}
		if adminReadAudit(w, r, d, a, brand, "auth_config.view") {
			respond(w, r, 200, record)
		}
	})
	handle("PATCH", "/auth-settings", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := managementActor(w, r, d, "auth_config", "write")
		if !ok {
			return
		}
		var in identity.AuthSettingsInput
		if !decodeBody(w, r, &in) {
			return
		}
		in.Reason = strings.TrimSpace(in.Reason)
		managementWrite(w, r, d, a, brand, "auth_config", "write", "admin.auth_config.update", "", in, false, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			return d.Identity.UpdateAuthSettings(ctx, tx, brand, fresh.ID, in, meta(r))
		})
	})
}
