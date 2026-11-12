package httpapi

import (
	"context"
	"encoding/json"
	"testing"
)

func TestPlatformOverviewPermissionAndPresence(t *testing.T) {
	f := pointsFixture(t)
	ctx := context.Background()
	path := "/api/v1/platform/overview"
	mustStatus(t, f.call("GET", path, "", "", "", nil), 401)
	mustStatus(t, f.call("GET", path, "", f.token, "", nil), 403)
	mustStatus(t, f.call("GET", "/api/v1/admin/overview", "", f.token, managedBrand, nil), 403)
	if _, err := f.pool.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	token := platformAdminToken(t, f.managementHTTP)
	mustStatus(t, f.call("GET", path, "", token, "", nil), 403)
	role := grantPlatformPermission(t, f.managementHTTP, "overview.view.platform")
	mustStatus(t, f.call("GET", path, "", token, managedBrand, nil), 400)
	mustStatus(t, f.call("GET", path+"?brand_id="+managedBrand, "", token, "", nil), 400)
	read := func() map[string]any {
		r := f.call("GET", path, "", token, "", nil)
		mustStatus(t, r, 200)
		var data map[string]any
		managedData(t, r, &data)
		return data
	}
	data := read()
	if data["online_members"] != "1" || len(data["days"].([]any)) != 7 || data["unsettled_points"] != "0" {
		t.Fatalf("overview: %+v", data)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE session_activity SET last_seen_at=clock_timestamp()-interval '6 minutes'`); err != nil {
		t.Fatal(err)
	}
	if read()["online_members"] != "0" {
		t.Fatal("idle session counted online")
	}
	mustStatus(t, f.call("GET", "/api/v1/me", "", f.userToken, managedBrand, nil), 200)
	if read()["online_members"] != "1" {
		t.Fatal("member heartbeat did not restore online presence")
	}
	if _, err := f.pool.Exec(ctx, `UPDATE sessions SET revoked_at=clock_timestamp() WHERE user_id IS NOT NULL`); err != nil {
		t.Fatal(err)
	}
	if read()["online_members"] != "0" {
		t.Fatal("revoked session counted online")
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='overview.view.platform'`, role); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("GET", path, "", token, "", nil), 403)
}

