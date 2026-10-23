package betting

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

type MarkSixItem struct {
	LegOdds     []string      `json:"leg_odds,omitempty"`
	HitCount    int           `json:"hit_count,omitempty"`
	Selection   []int         `json:"selection,omitempty"`
	OddsCode    string        `json:"odds_code,omitempty"`
	OddsFull    string        `json:"odds_full,omitempty"`
	WinningOdds string        `json:"winning_odds,omitempty"`
	Code        string        `json:"code"`
	Points      points.Amount `json:"points"`
	Odds        string        `json:"odds"`
	Outcome     string        `json:"outcome"`
	Payout      points.Amount `json:"payout"`
}
type MarkSixPurchase struct {
	Plate        string                            `json:"plate"`
	Attributes   platformlottery.MarkSixAttributes `json:"attributes"`
	GameID       string                            `json:"game_id"`
	PeriodID     string                            `json:"period_id"`
	RuleVersion  int64                             `json:"rule_version"`
	SalesVersion int64                             `json:"sales_version"`
	Items        []MarkSixItem                     `json:"items"`
	TotalPoints  points.Amount                     `json:"total_points"`
}
type MarkSixOrder struct {
	ID       string `json:"id"`
	BrandID  string `json:"brand_id"`
	MemberID string `json:"member_id"`
	PeriodNo string `json:"period_no"`
	MarkSixPurchase
	PrizePoints     points.Amount `json:"prize_points"`
	RefundPoints    points.Amount `json:"refund_points"`
	EffectivePoints points.Amount `json:"effective_points"`
	Status          string        `json:"status"`
	PlacedAt        time.Time     `json:"placed_at"`
	SettledAt       *time.Time    `json:"settled_at"`
}
type markSixQuery interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func readMarkSixPeriod(ctx context.Context, q markSixQuery, id string) (platformlottery.Period, error) {
	var p platformlottery.Period
	var raw []byte
	e := q.QueryRow(ctx, `SELECT id::text,game_id::text,period_no,bet_start_at,bet_end_at,draw_at,result,published_at FROM platform_game_periods WHERE id=$1`, id).Scan(&p.ID, &p.GameID, &p.PeriodNo, &p.BetStartAt, &p.BetEndAt, &p.DrawAt, &raw, &p.PublishedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	if e == nil && len(raw) > 0 {
		p.Result = &rules.Draw{}
		e = json.Unmarshal(raw, p.Result)
	}
	if e == nil {
		p.Attributes, e = platformlottery.ReadPeriodMarkSixAttributes(ctx, q, id)
	}
	return p, e
}

const markSixOrderSelect = `SELECT o.id::text,o.brand_id::text,o.member_id::text,p.period_no,o.game_id::text,o.period_id::text,o.rule_version,o.sales_version,o.plate,o.items,o.total_points,o.prize_points,o.refund_points,o.effective_points,o.status,o.placed_at,o.settled_at,a.id::text,a.year,a.version,a.effective_at,a.expires_at,o.attribute_snapshot FROM platform_mark_six_orders o JOIN platform_game_periods p ON p.id=o.period_id JOIN platform_mark_six_attributes a ON a.id=o.attribute_id`

func scanMarkSixOrder(row pgx.Row) (o MarkSixOrder, e error) {
	var raw, waves []byte
	e = row.Scan(&o.ID, &o.BrandID, &o.MemberID, &o.PeriodNo, &o.GameID, &o.PeriodID, &o.RuleVersion, &o.SalesVersion, &o.Plate, &raw, &o.TotalPoints, &o.PrizePoints, &o.RefundPoints, &o.EffectivePoints, &o.Status, &o.PlacedAt, &o.SettledAt, &o.Attributes.ID, &o.Attributes.Year, &o.Attributes.Version, &o.Attributes.EffectiveAt, &o.Attributes.ExpiresAt, &waves)
	if e == nil {
		e = json.Unmarshal(raw, &o.Items)
		if e == nil {
			e = json.Unmarshal(waves, &o.Attributes)
		}
	}
	return
}
func (s Service) MarkSixOrders(ctx context.Context, brand, member, game string, offset int) ([]MarkSixOrder, error) {
	if !validIDs(brand) || game != "" && !validIDs(game) || member != "" && !validIDs(member) || offset < 0 || offset > 1000000 {
		return nil, ErrInvalid
	}
	rows, e := s.DB.Query(ctx, markSixOrderSelect+` WHERE o.brand_id=$1 AND ($2='' OR o.member_id=NULLIF($2,'')::uuid) AND ($3='' OR o.game_id=NULLIF($3,'')::uuid) ORDER BY o.placed_at DESC,o.id DESC LIMIT 50 OFFSET $4`, brand, member, game, offset)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []MarkSixOrder{}
	for rows.Next() {
		o, e := scanMarkSixOrder(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
