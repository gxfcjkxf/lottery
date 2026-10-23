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
		`CREATE TEMP TABLE brand_members(id text,brand_id text,global_user_id text,status text,terms_accepted boolean,joined_at timestamptz)`,
		`CREATE TEMP TABLE point_buckets(state text,points numeric); INSERT INTO point_buckets VALUES('available',9007199254740993),('system_frozen',30),('manual_frozen',20),('withdrawal',50)`,
		`CREATE TEMP TABLE withdrawal_orders(state text,points numeric); INSERT INTO withdrawal_orders VALUES('reviewing',50),('paid',200)`,
		`CREATE TEMP TABLE settlement_jobs(state text); INSERT INTO settlement_jobs VALUES('failed'),('completed')`,
		`CREATE TEMP TABLE period_cancellations(state text); INSERT INTO period_cancellations VALUES('failed'),('processing'),('completed')`,
		`CREATE TEMP TABLE bet_orders(brand_id text,global_user_id text,total_points numeric,prize_points numeric,status text,placed_at timestamptz,settled_at timestamptz,cancelled_at timestamptz)`,
		`CREATE TEMP TABLE platform_mark_six_orders(brand_id text,user_id text,total_points numeric,prize_points numeric,refund_points numeric,status text,placed_at timestamptz,settled_at timestamptz)`,
		`CREATE TEMP TABLE platform_canada_orders(brand_id text,user_id text,total_points numeric,prize_points numeric,refund_points numeric,lost_points numeric,profile text,status text,placed_at timestamptz,settled_at timestamptz)`,
		`CREATE TEMP TABLE configured_orders(brand_id text,user_id text,total_points numeric,status text,snapshot jsonb,result jsonb,placed_at timestamptz,settled_at timestamptz)`,
		`INSERT INTO bet_orders VALUES('a','u',9007199254740993,0,'placed',now(),NULL,NULL),('a','u',10,25,'won',now()-interval '2 days',now(),NULL),('b','u',20,0,'abnormal',now(),NULL,NULL),('b','old',100,0,'bet_cancelled',now(),NULL,now())`,
		`INSERT INTO platform_mark_six_orders VALUES('a','u',100,0,100,'settled',now(),now()),('a','u',7,0,0,'placed',now()-interval '8 days',NULL)`,
		`INSERT INTO platform_canada_orders VALUES('a','u',10,0,170,30,'canada_niuniu','settled',now(),now()),('b','u',10,40,200,0,'canada_niuniu','settled',now(),now()),('b','u',20,0,0,0,'canada_28','settled',now(),now())`,
		`INSERT INTO configured_orders VALUES('b','u',10,'settled','{"catalog":{"funding":"reserve"}}','{"prize_points":"0","refund_points":"10","consumed_points":"190"}',now(),now()),('a','u',10,'settled','{"catalog":{"funding":"reserve"}}','{"prize_points":"50","refund_points":"200"}',now(),now()),('a','u',30,'settled','{"catalog":{"funding":"debit"}}','{"prize_points":"20","refund_points":"10"}',now(),now())`,
		`INSERT INTO sessions VALUES('s1','u','m1','a',NULL,now()+interval '1 day'),('s2','u','m2','b',NULL,now()+interval '1 day'),('s3','old','m3','b',NULL,now()+interval '1 day'),('s4','u','m1','a',NULL,now()+interval '1 day'); INSERT INTO session_activity VALUES('s1',now()),('s2',now()),('s3',now()),('s4',now()); INSERT INTO brand_members VALUES('m1','a','u','normal',true,now()),('m2','b','u','normal',true,now()-interval '1 day'),('m3','b','old','normal',true,now()),('m4','a','deleted','normal',true,now())`,
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
	for key, wanted := range map[string]string{"total_members": "3", "online_members": "2", "unsettled_points": "9007199254741020", "available_points": "9007199254740993", "frozen_points": "50", "withdrawal_points": "50", "withdrawal_count": "1", "abnormal_count": "1", "failed_settlement_count": "1", "cancellation_count": "2"} {
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
	if _, err := tx.Exec(ctx, `TRUNCATE bet_orders,platform_mark_six_orders,platform_canada_orders,configured_orders;
 INSERT INTO bet_orders VALUES
 ('a','u',11,0,'lost',date_trunc('day',now() AT TIME ZONE 'Pacific/Kiritimati') AT TIME ZONE 'Pacific/Kiritimati',date_trunc('day',now() AT TIME ZONE 'Pacific/Kiritimati') AT TIME ZONE 'Pacific/Kiritimati',NULL),
 ('b','u',13,0,'lost',(date_trunc('day',now() AT TIME ZONE 'Pacific/Kiritimati') AT TIME ZONE 'Pacific/Kiritimati')-interval '1 microsecond',(date_trunc('day',now() AT TIME ZONE 'Pacific/Kiritimati') AT TIME ZONE 'Pacific/Kiritimati')-interval '1 microsecond',NULL)`); err != nil {
		t.Fatal(err)
	}
	days := read()["days"].([]any)
	if days[6].(map[string]any)["stake_points"] != "11" || days[5].(map[string]any)["stake_points"] != "13" {
		t.Fatal(days)
	}
}
