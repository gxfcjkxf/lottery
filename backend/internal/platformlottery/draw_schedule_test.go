package platformlottery

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestGameScheduleDailyTimesAndYearReset(t *testing.T) {
	db := testdb.New(t)
	s := Service{DB: db}
	ctx := context.Background()
	tx, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	p := DrawSchedule{Mode: "daily", DrawTime: "21:30:00", OpenBeforeHours: 24, CloseBeforeSeconds: 30, CollectAfterSeconds: 90}
	g := Game{Timezone: "Asia/Macau"}
	now := time.Date(2026, 12, 31, 14, 0, 0, 0, time.UTC)
	slots, e := s.scheduleSlots(ctx, tx, g, p, now)
	if e != nil {
		t.Fatal(e)
	}
	if len(slots) != 2 || slots[0].PeriodNo != "2027001" || slots[1].PeriodNo != "2027002" || slots[0].DrawAt.UTC().Hour() != 13 || slots[0].DrawAt.Sub(slots[0].BetStartAt) != 24*time.Hour || slots[0].DrawAt.Sub(slots[0].BetEndAt) != 30*time.Second {
		t.Fatalf("wrong slots: %+v", slots)
	}
}

func TestMarkSixSchedulesFollowPlatformTimezone(t *testing.T) {
	db := testdb.NewUnseeded(t)
	s := Service{DB: db}
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	p := DrawSchedule{Mode: "daily", DrawTime: "21:30:00", OpenBeforeHours: 6, CloseBeforeSeconds: 60}
	now := time.Date(2026, 12, 31, 22, 0, 0, 0, time.UTC)
	for _, platform := range []struct {
		zone, number, draw string
	}{
		{"UTC", "2027001", "2027-01-01T21:30:00Z"},
		{"America/New_York", "2026365", "2027-01-01T02:30:00Z"},
	} {
		if _, err = tx.Exec(ctx, `UPDATE platform_settings SET timezone=$1 WHERE id=true`, platform.zone); err != nil {
			t.Fatal(err)
		}
		for _, game := range []Game{
			{Code: "mark_six_trial", Timezone: "Asia/Hong_Kong"},
			{Code: "new_macau_mark_six", Timezone: "Asia/Macau"},
			{Code: "old_macau_mark_six", Timezone: "Asia/Macau"},
		} {
			slots, err := s.scheduleSlots(ctx, tx, game, p, now)
			if err != nil {
				t.Fatal(err)
			}
			if len(slots) != 2 || slots[0].PeriodNo != platform.number || slots[0].DrawAt.UTC().Format(time.RFC3339) != platform.draw || slots[0].DrawAt.Sub(slots[0].BetStartAt) != 6*time.Hour || slots[0].DrawAt.Sub(slots[0].BetEndAt) != time.Minute {
				t.Fatalf("%s did not follow platform timezone %s: %+v", game.Code, platform.zone, slots)
			}
		}
	}
}

