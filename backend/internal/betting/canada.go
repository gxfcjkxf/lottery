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

type CanadaItem struct {
	RiskPoints  points.Amount `json:"risk_points"`
	LossPoints  points.Amount `json:"loss_points"`
	Code        string        `json:"code"`
	Points      points.Amount `json:"points"`
	Odds        string        `json:"odds"`
	SpecialOdds string        `json:"special_odds,omitempty"`
	WinningOdds string        `json:"winning_odds,omitempty"`
	Outcome     string        `json:"outcome"`
	Payout      points.Amount `json:"payout"`
	Effective   bool          `json:"effective"`
}
type CanadaPurchase struct {
	ReservedPoints points.Amount `json:"reserved_points"`
	Profile        string        `json:"profile"`
	GameID         string        `json:"game_id"`
	PeriodID       string        `json:"period_id"`
	RuleVersion    int64         `json:"rule_version"`
	SalesVersion   int64         `json:"sales_version"`
	Items          []CanadaItem  `json:"items"`
	TotalPoints    points.Amount `json:"total_points"`
}
type CanadaOrder struct {
	LostPoints points.Amount `json:"lost_points"`
	ID         string        `json:"id"`
	BrandID    string        `json:"brand_id"`
	MemberID   string        `json:"member_id"`
	PeriodNo   string        `json:"period_no"`
	CanadaPurchase
	PrizePoints     points.Amount `json:"prize_points"`
	RefundPoints    points.Amount `json:"refund_points"`
	EffectivePoints points.Amount `json:"effective_points"`
	Status          string        `json:"status"`
	PlacedAt        time.Time     `json:"placed_at"`
	SettledAt       *time.Time    `json:"settled_at"`
}
type canadaQuery interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func readCanadaPeriod(ctx context.Context, q canadaQuery, id string) (platformlottery.Period, error) {
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
	return p, e
}

const canadaOrderSelect = `SELECT o.id::text,o.brand_id::text,o.member_id::text,p.period_no,o.game_id::text,o.period_id::text,o.rule_version,o.sales_version,o.items,o.total_points,o.prize_points,o.refund_points,o.effective_points,o.status,o.placed_at,o.settled_at,o.profile,o.reserved_points,o.lost_points FROM platform_canada_orders o JOIN platform_game_periods p ON p.id=o.period_id`

func scanCanadaOrder(row pgx.Row) (o CanadaOrder, e error) {
	var raw []byte
	e = row.Scan(&o.ID, &o.BrandID, &o.MemberID, &o.PeriodNo, &o.GameID, &o.PeriodID, &o.RuleVersion, &o.SalesVersion, &raw, &o.TotalPoints, &o.PrizePoints, &o.RefundPoints, &o.EffectivePoints, &o.Status, &o.PlacedAt, &o.SettledAt, &o.Profile, &o.ReservedPoints, &o.LostPoints)
	if e == nil {
		e = json.Unmarshal(raw, &o.Items)
	}
	return
}
func (s Service) CanadaOrders(ctx context.Context, brand, member, game string, offset int) ([]CanadaOrder, error) {
	if !validIDs(brand) || game != "" && !validIDs(game) || member != "" && !validIDs(member) || offset < 0 || offset > 1000000 {
		return nil, ErrInvalid
	}
	rows, e := s.DB.Query(ctx, canadaOrderSelect+` WHERE o.brand_id=$1 AND ($2='' OR o.member_id=NULLIF($2,'')::uuid) AND ($3='' OR o.game_id=NULLIF($3,'')::uuid) ORDER BY o.placed_at DESC,o.id DESC LIMIT 50 OFFSET $4`, brand, member, game, offset)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []CanadaOrder{}
	for rows.Next() {
		o, e := scanCanadaOrder(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
