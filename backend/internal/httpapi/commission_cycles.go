package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
)

func commissionCycleResult(out any, err error) (mutation.Result, error) {
	switch {
	case err == nil:
		return mutation.OK(200, out), nil
	case errors.Is(err, commission.ErrBusy):
		return mutation.Fail(503, "COMMISSION_CYCLE_BUSY", "佣金周期数据正在变化，请稍后重试"), nil
	case errors.Is(err, commission.ErrInvalid):
		return mutation.Fail(400, "COMMISSION_CYCLE_INPUT_INVALID", "佣金周期请求参数不正确"), nil
	case errors.Is(err, commission.ErrDenied):
		return mutation.Fail(403, "PERMISSION_DENIED", "无佣金周期操作权限"), nil
	case errors.Is(err, commission.ErrNotFound):
		return mutation.Fail(404, "COMMISSION_CYCLE_NOT_FOUND", "当前品牌未找到该佣金周期或锚定订单"), nil
	case errors.Is(err, commission.ErrPolicyEvidence):
		return mutation.Fail(409, "COMMISSION_CYCLE_POLICY_EVIDENCE_CONFLICT", "锚定订单的佣金规则证据不可用"), nil
	case errors.Is(err, commission.ErrCycleState):
		return mutation.Fail(409, "COMMISSION_CYCLE_STATE_CONFLICT", "当前周期或品牌状态不允许此操作"), nil
	case errors.Is(err, commission.ErrCycleVersion):
		return mutation.Fail(409, "COMMISSION_CYCLE_VERSION_CONFLICT", "佣金周期版本已变化，请重新读取"), nil
	case err != nil:
		return mutation.Result{}, err
	default:
		return mutation.OK(200, out), nil
	}
}

func commissionCycleBrand(w http.ResponseWriter, r *http.Request) (string, bool) {
	brand := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Brand-ID")))
	if !uuidPattern.MatchString(brand) {
		failure(w, r, 400, "REQUEST_INVALID", "必须选择有效品牌")
		return "", false
	}
	return brand, true
}

func commissionCyclePage(r *http.Request) (int, int, bool) {
	limit, offset := 20, 0
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return 0, 0, false
	}
	for key, values := range query {
		if len(values) != 1 || values[0] == "" || (key != "limit" && key != "offset") {
			return 0, 0, false
		}
		var value int
		value, err = strconv.Atoi(values[0])
		if err != nil {
			return 0, 0, false
		}
		if key == "limit" {
			limit = value
		} else {
			offset = value
		}
	}
	return limit, offset, limit >= 1 && limit <= 100 && offset >= 0 && offset <= 1_000_000
}

// decodeCommissionCycleBody keeps the exact validated bytes for idempotency
// while applying decodeBody's origin, media type, size, single-value rules.
func decodeCommissionCycleBody(w http.ResponseWriter, r *http.Request, dst any) ([]byte, bool) {
	if !safeOrigin(r) {
		failure(w, r, 403, "CSRF_REJECTED", "请求来源不可信")
		return nil, false
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		failure(w, r, 415, "CONTENT_TYPE_INVALID", "需要 application/json")
		return nil, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
		return nil, false
	}
	if err = json.Unmarshal(raw, dst); err != nil {
		failure(w, r, 400, "REQUEST_INVALID", "请求格式不正确")
		return nil, false
	}
	return raw, true
}

func freshCheckedAuth(ctx context.Context, tx pgx.Tx, r *http.Request, d Dependencies, initial access.Account, brand, action string) (access.Account, error) {
	fresh, err := freshAdmin(ctx, tx, r, d, initial, false)
	if errors.Is(err, adminsys.ErrDenied) || errors.Is(err, identity.ErrSession) {
		return access.Account{}, identity.ErrSession
	}
	if err != nil {
		return access.Account{}, err
	}
	if !commission.AllowedCycle(fresh, brand, action) {
		return fresh, commission.ErrDenied
	}
	return fresh, nil
}

