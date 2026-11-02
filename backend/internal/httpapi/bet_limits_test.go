package httpapi

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"testing"
)

func TestBetLimitsHTTPScopesReplayAndSalesIsolation(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	game, rule := ids.New(), ids.New()
	queries := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO platform_games(id,code,name,model,timezone,created_by) VALUES($1,'limit_game','Limit game','{"type":"digits","digits":3,"allow_repeat":true}','UTC',$2)`, []any{game, f.root}},
		{`INSERT INTO brand_platform_games(brand_id,game_id,connected_by) VALUES($1,$2,$3)`, []any{managedBrand, game, f.root}},
		{`INSERT INTO configured_rule_versions(id,game_id,snapshot,snapshot_hash,cases,status,created_by) VALUES($1,$2,'{}','limit-test','[]','published',$3)`, []any{rule, game, f.root}},
		{`INSERT INTO configured_sales(brand_id,game_id,version,rule_id,enabled,prices,min_points,max_points,max_period_points,max_user_period_points) VALUES($1,$2,1,$3,true,'{"unchanged":"2"}',1,100,1000,500)`, []any{managedBrand, game, rule}},
	}
	for _, q := range queries {
		if _, e := f.pool.Exec(ctx, q.sql, q.args...); e != nil {
			t.Fatal(e)
		}
	}
	path := "/api/v1/admin/platform-games/" + game + "/bet-limits"
	// No grants: both catalog and settings are inaccessible.
	mustStatus(t, f.call("GET", "/api/v1/admin/filter-options?resource=bet_policy&kind=game&limit=100&offset=0", "", f.token, managedBrand, nil), 403)
	mustStatus(t, f.call("GET", "/api/v1/admin/bet-limits", "", f.token, managedBrand, nil), 403)
	var role string
	if err := f.pool.QueryRow(ctx, `SELECT role_id::text FROM admin_account_roles WHERE account_id=$1`, f.root).Scan(&role); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"bet_policy.view.brand", "bet_policy.write.brand"} {
		if _, e := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2) ON CONFLICT DO NOTHING`, role, p); e != nil {
			t.Fatal(e)
		}
	}
	list := f.call("GET", "/api/v1/admin/filter-options?resource=bet_policy&kind=game&limit=100&offset=0", "", f.token, managedBrand, nil)
	mustStatus(t, list, 200)
	var catalog struct {
		Items []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}
	managedData(t, list, &catalog)
	if len(catalog.Items) != 1 || catalog.Items[0].ID != game {
		t.Fatal("must list connected platform games", catalog)
	}
	var in platformlottery.BetLimits
	read := f.call("GET", path, "", f.token, managedBrand, nil)
	mustStatus(t, read, 200)
	managedData(t, read, &in)
	readCatalog := func() []platformlottery.BetLimitGame {
		t.Helper()
		response := f.call("GET", "/api/v1/admin/bet-limits", "", f.token, managedBrand, nil)
		mustStatus(t, response, 200)
		var result struct {
			Items []platformlottery.BetLimitGame `json:"items"`
		}
		managedData(t, response, &result)
		return result.Items
	}
	if items := readCatalog(); len(items) != 1 || items[0].ID != game || items[0].Configured {
		t.Fatal("initial sales must not count as manually configured limits", items)
	}
	mustStatus(t, f.call("GET", "/api/v1/admin/bet-limits", "", f.token, pointsBrandB, nil), 403)
	mustStatus(t, f.call("GET", path, "", f.token, pointsBrandB, nil), 403)
	in.MaxPoints = 20
	in.MaxPeriodPoints = 200
	in.MaxUserPeriodPoints = 100
	saved := f.call("POST", path, "limit-write-001", f.token, managedBrand, in)
	mustStatus(t, saved, 200)
	var out, replay platformlottery.BetLimits
	managedData(t, saved, &out)
	again := f.call("POST", path, "limit-write-001", f.token, managedBrand, in)
	mustStatus(t, again, 200)
	managedData(t, again, &replay)
	if out.Version != 2 || out != replay || out.MaxPoints != 20 {
		t.Fatal("idempotent receipt", out, replay)
	}
	if items := readCatalog(); len(items) != 1 || !items[0].Configured {
		t.Fatal("saved limits must remain configured after reloading", items)
	}
	if _, e := f.pool.Exec(ctx, `INSERT INTO brand_platform_games(brand_id,game_id,connected_by) VALUES($1,$2,$3)`, pointsBrandB, game, f.root); e != nil {
		t.Fatal(e)
	}
	otherItems, e := (platformlottery.Service{DB: f.pool}).BetLimitGames(ctx, pointsBrandB)
	if e != nil || len(otherItems) != 1 || otherItems[0].Configured {
		t.Fatal("another brand must not inherit the configured marker", otherItems, e)
	}
	mustStatus(t, f.call("POST", path, "limit-write-stale", f.token, managedBrand, in), 409)
	changed := in
	changed.MaxPoints = 21
	mustStatus(t, f.call("POST", path, "limit-write-001", f.token, managedBrand, changed), 409)
	invalid := out
	invalid.MaxUserPeriodPoints = 1
	mustStatus(t, f.call("POST", path, "limit-invalid-001", f.token, managedBrand, invalid), 400)
	spoof := out
	spoof.BrandID = pointsBrandB
	mustStatus(t, f.call("POST", path, "limit-spoof-001", f.token, managedBrand, spoof), 400)
	// The service returns no record from the other brand regardless of the caller's grants.
	svc := platformlottery.Service{DB: f.pool}
	if _, e := svc.BetLimits(ctx, pointsBrandB, game); e == nil {
		t.Fatal("cross-brand limits exposed")
	}
	var version int64
	var gotRule string
	var enabled bool
	var prices []byte
	if e := f.pool.QueryRow(ctx, `SELECT version,rule_id::text,enabled,prices FROM configured_sales WHERE brand_id=$1 AND game_id=$2`, managedBrand, game).Scan(&version, &gotRule, &enabled, &prices); e != nil {
		t.Fatal(e)
	}
	var price map[string]string
	if json.Unmarshal(prices, &price) != nil || version != 2 || gotRule != rule || !enabled || price["unchanged"] != "2" {
		t.Fatal("non-limit sales settings changed", version, gotRule, enabled, string(prices))
	}
	if _, e := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='bet_policy.write.brand'`, role); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("POST", path, "limit-revoked-001", f.token, managedBrand, out), 403)
}
