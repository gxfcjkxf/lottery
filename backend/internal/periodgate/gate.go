// Package periodgate enforces the settlement/refund boundary between periods.
package periodgate

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrBlocked = errors.New("prior period has unfinished settlement or refunds")

// Check must run under a lock on the same game: exclusive for opening and
// shared for betting. This serializes opening/correction with admissions, while
// settlements may finish concurrently; an older snapshot can only delay opening.
// Skipped, unused pending periods have no cancellation job and nothing to refund.
// Sequence identifies the candidate, not chronology: future calendar reservations
// may have been created before an earlier scheduled draw. Every begun nonterminal
// sibling blocks, regardless of creation order or planned draw time.
func Check(ctx context.Context, tx pgx.Tx, brand, game string, sequence int64, drawAt time.Time) error {
	var blocked bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(
 SELECT 1 FROM periods p WHERE p.brand_id=$1 AND p.game_id=$2 AND p.sequence<>$3
 AND (p.status<>'pending' OR p.draw_at<$4 OR (p.draw_at=$4 AND p.sequence<$3))
 AND NOT (
   p.status='settled' OR
   (p.status IN ('bet_cancelled','judged_cancelled') AND (
     EXISTS(SELECT 1 FROM period_cancellations c WHERE c.period_id=p.id AND c.state='completed') OR
     (NOT EXISTS(SELECT 1 FROM period_cancellations c WHERE c.period_id=p.id) AND NOT EXISTS(SELECT 1 FROM bet_orders o WHERE o.period_id=p.id))
   ))
 ))`, brand, game, sequence, drawAt).Scan(&blocked)
	if err != nil {
		return err
	}
	if blocked {
		return ErrBlocked
	}
	return nil
}
