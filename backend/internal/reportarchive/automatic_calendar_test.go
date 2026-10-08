package reportarchive

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestActivationPeriodsUsesLocalCivilCalendar(t *testing.T) {
	at := time.Date(2024, 1, 1, 1, 30, 0, 0, time.UTC)
	daily, monthly, err := ActivationPeriods(at, "America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	if daily != "2023-12-31" || monthly != "2023-12" {
		t.Fatalf("ActivationPeriods() = (%q, %q), want (2023-12-31, 2023-12)", daily, monthly)
	}
	dailyAgain, monthlyAgain, err := ActivationPeriods(at, "America/Los_Angeles")
	if err != nil || dailyAgain != daily || monthlyAgain != monthly {
		t.Fatalf("repeated ActivationPeriods() = (%q, %q, %v), want same keys", dailyAgain, monthlyAgain, err)
	}
}

func TestActivationPeriodsRejectsInvalidInputsAndUTCUnderflow(t *testing.T) {
	tests := []struct {
		name     string
		at       time.Time
		timezone string
	}{
		{"zero time", time.Time{}, "UTC"},
		{"local alias", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), "Local"},
		{"bad timezone", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), "Mars/Olympus"},
		{"UTC lower boundary", time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC), "Asia/Kolkata"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ActivationPeriods(tt.at, tt.timezone)
			if !errors.Is(err, ErrWindowInvalid) {
				t.Fatalf("ActivationPeriods() error = %v, want ErrWindowInvalid", err)
			}
		})
	}
}

func TestNextArchivePeriodCalendarArithmeticAndYearCap(t *testing.T) {
	tests := []struct{ kind, key, want string }{
		{Daily, "2024-02-28", "2024-02-29"},
		{Daily, "2024-02-29", "2024-03-01"},
		{Daily, "2024-03-10", "2024-03-11"},
		{Monthly, "2024-02", "2024-03"},
		{Monthly, "2024-12", "2025-01"},
	}
	for _, tt := range tests {
		got, err := NextArchivePeriod(tt.kind, tt.key)
		if err != nil || got != tt.want {
			t.Errorf("NextArchivePeriod(%q, %q) = (%q, %v), want %q", tt.kind, tt.key, got, err, tt.want)
		}
	}
	for _, tt := range []struct{ kind, key string }{
		{Daily, "2024-2-01"}, {Daily, "2023-02-29"}, {Monthly, "2024-13"},
		{Monthly, "0000-12"}, {Daily, "9999-01-01"}, {"weekly", "2024-01-01"},
		{Daily, "9998-12-31"}, {Monthly, "9998-12"},
	} {
		if got, err := NextArchivePeriod(tt.kind, tt.key); !errors.Is(err, ErrWindowInvalid) {
			t.Errorf("NextArchivePeriod(%q, %q) = (%q, %v), want ErrWindowInvalid", tt.kind, tt.key, got, err)
		}
	}
}

func TestDueArchiveWindowsDailyDSTWindows(t *testing.T) {
	tests := []struct {
		name, key, zone string
		hours           float64
		asOf            time.Time
	}{
		{"spring forward", "2024-03-10", "America/New_York", 23, time.Date(2024, 3, 11, 4, 0, 0, 0, time.UTC)},
		{"fall back", "2024-11-03", "America/New_York", 25, time.Date(2024, 11, 4, 5, 0, 0, 0, time.UTC)},
		{"Havana fold", "2020-11-01", "America/Havana", 25, time.Date(2020, 11, 2, 5, 0, 0, 0, time.UTC)},
		{"Sao Paulo midnight gap", "2018-11-04", "America/Sao_Paulo", 23, time.Date(2018, 11, 5, 3, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			windows, next, err := DueArchiveWindows(Daily, tt.key, tt.zone, tt.asOf, 1)
			if err != nil || len(windows) != 1 || windows[0].PeriodKey != tt.key || windows[0].To.Sub(windows[0].From).Hours() != tt.hours || next == tt.key {
				t.Fatalf("DueArchiveWindows() = (%+v, %q, %v)", windows, next, err)
			}
		})
	}
}

