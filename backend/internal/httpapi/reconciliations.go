package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/reconciliation"
	"github.com/jackc/pgx/v5"
)

func reconciliationResult(out any, err error) (mutation.Result, error) {
	switch {
	case errors.Is(err, reconciliation.ErrInvalid):
		return mutation.Fail(400, "RECONCILIATION_INPUT_INVALID", "对账请求参数不正确"), nil
	case errors.Is(err, reconciliation.ErrDenied):
		return mutation.Fail(403, "PERMISSION_DENIED", "无钱包对账权限"), nil
	case errors.Is(err, reconciliation.ErrNotFound):
		return mutation.Fail(404, "RECONCILIATION_NOT_FOUND", "当前品牌未找到该对账任务"), nil
	case errors.Is(err, reconciliation.ErrState):
		return mutation.Fail(409, "RECONCILIATION_STATE_CONFLICT", "当前对账任务或品牌状态不允许此操作"), nil
	case errors.Is(err, reconciliation.ErrVersion):
		return mutation.Fail(409, "RECONCILIATION_VERSION_CONFLICT", "对账任务版本已变化，请重新读取"), nil
	case errors.Is(err, reconciliation.ErrTooLarge):
		return mutation.Fail(413, "RECONCILIATION_TOO_LARGE", "品牌钱包数量超过单次对账上限"), nil
	case err != nil:
		return mutation.Result{}, err
	default:
		return mutation.OK(200, out), nil
	}
}

func reconciliationBrand(w http.ResponseWriter, r *http.Request) (string, bool) {
	brand := strings.ToLower(r.Header.Get("X-Brand-ID"))
	if !uuidPattern.MatchString(brand) {
		failure(w, r, 400, "REQUEST_INVALID", "必须选择有效品牌")
		return "", false
	}
	return brand, true
}

func reconciliationPage(r *http.Request, allowOutcome bool) (limit, offset int, outcome string, ok bool) {
	limit, offset = 20, 0
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return 0, 0, "", false
	}
	for key, values := range query {
		if len(values) != 1 || len(values[0]) == 0 || key != "limit" && key != "offset" && !(allowOutcome && key == "outcome") {
			return 0, 0, "", false
		}
		switch key {
		case "limit":
			limit, err = strconv.Atoi(values[0])
			if err == nil && strconv.Itoa(limit) != values[0] {
				return 0, 0, "", false
			}
		case "offset":
			offset, err = strconv.Atoi(values[0])
			if err == nil && strconv.Itoa(offset) != values[0] {
				return 0, 0, "", false
			}
		case "outcome":
			outcome = values[0]
		}
		if err != nil {
			return 0, 0, "", false
		}
	}
	if limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return 0, 0, "", false
	}
	switch outcome {
	case "", "pending", "failed", "consistent", "repairable", "corrupt":
	default:
		return 0, 0, "", false
	}
	return limit, offset, outcome, true
}

// reconciliationRead keeps authorization, query, audit, and commit on one
// primary transaction. The response is emitted only after the audit commits.
func reconciliationRead(w http.ResponseWriter, r *http.Request, d Dependencies, brand, action string, query func(pgx.Tx) (any, error)) (any, error) {
	ctx := r.Context()
	tx, err := d.Admins.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	id, err := d.Identity.AdminAuthenticateTx(ctx, tx, requestToken(r, administrativeCookie(r)))
	if errors.Is(err, identity.ErrSession) || errors.Is(err, pgx.ErrNoRows) {
		return nil, identity.ErrSession
	}
	if err != nil {
		return nil, err
	}
	initial := access.Account{ID: id}
	fresh, err := freshAdmin(ctx, tx, r, d, initial, false)
	if errors.Is(err, adminsys.ErrDenied) || errors.Is(err, identity.ErrSession) {
		return nil, identity.ErrSession
	}
	if err != nil {
		return nil, err
	}
	if !reconciliation.Allowed(fresh, brand, "view") {
		return nil, reconciliationDenyRead(ctx, tx, r, fresh, brand, "wallet.view.brand")
	}
	out, queryErr := query(tx)
	// Recheck after reading, while retaining the shared access/session locks.
	fresh, err = freshAdmin(ctx, tx, r, d, initial, false)
	if errors.Is(err, adminsys.ErrDenied) || errors.Is(err, identity.ErrSession) {
		return nil, identity.ErrSession
	}
	if err != nil {
		return nil, err
	}
	if !reconciliation.Allowed(fresh, brand, "view") {
		return nil, reconciliationDenyRead(ctx, tx, r, fresh, brand, "wallet.view.brand")
	}
	_, err = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: fresh.ID, Action: action, ResourceType: "query", RequestID: requestID(r), IP: meta(r).IP})
	if err == nil {
		fresh, err = freshAdmin(ctx, tx, r, d, initial, false)
		if errors.Is(err, adminsys.ErrDenied) || errors.Is(err, identity.ErrSession) {
			return nil, identity.ErrSession
		}
		if err == nil && !reconciliation.Allowed(fresh, brand, "view") {
			return nil, reconciliation.ErrDenied
		}
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		return nil, err
	}
	return out, queryErr
}

