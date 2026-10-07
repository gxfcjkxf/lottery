package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var errCommissionAdjustmentActor = errors.New("commission adjustment actor context changed")

func commissionAdjustmentResult(out any, err error) (mutation.Result, error) {
	switch {
	case err == nil:
		return mutation.OK(200, out), nil
	case errors.Is(err, commission.ErrInvalid):
		return mutation.Fail(400, "COMMISSION_ADJUSTMENT_INPUT_INVALID", "佣金修正请求参数不正确"), nil
	case errors.Is(err, commission.ErrDenied):
		return mutation.Fail(403, "PERMISSION_DENIED", "无佣金人工修正权限"), nil
	case errors.Is(err, commission.ErrNotFound):
		return mutation.Fail(404, "COMMISSION_ADJUSTMENT_NOT_FOUND", "当前品牌未找到该佣金派发目标"), nil
	case errors.Is(err, commission.ErrAdjustmentState):
		return mutation.Fail(409, "COMMISSION_ADJUSTMENT_STATE_CONFLICT", "目标状态或佣金证据不允许修正"), nil
	case errors.Is(err, commission.ErrPaymentEvidence), errors.Is(err, commission.ErrPaymentState):
		return mutation.Fail(409, "COMMISSION_ADJUSTMENT_STATE_CONFLICT", "目标状态或佣金证据不允许修正"), nil
	case errors.Is(err, commission.ErrAdjustmentVersion):
		return mutation.Fail(409, "COMMISSION_ADJUSTMENT_VERSION_CONFLICT", "修正版本已变化，请重新读取"), nil
	case errors.Is(err, commission.ErrAdjustmentInsufficient):
		return mutation.Fail(409, "COMMISSION_ADJUSTMENT_INSUFFICIENT", "佣金可用积分不足，无法完成修正"), nil
	case errors.Is(err, points.ErrPolicyLimit):
		return mutation.Fail(409, "COMMISSION_ADJUSTMENT_POLICY_LIMIT", "修正后的积分超出当前积分规则限制"), nil
	default:
		return mutation.Result{}, err
	}
}

func freshAdjustmentAuth(ctx context.Context, tx pgx.Tx, r *http.Request, d Dependencies, initial access.Account, brand, action string) (access.Account, error) {
	fresh, err := freshAdmin(ctx, tx, r, d, initial, false)
	if errors.Is(err, adminsys.ErrDenied) || errors.Is(err, identity.ErrSession) {
		return access.Account{}, identity.ErrSession
	}
	if err != nil {
		return access.Account{}, err
	}
	if !commission.AllowedAdjustment(fresh, brand, action) {
		return fresh, commission.ErrDenied
	}
	var brandState string
	err = tx.QueryRow(ctx, `SELECT status FROM brands WHERE id=$1 FOR SHARE NOWAIT`, brand).Scan(&brandState)
	if errors.Is(err, pgx.ErrNoRows) {
		return fresh, commission.ErrNotFound
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "55P03" || pgErr.Code == "40P01" || pgErr.Code == "40001") {
			return fresh, commission.ErrBusy
		}
		return fresh, err
	}
	if action == "write" && brandState == "disabled" {
		return fresh, commission.ErrAdjustmentState
	}
	return fresh, nil
}

func commissionAdjustmentWriteCheck(ctx context.Context, tx pgx.Tx, r *http.Request, d Dependencies, initial access.Account, brand string, fresh *access.Account) error {
	a, err := freshAdjustmentAuth(ctx, tx, r, d, initial, brand, "write")
	if err != nil {
		return err
	}
	*fresh = a
	actor := r.Header.Get("X-Commission-Payment-Actor-ID")
	if !cycleCanonicalID(actor) || actor != a.ID {
		return errCommissionAdjustmentActor
	}
	return nil
}

func commissionAdjustmentRead(w http.ResponseWriter, r *http.Request, d Dependencies, initial access.Account, brand, auditAction string, query func(pgx.Tx) (any, error)) (any, error) {
	ctx := r.Context()
	tx, err := d.Admins.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	fresh, err := freshAdjustmentAuth(ctx, tx, r, d, initial, brand, "view")
	if errors.Is(err, identity.ErrSession) {
		return nil, err
	}
	if errors.Is(err, commission.ErrDenied) {
		return nil, commissionAdjustmentDenyRead(ctx, tx, r, fresh, brand)
	}
	if err != nil {
		return nil, err
	}
	out, queryErr := query(tx)
	if _, err = freshAdjustmentAuth(ctx, tx, r, d, initial, brand, "view"); errors.Is(err, identity.ErrSession) {
		return nil, err
	} else if errors.Is(err, commission.ErrDenied) {
		return nil, commissionAdjustmentDenyRead(ctx, tx, r, fresh, brand)
	} else if err != nil {
		return nil, err
	}
	_, err = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: fresh.ID, Action: auditAction, ResourceType: "query", RequestID: requestID(r), IP: meta(r).IP})
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		return nil, err
	}
	return out, queryErr
}

func commissionAdjustmentDenyRead(ctx context.Context, tx pgx.Tx, r *http.Request, actor access.Account, brand string) error {
	_, err := audit.Append(ctx, tx, audit.Record{ActorType: "admin", ActorID: actor.ID, Action: "access.denied", ResourceType: "permission", Reason: "administrative permission denied", RequestID: requestID(r), IP: meta(r).IP, After: map[string]string{"attempted_brand": brand, "permission": "commission.view"}})
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		return err
	}
	return commission.ErrDenied
}

