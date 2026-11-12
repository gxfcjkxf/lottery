// Package dataretention implements the monthly business-data retention policy.
package dataretention

import (
	"errors"
	"regexp"
	"time"
)

var ErrSchedule = errors.New("invalid data retention schedule")
var ErrHistoricalRange = errors.New("historical reports require complete archive-local days")

var clockPattern = regexp.MustCompile(`^(?:[01][0-9]|2[0-3]):[0-5][0-9]$`)

// Schedule uses the platform timezone. Day is limited to 1..28 so every
// calendar month has exactly one execution date.
type Schedule struct {
	ExecutionDay  int    `json:"execution_day"`
	ExecutionTime string `json:"execution_time"`
}

type Run struct {
	ExecuteAt time.Time `json:"execute_at"`
	CutoffAt  time.Time `json:"cutoff_at"`
}

func (s Schedule) Validate() error {
	if s.ExecutionDay < 1 || s.ExecutionDay > 28 || !clockPattern.MatchString(s.ExecutionTime) {
		return ErrSchedule
	}
	return nil
}

// ValidateHistoricalRange applies to any query touching summarized history.
// timezone is the timezone frozen with that history, not a browser or current
// brand timezone. The caller must reject a range spanning different historical
// timezones. Recent-only queries retain their existing timestamp contract.
func ValidateHistoricalRange(from, to time.Time, timezone string) error {
	if from.IsZero() || to.IsZero() || !to.After(from) || timezone == "" || timezone == "Local" {
		return ErrHistoricalRange
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return ErrHistoricalRange
	}
	for _, bound := range []time.Time{from.In(loc), to.In(loc)} {
		if bound.Hour() != 0 || bound.Minute() != 0 || bound.Second() != 0 || bound.Nanosecond() != 0 {
			return ErrHistoricalRange
		}
	}
	return nil
}

// NextRun returns the first execution strictly after after. The cutoff belongs
// to the execution month, never the worker's eventual catch-up time. A missing
// local wall clock is an explicit error; it must not silently move execution.
func (s Schedule) NextRun(after time.Time, timezone string) (Run, error) {
	if s.Validate() != nil || after.IsZero() || timezone == "" || timezone == "Local" {
		return Run{}, ErrSchedule
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return Run{}, ErrSchedule
	}
	clock, _ := time.Parse("15:04", s.ExecutionTime)
	local := after.In(loc)
	month := time.Date(local.Year(), local.Month(), 1, 12, 0, 0, 0, loc)
	for i := 0; i < 2; i++ {
		if month.Year() == local.Year() && month.Month() == local.Month() && (local.Day() > s.ExecutionDay || local.Day() == s.ExecutionDay && local.Hour()*60+local.Minute() >= clock.Hour()*60+clock.Minute()) {
			month = month.AddDate(0, 1, 0)
			continue
		}
		execution := time.Date(month.Year(), month.Month(), s.ExecutionDay, clock.Hour(), clock.Minute(), 0, 0, loc)
		if execution.Day() != s.ExecutionDay || execution.Hour() != clock.Hour() || execution.Minute() != clock.Minute() {
			return Run{}, ErrSchedule
		}
		if execution.After(after) {
			previous := month.AddDate(0, -1, 0)
			cutoff := time.Date(previous.Year(), previous.Month(), 1, 0, 0, 0, 0, loc)
			if cutoff.Day() != 1 || cutoff.Hour() != 0 || cutoff.Minute() != 0 {
				return Run{}, ErrSchedule
			}
			return Run{ExecuteAt: execution.UTC(), CutoffAt: cutoff.UTC()}, nil
		}
		month = month.AddDate(0, 1, 0)
	}
	return Run{}, ErrSchedule
}
