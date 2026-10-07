package commission

import (
	"errors"
	"testing"
	"time"
)

func intPtr(v int) *int { return &v }

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("time.Parse(%q): %v", value, err)
	}
	return parsed
}

func assertWindow(t *testing.T, got Window, from, to string) {
	t.Helper()
	wantFrom, wantTo := mustTime(t, from), mustTime(t, to)
	if !got.From.Equal(wantFrom) || !got.To.Equal(wantTo) {
		t.Fatalf("WindowAt() = [%s, %s), want [%s, %s)", got.From.Format(time.RFC3339), got.To.Format(time.RFC3339), from, to)
	}
	if got.From.Location() != time.UTC || got.To.Location() != time.UTC {
		t.Fatalf("WindowAt() locations = %s, %s; want UTC", got.From.Location(), got.To.Location())
	}
}

func TestCalendarWeeklyManilaBoundaryAndHalfOpenMembership(t *testing.T) {
	c := Calendar{Timezone: "Asia/Manila", Cycle: "weekly", BoundaryTime: "18:30:00", Weekday: intPtr(3)}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	tests := []struct {
		name string
		at   string
	}{
		{name: "just before", at: "2024-01-10T10:29:59Z"},
		{name: "at boundary", at: "2024-01-10T10:30:00Z"},
		{name: "just after", at: "2024-01-10T10:30:01Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			window, err := c.WindowAt(mustTime(t, tt.at))
			if err != nil {
				t.Fatalf("WindowAt() error = %v", err)
			}
			if tt.name == "just before" {
				assertWindow(t, window, "2024-01-03T10:30:00Z", "2024-01-10T10:30:00Z")
			} else {
				assertWindow(t, window, "2024-01-10T10:30:00Z", "2024-01-17T10:30:00Z")
			}
		})
	}
}

func TestCalendarMonthlyManilaLeapYearAndShortMonthPolicies(t *testing.T) {
	t.Run("last day clamps February in leap year", func(t *testing.T) {
		c := Calendar{Timezone: "Asia/Manila", Cycle: "monthly", BoundaryTime: "06:07:08", MonthDay: intPtr(31), ShortMonth: "last_day"}
		window, err := c.WindowAt(mustTime(t, "2024-02-29T00:00:00Z"))
		if err != nil {
			t.Fatalf("WindowAt() error = %v", err)
		}
		assertWindow(t, window, "2024-02-28T22:07:08Z", "2024-03-30T22:07:08Z")
	})

	t.Run("skip February day 31", func(t *testing.T) {
		c := Calendar{Timezone: "Asia/Manila", Cycle: "monthly", BoundaryTime: "06:07:08", MonthDay: intPtr(31), ShortMonth: "skip"}
		window, err := c.WindowAt(mustTime(t, "2024-02-29T00:00:00Z"))
		if err != nil {
			t.Fatalf("WindowAt() error = %v", err)
		}
		assertWindow(t, window, "2024-01-30T22:07:08Z", "2024-03-30T22:07:08Z")
	})

	t.Run("at day 31 cutoff starts new month", func(t *testing.T) {
		c := Calendar{Timezone: "Asia/Manila", Cycle: "monthly", BoundaryTime: "06:07:08", MonthDay: intPtr(31), ShortMonth: "last_day"}
		window, err := c.WindowAt(mustTime(t, "2024-03-30T22:07:08Z"))
		if err != nil {
			t.Fatalf("WindowAt() error = %v", err)
		}
		assertWindow(t, window, "2024-03-30T22:07:08Z", "2024-04-29T22:07:08Z")
	})
}

func TestCalendarDSTWeeklySpansUseWallCalendar(t *testing.T) {
	c := Calendar{Timezone: "America/New_York", Cycle: "weekly", BoundaryTime: "03:00:00", Weekday: intPtr(0)}
	tests := []struct {
		name  string
		at    string
		from  string
		to    string
		hours float64
	}{
		{name: "spring forward", at: "2024-03-06T12:00:00Z", from: "2024-03-03T08:00:00Z", to: "2024-03-10T07:00:00Z", hours: 167},
		{name: "fall back", at: "2024-10-30T12:00:00Z", from: "2024-10-27T07:00:00Z", to: "2024-11-03T08:00:00Z", hours: 169},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			window, err := c.WindowAt(mustTime(t, tt.at))
			if err != nil {
				t.Fatalf("WindowAt() error = %v", err)
			}
			assertWindow(t, window, tt.from, tt.to)
			if hours := window.To.Sub(window.From).Hours(); hours != tt.hours {
				t.Fatalf("window duration = %v hours, want %v", hours, tt.hours)
			}
		})
	}
}

