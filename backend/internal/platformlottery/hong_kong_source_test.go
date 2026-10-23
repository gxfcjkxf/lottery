package platformlottery

import (
	"encoding/json"
	"testing"
	"time"
)

func TestHongKongParserRequiresCompleteCurrentAuthenticPayload(t *testing.T) {
	now := time.Date(2026, 10, 16, 8, 0, 0, 0, hkLocation)
	data := map[string]any{"preDrawIssue": 2026110, "drawIssue": 2026111, "preDrawTime": "2026-10-15 21:41:21", "drawTime": "2026-10-17 21:30:00", "serverTime": "2026-10-16 08:00:00", "type": 8, "lotCode": 10048, "preDrawCode": "3,10,7,8,36,42,20", "chineseZodiac": []int{5, 10, 1, 12, 8, 2, 12}, "fiveElements": []int{4, 4, 5, 2, 5, 1, 5}, "color": []int{3, 3, 1, 1, 3, 3, 3}}
	raw := func(d map[string]any) []byte {
		b, _ := json.Marshal(map[string]any{"errorCode": 0, "result": map[string]any{"businessCode": 0, "data": d}})
		return b
	}
	r, e := parseHongKong(raw(data), now)
	if e != nil || r.Completed == nil || r.Completed.PeriodNo != "2026110" || r.Completed.Result.Special[0] != 20 || r.Completed.Attributes.Waves["03"] != "blue" || r.Completed.Attributes.Elements["42"] != "metal" || r.NextNo != "2026111" || r.NextAt.Format(time.RFC3339) != "2026-10-17T13:30:00Z" {
		t.Fatal(r, e)
	}
	for _, field := range []string{"serverTime", "type", "lotCode", "drawIssue", "drawTime", "preDrawIssue", "preDrawTime", "preDrawCode", "color", "chineseZodiac", "fiveElements"} {
		t.Run("missing_"+field, func(t *testing.T) {
			d := map[string]any{}
			for k, v := range data {
				if k != field {
					d[k] = v
				}
			}
			if _, e := parseHongKong(raw(d), now); e == nil {
				t.Fatal("accepted incomplete source")
			}
		})
	}
	for _, mutate := range []func(map[string]any){func(d map[string]any) { d["preDrawCode"] = "3,3,7,8,36,42,20" }, func(d map[string]any) { d["serverTime"] = "2026-10-15 21:41:21" }, func(d map[string]any) { d["lotCode"] = 10049 }, func(d map[string]any) { d["drawIssue"] = 2026110 }, func(d map[string]any) { d["color"] = []int{0, 3, 1, 1, 3, 3, 3} }} {
		d := map[string]any{}
		for k, v := range data {
			d[k] = v
		}
		mutate(d)
		if _, e := parseHongKong(raw(d), now); e == nil {
			t.Fatal("accepted invalid source")
		}
	}
	data["type"] = 1
	data["preDrawCode"] = "3,10"
	r, e = parseHongKong(raw(data), now)
	if e != nil || r.Completed != nil {
		t.Fatal("partial draw became payable", r, e)
	}
	if _, e = parseHongKong([]byte(`{"result":{"data":{}}}`), now); e == nil {
		t.Fatal("absent success code accepted")
	}
}
