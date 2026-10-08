// Package reportarchive resolves report periods into brand-local time windows.
package reportarchive

import (
	"errors"
	"fmt"
	"time"
)

const (
	Daily   = "daily"
	Monthly = "monthly"
)

// ErrWindowInvalid marks an invalid period kind, key, timezone, or local
// calendar boundary.
var ErrWindowInvalid = errors.New("invalid report window")

// Window is the half-open time range for one report period.
type Window struct {
	Kind      string    `json:"kind"`
	PeriodKey string    `json:"period_key"`
	Timezone  string    `json:"timezone"`
	From      time.Time `json:"from"`
	To        time.Time `json:"to"`
}

// ResolveWindow resolves a canonical daily or monthly period key in timezone.
// Daily keys have the form YYYY-MM-DD; monthly keys have the form YYYY-MM.
func ResolveWindow(kind, key, timezone string) (Window, error) {
	invalid := func(reason string) (Window, error) {
		return Window{}, fmt.Errorf("%w: %s", ErrWindowInvalid, reason)
	}

	loc, err := time.LoadLocation(timezone)
	if err != nil || timezone == "" || timezone == "Local" {
		return invalid("timezone must be a valid named timezone")
	}

	var year int
	var month time.Month
	var day int
	switch kind {
	case Daily:
		if len(key) != len("2006-01-02") {
			return invalid("daily period key must use YYYY-MM-DD")
		}
		date, parseErr := time.Parse("2006-01-02", key)
		if parseErr != nil || date.Format("2006-01-02") != key {
			return invalid("daily period key must use a real canonical date")
		}
		year, month, day = date.Date()
	case Monthly:
		if len(key) != len("2006-01") {
			return invalid("monthly period key must use YYYY-MM")
		}
		date, parseErr := time.Parse("2006-01", key)
		if parseErr != nil || date.Format("2006-01") != key {
			return invalid("monthly period key must use a real canonical month")
		}
		year, month, _ = date.Date()
		day = 1
	default:
		return invalid("kind must be daily or monthly")
	}
	if year < 1 || year > 9998 {
		return invalid("period year must be between 0001 and 9998")
	}

	from, ok := localDayStart(year, month, day, loc)
	if !ok {
		return invalid("period start does not have a real local civil date")
	}
	endYear, endMonth, endDay := year, month, day
	if kind == Daily {
		civilNext := time.Date(year, month, day+1, 0, 0, 0, 0, time.UTC)
		endYear, endMonth, endDay = civilNext.Date()
	} else {
		civilNext := time.Date(year, month+1, 1, 0, 0, 0, 0, time.UTC)
		endYear, endMonth, endDay = civilNext.Date()
	}
	to, ok := localDayStart(endYear, endMonth, endDay, loc)
	if !ok {
		return invalid("period end does not have a real local civil date")
	}
	if !to.After(from) {
		return invalid("period boundaries do not form a positive interval")
	}
	if from.UTC().Year() < 1 || to.UTC().Year() > 9999 {
		return invalid("UTC boundaries must fit the supported timestamp range")
	}

	return Window{
		Kind:      kind,
		PeriodKey: key,
		Timezone:  timezone,
		From:      from,
		To:        to,
	}, nil
}

// localDayStart selects the first real instant of a civil date. For repeated
// midnight it includes the earlier occurrence; for a midnight gap it uses the
// first valid time after the gap. A wholly skipped civil date has no boundary.
func localDayStart(year int, month time.Month, day int, loc *time.Location) (time.Time, bool) {
	civil := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	candidates := make(map[int64]time.Time)
	for delta := -48 * time.Hour; delta <= 48*time.Hour; delta += 30 * time.Minute {
		probe := civil.Add(delta).In(loc)
		_, offset := probe.Zone()
		candidate := civil.Add(-time.Duration(offset) * time.Second)
		local := candidate.In(loc)
		y, m, d := local.Date()
		if y == year && m == month && d == day {
			candidates[candidate.Unix()] = candidate
		}
	}
	if len(candidates) == 0 {
		return time.Time{}, false
	}
	var first time.Time
	for _, candidate := range candidates {
		if first.IsZero() || candidate.Before(first) {
			first = candidate
		}
	}
	return first, true
}
