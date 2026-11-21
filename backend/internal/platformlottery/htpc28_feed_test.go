package platformlottery

import (
	"reflect"
	"strings"
	"testing"
)

func TestHTPC28ObjectArrayMapping(t *testing.T) {
	m := FeedMapping{Format: "json", Schedule: "game", TimeFormat: "custom", TimeLayout: "2006-01-02 15:04:05", Timezone: "Asia/Hong_Kong", Period: FeedField{Path: "/data/lhc_current_period_sn"}, DrawTime: FeedField{Path: "/data/lhc_current_open_time"}, Regular: &FeedNumbers{FeedField: FeedField{Path: "/data/lottery_numbers/*/num"}, Count: 6}, Special: &FeedNumbers{FeedField: FeedField{Path: "/data/special_number/num"}, Count: 1}, Complete: &FeedCondition{FeedField: FeedField{Path: "/code"}, Equals: "0"}}
	body := `{"code":0,"data":{"lhc_current_open_time":"2026-11-15 21:30:00","lhc_current_period_sn":"2026121","lottery_numbers":[{"num":8,"zodiac":"猪","color":"red"},{"num":35,"zodiac":"猴","color":"red"},{"num":47,"zodiac":"猴","color":"blue"},{"num":43,"zodiac":"鼠","color":"green"},{"num":11,"zodiac":"猴","color":"green"},{"num":2,"zodiac":"蛇","color":"red"}],"special_number":{"num":48,"zodiac":"羊","color":"blue"}}}`
	reading, err := ParseFeed(m, []byte(body))
	if err != nil || reading.Completed == nil {
		t.Fatal(reading, err)
	}
	if reading.Completed.PeriodNo != "2026121" || !reflect.DeepEqual(reading.Completed.Result.Regular, []int{8, 35, 47, 43, 11, 2}) || reading.Completed.Result.Special[0] != 48 || reading.Completed.DrawnAt.Format("2006-01-02T15:04:05Z07:00") != "2026-11-15T13:30:00Z" {
		t.Fatalf("incorrect source mapping: %+v", reading.Completed)
	}
	for _, bad := range []string{strings.Replace(body, `"num":8`, `"wrong":8`, 1), strings.Replace(body, `"num":8`, `"num":null`, 1)} {
		if _, err := ParseFeed(m, []byte(bad)); err == nil {
			t.Fatal("invalid number accepted")
		}
	}
}
