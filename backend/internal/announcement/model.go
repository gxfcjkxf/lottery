package announcement

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var ErrInvalid = errors.New("announcement input invalid")
var ErrDenied = errors.New("announcement permission denied")
var ErrConflict = errors.New("announcement version or brand state conflict")
var ErrNotFound = errors.New("announcement not found")
var uuid = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
var asset = regexp.MustCompile(`^/api/v1/announcement-assets/([a-f0-9]{64})\.(png|jpg|webp)$`)

type Copy struct {
	Title  string `json:"title"`
	HTML   string `json:"html"`
	Button string `json:"button"`
	URL    string `json:"url"`
}
type Content struct {
	En Copy `json:"en"`
	Zh Copy `json:"zh-CN"`
}
type Config struct {
	Content      Content    `json:"content"`
	Targets      []string   `json:"targets"`
	Audience     string     `json:"audience"`
	Popup        bool       `json:"popup"`
	Frequency    string     `json:"frequency"`
	IntervalDays int        `json:"interval_days"`
	AllowMute    bool       `json:"allow_mute"`
	Pinned       bool       `json:"pinned"`
	Sort         int        `json:"sort"`
	StartsAt     *time.Time `json:"starts_at"`
	EndsAt       *time.Time `json:"ends_at"`
}
type Input struct {
	Version int64  `json:"version"`
	Status  string `json:"status"`
	Remind  bool   `json:"remind"`
	Config  Config `json:"config"`
}
type Item struct {
	ID          string     `json:"id"`
	BrandID     *string    `json:"brand_id"`
	Version     int64      `json:"version"`
	Generation  int64      `json:"generation"`
	Status      string     `json:"status"`
	State       string     `json:"state"`
	Config      Config     `json:"config"`
	PublishedAt *time.Time `json:"published_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	Eligible    bool       `json:"eligible"`
}

func ValidID(s string) bool { return uuid.MatchString(s) }
func AssetID(s string) string {
	m := asset.FindStringSubmatch(s)
	if len(m) == 0 {
		return ""
	}
	return m[1]
}
func Allowed(a access.Account, brand, action string) bool {
	scope := access.ScopeBrand
	if brand == "" {
		scope = access.ScopePlatform
	}
	return a.SuperAdmin == (brand == "") && access.Authorize(a, "announcement", action, scope, brand)
}
func ValidLink(s string) bool {
	if strings.ContainsAny(s, "\\\r\n\t") || strings.TrimSpace(s) != s {
		return false
	}
	u, e := url.Parse(s)
	if e != nil {
		return false
	}
	if strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "//") && u.Host == "" && u.Scheme == "" {
		return true
	}
	return u.Scheme == "https" && u.Hostname() != "" && u.User == nil
}

// Validate and normalize the same limited markup rendered by editor previews and clients.
// Unsupported elements/attributes are errors, never silently stripped.
func NormalizeHTML(source string) (string, []string, error) {
	if len(source) > 60000 {
		return "", nil, ErrInvalid
	}
	nodes, e := html.ParseFragment(strings.NewReader(source), &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div})
	if e != nil {
		return "", nil, ErrInvalid
	}
	assets := []string{}
	visible := false
	var walk func(*html.Node) error
	walk = func(n *html.Node) error {
		if n.Type == html.TextNode {
			if strings.TrimSpace(n.Data) != "" {
				visible = true
			}
			return nil
		}
		if n.Type != html.ElementNode || n.Namespace != "" {
			return ErrInvalid
		}
		switch n.Data {
		case "p", "br", "h2", "h3", "strong", "em", "u", "s", "ul", "ol", "li", "blockquote", "a", "img", "mark":
		default:
			return ErrInvalid
		}
		for _, a := range n.Attr {
			if a.Namespace != "" {
				return ErrInvalid
			}
			switch {
			case n.Data == "a" && a.Key == "rel" && a.Val == "noopener noreferrer":
			case n.Data == "a" && a.Key == "href":
				if !ValidLink(a.Val) {
					return ErrInvalid
				}
			case n.Data == "img" && a.Key == "src":
				if AssetID(a.Val) == "" {
					return ErrInvalid
				}
				assets = append(assets, AssetID(a.Val))
				visible = true
			case n.Data == "img" && a.Key == "alt":
				if utf8.RuneCountInString(a.Val) > 240 {
					return ErrInvalid
				}
			default:
				return ErrInvalid
			}
		}
		if n.Data == "img" {
			hasSrc := false
			for _, a := range n.Attr {
				hasSrc = hasSrc || a.Key == "src"
			}
			if !hasSrc {
				return ErrInvalid
			}
		}
		if n.Data == "a" {
			attrs := n.Attr[:0]
			for _, a := range n.Attr {
				if a.Key != "rel" {
					attrs = append(attrs, a)
				}
			}
			n.Attr = append(attrs, html.Attribute{Key: "rel", Val: "noopener noreferrer"})
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if e := walk(c); e != nil {
				return e
			}
		}
		return nil
	}
	var out strings.Builder
	for _, n := range nodes {
		if e := walk(n); e != nil {
			return "", nil, e
		}
		if e := html.Render(&out, n); e != nil {
			return "", nil, e
		}
	}
	if !visible {
		return "", nil, ErrInvalid
	}
	return out.String(), assets, nil
}
func Validate(in *Input, brand string) ([]string, error) {
	if in.Version < 0 || in.Version >= 9007199254740991 || (in.Status != "draft" && in.Status != "published" && in.Status != "withdrawn") {
		return nil, ErrInvalid
	}
	c := &in.Config
	if c.Targets == nil {
		return nil, ErrInvalid
	}
	if brand != "" && len(c.Targets) > 0 {
		return nil, ErrInvalid
	}
	seen := map[string]bool{}
	for _, id := range c.Targets {
		if !ValidID(id) || seen[id] {
			return nil, ErrInvalid
		}
		seen[id] = true
	}
	if c.Audience != "all" && c.Audience != "guest" && c.Audience != "member" {
		return nil, ErrInvalid
	}
	if c.Frequency != "session" && c.Frequency != "daily" && c.Frequency != "interval" && c.Frequency != "once" {
		return nil, ErrInvalid
	}
	if c.IntervalDays < 1 || c.IntervalDays > 365 || c.Sort < 0 || c.Sort > 9999 {
		return nil, ErrInvalid
	}
	if c.StartsAt != nil && c.EndsAt != nil && !c.EndsAt.After(*c.StartsAt) {
		return nil, ErrInvalid
	}
	assets := []string{}
	for _, v := range []*Copy{&c.Content.En, &c.Content.Zh} {
		if (in.Status == "published" && strings.TrimSpace(v.Title) == "") || utf8.RuneCountInString(v.Title) > 120 || utf8.RuneCountInString(v.Button) > 40 {
			return nil, ErrInvalid
		}
		if (v.Button == "") != (v.URL == "") || v.URL != "" && !ValidLink(v.URL) {
			return nil, ErrInvalid
		}
		if v.HTML == "" && in.Status != "published" {
			continue
		}
		normalized, ids, e := NormalizeHTML(v.HTML)
		if e != nil {
			return nil, fmt.Errorf("%w: markup", ErrInvalid)
		}
		v.HTML = normalized
		assets = append(assets, ids...)
	}
	return assets, nil
}
func State(item Item, now time.Time) string {
	if item.Status != "published" {
		return item.Status
	}
	if item.Config.StartsAt != nil && now.Before(*item.Config.StartsAt) {
		return "scheduled"
	}
	if item.Config.EndsAt != nil && !now.Before(*item.Config.EndsAt) {
		return "ended"
	}
	return "active"
}
func Day(now time.Time, loc *time.Location) string {
	return now.In(loc).Format("2006-01-02")
}
