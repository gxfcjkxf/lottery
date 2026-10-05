package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"github.com/gxfcjkxf/lottery/backend/internal/schedule"
	"github.com/jackc/pgx/v5"
)

func periodResult(status int, out any, err error) (mutation.Result, error) {
	if errors.Is(err, rulebook.ErrDenied) {
		return mutation.Fail(403, "PERMISSION_DENIED", "需要品牌排期或期次权限；平台账号不能执行品牌写操作"), nil
	}
	if errors.Is(err, rulebook.ErrVersion) {
		return mutation.Fail(409, "SCHEDULE_VERSION_CONFLICT", "彩种版本已变化，请重新读取后保存"), nil
	}
	if errors.Is(err, rulebook.ErrState) {
		return mutation.Fail(409, "PERIOD_STATE_CONFLICT", "已有同编号但时间窗口不同的期次，不允许覆盖历史"), nil
	}
	if errors.Is(err, schedule.ErrInvalid) || errors.Is(err, schedule.ErrLimit) {
		return mutation.Fail(400, "SCHEDULE_INVALID", "日历参数、生成范围或数量超出限制"), nil
	}
	if errors.Is(err, rulebook.ErrInvalid) {
		return mutation.Fail(400, "SCHEDULE_INVALID", "日历、时区或生成参数不正确"), nil
	}
	return rulebookResult(status, out, err)
}
func registerPeriodRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := rulebook.Store{DB: d.Admins.DB}
	handle("GET", "/games/{id}/schedule", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := managementActor(w, r, d, "schedule", "view")
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		out, e := s.Schedule(r.Context(), b, id)
		if e != nil {
			result, e := periodResult(200, nil, e)
			outputMutation(w, r, result, e)
			return
		}
		if adminReadAudit(w, r, d, a, b, "schedule.view") {
			respond(w, r, 200, out)
		}
	})
	handle("PUT", "/games/{id}/schedule", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := managementActor(w, r, d, "schedule", "write")
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		var in struct {
			Version int64         `json:"version"`
			Spec    schedule.Spec `json:"spec"`
			Reason  string        `json:"reason"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		managementWrite(w, r, d, a, b, "schedule", "write", "admin.schedule.write", id, in, false, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, e := s.WriteSchedule(ctx, tx, b, fresh, id, in.Version, in.Spec, in.Reason, pointMeta(r, fresh))
			return periodResult(200, out, e)
		})
	})
	handle("GET", "/games/{id}/periods", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := managementActor(w, r, d, "period", "view")
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		limit, offset, ok := pageParams(r)
		if !ok {
			failure(w, r, 400, "REQUEST_INVALID", "分页不正确")
			return
		}
		out, e := s.Periods(r.Context(), b, id, limit, offset)
		if e != nil {
			result, e := periodResult(200, nil, e)
			outputMutation(w, r, result, e)
			return
		}
		if adminReadAudit(w, r, d, a, b, "period.view") {
			respond(w, r, 200, map[string]any{"periods": out, "limit": limit, "offset": offset})
		}
	})
	handle("POST", "/games/{id}/periods/generate", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := managementActor(w, r, d, "period", "generate")
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		var in struct {
			From   time.Time `json:"from"`
			To     time.Time `json:"to"`
			Reason string    `json:"reason"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		managementWrite(w, r, d, a, b, "period", "generate", "admin.period.generate", id, in, false, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, e := s.GeneratePeriods(ctx, tx, b, fresh, id, in.From, in.To, in.Reason, pointMeta(r, fresh))
			return periodResult(200, out, e)
		})
	})
}
