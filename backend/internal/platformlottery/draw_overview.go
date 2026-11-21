package platformlottery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

type DrawNumberAttribute struct {
	Wave    string `json:"wave"`
	Zodiac  string `json:"zodiac"`
	Element string `json:"element"`
}

type DrawHistoryItem struct {
	Attributes map[string]DrawNumberAttribute `json:"attributes"`
	PeriodNo   string                         `json:"period_no"`
	DrawAt     *time.Time                     `json:"draw_at"`
	DrawDate   *string                        `json:"draw_date"`
	Result     rules.Draw                     `json:"result"`
	Source     string                         `json:"source"`
}
type PendingDraw struct {
	Period
	State string `json:"state"`
}
type DrawOverview struct {
	LatestPeriodNo  *string           `json:"latest_period_no"`
	RecentSupported bool              `json:"recent_supported"`
	Recent          int               `json:"recent"`
	Daily           bool              `json:"daily"`
	Date            string            `json:"date"`
	Today           string            `json:"today"`
	Yesterday       string            `json:"yesterday"`
	MarkSix         bool              `json:"mark_six"`
	TotalCount      string            `json:"total_count"`
	Years           []string          `json:"years"`
	Year            string            `json:"year"`
	Items           []DrawHistoryItem `json:"items"`
	HasMore         bool              `json:"has_more"`
	Pending         []PendingDraw     `json:"pending"`
	PendingHasMore  bool              `json:"pending_has_more"`
}

