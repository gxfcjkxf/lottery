package reportarchive

import "time"

// Automatic policy is an operational gate, not a financial closure. Start
// periods are frozen with each policy revision; disabled is the initial state.
type AutomaticSchedule struct {
	DailyExecutionTime   string `json:"daily_execution_time"`
	DailyPeriodOffset    int    `json:"daily_period_offset"`
	MonthlyExecutionDay  int    `json:"monthly_execution_day"`
	MonthlyExecutionTime string `json:"monthly_execution_time"`
	MonthlyPeriodOffset  int    `json:"monthly_period_offset"`
}

// A scheduled run captures a complete target period at the configured brand-local time.
type ScheduledRun struct {
	ExecuteAt time.Time `json:"execute_at"`
	PeriodKey string    `json:"period_key"`
}
type AutomaticNextRuns struct {
	Daily   *ScheduledRun `json:"daily"`
	Monthly *ScheduledRun `json:"monthly"`
}

type AutomaticPolicy struct {
	AutomaticSchedule
	NextRuns           *AutomaticNextRuns `json:"next_runs,omitempty"`
	BrandID            string             `json:"brand_id"`
	Version            int64              `json:"version"`
	DailyEnabled       bool               `json:"daily_enabled"`
	MonthlyEnabled     bool               `json:"monthly_enabled"`
	DailyStartPeriod   *string            `json:"daily_start_period"`
	MonthlyStartPeriod *string            `json:"monthly_start_period"`
	Timezone           string             `json:"timezone"`
	AuditLogID         *string            `json:"audit_log_id"`
	UpdatedAt          time.Time          `json:"updated_at"`
}
type AutomaticPolicyInput struct {
	AutomaticSchedule
	Version            int64   `json:"version"`
	DailyEnabled       bool    `json:"daily_enabled"`
	MonthlyEnabled     bool    `json:"monthly_enabled"`
	DailyStartPeriod   *string `json:"daily_start_period"`
	MonthlyStartPeriod *string `json:"monthly_start_period"`
	Reason             string  `json:"reason"`
}
type AutomaticTask struct {
	ID                 string    `json:"id"`
	BrandID            string    `json:"brand_id"`
	PolicyVersion      int64     `json:"policy_version"`
	Window             Window    `json:"window"`
	State              string    `json:"state"`
	Version            int64     `json:"version"`
	AttemptCount       int64     `json:"attempt_count"`
	ArchiveID          *string   `json:"archive_id"`
	LastErrorCode      *string   `json:"last_error_code"`
	CreationAuditLogID string    `json:"creation_audit_log_id"`
	LastAuditLogID     string    `json:"last_audit_log_id"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}
type AutomaticTaskPage struct {
	BrandID    string          `json:"brand_id"`
	Items      []AutomaticTask `json:"items"`
	TotalCount string          `json:"total_count"`
	Limit      int             `json:"limit"`
	Offset     int             `json:"offset"`
}
