package platformlottery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

// Upstream-announced schedules use the same mapping reader as interval feeds.
func (s Service) SyncMappedSources(ctx context.Context, sources AutomationSources) (int, error) {
	rows, e := s.DB.Query(ctx, `SELECT v.game_id::text,v.version FROM platform_automation_versions v JOIN platform_games g ON g.id=v.game_id LEFT JOIN platform_automation_settings h ON h.game_id=v.game_id LEFT JOIN platform_hk_source_checks c ON c.game_id=v.game_id AND c.version=v.version WHERE v.source_type='mapped' AND v.mapping->>'schedule'='source' AND ((v.enabled AND g.status='active' AND h.version=v.version) OR EXISTS(SELECT 1 FROM platform_game_periods p WHERE p.game_id=v.game_id AND p.automation_version=v.version AND p.result IS NULL AND p.draw_at<=clock_timestamp())) AND (c.checked_at IS NULL OR c.checked_at<clock_timestamp()-interval '5 seconds') ORDER BY v.game_id,v.version LIMIT 12`)
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
		n, e := s.syncMappedSource(ctx, sources, k.game, k.version)
		if e != nil {
			return count, e
		}
		count += n
	}
	return count, nil
}
func (s Service) syncMappedSource(ctx context.Context, sources AutomationSources, game string, version int64) (int, error) {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback(ctx)
	var acquired bool
	if e = tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,0))`, fmt.Sprintf("mapped-source:%s:%d", game, version)).Scan(&acquired); e != nil || !acquired {
		return 0, e
	}
	var due bool
	if e = tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM platform_hk_source_checks WHERE game_id=$1 AND version=$2 AND checked_at>clock_timestamp()-interval '5 seconds')`, game, version).Scan(&due); e != nil || !due {
		return 0, e
	}
	plan, e := scanAutomation(tx.QueryRow(ctx, `SELECT `+automationPlanFields+` FROM platform_automation_versions v WHERE game_id=$1 AND version=$2`, game, version))
	if e != nil {
		return 0, e
	}
	var actor, gameCode string
	var current bool
	if e = tx.QueryRow(ctx, `SELECT v.created_by::text,g.code,EXISTS(SELECT 1 FROM platform_automation_settings h WHERE h.game_id=v.game_id AND h.version=v.version) FROM platform_automation_versions v JOIN platform_games g ON g.id=v.game_id WHERE v.game_id=$1 AND v.version=$2`, game, version).Scan(&actor, &gameCode, &current); e != nil {
		return 0, e
	}
	code := "source_error"
	reading, readErr := FetchMappedFeed(ctx, sources, plan, "", gameCode)
	count := 0
	publishedID := ""
	if readErr == nil {
		code = "ready"
		if reading.Completed == nil {
			code = "waiting_result"
		}
		g, e := lockedGame(ctx, tx, game)
		if e != nil {
			return 0, e
		}
		if reading.Completed != nil {
			c := reading.Completed
			if rules.ValidateDraw(g.Model, c.Result) != nil || c.DrawnAt.After(time.Now().Add(10*time.Second)) {
				code = "invalid_result"
			} else {
				var id string
				var at time.Time
				e = tx.QueryRow(ctx, `SELECT id::text,draw_at FROM platform_game_periods WHERE game_id=$1 AND automation_version=$2 AND period_no=$3 AND result IS NULL AND draw_at<=clock_timestamp() FOR UPDATE`, game, version, c.PeriodNo).Scan(&id, &at)
				if e != nil && !errors.Is(e, pgx.ErrNoRows) {
					return 0, e
				}
				if e == nil {
					if c.DrawnAt.Before(at) {
						code = "invalid_result"
					} else {
						if e = publishAutomaticResult(ctx, tx, id, actor, c, "platform_game.automation.result", "Mapped source matched frozen period and configured model"); e != nil {
							return 0, e
						}
						count++
						publishedID = id
					}
				}
			}
		}
		if current && plan.Enabled && g.Status == "active" && code != "invalid_result" && reading.NextDrawAt != nil && reading.NextDrawAt.After(time.Now()) {
			var id string
			var at time.Time
			var frozenVersion *int64
			e = tx.QueryRow(ctx, `SELECT id::text,draw_at,automation_version FROM platform_game_periods WHERE game_id=$1 AND period_no=$2`, game, *reading.NextPeriodNo).Scan(&id, &at, &frozenVersion)
			if e != nil && !errors.Is(e, pgx.ErrNoRows) {
				return 0, e
			}
			if e == nil {
				if !at.Equal(*reading.NextDrawAt) || frozenVersion == nil || *frozenVersion != version {
					code = "schedule_conflict"
				}
			} else {
				end := reading.NextDrawAt.Add(-time.Duration(plan.CloseBeforeSeconds) * time.Second)
				if end.After(time.Now()) {
					_, e = s.CreatePeriod(ctx, tx, systemAutomationActor(actor), game, PeriodInput{AutomationVersion: &plan.Version, PeriodNo: *reading.NextPeriodNo, BetStartAt: time.Now(), BetEndAt: end, DrawAt: *reading.NextDrawAt}, points.Metadata{ActorType: "system", ActorID: actor})
					if e != nil {
						return 0, e
					}
				}
			}
			if e = reserveBrandSeals(ctx, tx, game); e != nil {
				return 0, e
			}
		}
	} else if errors.Is(readErr, ErrInvalid) {
		code = "invalid_result"
	}
	attemptCode := code
	if attemptCode == "ready" || attemptCode == "waiting_result" {
		attemptCode = "no_data"
	}
	if attemptCode == "schedule_conflict" {
		attemptCode = "invalid_result"
	}
	if _, e = tx.Exec(ctx, `INSERT INTO platform_automation_attempts(period_id,last_attempt_at,code) SELECT id,clock_timestamp(),CASE WHEN id=NULLIF($4,'')::uuid THEN 'published' ELSE $3 END FROM platform_game_periods WHERE game_id=$1 AND automation_version=$2 AND draw_at<=clock_timestamp() AND (result IS NULL OR id=NULLIF($4,'')::uuid) ON CONFLICT(period_id) DO UPDATE SET last_attempt_at=excluded.last_attempt_at,code=excluded.code,attempts=platform_automation_attempts.attempts+1`, game, version, attemptCode, publishedID); e != nil {
		return 0, e
	}
	var last []byte
	if reading.Completed != nil && (code == "ready" || code == "waiting_result") {
		c := reading.Completed
		last, e = json.Marshal(c)
		if e != nil {
			return 0, e
		}
	}
	if _, e = tx.Exec(ctx, `INSERT INTO platform_hk_source_checks(game_id,version,checked_at,code,next_period_no,next_draw_at,last_result) VALUES($1,$2,clock_timestamp(),$3,$4,$5,$6) ON CONFLICT(game_id,version) DO UPDATE SET checked_at=excluded.checked_at,code=excluded.code,next_period_no=excluded.next_period_no,next_draw_at=excluded.next_draw_at,last_result=excluded.last_result`, game, version, code, reading.NextPeriodNo, reading.NextDrawAt, last); e != nil {
		return 0, e
	}
	return count, tx.Commit(ctx)
}
