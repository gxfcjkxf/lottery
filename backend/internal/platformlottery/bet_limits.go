package platformlottery

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

type BetLimits struct {
	BrandID             string        `json:"brand_id"`
	GameID              string        `json:"game_id"`
	Version             int64         `json:"version"`
	MinPoints           points.Amount `json:"min_points"`
	MaxPoints           points.Amount `json:"max_points"`
	MaxPeriodPoints     points.Amount `json:"max_period_points"`
	MaxUserPeriodPoints points.Amount `json:"max_user_period_points"`
}

type BetLimitGame struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
}

func (s Service) BetLimitGames(ctx context.Context, brand string) ([]BetLimitGame, error) {
	rows, err := s.DB.Query(ctx, `SELECT g.id::text,g.name,EXISTS (
		SELECT 1 FROM retention_audit_logs() a WHERE a.brand_id=b.brand_id AND a.resource_id=g.id
		AND a.resource_type='platform_game' AND a.action='bet_limits.write'
	) FROM platform_games g JOIN brand_platform_games b ON b.game_id=g.id
	WHERE b.brand_id=$1 AND g.status<>'deleted' ORDER BY g.name,g.id`, brand)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []BetLimitGame{}
	for rows.Next() {
		var game BetLimitGame
		if err := rows.Scan(&game.ID, &game.Name, &game.Configured); err != nil {
			return nil, err
		}
		items = append(items, game)
	}
	return items, rows.Err()
}

func limitsFromSales(brand, game string, v ConfiguredSales) BetLimits {
	return BetLimits{brand, game, v.Version, v.MinPoints, v.MaxPoints, v.MaxPeriodPoints, v.MaxUserPeriodPoints}
}
func (s Service) BetLimits(ctx context.Context, brand, game string) (BetLimits, error) {
	if _, err := s.Detail(ctx, brand, game, 1, 0); err != nil {
		return BetLimits{}, err
	}
	sales, err := ReadConfiguredSales(ctx, s.DB, brand, game, false)
	return limitsFromSales(brand, game, sales), err
}
func (s Service) WriteBetLimits(ctx context.Context, tx pgx.Tx, a access.Account, brand, game string, in BetLimits, meta points.Metadata) (BetLimits, error) {
	if a.SuperAdmin || !access.Authorize(a, "bet_policy", "write", access.ScopeBrand, brand) {
		return BetLimits{}, ErrDenied
	}
	if in.BrandID != brand || in.GameID != game || in.Version < 1 || in.MinPoints < 1 || in.MaxPoints < in.MinPoints || in.MaxPeriodPoints < in.MaxPoints || in.MaxUserPeriodPoints < in.MaxPoints {
		return BetLimits{}, ErrInvalid
	}
	// Match admission's game -> brand connection -> sales lock order. Only limits
	// and the sales version change; prices, rule binding and sales state stay intact.
	if _, err := lockedGame(ctx, tx, game); err != nil {
		return BetLimits{}, err
	}
	var connected bool
	if err := tx.QueryRow(ctx, `SELECT enabled FROM brand_platform_games WHERE brand_id=$1 AND game_id=$2 FOR SHARE`, brand, game).Scan(&connected); err != nil {
		if err == pgx.ErrNoRows {
			return BetLimits{}, ErrNotFound
		}
		return BetLimits{}, err
	}
	before, err := ReadConfiguredSales(ctx, tx, brand, game, true)
	if err != nil {
		return BetLimits{}, err
	}
	if before.Version != in.Version {
		return BetLimits{}, ErrVersionConflict
	}
	err = tx.QueryRow(ctx, `UPDATE configured_sales SET version=version+1,min_points=$3,max_points=$4,max_period_points=$5,max_user_period_points=$6 WHERE brand_id=$1 AND game_id=$2 RETURNING version`, brand, game, in.MinPoints, in.MaxPoints, in.MaxPeriodPoints, in.MaxUserPeriodPoints).Scan(&in.Version)
	if err == nil {
		err = record(ctx, tx, a, brand, "bet_limits.write", game, "", meta, limitsFromSales(brand, game, before), in)
	}
	return in, err
}
