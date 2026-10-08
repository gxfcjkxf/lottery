package main

import (
	"math"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rewards"
)

func rewardExamples() map[string]any {
	const id = "11111111-1111-4111-8111-111111111111"
	now := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	o := rewards.Order{ID: id, BrandID: id, MemberID: id, Points: math.MaxInt64, State: "granted", Version: 1, GrantLedgerEntryID: id,
		CreationAuditLogID: id, LastAuditLogID: id, CreatedBy: id, Reason: "explicit manual grant", PointPolicyVersion: "1", CreatedAt: now, UpdatedAt: now}
	a := rewards.Action{ID: id, BrandID: id, OrderID: id, Version: 1, Operation: "grant", StateAfter: "granted", ActorID: id, Reason: o.Reason, AuditLogID: id, LedgerEntryID: &o.GrantLedgerEntryID, CreatedAt: now}
	return map[string]any{
		"RewardGrantInput":      rewards.GrantInput{MemberID: id, Points: points.Amount(math.MaxInt64), Reason: o.Reason},
		"RewardActionInput":     rewards.ActionInput{Version: 2, Reason: "explicit pending continuation"},
		"RewardOrder":           o,
		"RewardOrderPage":       rewards.OrderPage{BrandID: id, Items: []rewards.Order{o}, TotalCount: "1", Limit: 20, Offset: 0},
		"RewardOrderAction":     a,
		"RewardOrderActionPage": rewards.ActionPage{BrandID: id, OrderID: id, Items: []rewards.Action{a}, TotalCount: "1", Limit: 20, Offset: 0},
	}
}