func TestPlatformOverviewAllEnginesAndLocalDays(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	// Shadow only the aggregation's sources inside this transaction. This tests
	// PostgreSQL arithmetic and grouping without bypassing financial guards.
	statements := []string{
		`CREATE TEMP TABLE platform_settings(id boolean,timezone text); INSERT INTO platform_settings VALUES(true,'Pacific/Kiritimati')`,
		`CREATE TEMP TABLE global_users(id text,status text,created_at timestamptz); INSERT INTO global_users VALUES('u','active',now()),('old','disabled',now()-interval '1 day'),('deleted','deleted',now())`,
		`CREATE TEMP TABLE brands(id text,name text,status text); INSERT INTO brands VALUES('a','Alpha','active'),('b','Beta','active')`,
		`CREATE TEMP TABLE sessions(id text,user_id text,member_id text,brand_id text,revoked_at timestamptz,expires_at timestamptz)`,
		`CREATE TEMP TABLE session_activity(session_id text,last_seen_at timestamptz)`,
		`CREATE TEMP TABLE brand_members(id text,brand_id text,global_user_id text,status text,joined_at timestamptz)`,
		`CREATE TEMP TABLE point_accounts(id text,brand_id text,brand_member_id text); INSERT INTO point_accounts VALUES('p1','a','m1'),('p3','b','m3'),('p4','a','m4'),('p5','a','m5')`,
		`CREATE TEMP TABLE point_buckets(account_id text,brand_id text,state text,points numeric); INSERT INTO point_buckets VALUES('p1','a','available',9007199254740993),('p3','b','system_frozen',30),('p1','a','manual_frozen',20),('p1','a','withdrawal',50),('p4','a','available',66313),('p4','a','manual_frozen',111),('p5','a','available',28),('p5','a','system_frozen',222)`,
		`CREATE TEMP TABLE withdrawal_orders(state text,points numeric); INSERT INTO withdrawal_orders VALUES('reviewing',50),('paid',200)`,
		`INSERT INTO sessions VALUES('s1','u','m1','a',NULL,now()+interval '1 day'),('s2','u','m2','b',NULL,now()+interval '1 day'),('s3','old','m3','b',NULL,now()+interval '1 day'),('s4','u','m1','a',NULL,now()+interval '1 day'); INSERT INTO session_activity VALUES('s1',now()),('s2',now()),('s3',now()),('s4',now()); INSERT INTO brand_members VALUES('m1','a','u','normal',now()),('m2','b','u','normal',now()-interval '1 day'),('m3','b','old','normal',now()),('m4','a','deleted','normal',now()),('m5','a','old','cancelled',now())`,
		`CREATE TEMP TABLE betting_financial_facts(brand_id text,global_user_id text,total_points numeric,consumed_points numeric,prize_points numeric,status text,placed_at timestamptz,settled_at timestamptz,refund_points numeric DEFAULT 0,snapshot jsonb DEFAULT '{}')`,
		`INSERT INTO betting_financial_facts VALUES('a','u',9007199254740993,0,0,'placed',now(),NULL,0,'{}'),('a','u',10,10,25,'won',now()-interval '2 days',now(),0,'{}'),('b','u',20,0,0,'placed',now(),NULL,0,'{}'),('b','old',100,0,0,'lost',now(),now(),100,'{}'),('a','u',100,0,0,'lost',now(),now(),100,'{}'),('a','u',7,0,0,'placed',now()-interval '8 days',NULL,0,'{}'),('a','u',10,30,0,'lost',now(),now(),0,'{"catalog":{"funding":"reserve"}}'),('b','u',10,0,40,'won',now(),now(),0,'{"catalog":{"funding":"reserve"}}'),('b','u',20,20,0,'lost',now(),now(),0,'{}'),('b','u',10,190,0,'lost',now(),now(),0,'{"catalog":{"funding":"reserve"}}'),('a','u',10,0,50,'won',now(),now(),0,'{"catalog":{"funding":"reserve"}}'),('a','u',30,20,20,'won',now(),now(),10,'{}')`,
	}
	for _, statement := range statements {
		if _, err = tx.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	read := func() map[string]any {
		var raw []byte
		if err := tx.QueryRow(ctx, platformOverviewSQL).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	out := read()
	for key, wanted := range map[string]string{"total_members": "3", "online_members": "2", "unsettled_points": "9007199254741020", "available_points": "9007199254740993", "frozen_points": "50", "withdrawal_points": "50", "withdrawal_count": "1"} {
		if out[key] != wanted {
			t.Fatalf("%s=%v, want %s", key, out[key], wanted)
		}
	}
	today := out["days"].([]any)[6].(map[string]any)
	for key, wanted := range map[string]string{"stake_points": "9007199254741303", "net_points": "135", "refund_points": "210", "bettor_count": "3", "order_count": "10", "new_members": "2"} {
		if today[key] != wanted {
			t.Fatalf("today %s=%v, want %s", key, today[key], wanted)
		}
	}
	if out["days"].([]any)[5].(map[string]any)["new_members"] != "1" {
		t.Fatal("new members must use brand join time")
	}
	// Exact local midnight belongs to today; one microsecond before belongs to yesterday.
	if _, err := tx.Exec(ctx, `TRUNCATE betting_financial_facts; INSERT INTO betting_financial_facts(brand_id,global_user_id,total_points,consumed_points,prize_points,status,placed_at,settled_at) VALUES ('a','u',11,11,0,'lost',date_trunc('day',now() AT TIME ZONE 'Pacific/Kiritimati') AT TIME ZONE 'Pacific/Kiritimati',now()),('b','u',13,13,0,'lost',(date_trunc('day',now() AT TIME ZONE 'Pacific/Kiritimati') AT TIME ZONE 'Pacific/Kiritimati')-interval '1 microsecond',now())`); err != nil {
		t.Fatal(err)
	}
	days := read()["days"].([]any)
	if days[6].(map[string]any)["stake_points"] != "11" || days[5].(map[string]any)["stake_points"] != "13" {
		t.Fatal(days)
	}
}
