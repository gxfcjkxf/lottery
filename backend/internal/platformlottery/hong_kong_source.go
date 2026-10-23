package platformlottery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type hkPayload struct {
	PreIssue   int64  `json:"preDrawIssue"`
	NextIssue  int64  `json:"drawIssue"`
	PreTime    string `json:"preDrawTime"`
	NextTime   string `json:"drawTime"`
	Numbers    string `json:"preDrawCode"`
	Zodiac     []int  `json:"chineseZodiac"`
	Elements   []int  `json:"fiveElements"`
	Colors     []int  `json:"color"`
	ServerTime string `json:"serverTime"`
	Type       *int   `json:"type"`
	LotCode    int    `json:"lotCode"`
}
type hkReading struct {
	NextNo    string
	NextAt    time.Time
	Completed *SourceObservedDraw
}

var hkLocation = time.FixedZone("Asia/Hong_Kong", 8*60*60)

func hkTime(v string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02 15:04:05", v, hkLocation)
}
func parseHongKong(body []byte, now time.Time) (hkReading, error) {
	var envelope struct {
		ErrorCode *int `json:"errorCode"`
		Result    *struct {
			BusinessCode *int       `json:"businessCode"`
			Data         *hkPayload `json:"data"`
		} `json:"result"`
	}
	out := hkReading{}
	if json.Unmarshal(body, &envelope) != nil || envelope.ErrorCode == nil || *envelope.ErrorCode != 0 || envelope.Result == nil || envelope.Result.BusinessCode == nil || *envelope.Result.BusinessCode != 0 || envelope.Result.Data == nil {
		return out, ErrInvalid
	}
	d := envelope.Result.Data
	server, e := hkTime(d.ServerTime)
	if e != nil || server.Before(now.Add(-5*time.Minute)) || server.After(now.Add(10*time.Second)) || d.Type == nil || d.LotCode != 10048 {
		return out, ErrInvalid
	}
	next, e := hkTime(d.NextTime)
	if e != nil || len(strconv.FormatInt(d.NextIssue, 10)) != 7 || d.NextIssue/1000 != int64(next.Year()) || d.NextIssue%1000 < 1 {
		return out, ErrInvalid
	}
	out.NextNo = strconv.FormatInt(d.NextIssue, 10)
	out.NextAt = next.UTC()
	switch *d.Type {
	case 0, 1, 2, 3, 4, 6:
		return out, nil
	case 8:
	default:
		return out, ErrInvalid
	}
	drawn, e := hkTime(d.PreTime)
	if e != nil || drawn.After(server) || len(strconv.FormatInt(d.PreIssue, 10)) != 7 || d.PreIssue/1000 != int64(drawn.Year()) || d.PreIssue%1000 < 1 || d.NextIssue <= d.PreIssue || !next.After(drawn) {
		return out, ErrInvalid
	}
	numbers := strings.Split(d.Numbers, ",")
	if len(numbers) != 7 || len(d.Colors) != 7 || len(d.Zodiac) != 7 || len(d.Elements) != 7 {
		return out, ErrInvalid
	}
	draw := rules.Draw{Regular: []int{}, Special: []int{}, Digits: []int{}}
	attrs := DrawAttributes{Waves: map[string]string{}, Zodiac: map[string]string{}, Elements: map[string]string{}}
	waves := []string{"", "red", "green", "blue"}
	zodiac := []string{"", "rat", "ox", "tiger", "rabbit", "dragon", "snake", "horse", "goat", "monkey", "rooster", "dog", "pig"}
	elements := []string{"", "metal", "wood", "water", "fire", "earth"}
	seen := map[int]bool{}
	for i, v := range numbers {
		n, e := strconv.Atoi(v)
		if e != nil || n < 1 || n > 49 || seen[n] || d.Colors[i] < 1 || d.Colors[i] > 3 || d.Zodiac[i] < 1 || d.Zodiac[i] > 12 || d.Elements[i] < 1 || d.Elements[i] > 5 {
			return out, ErrInvalid
		}
		seen[n] = true
		if i < 6 {
			draw.Regular = append(draw.Regular, n)
		} else {
			draw.Special = append(draw.Special, n)
		}
		key := fmt.Sprintf("%02d", n)
		attrs.Waves[key] = waves[d.Colors[i]]
		attrs.Zodiac[key] = zodiac[d.Zodiac[i]]
		attrs.Elements[key] = elements[d.Elements[i]]
	}
	out.Completed = &SourceObservedDraw{PeriodNo: strconv.FormatInt(d.PreIssue, 10), DrawnAt: drawn.UTC(), Result: draw, Attributes: &attrs}
	return out, nil
}
func (s Service) SyncHongKongSources(ctx context.Context, sources AutomationSources) (int, error) {
	rows, e := s.DB.Query(ctx, `SELECT v.game_id::text,v.version FROM platform_automation_versions v JOIN platform_games g ON g.id=v.game_id LEFT JOIN platform_automation_settings current ON current.game_id=v.game_id LEFT JOIN platform_hk_source_checks c ON c.game_id=v.game_id AND c.version=v.version WHERE v.source_type='hk_marksix_6hch' AND ((v.enabled AND g.status='active' AND current.version=v.version) OR EXISTS(SELECT 1 FROM platform_game_periods p WHERE p.game_id=v.game_id AND p.automation_version=v.version AND p.result IS NULL AND p.draw_at<=clock_timestamp())) AND (c.checked_at IS NULL OR c.checked_at<clock_timestamp()-CASE WHEN c.next_draw_at<clock_timestamp()+interval '2 minutes' OR c.code<>'ready' THEN interval '5 seconds' ELSE interval '60 seconds' END) ORDER BY v.game_id,v.version LIMIT 12`)
	if e != nil {
		return 0, e
	}
	type key struct {
		game    string
		version int64
	}
	keys := []key{}
	for rows.Next() {
		var k key
		if e = rows.Scan(&k.game, &k.version); e != nil {
			rows.Close()
			return 0, e
		}
		keys = append(keys, k)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return 0, e
	}
	count := 0
	for _, k := range keys {
		n, e := s.syncHongKong(ctx, sources, k.game, k.version)
		if e != nil {
			return count, e
		}
		count += n
	}
	return count, nil
}
func (s Service) syncHongKong(ctx context.Context, sources AutomationSources, game string, version int64) (int, error) {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback(ctx)
	var acquired bool
	if e = tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,0))`, fmt.Sprintf("hk-source:%s:%d", game, version)).Scan(&acquired); e != nil || !acquired {
		return 0, e
	}
	var due bool
	if e = tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM platform_hk_source_checks c WHERE c.game_id=$1 AND c.version=$2 AND c.checked_at>clock_timestamp()-CASE WHEN c.next_draw_at<clock_timestamp()+interval '2 minutes' OR c.code<>'ready' THEN interval '5 seconds' ELSE interval '60 seconds' END)`, game, version).Scan(&due); e != nil || !due {
		return 0, e
	}
	plan, e := scanAutomation(tx.QueryRow(ctx, `SELECT `+automationPlanFields+` FROM platform_automation_versions v WHERE game_id=$1 AND version=$2`, game, version))
	if e != nil {
		return 0, e
	}
	code := "source_error"
	var reading hkReading
	if sources.Allows(plan.SourceURL) {
		req, e := http.NewRequestWithContext(ctx, "GET", plan.SourceURL, nil)
		if e != nil {
			return 0, e
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Cache-Control", "no-cache")
		resp, e := sources.Client.Do(req)
		if e == nil {
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, 65537))
			resp.Body.Close()
			if resp.StatusCode == 200 && readErr == nil {
				code = "invalid_result"
				if len(body) <= 65536 {
					reading, e = parseHongKong(body, time.Now())
					if e == nil {
						code = "ready"
						if reading.Completed == nil {
							code = "waiting_result"
						}
					}
				}
			}
		}
	}
	count := 0
	publishedID := ""
	if code == "ready" || code == "waiting_result" {
		g, e := lockedGame(ctx, tx, game)
		if e != nil {
			return 0, e
		}
		var actor string
		var current bool
		if e = tx.QueryRow(ctx, `SELECT created_by::text,EXISTS(SELECT 1 FROM platform_automation_settings s WHERE s.game_id=v.game_id AND s.version=v.version) FROM platform_automation_versions v WHERE game_id=$1 AND version=$2`, game, version).Scan(&actor, &current); e != nil {
			return 0, e
		}
		if reading.Completed != nil {
			var id string
			var scheduled time.Time
			var attributes []byte
			e = tx.QueryRow(ctx, `SELECT p.id::text,p.draw_at,to_jsonb(a) FROM platform_game_periods p JOIN platform_mark_six_period_attributes pa ON pa.period_id=p.id JOIN platform_mark_six_attributes a ON a.id=pa.attribute_id WHERE p.game_id=$1 AND p.automation_version=$2 AND p.period_no=$3 AND p.result IS NULL AND p.draw_at<=clock_timestamp() FOR UPDATE OF p`, game, version, reading.Completed.PeriodNo).Scan(&id, &scheduled, &attributes)
			if e != nil && !errors.Is(e, pgx.ErrNoRows) {
				return 0, e
			}
			if e == nil {
				var a MarkSixAttributes
				if e = json.Unmarshal(attributes, &a); e != nil {
					return 0, e
				}
				valid := !reading.Completed.DrawnAt.Before(scheduled)
				for key, wave := range reading.Completed.Attributes.Waves {
					if a.Waves[key] != wave || a.Zodiac[key] != reading.Completed.Attributes.Zodiac[key] || a.Elements[key] != reading.Completed.Attributes.Elements[key] {
						valid = false
					}
				}
				if valid {
					if e = publishAutomaticResult(ctx, tx, id, actor, reading.Completed, "platform_game.hk.result", "Completed Hong Kong source result matched period and purchased annual attributes"); e != nil {
						return 0, e
					}
					count++
					publishedID = id
				} else {
					code = "invalid_result"
				}
			}
		}
		if current && plan.Enabled && g.Status == "active" && reading.NextAt.After(time.Now()) && code != "invalid_result" {
			var id string
			var scheduled time.Time
			e = tx.QueryRow(ctx, `SELECT id::text,draw_at FROM platform_game_periods WHERE game_id=$1 AND period_no=$2`, game, reading.NextNo).Scan(&id, &scheduled)
			if e != nil && !errors.Is(e, pgx.ErrNoRows) {
				return 0, e
			}
			if e == nil {
				if !scheduled.Equal(reading.NextAt) {
					code = "schedule_conflict"
				}
			} else {
				end := reading.NextAt.Add(-time.Duration(plan.CloseBeforeSeconds) * time.Second)
				if end.After(time.Now()) {
					_, e = s.CreatePeriod(ctx, tx, systemAutomationActor(actor), game, PeriodInput{AutomationVersion: &plan.Version, PeriodNo: reading.NextNo, BetStartAt: time.Now(), BetEndAt: end, DrawAt: reading.NextAt, Reason: "Announced Hong Kong source period and time"}, points.Metadata{ActorType: "system", ActorID: actor})
					if errors.Is(e, ErrAttributesMissing) {
						code = "attributes_missing"
					} else if e != nil {
						return 0, e
					}
				}
			}
			if e = reserveBrandSeals(ctx, tx, game); e != nil {
				return 0, e
			}
		}
	}
	if code == "ready" {
		var waiting bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform_game_periods WHERE game_id=$1 AND automation_version=$2 AND result IS NULL AND draw_at<=clock_timestamp())`, game, version).Scan(&waiting); e != nil {
			return 0, e
		}
		if waiting {
			code = "waiting_result"
		}
	}
	attemptCode := code
	if attemptCode == "ready" || attemptCode == "waiting_result" {
		attemptCode = "no_data"
	}
	if attemptCode == "schedule_conflict" || attemptCode == "attributes_missing" {
		attemptCode = "invalid_result"
	}
	_, e = tx.Exec(ctx, `INSERT INTO platform_automation_attempts(period_id,last_attempt_at,code) SELECT id,clock_timestamp(),CASE WHEN id=NULLIF($4,'')::uuid THEN 'published' ELSE $3 END FROM platform_game_periods WHERE game_id=$1 AND automation_version=$2 AND draw_at<=clock_timestamp() AND (result IS NULL OR id=NULLIF($4,'')::uuid) ON CONFLICT(period_id) DO UPDATE SET last_attempt_at=excluded.last_attempt_at,code=excluded.code,attempts=platform_automation_attempts.attempts+1`, game, version, attemptCode, publishedID)
	if e != nil {
		return 0, e
	}
	var next *string
	var at *time.Time
	var last []byte
	if reading.NextNo != "" {
		next = &reading.NextNo
		at = &reading.NextAt
	}
	if reading.Completed != nil {
		last, e = json.Marshal(reading.Completed)
		if e != nil {
			return 0, e
		}
	}
	_, e = tx.Exec(ctx, `INSERT INTO platform_hk_source_checks(game_id,version,checked_at,code,next_period_no,next_draw_at,last_result)VALUES($1,$2,clock_timestamp(),$3,$4,$5,$6) ON CONFLICT(game_id,version) DO UPDATE SET checked_at=excluded.checked_at,code=excluded.code,next_period_no=coalesce(excluded.next_period_no,platform_hk_source_checks.next_period_no),next_draw_at=coalesce(excluded.next_draw_at,platform_hk_source_checks.next_draw_at),last_result=coalesce(excluded.last_result,platform_hk_source_checks.last_result)`, game, version, code, next, at, last)
	if e != nil {
		return 0, e
	}
	return count, tx.Commit(ctx)
}