func reconciliationDenyRead(ctx context.Context, tx pgx.Tx, r *http.Request, actor access.Account, brand, permission string) error {
	_, err := audit.Append(ctx, tx, audit.Record{
		ActorType: "admin", ActorID: actor.ID, Action: "access.denied", ResourceType: "permission",
		Reason: "administrative permission denied", RequestID: requestID(r), IP: meta(r).IP,
		After: map[string]string{"attempted_brand": brand, "permission": permission},
	})
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		return err
	}
	return reconciliation.ErrDenied
}

func reconciliationReadFailure(w http.ResponseWriter, r *http.Request, err error) bool {
	if errors.Is(err, identity.ErrSession) {
		failure(w, r, 401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录")
		return true
	}
	if errors.Is(err, reconciliation.ErrDenied) {
		failure(w, r, 403, "PERMISSION_DENIED", "无钱包对账查询权限")
		return true
	}
	return false
}

func reconciliationWriteCheck(ctx context.Context, tx pgx.Tx, r *http.Request, d Dependencies, initial access.Account, brand, action string, fresh *access.Account) error {
	account, err := freshAdmin(ctx, tx, r, d, initial, false)
	if errors.Is(err, adminsys.ErrDenied) || errors.Is(err, identity.ErrSession) {
		return identity.ErrSession
	}
	if err != nil {
		return err
	}
	*fresh = account
	if !reconciliation.Allowed(account, brand, action) {
		return reconciliation.ErrDenied
	}
	var state string
	err = tx.QueryRow(ctx, `SELECT status FROM brands WHERE id=$1 FOR SHARE`, brand).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return reconciliation.ErrNotFound
	}
	if err != nil {
		return err
	}
	if state == "disabled" {
		return reconciliation.ErrState
	}
	return nil
}

