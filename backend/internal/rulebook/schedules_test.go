package rulebook

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/schedule"
	"github.com/jackc/pgx/v5"
)

func scheduleActor(id string) access.Account {
	a := actor(id)
	for i := range a.Roles {
		a.Roles[i].Permissions = append(a.Roles[i].Permissions,
			access.Permission{Resource: "schedule", Action: "write", Scope: access.ScopeBrand},
			access.Permission{Resource: "period", Action: "generate", Scope: access.ScopeBrand},
		)
	}
	return a
}

func scheduleActorForBrand(id, brandID string) access.Account {
	a := actor(id)
	a.BrandIDs = []string{brandID}
	a.Roles = []access.Role{{BrandID: brandID, Permissions: []access.Permission{
		{Resource: "schedule", Action: "write", Scope: access.ScopeBrand},
		{Resource: "period", Action: "generate", Scope: access.ScopeBrand},
	}}}
	return a
}

func dailySpec(timezone string, times ...string) schedule.Spec {
	return schedule.Spec{Timezone: timezone, Mode: "daily", DailyDrawTimes: times,
		BetOpenBeforeSeconds: 3600, BetCloseBeforeSeconds: 60,
		Weekdays: []int{0, 1, 2, 3, 4, 5, 6}, HolidayPolicy: "normal"}
}

func writeSchedule(t *testing.T, s Store, a access.Account, g Game, spec schedule.Spec) ScheduleRecord {
	t.Helper()
	var got ScheduleRecord
	transact(t, s.DB, func(tx pgx.Tx) error {
		var err error
		got, err = s.WriteSchedule(context.Background(), tx, brand, a, g.ID, g.Version, spec, "schedule test", points.Metadata{RequestID: "schedule-write"})
		return err
	})
	return got
}

