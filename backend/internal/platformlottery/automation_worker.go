package platformlottery

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

type AutomationSources struct {
	Origins []string
	Client  *http.Client
}

func LoadAutomationSources() (AutomationSources, error) {
	out := AutomationSources{Origins: []string{}, Client: &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return errors.New("redirects not supported") }}}
	raw := os.Getenv("PLATFORM_DRAW_ALLOWED_ORIGINS")
	if raw == "" {
		return out, nil
	}
	if json.Unmarshal([]byte(raw), &out.Origins) != nil || out.Origins == nil {
		return out, ErrInvalid
	}
	for _, origin := range out.Origins {
		u, e := url.Parse(origin)
		if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || u.Scheme != "https" && (u.Scheme != "http" || u.Hostname() != "127.0.0.1" || os.Getenv("APP_ENV") == "production") {
			return out, ErrInvalid
		}
	}
	return out, nil
}
func (s AutomationSources) Allows(endpoint string) bool {
	u, e := url.Parse(endpoint)
	if e != nil {
		return false
	}
	for _, origin := range s.Origins {
		if u.Scheme+"://"+u.Host == origin {
			return true
		}
	}
	return false
}
func systemAutomationActor(id string) access.Account {
	return access.Account{ID: id, Type: access.AccountAdmin, SuperAdmin: true, Roles: []access.Role{{Permissions: []access.Permission{{Resource: "platform_game", Action: "write", Scope: access.ScopePlatform}}}}}
}