func TestGameScheduleCalendarDatesAndNumbering(t *testing.T) {
	db := testdb.New(t)
	s := Service{DB: db}
	ctx := context.Background()
	tx, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	g := Game{ID: ids.New(), Timezone: "Asia/Hong_Kong"}
	if _, e = tx.Exec(ctx, `UPDATE platform_settings SET timezone='UTC' WHERE id=true`); e != nil {
		t.Fatal(e)
	}
	p := DrawSchedule{Mode: "hkjc", DrawTime: "21:30:00", OpenBeforeHours: 48, CloseBeforeSeconds: 15, CollectAfterSeconds: 60}
	// Unpublished dates must not be replaced by a fixed weekday assumption.
	if _, e = s.scheduleSlots(ctx, tx, g, p, time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)); !errors.Is(e, ErrDrawCalendar) {
		t.Fatal("missing calendar accepted", e)
	}
	if _, e = tx.Exec(ctx, `INSERT INTO hkjc_calendar_months(month,draw_dates,snowball_dates,presale_dates,synced_at) VALUES('2026-10-01',ARRAY['2026-10-27','2026-10-29','2026-10-31']::date[],'{}','{}',now())`); e != nil {
		t.Fatal(e)
	}
	// Supply an independently known result anchor, not a guessed annual count.
	raw, e := os.ReadFile("../rules/config/examples/lucky_group_preview.json")
	if e != nil {
		t.Fatal(e)
	}
	var snapshot rules.ConfiguredSnapshot
	if e = json.Unmarshal(raw, &snapshot); e != nil {
		t.Fatal(e)
	}
	model, _ := json.Marshal(snapshot.Catalog.Model)
	actor := ids.New()
	if _, e = tx.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,'calendar_operator','test-only')`, actor); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, `INSERT INTO platform_games(id,code,name,model,timezone,created_by) VALUES($1,'calendar_schedule','Calendar schedule',$2,'Asia/Hong_Kong',$3)`, g.ID, model, actor); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, `INSERT INTO platform_draw_history(game_id,period_no,draw_year,draw_date,result,source) VALUES($1,'2026100',2026,'2026-10-27','{"regular":[1,2,3,4,5,6],"special":[7],"digits":[]}','hkjc')`, g.ID); e != nil {
		t.Fatal(e)
	}
	slots, e := s.scheduleSlots(ctx, tx, g, p, time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	if e != nil {
		t.Fatal(e)
	}
	if len(slots) != 2 || slots[0].PeriodNo != "2026101" || slots[0].DrawAt.Day() != 29 || slots[0].DrawAt.UTC().Hour() != 21 || slots[1].PeriodNo != "2026102" || slots[1].DrawAt.Day() != 31 {
		t.Fatalf("calendar ignored %+v", slots)
	}
	if slots[0].DrawAt.Sub(slots[0].BetStartAt) != 48*time.Hour {
		t.Fatal("opening hours ignored")
	}
}

func TestGameScheduleCollectionDelayAndFrozenPeriods(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	s := Service{DB: db}
	a := access.Account{ID: ids.New(), Type: access.AccountAdmin, SuperAdmin: true, Roles: []access.Role{{Permissions: []access.Permission{{Resource: "platform_game", Action: "write", Scope: access.ScopePlatform}}}}}
	if _, e := db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,'schedule_operator','test-only')`, a.ID); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile("../rules/config/examples/lucky_group_preview.json")
	if e != nil {
		t.Fatal(e)
	}
	var snapshot rules.ConfiguredSnapshot
	if e = json.Unmarshal(raw, &snapshot); e != nil {
		t.Fatal(e)
	}
	var requests atomic.Int32
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		json.NewEncoder(w).Encode(map[string]any{"period_no": r.URL.Query().Get("period"), "drawn_at": time.Now().UTC().Format(time.RFC3339Nano), "regular": []int{1, 2, 3, 4, 5, 6}, "special": []int{7}})
	}))
	defer feed.Close()
	tx, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	g, e := s.Create(ctx, tx, a, GameInput{Code: "schedule_mark_six", Name: "Schedule Mark Six", Timezone: "Asia/Macau", Model: snapshot.Catalog.Model}, points.Metadata{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.SaveConfiguredRules(ctx, tx, a, g.ID, ConfiguredRulesInput{Snapshot: raw}, points.Metadata{}); e != nil {
		t.Fatal(e)
	}
	schedule := DrawSchedule{Enabled: true, Mode: "daily", DrawTime: "21:30:00", OpenBeforeHours: 24, CloseBeforeSeconds: 15, CollectAfterSeconds: 90}
	saved, e := s.SaveDrawSchedule(ctx, tx, a, g.ID, schedule, points.Metadata{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.SaveDrawSchedule(ctx, tx, a, g.ID, schedule, points.Metadata{}); !errors.Is(e, ErrVersionConflict) {
		t.Fatal("stale version accepted", e)
	}
	if _, e = s.SaveDrawSchedule(ctx, tx, access.Account{}, g.ID, saved, points.Metadata{}); !errors.Is(e, ErrDenied) {
		t.Fatal("brand/no-permission write accepted", e)
	}
	plan := AutomationPlan{Enabled: true, SourceType: "mapped", SourceURL: feed.URL, Mapping: &FeedMapping{Format: "json", Schedule: "game", TimeFormat: "rfc3339", Period: FeedField{Path: "/period_no"}, DrawTime: FeedField{Path: "/drawn_at"}, Regular: &FeedNumbers{FeedField: FeedField{Path: "/regular"}, Count: 6}, Special: &FeedNumbers{FeedField: FeedField{Path: "/special"}, Count: 1}, PeriodQuery: "period"}}
	source, e := s.SaveDrawSource(ctx, tx, a, g.ID, DrawSourceInput{DrawSource: DrawSource{Name: "Result only", Plan: plan}}, points.Metadata{})
	if e != nil {
		t.Fatal(e)
	}
	active, e := s.ActivateDrawSource(ctx, tx, a, g.ID, source.ID, DrawSourceActivation{Revision: 1}, points.Metadata{})
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC()
	periods := []Period{}
	for _, v := range []struct {
		no    string
		delay int
	}{{"delay", 86400}, {"ready", 1}} {
		p, e := s.CreatePeriod(ctx, tx, a, g.ID, PeriodInput{PeriodNo: v.no, BetStartAt: now.Add(-time.Hour), BetEndAt: now.Add(-time.Minute), DrawAt: now.Add(-30 * time.Second), AutomationVersion: &active.Version, ScheduleVersion: &saved.Version, CollectAfterSeconds: v.delay}, points.Metadata{})
		if e != nil {
			t.Fatal(e)
		}
		periods = append(periods, p)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	n, e := s.GenerateAutomaticPeriods(ctx)
	if e != nil || n != 2 {
		t.Fatal("generation failed", n, e)
	}
	if n, e = s.GenerateAutomaticPeriods(ctx); e != nil || n != 0 {
		t.Fatal("generation not idempotent", n, e)
	}
	sources := AutomationSources{Origins: []string{feed.URL}, Client: feed.Client()}
	n, e = s.CollectAutomaticDraws(ctx, sources)
	if e != nil || n != 1 || requests.Load() != 1 {
		t.Fatal("delayed request escaped or ready result not published", n, e, requests.Load())
	}
	n, e = s.CollectAutomaticDraws(ctx, sources)
	if e != nil || n != 0 || requests.Load() != 1 {
		t.Fatal("duplicate publication/request", n, e, requests.Load())
	}
	tx, e = db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	saved.DrawTime = "22:00:00"
	saved.CollectAfterSeconds = 120
	if _, e = s.SaveDrawSchedule(ctx, tx, a, g.ID, saved, points.Metadata{}); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	var delay int
	if e = db.QueryRow(ctx, `SELECT collect_after_seconds FROM platform_game_periods WHERE id=$1`, periods[0].ID).Scan(&delay); e != nil || delay != 86400 {
		t.Fatal("frozen delay changed", delay, e)
	}
	if _, e = db.Exec(ctx, `UPDATE platform_game_periods SET collect_after_seconds=0 WHERE id=$1`, periods[0].ID); e == nil {
		t.Fatal("period delay is mutable")
	}
	tx, e = db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if _, e = s.UpdatePeriodOpening(ctx, tx, a, periods[0].ID, PeriodOpeningInput{BetStartAt: now.Add(-2 * time.Hour), ExpectedBetStartAt: periods[0].BetStartAt}, points.Metadata{}); !errors.Is(e, ErrPeriodInUse) {
		t.Fatal("scheduled period manually changed", e)
	}
}
