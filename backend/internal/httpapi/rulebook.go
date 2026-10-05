package httpapi

import (
	"context"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
	"net/http"
)

func rulebookResult(status int, out any, err error) (mutation.Result, error) {
	switch {
	case errors.Is(err, rulebook.ErrDenied):
		return mutation.Fail(403, "PERMISSION_DENIED", "创建者或编辑者不能审核；平台账号不能修改品牌玩法"), nil
	case errors.Is(err, rulebook.ErrInvalid) || errors.Is(err, rules.ErrInvalid):
		return mutation.Fail(400, "RULE_INVALID", "规则配置、样例或操作参数不正确"), nil
	case errors.Is(err, rules.ErrLimit):
		return mutation.Fail(400, "RULE_EXECUTION_LIMIT", "规则验证执行量超出限制"), nil
	case errors.Is(err, rulebook.ErrVersion):
		return mutation.Fail(409, "RULE_VERSION_CONFLICT", "版本或编号已变化，请刷新后重试"), nil
	case errors.Is(err, rulebook.ErrState):
		return mutation.Fail(409, "RULE_STATE_CONFLICT", "当前状态不允许此操作或已有待生效版本"), nil
	case errors.Is(err, rulebook.ErrValidation):
		return mutation.Fail(409, "RULE_VALIDATION_REQUIRED", "需要有效验证报告或确认审核警告"), nil
	case errors.Is(err, rulebook.ErrNotFound):
		return mutation.Fail(404, "RESOURCE_NOT_FOUND", "彩种、玩法或规则版本不存在"), nil
	case errors.Is(err, points.ErrOverflow):
		return mutation.Fail(400, "RULE_POINTS_OVERFLOW", "规则验证积分溢出"), nil
	case err != nil:
		return mutation.Result{}, err
	default:
		return mutation.OK(status, out), nil
	}
}
func ruleRead(w http.ResponseWriter, r *http.Request, d Dependencies, a access.Account, brand string, out any, err error) {
	if err != nil {
		result, e := rulebookResult(200, nil, err)
		outputMutation(w, r, result, e)
		return
	}
	if !adminReadAudit(w, r, d, a, brand, "rule.workflow.view") {
		return
	}
	respond(w, r, 200, out)
}
func ruleID(w http.ResponseWriter, r *http.Request, key string) (string, bool) {
	id := r.PathValue(key)
	if !uuidPattern.MatchString(id) {
		failure(w, r, 400, "REQUEST_INVALID", "记录编号不正确")
		return id, false
	}
	return id, true
}
func registerRuleBookRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	s := rulebook.Store{DB: d.Admins.DB}
	handle("GET", "/games", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := managementActor(w, r, d, "game", "view")
		if !ok {
			return
		}
		limit, offset, ok := pageParams(r)
		if !ok {
			failure(w, r, 400, "REQUEST_INVALID", "分页不正确")
			return
		}
		out, e := s.Games(r.Context(), b, limit, offset)
		ruleRead(w, r, d, a, b, map[string]any{"games": out, "limit": limit, "offset": offset}, e)
	})
	handle("POST", "/games", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := managementActor(w, r, d, "game", "write")
		if !ok {
			return
		}
		var in struct {
			Code     string      `json:"code"`
			Name     string      `json:"name"`
			Model    rules.Model `json:"model"`
			Timezone string      `json:"timezone"`
			Reason   string      `json:"reason"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		managementWrite(w, r, d, a, b, "game", "write", "admin.game.create", b, in, false, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, e := s.CreateGame(ctx, tx, b, fresh, in.Code, in.Name, in.Model, in.Timezone, in.Reason, pointMeta(r, fresh))
			return rulebookResult(201, out, e)
		})
	})
	handle("GET", "/games/{id}/plays", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := managementActor(w, r, d, "game", "view")
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
		out, e := s.Plays(r.Context(), b, id, limit, offset)
		ruleRead(w, r, d, a, b, map[string]any{"plays": out, "limit": limit, "offset": offset}, e)
	})
	handle("POST", "/games/{id}/plays", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := managementActor(w, r, d, "game", "write")
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		var in struct {
			Code   string `json:"code"`
			Name   string `json:"name"`
			Reason string `json:"reason"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		managementWrite(w, r, d, a, b, "game", "write", "admin.play.create", id, in, false, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, e := s.CreatePlay(ctx, tx, b, fresh, id, in.Code, in.Name, in.Reason, pointMeta(r, fresh))
			return rulebookResult(201, out, e)
		})
	})
	handle("GET", "/plays/{id}/rule-versions", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := managementActor(w, r, d, "rule", "view")
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
		out, e := s.Versions(r.Context(), b, id, limit, offset)
		ruleRead(w, r, d, a, b, map[string]any{"versions": out, "limit": limit, "offset": offset}, e)
	})
	handle("GET", "/rule-versions/{id}", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := managementActor(w, r, d, "rule", "view")
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		out, e := s.GetVersion(r.Context(), b, id)
		ruleRead(w, r, d, a, b, out, e)
	})
	handle("POST", "/rule-versions", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := managementActor(w, r, d, "rule", "write")
		if !ok {
			return
		}
		var in struct {
			PlayID     string           `json:"play_id"`
			Definition rules.Definition `json:"definition"`
			EffectMode string           `json:"effect_mode"`
			Reason     string           `json:"reason"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		managementWrite(w, r, d, a, b, "rule", "write", "admin.rule.draft.create", in.PlayID, in, false, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, e := s.CreateVersion(ctx, tx, b, fresh, in.PlayID, in.Definition, in.EffectMode, in.Reason, pointMeta(r, fresh))
			return rulebookResult(201, out, e)
		})
	})
	handle("PUT", "/rule-versions/{id}", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := managementActor(w, r, d, "rule", "write")
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		var in struct {
			Version    int64            `json:"version"`
			Definition rules.Definition `json:"definition"`
			EffectMode string           `json:"effect_mode"`
			Reason     string           `json:"reason"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		managementWrite(w, r, d, a, b, "rule", "write", "admin.rule.draft.update", id, in, false, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, e := s.UpdateVersion(ctx, tx, b, fresh, id, in.Version, in.Definition, in.EffectMode, in.Reason, pointMeta(r, fresh))
			return rulebookResult(200, out, e)
		})
	})
	handle("POST", "/rule-versions/{id}/validate", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := managementActor(w, r, d, "rule", "validate")
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		var in struct {
			Version int64                  `json:"version"`
			Cases   []rules.ValidationCase `json:"cases"`
			Reason  string                 `json:"reason"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		managementWrite(w, r, d, a, b, "rule", "validate", "admin.rule.draft.validate", id, in, false, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, e := s.Validate(ctx, tx, b, fresh, id, in.Version, in.Cases, in.Reason, pointMeta(r, fresh))
			return rulebookResult(200, out, e)
		})
	})
	handle("POST", "/rule-versions/{id}/submit-review", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := managementActor(w, r, d, "rule", "submit")
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
		managementWrite(w, r, d, a, b, "rule", "submit", "admin.rule.review.submit", id, in, false, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, e := s.Submit(ctx, tx, b, fresh, id, in.Version, in.Reason, pointMeta(r, fresh))
			return rulebookResult(200, out, e)
		})
	})
	for _, decision := range []string{"approve", "reject"} {
		handle("POST", "/rule-versions/{id}/"+decision, func(w http.ResponseWriter, r *http.Request) {
			a, b, ok := managementActor(w, r, d, "rule", "review")
			if !ok {
				return
			}
			id, ok := ruleID(w, r, "id")
			if !ok {
				return
			}
			var in struct {
				Version              int64  `json:"version"`
				Reason               string `json:"reason"`
				WarningsAcknowledged bool   `json:"warnings_acknowledged"`
			}
			if !decodeBody(w, r, &in) {
				return
			}
			managementWrite(w, r, d, a, b, "rule", "review", "admin.rule.review."+decision, id, in, false, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
				out, e := s.Review(ctx, tx, b, fresh, id, in.Version, decision == "approve", in.WarningsAcknowledged, in.Reason, pointMeta(r, fresh))
				return rulebookResult(200, out, e)
			})
		})
	}
	handle("POST", "/rule-versions/{id}/clone", func(w http.ResponseWriter, r *http.Request) {
		a, b, ok := managementActor(w, r, d, "rule", "write")
		if !ok {
			return
		}
		id, ok := ruleID(w, r, "id")
		if !ok {
			return
		}
		var in struct {
			EffectMode string `json:"effect_mode"`
			Reason     string `json:"reason"`
		}
		if !decodeBody(w, r, &in) {
			return
		}
		managementWrite(w, r, d, a, b, "rule", "write", "admin.rule.rollback.draft", id, in, false, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, e := s.Clone(ctx, tx, b, fresh, id, in.EffectMode, in.Reason, pointMeta(r, fresh))
			return rulebookResult(201, out, e)
		})
	})
}
