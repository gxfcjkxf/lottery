package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
	"net/http"
)

func registerRuleSimulationRoutes(handle func(string, string, http.HandlerFunc), d Dependencies) {
	handle("POST", "/rule-simulations", func(w http.ResponseWriter, r *http.Request) {
		a, brand, ok := managementActor(w, r, d, "rule", "simulate")
		if !ok {
			return
		}
		var in rules.SimulationInput
		if !decodeBody(w, r, &in) {
			return
		}
		managementWrite(w, r, d, a, brand, "rule", "simulate", "admin.rule.simulate", brand, in, false, func(ctx context.Context, tx pgx.Tx, fresh access.Account) (mutation.Result, error) {
			out, err := rules.SimulateContext(ctx, in)
			if errors.Is(err, rules.ErrInvalid) {
				return mutation.Fail(400, "RULE_INVALID", "玩法定义、选号或开奖结果不符合规则"), nil
			}
			if errors.Is(err, rules.ErrLimit) {
				return mutation.Fail(400, "RULE_EXECUTION_LIMIT", "组合数、倍数、投注额或表达式执行量超出限制"), nil
			}
			if errors.Is(err, points.ErrOverflow) {
				return mutation.Fail(400, "RULE_POINTS_OVERFLOW", "投注或最终派奖超出整数范围"), nil
			}
			if err != nil {
				return mutation.Result{}, err
			}
			raw, _ := json.Marshal(in.Definition)
			sum := sha256.Sum256(raw)
			_, err = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: fresh.ID, Action: "rule.simulate", ResourceType: "rule_simulation", Reason: "operator requested non-financial rule simulation", RequestID: requestID(r), IP: meta(r).IP, Before: map[string]any{"definition_sha256": hex.EncodeToString(sum[:])}, After: map[string]any{"combination_count": out.CombinationCount, "bet_points": out.BetPoints, "prize_points": out.PrizePoints, "won": out.Won}})
			return mutation.OK(200, out), err
		})
	})
}
