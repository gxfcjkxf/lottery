package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const commissionPaymentCollection = "/commission-payments"
const commissionPaymentPolicyPath = "/commission-payment-policy"

var errCommissionPaymentActor = errors.New("commission payment actor context changed")

func commissionPaymentResult(out any, err error) (mutation.Result, error) {
	switch {
	case err == nil:
		return mutation.OK(200, out), nil
	case errors.Is(err, commission.ErrBusy):
		return mutation.Fail(503, "COMMISSION_PAYMENT_BUSY", "佣金派发数据正在变化，请稍后重试"), nil
	case errors.Is(err, commission.ErrInvalid):
		return mutation.Fail(400, "COMMISSION_PAYMENT_INPUT_INVALID", "佣金派发请求参数不正确"), nil
	case errors.Is(err, commission.ErrDenied):
		return mutation.Fail(403, "PERMISSION_DENIED", "无佣金派发操作权限"), nil
	case errors.Is(err, commission.ErrNotFound):
		return mutation.Fail(404, "COMMISSION_PAYMENT_NOT_FOUND", "当前品牌未找到该佣金派发记录"), nil
	case errors.Is(err, commission.ErrPaymentEvidence):
		return mutation.Fail(409, "COMMISSION_PAYMENT_EVIDENCE_CONFLICT", "佣金派发证据已变化或不可用"), nil
	case errors.Is(err, commission.ErrPolicyEvidence):
		return mutation.Fail(409, "COMMISSION_PAYMENT_EVIDENCE_CONFLICT", "佣金派发证据已变化或不可用"), nil
	case errors.Is(err, commission.ErrPaymentState):
		return mutation.Fail(409, "COMMISSION_PAYMENT_STATE_CONFLICT", "当前佣金派发状态不允许此操作"), nil
	case errors.Is(err, commission.ErrCycleState):
		return mutation.Fail(409, "COMMISSION_PAYMENT_STATE_CONFLICT", "当前佣金派发状态不允许此操作"), nil
	case errors.Is(err, commission.ErrPaymentVersion):
		return mutation.Fail(409, "COMMISSION_PAYMENT_VERSION_CONFLICT", "佣金派发版本已变化，请重新读取"), nil
	case errors.Is(err, commission.ErrCycleVersion):
		return mutation.Fail(409, "COMMISSION_PAYMENT_VERSION_CONFLICT", "佣金派发版本已变化，请重新读取"), nil
	default:
		return mutation.Result{}, err
	}
}

