// Package schedule expands a validated schedule into a bounded set of draw slots.
package schedule

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalid = errors.New("invalid schedule")
	ErrLimit   = errors.New("schedule limit exceeded")
)

const (
	modeDaily       = "daily"
	modeInterval    = "interval"
	policySkip      = "skip"
	policyNormal    = "normal"
	maxRange        = 7 * 24 * time.Hour
	maxSlots        = 10000
	maxListEntries  = 10000
	maxBusyWindows  = 128
	maxDailyTimes   = 2880
	secondsPerDay   = 24 * 60 * 60
	minIntervalSecs = 30
	maxIntervalSecs = secondsPerDay
)

// Spec describes a recurring draw schedule in its local civil timezone.
type Spec struct {
	Timezone              string       `json:"timezone"`
	Mode                  string       `json:"mode"`
	DailyDrawTimes        []string     `json:"daily_draw_times"`
	IntervalSeconds       int          `json:"interval_seconds"`
	BusyWindows           []BusyWindow `json:"busy_windows"`
	BetOpenBeforeSeconds  int          `json:"bet_open_before_seconds"`
	BetCloseBeforeSeconds int          `json:"bet_close_before_seconds"`
	PauseDates            []string     `json:"pause_dates"`
	Weekdays              []int        `json:"weekdays"`
	HolidayDates          []string     `json:"holiday_dates"`
	HolidayPolicy         string       `json:"holiday_policy"`
}

// BusyWindow overrides the interval cadence for [Start, End) on each local day.
type BusyWindow struct {
	Start           string `json:"start"`
	End             string `json:"end"`
	IntervalSeconds int    `json:"interval_seconds"`
}

// Slot is one draw and its associated betting window.
type Slot struct {
	PeriodNo   string    `json:"period_no"`
	BetStartAt time.Time `json:"bet_start_at"`
	BetEndAt   time.Time `json:"bet_end_at"`
	DrawAt     time.Time `json:"draw_at"`
}

// Validate checks the full schedule definition. Wall-clock times that fall into
// a DST gap are valid definitions; those occurrences simply produce no slot.
func Validate(s Spec) error {
	if len(s.DailyDrawTimes) > maxDailyTimes || len(s.BusyWindows) > maxBusyWindows ||
		len(s.PauseDates) > maxListEntries || len(s.Weekdays) > 7 ||
		len(s.HolidayDates) > maxListEntries {
		return ErrLimit
	}
	loc, err := loadIANA(s.Timezone)
	if err != nil {
		return invalid("timezone")
	}
	_ = loc
	if s.Mode != modeDaily && s.Mode != modeInterval {
		return invalid("mode")
	}
	if s.BetOpenBeforeSeconds < 1 || s.BetOpenBeforeSeconds > secondsPerDay ||
		s.BetCloseBeforeSeconds < 0 || s.BetCloseBeforeSeconds >= s.BetOpenBeforeSeconds {
		return invalid("bet window")
	}
	if len(s.Weekdays) == 0 {
		return invalid("weekdays")
	}
	seenWeekdays := make(map[int]struct{}, len(s.Weekdays))
	for _, day := range s.Weekdays {
		if day < 0 || day > 6 {
			return invalid("weekday")
		}
		if _, ok := seenWeekdays[day]; ok {
			return invalid("duplicate weekday")
		}
		seenWeekdays[day] = struct{}{}
	}
	if s.HolidayPolicy != policySkip && s.HolidayPolicy != policyNormal {
		return invalid("holiday policy")
	}
	if err := validateDateList(s.PauseDates); err != nil {
		return err
	}
	if err := validateDateList(s.HolidayDates); err != nil {
		return err
	}
	if s.Mode == modeDaily {
		if s.IntervalSeconds != 0 || len(s.BusyWindows) != 0 || len(s.DailyDrawTimes) == 0 {
			return invalid("daily fields")
		}
		seen := make(map[string]struct{}, len(s.DailyDrawTimes))
		for _, value := range s.DailyDrawTimes {
			if _, _, _, ok := parseClock(value); !ok {
				return invalid("daily draw time")
			}
			if _, ok := seen[value]; ok {
				return invalid("duplicate daily draw time")
			}
			seen[value] = struct{}{}
		}
		return nil
	}
	if len(s.DailyDrawTimes) != 0 || !validInterval(s.IntervalSeconds) {
		return invalid("interval fields")
	}
	windows := make([]wallWindow, 0, len(s.BusyWindows))
	for _, w := range s.BusyWindows {
		start, _, _, startOK := parseClock(w.Start)
		end, _, _, endOK := parseClock(w.End)
		if !startOK || !endOK || start >= end || !validInterval(w.IntervalSeconds) {
			return invalid("busy window")
		}
		windows = append(windows, wallWindow{start: start, end: end})
	}
	sort.Slice(windows, func(i, j int) bool { return windows[i].start < windows[j].start })
	for i := 1; i < len(windows); i++ {
		if windows[i].start < windows[i-1].end {
			return invalid("overlapping busy windows")
		}
	}
	return nil
}