func TestCalendarRejectsNonexistentAndAmbiguousDSTBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		clock string
		at    string
	}{
		{name: "nonexistent spring cutoff", clock: "02:30:00", at: "2024-03-11T12:00:00Z"},
		{name: "ambiguous fall cutoff", clock: "01:30:00", at: "2024-11-04T12:00:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Calendar{Timezone: "America/New_York", Cycle: "weekly", BoundaryTime: tt.clock, Weekday: intPtr(0)}
			_, err := c.WindowAt(mustTime(t, tt.at))
			if !errors.Is(err, ErrCalendarBoundary) {
				t.Fatalf("WindowAt() error = %v, want ErrCalendarBoundary", err)
			}
		})
	}
}

func TestCalendarInvalidConfigurationAndZeroTime(t *testing.T) {
	validWeekly := Calendar{Timezone: "UTC", Cycle: "weekly", BoundaryTime: "00:00:00", Weekday: intPtr(1)}
	tests := []struct {
		name string
		edit func(*Calendar)
	}{
		{name: "blank timezone", edit: func(c *Calendar) { c.Timezone = " " }},
		{name: "Local timezone", edit: func(c *Calendar) { c.Timezone = "Local" }},
		{name: "unknown timezone", edit: func(c *Calendar) { c.Timezone = "Mars/Olympus" }},
		{name: "unknown cycle", edit: func(c *Calendar) { c.Cycle = "daily" }},
		{name: "missing weekday", edit: func(c *Calendar) { c.Weekday = nil }},
		{name: "weekday below range", edit: func(c *Calendar) { c.Weekday = intPtr(-1) }},
		{name: "weekday above range", edit: func(c *Calendar) { c.Weekday = intPtr(7) }},
		{name: "inactive monthly fields", edit: func(c *Calendar) { c.MonthDay = intPtr(1) }},
		{name: "inactive short month", edit: func(c *Calendar) { c.ShortMonth = "skip" }},
		{name: "one digit hour", edit: func(c *Calendar) { c.BoundaryTime = "1:00:00" }},
		{name: "fractional seconds", edit: func(c *Calendar) { c.BoundaryTime = "01:00:00.5" }},
		{name: "invalid clock", edit: func(c *Calendar) { c.BoundaryTime = "24:00:00" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validWeekly
			tt.edit(&c)
			if err := c.Validate(); !errors.Is(err, ErrInvalid) {
				t.Fatalf("Validate() error = %v, want ErrInvalid", err)
			}
		})
	}

	monthly := Calendar{Timezone: "UTC", Cycle: "monthly", BoundaryTime: "00:00:00", MonthDay: intPtr(31), ShortMonth: "last_day"}
	for _, edit := range []struct {
		name string
		fn   func(*Calendar)
	}{
		{name: "missing month day", fn: func(c *Calendar) { c.MonthDay = nil }},
		{name: "month day below range", fn: func(c *Calendar) { c.MonthDay = intPtr(0) }},
		{name: "month day above range", fn: func(c *Calendar) { c.MonthDay = intPtr(32) }},
		{name: "missing short month policy", fn: func(c *Calendar) { c.ShortMonth = "" }},
		{name: "unknown short month policy", fn: func(c *Calendar) { c.ShortMonth = "clamp" }},
		{name: "inactive weekday", fn: func(c *Calendar) { c.Weekday = intPtr(1) }},
	} {
		t.Run(edit.name, func(t *testing.T) {
			c := monthly
			edit.fn(&c)
			if err := c.Validate(); !errors.Is(err, ErrInvalid) {
				t.Fatalf("Validate() error = %v, want ErrInvalid", err)
			}
		})
	}

	if _, err := validWeekly.WindowAt(time.Time{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("WindowAt(zero) error = %v, want ErrInvalid", err)
	}
}
