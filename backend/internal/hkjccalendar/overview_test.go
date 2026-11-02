package hkjccalendar

import (
	"context"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestOverviewCompleteMonthsPresalesAndSyncStates(t *testing.T) {
	db := testdb.NewUnseeded(t)
	ctx := context.Background()
	s := Service{DB: db}
	out, err := s.Overview(ctx)
	if err != nil || out.Status != "pending" || out.Months == nil || len(out.Months) != 0 || len(out.CurrentMonth) != 7 || out.LastSuccessAt != nil || out.NextAttemptAt != nil {
		t.Fatalf("uninitialized: %+v %v", out, err)
	}
	var today time.Time
	if err = db.QueryRow(ctx, `SELECT (clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date`).Scan(&today); err != nil {
		t.Fatal(err)
	}
	dates := []string{today.AddDate(0, 0, -1).Format("2006-01-02"), today.Format("2006-01-02"), today.AddDate(0, 0, 2).Format("2006-01-02")}
	if _, err = db.Exec(ctx, `INSERT INTO hkjc_calendar_months(month,draw_dates,snowball_dates,presale_dates,synced_at) VALUES(date_trunc('month',$1::date),$2::text[]::date[],ARRAY[$3::date],ARRAY[$1::date],now())`, today, dates, dates[2]); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO hkjc_calendar_sync(id,last_success_at,last_attempt_at,next_attempt_at) VALUES(1,now(),now(),now()+interval '1 day')`); err != nil {
		t.Fatal(err)
	}
	out, err = s.Overview(ctx)
	if err != nil || out.Status != "synced" || len(out.Months) != 1 || len(out.Months[0].DrawDates) != 3 || out.Months[0].DrawDates[0] != dates[0] || out.Months[0].SnowballDates[0] != dates[2] || out.Months[0].PresaleDates[0] != dates[1] || out.LastSuccessAt == nil {
		t.Fatalf("synced: %+v %v", out, err)
	}
	if _, err = db.Exec(ctx, `UPDATE hkjc_calendar_sync SET last_error_code='HKJC_CALENDAR_FETCH_FAILED'`); err != nil {
		t.Fatal(err)
	}
	out, err = s.Overview(ctx)
	if err != nil || out.Status != "failed" || len(out.Months) != 1 || out.LastSuccessAt == nil {
		t.Fatalf("failed: %+v %v", out, err)
	}
	if _, err = db.Exec(ctx, `UPDATE hkjc_calendar_sync SET last_error_code='',next_attempt_at=now()-interval '6 minutes'`); err != nil {
		t.Fatal(err)
	}
	out, err = s.Overview(ctx)
	if err != nil || out.Status != "overdue" || len(out.Months) != 1 {
		t.Fatalf("overdue: %+v %v", out, err)
	}
	if _, err = db.Exec(ctx, `UPDATE hkjc_calendar_sync SET last_success_at=NULL,next_attempt_at='-infinity'`); err != nil {
		t.Fatal(err)
	}
	out, err = s.Overview(ctx)
	if err != nil || out.Status != "pending" || out.NextAttemptAt != nil {
		t.Fatalf("pending: %+v %v", out, err)
	}
}