// Expand emits slots whose UTC draw times are in [from, to). The range may be
// at most seven days and the result may contain at most 10,000 slots.
//
// For interval schedules, each local day has a baseline grid anchored at
// local midnight. Each busy window replaces that grid on [start,end) with a
// grid anchored at its start; the end belongs to the baseline again. Wall-time
// candidates are resolved against the timezone: nonexistent DST times are
// skipped, and repeated times use the earlier UTC occurrence. Final output is
// ordered by UTC draw time and duplicate instants are emitted only once.
func Expand(s Spec, from, to time.Time) ([]Slot, error) {
	empty := []Slot{}
	if err := Validate(s); err != nil {
		return empty, err
	}
	if from.IsZero() || to.IsZero() || !to.After(from) ||
		from.UTC().Year() < 1 || from.UTC().Year() > 9999 ||
		to.UTC().Year() < 1 || to.UTC().Year() > 9999 {
		return empty, invalid("range")
	}
	duration := to.Sub(from)
	if duration > maxRange {
		return empty, ErrLimit
	}
	if from.Equal(to) {
		return empty, nil
	}
	loc, _ := loadIANA(s.Timezone)
	paused := stringSet(s.PauseDates)
	holiday := stringSet(s.HolidayDates)
	weekdays := make(map[time.Weekday]struct{}, len(s.Weekdays))
	for _, day := range s.Weekdays {
		weekdays[time.Weekday(day)] = struct{}{}
	}

	// Timezone offsets are strictly less than two days from UTC for IANA data;
	// padding the UTC dates by two days covers every local civil date touched.
	first := from.UTC().Add(-48 * time.Hour).Truncate(24 * time.Hour)
	last := to.UTC().Add(48 * time.Hour).Truncate(24 * time.Hour)
	slots := make([]Slot, 0)
	walls := dailyWalls(s)
	for day := first; !day.After(last); day = day.Add(24 * time.Hour) {
		y, m, d := day.Date()
		date := fmt.Sprintf("%04d-%02d-%02d", y, int(m), d)
		if _, ok := paused[date]; ok {
			continue
		}
		if _, ok := weekdays[day.Weekday()]; !ok {
			continue
		}
		if s.HolidayPolicy == policySkip {
			if _, ok := holiday[date]; ok {
				continue
			}
		}
		wallMidnight := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
		offsets := wallOffsets(wallMidnight, loc)
		for _, seconds := range walls {
			wall := wallMidnight.Add(time.Duration(seconds) * time.Second)
			instant, ok := resolveWall(wall, loc, offsets)
			if !ok || instant.Before(from) || !instant.Before(to) {
				continue
			}
			draw := instant.UTC()
			slots = append(slots, Slot{
				PeriodNo:   draw.Format("20060102T150405Z"),
				BetStartAt: draw.Add(-time.Duration(s.BetOpenBeforeSeconds) * time.Second),
				BetEndAt:   draw.Add(-time.Duration(s.BetCloseBeforeSeconds) * time.Second),
				DrawAt:     draw,
			})
			if len(slots) > maxSlots {
				return empty, ErrLimit
			}
		}
	}
	sort.Slice(slots, func(i, j int) bool { return slots[i].DrawAt.Before(slots[j].DrawAt) })
	return slots, nil
}

