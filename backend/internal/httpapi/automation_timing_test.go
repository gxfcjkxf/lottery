package httpapi

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestAutomationTimingHTTPReplayPermissionsAndFrozenPeriods(t *testing.T) {
	nextDraw := time.Now().UTC().Add(20 * time.Minute).Truncate(time.Second)
	feed := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"period_no": "3500000", "drawn_at": time.Now().Add(-time.Minute).Format(time.RFC3339), "digits": []int{4, 3, 5}, "next_no": "3500002", "next_at": nextDraw.Format(time.RFC3339)})
	}))
	defer feed.Close()
	origins, _ := json.Marshal([]string{feed.URL})
	t.Setenv("PLATFORM_DRAW_ALLOWED_ORIGINS", string(origins))
	f := managedFixture(t)
	ctx := context.Background()
	operator := ids.New()
	if _, e := f.pool.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash,is_super_admin) SELECT $1,'timing_operator',password_hash,true FROM admin_accounts WHERE id=$2`, operator, f.root); e != nil {
		t.Fatal(e)
	}
	pf := f
	pf.root = operator
	grantPlatformPermission(t, pf, "platform_game.view.platform", "platform_game.write.platform")
	login := f.call("POST", "/api/v1/platform/auth/login", "timing-login", "", "", map[string]string{"identifier": "timing_operator", "password": "root-test-password-2026"})
	mustStatus(t, login, 200)
	var auth identity.AdminAuthentication
	managedData(t, login, &auth)
	actor := access.Account{ID: operator, Type: access.AccountAdmin, SuperAdmin: true, Roles: []access.Role{{Permissions: []access.Permission{{Resource: "platform_game", Action: "write", Scope: access.ScopePlatform}}}}}
	catalog, e := rules.ConfiguredTemplateCatalog("canada_28")
	if e != nil {
		t.Fatal(e)
	}
	book := map[string]string{}
	for _, play := range catalog.Plays {
		for _, outcome := range play.Decisions.Outcomes {
			if outcome.PriceKey != "" {
				book[outcome.PriceKey] = "2.5"
			}
		}
	}
	engine, e := rules.NewConfiguredEngine(catalog, rules.ConfiguredBindings{Version: "timing-test", Attributes: map[string]map[string][]int{}}, rules.ConfiguredPrices{Version: "timing-test", Plates: map[string]map[string]string{"A": book}})
	if e != nil {
		t.Fatal(e)
	}
	s := platformlottery.Service{DB: f.pool}
	tx, e := f.pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	game, e := s.Create(ctx, tx, actor, platformlottery.GameInput{Code: "canada_timing", Name: "Canada28 timing test", Model: catalog.Model, Timezone: "America/Toronto"}, points.Metadata{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.SaveConfiguredRules(ctx, tx, actor, game.ID, platformlottery.ConfiguredRulesInput{Snapshot: engine.Snapshot()}, points.Metadata{}); e != nil {
		t.Fatal(e)
	}
	open := 210
	mapping := &platformlottery.FeedMapping{Format: "json", Schedule: "source", TimeFormat: "rfc3339", Period: platformlottery.FeedField{Path: "/period_no"}, DrawTime: platformlottery.FeedField{Path: "/drawn_at"}, Digits: &platformlottery.FeedNumbers{FeedField: platformlottery.FeedField{Path: "/digits"}, Count: 3}, NextPeriod: &platformlottery.FeedField{Path: "/next_no"}, NextDrawTime: &platformlottery.FeedField{Path: "/next_at"}}
	source, e := s.SaveDrawSource(ctx, tx, actor, game.ID, platformlottery.DrawSourceInput{DrawSource: platformlottery.DrawSource{Name: "Timing feed", Plan: platformlottery.AutomationPlan{SourceType: "mapped", Enabled: true, Mapping: mapping, SourceURL: feed.URL + "/latest", OpenBeforeSeconds: &open, CloseBeforeSeconds: 15}}}, points.Metadata{})
	if e != nil {
		t.Fatal(e)
	}
	original, e := s.ActivateDrawSource(ctx, tx, actor, game.ID, source.ID, platformlottery.DrawSourceActivation{Revision: source.Revision, Version: 0}, points.Metadata{})
	if e != nil {
		t.Fatal(e)
	}
	draw := time.Now().UTC().Add(10 * time.Minute)
	version := original.Version
	period, e := s.CreatePeriod(ctx, tx, actor, game.ID, platformlottery.PeriodInput{PeriodNo: "3500001", AutomationVersion: &version, BetStartAt: draw.Add(-210 * time.Second), BetEndAt: draw.Add(-15 * time.Second), DrawAt: draw}, points.Metadata{})
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	path := "/api/v1/platform/platform-games/" + game.ID + "/automation-timing"
	input := platformlottery.AutomationTimingInput{Version: 1, OpenBeforeSeconds: 180, CloseBeforeSeconds: 20}
	response := f.call("POST", path, "timing-save", auth.AccessToken, "", input)
	mustStatus(t, response, 200)
	var saved platformlottery.AutomationPlan
	managedData(t, response, &saved)
	if saved.Version != 2 || *saved.OpenBeforeSeconds != 180 || saved.CloseBeforeSeconds != 20 || saved.SourceURL != original.SourceURL || !reflect.DeepEqual(saved.Mapping, original.Mapping) {
		t.Fatal("timing changed source or was not applied", saved)
	}
	replay := f.call("POST", path, "timing-save", auth.AccessToken, "", input)
	mustStatus(t, replay, 200)
	var same platformlottery.AutomationPlan
	managedData(t, replay, &same)
	if !reflect.DeepEqual(saved, same) {
		t.Fatal("replay changed version")
	}
	mustStatus(t, f.call("POST", path, "timing-stale", auth.AccessToken, "", input), 409)
	mustStatus(t, f.call("POST", path, "timing-brand", f.token, "", input), 403)
	mustStatus(t, f.call("POST", strings.Replace(path, "/platform/", "/admin/", 1), "timing-brand-route", f.token, managedBrand, input), 404)
	mustStatus(t, f.call("POST", path, "timing-anonymous", "", "", input), 401)
	input.Version = 2
	input.CloseBeforeSeconds = 180
	mustStatus(t, f.call("POST", path, "timing-invalid", auth.AccessToken, "", input), 400)
	var frozen int64
	var start, end time.Time
	if e = f.pool.QueryRow(ctx, `SELECT automation_version,bet_start_at,bet_end_at FROM platform_game_periods WHERE id=$1`, period.ID).Scan(&frozen, &start, &end); e != nil || frozen != 1 || !start.Equal(period.BetStartAt) || !end.Equal(period.BetEndAt) {
		t.Fatal("reserved period changed", frozen, start, end, e)
	}
	if _, e = s.CollectAutomaticDraws(ctx, platformlottery.AutomationSources{Origins: []string{feed.URL}, Client: feed.Client()}); e != nil {
		t.Fatal(e)
	}
	if e = f.pool.QueryRow(ctx, `SELECT automation_version,bet_start_at,bet_end_at FROM platform_game_periods WHERE game_id=$1 AND period_no='3500002'`, game.ID).Scan(&frozen, &start, &end); e != nil || frozen != 2 || !start.Equal(nextDraw.Add(-180*time.Second)) || !end.Equal(nextDraw.Add(-20*time.Second)) {
		t.Fatal("new period ignored new timing", frozen, start, end, e)
	}
	// Selecting a stored source must not restore its outdated opening/closing values.
	tx, e = f.pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	selected, e := s.ActivateDrawSource(ctx, tx, actor, game.ID, source.ID, platformlottery.DrawSourceActivation{Revision: source.Revision, Version: 2}, points.Metadata{})
	if e != nil || selected.OpenBeforeSeconds == nil || *selected.OpenBeforeSeconds != 180 || selected.CloseBeforeSeconds != 20 {
		t.Fatal("source selection reset timing", selected, e)
	}
}
