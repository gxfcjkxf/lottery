package platformlottery

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestMappedClassNumbersAndResponseTime(t *testing.T) {
	m := testFeedMapping("html")
	m.Format, m.Schedule = "browser", "source"
	m.Browser = &FeedBrowser{WaitSelector: ".result", TimeoutMS: 5000, JSONRequests: []FeedJSONRequest{{ID: "history", URL: "https://source.example/history", Method: "POST"}}}
	m.TimeFormat, m.TimeLayout, m.Timezone = "custom", "2006-01-02 15:04:05", "Asia/Shanghai"
	m.DrawTime = FeedField{Response: "history", Path: "/data/0/time"}
	m.TimePeriod = &FeedField{Response: "history", Path: "/data/0/issue"}
	m.Regular = &FeedNumbers{FeedField: FeedField{Path: ".regular i", Attribute: "class", Capture: `ball_([0-9]+)`}, Count: 2}
	m.Special = &FeedNumbers{FeedField: FeedField{Path: ".special i", Attribute: "class", Capture: `ball_([0-9]+)`}, Count: 1}
	body := []byte(`<section class="result"><b class="issue">round-007</b><div class="regular"><i class="ball_01 ">&nbsp;</i><i class="ball_23 ">&nbsp;</i></div><div class="special"><i class="ball_49 ">&nbsp;</i></div></section>`)
	responses := map[string]json.RawMessage{"history": json.RawMessage(`{"data":[{"issue":"round-007","time":"2026-10-19 21:30:00"}]}`)}
	r, err := parseFeed(m, body, responses)
	if err != nil || r.Completed == nil || r.Completed.Result.Regular[1] != 23 || r.Completed.Result.Special[0] != 49 || r.Completed.DrawnAt.Format(time.RFC3339) != "2026-10-19T13:30:00Z" || r.NextDrawAt != nil {
		t.Fatal(r, err)
	}
	for name, invalid := range map[string]json.RawMessage{
		"different period":       json.RawMessage(`{"data":[{"issue":"round-008","time":"2026-10-19 21:30:00"}]}`),
		"missing year and clock": json.RawMessage(`{"data":[{"issue":"round-007","time":"10-19"}]}`),
		"missing time":           json.RawMessage(`{"data":[{"issue":"round-007"}]}`),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseFeed(m, body, map[string]json.RawMessage{"history": invalid}); err == nil {
				t.Fatal("invalid source accepted")
			}
		})
	}
	m.Regular.Capture = `ball_[0-9]+`
	if m.Validate() == nil {
		t.Fatal("capture without a group accepted")
	}
	m.Regular.Capture = `ball_([0-9]+)`
	for _, layout := range []string{"01-02", "2006-01-02", "15:04:05"} {
		m.TimeLayout = layout
		if m.Validate() == nil {
			t.Fatalf("incomplete time format accepted: %s", layout)
		}
	}
}

