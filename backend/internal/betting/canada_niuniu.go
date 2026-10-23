package betting

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
	"math"
)

func (s Service) settleCanadaNiu(ctx context.Context, tx pgx.Tx, o *CanadaOrder, d rules.Draw, alloc []points.Allocation) error {
	ps := points.Store{DB: s.DB}
	if _, e := ps.LockedSnapshot(ctx, tx, o.BrandID, o.MemberID); e != nil {
		return e
	}
	var used int64
	if e := tx.QueryRow(ctx, `SELECT coalesce(sum(prize_points),0)::bigint FROM platform_canada_orders WHERE brand_id=$1 AND member_id=$2 AND period_id=$3 AND status='settled'`, o.BrandID, o.MemberID, o.PeriodID).Scan(&used); e != nil {
		return e
	}
	cap := int64(888888) - used
	if cap < 0 {
		return ErrSnapshot
	}
	remaining := append([]points.Allocation(nil), alloc...)
	releaseBySource, lossBySource := map[string]points.Amount{}, map[string]points.Amount{}
	for i := range o.Items {
		item := &o.Items[i]
		out, win, loss, e := rules.CanadaNiu(item.Code, d)
		if e != nil {
			return e
		}
		risk := rules.CanadaRisk(o.Profile, item.Code)
		if item.Points > points.Amount(math.MaxInt64/risk) || item.RiskPoints != item.Points*points.Amount(risk) {
			return ErrSnapshot
		}
		item.Outcome = out
		item.Effective = out != "push"
		if item.Effective {
			o.EffectivePoints += item.Points
		}
		item.LossPoints = item.Points * points.Amount(loss)
		o.LostPoints += item.LossPoints
		if win > 0 {
			amount, rate, e := rules.CanadaNiuProfit(item.Points, item.Odds, win)
			if e != nil {
				return e
			}
			item.WinningOdds = rate
			if int64(amount) > cap {
				amount = points.Amount(cap)
			}
			cap -= int64(amount)
			item.Payout = amount
			o.PrizePoints += amount
		}

		needed, lossNeeded := item.RiskPoints, item.LossPoints
		for j := range remaining {
			take := remaining[j].Points
			if take > needed {
				take = needed
			}
			lost := take
			if lost > lossNeeded {
				lost = lossNeeded
			}
			lossBySource[remaining[j].Source] += lost
			releaseBySource[remaining[j].Source] += take - lost
			remaining[j].Points -= take
			needed -= take
			lossNeeded -= lost
			if needed == 0 {
				break
			}
		}
		if needed != 0 || lossNeeded != 0 {
			return ErrSnapshot
		}
	}
	losses, releases := []points.Allocation{}, []points.Allocation{}
	for _, a := range alloc {
		if n := lossBySource[a.Source]; n > 0 {
			losses = append(losses, points.Allocation{Source: a.Source, State: "system_frozen", Points: n})
		}
		if n := releaseBySource[a.Source]; n > 0 {
			releases = append(releases, points.Allocation{Source: a.Source, State: "system_frozen", Points: n})
			o.RefundPoints += n
		}
	}
	post := func(kind, operation, reason string, a []points.Allocation, from, to string) (string, error) {
		delta, e := points.AllocationDelta(a, from, to)
		if e != nil {
			return "", e
		}
		entry, e := ps.Post(ctx, tx, points.Change{BrandID: o.BrandID, MemberID: o.MemberID, EntryType: kind, ReferenceType: "canada_order", ReferenceID: o.ID, OperationKey: "canada-" + operation + ":" + o.ID, Reason: reason, ActorType: "system", RequestID: "canada:" + o.ID, Delta: delta, Allocation: a})
		return entry.ID, e
	}
	var lossID, refundID, prizeID string
	var e error
	if len(losses) > 0 {
		lossID, e = post("bet", "loss", "Niu loss consumed from reserved original sources", losses, "system_frozen", "")
		if e != nil {
			return e
		}
	}
	if len(releases) > 0 {
		refundID, e = post("refund", "refund", "Niu unused risk released to original available sources", releases, "system_frozen", "available")
		if e != nil {
			return e
		}
	}
	if o.PrizePoints > 0 {
		prizeID, e = post("prize", "prize", "Niu net profit after 3 percent commission", []points.Allocation{{Source: "winning", State: "available", Points: o.PrizePoints}}, "", "available")
		if e != nil {
			return e
		}
	}
	raw, _ := json.Marshal(o.Items)
	_, e = tx.Exec(ctx, `UPDATE platform_canada_orders SET items=$2,prize_points=$3,refund_points=$4,effective_points=$5,lost_points=$6,loss_entry_id=NULLIF($7,'')::uuid,payout_entry_id=NULLIF($8,'')::uuid,refund_entry_id=NULLIF($9,'')::uuid,status='settled',settled_at=clock_timestamp() WHERE id=$1`, o.ID, raw, o.PrizePoints, o.RefundPoints, o.EffectivePoints, o.LostPoints, lossID, prizeID, refundID)
	if e != nil {
		return e
	}
	_, e = audit.Append(ctx, tx, audit.Record{BrandID: o.BrandID, ActorType: "system", Action: "canada.settle", ResourceType: "canada_order", ResourceID: o.ID, Reason: "Niu risk settlement with purchased rates and original sources", RequestID: "canada:" + o.ID, After: o})
	return e
}
