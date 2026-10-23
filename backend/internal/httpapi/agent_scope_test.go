package httpapi

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
)

func TestIndependentAgentsLineScope(t *testing.T) {
	f := managedFixture(t)
	agentGrantRoot(t, f)
	type agent struct {
		node   agency.Node
		token  string
		member string
	}
	create := func(name string, parent *agent) agent {
		t.Helper()
		in := agency.CreateInput{PolicyVersion: 1, Username: name, Password: "agent123", Config: agency.NodeConfig{Ratio: "0", Status: "active", CanCreateChildren: true}}
		path, token, brand := "/api/v1/admin/agents", f.token, managedBrand
		if parent != nil {
			path = "/api/v1/b/aurora/agent/children"
			token = parent.token
			brand = ""
		} else {
			in.Config.Mode = new("loss")
		}
		r := f.call("POST", path, "create-"+name, token, brand, in)
		mustStatus(t, r, 201)
		var a agent
		managedData(t, r, &a.node)
		login := f.call("POST", "/api/v1/b/aurora/agent/login", "", "", "", identity.AgentLoginInput{Username: name, Password: "agent123"})
		mustStatus(t, login, 200)
		for _, c := range login.Result().Cookies() {
			if c.Name == agentCookieName(managedBrand) {
				a.token = c.Value
			}
		}
		r = f.call("POST", "/api/v1/b/aurora/agent/members", "member-"+name, a.token, "", identity.OperatorInput{Username: "member_" + name, Password: "member123"})
		mustStatus(t, r, 201)
		var m struct {
			ID string `json:"member_id"`
		}
		managedData(t, r, &m)
		a.member = m.ID
		return a
	}
	root := create("scope_root", nil)
	child := create("scope_child", &root)
	grand := create("scope_grand", &child)
	sibling := create("scope_sibling", &root)
	outsider := create("scope_outside", nil)
	for _, tc := range []struct {
		who          agent
		scope, count string
	}{{root, "direct", "1"}, {root, "line", "4"}, {child, "line", "2"}, {grand, "line", "1"}, {sibling, "line", "1"}, {outsider, "line", "1"}} {
		r := f.call("GET", "/api/v1/b/aurora/agent/members?scope="+tc.scope+"&limit=1", "", tc.who.token, "", nil)
		mustStatus(t, r, 200)
		var data struct {
			Total   string            `json:"total_count"`
			Items   []json.RawMessage `json:"items"`
			Summary map[string]string `json:"summary"`
		}
		managedData(t, r, &data)
		if data.Total != tc.count || data.Summary["member_count"] != tc.count || len(data.Items) != 1 {
			t.Fatalf("scope %s %+v", tc.scope, data)
		}
	}
	for _, suffix := range []string{"summary", "bets"} {
		path := "/api/v1/b/aurora/agent/members/" + grand.member + "/" + suffix
		mustStatus(t, f.call("GET", path+"?scope=line", "", root.token, "", nil), 200)
		mustStatus(t, f.call("GET", path+"?scope=direct", "", root.token, "", nil), 404)
		mustStatus(t, f.call("GET", path+"?scope=line", "", sibling.token, "", nil), 404)
		mustStatus(t, f.call("GET", path+"?scope=line", "", outsider.token, "", nil), 404)
		mustStatus(t, f.call("GET", "/api/v1/b/aurora/agent/members/"+root.member+"/"+suffix+"?scope=line", "", child.token, "", nil), 404)
		mustStatus(t, f.call("GET", "/api/v1/b/harbor/agent/members/"+grand.member+"/"+suffix+"?scope=line", "", root.token, "", nil), 401)
	}
	for _, q := range []string{"scope=other", "scope=", "scope=direct&scope=line"} {
		mustStatus(t, f.call("GET", "/api/v1/b/aurora/agent/members?"+q, "", root.token, "", nil), 400)
	}
	// Local order tables exercise all protocol arithmetic without ledger bypasses.
	ctx := context.Background()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	statements := []string{
		`CREATE TEMP TABLE bet_orders(brand_id uuid,brand_member_id uuid,total_points bigint,prize_points bigint,status text,settled_at timestamptz) ON COMMIT DROP`,
		`CREATE TEMP TABLE platform_mark_six_orders(brand_id uuid,member_id uuid,total_points bigint,prize_points bigint,refund_points bigint,status text,settled_at timestamptz) ON COMMIT DROP`,
		`CREATE TEMP TABLE platform_canada_orders(brand_id uuid,member_id uuid,total_points bigint,prize_points bigint,refund_points bigint,lost_points bigint,profile text,status text,settled_at timestamptz) ON COMMIT DROP`,
		`CREATE TEMP TABLE configured_orders(brand_id uuid,member_id uuid,total_points bigint,status text,snapshot jsonb,result jsonb,settled_at timestamptz) ON COMMIT DROP`,
	}
	for _, q := range statements {
		if _, err = tx.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := tx.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	exec(`INSERT INTO bet_orders VALUES($1,$2,9007199254740993,0,'placed',NULL),($1,$2,10,25,'won',now())`, managedBrand, root.member)
	exec(`INSERT INTO platform_mark_six_orders VALUES($1,$2,100,0,100,'settled',now())`, managedBrand, child.member)
	exec(`INSERT INTO platform_canada_orders VALUES($1,$2,10,0,0,30,'canada_niuniu','settled',now())`, managedBrand, grand.member)
	exec(`INSERT INTO configured_orders VALUES($1,$2,10,'settled','{"catalog":{"funding":"reserve"}}','{"prize_points":"50","consumed_points":"0","refund_points":"200"}',now())`, managedBrand, grand.member)
	exec(`INSERT INTO bet_orders VALUES($1,$2,999,1999,'won',now())`, managedBrand, outsider.member)
	// Same member ID in a different brand must never enter this brand's totals.
	exec(`INSERT INTO bet_orders VALUES($1,$2,999,1999,'won',now())`, "22222222-2222-4222-8222-222222222222", grand.member)
	// Multiple point buckets and multiple orders must not multiply the wallet.
	exec(`CREATE TEMP TABLE point_accounts(brand_id uuid,id uuid,brand_member_id uuid) ON COMMIT DROP`)
	exec(`CREATE TEMP TABLE point_buckets(brand_id uuid,account_id uuid,points bigint,state text) ON COMMIT DROP`)
	exec(`INSERT INTO point_accounts VALUES($1,$2,$2),($1,$3,$3)`, managedBrand, root.member, grand.member)
	exec(`INSERT INTO point_buckets VALUES($1,$2,7,'available'),($1,$2,8,'available'),($1,$3,11,'available'),($1,$3,99,'frozen')`, managedBrand, root.member, grand.member)
	for _, tc := range []struct {
		scope, q, count, bets, points, net, stake, pending string
		offset                                             int
	}{
		{"line", "", "4", "5", "26", "35", "9007199254741123", "9007199254740993", 0},
		{"line", "", "4", "5", "26", "35", "9007199254741123", "9007199254740993", 3},
		{"direct", "", "1", "2", "15", "15", "9007199254741003", "9007199254740993", 0},
		{"line", "member_scope_grand", "1", "2", "11", "20", "20", "0", 0},
	} {
		var raw []byte
		if err = tx.QueryRow(ctx, agentMembersSQL, managedBrand, root.node.ID, 1, tc.offset, tc.q, tc.scope).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var out struct {
			Summary map[string]string `json:"summary"`
		}
		if err = json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		expected := map[string]string{"member_count": tc.count, "bet_count": tc.bets, "available_points": tc.points, "net_points": tc.net, "stake_points": tc.stake, "unsettled_points": tc.pending}
		for key, want := range expected {
			if out.Summary[key] != want {
				t.Fatalf("scope=%s q=%s offset=%d %s: got %s want %s", tc.scope, tc.q, tc.offset, key, out.Summary[key], want)
			}
		}
	}
}
