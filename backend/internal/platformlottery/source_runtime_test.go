package platformlottery

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestFeedFailuresHaveDistinctSafeReasons(t *testing.T) {
	requests := 0
	status, body := 200, `not-json`
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; w.WriteHeader(status); w.Write([]byte(body)) }))
	defer feed.Close()
	m := testFeedMapping("json")
	p := AutomationPlan{SourceType: "mapped", SourceURL: feed.URL, Mapping: &m}
	sources := AutomationSources{Client: feed.Client()}
	_, err := FetchMappedFeed(context.Background(), sources, p, "1", "test")
	var failure *FeedError
	if !errors.As(err, &failure) || failure.Reason != "source_denied" || requests != 0 || feedFailureCode(err) != "source_error" {
		t.Fatal("denied source fetched or mislabeled", err, requests)
	}
	sources.Origins = []string{feed.URL}
	p.CredentialRef = "DRAW_FEED_RUNTIME_TEST"
	t.Setenv(p.CredentialRef, "")
	_, err = FetchMappedFeed(context.Background(), sources, p, "1", "test")
	if feedFailureCode(err) != "credential_missing" || requests != 0 {
		t.Fatal(err, requests)
	}
	p.CredentialRef = ""
	_, err = FetchMappedFeed(context.Background(), sources, p, "1", "test")
	if feedFailureCode(err) != "invalid_result" {
		t.Fatal("bad body mislabeled", err)
	}
	status = 503
	_, err = FetchMappedFeed(context.Background(), sources, p, "1", "test")
	if !errors.As(err, &failure) || failure.Reason != "http_status_503" || feedFailureCode(err) != "source_error" {
		t.Fatal("HTTP failure mislabeled", err)
	}
	feed.Close()
	_, err = FetchMappedFeed(context.Background(), sources, p, "1", "test")
	if !errors.As(err, &failure) || failure.Reason != "network_error" || strings.Contains(err.Error(), feed.URL) {
		t.Fatal("network failure unsafe or mislabeled", err)
	}
}

func TestDrawSourceRuntimeChecksAndProbeAreReadOnly(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	s := Service{DB: db}
	if err := s.CheckDrawSources(ctx, AutomationSources{}, false); err != nil {
		t.Fatal("empty installation must start", err)
	}
	a := access.Account{ID: ids.New(), Type: access.AccountAdmin, SuperAdmin: true, Roles: []access.Role{{Permissions: []access.Permission{{Resource: "platform_game", Action: "write", Scope: access.ScopePlatform}}}}}
	if _, err := db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,'source_runtime','test-only')`, a.ID); err != nil {
		t.Fatal(err)
	}
	requests := 0
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		json.NewEncoder(w).Encode(map[string]any{"rows": []any{map[string]any{"issue": "round-1", "time": time.Now().Format(time.RFC3339Nano), "balls": "1,2,3,4,5,6,49"}}})
	}))
	defer feed.Close()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	raw, model := configuredModelFixture(t)
	g, err := s.Create(ctx, tx, a, GameInput{Code: "runtime_check", Name: "Runtime check", Timezone: "UTC", Model: model}, points.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveConfiguredRules(ctx, tx, a, g.ID, ConfiguredRulesInput{Snapshot: raw}, points.Metadata{}); err != nil {
		t.Fatal(err)
	}
	m := testFeedMapping("json")
	m.Regular.Count = 6
	m.Special.Offset = 6
	anchor, interval, first := time.Now().Add(-time.Hour), 120, "1"
	plan, err := publishTestAutomation(ctx, s, tx, a, g.ID, AutomationInput{AutomationPlan: AutomationPlan{Enabled: true, SourceType: "mapped", SourceURL: feed.URL, Mapping: &m, AnchorDrawAt: &anchor, AnchorPeriodNo: &first, IntervalSeconds: &interval, OpenBeforeSeconds: &interval}}, points.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CreatePeriod(ctx, tx, a, g.ID, PeriodInput{PeriodNo: "round-1", BetStartAt: anchor, BetEndAt: anchor.Add(time.Minute), DrawAt: anchor.Add(2 * time.Minute), AutomationVersion: &plan.Version}, points.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.CheckDrawSources(ctx, AutomationSources{}, false); err == nil || !strings.Contains(err.Error(), "PLATFORM_DRAW_ALLOWED_ORIGINS") || requests != 0 {
		t.Fatal("missing whitelist accepted", err, requests)
	}
	sources := AutomationSources{Origins: []string{feed.URL}, Client: feed.Client()}
	if err = s.CheckDrawSources(ctx, sources, true); err != nil || requests != 1 {
		t.Fatal("read probe failed", err, requests)
	}
	var published, attempts int
	if err = db.QueryRow(ctx, `SELECT (SELECT count(*) FROM platform_game_periods WHERE result IS NOT NULL),(SELECT count(*) FROM platform_automation_attempts)`).Scan(&published, &attempts); err != nil || published != 0 || attempts != 0 {
		t.Fatal("probe wrote business data", published, attempts, err)
	}
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	plan.Enabled = false
	if _, err = publishTestAutomation(ctx, s, tx, a, g.ID, AutomationInput{AutomationPlan: plan}, points.Metadata{}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.CheckDrawSources(ctx, AutomationSources{}, false); err == nil {
		t.Fatal("unfinished frozen source version escaped whitelist validation")
	}
	if _, err = s.CollectAutomaticDraws(ctx, AutomationSources{}); err != nil {
		t.Fatal(err)
	}
	var code string
	if err = db.QueryRow(ctx, `SELECT code FROM platform_automation_attempts`).Scan(&code); err != nil || code != "source_error" {
		t.Fatal("denied collection mislabeled", code, err)
	}
}
