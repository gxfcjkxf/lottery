package httpapi

import (
	"context"
	"fmt"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"strings"
	"testing"
	"time"
)

func TestPlatformLotteryHTTPPermissionsReplayAndSharedRead(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	var brandRole string
	if e := f.pool.QueryRow(ctx, `SELECT role_id::text FROM admin_account_roles WHERE account_id=$1`, f.root).Scan(&brandRole); e != nil {
		t.Fatal(e)
	}
	for _, permission := range []string{"platform_game.view.brand", "platform_game.connect.brand", "platform_game.sell.brand"} {
		if _, e := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2)`, brandRole, permission); e != nil {
			t.Fatal(e)
		}
	}
	platformID := ids.New()
	if _, e := f.pool.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash,is_super_admin) SELECT $1,'platform_lottery_operator',password_hash,true FROM admin_accounts WHERE id=$2`, platformID, f.root); e != nil {
		t.Fatal(e)
	}
	pf := f
	pf.root = platformID
	role := grantPlatformPermission(t, pf, "platform_game.view.platform", "platform_game.write.platform")
	login := f.call("POST", "/api/v1/platform/auth/login", "platform-lottery-login", "", "", map[string]string{"identifier": "platform_lottery_operator", "password": "root-test-password-2026"})
	mustStatus(t, login, 200)
	var auth identity.AdminAuthentication
	managedData(t, login, &auth)
	calendarPath := "/api/v1/platform/platform-games/official-calendar"
	mustStatus(t, f.call("GET", calendarPath, "", auth.AccessToken, "", nil), 200)
	mustStatus(t, f.call("GET", calendarPath+"?year=2026", "", auth.AccessToken, "", nil), 400)
	mustStatus(t, f.call("GET", calendarPath, "", "", "", nil), 401)
	mustStatus(t, f.call("GET", calendarPath, "", f.token, "", nil), 403)
	mustStatus(t, f.call("GET", strings.Replace(calendarPath, "/platform/", "/admin/", 1), "", f.token, managedBrand, nil), 400)
	body := map[string]any{"code": "platform_shared", "name": "Platform shared", "timezone": "UTC", "model": rules.Model{Type: "DIGITS_0_9", Length: 3, Ordered: true}, "reason": "Create shared game"}
	created := f.call("POST", "/api/v1/platform/platform-games", "platform-game-create", auth.AccessToken, "", body)
	mustStatus(t, created, 201)
	var game platformlottery.Game
	managedData(t, created, &game)
	overviewPath := "/api/v1/platform/platform-games/" + game.ID + "/draw-overview"
	mustStatus(t, f.call("GET", overviewPath, "", auth.AccessToken, "", nil), 200)
	mustStatus(t, f.call("GET", overviewPath+"?year=bad", "", auth.AccessToken, "", nil), 400)
	mustStatus(t, f.call("GET", overviewPath, "", f.token, "", nil), 403)
	mustStatus(t, f.call("GET", strings.Replace(overviewPath, "/platform/", "/admin/", 1), "", f.token, managedBrand, nil), 404)

	replay := f.call("POST", "/api/v1/platform/platform-games", "platform-game-create", auth.AccessToken, "", body)
	mustStatus(t, replay, 201)
	var same platformlottery.Game
	managedData(t, replay, &same)
	if same.ID != game.ID {
		t.Fatal("creation replay duplicated game")
	}
	extraBody := map[string]any{"code": "delete_unused", "name": "Delete unused", "timezone": "UTC", "model": rules.Model{Type: "DIGITS_0_9", Length: 3, AllowRepeat: true, Ordered: true}}
	extraResponse := f.call("POST", "/api/v1/platform/platform-games", "lifecycle-extra", auth.AccessToken, "", extraBody)
	mustStatus(t, extraResponse, 201)
	var extra platformlottery.Game
	managedData(t, extraResponse, &extra)
	path := "/api/v1/platform/platform-games/" + extra.ID + "/lifecycle"
	offline := platformlottery.LifecycleInput{ExpectedVersion: 1, Status: "offline", Reason: "test offline"}
	mustStatus(t, f.call("POST", path, "offline-lifecycle", auth.AccessToken, "", offline), 200)
	mustStatus(t, f.call("POST", path, "offline-lifecycle", auth.AccessToken, "", offline), 200)
	mustStatus(t, f.call("POST", path, "offline-stale", auth.AccessToken, "", offline), 409)
	mustStatus(t, f.call("POST", path, "brand-offline", f.token, "", offline), 403)
	mustStatus(t, f.call("POST", "/api/v1/admin/platform-games/"+extra.ID+"/lifecycle", "wrong-entry", f.token, managedBrand, offline), 404)
	deleted := platformlottery.LifecycleInput{ExpectedVersion: 2, Status: "deleted", Reason: "test delete"}
	mustStatus(t, f.call("POST", path, "delete-unused", auth.AccessToken, "", deleted), 200)
	mustStatus(t, f.call("POST", path, "delete-unused", auth.AccessToken, "", deleted), 200)
	mustStatus(t, f.call("GET", "/api/v1/platform/platform-games/"+extra.ID, "", auth.AccessToken, "", nil), 200) // Historical detail remains readable after soft deletion.
	modelBody := platformlottery.ModelInput{Model: rules.Model{Type: "DIGITS_0_9", Length: 3, AllowRepeat: true, Ordered: true}, ExpectedVersion: 1, Reason: "Visual model edit"}
	updated := f.call("POST", "/api/v1/platform/platform-games/"+game.ID+"/model", "platform-model-update", auth.AccessToken, "", modelBody)
	mustStatus(t, updated, 200)
	var modelGame platformlottery.Game
	managedData(t, updated, &modelGame)
	if modelGame.Version != 2 || !modelGame.Model.AllowRepeat {
		t.Fatal("model update did not persist")
	}
	mustStatus(t, f.call("POST", "/api/v1/platform/platform-games/"+game.ID+"/model", "platform-model-update", auth.AccessToken, "", modelBody), 200)
	mustStatus(t, f.call("POST", "/api/v1/platform/platform-games/"+game.ID+"/model", "platform-model-stale", auth.AccessToken, "", modelBody), 409)
	mustStatus(t, f.call("POST", "/api/v1/admin/platform-games/"+game.ID+"/model", "brand-cannot-update-model", f.token, managedBrand, modelBody), 404)
	mustStatus(t, f.call("POST", "/api/v1/platform/platform-games/"+game.ID+"/model", "brand-wrong-model-surface", f.token, "", modelBody), 403)
	mustStatus(t, f.call("GET", "/api/v1/admin/platform-games/"+game.ID, "", f.token, managedBrand, nil), 404)
	connect := map[string]string{"reason": "Connect brand to shared game"}
	mustStatus(t, f.call("POST", "/api/v1/admin/platform-games/"+game.ID+"/connect", "brand-connect-game", f.token, managedBrand, connect), 201)
	mustStatus(t, f.call("POST", "/api/v1/admin/platform-games/"+game.ID+"/connect", "brand-connect-game", f.token, managedBrand, connect), 201)
	mustStatus(t, f.call("POST", "/api/v1/admin/platform-games/"+game.ID+"/connect", "brand-connect-again", f.token, managedBrand, connect), 409)
	no := false
	yes := true
	availabilityPath := "/api/v1/admin/platform-games/" + game.ID + "/availability"
	availability := platformlottery.AvailabilityInput{ExpectedVersion: 1, Enabled: &no, Reason: "test disable"}
	mustStatus(t, f.call("POST", availabilityPath, "disable-brand", f.token, managedBrand, availability), 200)
	mustStatus(t, f.call("POST", availabilityPath, "disable-brand", f.token, managedBrand, availability), 200)
	mustStatus(t, f.call("POST", availabilityPath, "stale-brand", f.token, managedBrand, availability), 409)
	availability.ExpectedVersion = 2
	availability.Enabled = &yes
	mustStatus(t, f.call("POST", availabilityPath, "enable-brand", f.token, managedBrand, availability), 200)

	mustStatus(t, f.call("POST", "/api/v1/admin/platform-games", "brand-cannot-create", f.token, managedBrand, body), 404)
	mustStatus(t, f.call("POST", "/api/v1/platform/platform-games", "brand-wrong-surface", f.token, "", body), 403)
	catalog, err := rules.ConfiguredTemplateCatalog("canada_28")
	if err != nil {
		t.Fatal(err)
	}
	prices := map[string]string{}
	for _, play := range catalog.Plays {
		for _, outcome := range play.Decisions.Outcomes {
			if outcome.PriceKey != "" {
				prices[outcome.PriceKey] = "2"
			}
		}
	}
	engine, err := rules.NewConfiguredEngine(catalog, rules.ConfiguredBindings{Version: "http-test", Attributes: map[string]map[string][]int{}}, rules.ConfiguredPrices{Version: "http-test", Plates: map[string]map[string]string{"A": prices}})
	if err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("POST", "/api/v1/platform/platform-games/"+game.ID+"/configured/rules", "platform-save-rules", auth.AccessToken, "", platformlottery.ConfiguredRulesInput{Snapshot: engine.Snapshot()}), 200)
	manualPath := "/api/v1/platform/platform-games/" + game.ID + "/results"
	historical := platformlottery.ManualResultInput{PeriodNo: "1999001", DrawDate: "1999-01-02", Result: rules.Draw{Digits: []int{1, 2, 3}}}
	mustStatus(t, f.call("POST", manualPath, "history-brand-denied", f.token, "", historical), 403)
	mustStatus(t, f.call("POST", strings.Replace(manualPath, "/platform/", "/admin/", 1), "history-brand-no-route", f.token, managedBrand, historical), 404)
	firstHistory := f.call("POST", manualPath, "manual-history", auth.AccessToken, "", historical)
	mustStatus(t, firstHistory, 201)
	mustStatus(t, f.call("POST", manualPath, "manual-history", auth.AccessToken, "", historical), 201)
	mustStatus(t, f.call("POST", manualPath, "manual-history-duplicate", auth.AccessToken, "", historical), 409)
	var archived platformlottery.DrawHistoryItem
	managedData(t, firstHistory, &archived)
	if archived.DrawDate == nil || *archived.DrawDate != "1999-01-02" || archived.Source != "manual" {
		t.Fatalf("historical record %#v", archived)
	}
	var historyCount, periodCount int
	if e := f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM platform_draw_history WHERE game_id=$1),(SELECT count(*) FROM platform_game_periods WHERE game_id=$1)`, game.ID).Scan(&historyCount, &periodCount); e != nil {
		t.Fatal(e)
	}
	if historyCount != 1 || periodCount != 0 {
		t.Fatalf("history=%d periods=%d", historyCount, periodCount)
	}
	for i, date := range []string{"1999-02-30", "2999-01-01"} {
		invalid := historical
		invalid.PeriodNo = "bad-date"
		invalid.DrawDate = date
		mustStatus(t, f.call("POST", manualPath, fmt.Sprintf("manual-date-%d", i), auth.AccessToken, "", invalid), 400)
	}
	invalidNumbers := historical
	invalidNumbers.PeriodNo = "bad-numbers"
	invalidNumbers.Result.Digits = []int{1, 1, 11}
	mustStatus(t, f.call("POST", manualPath, "manual-numbers", auth.AccessToken, "", invalidNumbers), 400)
	historyRead := f.call("GET", overviewPath+"?year=1999", "", auth.AccessToken, "", nil)
	mustStatus(t, historyRead, 200)
	var historyOverview platformlottery.DrawOverview
	managedData(t, historyRead, &historyOverview)
	if len(historyOverview.Items) != 1 || historyOverview.Items[0].PeriodNo != historical.PeriodNo {
		t.Fatal("manual history missing")
	}
	now := time.Now().UTC()
	futureBody := platformlottery.PeriodInput{PeriodNo: "opening-edit", BetStartAt: now.Add(time.Hour), BetEndAt: now.Add(3 * time.Hour), DrawAt: now.Add(4 * time.Hour)}
	futureResponse := f.call("POST", "/api/v1/platform/platform-games/"+game.ID+"/periods", "opening-period-create", auth.AccessToken, "", futureBody)
	mustStatus(t, futureResponse, 201)
	var future platformlottery.Period
	managedData(t, futureResponse, &future)
	var platformZone string
	if e := f.pool.QueryRow(ctx, `SELECT timezone FROM platform_settings WHERE id=true`).Scan(&platformZone); e != nil {
		t.Fatal(e)
	}
	loc, e := time.LoadLocation(platformZone)
	if e != nil {
		t.Fatal(e)
	}
	manualFuture := platformlottery.ManualResultInput{PeriodNo: future.PeriodNo, DrawDate: future.DrawAt.In(loc).Format("2006-01-02"), Result: rules.Draw{Digits: []int{1, 2, 3}}}
	mustStatus(t, f.call("POST", manualPath, "manual-future", auth.AccessToken, "", manualFuture), 409)
	openingPath := "/api/v1/platform/platform-games/periods/" + future.ID + "/opening"
	openingBody := platformlottery.PeriodOpeningInput{BetStartAt: now.Add(2 * time.Hour), ExpectedBetStartAt: future.BetStartAt}
	mustStatus(t, f.call("POST", openingPath, "brand-opening-denied", f.token, "", openingBody), 403)
	mustStatus(t, f.call("POST", strings.Replace(openingPath, "/platform/", "/admin/", 1), "brand-opening-no-route", f.token, managedBrand, openingBody), 404)
	updatedOpening := f.call("POST", openingPath, "opening-change", auth.AccessToken, "", openingBody)
	mustStatus(t, updatedOpening, 200)
	var changedOpening platformlottery.Period
	managedData(t, updatedOpening, &changedOpening)
	if !changedOpening.BetStartAt.Equal(openingBody.BetStartAt) {
		t.Fatal("opening date/time not saved")
	}
	mustStatus(t, f.call("POST", openingPath, "opening-change", auth.AccessToken, "", openingBody), 200)
	mustStatus(t, f.call("POST", openingPath, "opening-stale", auth.AccessToken, "", openingBody), 409)
	openingBody.ExpectedBetStartAt = changedOpening.BetStartAt
	openingBody.BetStartAt = future.BetEndAt
	mustStatus(t, f.call("POST", openingPath, "opening-invalid", auth.AccessToken, "", openingBody), 400)
	periodBody := platformlottery.PeriodInput{PeriodNo: "shared-001", BetStartAt: now.Add(-3 * time.Hour), BetEndAt: now.Add(-2 * time.Hour), DrawAt: now.Add(-time.Hour), Reason: "Create shared period"}
	createdPeriod := f.call("POST", "/api/v1/platform/platform-games/"+game.ID+"/periods", "platform-create-period", auth.AccessToken, "", periodBody)
	mustStatus(t, createdPeriod, 201)
	var period platformlottery.Period
	managedData(t, createdPeriod, &period)
	manualExisting := platformlottery.ManualResultInput{PeriodNo: period.PeriodNo, DrawDate: "1999-01-02", Result: rules.Draw{Digits: []int{1, 2, 3}}}
	mustStatus(t, f.call("POST", manualPath, "manual-existing-date-mismatch", auth.AccessToken, "", manualExisting), 400)
	manualExisting.DrawDate = period.DrawAt.In(loc).Format("2006-01-02")
	mustStatus(t, f.call("POST", manualPath, "manual-existing-publish", auth.AccessToken, "", manualExisting), 201)
	mustStatus(t, f.call("POST", manualPath, "manual-existing-publish", auth.AccessToken, "", manualExisting), 201)
	mustStatus(t, f.call("POST", manualPath, "manual-existing-duplicate", auth.AccessToken, "", manualExisting), 409)
	resultBody := map[string]any{"result": rules.Draw{Digits: []int{1, 2, 3}}, "reason": "Publish shared result"}
	mustStatus(t, f.call("POST", "/api/v1/platform/platform-games/periods/"+period.ID+"/result", "platform-publish-result", auth.AccessToken, "", resultBody), 409)
	modelBody.ExpectedVersion = 2
	locked := f.call("POST", "/api/v1/platform/platform-games/"+game.ID+"/model", "platform-used-model", auth.AccessToken, "", modelBody)
	mustStatus(t, locked, 409)
	if !strings.Contains(locked.Body.String(), "PLATFORM_GAME_MODEL_LOCKED") {
		t.Fatal("missing explicit used-model rejection")
	}
	// The shared client requires exact pagination totals on lists and detail reads.
	for _, surface := range []struct{ prefix, token, brand string }{
		{"/api/v1/platform", auth.AccessToken, ""},
		{"/api/v1/admin", f.token, managedBrand},
		{"/api/v1/b/aurora", "", ""},
	} {
		for _, offset := range []string{"0", "99"} {
			var detail platformlottery.Detail
			managedData(t, f.call("GET", surface.prefix+"/platform-games/"+game.ID+"?limit=1&offset="+offset, "", surface.token, surface.brand, nil), &detail)
			if detail.TotalCount != "2" || len(detail.Periods) > 1 {
				t.Fatalf("%s offset=%s: total=%q periods=%d", surface.prefix, offset, detail.TotalCount, len(detail.Periods))
			}
		}
	}
	read := f.call("GET", "/api/v1/admin/platform-games/"+game.ID, "", f.token, managedBrand, nil)
	mustStatus(t, read, 200)
	if !strings.Contains(read.Body.String(), period.ID) {
		t.Fatal("connected brand did not read canonical period")
	}
	mustStatus(t, f.call("GET", "/api/v1/admin/platform-games/"+game.ID, "", f.token, "0199a000-0000-7000-8000-000000000002", nil), 403)
	if _, e := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='platform_game.write.platform'`, role); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("POST", "/api/v1/platform/platform-games", "platform-game-create", auth.AccessToken, "", body), 403)
	modelBody.ExpectedVersion = 1
	mustStatus(t, f.call("POST", "/api/v1/platform/platform-games/"+game.ID+"/model", "platform-model-update", auth.AccessToken, "", modelBody), 403)
}