func testFeedMapping(format string) FeedMapping {
	m := FeedMapping{Format: format, Schedule: "interval", TimeFormat: "rfc3339", Period: FeedField{Path: "/rows/0/issue"}, DrawTime: FeedField{Path: "/rows/0/time"}, Regular: &FeedNumbers{FeedField: FeedField{Path: "/rows/0/balls"}, Separator: ",", Count: 2}, Special: &FeedNumbers{FeedField: FeedField{Path: "/rows/0/balls"}, Separator: ",", Offset: 2, Count: 1}}
	if format == "html" {
		m.Period = FeedField{Path: ".result .issue"}
		m.DrawTime = FeedField{Path: ".result time", Attribute: "datetime"}
		m.Regular = &FeedNumbers{FeedField: FeedField{Path: ".result .regular .ball"}, Count: 2}
		m.Special = &FeedNumbers{FeedField: FeedField{Path: ".result .special", Attribute: "data-number"}, Count: 1}
	}
	return m
}
func TestMappedJSONAndDOMSources(t *testing.T) {
	jsonBody := `{"rows":[{"issue":"round-007","time":"2026-10-20T20:00:00+08:00","balls":"01, 23, 49"}]}`
	htmlBody := `<html><body><script>var balls=[8,8,8]</script><section class="result"><b class="issue">round-007</b><time datetime="2026-10-20T20:00:00+08:00">8 PM</time><div class="regular"><i class="ball">01</i><i class="ball"><b>23</b></i></div><i class="special" data-number="49"></i></section></body></html>`
	for format, body := range map[string]string{"json": jsonBody, "html": htmlBody} {
		t.Run(format, func(t *testing.T) {
			m := testFeedMapping(format)
			r, e := ParseFeed(m, []byte(body))
			if e != nil || r.Completed == nil || r.Completed.PeriodNo != "round-007" || r.Completed.Result.Regular[1] != 23 || r.Completed.Result.Special[0] != 49 || !r.Completed.DrawnAt.Equal(time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)) {
				t.Fatal(r, e)
			}
			m.Period.Path = "missing"
			if _, e = ParseFeed(m, []byte(body)); e == nil {
				t.Fatal("missing mapping accepted")
			}
		})
	}
	m := testFeedMapping("html")
	m.Period.Path = ".ball"
	if _, e := ParseFeed(m, []byte(htmlBody)); e == nil {
		t.Fatal("ambiguous scalar accepted")
	}
	m.Period.Path = "["
	if m.Validate() == nil {
		t.Fatal("invalid CSS accepted")
	}
	if _, e := ParseFeed(testFeedMapping("html"), []byte(`<html><div id="app"></div><script>render()</script></html>`)); e == nil {
		t.Fatal("unrendered DOM succeeded")
	}
	if _, e := ParseFeed(testFeedMapping("json"), []byte(jsonBody+` {}`)); e == nil {
		t.Fatal("extra JSON accepted")
	}
	if _, e := ParseFeed(testFeedMapping("html"), []byte(strings.Repeat("a", 1048577))); e == nil {
		t.Fatal("oversized body accepted")
	}
}
func TestMappedSourceScheduleAndTimeFormats(t *testing.T) {
	m := testFeedMapping("json")
	m.Schedule = "source"
	m.NextPeriod = &FeedField{Path: "/next/id"}
	m.NextDrawTime = &FeedField{Path: "/next/at"}
	m.Complete = &FeedCondition{FeedField: FeedField{Path: "/status"}, Equals: "done"}
	m.TimeFormat = "custom"
	m.TimeLayout = "2006-01-02 15:04:05"
	m.Timezone = "Asia/Shanghai"
	body := []byte(`{"status":"pending","next":{"id":"r2","at":"2026-10-21 20:00:00"}}`)
	r, e := ParseFeed(m, body)
	if e != nil || r.Completed != nil || r.NextPeriodNo == nil || *r.NextPeriodNo != "r2" || r.NextDrawAt.Hour() != 12 {
		t.Fatal(r, e)
	}
	m.Timezone = "invalid"
	if m.Validate() == nil {
		t.Fatal("invalid timezone accepted")
	}
	for format, value := range map[string]string{"unix_seconds": "1792497600", "unix_millis": "1792497600000"} {
		m = testFeedMapping("json")
		m.TimeFormat = format
		body := `{"rows":[{"issue":7,"time":` + value + `,"balls":[1,23,49]}]}`
		m.Regular.Separator = ""
		m.Special.Separator = ""
		r, e := ParseFeed(m, []byte(body))
		if e != nil || r.Completed.PeriodNo != "7" || r.Completed.DrawnAt.Unix() != 1792497600 {
			t.Fatal(r, e)
		}
	}
}

func TestMappedBrowserConfigurationAndNoStaticFallback(t *testing.T) {
	m := testFeedMapping("html")
	m.Format = "browser"
	if m.Validate() == nil {
		t.Fatal("missing browser configuration accepted")
	}
	m.Browser = &FeedBrowser{WaitSelector: "#ready", TimeoutMS: 5000}
	if e := m.Validate(); e != nil {
		t.Fatal(e)
	}
	if _, e := ParseFeed(m, []byte(`<b id="ready">static sample</b>`)); e == nil {
		t.Fatal("browser source silently parsed static sample")
	}
	m.Browser.WaitSelector = "["
	if m.Validate() == nil {
		t.Fatal("invalid selector accepted")
	}
	m.Browser.WaitSelector = "#ready"
	m.Browser.TimeoutMS = 0
	if m.Validate() == nil {
		t.Fatal("invalid timeout accepted")
	}
}
