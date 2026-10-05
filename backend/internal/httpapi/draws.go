package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/drawfeed"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

// drawAPIResult keeps draw-specific errors distinct from rule workflow errors.
func drawAPIResult(status int, out any, err error) (mutation.Result, error) {
	switch {
	case errors.Is(err, rulebook.ErrDenied):
		return mutation.Fail(403, "PERMISSION_DENIED", "无操作权限"), nil
	case errors.Is(err, rulebook.ErrDrawAbnormal):
		return mutation.Fail(409, "DRAW_ABNORMAL", "开奖结果与上一期相同"), nil
	case errors.Is(err, rulebook.ErrVersion):
		return mutation.Fail(409, "DRAW_VERSION_CONFLICT", "版本已变化，请重新读取后保存"), nil
	case errors.Is(err, rulebook.ErrState):
		return mutation.Fail(409, "DRAW_STATE_CONFLICT", "当前期次状态不允许此操作"), nil
	case errors.Is(err, rulebook.ErrNotFound):
		return mutation.Fail(404, "DRAW_NOT_FOUND", "彩种、期次或开奖结果不存在"), nil
	case errors.Is(err, rulebook.ErrInvalid), errors.Is(err, drawfeed.ErrInvalid), errors.Is(err, drawfeed.ErrLimit), errors.Is(err, rules.ErrInvalid):
		return mutation.Fail(400, "DRAW_INVALID", "开奖数据、来源配置或操作参数不正确"), nil
	case err != nil:
		return mutation.Result{}, err
	default:
		return mutation.OK(status, out), nil
	}
}

// RegisterDrawRoutes is called by the admin route registry.
func RegisterDrawRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := rulebook.Store{DB: d.Admins.DB}
	handle("GET", "/games/{id}/draw-sources", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := managementActor(w, r, d, "draw_source", "view")
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		out, err := s.Sources(r.Context(), brand, id)
		if err != nil {
			result, e := drawAPIResult(200, nil, err)
			outputMutation(w, r, result, e)
			return
		}
		if adminReadAudit(w, r, d, a, brand, "draw_source.view") {
			respond(w, r, 200, out)
		}
	})
	handle("PUT", "/games/{id}/draw-sources", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := managementActor(w, r, d, "draw_source", "write")
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		var in struct {
			Version int64                   `json:"version"`
			Sources []drawfeed.SourceConfig `json:"sources"`
			Reason  string                  `json:"reason"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		managementWrite(w, r, d, a, brand, "draw_source", "write", "admin.draw_source.write", id, in, false, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			if !rulebook.Allowed(fresh, brand, "draw_source", "write") {
				return mutation.Fail(403, "PERMISSION_DENIED", "无操作权限"), nil
			}
			out, err := s.WriteSources(ctx, tx, brand, fresh, id, in.Version, in.Sources, in.Reason, pointMeta(r, fresh))
			return drawAPIResult(200, out, err)
		})
	})
	handle("GET", "/periods/{id}/draw", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := managementActor(w, r, d, "draw", "view")
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
		out, err := s.Draw(r.Context(), brand, id, limit, offset)
		if err != nil {
			result, e := drawAPIResult(200, nil, err)
			outputMutation(w, r, result, e)
			return
		}
		if adminReadAudit(w, r, d, a, brand, "draw.view") {
			respond(w, r, 200, out)
		}
	})
	handle("POST", "/periods/{id}/manual-draw", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := managementActor(w, r, d, "draw", "manual_create")
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		var in struct {
			Version  int64      `json:"version"`
			PeriodNo string     `json:"period_no"`
			Result   rules.Draw `json:"result"`
			DrawnAt  time.Time  `json:"drawn_at"`
			Reason   string     `json:"reason"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		managementWrite(w, r, d, a, brand, "draw", "manual_create", "admin.draw.manual_create", id, in, false, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			if !rulebook.Allowed(fresh, brand, "draw", "manual_create") {
				return mutation.Fail(403, "PERMISSION_DENIED", "无操作权限"), nil
			}
			if in.Version < 1 || in.PeriodNo == "" || in.DrawnAt.IsZero() {
				return drawAPIResult(201, nil, rulebook.ErrInvalid)
			}
			out, err := s.ManualDraw(ctx, tx, brand, fresh, id, in.Version, in.PeriodNo, in.Result, in.DrawnAt, in.Reason, pointMeta(r, fresh))
			return drawAPIResult(201, out, err)
		})
	})
}