func TestDueArchiveWindowsSkipsApiaSkippedDateAndKeepsCursor(t *testing.T) {
	asOf := time.Date(2012, 1, 1, 12, 0, 0, 0, time.UTC)
	windows, next, err := DueArchiveWindows(Daily, "2011-12-29", "Pacific/Apia", asOf, 10)
	if err != nil {
		t.Fatal(err)
	}
	keys := make([]string, len(windows))
	for i, window := range windows {
		keys[i] = window.PeriodKey
	}
	if !reflect.DeepEqual(keys, []string{"2011-12-29", "2011-12-31", "2012-01-01"}) || next != "2012-01-02" {
		t.Fatalf("windows/cursor = (%v, %q), want ([2011-12-29 2011-12-31 2012-01-01], 2012-01-02)", keys, next)
	}
	// Repeating from the returned cursor is stable and does not replay a window.
	again, nextAgain, err := DueArchiveWindows(Daily, next, "Pacific/Apia", asOf, 10)
	if err != nil || len(again) != 0 || nextAgain != next {
		t.Fatalf("repeat from cursor = (%+v, %q, %v), want ([], %q, nil)", again, nextAgain, err, next)
	}
}

func TestDueArchiveWindowsMonthlyLeapAndMidnightCutoff(t *testing.T) {
	asOf := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	windows, next, err := DueArchiveWindows(Monthly, "2024-02", "UTC", asOf, 10)
	if err != nil || len(windows) != 1 || windows[0].To.Sub(windows[0].From) != 29*24*time.Hour || next != "2024-03" {
		t.Fatalf("leap-month due result = (%+v, %q, %v)", windows, next, err)
	}
	beforeEnd := asOf.Add(-time.Nanosecond)
	windows, next, err = DueArchiveWindows(Monthly, "2024-02", "UTC", beforeEnd, 10)
	if err != nil || len(windows) != 0 || next != "2024-02" {
		t.Fatalf("before-boundary result = (%+v, %q, %v), want no windows and unchanged cursor", windows, next, err)
	}
}

func TestDueArchiveWindowsLimitCanonicalValidationAndPureRepeat(t *testing.T) {
	for _, limit := range []int{0, -1, 101} {
		if _, _, err := DueArchiveWindows(Daily, "2024-01-01", "UTC", time.Now(), limit); !errors.Is(err, ErrWindowInvalid) {
			t.Errorf("limit %d error = %v, want ErrWindowInvalid", limit, err)
		}
	}
	for _, tc := range []struct{ kind, key, zone string }{
		{Daily, "2024-1-01", "UTC"}, {Daily, "2024-01-01", "Local"},
		{Monthly, "2024-00", "UTC"}, {"weekly", "2024-01-01", "UTC"},
	} {
		if _, _, err := DueArchiveWindows(tc.kind, tc.key, tc.zone, time.Now(), 1); !errors.Is(err, ErrWindowInvalid) {
			t.Errorf("invalid input (%q,%q,%q) error = %v", tc.kind, tc.key, tc.zone, err)
		}
	}
	if _, _, err := DueArchiveWindows(Daily, "0001-01-01", "Asia/Kolkata", time.Date(1, 1, 3, 0, 0, 0, 0, time.UTC), 1); !errors.Is(err, ErrWindowInvalid) {
		t.Errorf("UTC boundary error = %v, want ErrWindowInvalid", err)
	}

	asOf := time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC)
	one, cursor, err := DueArchiveWindows(Daily, "2024-01-01", "UTC", asOf, 2)
	if err != nil || len(one) != 2 || cursor != "2024-01-03" {
		t.Fatalf("bounded result = (%+v, %q, %v)", one, cursor, err)
	}
	two, cursorAgain, err := DueArchiveWindows(Daily, "2024-01-01", "UTC", asOf, 2)
	if err != nil || !reflect.DeepEqual(one, two) || cursorAgain != cursor {
		t.Fatalf("repeated pure result differs: first=(%+v,%q), second=(%+v,%q), err=%v", one, cursor, two, cursorAgain, err)
	}
}
