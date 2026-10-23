package betting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

type ArchiveNumberAttribute struct {
	Wave    string `json:"wave"`
	Zodiac  string `json:"zodiac"`
	Element string `json:"element"`
}
type DrawArchiveRow struct {
	PeriodNo    string                            `json:"period_no"`
	Result      rules.Draw                        `json:"result"`
	DrawnAt     time.Time                         `json:"drawn_at"`
	Attributes  map[string]ArchiveNumberAttribute `json:"attributes"`
	periodID    string
	scheduledAt time.Time
}
type DrawArchive struct {
	MarkSix  bool             `json:"mark_six"`
	GameID   string           `json:"game_id"`
	GameName string           `json:"game_name"`
	Timezone string           `json:"timezone"`
	Daily    bool             `json:"daily"`
	Choices  []string         `json:"choices"`
	Selected string           `json:"selected"`
	Items    []DrawArchiveRow `json:"items"`
	HasMore  bool             `json:"has_more"`
}

// DrawArchive exposes only the selected game's current published results.
func (s Service) DrawArchive(ctx context.Context, brand, game, kind, selected string, offset int) (DrawArchive, error) {
	out := DrawArchive{GameID: game, Choices: []string{}, Items: []DrawArchiveRow{}}
	if !validIDs(game) || (kind != "standard" && kind != "configured") || offset < 0 || offset > 1000000 {
		return out, ErrInvalid
	}
	tx, _, now, err := s.catalogTx(ctx, brand)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	var fast, configured bool
	var modelRaw []byte
	var model rules.Model
	if kind == "configured" {
		err = tx.QueryRow(ctx, `SELECT g.model,g.name,(SELECT timezone FROM platform_settings WHERE id=true),coalesce((SELECT v.interval_seconds<86400 FROM platform_automation_settings a JOIN platform_automation_versions v ON v.game_id=a.game_id AND v.version=a.version WHERE a.game_id=g.id),false) FROM platform_games g JOIN brand_platform_games b ON b.game_id=g.id AND b.brand_id=$1 WHERE g.id=$2`, brand, game).Scan(&modelRaw, &out.GameName, &out.Timezone, &fast)
	} else {
		err = tx.QueryRow(ctx, `SELECT g.model,g.name,(SELECT timezone FROM platform_settings WHERE id=true),coalesce((SELECT s.spec->>'mode'='interval' FROM game_schedules s WHERE s.id=g.schedule_id),false) FROM games g WHERE g.brand_id=$1 AND g.id=$2 AND g.status IN ('active','paused')`, brand, game).Scan(&modelRaw, &out.GameName, &out.Timezone, &fast)
	}
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
	if kind == "configured" && out.MarkSix {
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM configured_rule_heads WHERE game_id=$1)`, game).Scan(&configured); err != nil {
			return out, err
		}
	}
	location, err := time.LoadLocation(out.Timezone)
	if err != nil {
		return out, err
	}
	// Source-scheduled games also count as daily archives when actual draws occur multiple times a day.
	from := ` FROM platform_game_periods p WHERE p.game_id=$2 AND p.result IS NOT NULL AND p.draw_at<=$4 AND EXISTS(SELECT 1 FROM brand_platform_games b WHERE b.brand_id=$1 AND b.game_id=p.game_id)`
	resultFields := `p.period_no,p.result,p.published_at,p.id::text,p.draw_at`
	if kind == "standard" {
		from = publicDrawJoin + ` WHERE ` + `p.brand_id=$1 AND p.game_id=$2 AND g.status IN ('active','paused') AND p.bet_start_at<=$4 AND p.status IN ('drawn','settling','settled','bet_cancelled','judged_cancelled')`
		resultFields = `p.period_no,r.result,r.drawn_at,p.id::text,p.draw_at`
	}
	dates, err := tx.Query(ctx, `SELECT to_char(p.draw_at AT TIME ZONE $3,'YYYY-MM-DD'),count(*)`+from+` GROUP BY 1 ORDER BY 1 DESC`, brand, game, out.Timezone, now)
	if err != nil {
		return out, err
	}
	var days []string
	for dates.Next() {
		var date string
		var count int
		if err = dates.Scan(&date, &count); err != nil {
			dates.Close()
			return out, err
		}
		days = append(days, date)
		if count > 1 {
			fast = true
		}
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
	start, err := time.ParseInLocation(layout, selected, location)
	if err != nil || start.Format(layout) != selected || selected > current {
		return out, ErrInvalid
	}
	out.Selected = selected
	end := start.AddDate(1, 0, 0)
	if fast {
		end = start.AddDate(0, 0, 1)
	}
	rows, err := tx.Query(ctx, `SELECT `+resultFields+strings.ReplaceAll(from, "$4", "$3")+` AND p.draw_at>=$4 AND p.draw_at<$5 ORDER BY CASE WHEN p.period_no ~ '^[0-9]+$' THEN p.period_no::numeric END DESC NULLS LAST,p.period_no DESC,p.id DESC LIMIT 51 OFFSET $6`, brand, game, now, start, end, offset)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var row DrawArchiveRow
		var raw []byte
		if err = rows.Scan(&row.PeriodNo, &raw, &row.DrawnAt, &row.periodID, &row.scheduledAt); err != nil {
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
		out.Items = append(out.Items, row)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > 50 {
		out.HasMore = true
		out.Items = out.Items[:50]
	}
	if out.MarkSix && kind == "configured" {
		for i := range out.Items {
			row := &out.Items[i]
			if configured {
				var raw []byte
				err = tx.QueryRow(ctx, `SELECT v.snapshot->'bindings'->'attributes' FROM configured_rule_versions v JOIN audit_logs a ON a.resource_id=v.id AND a.action='configured_rule.publish' WHERE v.game_id=$1 AND a.created_at<=$2 ORDER BY a.created_at DESC,a.id DESC LIMIT 1`, game, row.scheduledAt).Scan(&raw)
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
			} else {
				attributes, e := platformlottery.ReadPeriodMarkSixAttributes(ctx, tx, row.periodID)
				if errors.Is(e, platformlottery.ErrAttributesMissing) {
					continue
				}
				if e != nil {
					return out, e
				}
				for key, wave := range attributes.Waves {
					row.Attributes[key] = ArchiveNumberAttribute{Wave: wave, Zodiac: attributes.Zodiac[key], Element: attributes.Elements[key]}
				}
			}
		}
	}
	return out, tx.Commit(ctx)
}
