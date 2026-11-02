package platformlottery

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

type DrawHistoryItem struct {
	PeriodNo string     `json:"period_no"`
	DrawDate *string    `json:"draw_date"`
	Result   rules.Draw `json:"result"`
	Source   string     `json:"source"`
}
type PendingDraw struct {
	Period
	State string `json:"state"`
}
type DrawOverview struct {
	TotalCount     string            `json:"total_count"`
	Years          []string          `json:"years"`
	Year           string            `json:"year"`
	Items          []DrawHistoryItem `json:"items"`
	HasMore        bool              `json:"has_more"`
	Pending        []PendingDraw     `json:"pending"`
	PendingHasMore bool              `json:"pending_has_more"`
}

// DrawOverview reads historical evidence without turning imported records into payable periods.
func (s Service) DrawOverview(ctx context.Context, game, year string, limit, offset int) (DrawOverview, error) {
	out := DrawOverview{Years: []string{}, Items: []DrawHistoryItem{}, Pending: []PendingDraw{}}
	if limit < 1 || limit > 100 || offset < 0 {
		return out, ErrInvalid
	}
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	var zone string
	var now time.Time
	err = tx.QueryRow(ctx, `SELECT (SELECT timezone FROM platform_settings WHERE id=true),clock_timestamp() FROM platform_games WHERE id=$1`, game).Scan(&zone, &now)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return out, err
	}
	current := now.In(location).Year()
	// Prefer imported evidence when the same period also exists in the operational table.
	const history = ` FROM (
 SELECT h.period_no,h.draw_date::text AS draw_date,h.draw_year AS year,h.result,h.source FROM platform_draw_history h WHERE h.game_id=$1
 UNION ALL
 SELECT p.period_no,to_char(p.draw_at AT TIME ZONE $2,'YYYY-MM-DD'),extract(year FROM p.draw_at AT TIME ZONE $2)::int,p.result,'published' FROM platform_game_periods p WHERE p.game_id=$1 AND p.result IS NOT NULL AND NOT EXISTS(SELECT 1 FROM platform_draw_history h WHERE h.game_id=p.game_id AND h.period_no=p.period_no)
 ) h`
	rows, err := tx.Query(ctx, `SELECT DISTINCT year`+history+` ORDER BY year DESC`, game, zone)
	if err != nil {
		return out, err
	}
	out.Years = append(out.Years, strconv.Itoa(current))
	for rows.Next() {
		var y int
		if err = rows.Scan(&y); err != nil {
			rows.Close()
			return out, err
		}
		if y != current {
			out.Years = append(out.Years, strconv.Itoa(y))
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if year == "" {
		year = strconv.Itoa(current)
	}
	y, err := strconv.Atoi(year)
	if err != nil || strconv.Itoa(y) != year || y < 1 || y > current {
		return out, ErrInvalid
	}
	out.Year = year
	if err := tx.QueryRow(ctx, `SELECT count(*)::text`+history+` WHERE year=$3`, game, zone, y).Scan(&out.TotalCount); err != nil {
		return out, err
	}

	rows, err = tx.Query(ctx, `SELECT period_no,draw_date,result,source`+history+` WHERE year=$3 ORDER BY CASE WHEN period_no ~ '^[0-9]+$' THEN period_no::numeric END DESC NULLS LAST,period_no DESC LIMIT $4 OFFSET $5`, game, zone, y, limit+1, offset)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var item DrawHistoryItem
		var raw []byte
		if err = rows.Scan(&item.PeriodNo, &item.DrawDate, &raw, &item.Source); err != nil {
			rows.Close()
			return out, err
		}
		if err = json.Unmarshal(raw, &item.Result); err != nil {
			rows.Close()
			return out, err
		}
		out.Items = append(out.Items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.HasMore = true
		out.Items = out.Items[:limit]
	}
	rows, err = tx.Query(ctx, `SELECT p.id::text,p.game_id::text,p.period_no,p.bet_start_at,p.bet_end_at,p.draw_at,CASE WHEN p.draw_at<=$2 THEN 'waiting' WHEN p.bet_start_at>$2 THEN 'scheduled' WHEN p.bet_end_at<=$2 THEN 'closed' ELSE 'open' END FROM platform_game_periods p WHERE p.game_id=$1 AND p.result IS NULL ORDER BY p.draw_at,p.id LIMIT 51`, game, now)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var item PendingDraw
		if err = rows.Scan(&item.ID, &item.GameID, &item.PeriodNo, &item.BetStartAt, &item.BetEndAt, &item.DrawAt, &item.State); err != nil {
			rows.Close()
			return out, err
		}
		out.Pending = append(out.Pending, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Pending) > 50 {
		out.PendingHasMore = true
		out.Pending = out.Pending[:50]
	}
	return out, tx.Commit(ctx)
}
