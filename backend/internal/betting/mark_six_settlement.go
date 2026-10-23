package betting

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"strings"

	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

// Each transaction settles one purchased batch. A durable placed order remains
// discoverable after a crash; the order lock and ledger keys prevent double payment.
func (s Service) ProcessMarkSixSettlements(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 100 {
		return 0, ErrInvalid
	}
	n := 0
	for ; n < limit; n++ {
		done, e := s.processMarkSixSettlement(ctx)
		if e != nil {
			return n, e
		}
		if !done {
			break
		}
	}
	return n, nil
}
func (s Service) processMarkSixSettlement(ctx context.Context) (bool, error) {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return false, e
	}
	defer tx.Rollback(ctx)
	var id, brand, member string
	// Do not hold a global period lock while paying brand wallets.
	e = tx.QueryRow(ctx, `SELECT o.id::text,o.brand_id::text,o.member_id::text FROM platform_mark_six_orders o JOIN platform_game_periods p ON p.id=o.period_id WHERE o.status='placed' AND p.result IS NOT NULL ORDER BY p.draw_at,o.id FOR UPDATE OF o SKIP LOCKED LIMIT 1`).Scan(&id, &brand, &member)
	if errors.Is(e, pgx.ErrNoRows) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	o, e := scanMarkSixOrder(tx.QueryRow(ctx, markSixOrderSelect+` WHERE o.id=$1`, id))
	if e != nil {
		return false, e
	}
	p, e := readMarkSixPeriod(ctx, tx, o.PeriodID)
	if e != nil {
		return false, e
	}
	if p.Result == nil || len(p.Result.Special) != 1 {
		return false, ErrInvalid
	}
	var raw, allocRaw []byte
	e = tx.QueryRow(ctx, `SELECT g.model,o.allocation FROM platform_mark_six_orders o JOIN platform_games g ON g.id=o.game_id WHERE o.id=$1`, id).Scan(&raw, &allocRaw)
	if e != nil {
		return false, e
	}
	var model rules.Model
	var alloc []points.Allocation
	if json.Unmarshal(raw, &model) != nil || rules.ValidateMarkSixModel(model) != nil || rules.ValidateDraw(model, *p.Result) != nil || json.Unmarshal(allocRaw, &alloc) != nil {
		return false, ErrSnapshot
	}
	// The consumed-source cursor fixes which original sources fund each item.
	remaining := append([]points.Allocation(nil), alloc...)
	refunds := map[string]points.Amount{}
	for i := range o.Items {
		item := &o.Items[i]
		outcome, full, e := rules.MarkSixLineOutcome(item.Code, item.Selection, *p.Result, rules.MarkSixNumberAttributes{Waves: o.Attributes.Waves, Zodiac: o.Attributes.Zodiac, Elements: o.Attributes.Elements})
		if e != nil {
			return false, e
		}
		odds := item.Odds
		if full {
			odds = item.OddsFull
			if odds == "" {
				return false, ErrSnapshot
			}
			item.WinningOdds = odds
		}
		var payout points.Amount
		if item.Code == "parlay" && outcome == "won" {
			codes, err := rules.MarkSixParlayCodes(item.Selection)
			if err != nil || len(item.LegOdds) != len(codes) {
				return false, ErrSnapshot
			}
			prices := []string{}
			for j, leg := range codes {
				v, err := rules.MarkSixPositionOutcome(leg, *p.Result, rules.MarkSixNumberAttributes{Waves: o.Attributes.Waves, Zodiac: o.Attributes.Zodiac, Elements: o.Attributes.Elements})
				if err != nil {
					return false, err
				}
				if v != "push" {
					prices = append(prices, item.LegOdds[j])
				}
			}
			odds, e = rules.MarkSixProduct(prices)
			if e != nil {
				return false, e
			}
			item.WinningOdds = odds
			rate, ok := new(big.Rat).SetString(odds)
			if !ok {
				return false, ErrSnapshot
			}
			payout, e = rules.MarkSixExactPayout(item.Points, rate)
		} else if strings.HasPrefix(item.Code, "regular_zodiac_") && outcome == "won" {
			item.HitCount = rules.MarkSixRegularZodiacHits(item.Code, *p.Result, rules.MarkSixNumberAttributes{Zodiac: o.Attributes.Zodiac})
			payout, e = rules.MarkSixZodiacPayout(item.Points, odds, item.HitCount)
		} else {
			payout, e = rules.MarkSixPayout(item.Points, odds, outcome)
		}
		if e != nil {
			return false, e
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
		if outcome == "push" {
			o.RefundPoints += payout
		} else {
			if int64(payout) > math.MaxInt64-int64(o.PrizePoints) {
				return false, points.ErrOverflow
			}
			o.PrizePoints += payout
			o.EffectivePoints += item.Points
		}
	}
	for _, a := range remaining {
		if a.Points != 0 {
			return false, ErrSnapshot
		}
	}
	ps := points.Store{DB: s.DB}
	if _, e = ps.LockedSnapshot(ctx, tx, brand, member); e != nil {
		return false, e
	}
	payoutID, refundID := "", ""
	if o.PrizePoints > 0 {
		var delta points.Balance
		delta[1][0] = o.PrizePoints
		entry, e := ps.Post(ctx, tx, points.Change{BrandID: brand, MemberID: member, EntryType: "prize", ReferenceType: "mark_six_order", ReferenceID: id, OperationKey: "mark-six-prize:" + id, Reason: "Mark Six winnings including stake", ActorType: "system", RequestID: "mark-six:" + id, Delta: delta, Allocation: []points.Allocation{{Source: "winning", State: "available", Points: o.PrizePoints}}})
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
		entry, e := ps.Post(ctx, tx, points.Change{BrandID: brand, MemberID: member, EntryType: "refund", ReferenceType: "mark_six_order", ReferenceID: id, OperationKey: "mark-six-refund:" + id, Reason: "Mark Six push; original stake sources", ActorType: "system", RequestID: "mark-six:" + id, Delta: delta, Allocation: refundAlloc})
		if e != nil {
			return false, e
		}
		refundID = entry.ID
	}
	items, _ := json.Marshal(o.Items)
	_, e = tx.Exec(ctx, `UPDATE platform_mark_six_orders SET items=$2,prize_points=$3,refund_points=$4,effective_points=$5,payout_entry_id=NULLIF($6,'')::uuid,refund_entry_id=NULLIF($7,'')::uuid,status='settled',settled_at=clock_timestamp() WHERE id=$1`, id, items, o.PrizePoints, o.RefundPoints, o.EffectivePoints, payoutID, refundID)
	if e != nil {
		return false, e
	}
	_, e = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "system", Action: "mark_six.settle", ResourceType: "mark_six_order", ResourceID: id, Reason: "published shared draw; purchased odds", RequestID: "mark-six:" + id, After: map[string]any{"prize_points": o.PrizePoints, "refund_points": o.RefundPoints, "effective_points": o.EffectivePoints, "items": o.Items, "payout_entry_id": payoutID, "refund_entry_id": refundID}})
	if e != nil {
		return false, e
	}
	return true, tx.Commit(ctx)
}
