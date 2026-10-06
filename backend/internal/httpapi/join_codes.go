package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/attribution"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
)

func joinCodeResult(out any, err error) (mutation.Result, error) {
	switch {
	case err == nil:
		return mutation.OK(200, out), nil
	case errors.Is(err, attribution.ErrInvalid):
		return mutation.Fail(400, "JOIN_CODE_INPUT_INVALID", "加入码请求或筛选参数不正确"), nil
	case errors.Is(err, attribution.ErrNotFound):
		return mutation.Fail(404, "JOIN_CODE_NOT_FOUND", "当前品牌未找到对应加入码"), nil
	case errors.Is(err, attribution.ErrDenied):
		return mutation.Fail(403, "JOIN_CODE_DENIED", "无权执行此加入码操作"), nil
	case errors.Is(err, attribution.ErrVersion):
		return mutation.Fail(409, "JOIN_CODE_VERSION_CONFLICT", "加入码版本已变化，请重新读取并核对"), nil
	case errors.Is(err, attribution.ErrState), errors.Is(err, attribution.ErrUnavailable):
		return mutation.Fail(409, "JOIN_CODE_STATE_CONFLICT", "加入码状态冲突或编码已被占用"), nil
	default:
		return mutation.Result{}, err
	}
}

func joinCodePage(r *http.Request, allowed ...string) (int, int, map[string]string, bool) {
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return 0, 0, nil, false
	}
	keys := make(map[string]bool, len(allowed)+2)
	for _, key := range allowed {
		keys[key] = true
	}
	keys["limit"], keys["offset"] = true, true
	values := make(map[string]string, len(q))
	for key, list := range q {
		if !keys[key] || len(list) != 1 || list[0] == "" {
			return 0, 0, nil, false
		}
		values[key] = list[0]
	}
	limit, offset := 20, 0
	if v, ok := values["limit"]; ok {
		limit, err = strconv.Atoi(v)
		if err != nil {
			return 0, 0, nil, false
		}
	}
	if v, ok := values["offset"]; ok {
		offset, err = strconv.Atoi(v)
		if err != nil {
			return 0, 0, nil, false
		}
	}
	if limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return 0, 0, nil, false
	}
	return limit, offset, values, true
}

func joinCodeWrite(w http.ResponseWriter, r *http.Request, d Dependencies, initial access.Account, brand, operation, resourceID string, body any, run func(context.Context, pgx.Tx, access.Account) (mutation.Result, error)) {
	encoded, err := json.Marshal(body)
	if err != nil {
		failure(w, r, 400, "JOIN_CODE_INPUT_INVALID", "加入码请求格式不正确")
		return
	}
	s := attribution.Service{DB: d.Admins.DB}
	var fresh access.Account
	result, err := d.Mutations.ExecuteChecked(r.Context(), brand, initial.ID, operation, r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(resourceID+":"+string(encoded)), func(ctx context.Context, tx pgx.Tx) error {
		var e error
		fresh, e = freshAdmin(ctx, tx, r, d, initial, false)
		if errors.Is(e, adminsys.ErrDenied) || errors.Is(e, identity.ErrSession) {
			return identity.ErrSession
		}
		if e != nil {
			return e
		}
		if fresh.SuperAdmin || !managementAllowed(fresh, "join_code", "write", brand) {
			return attribution.ErrDenied
		}
		return s.Lock(ctx, tx, brand)
	}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
		return run(ctx, tx, fresh)
	})
	if errors.Is(err, identity.ErrSession) {
		result, err = mutation.Fail(401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录"), nil
	} else if errors.Is(err, attribution.ErrDenied) {
		result, err = mutation.Fail(403, "JOIN_CODE_DENIED", "无加入码写权限"), nil
	}
	finishAdminMutation(w, r, d, initial, brand, "join_code.write", result, err)
}

