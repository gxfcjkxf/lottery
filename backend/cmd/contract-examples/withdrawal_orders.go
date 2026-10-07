package main

import (
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/withdrawal"
)

// withdrawalExamples converts domain values through the exported response DTOs.
// These deterministic examples never open a database or perform a withdrawal.
func withdrawalExamples() map[string]any {
	const id = "11111111-1111-4111-8111-111111111111"
	now := time.Date(2026, 10, 6, 12, 30, 0, 0, time.UTC)
	cycleFromAt := now.Add(-24 * time.Hour)
	order := withdrawal.Order{
		ID: id, BrandID: id, MemberID: id, AccountID: id,
		Points: 8, State: "reviewing", Version: 1,
		SourceAllocation: []points.Allocation{{Source: "recharge", State: "available", Points: 8}},
		CycleFromAt:      &cycleFromAt, CycleFromVersion: 1, ReserveVersion: 1,
		ReserveEntryID: id, CreatedAt: now, UpdatedAt: now,
		DecisionReason: "awaiting review", AuditLogID: id,
	}
	view := withdrawal.ToOrderView(order)
	availability := withdrawal.AvailabilityView{
		BrandID: id, MemberID: id, PolicyEnabled: true, EligibilityConfigured: true,
		CanApply: true, ReasonCode: "AVAILABLE", MinPoints: 1,
		AllowedSources: []string{"recharge", "winning", "gift"}, RealPayments: false,
		ActorContext: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	history := withdrawal.HistoryView{
		BrandID: id, OrderID: id,
		Items: []withdrawal.TransitionView{{
			ID: id, Version: 1, FromState: "", ToState: "reviewing",
			Reason: "withdrawal requested", ActorType: "user", CreatedAt: now, AuditLogID: id,
		}},
	}
	return map[string]any{
		"WithdrawalOrderExample":        view,
		"WithdrawalOrderPageExample":    withdrawal.OrderPage{BrandID: id, Items: []withdrawal.OrderView{view}, Limit: 20, Offset: 0, HasMore: false},
		"WithdrawalHistoryExample":      history,
		"WithdrawalAvailabilityExample": availability,
	}
}
