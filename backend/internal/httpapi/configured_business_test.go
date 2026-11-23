package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/dataretention"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestConfiguredHTTPWorkflowScopesReplayAndRevocation(t *testing.T) {
	f := pointsFixture(t)
	ctx := context.Background()
	var role string
	if e := f.pool.QueryRow(ctx, `SELECT role_id::text FROM admin_account_roles WHERE account_id=$1`, f.root).Scan(&role); e != nil {
		t.Fatal(e)
	}
	for _, p := range []string{"platform_game.view.brand", "platform_game.connect.brand", "platform_game.sell.brand", "bet.view.brand"} {
		if _, e := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2)`, role, p); e != nil {
			t.Fatal(e)
		}
	}
	platformID := ids.New()
	if _, e := f.pool.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash,is_super_admin) SELECT $1,'configured_platform',password_hash,true FROM admin_accounts WHERE id=$2`, platformID, f.root); e != nil {
		t.Fatal(e)
	}
	pf := f.managementHTTP
	pf.root = platformID
	platformRole := grantPlatformPermission(t, pf, "platform_game.view.platform", "platform_game.write.platform")
	login := f.call("POST", "/api/v1/platform/auth/login", "configured-login", "", "", map[string]string{"identifier": "configured_platform", "password": "root-test-password-2026"})
	mustStatus(t, login, 200)
	var auth identity.AdminAuthentication
	managedData(t, login, &auth)
	for _, name := range rules.ConfiguredTemplateNames {
		mustStatus(t, f.call("GET", "/api/v1/platform/platform-games/rule-preview/template?template="+name, "", auth.AccessToken, "", nil), 200)
	}
	for _, suffix := range []string{"", "?template=unknown", "?template=mark_six&extra=x"} {
		mustStatus(t, f.call("GET", "/api/v1/platform/platform-games/rule-preview/template"+suffix, "", auth.AccessToken, "", nil), 400)
	}
	raw, e := os.ReadFile("../rules/config/examples/lucky_group_preview.json")
	if e != nil {
		t.Fatal(e)
	}
	engine, e := rules.RestoreConfiguredEngine(raw)
	if e != nil {
		t.Fatal(e)
	}
	var snap rules.ConfiguredSnapshot
	json.Unmarshal(engine.Snapshot(), &snap)
	create := f.call("POST", "/api/v1/platform/platform-games", "configured-http-game", auth.AccessToken, "", platformlottery.GameInput{Code: "configured_http", Name: "Configured HTTP", Timezone: "UTC", Model: snap.Catalog.Model})
	mustStatus(t, create, 201)
	var game platformlottery.Game
	managedData(t, create, &game)
	path := "/api/v1/platform/platform-games/" + game.ID + "/configured/rules"
	mustStatus(t, f.call("POST", "/api/v1/platform/platform-games/"+game.ID+"/periods", "unpublished-period", auth.AccessToken, "", platformlottery.PeriodInput{PeriodNo: "unpublished", BetStartAt: time.Now().Add(-time.Hour), BetEndAt: time.Now().Add(time.Hour), DrawAt: time.Now().Add(time.Hour)}), 404)

	cases := []platformlottery.ConfiguredCase{{Requests: []rules.ConfiguredRequest{{PlayID: "operator_new_play", Options: []string{"lucky"}, Plate: "A", Points: 1000, Multiplier: 1}}, Draw: rules.Draw{Regular: []int{1, 3, 7, 4, 5, 6}, Special: []int{2}, Digits: []int{}}, Bet: 1000, Prize: 2500}}
	input := platformlottery.ConfiguredRulesInput{Snapshot: engine.Snapshot()}
	mustStatus(t, f.call("GET", path, "", auth.AccessToken, "", nil), 200)
	saved := f.call("POST", path, "configured-http-save", auth.AccessToken, "", input)
	mustStatus(t, saved, 200)
	var version, replayVersion platformlottery.ConfiguredVersion
	managedData(t, saved, &version)
	replaySave := f.call("POST", path, "configured-http-save", auth.AccessToken, "", input)
	mustStatus(t, replaySave, 200)
	managedData(t, replaySave, &replayVersion)
	if version.Status != "published" || version.ID != replayVersion.ID {
		t.Fatal("save must apply rules once", version, replayVersion)
	}
	mustStatus(t, f.call("POST", path, "configured-stale-save", auth.AccessToken, "", input), 409)
	bad := platformlottery.ConfiguredRulesInput{ID: version.ID, Revision: version.Revision, Snapshot: json.RawMessage(`{}`)}
	mustStatus(t, f.call("POST", path, "configured-invalid-save", auth.AccessToken, "", bad), 400)
	mustStatus(t, f.call("POST", path, "configured-brand-spoof", f.token, "", input), 403)
	mustStatus(t, f.call("POST", "/api/v1/admin/platform-games/"+game.ID+"/configured/rules", "configured-brand-rule", f.token, managedBrand, input), 404)
	mustStatus(t, f.call("POST", "/api/v1/admin/platform-games/"+game.ID+"/connect", "configured-connect", f.token, managedBrand, map[string]string{}), 201)
	salesPath := "/api/v1/admin/platform-games/" + game.ID + "/configured/sales"
	sales := platformlottery.ConfiguredSales{RuleID: version.ID, Enabled: true, Prices: snap.Prices, MinPoints: 100, MaxPoints: 10000, MaxPeriodPoints: 100000, MaxUserPeriodPoints: 50000}

	mustStatus(t, f.call("GET", salesPath, "", f.token, managedBrand, nil), 404)
	mustStatus(t, f.call("POST", "/api/v1/platform/platform-games/"+game.ID+"/configured/sales", "configured-platform-sales-denied", auth.AccessToken, managedBrand, sales), 404)
	mustStatus(t, f.call("POST", salesPath, "configured-sales-foreign", f.token, pointsBrandB, sales), 403)
	var beforeLedger string
	if e := f.pool.QueryRow(ctx, `SELECT count(*)::text FROM point_ledger_entries`).Scan(&beforeLedger); e != nil {
		t.Fatal(e)
	}
	firstSale := f.call("POST", salesPath, "configured-sales-first", f.token, managedBrand, sales)
	mustStatus(t, firstSale, 200)
	var appliedSale platformlottery.ConfiguredSales
	managedData(t, firstSale, &appliedSale)
	if appliedSale.Version != 1 || appliedSale.RuleID != version.ID || !appliedSale.Enabled {
		t.Fatal(appliedSale)
	}
	mustStatus(t, f.call("POST", salesPath, "configured-sales-first", f.token, managedBrand, sales), 200)
	stale := f.call("POST", salesPath, "configured-sales-stale", f.token, managedBrand, sales)
	mustStatus(t, stale, 409)
	if !strings.Contains(stale.Body.String(), `"CONFIGURED_SALES_VERSION_CONFLICT"`) {
		t.Fatal("missing sales conflict code", stale.Body.String())
	}
	changedSale := sales
	changedSale.Enabled = false
	mustStatus(t, f.call("POST", salesPath, "configured-sales-first", f.token, managedBrand, changedSale), 409)
	invalidSale := sales
	invalidSale.Version = 1
	invalidSale.MaxPoints = 0
	invalidLimits := f.call("POST", salesPath, "configured-sales-invalid", f.token, managedBrand, invalidSale)
	mustStatus(t, invalidLimits, 400)
	if !strings.Contains(invalidLimits.Body.String(), `"CONFIGURED_SALES_LIMITS_INVALID"`) {
		t.Fatal("missing sales limits error code", invalidLimits.Body.String())
	}
	// The common rules still constrain the valid odds range.
	invalidSale = sales
	invalidSale.Version = 1
	rawPrices, _ := json.Marshal(sales.Prices)
	json.Unmarshal(rawPrices, &invalidSale.Prices)
	invalidSale.Prices.Plates["A"]["lucky_price"] = "1001"
	invalidOdds := f.call("POST", salesPath, "configured-sales-price-out-of-range", f.token, managedBrand, invalidSale)
	mustStatus(t, invalidOdds, 400)
	if !strings.Contains(invalidOdds.Body.String(), `"CONFIGURED_SALES_ODDS_INVALID"`) {
		t.Fatal("missing sales odds error code", invalidOdds.Body.String())
	}
	if _, e := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='platform_game.sell.brand'`, role); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("POST", salesPath, "configured-sales-first", f.token, managedBrand, sales), 403)
	if _, e := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,'platform_game.sell.brand')`, role); e != nil {
		t.Fatal(e)
	}
	var afterLedger string
	var salesAudits int
	if e := f.pool.QueryRow(ctx, `SELECT (SELECT count(*)::text FROM point_ledger_entries),(SELECT count(*) FROM audit_logs WHERE action='configured_sales.write')`).Scan(&afterLedger, &salesAudits); e != nil || beforeLedger != afterLedger || salesAudits != 1 {
		t.Fatal("sales must be audited once without money changes", beforeLedger, afterLedger, salesAudits, e)
	}
	mustStatus(t, f.call("POST", "/api/v1/admin/platform-games/"+game.ID+"/automation", "configured-automation-denied", f.token, managedBrand, map[string]any{"enabled": true}), 404)
	mustStatus(t, f.call("GET", "/api/v1/admin/platform-games/"+game.ID+"/configured/rules", "", f.token, "0199a000-0000-7000-8000-000000000002", nil), 403)
	var period platformlottery.Period
	created := f.call("POST", "/api/v1/platform/platform-games/"+game.ID+"/periods", "configured-period", auth.AccessToken, "", platformlottery.PeriodInput{PeriodNo: "configured-http-1", BetStartAt: time.Now().Add(-time.Hour), BetEndAt: time.Now().Add(10 * time.Second), DrawAt: time.Now().Add(10 * time.Second)})
	mustStatus(t, created, 201)
	managedData(t, created, &period)
	tx, e := f.pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	var delta points.Balance
	delta[0][0] = 10000
	_, e = (points.Store{DB: f.pool}).Post(ctx, tx, points.Change{BrandID: managedBrand, MemberID: f.memberID, EntryType: "adjustment", ReferenceType: "test_fund", OperationKey: "configured-http-fund", Reason: "test funds", ActorType: "system", RequestID: ids.New(), Delta: delta, Allocation: []points.Allocation{{Source: "recharge", State: "available", Points: 10000}}})
	if e != nil {
		tx.Rollback(ctx)
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	in := betting.ConfiguredInput{PeriodID: period.ID, RuleID: version.ID, SalesVersion: 1, SnapshotHash: engine.SnapshotHash(), Requests: cases[0].Requests}
	preview := f.call("POST", "/api/v1/configured/preview", "", f.userToken, managedBrand, in)
	mustStatus(t, preview, 200)
	var q struct {
		betting.ConfiguredQuote
		ActorContext string `json:"actor_context"`
	}
	managedData(t, preview, &q)
	if len(q.Lines) != 1 || q.BetPoints != 1000 {
		t.Fatal(q)
	}
	mustStatus(t, f.call("POST", "/api/v1/configured/orders", "configured-missing-actor", f.userToken, managedBrand, in), 403)
	in.ActorContext = q.ActorContext
	receipt := f.call("POST", "/api/v1/configured/orders", "configured-http-order", f.userToken, managedBrand, in)
	mustStatus(t, receipt, 201)
	var placed betting.ConfiguredOrdersReceipt
	managedData(t, receipt, &placed)
	if len(placed.Items) != 1 {
		t.Fatal(placed)
	}
	order := placed.Items[0]
	var zone string
	if err := f.pool.QueryRow(ctx, `SELECT timezone FROM platform_settings WHERE id`).Scan(&zone); err != nil {
		t.Fatal(err)
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		t.Fatal(err)
	}
	day := order.PlacedAt.In(loc).Format(time.DateOnly)
	const frequencyPath = "/api/v1/platform/reports/bet-frequency"
	frequencyURL := frequencyPath + "?game_id=" + game.ID + "&from=" + day + "&to=" + day
	mustStatus(t, f.call("GET", frequencyURL, "", auth.AccessToken, "", nil), 403)
	grantPlatformPermission(t, pf, reportBettingPlat)
	mustStatus(t, f.call("GET", frequencyURL, "", auth.AccessToken, managedBrand, nil), 400)
	mustStatus(t, f.call("GET", frequencyURL+"&brand_id="+managedBrand, "", auth.AccessToken, "", nil), 400)
	mustStatus(t, f.call("GET", frequencyURL+"&game_id="+game.ID, "", auth.AccessToken, "", nil), 400)
	mustStatus(t, f.call("GET", strings.Replace(frequencyURL, game.ID, ids.New(), 1), "", auth.AccessToken, "", nil), 404)
	mustStatus(t, f.call("GET", strings.Replace(frequencyURL, "/platform/", "/admin/", 1), "", f.token, managedBrand, nil), 404)
	readFrequency := func() (page struct {
		Count     string `json:"effective_item_count"`
		PlayCount string `json:"play_count"`
		Items     []struct {
			PlayID string                `json:"play_id"`
			Count  string                `json:"effective_item_count"`
			Label  rules.ConfiguredLabel `json:"label"`
		} `json:"items"`
	}) {
		t.Helper()
		result := f.call("GET", frequencyURL, "", auth.AccessToken, "", nil)
		mustStatus(t, result, 200)
		managedData(t, result, &page)
		return page
	}
	if pending := readFrequency(); pending.Count != "0" || pending.PlayCount != "0" || pending.Items == nil || len(pending.Items) != 0 {
		t.Fatal("pending order counted", pending)
	}

	pushPeriodResponse := f.call("POST", "/api/v1/platform/platform-games/"+game.ID+"/periods", "configured-frequency-push-period", auth.AccessToken, "", platformlottery.PeriodInput{
		PeriodNo: "configured-frequency-push", BetStartAt: period.BetStartAt, BetEndAt: period.BetEndAt, DrawAt: period.DrawAt,
	})
	mustStatus(t, pushPeriodResponse, 201)
	var pushPeriod platformlottery.Period
	managedData(t, pushPeriodResponse, &pushPeriod)
	otherBrand, otherOrders := placeFrequencyOrdersInTemporaryBrand(t, f.pool, platformID, game.ID, pushPeriod.ID, in)
	orderPath := "/api/v1/configured/orders/" + order.ID
	detail := f.call("GET", orderPath, "", f.userToken, managedBrand, nil)
	mustStatus(t, detail, 200)
	var ticket struct {
		BrandID  string                  `json:"brand_id"`
		MemberID string                  `json:"member_id"`
		Order    betting.ConfiguredOrder `json:"order"`
	}
	managedData(t, detail, &ticket)
	if ticket.BrandID != managedBrand || ticket.MemberID == "" || ticket.Order.ID != order.ID || ticket.Order.GameName == "" || ticket.Order.PeriodNo != period.PeriodNo {
		t.Fatal("incomplete notification ticket", ticket)
	}
	mustStatus(t, f.call("GET", orderPath, "", "", managedBrand, nil), 401)
	mustStatus(t, f.call("GET", "/api/v1/b/harbor/configured/orders/"+order.ID, "", f.userToken, "", nil), 401)
	mustStatus(t, f.call("GET", "/api/v1/configured/orders/"+ids.New(), "", f.userToken, managedBrand, nil), 404)
	for _, scope := range [][2]string{{pointsBrandB, ticket.MemberID}, {managedBrand, ids.New()}} {
		if _, err := (betting.Service{DB: f.pool}).ConfiguredOrder(ctx, scope[0], scope[1], order.ID); !errors.Is(err, betting.ErrNotFound) {
			t.Fatal("order detail leaked across brand/member", err)
		}
	}
	// Availability changes are independent of the current rule and sales versions.
	platformAvailability := "/api/v1/platform/platform-games/" + game.ID + "/configured/availability"
	brandAvailability := "/api/v1/admin/platform-games/" + game.ID + "/configured/availability"
	platformInput := platformlottery.ConfiguredAvailabilityInput{
		RuleID: version.ID,
		Items:  platformlottery.ConfiguredOptionStates{"operator_new_play": {"lucky": false}},
	}
	postAvailability := func(path, key, token, brand string, input platformlottery.ConfiguredAvailabilityInput, status int) {
		t.Helper()
		mustStatus(t, f.call("POST", path, key, token, brand, input), status)
	}
	brandInput := platformlottery.ConfiguredAvailabilityInput{
		RuleID: version.ID,
		Items:  platformlottery.ConfiguredOptionStates{"operator_new_play": {"lucky": true}},
	}
	postAvailability(brandAvailability, "availability-brand-enable", f.token, managedBrand, brandInput, 200)
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='platform_game.sell.brand'`, role); err != nil {
		t.Fatal(err)
	}
	postAvailability(brandAvailability, "availability-brand-without-sell", f.token, managedBrand, brandInput, 403)
	postAvailability(brandAvailability, "availability-brand-enable", f.token, managedBrand, brandInput, 403)
	if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,'platform_game.sell.brand')`, role); err != nil {
		t.Fatal(err)
	}
	postAvailability(platformAvailability, "availability-platform", auth.AccessToken, "", platformInput, 200)
	postAvailability(platformAvailability, "availability-platform", auth.AccessToken, "", platformInput, 200)
	postAvailability(platformAvailability, "availability-stale", auth.AccessToken, "", platformInput, 409)
	postAvailability(brandAvailability, "availability-cross-brand", f.token, pointsBrandB, platformInput, 403)
	postAvailability(platformAvailability, "availability-brand-platform", f.token, "", platformInput, 403)
	postAvailability(brandAvailability, "availability-invalid-option", f.token, managedBrand,
		platformlottery.ConfiguredAvailabilityInput{RuleID: version.ID, Version: 1, Items: platformlottery.ConfiguredOptionStates{"operator_new_play": {"unknown": true}}}, 400)

	mustStatus(t, f.call("POST", brandAvailability, "availability-null-option", f.token, managedBrand,
		map[string]any{"rule_id": version.ID, "version": 0, "items": map[string]any{"operator_new_play": map[string]any{"lucky": nil}}}), 400)

	catalogPath := "/api/v1/configured/games/" + game.ID
	expectDisabled := func(want bool) {
		t.Helper()
		result := f.call("GET", catalogPath, "", f.userToken, managedBrand, nil)
		mustStatus(t, result, 200)
		var catalog struct {
			Disabled map[string][]string `json:"disabled_options"`
			RuleID   string              `json:"rule_id"`
			Sales    platformlottery.ConfiguredSales
		}
		managedData(t, result, &catalog)
		options := catalog.Disabled["operator_new_play"]
		if (len(options) == 1 && options[0] == "lucky") != want || catalog.RuleID != version.ID || catalog.Sales.Version != 1 {
			t.Fatal("wrong effective option availability", catalog)
		}
	}
	expectDisabled(true)
	mustStatus(t, f.call("POST", "/api/v1/configured/preview", "", f.userToken, managedBrand, in), 409)
	blocked := f.call("POST", "/api/v1/configured/orders", "availability-rejected-order", f.userToken, managedBrand, in)
	mustStatus(t, blocked, 409)
	if !strings.Contains(blocked.Body.String(), "BET_OPTION_DISABLED") {
		t.Fatal(blocked.Body.String())
	}
	brandInput.Version++
	brandInput.Items = platformlottery.ConfiguredOptionStates{}
	postAvailability(brandAvailability, "availability-brand-inherit-while-disabled", f.token, managedBrand, brandInput, 409)
	brandInput.Items = platformlottery.ConfiguredOptionStates{"operator_new_play": {"lucky": false}}
	postAvailability(brandAvailability, "availability-brand-write-while-disabled", f.token, managedBrand, brandInput, 409)
	expectDisabled(true)
	read := f.call("GET", brandAvailability, "", f.token, managedBrand, nil)
	mustStatus(t, read, 200)
	var availability platformlottery.ConfiguredAvailability
	managedData(t, read, &availability)
	if availability.Platform.Version != 1 || availability.Platform.Items["operator_new_play"]["lucky"] ||
		availability.Brand.Version != 1 || !availability.Brand.Items["operator_new_play"]["lucky"] {
		t.Fatal("platform disable did not preserve the stored brand setting", availability)
	}
	platformInput.Version++
	platformInput.Items = platformlottery.ConfiguredOptionStates{}
	postAvailability(platformAvailability, "availability-platform-enable", auth.AccessToken, "", platformInput, 200)
	expectDisabled(false)
	mustStatus(t, f.call("POST", "/api/v1/configured/preview", "", f.userToken, managedBrand, in), 200)
	postAvailability(brandAvailability, "availability-brand-disable", f.token, managedBrand, brandInput, 200)
	expectDisabled(true)

	replay := f.call("POST", "/api/v1/configured/orders", "configured-http-order", f.userToken, managedBrand, in)
	mustStatus(t, replay, 201)
	var same betting.ConfiguredOrdersReceipt
	managedData(t, replay, &same)
	if len(same.Items) != 1 || same.Items[0].ID != order.ID {
		t.Fatal("duplicate debit")
	}
	mustStatus(t, f.call("GET", "/api/v1/admin/configured-orders", "", f.token, managedBrand, nil), 200)
	mustStatus(t, f.call("GET", "/api/v1/admin/configured-orders", "", f.token, pointsBrandB, nil), 403)
	mustStatus(t, f.call("GET", "/api/v1/platform/configured-orders", "", auth.AccessToken, managedBrand, nil), 403)
	filterPath := "/filter-options?resource=bet&kind=game&limit=100&offset=0"
	mustStatus(t, f.call("GET", "/api/v1/platform"+filterPath, "", auth.AccessToken, managedBrand, nil), 403)
	grantPlatformPermission(t, pf, "bet.view.platform")
	mustStatus(t, f.call("GET", "/api/v1/platform/configured-orders", "", auth.AccessToken, managedBrand, nil), 200)

	for _, surface := range []struct{ prefix, token string }{{"admin", f.token}, {"platform", auth.AccessToken}} {
		response := f.call("GET", "/api/v1/"+surface.prefix+filterPath, "", surface.token, managedBrand, nil)
		mustStatus(t, response, 200)
		var options struct {
			BrandID string `json:"brand_id"`
			Items   []struct{ ID, Name string }
		}
		managedData(t, response, &options)
		if options.BrandID != managedBrand || len(options.Items) != 1 || options.Items[0].ID != game.ID || options.Items[0].Name != game.Name {
			t.Fatal("wrong configured-order game directory", response.Body.String())
		}
	}
	mustStatus(t, f.call("GET", "/api/v1/admin"+filterPath, "", f.token, pointsBrandB, nil), 403)

	summary := f.call("GET", "/api/v1/betting-summary", "", f.userToken, managedBrand, nil)
	mustStatus(t, summary, 200)
	var totals map[string]string
	managedData(t, summary, &totals)
	if totals["unsettled_points"] != "10" {
		t.Fatal("configured stake omitted from summary", totals)
	}
	in.Requests[0].Points = 1100
	mustStatus(t, f.call("POST", "/api/v1/configured/orders", "configured-http-order", f.userToken, managedBrand, in), 409)
	in.Requests[0].Points = 1000
	mustStatus(t, f.call("GET", "/api/v1/configured/games/"+game.ID+"/orders", "", f.userToken, managedBrand, nil), 200)
	mustStatus(t, f.call("GET", "/api/v1/admin/platform-games/"+game.ID+"/configured/orders", "", f.token, managedBrand, nil), 200)
	mustStatus(t, f.call("GET", "/api/v1/admin/platform-games/"+game.ID+"/configured/orders", "", f.token, "0199a000-0000-7000-8000-000000000002", nil), 403)
	mustStatus(t, f.call("GET", "/api/v1/configured/orders", "", f.userToken, managedBrand, nil), 200)
	mustStatus(t, f.call("GET", "/api/v1/configured/orders?offset=-1", "", f.userToken, managedBrand, nil), 400)
	mustStatus(t, f.call("GET", "/api/v1/b/harbor/configured/orders", "", f.userToken, "", nil), 401)

	// Editing sales changes future confirmation versions, never a purchased ticket.
	editedSale := appliedSale
	editedRaw, _ := json.Marshal(appliedSale.Prices)
	json.Unmarshal(editedRaw, &editedSale.Prices)
	editedSale.Prices.Plates["A"]["lucky_price"] = "2.6"
	brandChange := platformlottery.ConfiguredSalesInput{ConfiguredSales: editedSale, Availability: &platformlottery.ConfiguredAvailabilityScope{
		Version: 1, Items: platformlottery.ConfiguredOptionStates{},
	}}
	mustStatus(t, f.call("POST", salesPath, "configured-odds-stale-brand", f.token, managedBrand, brandChange), 409)
	expectDisabled(true) // The sales version also remains unchanged after the availability conflict.
	brandChange.Availability.Version++
	edited := f.call("POST", salesPath, "configured-sales-edit", f.token, managedBrand, brandChange)
	mustStatus(t, edited, 200)
	var nextSale platformlottery.ConfiguredSales
	managedData(t, edited, &nextSale)
	if nextSale.Version != 2 {
		t.Fatal(nextSale)
	}
	mustStatus(t, f.call("POST", salesPath, "configured-sales-edit", f.token, managedBrand, brandChange), 200)
	brandState, err := platformlottery.ReadConfiguredAvailability(ctx, f.pool, managedBrand, game.ID, version.ID)
	if err != nil || brandState.Brand.Version != 3 || brandState.OptionDisabled("operator_new_play", "lucky") {
		t.Fatal("brand odds and availability did not apply together exactly once", brandState, err)
	}
	mustStatus(t, f.call("POST", "/api/v1/configured/preview", "", f.userToken, managedBrand, in), 409)
	brandEngine, err := platformlottery.ConfiguredSalesEngine(version, nextSale)
	if err != nil {
		t.Fatal(err)
	}
	nextPreview := in
	nextPreview.SalesVersion = nextSale.Version
	nextPreview.SnapshotHash = brandEngine.SnapshotHash()
	brandQuote := f.call("POST", "/api/v1/configured/preview", "", f.userToken, managedBrand, nextPreview)
	mustStatus(t, brandQuote, 200)
	managedData(t, brandQuote, &q)
	if len(q.Lines) != 1 || q.Lines[0].Prices["hit"] != "2.6" {
		t.Fatal("preview did not use brand odds above platform odds", q)
	}
	mustStatus(t, f.call("POST", "/api/v1/configured/orders", "configured-http-order", f.userToken, managedBrand, in), 201)
	snap.Prices.Plates["A"]["lucky_price"] = "3"
	nextSnapshot, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	nextInput := platformlottery.ConfiguredRulesInput{ID: version.ID, Revision: version.Revision, Snapshot: nextSnapshot,
		Availability: &platformlottery.ConfiguredAvailabilityScope{Version: 1, Items: platformlottery.ConfiguredOptionStates{"operator_new_play": {"lucky": true}}},
	}
	mustStatus(t, f.call("POST", path, "configured-odds-stale-platform", auth.AccessToken, "", nextInput), 409)
	var retainedPublication platformlottery.ConfiguredVersion
	managedData(t, f.call("GET", path, "", auth.AccessToken, "", nil), &retainedPublication)
	if retainedPublication.ID != version.ID {
		t.Fatal("platform odds changed despite the availability conflict", retainedPublication)
	}
	nextInput.Availability.Version++
	updated := f.call("POST", path, "configured-replace", auth.AccessToken, "", nextInput)
	mustStatus(t, updated, 200)
	var nextVersion platformlottery.ConfiguredVersion
	managedData(t, updated, &nextVersion)
	if nextVersion.ID == version.ID || nextVersion.Status != "published" {
		t.Fatal("rules were not replaced", nextVersion)
	}
	mustStatus(t, f.call("POST", path, "configured-replace", auth.AccessToken, "", nextInput), 200)
	platformState, err := platformlottery.ReadConfiguredAvailability(ctx, f.pool, "", game.ID, nextVersion.ID)
	if err != nil || platformState.Platform.Version != 3 || !platformState.Platform.Items["operator_new_play"]["lucky"] {
		t.Fatal("platform odds and availability did not apply together exactly once", platformState, err)
	}
	mustStatus(t, f.call("POST", path, "configured-replace-stale", auth.AccessToken, "", nextInput), 409)
	assertListed := func(want bool) {
		t.Helper()
		response := f.call("GET", "/api/v1/configured/games", "", f.userToken, managedBrand, nil)
		mustStatus(t, response, 200)
		var directory struct {
			Games []betting.ConfiguredGameSummary
		}
		managedData(t, response, &directory)
		listed := false
		for _, entry := range directory.Games {
			listed = listed || entry.ID == game.ID
		}
		if listed != want {
			t.Fatal("wrong player catalogue", directory)
		}
	}
	assertListed(true)
	currentCatalog := f.call("GET", catalogPath, "", f.userToken, managedBrand, nil)
	mustStatus(t, currentCatalog, 200)
	var catalogue struct {
		Game         platformlottery.Game
		RuleID       string `json:"rule_id"`
		SnapshotHash string `json:"snapshot_hash"`
		Sales        platformlottery.ConfiguredSales
	}
	managedData(t, currentCatalog, &catalogue)
	carriedSale := nextSale
	carriedSale.RuleID = nextVersion.ID
	carriedSale.Version++
	if catalogue.Game.SalesStatus != "ready" || catalogue.RuleID != nextVersion.ID || !reflect.DeepEqual(catalogue.Sales, carriedSale) {
		t.Fatal("platform odds update changed the brand sales configuration", catalogue)
	}
	otherSale, err := platformlottery.ReadConfiguredSales(ctx, f.pool, otherBrand, game.ID, false)
	if err != nil || otherSale.RuleID != nextVersion.ID || otherSale.Prices.Plates["A"]["lucky_price"] != "2.5" {
		t.Fatal("platform odds update changed the other brand's prices", otherSale, err)
	}
	mustStatus(t, f.call("POST", "/api/v1/configured/preview", "", f.userToken, managedBrand, nextPreview), 409)
	nextPreview.RuleID, nextPreview.SalesVersion, nextPreview.SnapshotHash = catalogue.RuleID, catalogue.Sales.Version, catalogue.SnapshotHash
	newQuote := f.call("POST", "/api/v1/configured/preview", "", f.userToken, managedBrand, nextPreview)
	mustStatus(t, newQuote, 200)
	managedData(t, newQuote, &q)
	if q.RuleID != nextVersion.ID || len(q.Lines) != 1 || q.Lines[0].Prices["hit"] != "2.6" {
		t.Fatal("new quote did not use current rules and brand odds", q)
	}
	snap.Prices.Plates["A"]["lucky_price"] = "3.1"
	priceOnlySnapshot, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	priceOnly := f.call("POST", path, "configured-price-only", auth.AccessToken, "", platformlottery.ConfiguredRulesInput{
		ID: nextVersion.ID, Revision: nextVersion.Revision, Snapshot: priceOnlySnapshot,
	})
	mustStatus(t, priceOnly, 200)
	managedData(t, priceOnly, &nextVersion)
	assertListed(true)
	carriedSale.RuleID = nextVersion.ID
	carriedSale.Version++
	currentSale, err := platformlottery.ReadConfiguredSales(ctx, f.pool, managedBrand, game.ID, false)
	if err != nil || !reflect.DeepEqual(currentSale, carriedSale) {
		t.Fatal("price-only save did not carry the brand sales configuration forward", currentSale, err)
	}

	// A gameplay change requires a brand review, even when the price keys are identical.
	snap.Catalog.Domains["lucky"].Options[0].Mapping.Numbers = []int{8}
	changedRules, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	changed := f.call("POST", path, "configured-change-gameplay", auth.AccessToken, "", platformlottery.ConfiguredRulesInput{
		ID: nextVersion.ID, Revision: nextVersion.Revision, Snapshot: changedRules,
	})
	mustStatus(t, changed, 200)
	assertListed(false)
	brandDetail, err := (platformlottery.Service{DB: f.pool}).Detail(ctx, managedBrand, game.ID, 1, 0)
	if err != nil || brandDetail.Game.SalesStatus != "outdated" {
		t.Fatal("gameplay change did not require review", brandDetail.Game.SalesStatus, err)
	}
	var retainedRule, retainedPrice string
	if err := f.pool.QueryRow(ctx, `SELECT rule_id::text,snapshot->'prices'->'plates'->'A'->>'lucky_price' FROM configured_orders WHERE id=$1`, order.ID).Scan(&retainedRule, &retainedPrice); err != nil || retainedRule != version.ID || retainedPrice != "2.5" {
		t.Fatal("order rules changed", retainedRule, err)
	}
	mustStatus(t, f.call("POST", "/api/v1/configured/preview", "", f.userToken, managedBrand, in), 409)

	if wait := time.Until(period.DrawAt); wait > 0 {
		time.Sleep(wait)
	}
	published := f.call("POST", "/api/v1/platform/platform-games/periods/"+period.ID+"/result", "configured-publish", auth.AccessToken, "", map[string]any{"result": cases[0].Draw})
	mustStatus(t, published, 201)
	pushDraw := rules.Draw{Regular: []int{1, 3, 49, 4, 5, 6}, Special: []int{2}, Digits: []int{}}
	mustStatus(t, f.call("POST", "/api/v1/platform/platform-games/periods/"+pushPeriod.ID+"/result", "configured-frequency-push-result", auth.AccessToken, "", map[string]any{"result": pushDraw}), 201)
	settlementService := betting.Service{DB: f.pool}
	if n, e := settlementService.ProcessConfiguredSettlements(ctx, 10); e != nil || n != 3 {
		t.Fatal("configured settlement", n, e)
	}
	if n, e := settlementService.ProcessConfiguredSettlements(ctx, 10); e != nil || n != 0 {
		t.Fatal("settlement replay", n, e)
	}
	var refunded, effective string
	if err := f.pool.QueryRow(ctx, `SELECT result->>'refund_points',result->>'effective_points' FROM configured_orders WHERE id=$1 AND status='settled'`, otherOrders[1].ID).Scan(&refunded, &effective); err != nil || refunded != "10" || effective != "0" {
		t.Fatal("zero-effective fixture did not settle with a refund", refunded, effective, err)
	}
	var moneyEntries int
	var prize string
	if e := f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM point_ledger_entries WHERE reference_type='configured_order' AND reference_id=$1),result->>'prize_points' FROM configured_orders WHERE id=$1`, order.ID).Scan(&moneyEntries, &prize); e != nil || moneyEntries != 2 || prize != "25" {
		t.Fatal("frozen-price settlement", moneyEntries, prize, e)
	}
	wallet := f.call("GET", "/api/v1/wallet", "", f.userToken, managedBrand, nil)
	mustStatus(t, wallet, 200)
	var settledWallet struct {
		Available string `json:"available_points"`
	}
	managedData(t, wallet, &settledWallet)
	if settledWallet.Available != "115" {
		t.Fatal("settled wallet", settledWallet)
	}
	mustStatus(t, f.call("POST", "/api/v1/platform/platform-games/"+game.ID+"/lifecycle", "configured-http-offline", auth.AccessToken, "", platformlottery.LifecycleInput{ExpectedVersion: game.Version, Status: "offline"}), 200)
	history := f.call("GET", "/api/v1/b/aurora/configured/orders", "", f.userToken, "", nil)
	mustStatus(t, history, 200)
	var historyPage struct {
		Items []betting.ConfiguredOrder `json:"items"`
	}
	managedData(t, history, &historyPage)
	if len(historyPage.Items) != 1 || historyPage.Items[0].ID != order.ID {
		t.Fatal("offline history", historyPage)
	}
	// Notification details use the original ticket even after rules and sales change.
	offlineDetail := f.call("GET", "/api/v1/b/aurora/configured/orders/"+order.ID, "", f.userToken, "", nil)
	mustStatus(t, offlineDetail, 200)
	managedData(t, offlineDetail, &ticket)
	if ticket.Order.ID != order.ID || ticket.Order.RuleID != order.RuleID || ticket.Order.Status != "settled" || !reflect.DeepEqual(ticket.Order.Quote, order.Quote) {
		t.Fatal("notification detail changed purchased ticket", ticket)
	}
	unified := f.call("GET", "/api/v1/b/aurora/betting-history?game_id="+game.ID, "", f.userToken, "", nil)
	mustStatus(t, unified, 200)
	var unifiedPage struct {
		Games []betting.HistoryGame `json:"games"`
		Items []struct {
			Kind  string                  `json:"kind"`
			Order betting.ConfiguredOrder `json:"order"`
		} `json:"items"`
	}
	managedData(t, unified, &unifiedPage)
	if len(unifiedPage.Items) != 1 || unifiedPage.Items[0].Kind != "configured" || unifiedPage.Items[0].Order.ID != order.ID || len(unifiedPage.Games) != 1 || unifiedPage.Games[0].Name != game.Name {
		t.Fatal("unified offline configured history", unifiedPage)
	}
	var summaryFields map[string]json.RawMessage
	if err := json.Unmarshal(unifiedPage.Items[0].Order.Snapshot, &summaryFields); err != nil || len(summaryFields) != 1 || summaryFields["catalog"] == nil {
		t.Fatal("history returned executable snapshot", err)
	}
	if len(ticket.Order.Snapshot) <= len(unifiedPage.Items[0].Order.Snapshot) {
		t.Fatal("ticket detail lost its full snapshot")
	}
	// Query the now-offline game, run the real archive worker, and query the
	// same platform dates again after the source order has physically moved.
	directory := f.call("GET", frequencyPath+"/games", "", auth.AccessToken, "", nil)
	mustStatus(t, directory, 200)
	if !strings.Contains(directory.Body.String(), game.ID) {
		t.Fatal("offline game missing from frequency directory")
	}
	beforeArchive := readFrequency()
	if beforeArchive.Count != "2" || beforeArchive.PlayCount != "1" || len(beforeArchive.Items) != 1 ||
		beforeArchive.Items[0].PlayID != "operator_new_play" || beforeArchive.Items[0].Count != "2" ||
		beforeArchive.Items[0].Label != snap.Catalog.Plays[0].Label {
		t.Fatal("wrong settled frequency", beforeArchive)
	}
	cutoff, err := time.ParseInLocation(time.DateOnly, day, loc)
	if err != nil {
		t.Fatal(err)
	}
	cutoff = cutoff.AddDate(0, 0, 2)
	archiver := dataretention.Service{DB: f.pool}
	for _, source := range []struct{ brand, order string }{{managedBrand, order.ID}, {otherBrand, otherOrders[0].ID}} {
		taskID := ids.New()
		if _, err = f.pool.Exec(ctx, `INSERT INTO data_retention_tasks(id,brand_id,policy_version,timezone,execute_at,cutoff_at) VALUES($1,$2,1,$3,$4,$5)`, taskID, source.brand, zone, cutoff.AddDate(0, 2, 0), cutoff); err != nil {
			t.Fatal(err)
		}
		for range 20 {
			worked, err := archiver.Process(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !worked {
				break
			}
		}
		var liveOrders, receipts int
		if err = f.pool.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM configured_orders WHERE id=$1),
			(SELECT count(*) FROM data_retention_receipts WHERE family='configured_orders' AND id=$1 AND task_id=$2 AND NOT discard)`, source.order, taskID).Scan(&liveOrders, &receipts); err != nil || liveOrders != 0 || receipts != 1 {
			t.Fatal("configured order was not archived", liveOrders, receipts, err)
		}
		if afterArchive := readFrequency(); !reflect.DeepEqual(beforeArchive, afterArchive) {
			t.Fatal("frequency changed when order was archived", beforeArchive, afterArchive)
		}
	}

	if _, e = f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='platform_game.write.platform'`, platformRole); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("POST", path, "configured-http-save", auth.AccessToken, "", input), 403)
	if _, e = f.pool.Exec(ctx, `UPDATE sessions SET revoked_at=clock_timestamp() WHERE member_id=$1`, f.memberID); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("POST", "/api/v1/configured/orders", "configured-http-order", f.userToken, managedBrand, in), 401)
	mustStatus(t, f.call("GET", "/api/v1/configured/orders", "", f.userToken, managedBrand, nil), 401)
	mustStatus(t, f.call("GET", orderPath, "", f.userToken, managedBrand, nil), 401)
}

// The calling test owns the schema and drops it, including this temporary brand.
func placeFrequencyOrdersInTemporaryBrand(t *testing.T, pool *pgxpool.Pool, adminID, gameID, pushPeriodID string, in betting.ConfiguredInput) (string, []betting.ConfiguredOrder) {
	t.Helper()
	ctx := context.Background()
	brand, user, member, account, session := ids.New(), ids.New(), ids.New(), ids.New(), ids.New()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for _, step := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO brands(id,code,name,status) VALUES($1,$2,'Temporary frequency brand','active')`, []any{brand, "frequency_" + brand}},
		{`INSERT INTO global_users(id,username) VALUES($1,'frequency_member')`, []any{user}},
		{`INSERT INTO brand_members(id,brand_id,global_user_id,join_method) VALUES($1,$2,$3,'domain')`, []any{member, brand, user}},
		{`INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, []any{account, brand, member}},
		{`INSERT INTO point_buckets(brand_id,account_id,source,state)
		  SELECT $1,$2,source,state FROM unnest(ARRAY['recharge','winning','gift','commission']) source
		  CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal']) state ON CONFLICT DO NOTHING`, []any{brand, account}},
		{`INSERT INTO sessions(id,token_hash,user_id,member_id,brand_id,expires_at) VALUES($1,$2,$3,$4,$5,clock_timestamp()+interval '1 hour')`, []any{session, ids.New(), user, member, brand}},
		{`INSERT INTO brand_platform_games(brand_id,game_id,connected_by) VALUES($1,$2,$3)`, []any{brand, gameID, adminID}},
		{`INSERT INTO configured_sales(brand_id,game_id,version,rule_id,enabled,prices,min_points,max_points,max_period_points,max_user_period_points)
		  SELECT $1,game_id,version,rule_id,enabled,prices,min_points,max_points,max_period_points,max_user_period_points
		  FROM configured_sales WHERE brand_id=$2 AND game_id=$3`, []any{brand, managedBrand, gameID}},
	} {
		if _, err := tx.Exec(ctx, step.sql, step.args...); err != nil {
			t.Fatal(err)
		}
	}
	var delta points.Balance
	delta[0][0] = 10000
	if _, err = (points.Store{DB: pool}).Post(ctx, tx, points.Change{
		BrandID: brand, MemberID: member, EntryType: "adjustment", ReferenceType: "test_fund",
		OperationKey: ids.New(), RequestID: ids.New(), ActorType: "system", Delta: delta,
		Allocation: []points.Allocation{{Source: "recharge", State: "available", Points: 10000}},
	}); err != nil {
		t.Fatal(err)
	}
	actor := identity.Session{ID: session, View: identity.View{
		User: identity.User{ID: user}, Member: identity.Member{ID: member, BrandID: brand},
	}}
	service := betting.Service{DB: pool}
	var orders []betting.ConfiguredOrder
	for _, id := range []string{in.PeriodID, pushPeriodID} {
		in.PeriodID = id
		result, err := service.PlaceConfigured(ctx, tx, brand, actor, in, points.Metadata{RequestID: ids.New()})
		if err != nil || len(result.Items) != 1 {
			t.Fatal("temporary brand bet", len(result.Items), err)
		}
		orders = append(orders, result.Items[0])
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return brand, orders
}
