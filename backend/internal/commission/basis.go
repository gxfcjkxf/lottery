// Package commission derives commission bases from finalized order facts.
package commission

import (
	"errors"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

var ErrInvalid = errors.New("invalid commission fact")

// Basis contains the stake totals used to calculate turnover and loss
// commissions. Commission rates and payouts are deliberately outside this
// helper.
type Basis struct {
	TurnoverPoints points.Amount `json:"turnover_points"`
	LossPoints     points.Amount `json:"loss_points"`
}

// Bases derives commission bases from an authoritative finalized order fact.
// Only won and lost orders qualify. A won order contributes turnover even if
// its prize is zero or below its stake, but never contributes to the loss
// basis.
func Bases(status string, stake, prize points.Amount) (Basis, error) {
	if status != "won" && status != "lost" {
		return Basis{}, ErrInvalid
	}
	if stake <= 0 || prize < 0 || (status == "lost" && prize != 0) {
		return Basis{}, ErrInvalid
	}

	basis := Basis{TurnoverPoints: stake}
	if status == "lost" {
		basis.LossPoints = stake
	}
	return basis, nil
}
