package rules

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

// SpecialOnlyPlay identifies rules whose draw inputs depend exclusively on the
// special number. Regular, combined regular/special and decision-table plays
// cannot be projected from a special number alone.
func SpecialOnlyPlay(p ConfiguredPlay) bool {
	seen, valid := false, true
	var metric func(*ConfiguredMetric)
	metric = func(m *ConfiguredMetric) {
		if m == nil {
			return
		}
		if m.Scope != nil {
			seen = true
			valid = valid && m.Scope.Target == "special"
		}
		metric(m.Input)
	}
	var condition func(ConfiguredCondition)
	condition = func(c ConfiguredCondition) {
		metric(c.Left)
		metric(c.Right)
		for _, child := range c.Children {
			condition(child)
		}
	}
	if p.PushWhen != nil {
		condition(*p.PushWhen)
	}
	for _, t := range p.Tiers {
		condition(t.When)
		metric(t.Payout.Count)
		for _, c := range t.Price.Cases {
			condition(c.When)
		}
	}
	return seen && valid && p.Decisions == nil && p.Settlement == "tiers"
}

type SpecialNumberProjection struct {
	Number int
	Won    bool
	Return points.Amount
}

// ProjectSpecialLine keeps the purchased selection and period bindings, while
// preparing its prices again from this engine's current brand price book.
func (e *ConfiguredEngine) ProjectSpecialLine(ctx context.Context, line ConfiguredQuoteLine) ([]SpecialNumberProjection, error) {
	p, ok := e.plays[line.PlayID]
	if !ok {
		return nil, ErrInvalid
	}
	if !SpecialOnlyPlay(p) {
		return nil, nil
	}
	if p.RoundingScope != "line" && p.RoundingScope != "tier" || e.snapshot.Catalog.PeriodPrizeCap != nil || e.snapshot.Catalog.Funding == "reserve" {
		return nil, ErrInvalid
	}
	pool, err := poolValues(e.snapshot.Catalog.Model.SpecialPool)
	if err != nil || e.snapshot.Catalog.Model.SpecialCount != 1 {
		return nil, ErrInvalid
	}
	budget := &configuredBudget{ctx: ctx, remaining: maxTraceNodes}
	options, err := e.selected(p, line.Options)
	if err != nil {
		return nil, err
	}
	current, err := e.prepareLine(p, options, line.Plate, line.Stake, budget)
	if err != nil {
		return nil, err
	}
	out := make([]SpecialNumberProjection, 0, len(pool))
	for _, n := range pool {
		v, err := e.settleLine(current, Draw{Special: []int{n}}, budget)
		if err != nil {
			return nil, err
		}
		out = append(out, SpecialNumberProjection{Number: n, Won: v.Outcome == "won", Return: v.Prize + v.Refund})
	}
	return out, nil
}
