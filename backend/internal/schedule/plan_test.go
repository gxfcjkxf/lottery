package schedule

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func baseDaily() Spec {
	return Spec{
		Timezone:              "UTC",
		Mode:                  modeDaily,
		DailyDrawTimes:        []string{"12:00:00"},
		BetOpenBeforeSeconds:  300,
		BetCloseBeforeSeconds: 30,
		Weekdays:              []int{0, 1, 2, 3, 4, 5, 6},
		HolidayPolicy:         policySkip,
	}
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestExpandDailyAndUTCBounds(t *testing.T) {
	s := baseDaily()
	from := mustTime(t, "2024-02-29T11:55:00Z")
	to := mustTime(t, "2024-03-02T12:00:00Z")
	slots, err := Expand(s, from, to)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"20240229T120000Z", "20240301T120000Z"}
	got := make([]string, len(slots))
	for i, slot := range slots {
		got[i] = slot.PeriodNo
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("periods = %v, want %v", got, want)
	}
	if !slots[0].BetStartAt.Equal(mustTime(t, "2024-02-29T11:55:00Z")) ||
		!slots[0].BetEndAt.Equal(mustTime(t, "2024-02-29T11:59:30Z")) {
		t.Fatalf("unexpected bet window: %+v", slots[0])
	}
	if _, err := Expand(s, to, to); !errors.Is(err, ErrInvalid) {
		t.Fatalf("equal range error = %v, want ErrInvalid", err)
	}
}

func TestExpandNoMatchingDatesReturnsNonNilEmpty(t *testing.T) {
	s := baseDaily()
	s.Weekdays = []int{2} // 2024-01-01 is Monday.
	from := mustTime(t, "2024-01-01T00:00:00Z")
	to := mustTime(t, "2024-01-02T00:00:00Z")
	slots, err := Expand(s, from, to)
	if err != nil || slots == nil || len(slots) != 0 {
		t.Fatalf("no-match expansion = %#v, %v; want non-nil empty slice", slots, err)
	}
}

func TestExpandIntervalBusyWindowCadenceAndBoundaries(t *testing.T) {
	s := baseDaily()
	s.Mode = modeInterval
	s.DailyDrawTimes = nil
	s.IntervalSeconds = 120
	s.BusyWindows = []BusyWindow{{Start: "00:03:00", End: "00:07:00", IntervalSeconds: 60}}
	from := mustTime(t, "2024-01-01T00:00:00Z")
	to := mustTime(t, "2024-01-01T00:09:00Z")
	slots, err := Expand(s, from, to)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"20240101T000000Z", "20240101T000200Z", "20240101T000300Z",
		"20240101T000400Z", "20240101T000500Z", "20240101T000600Z",
		"20240101T000800Z",
	}
	got := make([]string, len(slots))
	for i, slot := range slots {
		got[i] = slot.PeriodNo
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("periods = %v, want %v", got, want)
	}
}

func TestExpandPauseWeekdayAndHoliday(t *testing.T) {
	s := baseDaily()
	s.DailyDrawTimes = []string{"09:00:00"}
	s.PauseDates = []string{"2024-01-02"}
	s.HolidayDates = []string{"2024-01-03"}
	s.Weekdays = []int{1, 2, 3, 4, 5}
	from := mustTime(t, "2024-01-01T00:00:00Z")
	to := mustTime(t, "2024-01-06T00:00:00Z")
	slots, err := Expand(s, from, to)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"20240101T090000Z", "20240104T090000Z", "20240105T090000Z"}
	got := make([]string, len(slots))
	for i, slot := range slots {
		got[i] = slot.PeriodNo
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("periods = %v, want %v", got, want)
	}
	s.HolidayPolicy = policyNormal
	slots, err = Expand(s, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 4 || slots[1].PeriodNo != "20240103T090000Z" {
		t.Fatalf("normal holiday policy slots = %+v", slots)
	}
}

func TestExpandDSTWallTimePolicy(t *testing.T) {
	t.Run("spring gap skipped", func(t *testing.T) {
		s := baseDaily()
		s.Timezone = "America/New_York"
		s.DailyDrawTimes = []string{"02:30:00"}
		from := mustTime(t, "2024-03-10T00:00:00Z")
		to := mustTime(t, "2024-03-11T00:00:00Z")
		slots, err := Expand(s, from, to)
		if err != nil || len(slots) != 0 {
			t.Fatalf("spring gap slots = %+v, err %v", slots, err)
		}
	})
	t.Run("fall fold chooses earlier occurrence", func(t *testing.T) {
		s := baseDaily()
		s.Timezone = "America/New_York"
		s.DailyDrawTimes = []string{"01:30:00"}
		from := mustTime(t, "2024-11-03T00:00:00Z")
		to := mustTime(t, "2024-11-04T00:00:00Z")
		slots, err := Expand(s, from, to)
		if err != nil || len(slots) != 1 {
			t.Fatalf("fall fold slots = %+v, err %v", slots, err)
		}
		if want := mustTime(t, "2024-11-03T05:30:00Z"); !slots[0].DrawAt.Equal(want) {
			t.Fatalf("fold draw = %s, want earlier occurrence %s", slots[0].DrawAt, want)
		}
	})
}

