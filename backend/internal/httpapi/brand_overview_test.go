package httpapi

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"testing"
)

func TestBrandOverviewHTTPPermissionAndHostScope(t *testing.T) {
	f := pointsFixture(t)
	ctx := context.Background()
	path := "/api/v1/admin/overview"
	mustStatus(t, f.call("GET", path, "", "", managedBrand, nil), 401)
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 403)
	if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,'overview.view.brand' FROM admin_account_roles WHERE account_id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("GET", path, "", f.token, "", nil), 400)
	mustStatus(t, f.call("GET", path+"?brand_id="+managedBrand, "", f.token, managedBrand, nil), 400)
	mustStatus(t, f.call("GET", path, "", f.userToken, managedBrand, nil), 401)
	response := f.call("GET", path, "", f.token, managedBrand, nil)
	mustStatus(t, response, 200)
	var out map[string]any
	managedData(t, response, &out)
	if out["brand_id"] != managedBrand || out["online_members"] != "1" || out["brands"] != nil {
		t.Fatal(out)
	}
	// Even a staff account granted both brands cannot query another brand's
	// dashboard from this brand's administrative host.
	if _, err := f.pool.Exec(ctx, `INSERT INTO admin_brand_scopes(account_id,brand_id) VALUES($1,$2)`, f.root, pointsBrandB); err != nil {
		t.Fatal(err)
	}
	role := ids.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO roles(id,brand_id,code,name) VALUES($1,$2,'second_overview','Second brand overview')`, []any{role, pointsBrandB}},
		{`INSERT INTO role_permissions(role_id,permission_key) VALUES($1,'overview.view.brand')`, []any{role}},
		{`INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, []any{f.root, role}},
	} {
		if _, err := f.pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	mustStatus(t, f.call("GET", path, "", f.token, pointsBrandB, nil), 403)
	mustStatus(t, f.call("GET", "/api/v1/platform/overview", "", f.token, "", nil), 403)
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE permission_key='overview.view.brand' AND role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1)`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 403)
}

func TestBrandOverviewSQLAllSourcesStayWithinBrand(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	statements := []string{
		`CREATE TEMP TABLE platform_settings(id boolean,timezone text); INSERT INTO platform_settings VALUES(true,'Pacific/Kiritimati')`,
		`CREATE TEMP TABLE global_users(id text,status text,created_at timestamptz); INSERT INTO global_users VALUES('shared','active',now()-interval '50 days'),('other','active',now()),('deleted','deleted',now())`,
		`CREATE TEMP TABLE brands(id text,name text,status text); INSERT INTO brands VALUES('a','Alpha','active'),('b','Beta','active')`,
		`CREATE TEMP TABLE brand_members(id text,brand_id text,global_user_id text,status text,terms_accepted boolean,joined_at timestamptz); INSERT INTO brand_members VALUES('ma','a','shared','normal',true,now()-interval '1 day'),('mb','b','shared','normal',true,now()),('mo','b','other','normal',true,now()),('md','a','deleted','cancelled',true,now()),('mc','a','other','cancelled',true,now())`,
		`CREATE TEMP TABLE sessions(id text,user_id text,member_id text,brand_id text,revoked_at timestamptz,expires_at timestamptz); INSERT INTO sessions VALUES('sa','shared','ma','a',NULL,now()+interval '1 day'),('sb','shared','mb','b',NULL,now()+interval '1 day')`,
		`CREATE TEMP TABLE session_activity(session_id text,last_seen_at timestamptz); INSERT INTO session_activity VALUES('sa',now()-interval '6 minutes'),('sb',now())`,
		`CREATE TEMP TABLE point_accounts(id text,brand_id text,brand_member_id text); INSERT INTO point_accounts VALUES('pa','a','ma'),('pb','b','mb'),('pd','a','md'),('pc','a','mc')`,
		`CREATE TEMP TABLE point_buckets(account_id text,brand_id text,state text,points numeric); INSERT INTO point_buckets VALUES('pa','a','available',9007199254740993),('pa','a','manual_frozen',10),('pa','a','system_frozen',20),('pb','b','available',999999),('pb','b','system_frozen',999999),('pd','a','available',66313),('pd','a','manual_frozen',111),('pc','a','available',28),('pc','a','system_frozen',222)`,
		`CREATE TEMP TABLE withdrawal_orders(brand_id text,state text,points numeric); INSERT INTO withdrawal_orders VALUES('a','reviewing',50),('b','reviewing',999999),('a','paid',200)`,
		`CREATE TEMP TABLE betting_financial_facts(brand_id text,global_user_id text,total_points numeric,consumed_points numeric,prize_points numeric,status text,placed_at timestamptz,settled_at timestamptz,refund_points numeric DEFAULT 0,snapshot jsonb DEFAULT '{}')`,
		`INSERT INTO betting_financial_facts VALUES('a','shared',9007199254740993,0,0,'placed',now(),NULL,0,'{}'),('a','shared',10,10,25,'won',now(),now(),0,'{}'),('a','shared',100,0,0,'lost',now(),now(),100,'{}'),('a','shared',10,30,0,'lost',now(),now(),0,'{"catalog":{"funding":"reserve"}}'),('a','shared',10,0,50,'won',now(),now(),0,'{"catalog":{"funding":"reserve"}}'),('a','shared',30,20,20,'won',now(),now(),10,'{}'),('b','other',999999,0,0,'placed',now(),NULL,0,'{}')`,
	}
	for _, q := range statements {
		if _, err := tx.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	read := func() map[string]any {
		var raw []byte
		if err := tx.QueryRow(ctx, brandOverviewSQL, "a").Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	out := read()
	for key, want := range map[string]string{"brand_id": "a", "total_members": "1", "online_members": "0", "available_points": "9007199254740993", "frozen_points": "30", "withdrawal_points": "50", "withdrawal_count": "1", "unsettled_points": "9007199254740993"} {
		if out[key] != want {
			t.Fatalf("%s=%v want %s", key, out[key], want)
		}
	}
	days := out["days"].([]any)
	today := days[6].(map[string]any)
	for key, want := range map[string]string{"stake_points": "9007199254741153", "net_points": "-35", "refund_points": "110", "order_count": "6", "bettor_count": "1", "new_members": "0"} {
		if today[key] != want {
			t.Fatalf("today %s=%v want %s", key, today[key], want)
		}
	}
	if days[5].(map[string]any)["new_members"] != "1" {
		t.Fatal("new member must use brand join time", days)
	}
	if _, err := tx.Exec(ctx, `UPDATE session_activity SET last_seen_at=now() WHERE session_id='sa'`); err != nil {
		t.Fatal(err)
	}
	if read()["online_members"] != "1" {
		t.Fatal("current brand activity not counted")
	}
	if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at=now() WHERE id='sa'`); err != nil {
		t.Fatal(err)
	}
	if read()["online_members"] != "0" {
		t.Fatal("revoked local session counted")
	}
	// The same platform-local midnight boundary applies to this brand's orders.
	if _, err := tx.Exec(ctx, `TRUNCATE betting_financial_facts; INSERT INTO betting_financial_facts(brand_id,global_user_id,total_points,consumed_points,prize_points,status,placed_at,settled_at) VALUES ('a','shared',11,11,0,'lost',date_trunc('day',now() AT TIME ZONE 'Pacific/Kiritimati') AT TIME ZONE 'Pacific/Kiritimati',now()),('a','shared',13,13,0,'lost',(date_trunc('day',now() AT TIME ZONE 'Pacific/Kiritimati') AT TIME ZONE 'Pacific/Kiritimati')-interval '1 microsecond',now())`); err != nil {
		t.Fatal(err)
	}
	days = read()["days"].([]any)
	if days[6].(map[string]any)["stake_points"] != "11" || days[5].(map[string]any)["stake_points"] != "13" {
		t.Fatal(days)
	}
}
