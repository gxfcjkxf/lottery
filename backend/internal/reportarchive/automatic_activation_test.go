package reportarchive

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

func TestActivationStartPeriodsUsesBrandCivilDateAndRetainsSavedStarts(t *testing.T) {
	tests := []struct {
		name      string
		zone      string
		at        time.Time
		before    AutomaticPolicy
		in        AutomaticActivationInput
		wantDaily *string
		wantMonth *string
	}{
		{
			name:      "Kiritimati crosses into next UTC day and month",
			zone:      "Pacific/Kiritimati",
			at:        time.Date(2024, 1, 31, 12, 30, 0, 0, time.UTC),
			in:        AutomaticActivationInput{DailyEnabled: true, MonthlyEnabled: true},
			wantDaily: strptr("2024-02-01"), wantMonth: strptr("2024-02"),
		},
		{
			name:      "Los Angeles remains on prior UTC date and month",
			zone:      "America/Los_Angeles",
			at:        time.Date(2024, 2, 1, 1, 30, 0, 0, time.UTC),
			in:        AutomaticActivationInput{DailyEnabled: true, MonthlyEnabled: true},
			wantDaily: strptr("2024-01-31"), wantMonth: strptr("2024-01"),
		},
		{
			name:      "turning off keeps saved first starts",
			zone:      "UTC",
			at:        time.Date(2024, 5, 12, 8, 0, 0, 0, time.UTC),
			before:    AutomaticPolicy{DailyStartPeriod: strptr("2024-02-01"), MonthlyStartPeriod: strptr("2024-01")},
			in:        AutomaticActivationInput{},
			wantDaily: strptr("2024-02-01"), wantMonth: strptr("2024-01"),
		},
		{
			name:      "reenabling keeps saved first starts",
			zone:      "UTC",
			at:        time.Date(2024, 5, 12, 8, 0, 0, 0, time.UTC),
			before:    AutomaticPolicy{DailyStartPeriod: strptr("2024-02-01"), MonthlyStartPeriod: strptr("2024-01")},
			in:        AutomaticActivationInput{DailyEnabled: true, MonthlyEnabled: true},
			wantDaily: strptr("2024-02-01"), wantMonth: strptr("2024-01"),
		},
		{
			name: "never enabled kinds stay null",
			zone: "UTC",
			at:   time.Date(2024, 5, 12, 8, 0, 0, 0, time.UTC),
			in:   AutomaticActivationInput{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			daily, monthly, err := activationStartPeriods(tt.before, tt.in, tt.zone, tt.at)
			if err != nil {
				t.Fatal(err)
			}
			if !sameStringPtr(daily, tt.wantDaily) || !sameStringPtr(monthly, tt.wantMonth) {
				t.Fatalf("activationStartPeriods() = (%v, %v), want (%v, %v)", daily, monthly, tt.wantDaily, tt.wantMonth)
			}
		})
	}
}

