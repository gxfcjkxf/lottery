package httpapi

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"os"
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
	for _, p := range []string{"platform_game.view.brand", "platform_game.connect.brand", "platform_game.sell.brand"} {
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
	path := "/api/v1/platform/platform-games/" + game.ID + "/configured/versions"
	cases := []platformlottery.ConfiguredCase{{Requests: []rules.ConfiguredRequest{{PlayID: "operator_new_play", Options: []string{"lucky"}, Plate: "A", Points: 10, Multiplier: 1}}, Draw: rules.Draw{Regular: []int{1, 3, 7, 4, 5, 6}, Special: []int{2}, Digits: []int{}}, Bet: 10, Prize: 25}}
	draft := platformlottery.ConfiguredDraftInput{Snapshot: engine.Snapshot(), Cases: cases}
	saved := f.call("POST", path, "configured-http-save", auth.AccessToken, "", draft)
	mustStatus(t, saved, 200)
	var version platformlottery.ConfiguredVersion
	managedData(t, saved, &version)
	mustStatus(t, f.call("POST", path, "configured-http-save", auth.AccessToken, "", draft), 200)
	act := func(action, key string, status int) {
		t.Helper()
		r := f.call("POST", path+"/"+version.ID, key, auth.AccessToken, "", platformlottery.ConfiguredAction{Revision: version.Revision, Action: action})
		mustStatus(t, r, status)
		if status == 200 {
			managedData(t, r, &version)
		}
	}
	act("submit", "configured-no-validation", 409)
	act("validate", "configured-validate", 200)
	act("submit", "configured-submit", 200)
	draft.ID = version.ID
	draft.Revision = version.Revision
	mustStatus(t, f.call("POST", path, "configured-edit-submitted", auth.AccessToken, "", draft), 409)
	act("approve", "configured-approve", 200)
	act("publish", "configured-publish", 200)
	mustStatus(t, f.call("POST", path, "configured-brand-spoof", f.token, "", draft), 403)
	mustStatus(t, f.call("POST", "/api/v1/admin/platform-games/"+game.ID+"/configured/versions", "configured-brand-rule", f.token, managedBrand, draft), 404)
	mustStatus(t, f.call("POST", "/api/v1/admin/platform-games/"+game.ID+"/connect", "configured-connect", f.token, managedBrand, map[string]string{}), 201)
	salesPath := "/api/v1/admin/platform-games/" + game.ID + "/configured/sales"
	sales := platformlottery.ConfiguredSales{RuleID: version.ID, Enabled: true, Prices: snap.Prices, MinPoints: 1, MaxPoints: 100, MaxPeriodPoints: 1000, MaxUserPeriodPoints: 500}
	excessive := sales
	excessive.Prices = rules.ConfiguredPrices{}
	priceRaw, _ := json.Marshal(sales.Prices)
	json.Unmarshal(priceRaw, &excessive.Prices)
	excessive.Prices.Plates["A"]["lucky_price"] = "9"
	mustStatus(t, f.call("POST", salesPath, "configured-price-ceiling", f.token, managedBrand, excessive), 400)
	mustStatus(t, f.call("POST", salesPath, "configured-sales", f.token, managedBrand, sales), 200)
	mustStatus(t, f.call("GET", "/api/v1/admin/platform-games/"+game.ID+"/configured/versions", "", f.token, "0199a000-0000-7000-8000-000000000002", nil), 403)
	var period platformlottery.Period
	created := f.call("POST", "/api/v1/platform/platform-games/"+game.ID+"/periods", "configured-period", auth.AccessToken, "", platformlottery.PeriodInput{PeriodNo: "configured-http-1", BetStartAt: time.Now().Add(-time.Hour), BetEndAt: time.Now().Add(time.Hour), DrawAt: time.Now().Add(time.Hour)})
	mustStatus(t, created, 201)
	managedData(t, created, &period)
	tx, e := f.pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	var delta points.Balance
	delta[0][0] = 100
	_, e = (points.Store{DB: f.pool}).Post(ctx, tx, points.Change{BrandID: managedBrand, MemberID: f.memberID, EntryType: "adjustment", ReferenceType: "test_fund", OperationKey: "configured-http-fund", Reason: "test funds", ActorType: "system", RequestID: ids.New(), Delta: delta, Allocation: []points.Allocation{{Source: "recharge", State: "available", Points: 100}}})
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
	if len(q.Lines) != 1 || q.BetPoints != 10 {
		t.Fatal(q)
	}
	in.ActorContext = q.ActorContext
	receipt := f.call("POST", "/api/v1/configured/orders", "configured-http-order", f.userToken, managedBrand, in)
	mustStatus(t, receipt, 201)
	var order betting.ConfiguredOrder
	managedData(t, receipt, &order)
	replay := f.call("POST", "/api/v1/configured/orders", "configured-http-order", f.userToken, managedBrand, in)
	mustStatus(t, replay, 201)
	var same betting.ConfiguredOrder
	managedData(t, replay, &same)
	if same.ID != order.ID {
		t.Fatal("duplicate debit")
	}
	summary := f.call("GET", "/api/v1/betting-summary", "", f.userToken, managedBrand, nil)
	mustStatus(t, summary, 200)
	var totals map[string]string
	managedData(t, summary, &totals)
	if totals["unsettled_points"] != "10" {
		t.Fatal("configured stake omitted from summary", totals)
	}
	in.Requests[0].Points = 11
	mustStatus(t, f.call("POST", "/api/v1/configured/orders", "configured-http-order", f.userToken, managedBrand, in), 409)
	in.Requests[0].Points = 10
	mustStatus(t, f.call("GET", "/api/v1/configured/games/"+game.ID+"/orders", "", f.userToken, managedBrand, nil), 200)
	mustStatus(t, f.call("GET", "/api/v1/admin/platform-games/"+game.ID+"/configured/orders", "", f.token, managedBrand, nil), 200)
	mustStatus(t, f.call("GET", "/api/v1/admin/platform-games/"+game.ID+"/configured/orders", "", f.token, "0199a000-0000-7000-8000-000000000002", nil), 403)
	mustStatus(t, f.call("GET", "/api/v1/configured/orders", "", f.userToken, managedBrand, nil), 200)
	mustStatus(t, f.call("GET", "/api/v1/configured/orders?offset=-1", "", f.userToken, managedBrand, nil), 400)
	mustStatus(t, f.call("GET", "/api/v1/b/harbor/configured/orders", "", f.userToken, "", nil), 401)
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
	if _, e = f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='platform_game.write.platform'`, platformRole); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("POST", path, "configured-http-save", auth.AccessToken, "", draft), 403)
	if _, e = f.pool.Exec(ctx, `UPDATE sessions SET revoked_at=clock_timestamp() WHERE member_id=$1`, f.memberID); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("POST", "/api/v1/configured/orders", "configured-http-order", f.userToken, managedBrand, in), 401)
	mustStatus(t, f.call("GET", "/api/v1/configured/orders", "", f.userToken, managedBrand, nil), 401)
}
