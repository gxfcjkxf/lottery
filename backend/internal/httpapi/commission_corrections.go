package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"unicode"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const correctionPlansPath = "/commission-correction-plans"
const correctionExecutionsPath = "/commission-correction-executions"
const correctionPolicyPath = "/commission-correction-policy"

var errCommissionCorrectionActor = errors.New("commission correction actor context changed")

func correctionHTTPDBError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "55P03" || pgErr.Code == "40P01" || pgErr.Code == "40001") {
		return commission.ErrBusy
	}
	return err
}

func correctionHTTPResult(out any, err error) (mutation.Result, error) {
	switch {
	case err == nil:
		return mutation.OK(200, out), nil
	case errors.Is(err, identity.ErrSession):
		return mutation.Fail(401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录"), nil
	case errors.Is(err, errCommissionCorrectionActor):
		return mutation.Fail(401, "AUTH_ACTOR_CONTEXT_CHANGED", "管理员身份与确认时不同，请重新核对"), nil
	case errors.Is(err, commission.ErrDenied):
		return mutation.Fail(403, "PERMISSION_DENIED", "无佣金更正操作权限"), nil
	case errors.Is(err, commission.ErrInvalid):
		return mutation.Fail(400, "COMMISSION_CORRECTION_INPUT_INVALID", "佣金更正参数不正确"), nil
	case errors.Is(err, commission.ErrNotFound):
		return mutation.Fail(404, "COMMISSION_CORRECTION_NOT_FOUND", "当前品牌未找到该更正记录"), nil
	case errors.Is(err, commission.ErrCorrectionPlanVersion), errors.Is(err, commission.ErrCorrectionExecutionVersion), errors.Is(err, commission.ErrCycleVersion):
		return mutation.Fail(409, "COMMISSION_CORRECTION_VERSION_CONFLICT", "版本已变化，请重新读取"), nil
	case errors.Is(err, commission.ErrCorrectionPlanState), errors.Is(err, commission.ErrCorrectionExecutionState), errors.Is(err, commission.ErrCycleState), errors.Is(err, commission.ErrPaymentState):
		return mutation.Fail(409, "COMMISSION_CORRECTION_STATE_CONFLICT", "当前更正或品牌状态不允许此操作"), nil
	case errors.Is(err, commission.ErrCorrectionPlanEvidence), errors.Is(err, commission.ErrCorrectionExecutionEvidence), errors.Is(err, commission.ErrPolicyEvidence):
		return mutation.Fail(409, "COMMISSION_CORRECTION_EVIDENCE_CONFLICT", "当前更正证据已变化或不可用"), nil
	case errors.Is(err, commission.ErrCorrectionPlanLimit):
		return mutation.Fail(409, "COMMISSION_CORRECTION_LIMIT", "更正目标超出处理上限"), nil
	default:
		return mutation.Result{}, err
	}
}

func correctionHTTPOutput(w http.ResponseWriter, r *http.Request, out any, err error) {
	err = correctionHTTPDBError(err)
	if errors.Is(err, commission.ErrBusy) {
		failure(w, r, 503, "COMMISSION_CORRECTION_BUSY", "佣金更正数据正在变化，请稍后重试")
		return
	}
	result, err := correctionHTTPResult(out, err)
	outputMutation(w, r, result, err)
}

func correctionHTTPAuth(ctx context.Context, tx pgx.Tx, r *http.Request, d Dependencies, initial access.Account, brand, action string) (access.Account, error) {
	a, err := freshAdmin(ctx, tx, r, d, initial, false)
	if errors.Is(err, adminsys.ErrDenied) || errors.Is(err, identity.ErrSession) {
		return access.Account{}, identity.ErrSession
	}
	if err != nil {
		return a, correctionHTTPDBError(err)
	}
	if action != "view" {
		actor := r.Header.Get("X-Commission-Correction-Actor-ID")
		if !cycleCanonicalID(actor) {
			return a, commission.ErrInvalid
		}
		if actor != a.ID {
			return a, errCommissionCorrectionActor
		}
	}
	allowed := commission.AllowedCorrectionExecution(a, brand, action)
	if action == "plan_retry" {
		allowed = commission.AllowedCorrectionPlan(a, brand, "retry")
	}
	if !allowed {
		return a, commission.ErrDenied
	}
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM brands WHERE id=$1 FOR SHARE NOWAIT`, brand).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, commission.ErrNotFound
	}
	if err != nil {
		return a, correctionHTTPDBError(err)
	}
	if action != "view" && status == "disabled" {
		return a, commission.ErrCorrectionExecutionState
	}
	return a, nil
}

func correctionHTTPRead(ctx context.Context, r *http.Request, d Dependencies, initial access.Account, brand, action string, query func(pgx.Tx) (any, error)) (any, error) {
	tx, err := d.Admins.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	a, err := correctionHTTPAuth(ctx, tx, r, d, initial, brand, "view")
	if errors.Is(err, commission.ErrDenied) {
		return nil, commissionCycleDenyRead(ctx, tx, r, a, brand)
	}
	if err != nil {
		return nil, err
	}
	out, queryErr := query(tx)
	_, err = correctionHTTPAuth(ctx, tx, r, d, initial, brand, "view")
	if errors.Is(err, commission.ErrDenied) {
		return nil, commissionCycleDenyRead(ctx, tx, r, a, brand)
	}
	if err != nil {
		return nil, err
	}
	_, err = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: a.ID, Action: action, ResourceType: "query", RequestID: requestID(r), IP: meta(r).IP})
	// Audit insertion may wait until a naturally expiring session is no longer
	// valid. Never release the snapshot or its query audit after that boundary.
	if err == nil {
		_, err = correctionHTTPAuth(ctx, tx, r, d, initial, brand, "view")
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		return nil, correctionHTTPDBError(err)
	}
	return out, queryErr
}

func correctionHTTPGetHasNoBody(w http.ResponseWriter, r *http.Request) bool {
	if len(r.TransferEncoding) != 0 || r.ContentLength > 0 {
		failure(w, r, 400, "COMMISSION_CORRECTION_INPUT_INVALID", "更正查询不接受正文")
		return false
	}
	if r.Body == nil || r.Body == http.NoBody {
		return true
	}
	probe, err := io.ReadAll(io.LimitReader(r.Body, 1))
	if len(probe) != 0 || err != nil && !errors.Is(err, io.EOF) {
		failure(w, r, 400, "COMMISSION_CORRECTION_INPUT_INVALID", "更正查询不接受正文")
		return false
	}
	return true
}

func decodeCorrectionHTTPBody(w http.ResponseWriter, r *http.Request, dst any) ([]byte, bool) {
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
	if err != nil || !utf8.Valid(raw) || json.Unmarshal(raw, dst) != nil {
		failure(w, r, 400, "COMMISSION_CORRECTION_INPUT_INVALID", "更正请求格式不正确")
		return nil, false
	}
	var reason string
	switch in := dst.(type) {
	case *commission.PaymentPolicyInput:
		reason = in.Reason
	case *commission.RetryCycleInput:
		reason = in.Reason
	}
	for _, c := range reason {
		if unicode.IsControl(c) {
			failure(w, r, 400, "COMMISSION_CORRECTION_INPUT_INVALID", "原因包含不允许的字符")
			return nil, false
		}
	}
	return raw, true
}

func correctionHTTPWrite(w http.ResponseWriter, r *http.Request, d Dependencies, initial access.Account, brand, id, action, operation, permission string, raw []byte, run func(context.Context, pgx.Tx, access.Account) (any, error)) {
	var fresh access.Account
	check := func(ctx context.Context, tx pgx.Tx) error {
		a, err := correctionHTTPAuth(ctx, tx, r, d, initial, brand, action)
		if err == nil {
			fresh = a
		}
		return err
	}
	result, err := d.Mutations.ExecuteChecked(r.Context(), brand, initial.ID, operation, r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(brand+":"+id+":"+string(raw)), check,
		func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			out, err := run(ctx, tx, fresh)
			if err = correctionHTTPDBError(err); errors.Is(err, commission.ErrBusy) {
				return mutation.Result{}, err
			}
			result, err := correctionHTTPResult(out, err)
			if err != nil {
				return mutation.Result{}, err
			}
			if err = check(ctx, tx); err != nil {
				return mutation.Result{}, err
			}
			return result, nil
		})
	err = correctionHTTPDBError(err)
	if errors.Is(err, commission.ErrBusy) {
		failure(w, r, 503, "COMMISSION_CORRECTION_BUSY", "佣金更正数据正在变化，请稍后重试")
		return
	}
	if err != nil {
		result, err = correctionHTTPResult(nil, err)
	}
	finishAdminMutation(w, r, d, initial, brand, permission, result, err)
}

func registerCommissionCorrectionRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := commission.Service{DB: d.Admins.DB}
	// Each read uses one brand-scoped primary transaction and a committed query
	// audit. Targets expose financial witnesses, never wallet or rule snapshots.
	reads := []struct {
		path, action string
		paged        bool
		query        func(context.Context, pgx.Tx, string, string, int, int) (any, error)
	}{
		{correctionPolicyPath, "commission.correction_policy.read", false, func(ctx context.Context, tx pgx.Tx, b, id string, l, o int) (any, error) {
			return s.CorrectionExecutionPolicyTx(ctx, tx, b)
		}},
		{correctionPlansPath, "commission.correction_plan.list", true, func(ctx context.Context, tx pgx.Tx, b, id string, l, o int) (any, error) {
			return s.CorrectionPlansTx(ctx, tx, b, l, o)
		}},
		{correctionPlansPath + "/{id}", "commission.correction_plan.read", false, func(ctx context.Context, tx pgx.Tx, b, id string, l, o int) (any, error) {
			return s.CorrectionPlanTx(ctx, tx, b, id)
		}},
		{correctionPlansPath + "/{id}/targets", "commission.correction_plan.targets", true, func(ctx context.Context, tx pgx.Tx, b, id string, l, o int) (any, error) {
			return s.CorrectionPlanTargetsTx(ctx, tx, b, id, l, o)
		}},
		{correctionExecutionsPath, "commission.correction_execution.list", true, func(ctx context.Context, tx pgx.Tx, b, id string, l, o int) (any, error) {
			return s.CorrectionExecutionsTx(ctx, tx, b, l, o)
		}},
		{correctionExecutionsPath + "/{id}", "commission.correction_execution.read", false, func(ctx context.Context, tx pgx.Tx, b, id string, l, o int) (any, error) {
			return s.CorrectionExecutionTx(ctx, tx, b, id)
		}},
		{correctionExecutionsPath + "/{id}/targets", "commission.correction_execution.targets", true, func(ctx context.Context, tx pgx.Tx, b, id string, l, o int) (any, error) {
			return s.CorrectionExecutionTargetsTx(ctx, tx, b, id, l, o)
		}},
	}
	for _, entry := range reads {
		entry := entry
		handle("GET", entry.path, func(w http.ResponseWriter, r *http.Request) {
			if !correctionHTTPGetHasNoBody(w, r) {
				return
			}
			brand, ok := rewardBrand(w, r)
			if !ok {
				return
			}
			id := r.PathValue("id")
			if id != "" && !cycleCanonicalID(id) {
				failure(w, r, 400, "COMMISSION_CORRECTION_INPUT_INVALID", "更正编号不正确")
				return
			}
			l, o := 20, 0
			if entry.paged {
				l, o, ok = rewardPage(r)
			} else {
				ok = r.URL.RawQuery == "" && !r.URL.ForceQuery
			}
			if !ok {
				failure(w, r, 400, "COMMISSION_CORRECTION_INPUT_INVALID", "更正查询参数不正确")
				return
			}
			a, ok := adminAccount(w, r, d)
			if !ok {
				return
			}
			out, err := correctionHTTPRead(r.Context(), r, d, a, brand, entry.action, func(tx pgx.Tx) (any, error) { return entry.query(r.Context(), tx, brand, id, l, o) })
			correctionHTTPOutput(w, r, out, err)
		})
	}
	handle("PUT", correctionPolicyPath, func(w http.ResponseWriter, r *http.Request) {
		brand, ok := rewardBrand(w, r)
		if !ok {
			return
		}
		if r.URL.RawQuery != "" || r.URL.ForceQuery {
			failure(w, r, 400, "COMMISSION_CORRECTION_INPUT_INVALID", "写入不接受查询参数")
			return
		}
		var in commission.PaymentPolicyInput
		raw, ok := decodeCorrectionHTTPBody(w, r, &in)
		if !ok {
			return
		}
		a, ok := adminAccount(w, r, d)
		if !ok {
			return
		}
		correctionHTTPWrite(w, r, d, a, brand, "", "policy_write", "admin.commission_correction_policy.update", "commission_correction_policy.write", raw,
			func(ctx context.Context, tx pgx.Tx, a access.Account) (any, error) {
				return s.UpdateCorrectionExecutionPolicyTx(ctx, tx, brand, a, in, pointMeta(r, a))
			})
	})
	for _, entry := range []struct{ collection, action, permission string }{
		{correctionPlansPath, "plan_retry", "commission_correction.retry"},
		{correctionExecutionsPath, "approve", "commission_correction.approve"},
		{correctionExecutionsPath, "continue", "commission_correction.continue"},
		{correctionExecutionsPath, "retry", "commission_correction.execute_retry"},
	} {
		entry := entry
		suffix := entry.action
		if suffix == "plan_retry" {
			suffix = "retry"
		}
		handle("POST", entry.collection+"/{id}/"+suffix, func(w http.ResponseWriter, r *http.Request) {
			brand, ok := rewardBrand(w, r)
			if !ok {
				return
			}
			id := r.PathValue("id")
			if !cycleCanonicalID(id) || r.URL.RawQuery != "" || r.URL.ForceQuery {
				failure(w, r, 400, "COMMISSION_CORRECTION_INPUT_INVALID", "更正编号或查询参数不正确")
				return
			}
			var in commission.RetryCycleInput
			raw, ok := decodeCorrectionHTTPBody(w, r, &in)
			if !ok {
				return
			}
			a, ok := adminAccount(w, r, d)
			if !ok {
				return
			}
			operation := "admin.commission_correction_execution." + entry.action
			if entry.action == "plan_retry" {
				operation = "admin.commission_correction_plan.retry"
			}
			correctionHTTPWrite(w, r, d, a, brand, id, entry.action, operation, entry.permission, raw, func(ctx context.Context, tx pgx.Tx, a access.Account) (any, error) {
				meta := pointMeta(r, a)
				switch entry.action {
				case "plan_retry":
					return s.RetryCorrectionPlanTx(ctx, tx, brand, id, a, in, meta)
				case "approve":
					return s.ApproveCorrectionExecutionTx(ctx, tx, brand, id, a, in, meta)
				case "continue":
					return s.ContinueCorrectionExecutionTx(ctx, tx, brand, id, a, in, meta)
				default:
					return s.RetryCorrectionExecutionTx(ctx, tx, brand, id, a, in, meta)
				}
			})
		})
	}
}
