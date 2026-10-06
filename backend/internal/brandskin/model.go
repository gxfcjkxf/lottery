// Package brandskin provides bounded presentation configuration, not executable
// CSS/HTML or financial policy. Asset URIs are never fetched by the server.
package brandskin

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"path"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalid      = errors.New("invalid brand presentation")
	ErrDenied       = errors.New("brand presentation access denied")
	ErrNotFound     = errors.New("brand presentation not found")
	ErrVersion      = errors.New("brand presentation version conflict")
	ErrState        = errors.New("brand presentation state conflict")
	uuidPattern     = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	colorPattern    = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	hostPattern     = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+$`)
	numericHostTail = regexp.MustCompile(`^(?:[0-9]+|0x[0-9a-f]+)$`)
	configKeys      = []string{"display_name", "logo_text", "logo_url", "favicon_url", "primary_color", "accent_color", "success_color", "warning_color", "danger_color", "font_family", "font_scale", "radius", "shadow", "default_locale", "available_locales", "content"}
)

type CopyOverrides struct {
	Tagline      *string `json:"tagline"`
	Announcement *string `json:"announcement"`
}
type ContentOverrides struct {
	English CopyOverrides `json:"en"`
	Chinese CopyOverrides `json:"zh-CN"`
}
type Config struct {
	DisplayName      *string           `json:"display_name"`
	LogoText         *string           `json:"logo_text"`
	LogoURL          *string           `json:"logo_url"`
	FaviconURL       *string           `json:"favicon_url"`
	PrimaryColor     *string           `json:"primary_color"`
	AccentColor      *string           `json:"accent_color"`
	SuccessColor     *string           `json:"success_color"`
	WarningColor     *string           `json:"warning_color"`
	DangerColor      *string           `json:"danger_color"`
	FontFamily       *string           `json:"font_family"`
	FontScale        *string           `json:"font_scale"`
	Radius           *string           `json:"radius"`
	Shadow           *string           `json:"shadow"`
	DefaultLocale    *string           `json:"default_locale"`
	AvailableLocales []string          `json:"available_locales"`
	Content          *ContentOverrides `json:"content"`
}
type Copy struct {
	Tagline      string `json:"tagline"`
	Announcement string `json:"announcement"`
}
type Content struct {
	English Copy `json:"en"`
	Chinese Copy `json:"zh-CN"`
}
type Effective struct {
	DisplayName      string   `json:"display_name"`
	LogoText         string   `json:"logo_text"`
	LogoURL          *string  `json:"logo_url"`
	FaviconURL       *string  `json:"favicon_url"`
	PrimaryColor     string   `json:"primary_color"`
	AccentColor      string   `json:"accent_color"`
	SuccessColor     string   `json:"success_color"`
	WarningColor     string   `json:"warning_color"`
	DangerColor      string   `json:"danger_color"`
	FontFamily       string   `json:"font_family"`
	FontScale        string   `json:"font_scale"`
	Radius           string   `json:"radius"`
	Shadow           string   `json:"shadow"`
	DefaultLocale    string   `json:"default_locale"`
	AvailableLocales []string `json:"available_locales"`
	Content          Content  `json:"content"`
}
type Input struct {
	Version int64  `json:"version"`
	Config  Config `json:"config"`
	Reason  string `json:"reason"`
}

func closed(data []byte, keys []string, nullable map[string]bool) error {
	if !utf8.Valid(data) {
		return ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(data))
	token, e := d.Token()
	if e != nil || token != json.Delim('{') {
		return ErrInvalid
	}
	allowed := map[string]bool{}
	seen := map[string]bool{}
	for _, key := range keys {
		allowed[key] = true
	}
	for d.More() {
		token, e = d.Token()
		key, ok := token.(string)
		if e != nil || !ok || !allowed[key] || seen[key] {
			return ErrInvalid
		}
		seen[key] = true
		var raw json.RawMessage
		if d.Decode(&raw) != nil || (!nullable[key] && bytes.Equal(bytes.TrimSpace(raw), []byte("null"))) {
			return ErrInvalid
		}
	}
	if token, e = d.Token(); e != nil || token != json.Delim('}') || len(seen) != len(keys) {
		return ErrInvalid
	}
	var tail any
	if d.Decode(&tail) != io.EOF {
		return ErrInvalid
	}
	return nil
}
func (c *CopyOverrides) UnmarshalJSON(data []byte) error {
	if c == nil || closed(data, []string{"tagline", "announcement"}, map[string]bool{"tagline": true, "announcement": true}) != nil {
		return ErrInvalid
	}
	type plain CopyOverrides
	var out plain
	if json.Unmarshal(data, &out) != nil {
		return ErrInvalid
	}
	*c = CopyOverrides(out)
	return nil
}
func (c *ContentOverrides) UnmarshalJSON(data []byte) error {
	if c == nil || closed(data, []string{"en", "zh-CN"}, nil) != nil {
		return ErrInvalid
	}
	type plain ContentOverrides
	var out plain
	if json.Unmarshal(data, &out) != nil {
		return ErrInvalid
	}
	*c = ContentOverrides(out)
	return nil
}
func (c *Config) UnmarshalJSON(data []byte) error {
	allowNull := map[string]bool{}
	for _, key := range configKeys {
		allowNull[key] = true
	}
	if c == nil || closed(data, configKeys, allowNull) != nil {
		return ErrInvalid
	}
	type plain Config
	var out plain
	if json.Unmarshal(data, &out) != nil {
		return ErrInvalid
	}
	*c = Config(out)
	return nil
}
func (in *Input) UnmarshalJSON(data []byte) error {
	if in == nil || closed(data, []string{"version", "config", "reason"}, nil) != nil {
		return ErrInvalid
	}
	type plain Input
	var out plain
	if json.Unmarshal(data, &out) != nil {
		return ErrInvalid
	}
	next := Input(out)
	if next.Version < 1 || !validText(next.Reason, 500, false) || strings.TrimSpace(next.Reason) == "" {
		return ErrInvalid
	}
	if _, e := Resolve("Brand", next.Config); e != nil {
		return e
	}
	*in = next
	return nil
}

func validText(s string, max int, oneLine bool) bool {
	if !utf8.ValidString(s) || len(s) > max {
		return false
	}
	if oneLine {
		for _, r := range s {
			if unicode.IsControl(r) {
				return false
			}
		}
	}
	return true
}
func enum(v string, options ...string) bool {
	for _, option := range options {
		if v == option {
			return true
		}
	}
	return false
}
func ValidAssetURI(value string) bool {
	if value == "" || len(value) > 512 || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\\%\r\n\t") || strings.Contains(value, "..") {
		return false
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	u, e := url.Parse(value)
	if e != nil || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return false
	}
	if !enum(strings.ToLower(path.Ext(u.Path)), ".png", ".jpg", ".jpeg", ".webp", ".ico") {
		return false
	}
	if u.Scheme == "" && u.Host == "" {
		return strings.HasPrefix(value, "/icons/") || strings.HasPrefix(value, "/brand-assets/")
	}
	host := strings.ToLower(u.Hostname())
	parts := strings.Split(host, ".")
	if len(parts) == 0 || numericHostTail.MatchString(parts[len(parts)-1]) {
		return false
	}
	if !strings.HasPrefix(value, "https://") || u.Scheme != "https" || u.Port() != "" || net.ParseIP(host) != nil || !hostPattern.MatchString(host) {
		return false
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return false
	}
	return true
}
func Resolve(baseName string, c Config) (Effective, error) {
	e := Effective{DisplayName: baseName, PrimaryColor: "#20594c", AccentColor: "#d9ef9b", SuccessColor: "#287b59", WarningColor: "#a96d20", DangerColor: "#bc4b43", FontFamily: "system", FontScale: "standard", Radius: "round", Shadow: "subtle", DefaultLocale: "en", AvailableLocales: []string{"en", "zh-CN"}, Content: Content{English: Copy{Tagline: "Lottery platform"}, Chinese: Copy{Tagline: "彩票平台"}}}
	if c.DisplayName != nil {
		if !validText(*c.DisplayName, 80, true) || strings.TrimSpace(*c.DisplayName) == "" {
			return e, ErrInvalid
		}
		e.DisplayName = *c.DisplayName
	}
	if c.LogoText != nil {
		if !validText(*c.LogoText, 32, true) || strings.TrimSpace(*c.LogoText) == "" {
			return e, ErrInvalid
		}
		e.LogoText = *c.LogoText
	} else {
		r := []rune(e.DisplayName)
		e.LogoText = "L"
		if len(r) > 0 {
			e.LogoText = string(r[0])
		}
	}
	for _, item := range []struct {
		value *string
		dest  **string
	}{{c.LogoURL, &e.LogoURL}, {c.FaviconURL, &e.FaviconURL}} {
		if item.value != nil {
			if !ValidAssetURI(*item.value) {
				return e, ErrInvalid
			}
			v := *item.value
			*item.dest = &v
		}
	}
	for _, item := range []struct {
		value *string
		dest  *string
	}{{c.PrimaryColor, &e.PrimaryColor}, {c.AccentColor, &e.AccentColor}, {c.SuccessColor, &e.SuccessColor}, {c.WarningColor, &e.WarningColor}, {c.DangerColor, &e.DangerColor}} {
		if item.value != nil {
			if !colorPattern.MatchString(*item.value) {
				return e, ErrInvalid
			}
			*item.dest = *item.value
		}
	}
	for _, item := range []struct {
		value   *string
		dest    *string
		options []string
	}{{c.FontFamily, &e.FontFamily, []string{"system", "serif", "mono"}}, {c.FontScale, &e.FontScale, []string{"compact", "standard", "large"}}, {c.Radius, &e.Radius, []string{"square", "soft", "round"}}, {c.Shadow, &e.Shadow, []string{"none", "subtle", "lifted"}}, {c.DefaultLocale, &e.DefaultLocale, []string{"en", "zh-CN"}}} {
		if item.value != nil {
			if !enum(*item.value, item.options...) {
				return e, ErrInvalid
			}
			*item.dest = *item.value
		}
	}
	if c.AvailableLocales != nil {
		if len(c.AvailableLocales) < 1 || len(c.AvailableLocales) > 2 {
			return e, ErrInvalid
		}
		seen := map[string]bool{}
		e.AvailableLocales = append([]string{}, c.AvailableLocales...)
		for _, locale := range c.AvailableLocales {
			if !enum(locale, "en", "zh-CN") || seen[locale] {
				return e, ErrInvalid
			}
			seen[locale] = true
		}
	}
	found := false
	for _, locale := range e.AvailableLocales {
		if locale == e.DefaultLocale {
			found = true
		}
	}
	if !found {
		return e, ErrInvalid
	}
	if c.Content != nil {
		for _, item := range []struct {
			source CopyOverrides
			dest   *Copy
		}{{c.Content.English, &e.Content.English}, {c.Content.Chinese, &e.Content.Chinese}} {
			if item.source.Tagline != nil {
				if !validText(*item.source.Tagline, 160, false) {
					return e, ErrInvalid
				}
				item.dest.Tagline = *item.source.Tagline
			}
			if item.source.Announcement != nil {
				if !validText(*item.source.Announcement, 2000, false) {
					return e, ErrInvalid
				}
				item.dest.Announcement = *item.source.Announcement
			}
		}
	}
	return e, nil
}