func TestPlatformGameRenamePermissionsVersionAndReplay(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	platformID := ids.New()
	if _, e := f.pool.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash,is_super_admin) SELECT $1,'rename_operator',password_hash,true FROM admin_accounts WHERE id=$2`, platformID, f.root); e != nil {
		t.Fatal(e)
	}
	pf := f
	pf.root = platformID
	grantPlatformPermission(t, pf, "platform_game.view.platform", "platform_game.write.platform")
	login := f.call("POST", "/api/v1/platform/auth/login", "rename-login", "", "", map[string]string{"identifier": "rename_operator", "password": "root-test-password-2026"})
	mustStatus(t, login, 200)
	var auth identity.AdminAuthentication
	managedData(t, login, &auth)
	created := f.call("POST", "/api/v1/platform/platform-games", "rename-create", auth.AccessToken, "", platformlottery.GameInput{Code: "rename_game", Name: "Original", Model: rules.Model{Type: "DIGITS_0_9", Length: 3, Ordered: true}, Timezone: "UTC"})
	mustStatus(t, created, 201)
	var game platformlottery.Game
	managedData(t, created, &game)
	overviewPath := "/api/v1/platform/platform-games/" + game.ID + "/draw-overview"
	mustStatus(t, f.call("GET", overviewPath, "", auth.AccessToken, "", nil), 200)
	mustStatus(t, f.call("GET", overviewPath+"?year=bad", "", auth.AccessToken, "", nil), 400)
	mustStatus(t, f.call("GET", overviewPath, "", f.token, "", nil), 403)
	mustStatus(t, f.call("GET", strings.Replace(overviewPath, "/platform/", "/admin/", 1), "", f.token, managedBrand, nil), 404)

	path := "/api/v1/platform/platform-games/" + game.ID + "/name"
	in := platformlottery.NameInput{Name: "Renamed", ExpectedVersion: 1}
	renamed := f.call("POST", path, "rename-once", auth.AccessToken, "", in)
	mustStatus(t, renamed, 200)
	var out platformlottery.Game
	managedData(t, renamed, &out)
	if out.Name != "Renamed" || out.Code != game.Code || out.Version != 2 || out.Model.Length != 3 {
		t.Fatalf("rename changed identity/model: %+v", out)
	}
	mustStatus(t, f.call("POST", path, "rename-once", auth.AccessToken, "", in), 200)
	mustStatus(t, f.call("POST", path, "rename-stale", auth.AccessToken, "", in), 409)
	mustStatus(t, f.call("POST", path, "rename-brand", f.token, "", in), 403)
	mustStatus(t, f.call("POST", "/api/v1/admin/platform-games/"+game.ID+"/name", "rename-wrong-entry", f.token, managedBrand, in), 404)
	mustStatus(t, f.call("POST", path, "rename-empty", auth.AccessToken, "", platformlottery.NameInput{Name: " ", ExpectedVersion: 2}), 400)
	var count int
	if e := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE resource_id=$1 AND action='platform_game.name.update'`, game.ID).Scan(&count); e != nil {
		t.Fatal(e)
	}
	if count != 1 {
		t.Fatalf("rename audit replayed: %d", count)
	}
}

func TestBrandPlatformGameMutationRoutesOnlyExposeConnectionAndSales(t *testing.T) {
	allowed := map[string]bool{
		"/api/v1/admin/platform-games/{id}/connect":      true,
		"/api/v1/admin/platform-games/{id}/availability": true,
		// Brands configure sales only for their own connected games.
		"/api/v1/admin/platform-games/{id}/configured/sales": true,
		// The limits page updates the same configured sales record.
		"/api/v1/admin/platform-games/{id}/bet-limits": true,
		// This endpoint evaluates an unsaved rule draft; it does not mutate a game.
		"/api/v1/admin/platform-games/rule-preview": true,
	}
	for _, route := range RegisteredRoutes() {
		if route.Method == "GET" || !strings.HasPrefix(route.Path, "/api/v1/admin/platform-games") {
			continue
		}
		if route.Method != "POST" || !allowed[route.Path] {
			t.Fatalf("brand can mutate platform game configuration: %+v", route)
		}
		delete(allowed, route.Path)
	}
	if len(allowed) != 0 {
		t.Fatalf("missing supported brand game routes: %v", allowed)
	}
}
