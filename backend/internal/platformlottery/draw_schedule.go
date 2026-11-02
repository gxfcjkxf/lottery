package platformlottery

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

var ErrDrawCalendar = errors.New("draw calendar or reference period is missing")

type DrawSchedule struct {
	Version             int64   `json:"version"`
	Enabled             bool    `json:"enabled"`
	Mode                string  `json:"mode"`
	DrawTime            string  `json:"draw_time"`
	OpenBeforeHours     float64 `json:"open_before_hours"`
	CloseBeforeSeconds  int     `json:"close_before_seconds"`
	CollectAfterSeconds int     `json:"collect_after_seconds"`
}

const drawScheduleFields = `v.version,v.enabled,v.mode,to_char(v.draw_time,'HH24:MI:SS'),v.open_before_hours,v.close_before_seconds,v.collect_after_seconds`

func scanDrawSchedule(row pgx.Row) (p DrawSchedule, err error) {
	err = row.Scan(&p.Version, &p.Enabled, &p.Mode, &p.DrawTime, &p.OpenBeforeHours, &p.CloseBeforeSeconds, &p.CollectAfterSeconds)
	return
}
func (s Service) DrawSchedule(ctx context.Context, game string) (*DrawSchedule, error) {
	p, err := scanDrawSchedule(s.DB.QueryRow(ctx, `SELECT `+drawScheduleFields+` FROM platform_draw_schedule_settings h JOIN platform_draw_schedule_versions v USING(game_id,version) WHERE h.game_id=$1`, game))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}