func TestValidateLeapDatesAndRejectsInvalidParameters(t *testing.T) {
	s := baseDaily()
	s.PauseDates = []string{"2024-02-29"}
	s.HolidayDates = []string{"2024-02-29"}
	if err := Validate(s); err != nil {
		t.Fatalf("valid leap date rejected: %v", err)
	}
	tests := map[string]func(*Spec){
		"unknown timezone":       func(s *Spec) { s.Timezone = "Mars/Olympus" },
		"local timezone":         func(s *Spec) { s.Timezone = "Local" },
		"bad mode":               func(s *Spec) { s.Mode = "weekly" },
		"bad wall clock":         func(s *Spec) { s.DailyDrawTimes = []string{"24:00:00"} },
		"duplicate draw time":    func(s *Spec) { s.DailyDrawTimes = []string{"12:00:00", "12:00:00"} },
		"bad pause date":         func(s *Spec) { s.PauseDates = []string{"2023-02-29"} },
		"duplicate holiday":      func(s *Spec) { s.HolidayDates = []string{"2024-01-01", "2024-01-01"} },
		"duplicate weekday":      func(s *Spec) { s.Weekdays = []int{1, 1} },
		"bad weekday":            func(s *Spec) { s.Weekdays = []int{7} },
		"bad holiday policy":     func(s *Spec) { s.HolidayPolicy = "move" },
		"zero lead":              func(s *Spec) { s.BetOpenBeforeSeconds = 0 },
		"close not before open":  func(s *Spec) { s.BetCloseBeforeSeconds = 300 },
		"daily with interval":    func(s *Spec) { s.IntervalSeconds = 60 },
		"daily with busy window": func(s *Spec) { s.BusyWindows = []BusyWindow{{Start: "01:00:00", End: "02:00:00", IntervalSeconds: 60}} },
		"interval too short":     func(s *Spec) { s.Mode = modeInterval; s.DailyDrawTimes = nil; s.IntervalSeconds = 29 },
		"interval with daily":    func(s *Spec) { s.Mode = modeInterval; s.IntervalSeconds = 60 },
		"window crosses midnight": func(s *Spec) {
			s.Mode = modeInterval
			s.DailyDrawTimes = nil
			s.IntervalSeconds = 60
			s.BusyWindows = []BusyWindow{{Start: "23:00:00", End: "01:00:00", IntervalSeconds: 30}}
		},
		"overlapping windows": func(s *Spec) {
			s.Mode = modeInterval
			s.DailyDrawTimes = nil
			s.IntervalSeconds = 60
			s.BusyWindows = []BusyWindow{{Start: "01:00:00", End: "03:00:00", IntervalSeconds: 30}, {Start: "02:59:59", End: "04:00:00", IntervalSeconds: 30}}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			invalid := baseDaily()
			mutate(&invalid)
			if err := Validate(invalid); !errors.Is(err, ErrInvalid) {
				t.Fatalf("Validate() error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestExpandLimits(t *testing.T) {
	s := baseDaily()
	from := mustTime(t, "2024-01-01T00:00:00Z")
	if _, err := Expand(s, from, from.Add(7*24*time.Hour+time.Second)); !errors.Is(err, ErrLimit) {
		t.Fatalf("long range error = %v, want ErrLimit", err)
	}
	if _, err := Expand(s, from.Add(time.Second), from); !errors.Is(err, ErrInvalid) {
		t.Fatalf("reversed range error = %v, want ErrInvalid", err)
	}
	for _, tc := range []struct {
		name     string
		from, to time.Time
	}{
		{name: "zero from", from: time.Time{}, to: from.Add(time.Hour)},
		{name: "zero to", from: from, to: time.Time{}},
		{name: "year below range", from: time.Date(0, 12, 31, 0, 0, 0, 0, time.UTC), to: time.Date(1, 1, 1, 1, 0, 0, 0, time.UTC)},
		{name: "year above range", from: time.Date(9999, 12, 31, 22, 0, 0, 0, time.UTC), to: time.Date(10000, 1, 1, 1, 0, 0, 0, time.UTC)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Expand(baseDaily(), tc.from, tc.to); !errors.Is(err, ErrInvalid) {
				t.Fatalf("range error = %v, want ErrInvalid", err)
			}
		})
	}
	s.Mode = modeInterval
	s.DailyDrawTimes = nil
	s.IntervalSeconds = 30
	if _, err := Expand(s, from, from.Add(7*24*time.Hour)); !errors.Is(err, ErrLimit) {
		t.Fatalf("dense interval error = %v, want ErrLimit", err)
	}
	tooMany := baseDaily()
	tooMany.DailyDrawTimes = make([]string, maxListEntries+1)
	if err := Validate(tooMany); !errors.Is(err, ErrLimit) {
		t.Fatalf("oversized configuration error = %v, want ErrLimit", err)
	}
	tooManyWindows := baseDaily()
	tooManyWindows.Mode = modeInterval
	tooManyWindows.DailyDrawTimes = nil
	tooManyWindows.IntervalSeconds = 60
	tooManyWindows.BusyWindows = make([]BusyWindow, maxBusyWindows+1)
	if err := Validate(tooManyWindows); !errors.Is(err, ErrLimit) {
		t.Fatalf("oversized busy windows error = %v, want ErrLimit", err)
	}
}