// DrawOverview reads historical evidence without turning imported records into payable periods.
func (s Service) DrawOverview(ctx context.Context, game, year, date string, limit, offset, recent int) (DrawOverview, error) {
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
	var modelRaw []byte
	var model rules.Model
	var now time.Time
	err = tx.QueryRow(ctx, `SELECT (SELECT timezone FROM platform_settings WHERE id=true),clock_timestamp(),model,coalesce((SELECT v.interval_seconds<86400 FROM platform_automation_settings a JOIN platform_automation_versions v ON v.game_id=a.game_id AND v.version=a.version WHERE a.game_id=platform_games.id),false) FROM platform_games WHERE id=$1`, game).Scan(&zone, &now, &modelRaw, &out.Daily)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(modelRaw, &model); err != nil {
		return out, err
	}
	out.RecentSupported = rules.IsCanada28Model(model)
	if out.RecentSupported && recent == 0 && year == "" && date == "" {
		recent = 50
	}
	out.MarkSix = rules.ValidateMarkSixModel(model) == nil
	out.Daily = out.Daily || rules.IsCanada28Model(model)
	location, err := time.LoadLocation(zone)
	if err != nil {
		return out, err
	}
	current := now.In(location).Year()
	out.Today = now.In(location).Format("2006-01-02")
	out.Yesterday = now.In(location).AddDate(0, 0, -1).Format("2006-01-02")
	// Prefer imported evidence when the same period also exists in the operational table.
	const history = ` FROM (
 SELECT h.period_no,h.draw_date::text AS draw_date,h.draw_year AS year,h.result,h.source,h.attributes,NULL::timestamptz AS draw_at,NULL::timestamptz AS published_at FROM platform_draw_history h WHERE h.game_id=$1
 UNION ALL
 SELECT p.period_no,to_char(p.draw_at AT TIME ZONE $2,'YYYY-MM-DD'),extract(year FROM p.draw_at AT TIME ZONE $2)::int,p.result,'published',NULL::jsonb,p.draw_at,p.published_at FROM platform_game_periods p WHERE p.game_id=$1 AND p.result IS NOT NULL AND NOT EXISTS(SELECT 1 FROM platform_draw_history h WHERE h.game_id=p.game_id AND h.period_no=p.period_no)
 ) h`
	if err = tx.QueryRow(ctx, `SELECT (SELECT period_no`+history+` ORDER BY CASE WHEN period_no ~ '^[0-9]+$' THEN period_no::numeric END DESC NULLS LAST,period_no DESC LIMIT 1)`, game, zone).Scan(&out.LatestPeriodNo); err != nil {
		return out, err
	}
	var rows pgx.Rows
	var filter string
	var selection any
	countExpr := "count(*)::text"
	pageLimit := "$4"
	if recent != 0 {
		if !rules.IsCanada28Model(model) || (recent != 50 && recent != 100) || year != "" || date != "" {
			return out, ErrInvalid
		}
		out.Recent = recent
		filter = "$3::int>0"
		selection = recent
		countExpr = "least(count(*),$3::int)::text"
		pageLimit = "least($4,greatest(0,$3::int-$5))"
	} else if out.Daily {
		if year != "" {
			return out, ErrInvalid
		}
		if date == "" {
			date = out.Today
		}
		parsed, e := time.Parse("2006-01-02", date)
		if e != nil || parsed.Format("2006-01-02") != date || date > out.Today {
			return out, ErrInvalid
		}
		out.Date = date
		filter = "draw_date=$3"
		selection = date
	} else {
		if date != "" {
			return out, ErrInvalid
		}
		rows, err = tx.Query(ctx, `SELECT DISTINCT year`+history+` ORDER BY year DESC`, game, zone)
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
		y, e := strconv.Atoi(year)
		if e != nil || strconv.Itoa(y) != year || y < 1 || y > current {
			return out, ErrInvalid
		}
		out.Year = year
		filter = "year=$3"
		selection = y
	}
	if err := tx.QueryRow(ctx, `SELECT `+countExpr+history+` WHERE `+filter, game, zone, selection).Scan(&out.TotalCount); err != nil {
		return out, err
	}

	// Resolve rule attributes only for the requested page and only for Mark Six.
	rows, err = tx.Query(ctx, `WITH page AS MATERIALIZED (
 SELECT *`+history+` WHERE `+filter+` ORDER BY CASE WHEN period_no ~ '^[0-9]+$' THEN period_no::numeric END DESC NULLS LAST,period_no DESC LIMIT `+pageLimit+` OFFSET $5
 ) SELECT period_no,draw_date,draw_at,result,source,attributes,
 CASE WHEN $6 THEN (SELECT v.snapshot->'bindings'->'attributes' FROM configured_rule_versions v WHERE v.game_id=$1 AND v.status IN ('published','retired') AND v.created_at<=page.published_at ORDER BY v.created_at DESC,v.id DESC LIMIT 1) END
 FROM page ORDER BY CASE WHEN period_no ~ '^[0-9]+$' THEN period_no::numeric END DESC NULLS LAST,period_no DESC`, game, zone, selection, limit+1, offset, out.MarkSix)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var item DrawHistoryItem
		var raw, attributesRaw, bindingsRaw []byte
		if err = rows.Scan(&item.PeriodNo, &item.DrawDate, &item.DrawAt, &raw, &item.Source, &attributesRaw, &bindingsRaw); err != nil {
			rows.Close()
			return out, err
		}
		if err = json.Unmarshal(raw, &item.Result); err != nil {
			rows.Close()
			return out, err
		}
		item.Attributes = map[string]DrawNumberAttribute{}
		if len(attributesRaw) > 0 {
			if err = json.Unmarshal(attributesRaw, &item.Attributes); err != nil {
				rows.Close()
				return out, err
			}
		}
		if out.MarkSix && len(bindingsRaw) > 0 {
			var bindings map[string]map[string][]int
			if err = json.Unmarshal(bindingsRaw, &bindings); err != nil {
				rows.Close()
				return out, err
			}
			for _, number := range append(append([]int{}, item.Result.Regular...), item.Result.Special...) {
				a := DrawNumberAttribute{}
				for group, labels := range bindings {
					for label, numbers := range labels {
						for _, n := range numbers {
							if n == number {
								switch group {
								case "wave":
									a.Wave = label
								case "zodiac":
									a.Zodiac = label
								case "element":
									a.Element = label
								}
							}
						}
					}
				}
				item.Attributes[fmt.Sprintf("%02d", number)] = a
			}
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