func validateDrawSchedule(p DrawSchedule) error {
	parsed, err := time.Parse("15:04:05", p.DrawTime)
	if err != nil || parsed.Format("15:04:05") != p.DrawTime || p.Version < 0 || (p.Mode != "daily" && p.Mode != "hkjc") || p.OpenBeforeHours <= 0 || p.OpenBeforeHours > 8760 || p.CloseBeforeSeconds < 0 || float64(p.CloseBeforeSeconds) >= math.Round(p.OpenBeforeHours*3600) || p.CollectAfterSeconds < 0 || p.CollectAfterSeconds > 86400 {
		return ErrInvalid
	}
	return nil
}
func (s Service) SaveDrawSchedule(ctx context.Context, tx pgx.Tx, a access.Account, game string, in DrawSchedule, meta points.Metadata) (DrawSchedule, error) {
	if !Allowed(a, "", "write") {
		return in, ErrDenied
	}
	if validateDrawSchedule(in) != nil {
		return in, ErrInvalid
	}
	g, err := lockedGame(ctx, tx, game)
	if err != nil {
		return in, err
	}
	if rules.ValidateMarkSixModel(g.Model) != nil || (in.Mode == "hkjc" && g.Timezone != "Asia/Hong_Kong") {
		return in, ErrInvalid
	}
	var current int64
	if err = tx.QueryRow(ctx, `SELECT coalesce((SELECT version FROM platform_draw_schedule_settings WHERE game_id=$1),0)`, game).Scan(&current); err != nil {
		return in, err
	}
	if in.Version != current {
		return in, ErrVersionConflict
	}
	var before any
	if current > 0 {
		previous, e := scanDrawSchedule(tx.QueryRow(ctx, `SELECT `+drawScheduleFields+` FROM platform_draw_schedule_versions v WHERE game_id=$1 AND version=$2`, game, current))
		if e != nil {
			return in, e
		}
		before = previous
	}
	// Check calendar evidence before accepting a calendar-based schedule; never invent dates or period numbers.
	if in.Enabled && in.Mode == "hkjc" {
		if _, err = s.scheduleSlots(ctx, tx, g, in, time.Now()); err != nil {
			return in, err
		}
	}
	in.Version++
	_, err = tx.Exec(ctx, `INSERT INTO platform_draw_schedule_versions(game_id,version,enabled,mode,draw_time,open_before_hours,close_before_seconds,collect_after_seconds,created_by) VALUES($1,$2,$3,$4,$5::time,$6,$7,$8,$9)`, game, in.Version, in.Enabled, in.Mode, in.DrawTime, in.OpenBeforeHours, in.CloseBeforeSeconds, in.CollectAfterSeconds, a.ID)
	if err != nil {
		return in, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO platform_draw_schedule_settings(game_id,version) VALUES($1,$2) ON CONFLICT(game_id) DO UPDATE SET version=excluded.version`, game, in.Version); err != nil {
		return in, err
	}
	return in, record(ctx, tx, a, "", "platform_game.schedule.save", game, "", meta, before, in)
}
func schedulePeriod(p DrawSchedule, date, zone, number string) (Period, error) {
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return Period{}, err
	}
	draw, err := time.ParseInLocation("2006-01-02 15:04:05", date+" "+p.DrawTime, loc)
	if err != nil {
		return Period{}, err
	}
	return Period{PeriodNo: number, DrawAt: draw, BetStartAt: draw.Add(-time.Duration(math.Round(p.OpenBeforeHours*3600)) * time.Second), BetEndAt: draw.Add(-time.Duration(p.CloseBeforeSeconds) * time.Second)}, nil
}
func (s Service) scheduleSlots(ctx context.Context, tx pgx.Tx, g Game, p DrawSchedule, now time.Time) ([]Period, error) {
	// All Mark Six schedules follow the current platform timezone.
	if err := tx.QueryRow(ctx, `SELECT timezone FROM platform_settings WHERE id=true FOR SHARE`).Scan(&g.Timezone); err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(g.Timezone)
	if err != nil {
		return nil, err
	}
	local := now.In(loc)
	dates := []string{}
	if p.Mode == "daily" {
		for day := 0; len(dates) < 2; day++ {
			date := local.AddDate(0, 0, day)
			slot, e := schedulePeriod(p, date.Format("2006-01-02"), g.Timezone, "")
			if e != nil {
				return nil, e
			}
			if !slot.DrawAt.Before(now) {
				dates = append(dates, date.Format("2006-01-02"))
			}
		}
	} else {
		rows, e := tx.Query(ctx, `SELECT DISTINCT d::text FROM hkjc_calendar_months m CROSS JOIN LATERAL unnest(m.draw_dates) d WHERE (d+$1::time) AT TIME ZONE $2 >= $3 ORDER BY d::text LIMIT 2`, p.DrawTime, g.Timezone, now)
		if e != nil {
			return nil, e
		}
		for rows.Next() {
			var d string
			if e = rows.Scan(&d); e != nil {
				rows.Close()
				return nil, e
			}
			dates = append(dates, d)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
		if len(dates) == 0 {
			return nil, ErrDrawCalendar
		}
	}
	slots := []Period{}
	for _, d := range dates {
		date, _ := time.Parse("2006-01-02", d)
		number := fmt.Sprintf("%04d%03d", date.Year(), date.YearDay())
		if p.Mode == "hkjc" {
			number, err = s.calendarPeriodNumber(ctx, tx, g, d)
			if err != nil {
				return nil, err
			}
		}
		slot, e := schedulePeriod(p, d, g.Timezone, number)
		if e != nil {
			return nil, e
		}
		slots = append(slots, slot)
	}
	return slots, nil
}
func (s Service) calendarPeriodNumber(ctx context.Context, tx pgx.Tx, g Game, date string) (string, error) {
	// A known period anchors the annual sequence. Every intervening month must have an official calendar.
	var anchorDate, anchorNo string
	err := tx.QueryRow(ctx, `SELECT d::text,period_no FROM (
 SELECT (draw_at AT TIME ZONE $2)::date d,period_no FROM platform_game_periods WHERE game_id=$1
 UNION ALL SELECT draw_date,period_no FROM platform_draw_history WHERE game_id=$1 AND draw_date IS NOT NULL
 ) x WHERE d<=$3::date AND extract(year FROM d)=extract(year FROM $3::date) AND period_no ~ '^[0-9]{7}$' ORDER BY d DESC LIMIT 1`, g.ID, g.Timezone, date).Scan(&anchorDate, &anchorNo)
	base := 0
	if errors.Is(err, pgx.ErrNoRows) {
		anchorDate = date[:4] + "-01-01"
	} else if err != nil {
		return "", err
	} else {
		base, err = strconv.Atoi(anchorNo[4:])
		if err != nil || anchorNo[:4] != date[:4] {
			return "", ErrDrawCalendar
		}
	}
	var complete bool
	if err = tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM generate_series(date_trunc('month',$1::date),date_trunc('month',$2::date),interval '1 month') m WHERE NOT EXISTS(SELECT 1 FROM hkjc_calendar_months c WHERE c.month=m::date))`, anchorDate, date).Scan(&complete); err != nil {
		return "", err
	}
	if !complete {
		return "", ErrDrawCalendar
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(DISTINCT d) FROM hkjc_calendar_months m CROSS JOIN LATERAL unnest(m.draw_dates) d WHERE d<=$2::date AND (d>$1::date OR ($3=0 AND d=$1::date))`, anchorDate, date, base).Scan(&count); err != nil {
		return "", err
	}
	if base+count < 1 {
		return "", ErrDrawCalendar
	}
	return fmt.Sprintf("%s%03d", date[:4], base+count), nil
}
func (s Service) GenerateScheduledPeriods(ctx context.Context) (int, error) {
	rows, err := s.DB.Query(ctx, `SELECT h.game_id::text FROM platform_draw_schedule_settings h JOIN platform_draw_schedule_versions v USING(game_id,version) JOIN platform_games g ON g.id=h.game_id JOIN platform_automation_settings a ON a.game_id=g.id JOIN platform_automation_versions f ON f.game_id=a.game_id AND f.version=a.version WHERE v.enabled AND f.enabled AND g.status='active' ORDER BY h.game_id`)
	if err != nil {
		return 0, err
	}
	games := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		games = append(games, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	count := 0
	var problems error
	for _, game := range games {
		n, e := s.generateScheduledGame(ctx, game)
		count += n
		problems = errors.Join(problems, e)
	}
	return count, problems
}
func (s Service) generateScheduledGame(ctx context.Context, game string) (int, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	g, err := lockedGame(ctx, tx, game)
	if err != nil {
		return 0, err
	}
	p, err := scanDrawSchedule(tx.QueryRow(ctx, `SELECT `+drawScheduleFields+` FROM platform_draw_schedule_settings h JOIN platform_draw_schedule_versions v USING(game_id,version) WHERE h.game_id=$1`, game))
	if err != nil {
		return 0, err
	}
	if !p.Enabled || g.Status != "active" {
		return 0, tx.Commit(ctx)
	}
	var sourceVersion int64
	var actor string
	var enabled bool
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT v.version,v.created_by::text,v.enabled,clock_timestamp() FROM platform_automation_settings h JOIN platform_automation_versions v USING(game_id,version) WHERE h.game_id=$1`, game).Scan(&sourceVersion, &actor, &enabled, &now); err != nil {
		return 0, err
	}
	if !enabled {
		return 0, tx.Commit(ctx)
	}
	slots, err := s.scheduleSlots(ctx, tx, g, p, now)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, slot := range slots {
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform_game_periods WHERE game_id=$1 AND period_no=$2)`, game, slot.PeriodNo).Scan(&exists); err != nil {
			return 0, err
		}
		if exists {
			continue
		}
		_, e := s.CreatePeriod(ctx, tx, systemAutomationActor(actor), game, PeriodInput{PeriodNo: slot.PeriodNo, BetStartAt: slot.BetStartAt, BetEndAt: slot.BetEndAt, DrawAt: slot.DrawAt, AutomationVersion: &sourceVersion, ScheduleVersion: &p.Version, CollectAfterSeconds: p.CollectAfterSeconds}, points.Metadata{ActorType: "system", ActorID: actor})
		if e != nil {
			return 0, e
		}
		count++
	}
	return count, tx.Commit(ctx)
}
