package commission

import (
	"encoding/json"
)

// OrderAllocation is the exact, per-agent result for one finalized order.
// It does not authorize recording or paying the amount.
type OrderAllocation struct {
	AgentID  string      `json:"agent_id"`
	MemberID string      `json:"member_id"`
	Exact    ExactAmount `json:"exact"`
}

// EvaluateOrder derives an order's unrounded allocations solely from the
// policy and attribution saved with that bet and its finalized settlement
// evidence. Callers still need transaction-bound policy and final-order
// witnesses before treating these results as financial authorization.
func EvaluateOrder(snapshot BetSnapshot, fact OrderFact) ([]OrderAllocation, error) {
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return nil, ErrPolicyEvidence
	}
	validated, err := ParseBetSnapshot(raw, fact.BrandID, fact.MemberID, fact.PlacedAt)
	if err != nil {
		return nil, err
	}

	derived, err := Bases(fact.Status, fact.Basis.TurnoverPoints, fact.PrizePoints)
	if err != nil || derived != fact.Basis {
		return nil, ErrEvidence
	}

	if !validated.Financial.Config.Enabled || len(validated.Path) == 0 {
		return []OrderAllocation{}, nil
	}

	// ParseBetSnapshot guarantees one effective mode for the complete chain.
	base := fact.Basis.LossPoints
	if validated.Path[0].EffectiveMode == "turnover" {
		base = fact.Basis.TurnoverPoints
	}
	ratios := make([]string, len(validated.Path))
	for i, node := range validated.Path {
		ratios[i] = node.Config.Ratio
	}
	exact, err := DifferentialSameBasis(base, ratios)
	if err != nil {
		return nil, err
	}
	allocations := make([]OrderAllocation, len(validated.Path))
	for i, node := range validated.Path {
		allocations[i] = OrderAllocation{AgentID: node.ID, MemberID: node.MemberID, Exact: exact[i]}
	}
	return allocations, nil
}
