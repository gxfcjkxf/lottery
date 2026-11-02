package httpapi

import (
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/reportarchive"
	"testing"
)

func TestReportArchiveScheduledPolicyStrictFieldsAndOriginalScheduleReceipt(t *testing.T) {
	base := managedFixture(t)
	f := securityOperator(t, base)
	grantPlatformPermission(t, f, "platform_settings.view.platform", "platform_settings.write.platform")
	body := map[string]any{"version": 1, "daily_enabled": true, "monthly_enabled": true, "daily_execution_time": "07:30", "daily_period_offset": 2, "monthly_execution_day": 5, "monthly_execution_time": "06:00", "monthly_period_offset": 2}
	for _, c := range []struct {
		field string
		value any
	}{
		{"daily_execution_time", "7:30"}, {"daily_execution_time", "24:00"}, {"daily_period_offset", 0}, {"daily_period_offset", 366}, {"daily_period_offset", 1.5},
		{"monthly_execution_day", 0}, {"monthly_execution_day", 32}, {"monthly_execution_time", nil}, {"monthly_period_offset", 0}, {"monthly_period_offset", 121},
	} {
		invalid := map[string]any{}
		for k, v := range body {
			invalid[k] = v
		}
		invalid[c.field] = c.value
		raw, _ := json.Marshal(invalid)
		mustStatus(t, activationRequest(f, "archive-invalid-schedule-"+c.field, f.token, "", f.root, string(raw)), 400)
	}
	for _, field := range []string{"daily_execution_time", "daily_period_offset", "monthly_execution_day", "monthly_execution_time", "monthly_period_offset"} {
		invalid := map[string]any{}
		for k, v := range body {
			if k != field {
				invalid[k] = v
			}
		}
		raw, _ := json.Marshal(invalid)
		mustStatus(t, activationRequest(f, "archive-missing-schedule-"+field, f.token, "", f.root, string(raw)), 400)
	}
	raw, _ := json.Marshal(body)
	accepted := activationRequest(f, "archive-scheduled-exact-receipt", f.token, "", f.root, string(raw))
	mustStatus(t, accepted, 200)
	var p reportarchive.PlatformPolicy
	managedData(t, accepted, &p)
	if p.MonthlyExecutionDay != 5 || p.MonthlyExecutionTime != "06:00" || p.MonthlyPeriodOffset != 2 || p.DailyExecutionTime != "07:30" || p.DailyPeriodOffset != 2 || p.NextRuns == nil || p.NextRuns.Daily == nil || p.NextRuns.Monthly == nil {
		t.Fatalf("incomplete schedule receipt: %+v", p)
	}
	replay := activationRequest(f, "archive-scheduled-exact-receipt", f.token, "", f.root, string(raw))
	mustStatus(t, replay, 200)
	var original reportarchive.PlatformPolicy
	managedData(t, replay, &original)
	if *original.NextRuns.Monthly != *p.NextRuns.Monthly || original.Version != p.Version {
		t.Fatal("replay changed original schedule")
	}
	body["monthly_period_offset"] = 1
	changed, _ := json.Marshal(body)
	mustStatus(t, activationRequest(f, "archive-scheduled-exact-receipt", f.token, "", f.root, string(changed)), 409)
}