func commissionCycleRead(w http.ResponseWriter, r *http.Request, d Dependencies, initial access.Account, brand, action, auditAction string, query func(pgx.Tx) (any, error), evidence ...map[string]any) (any, error) {
	ctx := r.Context()
	tx, err := d.Admins.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	fresh, err := freshCheckedAuth(ctx, tx, r, d, initial, brand, "view")
	if errors.Is(err, identity.ErrSession) {
		return nil, err
	}
	if errors.Is(err, commission.ErrDenied) {
		return nil, commissionCycleDenyRead(ctx, tx, r, fresh, brand)
	}
	if err != nil {
		return nil, err
	}
	out, queryErr := query(tx)
	if _, err = freshCheckedAuth(ctx, tx, r, d, initial, brand, "view"); errors.Is(err, identity.ErrSession) {
		return nil, err
	} else if errors.Is(err, commission.ErrDenied) {
		return nil, commissionCycleDenyRead(ctx, tx, r, fresh, brand)
	} else if err != nil {
		return nil, err
	}
	var after any
	if len(evidence) > 0 {
		after = evidence[0]
	}
	_, err = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: fresh.ID, Action: auditAction, ResourceType: "query", RequestID: requestID(r), IP: meta(r).IP, After: after})
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		return nil, err
	}
	return out, queryErr
}

func commissionCycleDenyRead(ctx context.Context, tx pgx.Tx, r *http.Request, actor access.Account, brand string) error {
	_, err := audit.Append(ctx, tx, audit.Record{ActorType: "admin", ActorID: actor.ID, Action: "access.denied", ResourceType: "permission", Reason: "administrative permission denied", RequestID: requestID(r), IP: meta(r).IP, After: map[string]string{"attempted_brand": brand, "permission": "commission.view"}})
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		return err
	}
	return commission.ErrDenied
}

func commissionCycleReadFailure(w http.ResponseWriter, r *http.Request, err error) bool {
	if errors.Is(err, identity.ErrSession) {
		failure(w, r, 401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录")
		return true
	}
	if errors.Is(err, commission.ErrDenied) {
		failure(w, r, 403, "PERMISSION_DENIED", "无佣金周期查询权限")
		return true
	}
	if errors.Is(err, commission.ErrBusy) {
		failure(w, r, 503, "COMMISSION_CYCLE_BUSY", "佣金周期数据正在变化，请稍后重试")
		return true
	}
	return false
}

func commissionCycleWriteCheck(ctx context.Context, tx pgx.Tx, r *http.Request, d Dependencies, initial access.Account, brand, action string, fresh *access.Account) error {
	a, err := freshCheckedAuth(ctx, tx, r, d, initial, brand, action)
	if err != nil {
		return err
	}
	*fresh = a
	var state string
	err = tx.QueryRow(ctx, `SELECT status FROM brands WHERE id=$1 FOR SHARE`, brand).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return commission.ErrNotFound
	}
	if err != nil {
		return err
	}
	if state == "disabled" {
		return commission.ErrCycleState
	}
	return nil
}

