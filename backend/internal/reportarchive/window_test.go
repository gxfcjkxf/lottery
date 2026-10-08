package reportarchive

import (
	"errors"
	"testing"
	"time"
)

func TestResolveWindowDailyDST(t *testing.T) {
	tests := []struct {
		key      string
		timezone string
		hours    float64
	}{
		{"2024-03-10", "America/New_York", 23},
		{"2024-11-03", "America/New_York", 25},
		{"2024-02-29", "UTC", 24},
		{"2024-10-08", "Asia/Singapore", 24},
	}
	for _, tt := range tests {
		t.Run(tt.key+"/"+tt.timezone, func(t *testing.T) {
			got, err := ResolveWindow(Daily, tt.key, tt.timezone)
			if err != nil {
				t.Fatalf("ResolveWindow() error = %v", err)
			}
			if got.From.In(mustLocation(t, tt.timezone)).Format("2006-01-02 15:04:05") != tt.key+" 00:00:00" {
				t.Errorf("From = %v, want local midnight for %s", got.From, tt.key)
			}
			if hours := got.To.Sub(got.From).Hours(); hours != tt.hours {
				t.Errorf("window duration = %v hours, want %v", hours, tt.hours)
			}
		})
	}
}

func TestResolveWindowMonthlyDSTAndLeapMonth(t *testing.T) {
	tests := []struct {
		key      string
		timezone string
		days     float64
	}{
		{"2024-03", "Europe/Berlin", 31*24 - 1},
		{"2024-02", "UTC", 29 * 24},
		{"2024-10", "Europe/Berlin", 31*24 + 1},
	}
	for _, tt := range tests {
		t.Run(tt.key+"/"+tt.timezone, func(t *testing.T) {
			got, err := ResolveWindow(Monthly, tt.key, tt.timezone)
			if err != nil {
				t.Fatalf("ResolveWindow() error = %v", err)
			}
			if hours := got.To.Sub(got.From).Hours(); hours != tt.days {
				t.Errorf("window duration = %v hours, want %v", hours, tt.days)
			}
		})
	}
}

func TestResolveWindowInvalidInput(t *testing.T) {
	tests := []struct {
		name, kind, key, timezone string
	}{
		{"unknown kind", "weekly", "2024-01-01", "UTC"},
		{"daily key with suffix", Daily, "2024-01-01x", "UTC"},
		{"daily whitespace", Daily, " 2024-01-01", "UTC"},
		{"daily null", Daily, "2024-01-01\x00", "UTC"},
		{"impossible date", Daily, "2024-02-30", "UTC"},
		{"year zero", Daily, "0000-01-01", "UTC"},
		{"year too high", Daily, "9999-01-01", "UTC"},
		{"monthly suffix", Monthly, "2024-01-", "UTC"},
		{"impossible month", Monthly, "2024-13", "UTC"},
		{"unknown timezone", Daily, "2024-01-01", "Mars/Olympus"},
		{"empty timezone", Daily, "2024-01-01", ""},
		{"local alias", Daily, "2024-01-01", "Local"},
		{"UTC lower boundary underflow", Daily, "0001-01-01", "Asia/Singapore"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ResolveWindow(tt.kind, tt.key, tt.timezone)
			if !errors.Is(err, ErrWindowInvalid) {
				t.Errorf("ResolveWindow() error = %v, want ErrWindowInvalid", err)
			}
		})
	}
}

func TestResolveWindowRejectsSkippedCivilDate(t *testing.T) {
	_, err := ResolveWindow(Daily, "2011-12-30", "Pacific/Apia")
	if !errors.Is(err, ErrWindowInvalid) {
		t.Fatalf("ResolveWindow() error = %v, want ErrWindowInvalid", err)
	}
}

func TestResolveWindowUsesFirstRealTimeAfterMidnightGap(t *testing.T) {
	// São Paulo advanced from 23:59 to 01:00 at the start of this date.
	w, err := ResolveWindow(Daily, "2018-11-04", "America/Sao_Paulo")
	if err != nil || w.From.Format(time.RFC3339) != "2018-11-04T03:00:00Z" || w.To.Sub(w.From) != 23*time.Hour {
		t.Fatalf("gap day boundary = %+v, err = %v", w, err)
	}
}

func TestResolveWindowUpperSupportedYear(t *testing.T) {
	got, err := ResolveWindow(Monthly, "9998-12", "UTC")
	if err != nil {
		t.Fatalf("ResolveWindow() error = %v", err)
	}
	if got.From.UTC().Format("2006-01-02") != "9998-12-01" || got.To.UTC().Format("2006-01-02") != "9999-01-01" {
		t.Errorf("window boundaries = [%s, %s), want [9998-12-01, 9999-01-01)", got.From, got.To)
	}
}

func TestResolveWindowIncludesBothRepeatedMidnights(t *testing.T) {
	// Havana repeated 00:00 when daylight saving time ended on 2020-11-01.
	w, err := ResolveWindow(Daily, "2020-11-01", "America/Havana")
	if err != nil || w.From.Format(time.RFC3339) != "2020-11-01T04:00:00Z" || w.To.Sub(w.From) != 25*time.Hour {
		t.Fatalf("fold day boundary = %+v, err = %v", w, err)
	}
}

func mustLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}
