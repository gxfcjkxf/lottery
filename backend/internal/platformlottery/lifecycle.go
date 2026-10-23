package platformlottery

import (
	"context"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

var ErrGameInUse = errors.New("game has unpublished periods or unsettled orders")

// Shared by catalogue eligibility and the locked write check.
const gameDeletionBlockers = `EXISTS(SELECT 1 FROM platform_game_periods p WHERE p.game_id=g.id AND p.result IS NULL)
 OR EXISTS(SELECT 1 FROM platform_canada_orders o WHERE o.game_id=g.id AND o.status='placed')
 OR EXISTS(SELECT 1 FROM platform_mark_six_orders o WHERE o.game_id=g.id AND o.status='placed')
 OR EXISTS(SELECT 1 FROM configured_orders o WHERE o.game_id=g.id AND o.status='placed')`

type LifecycleInput struct {
	ExpectedVersion int64  `json:"expected_version"`
	Status          string `json:"status"`
	Reason          string `json:"reason"`
}
type AvailabilityInput struct {
	ExpectedVersion int64  `json:"expected_version"`
	Enabled         *bool  `json:"enabled"`
	Reason          string `json:"reason"`
}

// Deletion is a catalogue tombstone: immutable configuration and audit evidence stay intact.
// Only unfinished periods and purchases block this terminal state.
func (s Service) SetLifecycle(ctx context.Context, tx pgx.Tx, a access.Account, id string, in LifecycleInput, meta points.Metadata) (Game, error) {
	if !Allowed(a, "", "write") {
		return Game{}, ErrDenied
	}
	if in.ExpectedVersion < 1 || (in.Reason != "" && !textOK(in.Reason, 500)) || (in.Status != "active" && in.Status != "offline" && in.Status != "deleted") {
		return Game{}, ErrInvalid
	}
	g, e := lockedGame(ctx, tx, id)
	if e != nil {
		return g, e
	}
	if g.Version != in.ExpectedVersion {
		return g, ErrVersionConflict
	}
	if in.Status == "deleted" {
		var used bool
		e = tx.QueryRow(ctx, `SELECT `+gameDeletionBlockers+` FROM platform_games g WHERE g.id=$1`, id).Scan(&used)
		if e != nil {
			return g, e
		}
		if used {
			return g, ErrGameInUse
		}
	}
	if _, e = tx.Exec(ctx, `UPDATE platform_games SET status=$2,version=version+1 WHERE id=$1`, id, in.Status); e != nil {
		return g, e
	}
	g.Status = in.Status
	g.Version++
	return g, record(ctx, tx, a, "", "platform_game.lifecycle", id, in.Reason, meta, g)
}
func (s Service) SetAvailability(ctx context.Context, tx pgx.Tx, a access.Account, brand, id string, in AvailabilityInput, meta points.Metadata) (Game, error) {
	if !Allowed(a, brand, "sell") {
		return Game{}, ErrDenied
	}
	if in.ExpectedVersion < 1 || in.Enabled == nil || (in.Reason != "" && !textOK(in.Reason, 500)) {
		return Game{}, ErrInvalid
	}
	g, e := lockedGame(ctx, tx, id)
	if e != nil {
		return g, e
	}
	e = tx.QueryRow(ctx, `SELECT enabled,version FROM brand_platform_games WHERE brand_id=$1 AND game_id=$2 FOR UPDATE`, brand, id).Scan(&g.BrandEnabled, &g.BrandVersion)
	if errors.Is(e, pgx.ErrNoRows) {
		return g, ErrNotFound
	}
	if e != nil {
		return g, e
	}
	if g.BrandVersion != in.ExpectedVersion {
		return g, ErrVersionConflict
	}
	if _, e = tx.Exec(ctx, `UPDATE brand_platform_games SET enabled=$3,version=version+1 WHERE brand_id=$1 AND game_id=$2`, brand, id, *in.Enabled); e != nil {
		return g, e
	}
	g.Connected = true
	g.BrandEnabled = *in.Enabled
	g.BrandVersion++
	return g, record(ctx, tx, a, brand, "brand.platform_game.availability", id, in.Reason, meta, g)
}
