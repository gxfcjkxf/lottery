package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
)

func cancellationResult(out any, err error) (mutation.Result, error) {
	switch {
	case errors.Is(err, betting.ErrInvalid):
		return mutation.Fail(400, "PERIOD_CANCEL_INPUT_INVALID", "期次取消参数或原因不正确"), nil
	case errors.Is(err, betting.ErrNotFound):
		return mutation.Fail(404, "PERIOD_CANCEL_NOT_FOUND", "当前品牌中不存在该期次或任务"), nil
	case errors.Is(err, betting.ErrVersion):
		return mutation.Fail(409, "PERIOD_CANCEL_VERSION_CONFLICT", "期次或退款任务版本已变化，请重新读取并确认"), nil
	case errors.Is(err, betting.ErrState):
		return mutation.Fail(409, "PERIOD_CANCEL_STATE_CONFLICT", "当前状态或开奖结果不允许此操作；结算中的期次必须另走回溯流程"), nil
	}
	return betError(out, err)
}
func registerPeriodCancellationRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := betService(d)
	handle("GET", "/periods/{id}/cancellation", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := betAdminActor(w, r, d, "period", "view", false)
		if !ok {
			return
		}
		out, err := s.PeriodCancellation(r.Context(), brand, r.PathValue("id"))
		if err == nil && !adminReadAudit(w, r, d, a, brand, "period.cancellation.view") {
			return
		}
		result, e := cancellationResult(map[string]any{"cancellation": out}, err)
		outputMutation(w, r, result, e)
	})
	handle("POST", "/periods/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := betAdminActor(w, r, d, "period", "cancel", true)
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		var in betting.CancelPeriodInput
		if !decodeBody(w, r, &in) {
			return
		}
		betAdminWrite(w, r, d, a, brand, "period", "cancel", "admin.period.cancel", id, in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, err := s.CancelPeriod(ctx, tx, brand, fresh, id, in, bettingMeta(r, fresh))
			result, e := cancellationResult(out, err)
			if err == nil && out.State == "processing" {
				result.Status = 202
			}
			return result, e
		})
	})
	handle("POST", "/periods/{id}/cancellation/retry", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := betAdminActor(w, r, d, "period", "cancel_retry", true)
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		var in struct {
			Version int64  `json:"version"`
			Reason  string `json:"reason"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		betAdminWrite(w, r, d, a, brand, "period", "cancel_retry", "admin.period.cancel_retry", id, in, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, err := s.RetryPeriodCancellation(ctx, tx, brand, fresh, id, in.Version, in.Reason, bettingMeta(r, fresh))
			result, e := cancellationResult(out, err)
			if err == nil {
				result.Status = 202
			}
			return result, e
		})
	})
}
