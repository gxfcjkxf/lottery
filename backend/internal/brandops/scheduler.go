package brandops

import (
	"context"
	"errors"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

// Tick processes due windows under the same brand lock used by betting and
// manual changes. Each transition is atomic; multiple workers skip locked rows.
func (s Service) Tick(ctx context.Context) (int, error) {
	n := 0
	for {
		changed, err := s.tickOne(ctx, time.Now().UTC())
		if err != nil {
			return n, err
		}
		if !changed {
			return n, nil
		}
		n++
	}
}

func (s Service) tickOne(ctx context.Context, now time.Time) (bool, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var before Record
	err = tx.QueryRow(ctx, `SELECT id::text,config_version,name,status,updated_at,pause_starts_at,pause_ends_at
 FROM brands WHERE pause_ends_at<=$1 OR (status='active' AND pause_starts_at<=$1)
 ORDER BY pause_starts_at,id LIMIT 1 FOR UPDATE SKIP LOCKED`, now).Scan(&before.BrandID, &before.Version, &before.Name, &before.Status, &before.UpdatedAt, &before.PauseStartsAt, &before.PauseEndsAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var changedBy, reason string
	err = tx.QueryRow(ctx, `SELECT changed_by::text,reason FROM brand_operation_revisions WHERE brand_id=$1 AND pause_starts_at=$2 AND pause_ends_at=$3 ORDER BY version DESC LIMIT 1`, before.BrandID, before.PauseStartsAt, before.PauseEndsAt).Scan(&changedBy, &reason)
	if err != nil {
		return false, err
	}
	in := Input{Version: before.Version, Status: "paused", Reason: reason, PauseStartsAt: before.PauseStartsAt, PauseEndsAt: before.PauseEndsAt}
	if !before.PauseEndsAt.After(now) {
		in.Status = "active"
		in.PauseStartsAt = nil
		in.PauseEndsAt = nil
	}
	_, err = saveTransition(ctx, tx, before, in, changedBy, points.Metadata{ActorType: "system", RequestID: "brand-pause:" + before.BrandID})
	if err != nil {
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}
