package platformlottery

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/andybalholm/cascadia"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"golang.org/x/net/html"
)

// A mapping describes source structure, never a lottery name or executable script.
type FeedField struct {
	Capture   string `json:"capture,omitempty"`
	Response  string `json:"response,omitempty"`
	Path      string `json:"path"`
	Attribute string `json:"attribute"`
}
type FeedNumbers struct {
	FeedField
	Separator string `json:"separator"`
	Offset    int    `json:"offset"`
	Count     int    `json:"count"`
}
type FeedCondition struct {
	FeedField
	Equals string `json:"equals"`
}
type FeedMapping struct {
	Browser      *FeedBrowser   `json:"browser,omitempty"`
	Format       string         `json:"format"`
	Schedule     string         `json:"schedule"`
	TimeFormat   string         `json:"time_format"`
	TimeLayout   string         `json:"time_layout"`
	Timezone     string         `json:"timezone"`
	Period       FeedField      `json:"period"`
	TimePeriod   *FeedField     `json:"time_period,omitempty"`
	DrawTime     FeedField      `json:"draw_time"`
	Regular      *FeedNumbers   `json:"regular"`
	Special      *FeedNumbers   `json:"special"`
	Digits       *FeedNumbers   `json:"digits"`
	NextPeriod   *FeedField     `json:"next_period"`
	NextDrawTime *FeedField     `json:"next_draw_time"`
	Complete     *FeedCondition `json:"complete"`
	PeriodQuery  string         `json:"period_query"`
	GameQuery    string         `json:"game_query"`
}
type FeedTimeError struct {
	Field FeedField `json:"field"`
	Value string    `json:"value"`
}

func (e *FeedTimeError) Error() string { return "source time cannot be parsed" }
func (e *FeedTimeError) Unwrap() error { return ErrInvalid }

type FeedReading struct {
	Completed    *SourceObservedDraw `json:"completed"`
	NextPeriodNo *string             `json:"next_period_no"`
	NextDrawAt   *time.Time          `json:"next_draw_at"`
}

