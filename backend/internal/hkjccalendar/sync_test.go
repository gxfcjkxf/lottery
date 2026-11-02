package hkjccalendar

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

type fetchFunc func(context.Context) ([]Month, error)

func (f fetchFunc) Fetch(ctx context.Context) ([]Month, error) { return f(ctx) }

func TestSyncDailyReconciliationAndFailurePreservesCalendar(t *testing.T) {
	db := testdb.NewUnseeded(t)
	ctx := context.Background()
	var now time.Time
	if err := db.QueryRow(ctx, `SELECT date_trunc('month',clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong') AT TIME ZONE 'UTC'`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	month := now.AddDate(0, 1, 0)
	first, second := month.Format("2006-01-02"), month.AddDate(0, 0, 1).Format("2006-01-02")
	months := []Month{{month, []string{first, second}, []string{second}, []string{first}}}
	calls := 0
	var fetchErr error
	s := Service{DB: db, Source: fetchFunc(func(context.Context) ([]Month, error) { calls++; return months, fetchErr })}
	if n, err := s.SyncDue(ctx); err != nil || n != 1 {
		t.Fatalf("first sync %d %v", n, err)
	}
	if n, err := s.SyncDue(ctx); err != nil || n != 0 || calls != 1 {
		t.Fatalf("not due %d %v calls=%d", n, err, calls)
	}
	var hour int
	var withinDay bool
	if err := db.QueryRow(ctx, `SELECT extract(hour FROM next_attempt_at AT TIME ZONE 'Asia/Hong_Kong')::int,next_attempt_at>last_attempt_at AND next_attempt_at<=last_attempt_at+interval '24 hours' FROM hkjc_calendar_sync`).Scan(&hour, &withinDay); err != nil || hour != 4 || !withinDay {
		t.Fatalf("daily schedule %d %v %v", hour, withinDay, err)
	}
	forceDue := func() {
		t.Helper()
		if _, err := db.Exec(ctx, `UPDATE hkjc_calendar_sync SET next_attempt_at=clock_timestamp()-interval '1 second'`); err != nil {
			t.Fatal(err)
		}
	}
	readDates := func() []string {
		t.Helper()
		var dates []string
		if err := db.QueryRow(ctx, `SELECT draw_dates::text[] FROM hkjc_calendar_months WHERE month=$1`, month).Scan(&dates); err != nil {
			t.Fatal(err)
		}
		return dates
	}
	// A changed announcement removes the old day, rather than accumulating it.
	months[0].DrawDates = []string{first}
	months[0].SnowballDates = []string{}
	forceDue()
	if _, err := s.SyncDue(ctx); err != nil {
		t.Fatal(err)
	}
	if got := readDates(); !reflect.DeepEqual(got, []string{first}) {
		t.Fatal(got)
	}
	fetchErr = errors.New("source unavailable")
	forceDue()
	if _, err := s.SyncDue(ctx); !errors.Is(err, fetchErr) {
		t.Fatalf("source failure was not recorded: %v", err)
	}
	if got := readDates(); !reflect.DeepEqual(got, []string{first}) {
		t.Fatal("failed sync changed calendar", got)
	}
	var code string
	var retry bool
	if err := db.QueryRow(ctx, `SELECT last_error_code,next_attempt_at=last_attempt_at+interval '1 hour' AND last_success_at IS NOT NULL FROM hkjc_calendar_sync`).Scan(&code, &retry); err != nil || code != "HKJC_CALENDAR_FETCH_FAILED" || !retry {
		t.Fatalf("failure state %s %v %v", code, retry, err)
	}
	// Invalid persisted data rolls back the whole replacement, including deletion.
	fetchErr = nil
	months[0].SnowballDates = []string{second}
	forceDue()
	if _, err := s.SyncDue(ctx); err == nil {
		t.Fatal("invalid snowball accepted")
	}
	if got := readDates(); !reflect.DeepEqual(got, []string{first}) {
		t.Fatal("transaction did not roll back", got)
	}
	months[0].SnowballDates = []string{}
	forceDue()
	if _, err := s.SyncDue(ctx); err != nil {
		t.Fatal(err)
	}
	// No game, automation settings, betting period or result is created.
	var count int
	if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM platform_games)+(SELECT count(*) FROM platform_game_periods)+(SELECT count(*) FROM platform_automation_versions)`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("business tables changed: %d %v", count, err)
	}
	// A removed future month disappears from the mirror; past months are retained.
	months = []Month{{now.AddDate(0, -1, 0), []string{}, []string{}, []string{}}}
	forceDue()
	if _, err := s.SyncDue(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM hkjc_calendar_months WHERE month=$1`, month).Scan(&count); err != nil || count != 0 {
		t.Fatal("unpublished month retained", err)
	}
}

func TestConcurrentWorkersFetchOnce(t *testing.T) {
	db := testdb.NewUnseeded(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	started, release := make(chan struct{}), make(chan struct{})
	s := Service{DB: db, Source: fetchFunc(func(ctx context.Context) ([]Month, error) {
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return []Month{{time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), []string{"2026-11-03"}, []string{}, []string{}}}, nil
	})}
	result := make(chan error, 1)
	go func() { _, err := s.SyncDue(ctx); result <- err }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if n, err := s.SyncDue(ctx); err != nil || n != 0 {
		t.Fatalf("concurrent worker %d %v", n, err)
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}
