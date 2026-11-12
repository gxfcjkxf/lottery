package betting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

type ArchiveNumberAttribute struct {
	Wave    string `json:"wave"`
	Zodiac  string `json:"zodiac"`
	Element string `json:"element"`
}
type DrawArchiveRow struct {
	PeriodNo     string                            `json:"period_no"`
	Result       rules.Draw                        `json:"result"`
	DrawnAt      *time.Time                        `json:"drawn_at"`
	RegularOrder []int                             `json:"regular_order"`
	DrawDate     *string                           `json:"draw_date"`
	Attributes   map[string]ArchiveNumberAttribute `json:"attributes"`
	periodID     string
	scheduledAt  *time.Time
}
type DrawArchive struct {
	TotalCount string           `json:"total_count"`
	MarkSix    bool             `json:"mark_six"`
	GameID     string           `json:"game_id"`
	GameName   string           `json:"game_name"`
	Timezone   string           `json:"timezone"`
	Daily      bool             `json:"daily"`
	Choices    []string         `json:"choices"`
	Selected   string           `json:"selected"`
	Items      []DrawArchiveRow `json:"items"`
	HasMore    bool             `json:"has_more"`
}

// DrawArchive exposes only the selected game's current published results.
func (s Service) DrawArchive(ctx context.Context, brand, game, kind, selected string, limit, offset int) (DrawArchive, error) {
	out := DrawArchive{TotalCount: "0", GameID: game, Choices: []string{}, Items: []DrawArchiveRow{}}
	if !validIDs(game) || kind != "configured" || limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return out, ErrInvalid
	}
	tx, _, now, err := s.catalogTx(ctx, brand)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	var fast bool
	var modelRaw []byte
	var model rules.Model
	err = tx.QueryRow(ctx, `SELECT g.model,g.name,(SELECT timezone FROM platform_settings WHERE id=true),coalesce((SELECT v.interval_seconds<86400 FROM platform_automation_settings a JOIN platform_automation_versions v ON v.game_id=a.game_id AND v.version=a.version WHERE a.game_id=g.id),false) FROM platform_games g JOIN brand_platform_games b ON b.game_id=g.id AND b.brand_id=$1 WHERE g.id=$2`, brand, game).Scan(&modelRaw, &out.GameName, &out.Timezone, &fast)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(modelRaw, &model); err != nil {
		return out, err
	}
	out.MarkSix = rules.ValidateMarkSixModel(model) == nil
	location, err := time.LoadLocation(out.Timezone)
	if err != nil {
		return out, err
	}
	// Imported history carries a calendar year and an optional date, never a fabricated timestamp.
	from := ` FROM (
 SELECT p.game_id,p.period_no,p.result,p.published_at AS drawn_at,p.id::text AS period_id,p.draw_at AS scheduled_at,
 NULL::text AS draw_date,to_char(p.draw_at AT TIME ZONE $3,'YYYY-MM-DD') AS calendar_date,to_char(p.draw_at AT TIME ZONE $3,'YYYY') AS archive_year,NULL::jsonb AS attributes,NULL::jsonb AS regular_order
 FROM platform_game_periods p WHERE p.game_id=$2 AND p.result IS NOT NULL AND p.draw_at<=$4
 AND NOT EXISTS (SELECT 1 FROM platform_draw_history h WHERE h.game_id=p.game_id AND h.period_no=p.period_no)
 UNION ALL
 SELECT h.game_id,h.period_no,h.result,NULL::timestamptz,''::text,NULL::timestamptz,h.draw_date::text,h.draw_date::text,h.draw_year::text,h.attributes,h.regular_order
 FROM platform_draw_history h WHERE h.game_id=$2 AND h.draw_year<=extract(year FROM $4 AT TIME ZONE $3)
 AND (h.draw_date IS NULL OR h.draw_date<=($4 AT TIME ZONE $3)::date)
 ) p WHERE p.game_id=$2 AND EXISTS(SELECT 1 FROM brand_platform_games b WHERE b.brand_id=$1 AND b.game_id=p.game_id)`
	resultFields := `p.period_no,p.result,p.drawn_at,p.period_id,p.scheduled_at,p.draw_date,p.attributes,p.regular_order`
	bucket := `p.archive_year`
	orderID := `p.period_id`
	if fast {
		bucket = `p.calendar_date`
	}

	dates, err := tx.Query(ctx, `SELECT DISTINCT `+bucket+from+` ORDER BY 1 DESC`, brand, game, out.Timezone, now)
	if err != nil {
		return out, err
	}
	var days []string
	for dates.Next() {
		var date string
		if err = dates.Scan(&date); err != nil {
			dates.Close()
			return out, err
		}
		days = append(days, date)
	}
	err = dates.Err()
	dates.Close()
	if err != nil {
		return out, err
	}
	out.Daily = fast
	layout := "2006"
	if fast {
		layout = "2006-01-02"
	}
	current := now.In(location).Format(layout)
	out.Choices = append(out.Choices, current)
	seen := map[string]bool{current: true}
	for _, date := range days {
		value := date
		if !fast {
			value = date[:4]
		}
		if !seen[value] {
			out.Choices = append(out.Choices, value)
			seen[value] = true
		}
	}
	if selected == "" {
		selected = current
	}
	parsed, err := time.ParseInLocation(layout, selected, location)
	if err != nil || parsed.Format(layout) != selected || selected > current {
		return out, ErrInvalid
	}
	out.Selected = selected
	if err := tx.QueryRow(ctx, `SELECT count(*)::text`+from+` AND `+bucket+`=$5`, brand, game, out.Timezone, now, selected).Scan(&out.TotalCount); err != nil {
		return out, err
	}

	rows, err := tx.Query(ctx, `SELECT `+resultFields+from+` AND `+bucket+`=$5 ORDER BY CASE WHEN p.period_no ~ '^[0-9]+$' THEN p.period_no::numeric END DESC NULLS LAST,p.period_no DESC,`+orderID+` DESC LIMIT $7 OFFSET $6`, brand, game, out.Timezone, now, selected, offset, limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var row DrawArchiveRow
		var raw, attributesRaw, orderRaw []byte
		if err = rows.Scan(&row.PeriodNo, &raw, &row.DrawnAt, &row.periodID, &row.scheduledAt, &row.DrawDate, &attributesRaw, &orderRaw); err != nil {
			rows.Close()
			return out, err
		}
		if err = json.Unmarshal(raw, &row.Result); err != nil {
			rows.Close()
			return out, err
		}
		if row.Result.Regular == nil {
			row.Result.Regular = []int{}
		}
		if row.Result.Special == nil {
			row.Result.Special = []int{}
		}
		if row.Result.Digits == nil {
			row.Result.Digits = []int{}
		}
		row.Attributes = map[string]ArchiveNumberAttribute{}
		if len(attributesRaw) > 0 {
			if err = json.Unmarshal(attributesRaw, &row.Attributes); err != nil {
				rows.Close()
				return out, err
			}
		}
		if len(orderRaw) > 0 {
			if err = json.Unmarshal(orderRaw, &row.RegularOrder); err != nil {
				rows.Close()
				return out, err
			}
		}
		if row.periodID != "" && out.MarkSix && model.Ordered {
			row.RegularOrder = append([]int{}, row.Result.Regular...)
		}
		out.Items = append(out.Items, row)
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
	if out.MarkSix {
		for i := range out.Items {
			row := &out.Items[i]
			if row.periodID == "" {
				continue
			}
			{
				var raw []byte
				err = tx.QueryRow(ctx, `SELECT v.snapshot->'bindings'->'attributes' FROM configured_rule_versions v WHERE v.game_id=$1 AND v.status IN ('published','retired') AND v.created_at<=$2 ORDER BY v.created_at DESC,v.id DESC LIMIT 1`, game, row.DrawnAt).Scan(&raw)
				if errors.Is(err, pgx.ErrNoRows) {
					continue
				}
				if err != nil {
					return out, err
				}
				var bindings map[string]map[string][]int
				if err = json.Unmarshal(raw, &bindings); err != nil {
					return out, err
				}
				for _, number := range append(append([]int{}, row.Result.Regular...), row.Result.Special...) {
					key := fmt.Sprintf("%02d", number)
					a := ArchiveNumberAttribute{}
					for group, values := range bindings {
						for label, numbers := range values {
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
					row.Attributes[key] = a
				}
			}
		}
	}
	return out, tx.Commit(ctx)
}

// LatestConfiguredDraw reads the same published evidence as the player archive,
// including a prior year when the current year has no completed draw yet.
func (s Service) LatestConfiguredDraw(ctx context.Context, brand, game string) (*DrawArchiveRow, error) {
	if !validIDs(game) {
		return nil, ErrInvalid
	}
	tx, _, now, err := s.catalogTx(ctx, brand)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var zone string
	var daily bool
	err = tx.QueryRow(ctx, `SELECT (SELECT timezone FROM platform_settings WHERE id=true),coalesce((SELECT v.interval_seconds<86400 FROM platform_automation_settings a JOIN platform_automation_versions v ON v.game_id=a.game_id AND v.version=a.version WHERE a.game_id=g.id),false) FROM platform_games g JOIN brand_platform_games b ON b.game_id=g.id WHERE b.brand_id=$1 AND g.id=$2`, brand, game).Scan(&zone, &daily)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	format := "YYYY"
	historicalBucket := `h.draw_year::text`
	if daily {
		format = "YYYY-MM-DD"
		historicalBucket = `h.draw_date::text`
	}
	var bucket *string
	err = tx.QueryRow(ctx, `SELECT max(bucket) FROM (
 SELECT to_char(p.draw_at AT TIME ZONE $2,$4) AS bucket FROM platform_game_periods p WHERE p.game_id=$1 AND p.result IS NOT NULL AND p.draw_at<=$3
 UNION ALL
 SELECT `+historicalBucket+` FROM platform_draw_history h WHERE h.game_id=$1 AND h.draw_year<=extract(year FROM $3 AT TIME ZONE $2) AND (h.draw_date IS NULL OR h.draw_date<=($3 AT TIME ZONE $2)::date)
 ) draws`, game, zone, now, format).Scan(&bucket)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	if bucket == nil {
		return nil, nil
	}
	archive, err := s.DrawArchive(ctx, brand, game, "configured", *bucket, 50, 0)
	if err != nil {
		return nil, err
	}
	if len(archive.Items) == 0 {
		return nil, nil
	}
	return &archive.Items[0], nil
}
