package commission

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrCalendarBoundary indicates that a configured local cycle boundary does
// not identify exactly one instant in its timezone, usually because of a DST
// transition.
var ErrCalendarBoundary = errors.New("invalid commission calendar boundary")

// Calendar describes the local-time boundaries of commission cycles.
// Weekday uses time.Weekday numbering (Sunday=0). Monthly short months must
// explicitly choose whether to clamp the day to month end or skip the month.
type Calendar struct {
	Timezone     string
	Cycle        string
	BoundaryTime string
	Weekday      *int
	MonthDay     *int
	ShortMonth   string
}

// Window is a half-open UTC interval [From, To).
type Window struct {
	From time.Time
	To   time.Time
}

// Validate checks the calendar configuration, including fields that must be
// inactive for the selected cycle.
func (c Calendar) Validate() error {
	if strings.TrimSpace(c.Timezone) == "" || c.Timezone == "Local" {
		return ErrInvalid
	}
	if _, err := time.LoadLocation(c.Timezone); err != nil {
		return ErrInvalid
	}
	if len(c.BoundaryTime) != 8 || c.BoundaryTime[2] != ':' || c.BoundaryTime[5] != ':' {
		return ErrInvalid
	}
	for i, r := range c.BoundaryTime {
		if i == 2 || i == 5 {
			continue
		}
		if r < '0' || r > '9' {
			return ErrInvalid
		}
	}
	if _, err := time.Parse("15:04:05", c.BoundaryTime); err != nil {
		return ErrInvalid
	}

	switch c.Cycle {
	case "weekly":
		if c.Weekday == nil || *c.Weekday < 0 || *c.Weekday > 6 || c.MonthDay != nil || c.ShortMonth != "" {
			return ErrInvalid
		}
	case "monthly":
		if c.MonthDay == nil || *c.MonthDay < 1 || *c.MonthDay > 31 || c.Weekday != nil {
			return ErrInvalid
		}
		if c.ShortMonth != "last_day" && c.ShortMonth != "skip" {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

// WindowAt returns the commission cycle containing t. The result is a UTC,
// half-open interval; an instant at a boundary belongs to the new cycle.
func (c Calendar) WindowAt(t time.Time) (Window, error) {
	if err := c.Validate(); err != nil || t.IsZero() {
		return Window{}, ErrInvalid
	}
	loc, _ := time.LoadLocation(c.Timezone) // Validate already checked it.
	local := t.In(loc)
	var from, to time.Time
	var err error
	if c.Cycle == "weekly" {
		from, to, err = c.weeklyBounds(local, loc)
	} else {
		from, to, err = c.monthlyBounds(local, loc)
	}
	if err != nil {
		return Window{}, err
	}
	return Window{From: from.UTC(), To: to.UTC()}, nil
}

func (c Calendar) weeklyBounds(local time.Time, loc *time.Location) (time.Time, time.Time, error) {
	day := int(local.Weekday() - time.Weekday(*c.Weekday))
	if day < 0 {
		day += 7
	}
	y, m, d := local.Date()
	anchorDate := time.Date(y, m, d-day, 12, 0, 0, 0, time.UTC)
	from, err := c.boundaryOn(anchorDate, loc)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if localToInstant(local).Before(from) {
		anchorDate = anchorDate.AddDate(0, 0, -7)
		from, err = c.boundaryOn(anchorDate, loc)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	}
	to, err := c.boundaryOn(anchorDate.AddDate(0, 0, 7), loc)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return from, to, nil
}

// localToInstant preserves the instant represented by local, regardless of
// its location. It is only used to compare against resolved boundary instants.
func localToInstant(local time.Time) time.Time { return local.UTC() }

func (c Calendar) monthlyBounds(local time.Time, loc *time.Location) (time.Time, time.Time, error) {
	y, m, _ := local.Date()
	month := time.Date(y, m, 1, 12, 0, 0, 0, time.UTC)
	var from, to time.Time
	var found bool
	for delta := 0; delta >= -2; delta-- {
		candidate, ok, err := c.monthBoundary(month.AddDate(0, delta, 0), loc)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		if ok && !candidate.After(local.UTC()) {
			from, found = candidate, true
			break
		}
	}
	if !found {
		return time.Time{}, time.Time{}, ErrCalendarBoundary
	}
	for delta := 0; delta <= 2; delta++ {
		candidate, ok, err := c.monthBoundary(month.AddDate(0, delta, 0), loc)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		if ok && candidate.After(from) {
			to = candidate
			break
		}
	}
	if to.IsZero() {
		return time.Time{}, time.Time{}, ErrCalendarBoundary
	}
	return from, to, nil
}

func (c Calendar) monthBoundary(month time.Time, loc *time.Location) (time.Time, bool, error) {
	y, m, _ := month.Date()
	day := *c.MonthDay
	daysInMonth := time.Date(y, m+1, 0, 12, 0, 0, 0, time.UTC).Day()
	if day > daysInMonth {
		if c.ShortMonth == "skip" {
			return time.Time{}, false, nil
		}
		day = daysInMonth
	}
	date := time.Date(y, m, day, 12, 0, 0, 0, time.UTC)
	boundary, err := c.boundaryOn(date, loc)
	return boundary, true, err
}

func (c Calendar) boundaryOn(date time.Time, loc *time.Location) (time.Time, error) {
	clock, _ := time.Parse("15:04:05", c.BoundaryTime)
	y, m, d := date.Date()
	wall := time.Date(y, m, d, clock.Hour(), clock.Minute(), clock.Second(), 0, time.UTC)
	// Gather every offset observed near the requested wall time, including
	// offsets on either side of transitions, and retain only exact round trips.
	offsets := make(map[int]struct{})
	for delta := -36 * time.Hour; delta <= 36*time.Hour; delta += 15 * time.Minute {
		_, offset := wall.Add(delta).In(loc).Zone()
		offsets[offset] = struct{}{}
	}
	candidates := make(map[int64]time.Time)
	for offset := range offsets {
		candidate := wall.Add(-time.Duration(offset) * time.Second)
		actual := candidate.In(loc)
		ay, am, ad := actual.Date()
		ah, amin, asec := actual.Clock()
		if ay == y && am == m && ad == d && ah == clock.Hour() && amin == clock.Minute() && asec == clock.Second() {
			candidates[candidate.Unix()] = candidate
		}
	}
	if len(candidates) != 1 {
		return time.Time{}, fmt.Errorf("%w: %04d-%02d-%02d %s in %s", ErrCalendarBoundary, y, m, d, c.BoundaryTime, loc)
	}
	for _, candidate := range candidates {
		return candidate, nil
	}
	return time.Time{}, ErrCalendarBoundary
}