func reconciliationWriteFailure(w http.ResponseWriter, r *http.Request, d Dependencies, initial access.Account, brand, action string, result mutation.Result, err error) {
	if errors.Is(err, identity.ErrSession) {
		result, err = mutation.Fail(401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录"), nil
	} else if errors.Is(err, reconciliation.ErrDenied) || errors.Is(err, reconciliation.ErrNotFound) || errors.Is(err, reconciliation.ErrState) || errors.Is(err, reconciliation.ErrInvalid) || errors.Is(err, reconciliation.ErrVersion) || errors.Is(err, reconciliation.ErrTooLarge) {
		result, err = reconciliationResult(nil, err)
	}
	finishAdminMutation(w, r, d, initial, brand, "wallet.reconcile."+action, result, err)
}

func registerReconciliationRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := reconciliation.Service{DB: d.Admins.DB}
	const collection = "/reconciliations"
	originalHandle := handle
	handle = func(method, path string, handler http.HandlerFunc) {
		originalHandle(method, path, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.ForceQuery {
				failure(w, r, 400, "REQUEST_INVALID", "此接口不接受空查询串")
				return
			}
			if method == "GET" && !reconciliationGetHasNoBody(w, r) {
				return
			}
			handler(w, r)
		})
	}
	handle("GET", collection, func(w http.ResponseWriter, r *http.Request) {
		brand, ok := reconciliationBrand(w, r)
		if !ok {
			return
		}
		limit, offset, _, ok := reconciliationPage(r, false)
		if !ok {
			failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
			return
		}
		out, err := reconciliationRead(w, r, d, brand, "wallet.reconcile.list", func(tx pgx.Tx) (any, error) {
			return s.ListTx(r.Context(), tx, brand, limit, offset)
		})
		if reconciliationReadFailure(w, r, err) {
			return
		}
		result, err := reconciliationResult(out, err)
		outputMutation(w, r, result, err)
	})
	handle("POST", collection, func(w http.ResponseWriter, r *http.Request) {
		brand, ok := reconciliationBrand(w, r)
		if !ok {
			return
		}
		if r.URL.RawQuery != "" {
			failure(w, r, 400, "REQUEST_INVALID", "此写接口不接受查询参数")
			return
		}
		var in reconciliationCreateInput
		if !decodeBody(w, r, &in) {
			return
		}
		raw, err := json.Marshal(in)
		if err != nil {
			failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
			return
		}
		initial, ok := adminAccount(w, r, d)
		if !ok {
			return
		}
		var fresh access.Account
		result, err := d.Mutations.ExecuteChecked(r.Context(), brand, initial.ID, "admin.wallet.reconciliation.create", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(brand+":"+string(raw)), func(ctx context.Context, tx pgx.Tx) error {
			return reconciliationWriteCheck(ctx, tx, r, d, initial, brand, "run", &fresh)
		}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			job, e := s.CreateScoped(ctx, tx, brand, fresh, in.CheckScope, in.Reason, pointMeta(r, fresh))
			if e == nil {
				if verifyErr := reconciliationWriteCheck(ctx, tx, r, d, initial, brand, "run", &fresh); verifyErr != nil {
					return mutation.Result{}, verifyErr
				}
			}
			if errors.Is(e, reconciliation.ErrTooLarge) {
				return mutation.Result{}, e
			}
			response, e := reconciliationResult(job, e)
			if e == nil && response.Error == nil {
				response.Status = 201
			}
			return response, e
		})
		if errors.Is(err, reconciliation.ErrDenied) || errors.Is(err, reconciliation.ErrNotFound) || errors.Is(err, reconciliation.ErrState) {
			result, err = reconciliationResult(nil, err)
		}
		reconciliationWriteFailure(w, r, d, initial, brand, "run", result, err)
	})
	handle("GET", collection+"/{id}", func(w http.ResponseWriter, r *http.Request) {
		brand, ok := reconciliationBrand(w, r)
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		if r.URL.RawQuery != "" {
			failure(w, r, 400, "REQUEST_INVALID", "此接口不接受查询参数")
			return
		}
		out, err := reconciliationRead(w, r, d, brand, "wallet.reconcile.read", func(tx pgx.Tx) (any, error) {
			return s.ReadTx(r.Context(), tx, brand, id)
		})
		if reconciliationReadFailure(w, r, err) {
			return
		}
		result, err := reconciliationResult(out, err)
		outputMutation(w, r, result, err)
	})
	handle("GET", collection+"/{id}/targets", func(w http.ResponseWriter, r *http.Request) {
		brand, ok := reconciliationBrand(w, r)
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		limit, offset, outcome, ok := reconciliationPage(r, true)
		if !ok {
			failure(w, r, 400, "REQUEST_INVALID", "分页或结果筛选参数不正确")
			return
		}
		out, err := reconciliationRead(w, r, d, brand, "wallet.reconcile.targets", func(tx pgx.Tx) (any, error) {
			return s.TargetsTx(r.Context(), tx, brand, id, outcome, limit, offset)
		})
		if reconciliationReadFailure(w, r, err) {
			return
		}
		result, err := reconciliationResult(out, err)
		outputMutation(w, r, result, err)
	})
	handle("POST", collection+"/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		brand, ok := reconciliationBrand(w, r)
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		if r.URL.RawQuery != "" {
			failure(w, r, 400, "REQUEST_INVALID", "此写接口不接受查询参数")
			return
		}
		var in reconciliationRetryInput
		if !decodeBody(w, r, &in) {
			return
		}
		raw, err := json.Marshal(in)
		if err != nil {
			failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
			return
		}
		initial, ok := adminAccount(w, r, d)
		if !ok {
			return
		}
		var fresh access.Account
		result, err := d.Mutations.ExecuteChecked(r.Context(), brand, initial.ID, "admin.wallet.reconciliation.retry", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(id+":"+string(raw)), func(ctx context.Context, tx pgx.Tx) error {
			return reconciliationWriteCheck(ctx, tx, r, d, initial, brand, "retry", &fresh)
		}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			job, e := s.Retry(ctx, tx, brand, id, fresh, in.Version, in.Reason, pointMeta(r, fresh))
			if e == nil {
				if verifyErr := reconciliationWriteCheck(ctx, tx, r, d, initial, brand, "retry", &fresh); verifyErr != nil {
					return mutation.Result{}, verifyErr
				}
			}
			return reconciliationResult(job, e)
		})
		if errors.Is(err, reconciliation.ErrDenied) || errors.Is(err, reconciliation.ErrNotFound) || errors.Is(err, reconciliation.ErrState) {
			result, err = reconciliationResult(nil, err)
		}
		reconciliationWriteFailure(w, r, d, initial, brand, "retry", result, err)
	})
}