func freshPaymentAuth(ctx context.Context, tx pgx.Tx, r *http.Request, d Dependencies, initial access.Account, brand, action string) (access.Account, error) {
	fresh, err := freshAdmin(ctx, tx, r, d, initial, false)
	if errors.Is(err, adminsys.ErrDenied) || errors.Is(err, identity.ErrSession) {
		return access.Account{}, identity.ErrSession
	}
	if err != nil {
		return access.Account{}, err
	}
	if !commission.AllowedPayment(fresh, brand, action) {
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
	if brandState == "disabled" {
		return fresh, commission.ErrPaymentState
	}
	return fresh, nil
}

func commissionPaymentWriteCheck(ctx context.Context, tx pgx.Tx, r *http.Request, d Dependencies, initial access.Account, brand, action string, fresh *access.Account) error {
	a, err := freshPaymentAuth(ctx, tx, r, d, initial, brand, action)
	if err != nil {
		return err
	}
	*fresh = a
	actor := r.Header.Get("X-Commission-Payment-Actor-ID")
	if !cycleCanonicalID(actor) {
		return commission.ErrInvalid
	}
	if actor != a.ID {
		return errCommissionPaymentActor
	}
	return nil
}

func commissionPaymentWriteFailure(w http.ResponseWriter, r *http.Request, d Dependencies, initial access.Account, brand, permission string, result mutation.Result, err error) {
	if errors.Is(err, errCommissionPaymentActor) {
		result, err = mutation.Fail(401, "AUTH_ACTOR_CONTEXT_CHANGED", "管理员身份与确认时不同，请重新登录并核对"), nil
	} else if errors.Is(err, identity.ErrSession) {
		result, err = mutation.Fail(401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录"), nil
	} else if errors.Is(err, commission.ErrDenied) {
		result, err = commissionPaymentResult(nil, err)
	} else if errors.Is(err, commission.ErrBusy) || errors.Is(err, commission.ErrInvalid) || errors.Is(err, commission.ErrNotFound) ||
		errors.Is(err, commission.ErrPaymentEvidence) || errors.Is(err, commission.ErrPolicyEvidence) ||
		errors.Is(err, commission.ErrPaymentState) || errors.Is(err, commission.ErrCycleState) ||
		errors.Is(err, commission.ErrPaymentVersion) || errors.Is(err, commission.ErrCycleVersion) {
		result, err = commissionPaymentResult(nil, err)
	}
	finishAdminMutation(w, r, d, initial, brand, permission, result, err)
}

func commissionPaymentReadFailure(w http.ResponseWriter, r *http.Request, err error) bool {
	if errors.Is(err, identity.ErrSession) {
		failure(w, r, 401, "AUTH_SESSION_REVOKED", "管理会话已变化，请重新登录")
		return true
	}
	if errors.Is(err, commission.ErrDenied) {
		failure(w, r, 403, "PERMISSION_DENIED", "无佣金派发查询权限")
		return true
	}
	if errors.Is(err, commission.ErrBusy) {
		failure(w, r, 503, "COMMISSION_PAYMENT_BUSY", "佣金派发数据正在变化，请稍后重试")
		return true
	}
	return false
}

func registerCommissionPaymentRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := commission.Service{DB: d.Admins.DB}

	handle("GET", commissionPaymentPolicyPath, func(w http.ResponseWriter, r *http.Request) {
		brand, ok := commissionCycleBrand(w, r)
		if !ok {
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
		out, err := commissionCycleRead(w, r, d, initial, brand, "view", "commission.payment_policy.read", func(tx pgx.Tx) (any, error) {
			return s.PaymentPolicyTx(r.Context(), tx, brand)
		})
		if commissionPaymentReadFailure(w, r, err) {
			return
		}
		result, err := commissionPaymentResult(out, err)
		outputMutation(w, r, result, err)
	})

	handle("PUT", commissionPaymentPolicyPath, func(w http.ResponseWriter, r *http.Request) {
		brand, ok := commissionCycleBrand(w, r)
		if !ok {
			return
		}
		if r.URL.RawQuery != "" {
			failure(w, r, 400, "REQUEST_INVALID", "此写接口不接受查询参数")
			return
		}
		var in commission.PaymentPolicyInput
		raw, ok := decodeCommissionCycleBody(w, r, &in)
		if !ok {
			return
		}
		initial, ok := adminAccount(w, r, d)
		if !ok {
			return
		}
		var fresh access.Account
		result, err := d.Mutations.ExecuteChecked(r.Context(), brand, initial.ID, "admin.commission_payment_policy.update", r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(brand+":"+string(raw)), func(ctx context.Context, tx pgx.Tx) error {
			return commissionPaymentWriteCheck(ctx, tx, r, d, initial, brand, "policy_write", &fresh)
		}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			policy, err := s.UpdatePaymentPolicyTx(ctx, tx, brand, fresh, in, pointMeta(r, fresh))
			if err != nil {
				return mutation.Result{}, err
			}
			return commissionPaymentResult(policy, nil)
		})
		commissionPaymentWriteFailure(w, r, d, initial, brand, "commission_payment_policy.write", result, err)
	})

	handle("GET", commissionPaymentCollection, func(w http.ResponseWriter, r *http.Request) {
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
		out, err := commissionCycleRead(w, r, d, initial, brand, "view", "commission.payment.list", func(tx pgx.Tx) (any, error) {
			return s.PaymentsTx(r.Context(), tx, brand, limit, offset)
		})
		if commissionPaymentReadFailure(w, r, err) {
			return
		}
		result, err := commissionPaymentResult(out, err)
		outputMutation(w, r, result, err)
	})

	handle("GET", commissionPaymentCollection+"/{id}", func(w http.ResponseWriter, r *http.Request) {
		brand, ok := commissionCycleBrand(w, r)
		if !ok {
			return
		}
		id := r.PathValue("id")
		if !cycleCanonicalID(id) || r.URL.RawQuery != "" {
			failure(w, r, 400, "REQUEST_INVALID", "派发编号或查询参数不正确")
			return
		}
		initial, ok := adminAccount(w, r, d)
		if !ok {
			return
		}
		out, err := commissionCycleRead(w, r, d, initial, brand, "view", "commission.payment.read", func(tx pgx.Tx) (any, error) {
			return s.PaymentTx(r.Context(), tx, brand, id)
		})
		if commissionPaymentReadFailure(w, r, err) {
			return
		}
		result, err := commissionPaymentResult(out, err)
		outputMutation(w, r, result, err)
	})

	for _, action := range []string{"approve", "retry"} {
		action := action
		handle("POST", commissionPaymentCollection+"/{id}/"+action, func(w http.ResponseWriter, r *http.Request) {
			brand, ok := commissionCycleBrand(w, r)
			if !ok {
				return
			}
			id := r.PathValue("id")
			if !cycleCanonicalID(id) {
				failure(w, r, 400, "REQUEST_INVALID", "派发编号不正确")
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
			operation := "admin.commission_payment." + action
			result, err := d.Mutations.ExecuteChecked(r.Context(), brand, initial.ID, operation, r.Header.Get("Idempotency-Key"), d.Mutations.Fingerprint(brand+":"+id+":"+string(raw)), func(ctx context.Context, tx pgx.Tx) error {
				return commissionPaymentWriteCheck(ctx, tx, r, d, initial, brand, action, &fresh)
			}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
				var payment commission.Payment
				var err error
				if action == "approve" {
					payment, err = s.ApprovePaymentTx(ctx, tx, brand, id, fresh, in, pointMeta(r, fresh))
				} else {
					payment, err = s.RetryPaymentTx(ctx, tx, brand, id, fresh, in, pointMeta(r, fresh))
				}
				if err != nil {
					return mutation.Result{}, err
				}
				return commissionPaymentResult(payment, nil)
			})
			commissionPaymentWriteFailure(w, r, d, initial, brand, "commission_payment."+action, result, err)
		})
	}
}
