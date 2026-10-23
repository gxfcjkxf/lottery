package rules

import "math/big"

// selectTierAmounts shares the established amount-based exclusive/additive
// semantics between both schema interpreters. Equal amounts keep stable order.
func selectTierAmounts(amounts []*big.Rat, matched, exclusive []bool, policy string) []bool {
	bestAll, bestExclusive := -1, -1
	for i, hit := range matched {
		if !hit {
			continue
		}
		if bestAll < 0 || amounts[i].Cmp(amounts[bestAll]) > 0 {
			bestAll = i
		}
		if exclusive[i] && (bestExclusive < 0 || amounts[i].Cmp(amounts[bestExclusive]) > 0) {
			bestExclusive = i
		}
	}
	selected := make([]bool, len(matched))
	for i, hit := range matched {
		if !hit {
			continue
		}
		selected[i] = true
		if bestExclusive >= 0 {
			if policy == "max_exclusive_plus_additive" {
				selected[i] = !exclusive[i] || i == bestExclusive
			} else {
				selected[i] = i == bestAll
			}
		}
	}
	return selected
}
