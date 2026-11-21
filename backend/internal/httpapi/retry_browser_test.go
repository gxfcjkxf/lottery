package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
)

// Runs the production HTTP handler against an owned database/schema. Browser
// tests discard committed HTTP responses; these assertions inspect real rows.
func TestCommittedResponseRetryBrowserFixture(t *testing.T) {
	file := os.Getenv("RETRY_REAL_FIXTURE_FILE")
	if file == "" {
		t.Skip("opt-in real browser fixture")
	}
	target, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil || !strings.HasPrefix(target.Path, "/lottery_test_retry_") {
		t.Fatal("dedicated lottery_test_retry_* database required")
	}
	f := pointsFixture(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := f.pool.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	exec(`UPDATE brands SET name='Retry verification brand' WHERE id=$1`, managedBrand)
	exec(`UPDATE brands SET name='Retry isolation brand' WHERE id=$1`, pointsBrandB)
	exec(`UPDATE admin_accounts SET username='retry_brand' WHERE id=$1`, f.root)
	exec(`INSERT INTO role_permissions(role_id,permission_key) SELECT ar.role_id,p.key FROM admin_account_roles ar CROSS JOIN permissions p WHERE ar.account_id=$1 AND p.key LIKE '%.brand' ON CONFLICT DO NOTHING`, f.root)
	exec(`UPDATE brand_recharge_quotas SET available=1000 WHERE brand_id=$1`, managedBrand)
	op := securityOperator(t, f.managementHTTP)
	exec(`UPDATE admin_accounts SET username='retry_platform' WHERE id=$1`, op.root)
	role := grantPlatformPermission(t, op, "brand.view.platform", "recharge.view.platform", "recharge_quota.write.platform", "admin.view.platform", "admin.write.platform", "role.view.platform", "platform_game.view.platform", "platform_game.write.platform", "platform_settings.view.platform")
	exec(`UPDATE roles SET name='Retry verification role' WHERE id=$1`, role)

	create := f.call("POST", "/api/v1/admin/agents", "retry-fixture-agent", f.token, managedBrand, agency.CreateInput{PolicyVersion: 1, Username: "retry_agent", Password: "retry-test-password-2026", Config: agency.NodeConfig{Ratio: "0", Mode: new("loss"), Status: "active", CanCreateChildren: true}})
	mustStatus(t, create, 201)
	var agent agency.Node
	managedData(t, create, &agent)
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var delta points.Balance
	delta[0][0] = 10000
	_, err = (points.Store{DB: f.pool}).Post(ctx, tx, points.Change{BrandID: managedBrand, MemberID: agent.MemberID, EntryType: "recharge", ReferenceType: "retry_test_fixture", OperationKey: "retry-agent-fund", ActorType: "system", RequestID: ids.New(), Delta: delta, Allocation: []points.Allocation{{Source: "recharge", State: "available", Points: 10000}}})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../rules/config/examples/lucky_group_preview.json")
	if err != nil {
		t.Fatal(err)
	}
	engine, err := rules.RestoreConfiguredEngine(raw)
	if err != nil {
		t.Fatal(err)
	}
	var snap rules.ConfiguredSnapshot
	if err = json.Unmarshal(engine.Snapshot(), &snap); err != nil {
		t.Fatal(err)
	}
	result := op.call("POST", "/api/v1/platform/platform-games", "retry-fixture-game", op.token, "", platformlottery.GameInput{Code: "retry_verification", Name: "Retry verification game", Timezone: "UTC", Model: snap.Catalog.Model})
	mustStatus(t, result, 201)
	var game platformlottery.Game
	managedData(t, result, &game)
	result = op.call("POST", "/api/v1/platform/platform-games/"+game.ID+"/configured/rules", "retry-fixture-rules", op.token, "", platformlottery.ConfiguredRulesInput{Snapshot: engine.Snapshot()})
	mustStatus(t, result, 200)
	var version platformlottery.ConfiguredVersion
	managedData(t, result, &version)
	mustStatus(t, f.call("POST", "/api/v1/admin/platform-games/"+game.ID+"/connect", "retry-fixture-connect", f.token, managedBrand, map[string]string{}), 201)
	mustStatus(t, f.call("POST", "/api/v1/admin/platform-games/"+game.ID+"/configured/sales", "retry-fixture-sales", f.token, managedBrand, platformlottery.ConfiguredSales{RuleID: version.ID, Enabled: true, Prices: snap.Prices, MinPoints: 100, MaxPoints: 10000, MaxPeriodPoints: 100000, MaxUserPeriodPoints: 50000}), 200)
	mustStatus(t, op.call("POST", "/api/v1/platform/platform-games/"+game.ID+"/periods", "retry-fixture-period", op.token, "", platformlottery.PeriodInput{PeriodNo: "retry-1", BetStartAt: time.Now().Add(-time.Hour), BetEndAt: time.Now().Add(time.Hour), DrawAt: time.Now().Add(time.Hour)}), 201)
	server := httptest.NewServer(f.http)
	defer server.Close()
	fixture := map[string]any{"url": server.URL, "database": f.pool.Config().ConnConfig.Database, "brand_id": managedBrand, "brand_name": "Retry verification brand", "game_id": game.ID, "platform_role_id": role, "brand_account_id": f.root, "platform_account_id": op.root}
	data, _ := json.Marshal(fixture)
	if err = os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(file)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(8 * time.Minute)
	defer deadline.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("real retry browser fixture timed out")
		case <-ticker.C:
			if _, err = os.Stat(file + ".stop"); err == nil {
				_ = os.Remove(file + ".stop")
				if _, err = os.Stat(file + ".browser-passed"); err != nil {
					t.Log("browser failed; cleanup only")
					return
				}
				assertCount := func(query string, want int, args ...any) {
					t.Helper()
					var count int
					if e := f.pool.QueryRow(ctx, query, args...).Scan(&count); e != nil || count != want {
						t.Fatalf("SQL verification: want %d got %d: %s: %v", want, count, query, e)
					}
				}
				assertCount(`SELECT count(*) FROM global_users WHERE username='retry_member'`, 1)
				assertCount(`SELECT count(*) FROM brand_members m JOIN global_users u ON u.id=m.global_user_id WHERE u.username='retry_member' AND m.brand_id=$1`, 1, managedBrand)
				assertCount(`SELECT count(*) FROM agent_accounts WHERE username='retry_child'`, 1)
				assertCount(`SELECT count(*) FROM admin_accounts WHERE username='retry_operator'`, 1)
				var member string
				if err = f.pool.QueryRow(ctx, `SELECT m.id::text FROM brand_members m JOIN global_users u ON u.id=m.global_user_id WHERE u.username='retry_member' AND m.brand_id=$1`, managedBrand).Scan(&member); err != nil {
					t.Fatal(err)
				}
				memberWallet, err := (points.Store{DB: f.pool}).Read(ctx, managedBrand, member)
				if err != nil || memberWallet.AvailablePoints != 6000 {
					t.Fatalf("member balance want60: %+v %v", memberWallet, err)
				}
				agentWallet, err := (points.Store{DB: f.pool}).Read(ctx, managedBrand, agent.MemberID)
				if err != nil || agentWallet.AvailablePoints != 8000 {
					t.Fatalf("agent balance want80: %+v %v", agentWallet, err)
				}
				assertCount(`SELECT count(*) FROM recharge_orders WHERE brand_id=$1 AND member_id=$2 AND method='manual' AND state='confirmed'`, 1, managedBrand, member)
				assertCount(`SELECT count(*) FROM configured_orders WHERE brand_id=$1 AND member_id=$2`, 1, managedBrand, member)
				assertCount(`SELECT count(*) FROM point_ledger_entries WHERE brand_id=$1 AND member_id=$2`, 3, managedBrand, member)
				assertCount(`SELECT count(*) FROM point_ledger_entries WHERE brand_id=$1 AND entry_type='agent_transfer_debit'`, 1, managedBrand)
				assertCount(`SELECT count(*) FROM point_ledger_entries WHERE brand_id=$1 AND entry_type='agent_transfer_credit'`, 1, managedBrand)
				assertCount(`SELECT count(*) FROM brand_recharge_quota_entries WHERE brand_id=$1`, 2, managedBrand)
				assertCount(`SELECT count(*) FROM point_ledger_entries WHERE brand_id=$1`, 0, pointsBrandB)
				var quota int64
				if err = f.pool.QueryRow(ctx, `SELECT available FROM brand_recharge_quotas WHERE brand_id=$1`, managedBrand).Scan(&quota); err != nil || quota != 1050 {
					t.Fatal("quota want1050", quota, err)
				}
				evidence := map[string]any{"member_points": "60", "agent_points": "80", "brand_quota": "1050", "member_ledger_entries": 3, "manual_recharges": 1, "bets": 1, "agent_transfer_debits": 1, "agent_transfer_credits": 1, "quota_entries": 2, "created_member": 1, "created_subagent": 1, "created_platform_account": 1, "foreign_brand_ledger_entries": 0}
				raw, _ := json.MarshalIndent(evidence, "", "  ")
				if err = os.WriteFile(file+".verified.json", raw, 0600); err != nil {
					t.Fatal(err)
				}
				t.Log("Committed-response retries: one account per creation; one grant, one recharge, one transfer and one bet; exact balances and foreign-brand isolation verified")
				return
			}
		}
	}
}
