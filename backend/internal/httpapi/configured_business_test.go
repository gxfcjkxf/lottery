package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"os"
	"reflect"
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
	mustStatus(t, f.call("POST", salesPath, "configured-sales-stale", f.token, managedBrand, sales), 409)
	changedSale := sales
	changedSale.Enabled = false
	mustStatus(t, f.call("POST", salesPath, "configured-sales-first", f.token, managedBrand, changedSale), 409)
	invalidSale := sales
	invalidSale.Version = 1
	invalidSale.MaxPoints = 0
	mustStatus(t, f.call("POST", salesPath, "configured-sales-invalid", f.token, managedBrand, invalidSale), 400)
	// Price ceilings apply equally to initial sales and edits.
	invalidSale = sales
	invalidSale.Version = 1
	rawPrices, _ := json.Marshal(sales.Prices)
	json.Unmarshal(rawPrices, &invalidSale.Prices)
	invalidSale.Prices.Plates["A"]["lucky_price"] = "999"
	mustStatus(t, f.call("POST", salesPath, "configured-sales-price-ceiling", f.token, managedBrand, invalidSale), 400)
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
	editedSale.Prices.Plates["A"]["lucky_price"] = "2"
	edited := f.call("POST", salesPath, "configured-sales-edit", f.token, managedBrand, editedSale)
	mustStatus(t, edited, 200)
	var nextSale platformlottery.ConfiguredSales
	managedData(t, edited, &nextSale)
	if nextSale.Version != 2 {
		t.Fatal(nextSale)
	}
	mustStatus(t, f.call("POST", "/api/v1/configured/preview", "", f.userToken, managedBrand, in), 409)
	mustStatus(t, f.call("POST", "/api/v1/configured/orders", "configured-http-order", f.userToken, managedBrand, in), 201)
	snap.Prices.Plates["A"]["lucky_price"] = "3"
	nextSnapshot, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	nextInput := platformlottery.ConfiguredRulesInput{ID: version.ID, Revision: version.Revision, Snapshot: nextSnapshot}
	updated := f.call("POST", path, "configured-replace", auth.AccessToken, "", nextInput)
	mustStatus(t, updated, 200)
	var nextVersion platformlottery.ConfiguredVersion
	managedData(t, updated, &nextVersion)
	if nextVersion.ID == version.ID || nextVersion.Status != "published" {
		t.Fatal("rules were not replaced", nextVersion)
	}
	mustStatus(t, f.call("POST", path, "configured-replace-stale", auth.AccessToken, "", nextInput), 409)
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
	settlementService := betting.Service{DB: f.pool}
	if n, e := settlementService.ProcessConfiguredSettlements(ctx, 10); e != nil || n != 1 {
		t.Fatal("configured settlement", n, e)
	}
	if n, e := settlementService.ProcessConfiguredSettlements(ctx, 10); e != nil || n != 0 {
		t.Fatal("settlement replay", n, e)
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