// Reservations and collection have separate loops, so network delays cannot postpone sealing.
func (s Service) GenerateAutomaticPeriods(ctx context.Context) (int, error) {
	rows, e := s.DB.Query(ctx, `SELECT s.game_id::text FROM platform_automation_settings s JOIN platform_automation_versions v USING(game_id,version) JOIN platform_games g ON g.id=s.game_id WHERE v.enabled AND (v.source_type='interval_json' OR v.source_type='mapped' AND v.mapping->>'schedule'='interval') AND g.status='active' ORDER BY s.game_id`)
	if e != nil {
		return 0, e
	}
	games := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return 0, e
		}
		games = append(games, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return 0, e
	}
	count := 0
	var problems error
	for _, game := range games {
		n, e := s.generateAutomaticGame(ctx, game)
		if e != nil {
			problems = errors.Join(problems, e)
			continue
		}
		count += n
	}
	return count, problems
}
func (s Service) generateAutomaticGame(ctx context.Context, game string) (int, error) {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback(ctx)
	g, e := lockedGame(ctx, tx, game)
	if e != nil {
		return 0, e
	}
	if g.Status != "active" {
		return 0, tx.Commit(ctx)
	}
	p, e := scanAutomation(tx.QueryRow(ctx, `SELECT `+automationPlanFields+` FROM platform_automation_settings s JOIN platform_automation_versions v USING(game_id,version) WHERE s.game_id=$1`, game))
	if e != nil {
		return 0, e
	}
	if !p.Enabled || !intervalPlan(p) {
		return 0, tx.Commit(ctx)
	}
	var actor string
	var now time.Time
	if e = tx.QueryRow(ctx, `SELECT created_by::text,clock_timestamp() FROM platform_automation_versions WHERE game_id=$1 AND version=$2`, game, p.Version).Scan(&actor, &now); e != nil {
		return 0, e
	}
	count := 0
	for offset := int64(0); offset < 2; offset++ {
		slot, e := automationSlot(p, now, offset)
		if e != nil {
			return 0, e
		}
		var exists bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform_game_periods WHERE game_id=$1 AND period_no=$2)`, game, slot.PeriodNo).Scan(&exists); e != nil {
			return 0, e
		}
		if exists {
			var matches bool
			if e = tx.QueryRow(ctx, `SELECT automation_version=$3 AND bet_start_at=$4 AND bet_end_at=$5 AND draw_at=$6 FROM platform_game_periods WHERE game_id=$1 AND period_no=$2`, game, slot.PeriodNo, p.Version, slot.BetStartAt, slot.BetEndAt, slot.DrawAt).Scan(&matches); e != nil {
				return 0, e
			}
			if !matches {
				return 0, ErrConflict
			}
			continue
		}
		_, e = s.CreatePeriod(ctx, tx, systemAutomationActor(actor), game, PeriodInput{PeriodNo: slot.PeriodNo, BetStartAt: slot.BetStartAt, BetEndAt: slot.BetEndAt, DrawAt: slot.DrawAt, AutomationVersion: &p.Version, Reason: "Automatic period reservation"}, points.Metadata{ActorType: "system", ActorID: actor})
		if e != nil {
			return 0, e
		}
		count++
	}
	if e = reserveBrandSeals(ctx, tx, game); e != nil {
		return 0, e
	}
	return count, tx.Commit(ctx)
}

func (s Service) CollectAutomaticDraws(ctx context.Context, sources AutomationSources) (int, error) {
	mappedCount, mappedErr := s.SyncMappedSources(ctx, sources)
	hkCount, hkErr := s.SyncHongKongSources(ctx, sources)
	problems := errors.Join(mappedErr, hkErr)
	rows, e := s.DB.Query(ctx, `SELECT p.id::text FROM platform_game_periods p LEFT JOIN platform_automation_attempts a ON a.period_id=p.id JOIN platform_automation_versions v ON v.game_id=p.game_id AND v.version=p.automation_version WHERE (v.source_type='interval_json' OR v.source_type='mapped' AND v.mapping->>'schedule'='interval') AND p.automation_version IS NOT NULL AND p.result IS NULL AND p.draw_at<=clock_timestamp() AND (a.last_attempt_at IS NULL OR a.last_attempt_at<clock_timestamp()-interval '5 seconds') ORDER BY a.last_attempt_at NULLS FIRST,p.draw_at,p.id LIMIT 12`)
	if e != nil {
		return mappedCount + hkCount, errors.Join(problems, e)
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return mappedCount + hkCount, errors.Join(problems, e)
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return mappedCount + hkCount, errors.Join(problems, e)
	}
	count := 0
	for _, id := range ids {
		n, e := s.collectAutomatic(ctx, sources, id)
		if e != nil {
			problems = errors.Join(problems, e)
			continue
		}
		count += n
	}
	return count + hkCount + mappedCount, problems
}
func (s Service) collectAutomatic(ctx context.Context, sources AutomationSources, id string) (int, error) {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback(ctx)
	var p Period
	var raw []byte
	var gameCode, actor string
	var now time.Time
	var v AutomationPlan
	e = tx.QueryRow(ctx, `SELECT p.id::text,p.game_id::text,p.period_no,p.draw_at,g.model,g.code,v.source_url,v.credential_ref,v.mapping,v.source_type,v.created_by::text,clock_timestamp() FROM platform_game_periods p JOIN platform_games g ON g.id=p.game_id JOIN platform_automation_versions v ON v.game_id=p.game_id AND v.version=p.automation_version WHERE p.id=$1 AND p.result IS NULL AND p.draw_at<=clock_timestamp() AND NOT EXISTS(SELECT 1 FROM platform_automation_attempts a WHERE a.period_id=p.id AND a.last_attempt_at>clock_timestamp()-interval '5 seconds') FOR UPDATE OF p SKIP LOCKED`, id).Scan(&p.ID, &p.GameID, &p.PeriodNo, &p.DrawAt, &raw, &gameCode, &v.SourceURL, &v.CredentialRef, &v.Mapping, &v.SourceType, &actor, &now)
	if errors.Is(e, pgx.ErrNoRows) {
		return 0, nil
	}
	if e != nil {
		return 0, e
	}
	var model rules.Model
	if e = json.Unmarshal(raw, &model); e != nil {
		return 0, e
	}
	code := "source_error"
	candidate := SourceObservedDraw{}
	if v.SourceType == "mapped" {
		reading, err := FetchMappedFeed(ctx, sources, v, p.PeriodNo, gameCode)
		code = "invalid_result"
		if err == nil {
			if reading.Completed == nil {
				code = "no_data"
			} else {
				candidate = *reading.Completed
				if candidate.PeriodNo == p.PeriodNo && !candidate.DrawnAt.Before(p.DrawAt) && !candidate.DrawnAt.After(time.Now().Add(10*time.Second)) && rules.ValidateDraw(model, candidate.Result) == nil {
					code = "published"
				}
			}
		}
	} else if sources.Allows(v.SourceURL) {
		token := ""
		if v.CredentialRef != "" {
			token = os.Getenv(v.CredentialRef)
		}
		if v.CredentialRef != "" && token == "" {
			code = "credential_missing"
		} else {
			u, _ := url.Parse(v.SourceURL)
			q := u.Query()
			q.Set("period_no", p.PeriodNo)
			q.Set("game_code", gameCode)
			u.RawQuery = q.Encode()
			req, e := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
			if e != nil {
				return 0, e
			}
			req.Header.Set("Accept", "application/json")
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			resp, e := sources.Client.Do(req)
			if e == nil {
				body, readErr := io.ReadAll(io.LimitReader(resp.Body, 65537))
				resp.Body.Close()
				if resp.StatusCode == 204 || resp.StatusCode == 404 {
					code = "no_data"
				} else if resp.StatusCode == 200 && readErr == nil {
					code = "invalid_result"
					if len(body) <= 65536 && json.Unmarshal(body, &candidate) == nil && candidate.PeriodNo == p.PeriodNo && !candidate.DrawnAt.Before(p.DrawAt) && !candidate.DrawnAt.After(time.Now().Add(10*time.Second)) && rules.ValidateDraw(model, candidate.Result) == nil {
						code = "published"
					}
				}
			}
		}
	}
	if code == "published" {
		if e = publishAutomaticResult(ctx, tx, id, actor, &candidate, "platform_game.automation.result", "Validated automatic source result"); e != nil {
			return 0, e
		}
	}
	_, e = tx.Exec(ctx, `INSERT INTO platform_automation_attempts(period_id,last_attempt_at,code)VALUES($1,clock_timestamp(),$2) ON CONFLICT(period_id) DO UPDATE SET last_attempt_at=excluded.last_attempt_at,code=excluded.code,attempts=platform_automation_attempts.attempts+1`, id, code)
	if e != nil {
		return 0, e
	}
	if e = tx.Commit(ctx); e != nil {
		return 0, e
	}
	if code == "published" {
		return 1, nil
	}
	return 0, nil
}
