package dataretention

import (
	"errors"
	"testing"
	"time"
)

func TestMonthlyCutoff(t *testing.T) {
	for _, test := range []struct{ name, now, timezone, execution, cutoff string }{
		{"before run", "2026-11-01T00:00:00Z", "Asia/Singapore", "2026-11-09T18:00:00Z", "2026-09-30T16:00:00Z"},
		{"at run", "2026-11-09T18:00:00Z", "Asia/Singapore", "2026-12-09T18:00:00Z", "2026-10-31T16:00:00Z"},
		{"year boundary", "2026-12-31T20:00:00Z", "Asia/Singapore", "2027-01-09T18:00:00Z", "2026-11-30T16:00:00Z"},
		{"leap february", "2028-02-01T00:00:00Z", "UTC", "2028-02-10T02:00:00Z", "2028-01-01T00:00:00Z"},
		{"DST", "2026-03-01T00:00:00Z", "America/New_York", "2026-03-10T06:00:00Z", "2026-02-01T05:00:00Z"},
	} {
		t.Run(test.name, func(t *testing.T) {
			now, err := time.Parse(time.RFC3339, test.now)
			if err != nil {
				t.Fatal(err)
			}
			run, err := (Schedule{ExecutionDay: 10, ExecutionTime: "02:00"}).NextRun(now, test.timezone)
			if err != nil {
				t.Fatal(err)
			}
			if run.ExecuteAt.Format(time.RFC3339) != test.execution || run.CutoffAt.Format(time.RFC3339) != test.cutoff {
				t.Fatalf("unexpected run: %+v", run)
			}
		})
	}
}

func TestScheduleRejectsInvalidInput(t *testing.T) {
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for _, s := range []Schedule{{0, "02:00"}, {29, "02:00"}, {10, "2:00"}, {10, "24:00"}, {10, "02:60"}} {
		if _, err := s.NextRun(now, "UTC"); !errors.Is(err, ErrSchedule) {
			t.Fatalf("accepted %+v", s)
		}
	}
	for _, timezone := range []string{"", "Local", "invalid/timezone"} {
		if _, err := (Schedule{10, "02:00"}).NextRun(now, timezone); !errors.Is(err, ErrSchedule) {
			t.Fatalf("accepted %q", timezone)
		}
	}
	// New York has no 02:30 on March 8, 2026.
	if _, err := (Schedule{8, "02:30"}).NextRun(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), "America/New_York"); !errors.Is(err, ErrSchedule) {
		t.Fatalf("nonexistent clock accepted: %v", err)
	}
	if run, err := (Schedule{8, "02:30"}).NextRun(time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC), "America/New_York"); err != nil || run.ExecuteAt.Format(time.RFC3339) != "2026-04-08T06:30:00Z" {
		t.Fatalf("past missing wall clock blocked a valid future execution: %+v %v", run, err)
	}
}

func TestHistoricalRangeUsesFrozenLocalDays(t *testing.T) {
	parse := func(raw string) time.Time {
		t.Helper()
		v, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, test := range []struct {
		from, to, timezone string
		valid              bool
	}{
		{"2026-09-30T16:00:00Z", "2026-10-31T16:00:00Z", "Asia/Singapore", true},
		{"2026-10-01T00:00:00+08:00", "2026-10-02T00:00:00+08:00", "Asia/Singapore", true},
		{"2026-10-01T00:00:00Z", "2026-10-02T00:00:00Z", "Asia/Singapore", false},
		{"2026-10-01T00:00:00.000000001+08:00", "2026-10-02T00:00:00+08:00", "Asia/Singapore", false},
		{"2026-03-08T05:00:00Z", "2026-03-09T04:00:00Z", "America/New_York", true},
		{"2026-11-01T04:00:00Z", "2026-11-02T05:00:00Z", "America/New_York", true},
		{"2026-10-01T00:00:00Z", "2026-10-01T00:00:00Z", "UTC", false},
		{"2026-10-02T00:00:00Z", "2026-10-01T00:00:00Z", "UTC", false},
		{"2026-10-01T00:00:00Z", "2026-10-02T00:00:00Z", "Local", false},
	} {
		err := ValidateHistoricalRange(parse(test.from), parse(test.to), test.timezone)
		if (err == nil) != test.valid {
			t.Fatalf("%+v: %v", test, err)
		}
	}
}
