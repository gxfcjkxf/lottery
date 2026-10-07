package commission

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestCalendarSQLMatchesHistoricalGoWindowsAndRejectsDST(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	for _, zone := range []string{"UTC", "Asia/Manila", "America/New_York", "Australia/Lord_Howe"} {
		calendars := []Calendar{
			{Timezone: zone, Cycle: "weekly", BoundaryTime: "18:30:00", Weekday: intPtr(1)},
			{Timezone: zone, Cycle: "monthly", BoundaryTime: "06:07:08", MonthDay: intPtr(31), ShortMonth: "last_day"},
			{Timezone: zone, Cycle: "monthly", BoundaryTime: "06:07:08", MonthDay: intPtr(31), ShortMonth: "skip"},
		}
		for _, c := range calendars {
			raw, _ := json.Marshal(c)
			for month := 1; month <= 12; month++ {
				at := time.Date(2024, time.Month(month), 15, 3, 4, 5, 0, time.UTC)
				w, err := c.WindowAt(at)
				if err != nil {
					t.Fatal(err)
				}
				for _, sample := range []time.Time{at, w.From, w.To, w.From.Add(-time.Microsecond)} {
					expected, e := c.WindowAt(sample)
					if e != nil {
						t.Fatal(e)
					}
					var from, to time.Time
					e = db.QueryRow(ctx, `SELECT window_from,window_to FROM commission_calendar_window($1,$2)`, raw, sample).Scan(&from, &to)
					if e != nil || !from.Equal(expected.From) || !to.Equal(expected.To) {
						t.Fatalf("SQL calendar %+v at %s got[%s,%s) want%+v err=%v", c, sample, from, to, expected, e)
					}
				}
			}
		}
	}
	for _, v := range []struct{ clock, at string }{{"02:30:00", "2024-03-11T12:00:00Z"}, {"01:30:00", "2024-11-04T12:00:00Z"}} {
		c := Calendar{Timezone: "America/New_York", Cycle: "weekly", BoundaryTime: v.clock, Weekday: intPtr(0)}
		raw, _ := json.Marshal(c)
		var from, to time.Time
		if err := db.QueryRow(ctx, `SELECT window_from,window_to FROM commission_calendar_window($1,$2)`, raw, mustTime(t, v.at)).Scan(&from, &to); err == nil {
			t.Fatal("SQL silently resolved ambiguous/nonexistent calendar boundary")
		}
	}
	// Pinned functions must keep resolving the real helpers after restore or
	// an untrusted caller supplies an empty application search_path.
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var schema string
	if err = tx.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `SET LOCAL search_path TO pg_catalog`); err != nil {
		t.Fatal(err)
	}
	c := Calendar{Timezone: "UTC", Cycle: "weekly", BoundaryTime: "18:30:00", Weekday: intPtr(1)}
	raw, _ := json.Marshal(c)
	var from, to time.Time
	if err = tx.QueryRow(ctx, `SELECT window_from,window_to FROM "`+schema+`".commission_calendar_window($1,$2)`, raw, mustTime(t, "2024-01-10T10:30:00Z")).Scan(&from, &to); err != nil {
		t.Fatal(err)
	}
}
