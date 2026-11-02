// Package hkjccalendar mirrors the official date calendar only. It never
// generates period numbers, draw times, betting periods or results.
package hkjccalendar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"time"
)

const configURL = "https://bet.hkjc.com/Config/GlobalConfig.js"
const graphURL = "https://consvc.hkjc.com/JCBW/api/graph"
const calendarQuery = `query MarksixFixtures($path: String!, $lang: String!) {
 item(path: $path, language: $lang) { years: children {
  year: name
  months: children {
   month: field(name: "DrawMonth") { value }
   dates: field(name: "NormalDrawDates") { ... on MultilistField { date: targetItems { value: name } } }
   snowballs: field(name: "SnowballDrawDates") { ... on MultilistField { date: targetItems { value: name } } }
   presales: field(name: "PresellDrawDates") { ... on MultilistField { date: targetItems { value: name } } }
  }
 } }
}`

// This is the public website's published CMS key, read afresh on each sync.
// Match PUBLISH specifically; preview credentials are never used.
var publishKey = regexp.MustCompile(`(?s)PUBLISH:\s*\{[^}]*SITECORE_APIKEY:\s*['"]([^'"]+)['"]`)

type Month struct {
	Month         time.Time
	DrawDates     []string
	SnowballDates []string
	PresaleDates  []string
}

type Source struct{ Client *http.Client }

func NewSource() Source {
	return Source{Client: &http.Client{
		Timeout:       15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("HKJC redirect rejected") },
	}}
}

func (s Source) read(req *http.Request, limit int64) ([]byte, error) {
	r, err := s.Client.Do(req)
	if err != nil {
		return nil, errors.New("HKJC request failed")
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HKJC HTTP status %d", r.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil || int64(len(body)) > limit {
		return nil, errors.New("HKJC response unreadable or too large")
	}
	return body, nil
}

func (s Source) Fetch(ctx context.Context) ([]Month, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, configURL, nil)
	if err != nil {
		return nil, err
	}
	config, err := s.read(req, 128<<10)
	if err != nil {
		return nil, err
	}
	key := publishKey.FindSubmatch(config)
	if len(key) != 2 {
		return nil, errors.New("HKJC published CMS key missing")
	}
	body, err := json.Marshal(map[string]any{
		"query":     calendarQuery,
		"variables": map[string]string{"path": "/sitecore/content/Sites/JCBW/NextDrawSchedule/Schedule", "lang": "zh-HK"},
	})
	if err != nil {
		return nil, err
	}
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, graphURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("sc_apikey", string(key[1]))
	response, err := s.read(req, 1<<20)
	if err != nil {
		return nil, err
	}
	return parse(response)
}

type value struct {
	Value string `json:"value"`
}
type dates struct {
	Date *[]value `json:"date"`
}

func parse(body []byte) ([]Month, error) {
	var response struct {
		Errors []json.RawMessage `json:"errors"`
		Data   struct {
			Item *struct {
				Years []struct {
					Year   string `json:"year"`
					Months []struct {
						Month     *value `json:"month"`
						Dates     dates  `json:"dates"`
						Snowballs dates  `json:"snowballs"`
						Presales  dates  `json:"presales"`
					} `json:"months"`
				} `json:"years"`
			} `json:"item"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil || len(response.Errors) > 0 || response.Data.Item == nil {
		return nil, errors.New("HKJC calendar response invalid")
	}
	out := []Month{}
	seen := map[string]bool{}
	for _, y := range response.Data.Item.Years {
		year, err := strconv.Atoi(y.Year)
		if err != nil || len(y.Year) != 4 || year < 1 {
			return nil, errors.New("HKJC year invalid")
		}
		for _, m := range y.Months {
			if m.Month == nil {
				return nil, errors.New("HKJC month missing")
			}
			n, err := strconv.Atoi(m.Month.Value)
			if err != nil || n < 1 || n > 12 {
				return nil, errors.New("HKJC month invalid")
			}
			first := time.Date(year, time.Month(n), 1, 0, 0, 0, 0, time.UTC)
			key := first.Format("2006-01-02")
			if seen[key] {
				return nil, errors.New("HKJC duplicate month")
			}
			seen[key] = true
			draw, err := parseDates(first, m.Dates)
			if err != nil {
				return nil, err
			}
			snow, err := parseDates(first, m.Snowballs)
			if err != nil {
				return nil, err
			}
			pre, err := parseDates(first, m.Presales)
			if err != nil {
				return nil, err
			}
			for _, d := range snow {
				if !contains(draw, d) {
					draw = append(draw, d)
				}
			}
			sort.Strings(draw)
			out = append(out, Month{first, draw, snow, pre})
		}
	}
	if len(out) == 0 {
		return nil, errors.New("HKJC calendar empty")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Month.Before(out[j].Month) })
	return out, nil
}

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func parseDates(first time.Time, field dates) ([]string, error) {
	if field.Date == nil {
		return nil, errors.New("HKJC dates missing")
	}
	out := []string{}
	for _, v := range *field.Date {
		d, err := strconv.Atoi(v.Value)
		date := time.Date(first.Year(), first.Month(), d, 0, 0, 0, 0, time.UTC)
		if err != nil || d < 1 || date.Month() != first.Month() {
			return nil, errors.New("HKJC date invalid")
		}
		value := date.Format("2006-01-02")
		if contains(out, value) {
			return nil, errors.New("HKJC duplicate date")
		}
		out = append(out, value)
	}
	sort.Strings(out)
	return out, nil
}
