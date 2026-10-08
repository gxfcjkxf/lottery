package reportarchive

import (
	"fmt"
	"time"
)

// ActivationPeriods returns the daily and monthly archive keys containing at.
// It is a pure calendar helper: callers own activation policy and persistence.
func ActivationPeriods(at time.Time, timezone string) (dailyKey, monthlyKey string, err error) {
	if at.IsZero() {
		return "", "", fmt.Errorf("%w: activation time must not be zero", ErrWindowInvalid)
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil || timezone == "" || timezone == "Local" {
		return "", "", fmt.Errorf("%w: timezone must be a valid named timezone", ErrWindowInvalid)
	}
	local := at.In(loc)
	year := local.Year()
	if year < 1 || year > 9998 {
		return "", "", fmt.Errorf("%w: period year must be between 0001 and 9998", ErrWindowInvalid)
	}
	dailyKey = local.Format("2006-01-02")
	monthlyKey = local.Format("2006-01")
	if _, err = ResolveWindow(Daily, dailyKey, timezone); err != nil {
		return "", "", err
	}
	if _, err = ResolveWindow(Monthly, monthlyKey, timezone); err != nil {
		return "", "", err
	}
	return dailyKey, monthlyKey, nil
}

// NextArchivePeriod returns the next canonical daily or monthly calendar key.
// It uses civil calendar arithmetic, so daylight-saving changes do not affect it.
func NextArchivePeriod(kind, key string) (string, error) {
	date, err := parseArchivePeriod(kind, key)
	if err != nil {
		return "", err
	}
	switch kind {
	case Daily:
		date = date.AddDate(0, 0, 1)
		if date.Year() > 9998 {
			return "", fmt.Errorf("%w: next period exceeds supported year", ErrWindowInvalid)
		}
		return date.Format("2006-01-02"), nil
	case Monthly:
		date = date.AddDate(0, 1, 0)
		if date.Year() > 9998 {
			return "", fmt.Errorf("%w: next period exceeds supported year", ErrWindowInvalid)
		}
		return date.Format("2006-01"), nil
	default:
		return "", fmt.Errorf("%w: kind must be daily or monthly", ErrWindowInvalid)
	}
}

// DueArchiveWindows resolves up to limit elapsed windows from startKey. The
// returned nextKey is the first key not handled, or empty when the supported
// calendar range is exhausted. Unresolvable skipped civil dates are consumed
// without manufacturing a window.
func DueArchiveWindows(kind, startKey, timezone string, asOf time.Time, limit int) (windows []Window, nextKey string, err error) {
	if limit < 1 || limit > 100 {
		return nil, "", fmt.Errorf("%w: limit must be between 1 and 100", ErrWindowInvalid)
	}
	if _, err := parseArchivePeriod(kind, startKey); err != nil {
		return nil, "", err
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil || timezone == "" || timezone == "Local" {
		return nil, "", fmt.Errorf("%w: timezone must be a valid named timezone", ErrWindowInvalid)
	}

	nextKey = startKey
	// A small allowance lets the cursor pass isolated dates skipped by timezone
	// changes while still bounding work for arbitrarily old activation cursors.
	maxSteps := limit + 7
	for steps := 0; steps < maxSteps; steps++ {
		window, resolveErr := ResolveWindow(kind, nextKey, timezone)
		if resolveErr != nil {
			date, _ := parseArchivePeriod(kind, nextKey)
			if !archivePeriodHasSkippedBoundary(kind, date, loc) {
				return windows, nextKey, resolveErr
			}
			advanced, advanceErr := NextArchivePeriod(kind, nextKey)
			if advanceErr != nil {
				return windows, "", advanceErr
			}
			nextKey = advanced
			continue
		}
		if window.To.After(asOf) {
			return windows, nextKey, nil
		}
		windows = append(windows, window)
		advanced, advanceErr := NextArchivePeriod(kind, nextKey)
		if advanceErr != nil {
			return windows, "", nil
		}
		nextKey = advanced
		if len(windows) == limit {
			return windows, nextKey, nil
		}
	}
	return windows, nextKey, fmt.Errorf("%w: too many unresolvable calendar periods", ErrWindowInvalid)
}

func archivePeriodHasSkippedBoundary(kind string, date time.Time, loc *time.Location) bool {
	if kind != Daily {
		return false
	}
	startYear, startMonth, startDay := date.Date()
	_, ok := localDayStart(startYear, startMonth, startDay, loc)
	return !ok
}

func parseArchivePeriod(kind, key string) (time.Time, error) {
	var layout string
	switch kind {
	case Daily:
		layout = "2006-01-02"
	case Monthly:
		layout = "2006-01"
	default:
		return time.Time{}, fmt.Errorf("%w: kind must be daily or monthly", ErrWindowInvalid)
	}
	date, err := time.Parse(layout, key)
	if err != nil || date.Format(layout) != key {
		return time.Time{}, fmt.Errorf("%w: period key must be canonical", ErrWindowInvalid)
	}
	if date.Year() < 1 || date.Year() > 9998 {
		return time.Time{}, fmt.Errorf("%w: period year must be between 0001 and 9998", ErrWindowInvalid)
	}
	return date, nil
}
