package reportarchive

import (
	"context"
	"testing"
	"time"
)

func TestArchiveScheduleCalendarAndNextTarget(t *testing.T) {
	s, _ := archiveFixture(t)
	ctx := context.Background()
	cases := []struct {
		kind, key, zone, clock string
		offset, day            int
		want                   string
	}{
		{Monthly, "2026-09", "Asia/Singapore", "06:00", 2, 5, "2026-11-04T22:00:00Z"},
		{Monthly, "2026-11", "Asia/Singapore", "06:00", 2, 5, "2027-01-04T22:00:00Z"},
		{Monthly, "2026-12", "Asia/Singapore", "06:00", 2, 31, "2027-02-27T22:00:00Z"},
		{Monthly, "2027-12", "Asia/Singapore", "06:00", 2, 31, "2028-02-28T22:00:00Z"},
		{Daily, "2026-10-25", "Asia/Singapore", "06:30", 2, 1, "2026-10-26T22:30:00Z"},
		{Daily, "2026-03-07", "America/New_York", "02:30", 1, 1, "2026-03-08T07:30:00Z"},
		{Daily, "2026-10-31", "America/New_York", "01:30", 1, 1, "2026-11-01T06:30:00Z"},
	}
	for _, c := range cases {
		var got time.Time
		err := s.DB.QueryRow(ctx, `SELECT report_archive_scheduled_due($1,$2,$3,$4,$5,$6)`, c.kind, c.key, c.zone, c.clock, c.offset, c.day).Scan(&got)
		if err != nil || got.UTC().Format(time.RFC3339) != c.want {
			t.Fatalf("schedule %+v = %v (%v)", c, got, err)
		}
	}
	for _, c := range []struct{ at, want string }{
		{"2026-11-04T21:59:59Z", "2026-09"},
		{"2026-11-04T22:00:00Z", "2026-10"},
		{"2026-12-05T00:00:00Z", "2026-11"},
	} {
		var got string
		err := s.DB.QueryRow(ctx, `SELECT report_archive_next_target('monthly','Asia/Singapore','06:00',2,5,$1::timestamptz)`, c.at).Scan(&got)
		if err != nil || got != c.want {
			t.Fatalf("next target at %s = %s (%v), want %s", c.at, got, err, c.want)
		}
	}
}

func TestArchiveScheduleDoesNotRunEarlyAndCapturesDueTargetOnce(t *testing.T) {
	s, a := archiveFixture(t)
	ctx := context.Background()
	var key string
	var version int64
	// This schedule's first target is always in the future, independent of the test machine's local timezone.
	err := s.DB.QueryRow(ctx, `SELECT report_archive_next_target('daily',timezone,'06:00',2,1,statement_timestamp()),version FROM brand_report_archive_policies WHERE brand_id=$1`, testBrand).Scan(&key, &version)
	if err != nil {
		t.Fatal(err)
	}
	schedule := AutomaticSchedule{DailyExecutionTime: "06:00", DailyPeriodOffset: 2, MonthlyExecutionDay: 5, MonthlyExecutionTime: "06:00", MonthlyPeriodOffset: 2}
	p := automaticPolicy(t, s, a, AutomaticPolicyInput{AutomaticSchedule: schedule, Version: version, DailyEnabled: true, DailyStartPeriod: &key})
	if p.NextRuns == nil || p.NextRuns.Daily == nil || p.NextRuns.Daily.PeriodKey != key {
		t.Fatal(p)
	}
	if n, err := s.DiscoverAutomatic(ctx, 1); err != nil || n != 0 {
		t.Fatalf("early discovery %d %v", n, err)
	}
	target, err := time.Parse("2006-01-02", key)
	if err != nil {
		t.Fatal(err)
	}
	elapsed := target.AddDate(0, 0, -1).Format("2006-01-02")
	p = automaticPolicy(t, s, a, AutomaticPolicyInput{AutomaticSchedule: schedule, Version: p.Version, DailyEnabled: true, DailyStartPeriod: &elapsed})
	if n, err := s.DiscoverAutomatic(ctx, 1); err != nil || n != 1 {
		t.Fatalf("due discovery %d %v", n, err)
	}
	if n, err := s.ProcessAutomatic(ctx, 1); err != nil || n != 1 {
		t.Fatalf("due processing %d %v", n, err)
	}
	tasks := automaticTasks(t, s)
	if len(tasks) != 1 || tasks[0].Window.PeriodKey != elapsed || tasks[0].State != "completed" {
		t.Fatal(tasks)
	}
	if n, err := s.DiscoverAutomatic(ctx, 1); err != nil || n != 0 {
		t.Fatalf("duplicate/early discovery %d %v", n, err)
	}
	if n, err := s.ProcessAutomatic(ctx, 1); err != nil || n != 0 {
		t.Fatalf("duplicate processing %d %v", n, err)
	}
}

func TestArchiveSchedulePlatformTimezoneChangeRetainsHistory(t *testing.T) {
	s, a := archiveFixture(t)
	ctx := context.Background()
	schedule := AutomaticSchedule{DailyExecutionTime: "06:00", DailyPeriodOffset: 2, MonthlyExecutionDay: 5, MonthlyExecutionTime: "06:00", MonthlyPeriodOffset: 2}
	key := "2026-01"
	p := automaticPolicy(t, s, a, AutomaticPolicyInput{AutomaticSchedule: schedule, Version: 1, MonthlyEnabled: true, MonthlyStartPeriod: &key})
	if n, err := s.DiscoverAutomatic(ctx, 1); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	before := automaticTasks(t, s)[0]
	if _, err := s.DB.Exec(ctx, `UPDATE platform_settings SET timezone='Asia/Tokyo' WHERE id=true`); err != nil {
		t.Fatal(err)
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	current, err := s.AutomaticPolicyTx(ctx, tx, testBrand)
	if err != nil {
		t.Fatal(err)
	}
	if current.Timezone != "Asia/Tokyo" || current.Version != p.Version+1 || current.NextRuns.Monthly == nil {
		t.Fatal(current)
	}
	after, err := s.AutomaticTaskTx(ctx, tx, testBrand, before.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Window != before.Window || after.PolicyVersion != before.PolicyVersion {
		t.Fatal("timezone update changed retained task", before, after)
	}
	var oldZone string
	if err := tx.QueryRow(ctx, `SELECT timezone FROM report_archive_policy_revisions WHERE brand_id=$1 AND version=$2`, testBrand, p.Version).Scan(&oldZone); err != nil || oldZone != p.Timezone {
		t.Fatal("old policy changed", oldZone, err)
	}
}
