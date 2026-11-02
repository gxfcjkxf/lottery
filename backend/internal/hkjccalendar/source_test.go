package hkjccalendar

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
)

func officialFixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/official-calendar.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseOfficialCalendar(t *testing.T) {
	months, err := parse(officialFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(months) != 3 || months[2].Month.Format("2006-01-02") != "2026-11-01" {
		t.Fatalf("months: %+v", months)
	}
	want := []string{"2026-11-03", "2026-11-05", "2026-11-07", "2026-11-10", "2026-11-12", "2026-11-15", "2026-11-17", "2026-11-19", "2026-11-24", "2026-11-26", "2026-11-28"}
	if !reflect.DeepEqual(months[2].DrawDates, want) || !reflect.DeepEqual(months[2].SnowballDates, []string{"2026-11-24"}) || len(months[2].PresaleDates) != 0 {
		t.Fatalf("November: %+v", months[2])
	}
	// In January 2026, the official source lists January 3 only as a snowball.
	if !contains(months[0].DrawDates, "2026-01-03") || !contains(months[0].SnowballDates, "2026-01-03") {
		t.Fatal("snowball-only draw was omitted", months[0])
	}
}

func TestRejectIncompleteOrInvalidCalendar(t *testing.T) {
	valid := string(officialFixture(t))
	for name, body := range map[string]string{
		"empty":         "{}",
		"graphql error": strings.Replace(valid, `"data":`, `"errors":[{"message":"failed"}],"data":`, 1),
		"no months":     `{"data":{"item":{"years":[]}}}`,
		"invalid day":   strings.Replace(valid, `"value": "03"`, `"value": "32"`, 1),
		"invalid month": strings.Replace(valid, `"value": "01"`, `"value": "13"`, 1),
		"duplicate day": strings.Replace(valid, `"value": "08"`, `"value": "06"`, 1),
		"missing dates": strings.Replace(valid, `"dates":`, `"missing":`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parse([]byte(body)); err == nil {
				t.Fatal("invalid calendar accepted")
			}
		})
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFetchOfficialEndpointsAndPublicPublishKey(t *testing.T) {
	fixture := officialFixture(t)
	calls := 0
	source := NewSource()
	source.Client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		var body string
		switch calls {
		case 1:
			if r.URL.String() != configURL || r.Method != "GET" {
				t.Fatal("unexpected config request")
			}
			body = `const cmsConfig={PUBLISH:{SITECORE_APIKEY:'published-key'},PREVIEW:{SITECORE_APIKEY:'preview-key'}}`
		case 2:
			if r.URL.String() != graphURL || r.Method != "POST" || r.Header.Get("sc_apikey") != "published-key" {
				t.Fatal("unexpected calendar request")
			}
			var request struct {
				Query     string
				Variables map[string]string
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if request.Query != calendarQuery || request.Variables["lang"] != "zh-HK" {
				t.Fatal("wrong GraphQL request")
			}
			body = string(fixture)
		default:
			t.Fatal("unexpected extra request")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	if months, err := source.Fetch(context.Background()); err != nil || len(months) != 3 || calls != 2 {
		t.Fatalf("fetch: %v %v", months, err)
	}
}

func TestFetchRejectsMissingPublishKeyAndHTTPFailure(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"preview only", `PREVIEW:{SITECORE_APIKEY:'preview-key'}`, 200},
		{"HTTP error", ``, 503},
		{"too large", strings.Repeat("x", (128<<10)+1), 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := NewSource()
			calls := 0
			source.Client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})
			if _, err := source.Fetch(context.Background()); err == nil || calls != 1 {
				t.Fatal("invalid config accepted")
			}
		})
	}
	source := NewSource()
	if source.Client.CheckRedirect(&http.Request{}, nil) == nil {
		t.Fatal("redirect allowed")
	}
}
