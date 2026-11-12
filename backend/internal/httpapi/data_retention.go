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
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/dataretention"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
)

func retentionResult(out any, err error) (mutation.Result, error) {
	switch {
	case errors.Is(err, dataretention.ErrInvalid), errors.Is(err, dataretention.ErrSchedule):
		return mutation.Fail(400, "DATA_RETENTION_INPUT_INVALID", "归档设置不正确"), nil
	case errors.Is(err, dataretention.ErrVersion):
		return mutation.Fail(409, "DATA_RETENTION_VERSION_CONFLICT", "归档状态已变化，请重新读取"), nil
	case errors.Is(err, dataretention.ErrNotFound):
		return mutation.Fail(404, "DATA_RETENTION_NOT_FOUND", "归档任务不存在"), nil
	case errors.Is(err, identity.ErrSession):
		return mutation.Fail(401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录"), nil
	case errors.Is(err, adminsys.ErrDenied):
		return mutation.Fail(403, "PERMISSION_DENIED", "无归档管理权限"), nil
	case err != nil:
		return mutation.Result{}, err
	default:
		return mutation.OK(200, out), nil
	}
}

type retentionInput struct{ dataretention.PolicyInput }

func (in *retentionInput) UnmarshalJSON(raw []byte) error {
	fields, err := reconciliationClosedObject(raw, map[string]bool{"version": true, "enabled": true, "execution_day": true, "execution_time": true})
	if err != nil || len(fields) != 4 {
		return dataretention.ErrInvalid
	}
	for _, name := range []string{"version", "enabled", "execution_day", "execution_time"} {
		if string(fields[name]) == "null" {
			return dataretention.ErrInvalid
		}
	}
	type body dataretention.PolicyInput
	var next body
	if err = json.Unmarshal(raw, &next); err != nil {
		return dataretention.ErrInvalid
	}
	in.PolicyInput = dataretention.PolicyInput(next)
	return nil
}

func registerDataRetentionRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := dataretention.Service{DB: d.Admins.DB}
	read := func(w http.ResponseWriter, r *http.Request, load func(pgx.Tx) (any, error)) {
		if r.ContentLength != 0 || len(r.TransferEncoding) > 0 {
			failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
			return
		}
		a, ok := platformAccountActor(w, r, d, "platform_settings", "view")
		if !ok {
			return
		}
		tx, err := d.Admins.DB.Begin(r.Context())
		if err != nil {
			outputMutation(w, r, mutation.Result{}, err)
			return
		}
		defer tx.Rollback(r.Context())
		out, err := load(tx)
		if err == nil {
			_, err = audit.Append(r.Context(), tx, audit.Record{ActorType: "admin", ActorID: a.ID, Action: "data_retention.read", ResourceType: "query", RequestID: requestID(r), IP: meta(r).IP})
		}
		if err == nil {
			var fresh access.Account
			fresh, err = freshAdmin(r.Context(), tx, r, d, a, true)
			if err == nil && (!fresh.SuperAdmin || !access.Authorize(fresh, "platform_settings", "view", access.ScopePlatform, "")) {
				err = adminsys.ErrDenied
			}
		}
		if err == nil {
			err = tx.Commit(r.Context())
		}
		result, err := retentionResult(out, err)
		outputMutation(w, r, result, err)
	}
	write := func(w http.ResponseWriter, r *http.Request, operation string, body any, run func(pgx.Tx, access.Account) (any, error)) {
		a, ok := platformAccountActor(w, r, d, "platform_settings", "write")
		if !ok {
			return
		}
		raw, _ := json.Marshal(body)
		var actor access.Account
		result, err := d.Mutations.ExecuteChecked(r.Context(), "", a.ID, operation, r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(r.PathValue("id")+":"+string(raw)), func(ctx context.Context, tx pgx.Tx) error {
			var e error
			actor, e = freshAdmin(ctx, tx, r, d, a, true)
			if e != nil {
				return e
			}
			if r.Header.Get("X-Data-Retention-Actor-ID") != actor.ID {
				return identity.ErrSession
			}
			if !actor.SuperAdmin || !access.Authorize(actor, "platform_settings", "write", access.ScopePlatform, "") {
				return adminsys.ErrDenied
			}
			return nil
		}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			out, e := run(tx, actor)
			return retentionResult(out, e)
		})
		if err != nil {
			result, err = retentionResult(nil, err)
		}
		finishAdminMutation(w, r, d, a, "", operation, result, err)
	}
	handle("GET", "/data-retention-policy", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" || r.URL.ForceQuery {
			failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
			return
		}
		read(w, r, func(tx pgx.Tx) (any, error) { return s.PolicyTx(r.Context(), tx) })
	})
	handle("PUT", "/data-retention-policy", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" || r.URL.ForceQuery {
			failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
			return
		}
		var in retentionInput
		if !decodeBody(w, r, &in) {
			return
		}
		write(w, r, "platform.data_retention.policy.update", in, func(tx pgx.Tx, a access.Account) (any, error) {
			return s.UpdatePolicyTx(r.Context(), tx, in.PolicyInput, pointMeta(r, a))
		})
	})
	handle("GET", "/data-retention-tasks", func(w http.ResponseWriter, r *http.Request) {
		limit, offset := 50, 0
		brand := ""
		values, parseErr := url.ParseQuery(r.URL.RawQuery)
		if parseErr != nil {
			failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
			return
		}
		for key, list := range values {
			if len(list) != 1 || list[0] == "" {
				failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
				return
			}
			switch key {
			case "brand_id":
				brand = list[0]
			case "limit", "offset":
				n, e := strconv.Atoi(list[0])
				if e != nil || strconv.Itoa(n) != list[0] {
					failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
					return
				}
				if key == "limit" {
					limit = n
				} else {
					offset = n
				}
			default:
				failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
				return
			}
		}
		read(w, r, func(tx pgx.Tx) (any, error) { return s.TasksTx(r.Context(), tx, brand, limit, offset) })
	})
	handle("POST", "/data-retention-tasks/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" || r.URL.ForceQuery {
			failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
			return
		}
		var in struct{}
		if !decodeBody(w, r, &in) {
			return
		}
		write(w, r, "platform.data_retention.task.retry", in, func(tx pgx.Tx, a access.Account) (any, error) {
			err := s.RetryTx(r.Context(), tx, r.PathValue("id"), pointMeta(r, a))
			return map[string]string{"id": r.PathValue("id")}, err
		})
	})
}