func commissionCycleWriteFailure(w http.ResponseWriter, r *http.Request, d Dependencies, initial access.Account, brand, action string, result mutation.Result, err error) {
	if errors.Is(err, identity.ErrSession) {
		result, err = mutation.Fail(401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录"), nil
	} else if errors.Is(err, commission.ErrBusy) {
		failure(w, r, 503, "COMMISSION_CYCLE_BUSY", "佣金周期数据正在变化，请稍后重试")
		return
	} else if errors.Is(err, commission.ErrInvalid) || errors.Is(err, commission.ErrDenied) || errors.Is(err, commission.ErrNotFound) || errors.Is(err, commission.ErrPolicyEvidence) || errors.Is(err, commission.ErrCycleState) || errors.Is(err, commission.ErrCycleVersion) {
		result, err = commissionCycleResult(nil, err)
	}
	finishAdminMutation(w, r, d, initial, brand, "commission."+action, result, err)
}

func registerCommissionCycleRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	registerCommissionAllocationRoutes(handle, d)
	s := commission.Service{DB: d.Admins.DB}
	const collection = "/commission-cycles"
	handle("GET", collection, func(w http.ResponseWriter, r *http.Request) {
		brand, ok := commissionCycleBrand(w, r)
		if !ok {
			return
		}
		limit, offset, ok := commissionCyclePage(r)
		if !ok {
			failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
			return
		}
		initial, ok := adminAccount(w, r, d)
		if !ok {
			return
		}
		out, err := commissionCycleRead(w, r, d, initial, brand, "view", "commission.cycle.list", func(tx pgx.Tx) (any, error) {
			return s.CyclesTx(r.Context(), tx, brand, limit, offset)
		})
		if commissionCycleReadFailure(w, r, err) {
			return
		}
		result, err := commissionCycleResult(out, err)
		outputMutation(w, r, result, err)
	})
	handle("POST", collection, func(w http.ResponseWriter, r *http.Request) {
		brand, ok := commissionCycleBrand(w, r)
		if !ok {
			return
		}
		if r.URL.RawQuery != "" {
			failure(w, r, 400, "REQUEST_INVALID", "此写接口不接受查询参数")
			return
		}
		var in commission.CreateCycleInput
		raw, ok := decodeCommissionCycleBody(w, r, &in)
		if !ok {
			return
		}
		initial, ok := adminAccount(w, r, d)
		if !ok {
			return
		}
		var fresh access.Account
		result, err := d.Mutations.ExecuteChecked(r.Context(), brand, initial.ID, "admin.commission_cycle.create", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(brand+":"+string(raw)), func(ctx context.Context, tx pgx.Tx) error {
			return commissionCycleWriteCheck(ctx, tx, r, d, initial, brand, "run", &fresh)
		}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			cycle, err := s.CreateCycleTx(ctx, tx, brand, fresh, in, pointMeta(r, fresh))
			if err != nil {
				return mutation.Result{}, err
			}
			out, err := commissionCycleResult(cycle, nil)
			if err == nil {
				out.Status = 201
			}
			return out, err
		})
		commissionCycleWriteFailure(w, r, d, initial, brand, "cycle.run", result, err)
	})
	handle("GET", collection+"/{id}", func(w http.ResponseWriter, r *http.Request) {
		brand, ok := commissionCycleBrand(w, r)
		if !ok {
			return
		}
		id := r.PathValue("id")
		if !cycleCanonicalID(id) {
			failure(w, r, 400, "REQUEST_INVALID", "佣金周期编号不正确")
			return
		}
		if r.URL.RawQuery != "" {
			failure(w, r, 400, "REQUEST_INVALID", "此接口不接受查询参数")
			return
		}
		initial, ok := adminAccount(w, r, d)
		if !ok {
			return
		}
		out, err := commissionCycleRead(w, r, d, initial, brand, "view", "commission.cycle.read", func(tx pgx.Tx) (any, error) {
			return s.CycleTx(r.Context(), tx, brand, id)
		})
		if commissionCycleReadFailure(w, r, err) {
			return
		}
		result, err := commissionCycleResult(out, err)
		outputMutation(w, r, result, err)
	})
	handle("GET", collection+"/{id}/earnings", func(w http.ResponseWriter, r *http.Request) {
		brand, ok := commissionCycleBrand(w, r)
		if !ok {
			return
		}
		id := r.PathValue("id")
		if !cycleCanonicalID(id) {
			failure(w, r, 400, "REQUEST_INVALID", "佣金周期编号不正确")
			return
		}
		limit, offset, ok := commissionCyclePage(r)
		if !ok {
			failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
			return
		}
		initial, ok := adminAccount(w, r, d)
		if !ok {
			return
		}
		out, err := commissionCycleRead(w, r, d, initial, brand, "view", "commission.cycle.earnings", func(tx pgx.Tx) (any, error) {
			return s.EarningsTx(r.Context(), tx, brand, id, limit, offset)
		})
		if commissionCycleReadFailure(w, r, err) {
			return
		}
		result, err := commissionCycleResult(out, err)
		outputMutation(w, r, result, err)
	})
	handle("GET", collection+"/{id}/runs", func(w http.ResponseWriter, r *http.Request) {
		brand, ok := commissionCycleBrand(w, r)
		if !ok {
			return
		}
		id := r.PathValue("id")
		if !cycleCanonicalID(id) {
			failure(w, r, 400, "REQUEST_INVALID", "佣金周期编号不正确")
			return
		}
		limit, offset, ok := commissionCyclePage(r)
		if !ok {
			failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
			return
		}
		initial, ok := adminAccount(w, r, d)
		if !ok {
			return
		}
		out, err := commissionCycleRead(w, r, d, initial, brand, "view", "commission.cycle.runs", func(tx pgx.Tx) (any, error) {
			return s.RunsTx(r.Context(), tx, brand, id, limit, offset)
		})
		if commissionCycleReadFailure(w, r, err) {
			return
		}
		result, err := commissionCycleResult(out, err)
		outputMutation(w, r, result, err)
	})
	handle("GET", collection+"/{id}/runs/{runID}/calculations", func(w http.ResponseWriter, r *http.Request) {
		brand, ok := commissionCycleBrand(w, r)
		if !ok {
			return
		}
		id, runID := r.PathValue("id"), r.PathValue("runID")
		if !cycleCanonicalID(id) || !cycleCanonicalID(runID) {
			failure(w, r, 400, "REQUEST_INVALID", "佣金周期或运行编号不正确")
			return
		}
		limit, offset, ok := commissionCyclePage(r)
		if !ok {
			failure(w, r, 400, "REQUEST_INVALID", "分页参数不正确")
			return
		}
		initial, ok := adminAccount(w, r, d)
		if !ok {
			return
		}
		out, err := commissionCycleRead(w, r, d, initial, brand, "view", "commission.cycle.calculations", func(tx pgx.Tx) (any, error) {
			return s.CalculationsTx(r.Context(), tx, brand, id, runID, limit, offset)
		})
		if commissionCycleReadFailure(w, r, err) {
			return
		}
		result, err := commissionCycleResult(out, err)
		outputMutation(w, r, result, err)
	})
	handle("POST", collection+"/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		brand, ok := commissionCycleBrand(w, r)
		if !ok {
			return
		}
		id := r.PathValue("id")
		if !cycleCanonicalID(id) {
			failure(w, r, 400, "REQUEST_INVALID", "佣金周期编号不正确")
			return
		}
		if r.URL.RawQuery != "" {
			failure(w, r, 400, "REQUEST_INVALID", "此写接口不接受查询参数")
			return
		}
		var in commission.RetryCycleInput
		raw, ok := decodeCommissionCycleBody(w, r, &in)
		if !ok {
			return
		}
		initial, ok := adminAccount(w, r, d)
		if !ok {
			return
		}
		var fresh access.Account
		result, err := d.Mutations.ExecuteChecked(r.Context(), brand, initial.ID, "admin.commission_cycle.retry", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(id+":"+string(raw)), func(ctx context.Context, tx pgx.Tx) error {
			return commissionCycleWriteCheck(ctx, tx, r, d, initial, brand, "retry", &fresh)
		}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			cycle, err := s.RetryCycleTx(ctx, tx, brand, id, fresh, in, pointMeta(r, fresh))
			if err != nil {
				return mutation.Result{}, err
			}
			return commissionCycleResult(cycle, nil)
		})
		commissionCycleWriteFailure(w, r, d, initial, brand, "cycle.retry", result, err)
	})
}

func cycleCanonicalID(id string) bool {
	return len(id) == 36 && strings.ToLower(id) == id && uuidPattern.MatchString(id)
}
