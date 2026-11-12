package platformlottery

import (
	"context"
	"errors"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

var ErrPeriodInUse = errors.New("period already has orders or is closed")

type PeriodOpeningInput struct {
	BetStartAt         time.Time `json:"bet_start_at"`
	ExpectedBetStartAt time.Time `json:"expected_bet_start_at"`
}

// Lock the game before the period, matching admission's lock order. Opening
// changes cannot race an accepted stake or rewrite a purchased sales window.
func (s Service) UpdatePeriodOpening(ctx context.Context, tx pgx.Tx, a access.Account, id string, in PeriodOpeningInput, meta points.Metadata) (Period, error) {
	if !Allowed(a, "", "write") {
		return Period{}, ErrDenied
	}
	if in.BetStartAt.IsZero() || in.ExpectedBetStartAt.IsZero() {
		return Period{}, ErrInvalid
	}
	var p Period
	if err := tx.QueryRow(ctx, `SELECT game_id::text FROM platform_game_periods WHERE id=$1`, id).Scan(&p.GameID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return p, ErrNotFound
		}
		return p, err
	}
	g, err := lockedGame(ctx, tx, p.GameID)
	if err != nil {
		return p, err
	}
	var scheduled bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform_draw_schedule_settings WHERE game_id=$1)`, p.GameID).Scan(&scheduled); err != nil {
		return p, err
	}
	if scheduled {
		return p, ErrPeriodInUse
	}
	if g.Status != "active" {
		return p, ErrConflict
	}
	var closed bool
	err = tx.QueryRow(ctx, `SELECT id::text,period_no,bet_start_at,bet_end_at,draw_at,result IS NOT NULL OR bet_end_at<=clock_timestamp() FROM platform_game_periods WHERE id=$1 FOR UPDATE`, id).Scan(&p.ID, &p.PeriodNo, &p.BetStartAt, &p.BetEndAt, &p.DrawAt, &closed)
	if err != nil {
		return p, err
	}
	if !p.BetStartAt.Equal(in.ExpectedBetStartAt) {
		return p, ErrVersionConflict
	}
	if !in.BetStartAt.Before(p.BetEndAt) {
		return p, ErrInvalid
	}
	var purchased bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM retention_configured_orders() WHERE period_id=$1)`, id).Scan(&purchased)
	if err != nil {
		return p, err
	}
	if closed || purchased {
		return p, ErrPeriodInUse
	}
	before := p.BetStartAt
	if _, err = tx.Exec(ctx, `UPDATE platform_game_periods SET bet_start_at=$2 WHERE id=$1`, id, in.BetStartAt); err != nil {
		return p, err
	}
	p.BetStartAt = in.BetStartAt
	err = record(ctx, tx, a, "", "platform_game.period.opening", id, "", meta, map[string]any{"bet_start_at": before, "period_no": p.PeriodNo}, map[string]any{"bet_start_at": p.BetStartAt, "period_no": p.PeriodNo})
	return p, err
}
