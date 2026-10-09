// Package drawnotice snapshots the unique betting audience when a result
// becomes the published period pointer. It never settles orders or moves points.
package drawnotice

import (
	"context"
	"errors"
	"regexp"

	"github.com/jackc/pgx/v5"
)

var uuid = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var ErrInvalid = errors.New("invalid published draw notification context")

// Append runs after the publication audit in the caller's transaction and
// under its existing period lock. All earlier orders count, including cancelled
// and abnormal orders; multiple orders never create duplicate member messages.
func Append(ctx context.Context, tx pgx.Tx, brand, period, draw, auditID string) error {
	if tx == nil || !uuid.MatchString(brand) || !uuid.MatchString(period) || !uuid.MatchString(draw) || !uuid.MatchString(auditID) {
		return ErrInvalid
	}
	cmd, err := tx.Exec(ctx, `INSERT INTO draw_notification_publications(draw_result_id,brand_id,game_id,period_id,event_type,period_no,result,drawn_at,previous_draw_id,audit_log_id)
 SELECT d.id,d.brand_id,d.game_id,d.period_id,CASE WHEN d.corrected_from_id IS NULL THEN 'draw.result.published' ELSE 'draw.result.corrected' END,
 p.period_no,d.result,d.drawn_at,d.corrected_from_id,$4 FROM draw_results d JOIN periods p ON p.brand_id=d.brand_id AND p.id=d.period_id
 WHERE d.brand_id=$1 AND d.period_id=$2 AND d.id=$3 AND p.draw_result_id=d.id ON CONFLICT(draw_result_id) DO NOTHING`, brand, period, draw, auditID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrInvalid
	}
	if _, err = tx.Exec(ctx, `INSERT INTO draw_notification_recipients(id,brand_id,draw_result_id,member_id)
 SELECT gen_random_uuid(),$1,$2,brand_member_id FROM bet_orders WHERE brand_id=$1 AND period_id=$3 GROUP BY brand_member_id`, brand, draw, period); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,brand_id,event_type,aggregate_id,payload)
 SELECT gen_random_uuid(),r.brand_id,p.event_type,r.id,jsonb_build_object('member_id',r.member_id::text,'resource_id',p.draw_result_id::text,
 'publication_id',p.draw_result_id::text,'recipient_id',r.id::text)
 FROM draw_notification_recipients r JOIN draw_notification_publications p ON p.brand_id=r.brand_id AND p.draw_result_id=r.draw_result_id
 WHERE r.brand_id=$1 AND r.draw_result_id=$2`, brand, draw)
	return err
}