func commissionAdjustmentReadFailure(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case errors.Is(err, identity.ErrSession):
		failure(w, r, 401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录")
	case errors.Is(err, commission.ErrDenied):
		failure(w, r, 403, "PERMISSION_DENIED", "无佣金人工修正查询权限")
	case errors.Is(err, commission.ErrBusy):
		failure(w, r, 503, "COMMISSION_ADJUSTMENT_BUSY", "佣金修正数据正在变化，请稍后重试")
	default:
		return false
	}
	return true
}

func commissionAdjustmentWriteFailure(w http.ResponseWriter, r *http.Request, d Dependencies, initial access.Account, brand string, result mutation.Result, err error) {
	switch {
	case errors.Is(err, errCommissionAdjustmentActor):
		result, err = mutation.Fail(401, "AUTH_ACTOR_CONTEXT_CHANGED", "管理员身份与确认时不同，请重新登录并核对"), nil
	case errors.Is(err, identity.ErrSession):
		result, err = mutation.Fail(401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录"), nil
	case errors.Is(err, commission.ErrBusy):
		failure(w, r, 503, "COMMISSION_ADJUSTMENT_BUSY", "佣金修正数据正在变化，请稍后重试")
		return
	case errors.Is(err, commission.ErrInvalid), errors.Is(err, commission.ErrDenied), errors.Is(err, commission.ErrNotFound),
		errors.Is(err, commission.ErrAdjustmentState), errors.Is(err, commission.ErrPaymentEvidence), errors.Is(err, commission.ErrPaymentState), errors.Is(err, commission.ErrAdjustmentVersion),
		errors.Is(err, commission.ErrAdjustmentInsufficient), errors.Is(err, points.ErrPolicyLimit):
		result, err = commissionAdjustmentResult(nil, err)
	}
	finishAdminMutation(w, r, d, initial, brand, "commission_adjustment.write", result, err)
}

func registerCommissionAdjustmentRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := commission.Service{DB: d.Admins.DB}
	const targetHistoryPath = "/commission-payment-targets/{id}/adjustments"
	handle("GET", "/commission-payments/{id}/targets", func(w http.ResponseWriter, r *http.Request) {
		brand, ok := commissionCycleBrand(w, r)
		if !ok {
			return
		}
		payment := r.PathValue("id")
		if !cycleCanonicalID(payment) {
			failure(w, r, 400, "REQUEST_INVALID", "派发编号不正确")
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
		out, err := commissionAdjustmentRead(w, r, d, initial, brand, "commission.payment_targets.list", func(tx pgx.Tx) (any, error) {
			return s.PaymentTargetsTx(r.Context(), tx, brand, payment, limit, offset)
		})
		if commissionAdjustmentReadFailure(w, r, err) {
			return
		}
		result, err := commissionAdjustmentResult(out, err)
		outputMutation(w, r, result, err)
	})

	handle("GET", targetHistoryPath, func(w http.ResponseWriter, r *http.Request) {
		brand, ok := commissionCycleBrand(w, r)
		if !ok {
			return
		}
		target := r.PathValue("id")
		if !cycleCanonicalID(target) {
			failure(w, r, 400, "REQUEST_INVALID", "目标编号不正确")
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
		out, err := commissionAdjustmentRead(w, r, d, initial, brand, "commission.adjustments.list", func(tx pgx.Tx) (any, error) {
			return s.AdjustmentsTx(r.Context(), tx, brand, target, limit, offset)
		})
		if commissionAdjustmentReadFailure(w, r, err) {
			return
		}
		result, err := commissionAdjustmentResult(out, err)
		outputMutation(w, r, result, err)
	})

	handle("POST", targetHistoryPath, func(w http.ResponseWriter, r *http.Request) {
		brand, ok := commissionCycleBrand(w, r)
		if !ok {
			return
		}
		target := r.PathValue("id")
		if !cycleCanonicalID(target) {
			failure(w, r, 400, "REQUEST_INVALID", "目标编号不正确")
			return
		}
		if r.URL.RawQuery != "" {
			failure(w, r, 400, "REQUEST_INVALID", "此写接口不接受查询参数")
			return
		}
		var in commission.AdjustmentInput
		raw, ok := decodeCommissionCycleBody(w, r, &in)
		if !ok {
			return
		}
		initial, ok := adminAccount(w, r, d)
		if !ok {
			return
		}
		var fresh access.Account
		result, err := d.Mutations.ExecuteChecked(r.Context(), brand, initial.ID, "admin.commission_adjustment.create", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(brand+":"+target+":"+string(raw)), func(ctx context.Context, tx pgx.Tx) error {
			return commissionAdjustmentWriteCheck(ctx, tx, r, d, initial, brand, &fresh)
		}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			adjustment, err := s.AdjustCommissionTx(ctx, tx, brand, target, fresh, in, pointMeta(r, fresh))
			if errors.Is(err, commission.ErrBusy) {
				return mutation.Result{}, err
			}
			mapped, err := commissionAdjustmentResult(adjustment, err)
			if err != nil {
				return mutation.Result{}, err
			}
			if mapped.Error == nil {
				mapped.Status = 201
			}
			return mapped, nil
		})
		commissionAdjustmentWriteFailure(w, r, d, initial, brand, result, err)
	})
}
