package main

import (
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

func commissionAllocationExamples() map[string]any {
	const id = "11111111-1111-4111-8111-111111111111"
	at := time.Date(2026, 10, 9, 0, 0, 0, 123456789, time.UTC)
	base := points.Amount(9007199254740993)
	exacts, err := commission.DifferentialSameBasis(base, []string{"0.123456", "0.120001"})
	if err != nil {
		panic(err)
	}
	allocation := commission.Allocation{BrandID: id, CycleID: id, RunID: id, CalculationID: id, OrderID: id, AgentID: id, MemberID: id, BettorMemberID: id,
		BasePoints: base, Mode: "turnover", AgentRatio: "0.123456", DownstreamRatio: "0.120001", DifferenceRatio: "0.003455", ExactAmount: exacts[0], CreatedAt: at}
	return map[string]any{
		"CommissionAllocation":     allocation,
		"CommissionAllocationPage": commission.AllocationPage{BrandID: id, CycleID: id, RunID: id, Items: []commission.Allocation{allocation}, TotalCount: "1", Limit: 20},
		"CommissionRunEarningPage": commission.RunEarningPage{BrandID: id, CycleID: id, RunID: id, Items: []commission.Earning{}, TotalCount: "0", Limit: 20},
	}
}
