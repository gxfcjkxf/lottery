package httpapi

import (
	"context"
	"testing"
)

func TestMemberSummaryHTTPIdentityAndEmpty(t *testing.T) {
	f := pointsFixture(t)
	for _, path := range []string{"/api/v1/betting-summary", "/api/v1/b/aurora/betting-summary"} {
		r := f.call("GET", path, "", f.userToken, managedBrand, nil)
		mustStatus(t, r, 200)
		var out map[string]string
		managedData(t, r, &out)
		if len(out) != 4 || out["brand_id"] != managedBrand || out["member_id"] != f.memberID || out["unsettled_points"] != "0" || out["today_net_points"] != "0" {
			t.Fatalf("summary: %+v", out)
		}
		mustStatus(t, f.call("GET", path, "", "", managedBrand, nil), 401)
	}
	mustStatus(t, f.call("GET", "/api/v1/betting-summary", "", "", managedBrand, nil), 401)
	mustStatus(t, f.call("GET", "/api/v1/betting-summary?member_id="+f.memberID, "", f.userToken, managedBrand, nil), 400)
	mustStatus(t, f.call("GET", "/api/v1/b/harbor/betting-summary", "", f.userToken, "", nil), 401)
}

func TestMemberSummaryAllEnginesExactNetAndBrandScope(t *testing.T) {
	f := pointsFixture(t)
	ctx := context.Background()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	// Local synthetic rows exercise the actual PostgreSQL aggregation without
	// bypassing business-ledger constraints or modifying financial fixture data.
	statements := []string{
		`CREATE TEMP TABLE platform_settings(id boolean,timezone text) ON COMMIT DROP`,
		`INSERT INTO platform_settings VALUES(true,'Pacific/Kiritimati')`,
		`CREATE TEMP TABLE bet_orders(brand_id text,brand_member_id text,total_points bigint,prize_points bigint,status text,settled_at timestamptz) ON COMMIT DROP`,
		`CREATE TEMP TABLE platform_mark_six_orders(brand_id text,member_id text,total_points bigint,prize_points bigint,refund_points bigint,status text,settled_at timestamptz) ON COMMIT DROP`,
		`CREATE TEMP TABLE platform_canada_orders(brand_id text,member_id text,total_points bigint,prize_points bigint,refund_points bigint,lost_points bigint,profile text,status text,settled_at timestamptz) ON COMMIT DROP`,
		`CREATE TEMP TABLE configured_orders(brand_id text,member_id text,total_points bigint,status text,snapshot jsonb,result jsonb,settled_at timestamptz) ON COMMIT DROP`,
		`INSERT INTO bet_orders VALUES('brand','member',9007199254740993,0,'placed',NULL),('brand','member',10,25,'won',now()),('other','member',999,999,'placed',NULL),('brand','other',999,999,'placed',NULL),('brand','member',100,0,'lost',now()-interval '2 days')`,
		`INSERT INTO platform_mark_six_orders VALUES('brand','member',20,0,0,'placed',NULL),('brand','member',100,0,100,'settled',now())`,
		`INSERT INTO platform_canada_orders VALUES('brand','member',30,0,0,0,'canada_28','placed',NULL),('brand','member',10,0,0,0,'canada_niuniu','placed',NULL),('brand','member',20,0,0,20,'canada_28','settled',now()),('brand','member',10,0,0,30,'canada_niuniu','settled',now()),('brand','member',10,40,0,0,'canada_niuniu','settled',now())`,
	}
	for _, sql := range statements {
		if _, err = tx.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	var pending, net string
	if err = tx.QueryRow(ctx, memberSummarySQL, "brand", "member").Scan(&pending, &net); err != nil {
		t.Fatal(err)
	}
	if pending != "9007199254741053" || net != "5" {
		t.Fatalf("pending=%s net=%s", pending, net)
	}
	// Records on either side of the configured local-day boundary, not UTC.
	if _, err = tx.Exec(ctx, `TRUNCATE bet_orders,platform_mark_six_orders,platform_canada_orders,configured_orders; INSERT INTO bet_orders VALUES('brand','member',1,8,'won',(date_trunc('day',now() AT TIME ZONE 'Pacific/Kiritimati') AT TIME ZONE 'Pacific/Kiritimati')),('brand','member',1,100,'won',(date_trunc('day',now() AT TIME ZONE 'Pacific/Kiritimati') AT TIME ZONE 'Pacific/Kiritimati')-interval '1 second')`); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, memberSummarySQL, "brand", "member").Scan(&pending, &net); err != nil {
		t.Fatal(err)
	}
	if pending != "0" || net != "7" {
		t.Fatalf("day boundary pending=%s net=%s", pending, net)
	}
	// Reserved risk release is not profit: win +50, loss -190, zero-loss release +0.
	if _, err = tx.Exec(ctx, `TRUNCATE bet_orders,platform_mark_six_orders,platform_canada_orders;
 INSERT INTO configured_orders VALUES
 ('brand','member',10,'settled','{"catalog":{"funding":"reserve"}}','{"prize_points":"50","refund_points":"200"}',now()),
 ('brand','member',10,'settled','{"catalog":{"funding":"reserve"}}','{"prize_points":"0","refund_points":"10","consumed_points":"190"}',now()),
 ('brand','member',10,'settled','{"catalog":{"funding":"reserve"}}','{"prize_points":"0","refund_points":"200"}',now()),
 ('other','member',10,'settled','{"catalog":{"funding":"reserve"}}','{"prize_points":"999","refund_points":"200"}',now()),
 ('brand','other',10,'settled','{"catalog":{"funding":"reserve"}}','{"prize_points":"999","refund_points":"200"}',now())`); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, memberSummarySQL, "brand", "member").Scan(&pending, &net); err != nil || pending != "0" || net != "-140" {
		t.Fatal("reserved net", pending, net, err)
	}

}