func databaseNow(t *testing.T, s Store) time.Time {
	t.Helper()
	var now time.Time
	if err := s.DB.QueryRow(context.Background(), `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	return now
}

func mustLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func insertPendingPeriod(t *testing.T, s Store, g Game, no string, seq int64, start, end, draw time.Time, scheduleID string) Period {
	t.Helper()
	var p Period
	transact(t, s.DB, func(tx pgx.Tx) error {
		var err error
		p, err = scanPeriod(tx.QueryRow(context.Background(), `INSERT INTO periods
			(id,brand_id,game_id,period_no,sequence,bet_start_at,bet_end_at,draw_at,status,schedule_id)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,'pending',NULLIF($9,'')::uuid) RETURNING `+periodFields,
			ids.New(), brand, g.ID, no, seq, start, end, draw, scheduleID))
		return err
	})
	return p
}

func TestScheduleWriteRevisionAndOptimisticGameVersion(t *testing.T) {
	s, a0, _, g, _, _ := fixture(t)
	a := scheduleActor(a0.ID)
	first := writeSchedule(t, s, a, g, dailySpec(g.Timezone, "12:00:00"))
	if first.Revision != 1 || first.GameVersion != g.Version+1 || first.Spec.DailyDrawTimes[0] != "12:00:00" {
		t.Fatalf("unexpected first schedule: %+v", first)
	}
	current, err := s.Schedule(context.Background(), brand, g.ID)
	if err != nil || current.ID != first.ID || current.Revision != 1 {
		t.Fatalf("current schedule = %+v, err=%v", current, err)
	}
	g.Version = first.GameVersion
	second := writeSchedule(t, s, a, g, dailySpec(g.Timezone, "13:00:00"))
	if second.Revision != 2 || second.ID == first.ID || second.GameVersion != first.GameVersion+1 {
		t.Fatalf("unexpected second schedule: %+v", second)
	}
	if n := concurrencyCount(t, s, `SELECT count(*) FROM game_schedules WHERE brand_id=$1 AND game_id=$2`, brand, g.ID); n != 2 {
		t.Fatalf("schedule revisions=%d, want 2", n)
	}
	if concurrencyCount(t, s, `SELECT count(*) FROM audit_logs WHERE action='schedule.write' AND resource_type='game_schedule'`) != 2 {
		t.Fatal("schedule write audit missing")
	}
	tx, err := s.DB.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.WriteSchedule(context.Background(), tx, brand, a, g.ID, first.GameVersion, dailySpec(g.Timezone, "14:00:00"), "stale", points.Metadata{})
	_ = tx.Rollback(context.Background())
	if !errors.Is(err, ErrVersion) {
		t.Fatalf("stale game version error=%v, want ErrVersion", err)
	}
	if concurrencyCount(t, s, `SELECT count(*) FROM game_schedules WHERE game_id=$1`, g.ID) != 2 {
		t.Fatal("stale write created a schedule revision")
	}
}

func TestScheduleWritePermissionBrandAndValidationBoundaries(t *testing.T) {
	s, a0, _, g, _, _ := fixture(t)
	a := scheduleActor(a0.ID)
	noPermission := actor(a.ID)
	foreign := "0199a000-0000-7000-8000-000000000002"
	tests := []struct {
		name  string
		brand string
		actor access.Account
		spec  schedule.Spec
		want  error
	}{
		{"permission", brand, noPermission, dailySpec(g.Timezone, "12:00:00"), ErrDenied},
		{"cross brand", foreign, scheduleActorForBrand(a.ID, foreign), dailySpec(g.Timezone, "12:00:00"), ErrNotFound},
		{"timezone mismatch", brand, a, dailySpec("UTC", "12:00:00"), ErrInvalid},
		{"invalid spec", brand, a, dailySpec(g.Timezone, "12:00"), ErrInvalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tx, err := s.DB.Begin(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.WriteSchedule(context.Background(), tx, tc.brand, tc.actor, g.ID, g.Version, tc.spec, "boundary", points.Metadata{})
			_ = tx.Rollback(context.Background())
			if !errors.Is(err, tc.want) {
				t.Fatalf("WriteSchedule error=%v, want %v", err, tc.want)
			}
		})
	}
	super := a
	super.SuperAdmin = true
	tx, err := s.DB.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.WriteSchedule(context.Background(), tx, brand, super, g.ID, g.Version, dailySpec(g.Timezone, "12:00:00"), "super", points.Metadata{})
	_ = tx.Rollback(context.Background())
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("super-admin write error=%v, want ErrDenied", err)
	}
}

func TestGeneratePeriodsUsesUTCSlotsAndIsIdempotent(t *testing.T) {
	s, a0, _, g, _, _ := fixture(t)
	a := scheduleActor(a0.ID)
	now := databaseNow(t, s)
	drawTime := now.In(mustLocation(t, g.Timezone)).Add(3 * time.Hour).Format("15:04:05")
	record := writeSchedule(t, s, a, g, dailySpec(g.Timezone, drawTime))
	from, to := now.Add(-time.Minute), now.Add(24*time.Hour)
	var first Generation
	transact(t, s.DB, func(tx pgx.Tx) error {
		var err error
		first, err = s.GeneratePeriods(context.Background(), tx, brand, a, g.ID, from, to, "generate", points.Metadata{RequestID: "period-generate"})
		return err
	})
	if first.Created != len(first.Periods) || first.Created == 0 {
		t.Fatalf("first generation = %+v", first)
	}
	for _, p := range first.Periods {
		if p.Status != "pending" || p.ScheduleID != record.ID || p.PeriodNo != p.DrawAt.UTC().Format("20060102T150405Z") {
			t.Fatalf("generated period is not a UTC schedule reservation: %+v", p)
		}
	}
	var again Generation
	transact(t, s.DB, func(tx pgx.Tx) error {
		var err error
		again, err = s.GeneratePeriods(context.Background(), tx, brand, a, g.ID, from, to, "repeat", points.Metadata{})
		return err
	})
	if again.Created != 0 || again.Existing != first.Created || concurrencyCount(t, s, `SELECT count(*) FROM periods WHERE game_id=$1`, g.ID) != first.Created {
		t.Fatalf("repeat generation duplicated reservations: first=%+v second=%+v", first, again)
	}
	if concurrencyCount(t, s, `SELECT count(*) FROM audit_logs WHERE action='period.generate' AND resource_id=$1`, g.ID) != 2 {
		t.Fatal("manual generation calls were not audited exactly once each")
	}
	tooWide := to.Add(7 * 24 * time.Hour)
	tx, err := s.DB.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.GeneratePeriods(context.Background(), tx, brand, a, g.ID, now, tooWide, "too wide", points.Metadata{})
	_ = tx.Rollback(context.Background())
	if !errors.Is(err, schedule.ErrLimit) {
		t.Fatalf("overlong generation error=%v, want schedule.ErrLimit", err)
	}
}

func TestGeneratePeriodsRejectsCalendarConflictWithoutPartialWrites(t *testing.T) {
	s, a0, _, g, _, _ := fixture(t)
	a := scheduleActor(a0.ID)
	now := databaseNow(t, s)
	drawTime := now.In(mustLocation(t, g.Timezone)).Add(3 * time.Hour).Format("15:04:05")
	first := writeSchedule(t, s, a, g, dailySpec(g.Timezone, drawTime))
	from, to := now.Add(-time.Minute), now.Add(24*time.Hour)
	transact(t, s.DB, func(tx pgx.Tx) error {
		_, err := s.GeneratePeriods(context.Background(), tx, brand, a, g.ID, from, to, "initial", points.Metadata{})
		return err
	})
	countBefore := concurrencyCount(t, s, `SELECT count(*) FROM periods WHERE game_id=$1`, g.ID)
	g.Version = first.GameVersion
	changed := dailySpec(g.Timezone, drawTime)
	changed.BetOpenBeforeSeconds++
	second := writeSchedule(t, s, a, g, changed)
	if second.ID == first.ID {
		t.Fatal("schedule revision did not change")
	}
	tx, err := s.DB.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.GeneratePeriods(context.Background(), tx, brand, a, g.ID, from, to, "conflicting calendar", points.Metadata{})
	_ = tx.Rollback(context.Background())
	if !errors.Is(err, ErrState) {
		t.Fatalf("conflicting UTC period error=%v, want ErrState", err)
	}
	if n := concurrencyCount(t, s, `SELECT count(*) FROM periods WHERE game_id=$1`, g.ID); n != countBefore {
		t.Fatalf("conflicting generation left partial rows: before=%d after=%d", countBefore, n)
	}
	if concurrencyCount(t, s, `SELECT count(*) FROM periods WHERE game_id=$1 AND schedule_id=$2`, g.ID, second.ID) != 0 {
		t.Fatal("conflicting generation inserted a row for the new schedule")
	}
}

func TestPeriodScheduleAndWindowHistoryAreImmutable(t *testing.T) {
	s, a0, _, g, _, _ := fixture(t)
	a := scheduleActor(a0.ID)
	record := writeSchedule(t, s, a, g, dailySpec(g.Timezone, "23:59:00"))
	now := databaseNow(t, s)
	p := insertPendingPeriod(t, s, g, "immutable-window", 1, now.Add(time.Hour), now.Add(2*time.Hour), now.Add(3*time.Hour), record.ID)
	for _, query := range []struct {
		name string
		sql  string
		id   string
	}{
		{"schedule update", `UPDATE game_schedules SET spec='{}'::jsonb WHERE id=$1`, record.ID},
		{"schedule delete", `DELETE FROM game_schedules WHERE id=$1`, record.ID},
		{"period state", `UPDATE periods SET status='closed',version=version+1 WHERE id=$1`, p.ID},
		{"period time", `UPDATE periods SET bet_start_at=bet_start_at+interval '1 second',version=version+1 WHERE id=$1`, p.ID},
		{"period schedule", `UPDATE periods SET schedule_id=NULL,version=version+1 WHERE id=$1`, p.ID},
		{"period delete", `DELETE FROM periods WHERE id=$1`, p.ID},
	} {
		t.Run(query.name, func(t *testing.T) {
			if _, err := s.DB.Exec(context.Background(), query.sql, query.id); err == nil {
				t.Fatal("immutable schedule or period history mutation succeeded")
			}
		})
	}
}

func TestGeneratePeriodsPermissionAndBrandBoundaries(t *testing.T) {
	s, a0, _, g, _, _ := fixture(t)
	a := scheduleActor(a0.ID)
	writeSchedule(t, s, a, g, dailySpec(g.Timezone, "23:59:00"))
	now := databaseNow(t, s)
	from, to := now.Add(-time.Minute), now.Add(time.Hour)
	for _, tc := range []struct {
		name string
		b    string
		a    access.Account
		want error
	}{{"permission", brand, actor(a.ID), ErrDenied}, {"cross brand", "0199a000-0000-7000-8000-000000000002", scheduleActorForBrand(a.ID, "0199a000-0000-7000-8000-000000000002"), ErrNotFound}} {
		t.Run(tc.name, func(t *testing.T) {
			tx, err := s.DB.Begin(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.GeneratePeriods(context.Background(), tx, tc.b, tc.a, g.ID, from, to, "boundary", points.Metadata{})
			_ = tx.Rollback(context.Background())
			if !errors.Is(err, tc.want) {
				t.Fatalf("GeneratePeriods error=%v, want %v", err, tc.want)
			}
		})
	}
}

func TestFillCalendarReservesActiveGameForNextDayIdempotently(t *testing.T) {
	s, a0, _, g, _, _ := fixture(t)
	a := scheduleActor(a0.ID)
	ctx := context.Background()
	now := databaseNow(t, s)
	drawTime := now.In(mustLocation(t, g.Timezone)).Add(3 * time.Hour).Format("15:04:05")
	activeSchedule := writeSchedule(t, s, a, g, dailySpec(g.Timezone, drawTime))
	var pausedGame Game
	transact(t, s.DB, func(tx pgx.Tx) error {
		var err error
		pausedGame, err = s.CreateGame(ctx, tx, brand, a, "paused_calendar", "Paused Calendar", g.Model, g.Timezone, "create paused calendar game", points.Metadata{})
		return err
	})
	pausedSchedule := writeSchedule(t, s, a, pausedGame, dailySpec(g.Timezone, drawTime))
	if _, err := s.DB.Exec(ctx, `UPDATE games SET status='paused' WHERE id=$1`, pausedGame.ID); err != nil {
		t.Fatal(err)
	}

	created, err := s.FillCalendar(ctx)
	if err != nil || created != 1 {
		t.Fatalf("FillCalendar created=%d, err=%v; want one reservation", created, err)
	}
	periods, err := s.Periods(ctx, brand, g.ID, 100, 0)
	if err != nil || len(periods) != 1 {
		t.Fatalf("active game reservations=%+v, err=%v", periods, err)
	}
	period := periods[0]
	if period.Status != "pending" || period.ScheduleID != activeSchedule.ID || period.DrawAt.Before(now) || period.DrawAt.After(now.Add(24*time.Hour)) {
		t.Fatalf("FillCalendar reservation outside next-day window: %+v", period)
	}
	pausedPeriods, err := s.Periods(ctx, brand, pausedGame.ID, 100, 0)
	if err != nil || len(pausedPeriods) != 0 {
		t.Fatalf("paused game was filled: periods=%+v err=%v schedule=%s", pausedPeriods, err, pausedSchedule.ID)
	}
	if audits := concurrencyCount(t, s, `SELECT count(*) FROM audit_logs WHERE action='period.generate' AND resource_id=$1 AND actor_type='system'`, g.ID); audits != 1 {
		t.Fatalf("automatic generation audit count=%d, want 1", audits)
	}
	created, err = s.FillCalendar(ctx)
	if err != nil || created != 0 {
		t.Fatalf("repeat FillCalendar created=%d, err=%v; want no new reservations", created, err)
	}
	if n := concurrencyCount(t, s, `SELECT count(*) FROM periods WHERE game_id=$1`, g.ID); n != 1 {
		t.Fatalf("repeat FillCalendar changed row count to %d", n)
	}
	if n := concurrencyCount(t, s, `SELECT count(*) FROM audit_logs WHERE action='period.generate' AND resource_id=$1 AND actor_type='system'`, g.ID); n != 1 {
		t.Fatalf("repeat FillCalendar added audit rows: count=%d", n)
	}
}

func TestTickConcurrentWorkersOpenPendingPeriodOnce(t *testing.T) {
	s, a0, b0, g, p, d := fixture(t)
	a, b := scheduleActor(a0.ID), scheduleActor(b0.ID)
	pending := draftReady(t, s, a, p, d, "next_period")
	transact(t, s.DB, func(tx pgx.Tx) error {
		var err error
		pending, err = s.Review(context.Background(), tx, brand, b, pending.ID, pending.Version, true, true, "queue activation", points.Metadata{})
		return err
	})
	now := databaseNow(t, s)
	period := insertPendingPeriod(t, s, g, "tick-concurrent", 100, now.Add(-time.Minute), now.Add(time.Hour), now.Add(2*time.Hour), "")
	const workers = 8
	start := make(chan struct{})
	results := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := s.Tick(context.Background())
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	opened, err := scanPeriod(s.DB.QueryRow(context.Background(), `SELECT `+periodFields+` FROM periods WHERE id=$1`, period.ID))
	if err != nil || opened.Status != "betting" || opened.Version != 2 {
		t.Fatalf("period opening = %+v, err=%v", opened, err)
	}
	activated, err := s.GetVersion(context.Background(), brand, pending.ID)
	if err != nil || activated.Status != "active" || activated.EffectivePeriodID != period.ID || activated.EffectiveSequence == nil || *activated.EffectiveSequence != 1 {
		t.Fatalf("queued version activation = %+v, err=%v", activated, err)
	}
	game, err := scanGame(s.DB.QueryRow(context.Background(), `SELECT `+gameFields+` FROM games WHERE id=$1`, g.ID))
	if err != nil || game.StartedSequence != 1 {
		t.Fatalf("game sequence=%d, err=%v; want opening ordinal 1 despite reservation sequence 100", game.StartedSequence, err)
	}
	if n := concurrencyCount(t, s, `SELECT count(*) FROM audit_logs WHERE action='period.betting' AND resource_id=$1`, period.ID); n != 1 {
		t.Fatalf("period opening audits=%d, want 1", n)
	}
	if n := concurrencyCount(t, s, `SELECT count(*) FROM audit_logs WHERE action='rule.period.activate' AND request_id=$1`, period.ID); n != 1 {
		t.Fatalf("rule activation audits=%d, want 1", n)
	}
	if n := concurrencyCount(t, s, `SELECT count(*) FROM period_rule_versions WHERE period_id=$1 AND rule_version_id=$2`, period.ID, pending.ID); n != 1 {
		t.Fatalf("period rule snapshots=%d, want 1", n)
	}
}

func TestTickContinuesOtherGamesAfterOneGameFails(t *testing.T) {
	s, a, _, firstGame, _, _ := fixture(t)
	var secondGame Game
	transact(t, s.DB, func(tx pgx.Tx) error {
		var err error
		secondGame, err = s.CreateGame(context.Background(), tx, brand, a, "tick_isolation", "Tick Isolation", firstGame.Model, firstGame.Timezone, "create tick isolation game", points.Metadata{})
		return err
	})
	failedGame, healthyGame := firstGame, secondGame
	if secondGame.ID < firstGame.ID {
		failedGame, healthyGame = secondGame, firstGame
	}
	if _, err := s.DB.Exec(context.Background(), `UPDATE games SET started_sequence=9223372036854775807 WHERE id=$1`, failedGame.ID); err != nil {
		t.Fatal(err)
	}
	now := databaseNow(t, s)
	failedPeriod := insertPendingPeriod(t, s, failedGame, "tick-isolation-failed", 1, now.Add(-time.Minute), now.Add(time.Hour), now.Add(2*time.Hour), "")
	healthyPeriod := insertPendingPeriod(t, s, healthyGame, "tick-isolation-healthy", 1, now.Add(-time.Minute), now.Add(time.Hour), now.Add(2*time.Hour), "")
	opened, err := s.Tick(context.Background())
	if !errors.Is(err, ErrVersion) || opened != 1 {
		t.Fatalf("Tick count=%d err=%v; want one opening and ErrVersion", opened, err)
	}
	failed, err := scanPeriod(s.DB.QueryRow(context.Background(), `SELECT `+periodFields+` FROM periods WHERE id=$1`, failedPeriod.ID))
	if err != nil || failed.Status != "pending" || failed.Version != 1 {
		t.Fatalf("failed game period changed: %+v err=%v", failed, err)
	}
	healthy, err := scanPeriod(s.DB.QueryRow(context.Background(), `SELECT `+periodFields+` FROM periods WHERE id=$1`, healthyPeriod.ID))
	if err != nil || healthy.Status != "betting" || healthy.Version != 2 {
		t.Fatalf("healthy game was not processed after peer failure: %+v err=%v", healthy, err)
	}
}

func TestTickExpiredPendingDoesNotAdvanceActivationSequence(t *testing.T) {
	s, a0, b0, g, p, d := fixture(t)
	a, b := scheduleActor(a0.ID), scheduleActor(b0.ID)
	pending := draftReady(t, s, a, p, d, "next_period")
	transact(t, s.DB, func(tx pgx.Tx) error {
		var err error
		pending, err = s.Review(context.Background(), tx, brand, b, pending.ID, pending.Version, true, true, "queue activation", points.Metadata{})
		return err
	})
	now := databaseNow(t, s)
	period := insertPendingPeriod(t, s, g, "expired-before-open", 101, now.Add(-2*time.Hour), now.Add(-time.Hour), now.Add(-30*time.Minute), "")
	_, err := s.Tick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	closed, err := scanPeriod(s.DB.QueryRow(context.Background(), `SELECT `+periodFields+` FROM periods WHERE id=$1`, period.ID))
	if err != nil || closed.Status != "judged_cancelled" || closed.Version != 2 {
		t.Fatalf("expired pending period=%+v, err=%v", closed, err)
	}
	current, err := s.GetVersion(context.Background(), brand, pending.ID)
	if err != nil || current.Status != "approved" {
		t.Fatalf("expired pending period activated queued rule: %+v, err=%v", current, err)
	}
	game, err := scanGame(s.DB.QueryRow(context.Background(), `SELECT `+gameFields+` FROM games WHERE id=$1`, g.ID))
	if err != nil || game.StartedSequence != 0 {
		t.Fatalf("expired reservation advanced started sequence to %d, err=%v", game.StartedSequence, err)
	}
	if concurrencyCount(t, s, `SELECT count(*) FROM audit_logs WHERE action='period.judged_cancelled' AND resource_id=$1`, period.ID) != 1 {
		t.Fatal("expired pending cancellation audit missing")
	}
}

func TestTickClosesThenWaitsForDrawAndPauseStillCloses(t *testing.T) {
	s, _, _, g, _, _ := fixture(t)
	now := databaseNow(t, s)
	closed := insertPendingPeriod(t, s, g, "draw-already-past", 1, now.Add(-2*time.Hour), now.Add(-time.Hour), now.Add(-30*time.Minute), "")
	if _, err := s.DB.Exec(context.Background(), `UPDATE periods SET status='betting',version=version+1 WHERE id=$1`, closed.ID); err != nil {
		t.Fatal(err)
	}
	paused := insertPendingPeriod(t, s, g, "paused-betting", 2, now.Add(-2*time.Hour), now.Add(-time.Minute), now.Add(time.Hour), "")
	if _, err := s.DB.Exec(context.Background(), `UPDATE periods SET status='betting',version=version+1 WHERE id=$1`, paused.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(context.Background(), `UPDATE games SET status='paused' WHERE id=$1`, g.ID); err != nil {
		t.Fatal(err)
	}
	pending := insertPendingPeriod(t, s, g, "paused-pending", 3, now.Add(-time.Minute), now.Add(time.Hour), now.Add(2*time.Hour), "")
	if _, err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := scanPeriod(s.DB.QueryRow(context.Background(), `SELECT `+periodFields+` FROM periods WHERE id=$1`, closed.ID))
	if err != nil || got.Status != "waiting_draw" || got.Version != 4 {
		t.Fatalf("past draw transition=%+v, err=%v", got, err)
	}
	got, err = scanPeriod(s.DB.QueryRow(context.Background(), `SELECT `+periodFields+` FROM periods WHERE id=$1`, paused.ID))
	if err != nil || got.Status != "closed" || got.Version != 3 {
		t.Fatalf("paused game did not close an already-open period: %+v, err=%v", got, err)
	}
	got, err = scanPeriod(s.DB.QueryRow(context.Background(), `SELECT `+periodFields+` FROM periods WHERE id=$1`, pending.ID))
	if err != nil || got.Status != "pending" || got.Version != 1 {
		t.Fatalf("paused game opened a pending period: %+v, err=%v", got, err)
	}
	rows, err := s.DB.Query(context.Background(), `SELECT action,(before_json->>'version')::bigint,(after_json->'period'->>'version')::bigint FROM audit_logs WHERE resource_id=$1 ORDER BY (after_json->'period'->>'version')::bigint`, closed.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type transition struct {
		action        string
		before, after int64
	}
	transitions := []transition{}
	for rows.Next() {
		var action string
		var before, after int64
		if err := rows.Scan(&action, &before, &after); err != nil {
			t.Fatal(err)
		}
		transitions = append(transitions, transition{action: action, before: before, after: after})
		if (len(transitions) == 1 && action != "period.closed") || (len(transitions) == 2 && action != "period.waiting_draw") {
			t.Fatalf("transition audit sequence action=%q rows=%v", action, transitions)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(transitions) != 2 || transitions[0].before != 2 || transitions[0].after != 3 || transitions[1].before != 3 || transitions[1].after != 4 {
		t.Fatalf("transition audit version chain=%v", transitions)
	}
}

func TestTickRollbackLeavesStatePointersSnapshotsAndAuditsUntouched(t *testing.T) {
	s, a0, b0, g, p, d := fixture(t)
	a, b := scheduleActor(a0.ID), scheduleActor(b0.ID)
	pending := draftReady(t, s, a, p, d, "next_period")
	transact(t, s.DB, func(tx pgx.Tx) error {
		var err error
		pending, err = s.Review(context.Background(), tx, brand, b, pending.ID, pending.Version, true, true, "queue activation", points.Metadata{})
		return err
	})
	now := databaseNow(t, s)
	period := insertPendingPeriod(t, s, g, "forced-tick-rollback", 1, now.Add(-time.Minute), now.Add(time.Hour), now.Add(2*time.Hour), "")
	// This deliberate preexisting duplicate makes bindOpening fail after the period
	// transition and rule activation have begun; the caller's transaction must undo all of it.
	if _, err := s.DB.Exec(context.Background(), `INSERT INTO period_rule_versions(brand_id,game_id,period_id,play_id,rule_version_id) VALUES($1,$2,$3,$4,$5)`, brand, g.ID, period.ID, p.ID, pending.ID); err != nil {
		t.Fatal(err)
	}
	beforeAudits := concurrencyCount(t, s, `SELECT count(*) FROM audit_logs`)
	tx, err := s.DB.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = tickGame(context.Background(), tx, brand, g.ID)
	if err == nil {
		_ = tx.Rollback(context.Background())
		t.Fatal("tickGame unexpectedly accepted duplicate snapshot")
	}
	_ = tx.Rollback(context.Background())
	got, err := scanPeriod(s.DB.QueryRow(context.Background(), `SELECT `+periodFields+` FROM periods WHERE id=$1`, period.ID))
	if err != nil || got.Status != "pending" || got.Version != 1 {
		t.Fatalf("failed tick changed period state: %+v, err=%v", got, err)
	}
	current, err := s.GetVersion(context.Background(), brand, pending.ID)
	if err != nil || current.Status != "approved" || current.EffectiveAt != nil || current.EffectivePeriodID != "" {
		t.Fatalf("failed tick changed queued rule: %+v, err=%v", current, err)
	}
	game, err := scanGame(s.DB.QueryRow(context.Background(), `SELECT `+gameFields+` FROM games WHERE id=$1`, g.ID))
	if err != nil || game.StartedSequence != 0 {
		t.Fatalf("failed tick changed game sequence=%d, err=%v", game.StartedSequence, err)
	}
	if after := concurrencyCount(t, s, `SELECT count(*) FROM audit_logs`); after != beforeAudits {
		t.Fatalf("failed tick retained audit rows: before=%d after=%d", beforeAudits, after)
	}
	if n := concurrencyCount(t, s, `SELECT count(*) FROM period_rule_versions WHERE period_id=$1`, period.ID); n != 1 {
		t.Fatalf("failed tick changed preexisting snapshot count to %d", n)
	}
}

func TestScheduleGenerateSQLCrossBrandIsolation(t *testing.T) {
	s, a0, _, g, _, _ := fixture(t)
	a := scheduleActor(a0.ID)
	writeSchedule(t, s, a, g, dailySpec(g.Timezone, "23:59:00"))
	if _, err := s.Schedule(context.Background(), "0199a000-0000-7000-8000-000000000002", g.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-brand schedule lookup error=%v", err)
	}
	if _, err := s.Periods(context.Background(), "0199a000-0000-7000-8000-000000000002", g.ID, 100, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-brand period lookup error=%v", err)
	}
}
