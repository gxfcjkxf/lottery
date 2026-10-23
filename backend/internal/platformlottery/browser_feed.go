package platformlottery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var ErrBrowserUnavailable = errors.New("draw browser runtime is not configured")
var ErrBrowserRender = errors.New("draw browser rendering failed")

type BrowserDiagnostic struct {
	Document *struct {
		Origin         string `json:"origin"`
		Path           string `json:"path"`
		Title          string `json:"title"`
		PasswordFields int    `json:"password_fields"`
	} `json:"document,omitempty"`
	Code    string  `json:"code"`
	Stage   string  `json:"stage"`
	Status  *int    `json:"status"`
	Network *string `json:"network"`
	Blocked []struct {
		Reason string `json:"reason"`
		Method string `json:"method"`
		Type   string `json:"type"`
		Origin string `json:"origin"`
		Path   string `json:"path"`
	} `json:"blocked"`
}
type BrowserRenderError struct{ Diagnostic BrowserDiagnostic }

func (e *BrowserRenderError) Error() string {
	raw, _ := json.Marshal(e.Diagnostic)
	return "draw browser rendering failed: " + string(raw)
}
func (e *BrowserRenderError) Unwrap() error { return ErrBrowserRender }

type FeedJSONRequest struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Method string `json:"method"`
	Body   string `json:"body"`
}
type RenderedFeed struct {
	HTML      string                     `json:"html"`
	Responses map[string]json.RawMessage `json:"responses"`
}
type FeedBrowser struct {
	EntryURL        string            `json:"entry_url,omitempty"`
	MaxRedirects    int               `json:"max_redirects,omitempty"`
	IgnoredRequests []string          `json:"ignored_requests,omitempty"`
	JSONRequests    []FeedJSONRequest `json:"json_requests,omitempty"`
	WaitSelector    string            `json:"wait_selector"`
	TimeoutMS       int               `json:"timeout_ms"`
}

func (b FeedBrowser) Validate() error {
	if b.MaxRedirects < 0 || b.MaxRedirects > 5 || len(b.IgnoredRequests) > 10 || len(b.JSONRequests) > 5 {
		return ErrInvalid
	}
	validURL := func(v string) bool {
		u, e := url.Parse(v)
		return e == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil && u.Fragment == "" && len(v) <= 2048
	}
	if b.EntryURL != "" && !validURL(b.EntryURL) {
		return ErrInvalid
	}
	for _, v := range b.IgnoredRequests {
		u, e := url.Parse(v)
		if !validURL(v) || e != nil || u.RawQuery != "" {
			return ErrInvalid
		}
	}
	seen := map[string]bool{}
	for _, q := range b.JSONRequests {
		if q.ID == "" || len(q.ID) > 40 || strings.ContainsAny(q.ID, " /\t\n") || seen[q.ID] || !validURL(q.URL) || (q.Method != "GET" && q.Method != "POST") || len(q.Body) > 4096 || q.Method == "GET" && q.Body != "" {
			return ErrInvalid
		}
		seen[q.ID] = true
	}
	return nil
}
func renderFeed(ctx context.Context, s AutomationSources, p AutomationPlan, endpoint, bearer string) (*RenderedFeed, error) {
	node, script, binary := os.Getenv("PLATFORM_DRAW_BROWSER_NODE"), os.Getenv("PLATFORM_DRAW_BROWSER_SCRIPT"), os.Getenv("PLATFORM_DRAW_BROWSER_EXECUTABLE")
	for _, path := range []string{node, script, binary} {
		if !filepath.IsAbs(path) {
			return nil, ErrBrowserUnavailable
		}
		if _, e := os.Stat(path); e != nil {
			return nil, ErrBrowserUnavailable
		}
	}
	if p.Mapping == nil || p.Mapping.Browser == nil || !s.Allows(endpoint) {
		return nil, ErrInvalid
	}
	config := p.Mapping.Browser
	origin, e := url.Parse(endpoint)
	if e != nil {
		return nil, ErrInvalid
	}
	for _, target := range append([]string{config.EntryURL}, func() []string {
		out := []string{}
		for _, q := range config.JSONRequests {
			out = append(out, q.URL)
		}
		return out
	}()...) {
		if target == "" {
			continue
		}
		u, e := url.Parse(target)
		if e != nil || !s.Allows(target) || u.Scheme != origin.Scheme || u.Host != origin.Host {
			return nil, ErrInvalid
		}
	}
	input, e := json.Marshal(struct {
		URL          string       `json:"url"`
		WaitSelector string       `json:"wait_selector"`
		TimeoutMS    int          `json:"timeout_ms"`
		Origins      []string     `json:"allowed_origins"`
		Bearer       string       `json:"bearer"`
		Browser      *FeedBrowser `json:"browser"`
	}{endpoint, config.WaitSelector, config.TimeoutMS, s.Origins, bearer, config})
	if e != nil {
		return nil, e
	}
	run, cancel := context.WithTimeout(ctx, time.Duration(config.TimeoutMS)*time.Millisecond+3*time.Second)
	defer cancel()
	command := exec.CommandContext(run, node, script)
	command.Stdin = bytes.NewReader(input)
	command.WaitDelay = 3 * time.Second
	command.Cancel = func() error { return command.Process.Signal(syscall.SIGTERM) }
	output, e := command.Output()
	if e != nil {
		var exit *exec.ExitError
		if errors.As(e, &exit) {
			var diagnostic BrowserDiagnostic
			if json.Unmarshal(exit.Stderr, &diagnostic) == nil && diagnostic.Code != "" {
				return nil, &BrowserRenderError{Diagnostic: diagnostic}
			}
		}
		return nil, fmt.Errorf("%w: process did not complete", ErrBrowserRender)
	}
	var result RenderedFeed
	if json.Unmarshal(output, &result) != nil || len(result.HTML) == 0 || len(result.HTML) > 1048576 {
		return nil, ErrBrowserRender
	}
	return &result, nil
}
