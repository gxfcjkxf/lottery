package reportarchive

import "regexp"

var executionTimePattern = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

func (s AutomaticSchedule) Valid() bool {
	return executionTimePattern.MatchString(s.DailyExecutionTime) && executionTimePattern.MatchString(s.MonthlyExecutionTime) &&
		s.DailyPeriodOffset >= 1 && s.DailyPeriodOffset <= 365 && s.MonthlyPeriodOffset >= 1 && s.MonthlyPeriodOffset <= 120 &&
		s.MonthlyExecutionDay >= 1 && s.MonthlyExecutionDay <= 31
}
