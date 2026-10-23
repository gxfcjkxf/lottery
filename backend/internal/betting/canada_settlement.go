package betting

import (
	"context"
	"encoding/json"
	"errors"
	"math"

	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

// Each transaction settles one purchased batch. A durable placed order remains
// discoverable after a crash; the order lock and ledger keys prevent double payment.
func (s Service) ProcessCanadaSettlements(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 100 {
		return 0, ErrInvalid
	}
	n := 0
	for ; n < limit; n++ {
		done, e := s.processCanadaSettlement(ctx)
		if e != nil {
			return n, e
		}
		if !done {
			break
		}
	}
	return n, nil
}
func (s Service) processCanadaSettlement(ctx context.Context) (bool, error) {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return false, e
	}
	defer tx.Rollback(ctx)
	var id, brand, member string
	// Do not hold a global period lock while paying brand wallets.
	e = tx.QueryRow(ctx, `SELECT o.id::text,o.brand_id::text,o.member_id::text FROM platform_canada_orders o JOIN platform_game_periods p ON p.id=o.period_id WHERE o.status='placed' AND p.result IS NOT NULL ORDER BY p.draw_at,o.id FOR UPDATE OF o SKIP LOCKED LIMIT 1`).Scan(&id, &brand, &member)
	if errors.Is(e, pgx.ErrNoRows) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	o, e := scanCanadaOrder(tx.QueryRow(ctx, canadaOrderSelect+` WHERE o.id=$1`, id))
	if e != nil {
		return false, e
	}
	p, e := readCanadaPeriod(ctx, tx, o.PeriodID)
	if e != nil {
		return false, e
	}
	if p.Result == nil || len(p.Result.Digits) != 3 {
		return false, ErrInvalid
	}
	var raw, allocRaw []byte
	e = tx.QueryRow(ctx, `SELECT g.model,o.allocation FROM platform_canada_orders o JOIN platform_games g ON g.id=o.game_id WHERE o.id=$1`, id).Scan(&raw, &allocRaw)
	if e != nil {
		return false, e
	}
	var model rules.Model
	var alloc []points.Allocation
	if json.Unmarshal(raw, &model) != nil || rules.ValidateCanadaModel(model) != nil || rules.ValidateDraw(model, *p.Result) != nil || json.Unmarshal(allocRaw, &alloc) != nil {
		return false, ErrSnapshot
	}
	if o.Profile == "canada_niuniu" {
		if e = s.settleCanadaNiu(ctx, tx, &o, *p.Result, alloc); e != nil {
			return false, e
		}
		return true, tx.Commit(ctx)
	}
	var nonTailTotal int64
	if e = tx.QueryRow(ctx, `SELECT coalesce(sum((i->>'points')::bigint),0)::bigint FROM platform_canada_orders o CROSS JOIN LATERAL jsonb_array_elements(o.items) i WHERE o.brand_id=$1 AND o.member_id=$2 AND o.period_id=$3 AND i->>'code' NOT LIKE 'tail_%'`, brand, member, o.PeriodID).Scan(&nonTailTotal); e != nil {
		return false, e
	}
	ps := points.Store{DB: s.DB}
	if _, e = ps.LockedSnapshot(ctx, tx, brand, member); e != nil {
		return false, e
	}
	var used int64
	if e = tx.QueryRow(ctx, `SELECT coalesce(sum(prize_points),0)::bigint FROM platform_canada_orders WHERE brand_id=$1 AND member_id=$2 AND period_id=$3 AND status='settled'`, brand, member, o.PeriodID).Scan(&used); e != nil {
		return false, e
	}
	remainingCap := int64(888888) - used
	if remainingCap < 0 {
		return false, ErrSnapshot
	}
	// The consumed-source cursor fixes which original sources fund each item.
	remaining := append([]points.Allocation(nil), alloc...)
	refunds := map[string]points.Amount{}
	for i := range o.Items {
		item := &o.Items[i]
		outcome, price, effective, e := rules.CanadaOutcome(o.Profile, item.Code, *p.Result, nonTailTotal)
		if e != nil {
			return false, e
		}
		odds := item.Odds
		if price != item.Code {
			odds = item.SpecialOdds
			if odds == "" {
				return false, ErrSnapshot
			}
			item.WinningOdds = odds
		}
		item.Effective = effective
		payout, e := rules.MarkSixPayout(item.Points, odds, outcome)
		if e != nil {
			return false, e
		}
		if outcome == "won" {
			if int64(payout) > remainingCap {
				payout = points.Amount(remainingCap)
			}
			remainingCap -= int64(payout)
		}
		item.Outcome = outcome
		item.Payout = payout
		needed := item.Points
		for j := range remaining {
			take := remaining[j].Points
			if take > needed {
				take = needed
			}
			if outcome == "push" {
				refunds[remaining[j].Source] += take
			}
			remaining[j].Points -= take
			needed -= take
			if needed == 0 {
				break
			}
		}
		if needed != 0 {
			return false, ErrSnapshot
		}
		if effective {
			o.EffectivePoints += item.Points
		}
		if outcome == "push" {
			o.RefundPoints += payout
		} else {
			if int64(payout) > math.MaxInt64-int64(o.PrizePoints) {
				return false, points.ErrOverflow
			}
			o.PrizePoints += payout

		}
	}
	for _, a := range remaining {
		if a.Points != 0 {
			return false, ErrSnapshot
		}
	}
	payoutID, refundID := "", ""
	if o.PrizePoints > 0 {
		var delta points.Balance
		delta[1][0] = o.PrizePoints
		entry, e := ps.Post(ctx, tx, points.Change{BrandID: brand, MemberID: member, EntryType: "prize", ReferenceType: "canada_order", ReferenceID: id, OperationKey: "canada-prize:" + id, Reason: "Canada winnings including stake", ActorType: "system", RequestID: "canada:" + id, Delta: delta, Allocation: []points.Allocation{{Source: "winning", State: "available", Points: o.PrizePoints}}})
		if e != nil {
			return false, e
		}
		payoutID = entry.ID
	}
	if o.RefundPoints > 0 {
		refundAlloc := []points.Allocation{}
		for _, a := range alloc {
			if v := refunds[a.Source]; v > 0 {
				refundAlloc = append(refundAlloc, points.Allocation{Source: a.Source, State: "available", Points: v})
			}
		}
		delta, e := points.AllocationDelta(refundAlloc, "", "available")
		if e != nil {
			return false, e
		}
		entry, e := ps.Post(ctx, tx, points.Change{BrandID: brand, MemberID: member, EntryType: "refund", ReferenceType: "canada_order", ReferenceID: id, OperationKey: "canada-refund:" + id, Reason: "Canada push; original stake sources", ActorType: "system", RequestID: "canada:" + id, Delta: delta, Allocation: refundAlloc})
		if e != nil {
			return false, e
		}
		refundID = entry.ID
	}
	items, _ := json.Marshal(o.Items)
	_, e = tx.Exec(ctx, `UPDATE platform_canada_orders SET items=$2,prize_points=$3,refund_points=$4,effective_points=$5,payout_entry_id=NULLIF($6,'')::uuid,refund_entry_id=NULLIF($7,'')::uuid,status='settled',settled_at=clock_timestamp() WHERE id=$1`, id, items, o.PrizePoints, o.RefundPoints, o.EffectivePoints, payoutID, refundID)
	if e != nil {
		return false, e
	}
	_, e = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "system", Action: "canada.settle", ResourceType: "canada_order", ResourceID: id, Reason: "published shared draw; purchased odds", RequestID: "canada:" + id, After: map[string]any{"prize_points": o.PrizePoints, "refund_points": o.RefundPoints, "effective_points": o.EffectivePoints, "items": o.Items, "payout_entry_id": payoutID, "refund_entry_id": refundID}})
	if e != nil {
		return false, e
	}
	return true, tx.Commit(ctx)
}
