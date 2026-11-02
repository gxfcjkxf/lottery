package httpapi

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/finance"
	"strings"
	"testing"
	"time"
)

func TestRechargeQuotaHTTPPlatformGrantManualIdempotencyAndIsolation(t *testing.T) {
	f := pointsFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE brand_recharge_quotas SET available=0 WHERE brand_id=$1`, managedBrand); err != nil {
		t.Fatal(err)
	}
	op := securityOperator(t, f.managementHTTP)
	grantPath := "/api/v1/platform/recharge-quota/grants"
	mustStatus(t, f.call("POST", grantPath, "brand-grant-denied", f.token, managedBrand, map[string]string{"points": "100"}), 403)
	mustStatus(t, op.call("POST", grantPath, "platform-grant-denied", op.token, managedBrand, map[string]string{"points": "100"}), 403)
	grantPlatformPermission(t, op, "recharge_quota.write.platform", "recharge.view.platform")
	for i := 0; i < 2; i++ {
		mustStatus(t, op.call("POST", grantPath, "quota-grant-once", op.token, managedBrand, map[string]string{"points": "100"}), 200)
	}
	mustStatus(t, op.call("POST", grantPath, "quota-grant-once", op.token, managedBrand, map[string]string{"points": "101"}), 409)
	body := map[string]string{"member_id": f.memberID, "points": "70"}
	path := "/api/v1/admin/recharges/manual"
	first := f.call("POST", path, "manual-credit-once", f.token, managedBrand, body)
	mustStatus(t, first, 201)
	var record finance.Recharge
	managedData(t, first, &record)
	if record.Method != "manual" || record.State != "confirmed" || record.ConfirmedBy != f.root {
		t.Fatalf("record=%+v", record)
	}
	mustStatus(t, f.call("POST", path, "manual-credit-once", f.token, managedBrand, body), 201)
	mustStatus(t, f.call("POST", path, "manual-overdraft", f.token, managedBrand, body), 409)
	mustStatus(t, f.call("POST", path, "manual-other-brand", f.token, pointsBrandB, body), 403)
	mustStatus(t, op.call("POST", "/api/v1/platform/recharges/manual", "platform-credit-denied", op.token, managedBrand, body), 404)
	var available int64
	var orders, entries, audits int
	if err := f.pool.QueryRow(ctx, `SELECT available FROM brand_recharge_quotas WHERE brand_id=$1`, managedBrand).Scan(&available); err != nil || available != 30 {
		t.Fatalf("quota=%d err=%v", available, err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM recharge_orders WHERE brand_id=$1`, managedBrand).Scan(&orders); err != nil || orders != 1 {
		t.Fatalf("orders=%d err=%v", orders, err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM brand_recharge_quota_entries WHERE brand_id=$1`, managedBrand).Scan(&entries); err != nil || entries != 2 {
		t.Fatalf("entries=%d err=%v", entries, err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE brand_id=$1 AND action='finance.recharge_quota.grant'`, managedBrand).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audits=%d err=%v", audits, err)
	}
	r := f.call("GET", "/api/v1/admin/recharge-quota", "", f.token, managedBrand, nil)
	mustStatus(t, r, 200)
	var quota finance.RechargeQuota
	managedData(t, r, &quota)
	if quota.Available != "30" {
		t.Fatal(quota)
	}
	mustStatus(t, f.call("GET", "/api/v1/admin/recharge-quota", "", f.token, pointsBrandB, nil), 403)
}

func TestRechargeQuotaHTTPReclaimIdempotencyPermissionsAndBrandSummary(t *testing.T) {
	f := pointsFixture(t)
	ctx := context.Background()
	op := securityOperator(t, f.managementHTTP)
	path := "/api/v1/platform/recharge-quota/reclaims"
	body := map[string]string{"points": "20"}
	if _, err := f.pool.Exec(ctx, `UPDATE brand_recharge_quotas SET available=100 WHERE brand_id=$1`, managedBrand); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("POST", path, "reclaim-brand-denied", f.token, managedBrand, body), 403)
	mustStatus(t, op.call("POST", path, "reclaim-ungranted", op.token, managedBrand, body), 403)
	role := grantPlatformPermission(t, op, "recharge_quota.write.platform", "recharge.view.platform", "brand.view.platform")
	first := op.call("POST", path, "reclaim-once", op.token, managedBrand, body)
	mustStatus(t, first, 200)
	var quota finance.RechargeQuota
	managedData(t, first, &quota)
	if quota.Available != "80" {
		t.Fatal(quota)
	}
	mustStatus(t, op.call("POST", path, "reclaim-once", op.token, managedBrand, body), 200)
	mustStatus(t, op.call("POST", path, "reclaim-once", op.token, managedBrand, map[string]string{"points": "21"}), 409)
	tooMuch := op.call("POST", path, "reclaim-too-much", op.token, managedBrand, map[string]string{"points": "81"})
	mustStatus(t, tooMuch, 409)
	if !strings.Contains(tooMuch.Body.String(), "RECHARGE_QUOTA_RECLAIM_EXCEEDED") {
		t.Fatal(tooMuch.Body.String())
	}
	mustStatus(t, op.call("POST", path, "reclaim-rest", op.token, managedBrand, map[string]string{"points": "80"}), 200)
	var count int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM brand_recharge_quota_entries WHERE brand_id=$1 AND kind='reclaim'`, managedBrand).Scan(&count); err != nil || count != 2 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	agentGrantRoot(t, f.managementHTTP)
	mustStatus(t, f.call("POST", "/api/v1/admin/agents", "brand-summary-agent", f.token, managedBrand, agentNodeBody(1, f.memberID, nil, nil, "0")), 201)
	r := op.call("GET", "/api/v1/platform/brands", "", op.token, "", nil)
	mustStatus(t, r, 200)
	var page struct {
		Items []struct {
			ID          string    `json:"id"`
			MemberCount string    `json:"member_count"`
			AgentCount  string    `json:"agent_count"`
			Quota       *string   `json:"recharge_quota_available"`
			CreatedAt   time.Time `json:"created_at"`
		} `json:"items"`
	}
	managedData(t, r, &page)
	found := false
	for _, b := range page.Items {
		if b.ID == managedBrand {
			found = true
			if b.MemberCount != "1" || b.AgentCount != "1" || b.Quota == nil || *b.Quota != "0" || b.CreatedAt.IsZero() {
				t.Fatalf("brand summary=%+v", b)
			}
		} else if b.MemberCount != "0" || b.AgentCount != "0" {
			t.Fatalf("cross-brand totals=%+v", b)
		}
	}
	if !found {
		t.Fatal("brand summary missing")
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key IN ('recharge.view.platform','recharge_quota.write.platform')`, role); err != nil {
		t.Fatal(err)
	}
	r = op.call("GET", "/api/v1/platform/brands", "", op.token, "", nil)
	mustStatus(t, r, 200)
	managedData(t, r, &page)
	for _, b := range page.Items {
		if b.Quota != nil {
			t.Fatal("quota exposed without view grant")
		}
	}
	mustStatus(t, op.call("POST", path, "reclaim-once", op.token, managedBrand, body), 403)
}
