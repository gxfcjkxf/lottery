package rules

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"testing"
)

func TestConfiguredCanadaTemplatesMatchEveryHistoricalDraw(t *testing.T) {
	for _, profile := range CanadaProfiles {
		t.Run(profile, func(t *testing.T) {
			c, err := ConfiguredTemplateCatalog(profile)
			if err != nil {
				t.Fatal(err)
			}
			book := map[string]string{}
			for _, p := range c.Plays {
				for _, v := range p.Decisions.Outcomes {
					if v.PriceKey != "" {
						book[v.PriceKey] = "2.5"
					}
				}
			}
			e, err := NewConfiguredEngine(c, ConfiguredBindings{Version: "explicit-golden-prices", Attributes: map[string]map[string][]int{}}, ConfiguredPrices{Version: "explicit-golden-prices", Plates: map[string]map[string]string{"A": book}})
			if err != nil {
				t.Fatal(err)
			}
			for _, total := range []int64{0, 10000} {
				for _, code := range CanadaOptions(profile) {
					q, err := e.Prepare(context.Background(), []ConfiguredRequest{{PlayID: code, Options: []string{code}, Plate: "A", Points: 10, Multiplier: 1}})
					if err != nil {
						t.Fatal(err)
					}
					for n := 0; n < 1000; n++ {
						d := Draw{Digits: []int{n / 100, n / 10 % 10, n % 10}}
						cap := points.Amount(888888)
						r, err := e.SettleWithContext(context.Background(), q, d, ConfiguredSettlementContext{Aggregates: map[string]points.Amount{"non_tail": points.Amount(total)}, PrizeRemaining: &cap})
						if err != nil {
							t.Fatalf("%s/%d: %v", code, n, err)
						}
						var outcome string
						var effective bool
						var prize, refund points.Amount
						if profile == "canada_niuniu" {
							var win, loss int64
							outcome, win, loss, err = CanadaNiu(code, d)
							effective = outcome != "push"
							refund = 10 * points.Amount(CanadaRisk(profile, code)-loss)
							if win > 0 {
								prize, _, err = CanadaNiuProfit(10, "2.5", win)
							}
						} else {
							outcome, _, effective, err = CanadaOutcome(profile, code, d, total)
							amount, e2 := MarkSixPayout(10, "2.5", outcome)
							if err == nil {
								err = e2
							}
							if outcome == "push" {
								refund = amount
							} else {
								prize = amount
							}
						}
						if err != nil {
							t.Fatal(err)
						}
						wantEffective := points.Amount(0)
						if effective {
							wantEffective = 10
						}
						if r.Lines[0].Outcome != outcome || r.PrizePoints != prize || r.RefundPoints != refund || r.EffectivePoints != wantEffective {
							t.Fatalf("%s/%d/%d: got %+v want %s prize %d refund %d effective %d", code, n, total, r, outcome, prize, refund, wantEffective)
						}
					}
				}
			}
		})
	}
}