func registerJoinCodeAdmin(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := attribution.Service{DB: d.Admins.DB}
	handle("GET", "/join-codes", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := pointAdminActor(w, r, d, "join_code", "view", false)
		if !ok {
			return
		}
		limit, offset, query, ok := joinCodePage(r, "kind", "owner_member_id")
		if !ok {
			failure(w, r, 400, "JOIN_CODE_INPUT_INVALID", "加入码筛选或分页参数不正确")
			return
		}
		var kind, owner *string
		if value, exists := query["kind"]; exists {
			kind = &value
		}
		if value, exists := query["owner_member_id"]; exists {
			owner = &value
		}
		out, err := s.List(r.Context(), brand, kind, owner, limit, offset)
		if err == nil && !adminReadAudit(w, r, d, a, brand, "join_code.list.view") {
			return
		}
		result, e := joinCodeResult(out, err)
		outputMutation(w, r, result, e)
	})
	handle("POST", "/join-codes", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := pointAdminActor(w, r, d, "join_code", "write", true)
		if !ok {
			return
		}
		var in attribution.CreateInput
		if !decodeBody(w, r, &in) {
			return
		}
		joinCodeWrite(w, r, d, a, brand, "admin.join_code.create", "", in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, err := s.Create(ctx, tx, brand, fresh, in, pointMeta(r, fresh))
			result, e := joinCodeResult(out, err)
			if e == nil && result.Status == 200 {
				result.Status = 201
			}
			return result, e
		})
	})
	handle("GET", "/join-codes/{id}", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := pointAdminActor(w, r, d, "join_code", "view", false)
		if !ok {
			return
		}
		id := r.PathValue("id")
		if !uuidPattern.MatchString(id) {
			failure(w, r, 400, "JOIN_CODE_INPUT_INVALID", "加入码编号不正确")
			return
		}
		if len(r.URL.RawQuery) != 0 {
			failure(w, r, 400, "JOIN_CODE_INPUT_INVALID", "此接口不接受查询参数")
			return
		}
		out, err := s.Get(r.Context(), brand, id)
		if err == nil && !adminReadAudit(w, r, d, a, brand, "join_code.view") {
			return
		}
		result, e := joinCodeResult(out, err)
		outputMutation(w, r, result, e)
	})
	handle("PUT", "/join-codes/{id}", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := pointAdminActor(w, r, d, "join_code", "write", true)
		if !ok {
			return
		}
		id := r.PathValue("id")
		if !uuidPattern.MatchString(id) {
			failure(w, r, 400, "JOIN_CODE_INPUT_INVALID", "加入码编号不正确")
			return
		}
		var in attribution.UpdateInput
		if !decodeBody(w, r, &in) {
			return
		}
		joinCodeWrite(w, r, d, a, brand, "admin.join_code.update", id, in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, err := s.Update(ctx, tx, brand, fresh, id, in, pointMeta(r, fresh))
			return joinCodeResult(out, err)
		})
	})
	handle("GET", "/join-codes/{id}/history", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := pointAdminActor(w, r, d, "join_code", "view", false)
		if !ok {
			return
		}
		id := r.PathValue("id")
		if !uuidPattern.MatchString(id) {
			failure(w, r, 400, "JOIN_CODE_INPUT_INVALID", "加入码编号不正确")
			return
		}
		limit, offset, _, ok := joinCodePage(r)
		if !ok {
			failure(w, r, 400, "JOIN_CODE_INPUT_INVALID", "分页参数不正确")
			return
		}
		out, err := s.History(r.Context(), brand, id, limit, offset)
		if err == nil && !adminReadAudit(w, r, d, a, brand, "join_code.history.view") {
			return
		}
		result, e := joinCodeResult(out, err)
		outputMutation(w, r, result, e)
	})
}

func registerJoinCodeUser(mux *http.ServeMux, d Dependencies) {
	readDB := d.Admins.DB
	if readDB == nil && d.Identity != nil {
		readDB = d.Identity.DB
	}
	s := attribution.Service{DB: readDB}
	for _, prefix := range []string{"/api/v1", "/api/v1/b/{brandCode}"} {
		mux.HandleFunc("GET "+prefix+"/me/join-codes", func(w http.ResponseWriter, r *http.Request) {
			if d.Identity == nil || readDB == nil {
				failure(w, r, 503, "JOIN_CODE_UNAVAILABLE", "加入码服务尚未配置")
				return
			}
			brand, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			session, ok := currentBetSession(w, r, d, brand.ID)
			if !ok {
				return
			}
			limit, offset, _, ok := joinCodePage(r)
			if !ok {
				failure(w, r, 400, "JOIN_CODE_INPUT_INVALID", "分页参数不正确")
				return
			}
			out, err := s.SelfCodes(r.Context(), brand.ID, session.View.Member.ID, limit, offset)
			result, e := joinCodeResult(out, err)
			outputMutation(w, r, result, e)
		})
		mux.HandleFunc("GET "+prefix+"/me/attribution", func(w http.ResponseWriter, r *http.Request) {
			if d.Identity == nil || readDB == nil {
				failure(w, r, 503, "JOIN_CODE_UNAVAILABLE", "归属服务尚未配置")
				return
			}
			if len(r.URL.RawQuery) != 0 {
				failure(w, r, 400, "JOIN_CODE_INPUT_INVALID", "此接口不接受查询参数")
				return
			}
			brand, ok := resolveBrand(w, r, d)
			if !ok {
				return
			}
			session, ok := currentBetSession(w, r, d, brand.ID)
			if !ok {
				return
			}
			out, err := s.Attribution(r.Context(), brand.ID, session.View.Member.ID)
			result, e := joinCodeResult(out, err)
			outputMutation(w, r, result, e)
		})
	}
}