type wallWindow struct{ start, end int }

func dailyWalls(s Spec) []int {
	if s.Mode == modeDaily {
		walls := make([]int, 0, len(s.DailyDrawTimes))
		for _, value := range s.DailyDrawTimes {
			sec, _, _, _ := parseClock(value)
			walls = append(walls, sec)
		}
		sort.Ints(walls)
		return walls
	}
	var windows []wallWindow
	for _, w := range s.BusyWindows {
		start, _, _, _ := parseClock(w.Start)
		end, _, _, _ := parseClock(w.End)
		windows = append(windows, wallWindow{start: start, end: end})
	}
	walls := make([]int, 0, secondsPerDay/s.IntervalSeconds)
	for sec := 0; sec < secondsPerDay; sec += s.IntervalSeconds {
		inside := false
		for _, w := range windows {
			if sec >= w.start && sec < w.end {
				inside = true
				break
			}
		}
		if !inside {
			walls = append(walls, sec)
		}
	}
	for _, w := range s.BusyWindows {
		start, _, _, _ := parseClock(w.Start)
		end, _, _, _ := parseClock(w.End)
		for sec := start; sec < end; sec += w.IntervalSeconds {
			walls = append(walls, sec)
		}
	}
	sort.Ints(walls)
	if len(walls) > 1 {
		unique := walls[:1]
		for _, sec := range walls[1:] {
			if sec != unique[len(unique)-1] {
				unique = append(unique, sec)
			}
		}
		walls = unique
	}
	return walls
}

func wallOffsets(wall time.Time, loc *time.Location) []int {
	wall = wall.UTC()
	// Sample both sides of this civil date for all offsets in force around it.
	// A 15-minute step also sees short-lived historical offset regimes.
	offsets := make(map[int]struct{}, 4)
	for delta := -48 * time.Hour; delta <= 48*time.Hour; delta += 15 * time.Minute {
		_, offset := wall.Add(delta).In(loc).Zone()
		offsets[offset] = struct{}{}
	}
	result := make([]int, 0, len(offsets))
	for offset := range offsets {
		result = append(result, offset)
	}
	sort.Ints(result)
	return result
}

func resolveWall(wall time.Time, loc *time.Location, offsets []int) (time.Time, bool) {
	wall = wall.UTC()
	var earliest time.Time
	found := false
	for _, offset := range offsets {
		candidate := wall.Add(-time.Duration(offset) * time.Second)
		local := candidate.In(loc)
		y, m, d := local.Date()
		h, min, sec := local.Clock()
		wy, wm, wd := wall.Date()
		wh, wmin, ws := wall.Clock()
		if y == wy && m == wm && d == wd && h == wh && min == wmin && sec == ws {
			if !found || candidate.Before(earliest) {
				earliest = candidate
				found = true
			}
		}
	}
	return earliest, found
}

func loadIANA(name string) (*time.Location, error) {
	if name == "" || name == "Local" || strings.TrimSpace(name) != name {
		return nil, ErrInvalid
	}
	return time.LoadLocation(name)
}

func parseClock(value string) (seconds, hour, minute int, ok bool) {
	if len(value) != 8 || value[2] != ':' || value[5] != ':' {
		return 0, 0, 0, false
	}
	h, errH := strconv.Atoi(value[:2])
	m, errM := strconv.Atoi(value[3:5])
	s, errS := strconv.Atoi(value[6:8])
	if errH != nil || errM != nil || errS != nil || h > 23 || m > 59 || s > 59 {
		return 0, 0, 0, false
	}
	return h*3600 + m*60 + s, h, m, true
}

func validateDateList(values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		parsed, err := time.Parse("2006-01-02", value)
		if err != nil || parsed.Format("2006-01-02") != value {
			return invalid("date")
		}
		if _, ok := seen[value]; ok {
			return invalid("duplicate date")
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validInterval(seconds int) bool {
	return seconds >= minIntervalSecs && seconds <= maxIntervalSecs
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func invalid(field string) error { return fmt.Errorf("%w: %s", ErrInvalid, field) }