func (m FeedMapping) Validate() error {
	if m.Format != "json" && m.Format != "html" && m.Format != "browser" || m.Schedule != "interval" && m.Schedule != "source" {
		return ErrInvalid
	}
	if m.Format == "browser" {
		if m.Browser == nil || m.Browser.TimeoutMS < 1000 || m.Browser.TimeoutMS > 10000 || len(m.Browser.WaitSelector) > 512 {
			return ErrInvalid
		}
		if _, e := cascadia.Compile(m.Browser.WaitSelector); e != nil || m.Browser.WaitSelector == "" {
			return ErrInvalid
		}
	} else if m.Browser != nil {
		return ErrInvalid
	}
	if m.TimeFormat != "rfc3339" && m.TimeFormat != "unix_seconds" && m.TimeFormat != "unix_millis" && m.TimeFormat != "custom" {
		return ErrInvalid
	}
	if m.Browser != nil {
		if m.Browser.Validate() != nil {
			return ErrInvalid
		}
	}
	if m.TimeFormat == "custom" {
		sample := time.Date(2006, 11, 23, 17, 48, 59, 0, time.UTC)
		parsed, e := time.Parse(m.TimeLayout, sample.Format(m.TimeLayout))
		if e != nil || parsed.Year() != 2006 || parsed.Month() != 11 || parsed.Day() != 23 || parsed.Hour() != 17 || parsed.Minute() != 48 {
			return ErrInvalid
		}
		if m.TimeLayout == "" {
			return ErrInvalid
		}
		if _, e := time.LoadLocation(m.Timezone); e != nil || m.Timezone == "" {
			return ErrInvalid
		}
	}
	fields := []FeedField{m.Period, m.DrawTime}
	if m.TimePeriod != nil {
		fields = append(fields, *m.TimePeriod)
	}
	for _, n := range []*FeedNumbers{m.Regular, m.Special, m.Digits} {
		if n != nil {
			if n.Offset < 0 || n.Count < 1 || n.Count > 100 {
				return ErrInvalid
			}
			fields = append(fields, n.FeedField)
		}
	}
	if m.Regular == nil && m.Special == nil && m.Digits == nil {
		return ErrInvalid
	}
	if m.Schedule == "source" {
		if (m.NextPeriod == nil) != (m.NextDrawTime == nil) {
			return ErrInvalid
		}
		if m.NextPeriod != nil {
			fields = append(fields, *m.NextPeriod, *m.NextDrawTime)
		}
	} else if m.NextPeriod != nil || m.NextDrawTime != nil {
		return ErrInvalid
	}
	if m.Complete != nil {
		fields = append(fields, m.Complete.FeedField)
	}
	for _, f := range fields {
		if f.Path == "" || len(f.Path) > 512 || len(f.Attribute) > 120 {
			return ErrInvalid
		}
		if f.Capture != "" {
			r, e := regexp.Compile(f.Capture)
			if e != nil || r.NumSubexp() != 1 || len(f.Capture) > 512 {
				return ErrInvalid
			}
		}
		if f.Response != "" {
			found := false
			if m.Browser != nil {
				for _, q := range m.Browser.JSONRequests {
					if q.ID == f.Response {
						found = true
					}
				}
			}
			if !found {
				return ErrInvalid
			}
		}
		if m.Format == "json" || f.Response != "" {
			if !strings.HasPrefix(f.Path, "/") || f.Attribute != "" {
				return ErrInvalid
			}
		} else if _, e := cascadia.Compile(f.Path); e != nil {
			return ErrInvalid
		}
	}
	for _, q := range []string{m.PeriodQuery, m.GameQuery} {
		if len(q) > 120 || strings.ContainsAny(q, "&=?# \t\r\n") {
			return ErrInvalid
		}
	}
	return nil
}
func nodeText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(nodeText(c))
	}
	return b.String()
}
func mappedScalar(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x), nil
	case json.Number:
		return x.String(), nil
	case bool:
		return strconv.FormatBool(x), nil
	}
	return "", ErrInvalid
}
func ParseFeed(m FeedMapping, body []byte) (out FeedReading, err error) {
	if m.Format == "browser" {
		return out, ErrInvalid
	}
	return parseFeed(m, body, nil)
}
func parseFeed(m FeedMapping, body []byte, responses map[string]json.RawMessage) (out FeedReading, err error) {
	if m.Validate() != nil || len(body) > 1048576 {
		return out, ErrInvalid
	}
	var root any
	var doc *html.Node
	if m.Format == "json" {
		d := json.NewDecoder(bytes.NewReader(body))
		d.UseNumber()
		if d.Decode(&root) != nil {
			return out, ErrInvalid
		}
		var extra any
		if d.Decode(&extra) != io.EOF {
			return out, ErrInvalid
		}
	} else {
		doc, err = html.Parse(bytes.NewReader(body))
		if err != nil {
			return out, ErrInvalid
		}
	}
	readRaw := func(f FeedField) ([]string, error) {
		if m.Format != "json" && f.Response == "" {
			selector, e := cascadia.Compile(f.Path)
			if e != nil {
				return nil, ErrInvalid
			}
			nodes := cascadia.QueryAll(doc, selector)
			if len(nodes) == 0 {
				return nil, ErrInvalid
			}
			out := []string{}
			for _, n := range nodes {
				v := nodeText(n)
				if f.Attribute != "" {
					found := false
					for _, a := range n.Attr {
						if a.Key == f.Attribute {
							v = a.Val
							found = true
							break
						}
					}
					if !found {
						return nil, ErrInvalid
					}
				}
				out = append(out, strings.TrimSpace(v))
			}
			return out, nil
		}
		value := root
		if f.Response != "" {
			raw, ok := responses[f.Response]
			if !ok || len(raw) > 1048576 {
				return nil, ErrInvalid
			}
			d := json.NewDecoder(bytes.NewReader(raw))
			d.UseNumber()
			if d.Decode(&value) != nil {
				return nil, ErrInvalid
			}
		}
		for _, part := range strings.Split(f.Path[1:], "/") {
			part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
			switch x := value.(type) {
			case map[string]any:
				var ok bool
				value, ok = x[part]
				if !ok {
					return nil, ErrInvalid
				}
			case []any:
				i, e := strconv.Atoi(part)
				if e != nil || i < 0 || i >= len(x) {
					return nil, ErrInvalid
				}
				value = x[i]
			default:
				return nil, ErrInvalid
			}
		}
		values, ok := value.([]any)
		if !ok {
			values = []any{value}
		}
		out := []string{}
		for _, v := range values {
			s, e := mappedScalar(v)
			if e != nil {
				return nil, e
			}
			out = append(out, s)
		}
		return out, nil
	}
	read := func(f FeedField) ([]string, error) {
		values, e := readRaw(f)
		if e != nil {
			return nil, e
		}
		if f.Capture != "" {
			r := regexp.MustCompile(f.Capture)
			for i, v := range values {
				match := r.FindStringSubmatch(v)
				if len(match) != 2 {
					return nil, ErrInvalid
				}
				values[i] = match[1]
			}
		}
		return values, nil
	}
	scalar := func(f FeedField) (string, error) {
		v, e := read(f)
		if e != nil || len(v) != 1 || v[0] == "" {
			return "", ErrInvalid
		}
		return v[0], nil
	}
	parseTime := func(f FeedField) (at time.Time, err error) {
		v, e := scalar(f)
		defer func() {
			if err != nil {
				safe := ""
				if len(v) < 100 && regexp.MustCompile(`^([0-9]{4}[-/][0-9]{2}[-/][0-9]{2}|/Date\([0-9]+)`).MatchString(v) {
					safe = v
				}
				err = &FeedTimeError{Field: f, Value: safe}
			}
		}()
		if e != nil {
			return time.Time{}, e
		}
		switch m.TimeFormat {
		case "rfc3339":
			return time.Parse(time.RFC3339, v)
		case "unix_seconds", "unix_millis":
			n, e := strconv.ParseInt(v, 10, 64)
			if e != nil {
				return time.Time{}, ErrInvalid
			}
			if m.TimeFormat == "unix_millis" {
				return time.UnixMilli(n).UTC(), nil
			}
			return time.Unix(n, 0).UTC(), nil
		case "custom":
			loc, e := time.LoadLocation(m.Timezone)
			if e != nil {
				return time.Time{}, ErrInvalid
			}
			return time.ParseInLocation(m.TimeLayout, v, loc)
		}
		return time.Time{}, ErrInvalid
	}
	if m.Schedule == "source" && m.NextPeriod != nil {
		no, e := scalar(*m.NextPeriod)
		if e != nil {
			return out, e
		}
		at, e := parseTime(*m.NextDrawTime)
		if e != nil {
			return out, e
		}
		if at.IsZero() || at.Year() < 1970 {
			return out, ErrInvalid
		}
		out.NextPeriodNo = &no
		at = at.UTC()
		out.NextDrawAt = &at
	}
	if m.Complete != nil {
		v, e := scalar(m.Complete.FeedField)
		if e != nil {
			return out, e
		}
		if v != m.Complete.Equals {
			return out, nil
		}
	}
	no, e := scalar(m.Period)
	if e != nil || !textOK(no, 100) {
		return out, ErrInvalid
	}
	if m.TimePeriod != nil {
		timePeriod, e := scalar(*m.TimePeriod)
		if e != nil || timePeriod != no {
			return out, ErrInvalid
		}
	}
	at, e := parseTime(m.DrawTime)
	if e != nil {
		return out, e
	}
	if at.IsZero() || at.Year() < 1970 {
		return out, ErrInvalid
	}
	numbers := func(n *FeedNumbers) ([]int, error) {
		out := []int{}
		if n == nil {
			return out, nil
		}
		values, e := read(n.FeedField)
		if e != nil {
			return nil, e
		}
		parts := []string{}
		for _, v := range values {
			if n.Separator != "" {
				parts = append(parts, strings.Split(v, n.Separator)...)
			} else {
				parts = append(parts, v)
			}
		}
		if n.Offset > len(parts) || n.Count > len(parts)-n.Offset {
			return nil, ErrInvalid
		}
		for _, v := range parts[n.Offset : n.Offset+n.Count] {
			i, e := strconv.Atoi(strings.TrimSpace(v))
			if e != nil {
				return nil, ErrInvalid
			}
			out = append(out, i)
		}
		return out, nil
	}
	draw := rules.Draw{}
	if draw.Regular, e = numbers(m.Regular); e != nil {
		return out, e
	}
	if draw.Special, e = numbers(m.Special); e != nil {
		return out, e
	}
	if draw.Digits, e = numbers(m.Digits); e != nil {
		return out, e
	}
	out.Completed = &SourceObservedDraw{PeriodNo: no, DrawnAt: at.UTC(), Result: draw}
	return out, nil
}
func FetchMappedFeed(ctx context.Context, s AutomationSources, p AutomationPlan, period, game string) (FeedReading, error) {
	if p.Mapping == nil || p.Mapping.Validate() != nil || !s.Allows(p.SourceURL) {
		return FeedReading{}, ErrInvalid
	}
	u, e := url.Parse(p.SourceURL)
	if e != nil {
		return FeedReading{}, e
	}
	q := u.Query()
	if p.Mapping.PeriodQuery != "" {
		q.Set(p.Mapping.PeriodQuery, period)
	}
	if p.Mapping.GameQuery != "" {
		q.Set(p.Mapping.GameQuery, game)
	}
	u.RawQuery = q.Encode()
	token := ""
	if p.CredentialRef != "" {
		token = os.Getenv(p.CredentialRef)
		if token == "" {
			return FeedReading{}, ErrInvalid
		}
	}
	if p.Mapping.Format == "browser" {
		body, e := renderFeed(ctx, s, p, u.String(), token)
		if e != nil {
			return FeedReading{}, e
		}
		return parseFeed(*p.Mapping, []byte(body.HTML), body.Responses)
	}
	req, e := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if e != nil {
		return FeedReading{}, e
	}
	req.Header.Set("Accept", map[string]string{"json": "application/json", "html": "text/html"}[p.Mapping.Format])
	req.Header.Set("Cache-Control", "no-cache")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, e := s.Client.Do(req)
	if e != nil {
		return FeedReading{}, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return FeedReading{}, ErrInvalid
	}
	body, e := io.ReadAll(io.LimitReader(resp.Body, 1048577))
	if e != nil {
		return FeedReading{}, e
	}
	return ParseFeed(*p.Mapping, body)
}

func validateMappingModel(p AutomationPlan, model rules.Model) error {
	if p.SourceType != "mapped" {
		return nil
	}
	if p.Mapping == nil {
		return ErrInvalid
	}
	for _, pair := range []struct {
		field *FeedNumbers
		count int
	}{{p.Mapping.Regular, model.RegularCount}, {p.Mapping.Special, model.SpecialCount}, {p.Mapping.Digits, model.Length}} {
		if pair.count == 0 {
			if pair.field != nil {
				return ErrInvalid
			}
		} else if pair.field == nil || pair.field.Count != pair.count {
			return ErrInvalid
		}
	}
	return nil
}