func TestActivationStartPeriodsRejectsBadCalendarInputs(t *testing.T) {
	for _, tt := range []struct {
		name string
		zone string
		at   time.Time
	}{
		{name: "invalid zone", zone: "Mars/Olympus", at: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{name: "local alias", zone: "Local", at: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{name: "zero time", zone: "UTC"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := activationStartPeriods(AutomaticPolicy{}, AutomaticActivationInput{DailyEnabled: true}, tt.zone, tt.at)
			if err == nil {
				t.Fatal("activationStartPeriods() accepted invalid calendar input")
			}
		})
	}
}

func TestUpdateAutomaticActivationTxKeepsStartsAndDoesNotImmediatelyDiscoverCurrentPeriods(t *testing.T) {
	t.Parallel()
	s, a := archiveFixture(t)
	a = automaticActor(a)
	ctx := context.Background()
	var zone string
	if err := s.DB.QueryRow(ctx, `SELECT timezone FROM brands WHERE id=$1`, testBrand).Scan(&zone); err != nil {
		t.Fatal(err)
	}
	dbNow := time.Now()
	if err := s.DB.QueryRow(ctx, `SELECT statement_timestamp()`).Scan(&dbNow); err != nil {
		t.Fatal(err)
	}
	wantDaily, wantMonthly, err := ActivationPeriods(dbNow, zone)
	if err != nil {
		t.Fatal(err)
	}
	update := func(in AutomaticActivationInput) AutomaticPolicy {
		t.Helper()
		tx, err := s.DB.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		out, err := s.UpdateAutomaticActivationTx(ctx, tx, testBrand, a, in, archiveMeta(a))
		if err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return out
	}
	first := update(AutomaticActivationInput{Version: 1, DailyEnabled: true, MonthlyEnabled: true, Reason: "activate current reporting periods"})
	if first.Version != 2 || first.DailyStartPeriod == nil || *first.DailyStartPeriod != wantDaily || first.MonthlyStartPeriod == nil || *first.MonthlyStartPeriod != wantMonthly {
		t.Fatalf("first activation = %+v, want version 2 and current brand-local day/month", first)
	}
	if n, err := s.DiscoverAutomatic(ctx, 20); err != nil || n != 0 {
		t.Fatalf("current incomplete periods must not create immediate tasks: count=%d err=%v", n, err)
	}
	off := update(AutomaticActivationInput{Version: 2, Reason: "temporarily disable automatic archives"})
	if off.Version != 3 || off.DailyEnabled || off.MonthlyEnabled || !sameStringPtr(off.DailyStartPeriod, first.DailyStartPeriod) || !sameStringPtr(off.MonthlyStartPeriod, first.MonthlyStartPeriod) {
		t.Fatalf("disable did not retain first starts: %+v", off)
	}
	on := update(AutomaticActivationInput{Version: 3, DailyEnabled: true, MonthlyEnabled: true, Reason: "resume automatic archives"})
	if on.Version != 4 || !sameStringPtr(on.DailyStartPeriod, first.DailyStartPeriod) || !sameStringPtr(on.MonthlyStartPeriod, first.MonthlyStartPeriod) {
		t.Fatalf("reenable moved first starts: first=%+v resumed=%+v", first, on)
	}
	var cursors, tasks int
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM report_archive_automatic_cursors WHERE brand_id=$1 AND policy_version=4`, testBrand).Scan(&cursors); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM report_archive_automatic_tasks WHERE brand_id=$1`, testBrand).Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if cursors != 2 || tasks != 0 {
		t.Fatalf("current activation cursor/task state = (%d cursors, %d tasks), want (2, 0)", cursors, tasks)
	}
}

func TestUpdateAutomaticActivationTxRejectsStaleVersionWithoutPartialWrites(t *testing.T) {
	t.Parallel()
	s, a := archiveFixture(t)
	a = automaticActor(a)
	ctx := context.Background()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	_, err = s.UpdateAutomaticActivationTx(ctx, tx, testBrand, a, AutomaticActivationInput{
		Version: 2, DailyEnabled: true, Reason: "stale activation must not write",
	}, points.Metadata{ActorType: "admin", ActorID: a.ID, RequestID: ids.New()})
	if !errors.Is(err, ErrVersion) {
		t.Fatalf("stale update error = %v, want ErrVersion", err)
	}
	var version int64
	var cursors int
	if err = tx.QueryRow(ctx, `SELECT version FROM brand_report_archive_policies WHERE brand_id=$1`, testBrand).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM report_archive_automatic_cursors WHERE brand_id=$1`, testBrand).Scan(&cursors); err != nil {
		t.Fatal(err)
	}
	if version != 1 || cursors != 0 {
		t.Fatalf("stale update partially wrote policy/cursor: version=%d cursors=%d", version, cursors)
	}
}

func strptr(v string) *string { return &v }

func sameStringPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
