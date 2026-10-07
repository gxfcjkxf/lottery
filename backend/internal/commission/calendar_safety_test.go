package commission

import (
	"testing"
	"time"
)

func TestCalendarWindowsContainTimestampAndMeetWithoutOverlap(t *testing.T) {
	for _, zone := range []string{"UTC", "Asia/Manila", "America/New_York", "Australia/Lord_Howe"} {
		for _, c := range []Calendar{
			{Timezone: zone, Cycle: "weekly", BoundaryTime: "12:34:56", Weekday: intPtr(1)},
			{Timezone: zone, Cycle: "monthly", BoundaryTime: "12:34:56", MonthDay: intPtr(31), ShortMonth: "last_day"},
			{Timezone: zone, Cycle: "monthly", BoundaryTime: "12:34:56", MonthDay: intPtr(31), ShortMonth: "skip"},
		} {
			for month := time.January; month <= time.December; month++ {
				for _, day := range []int{1, 15, 28} {
					at := time.Date(2024, month, day, 23, 59, 59, 999999999, time.UTC)
					w, err := c.WindowAt(at)
					if err != nil || w.From.After(at) || !w.To.After(at) || !w.To.After(w.From) {
						t.Fatal(c, at, w, err)
					}
					first, err := c.WindowAt(w.From)
					if err != nil || !first.From.Equal(w.From) || !first.To.Equal(w.To) {
						t.Fatal("start boundary does not belong to its cycle", c, w, first, err)
					}
					next, err := c.WindowAt(w.To)
					if err != nil || !next.From.Equal(w.To) || !next.To.After(w.To) {
						t.Fatal("adjacent cycles overlap or leave a gap", c, w, next, err)
					}
				}
			}
		}
	}
}
