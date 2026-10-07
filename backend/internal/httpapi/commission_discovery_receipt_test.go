package httpapi

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/commission"
)

func TestCommissionDiscoveryHTTPActualRetryReceiptAndRevokedPermission(t *testing.T) {
	var boundary time.Time
	f, order := settlementHTTPFixtureBeforeBet(t, func(f pointsHTTPFixture) {
		grantCommissionCycle(t, f.managementHTTP, "view", "retry")
		agentGrantRoot(t, f.managementHTTP)
		mustStatus(t, f.call("PUT", "/api/v1/admin/agent-policy", "discovery-receipt-agency-01", f.token, managedBrand, agentPolicyBody(true, 3, "0.1")), 200)
		grantCommissionPolicy(t, f.managementHTTP)
		boundary = time.Now().UTC().Add(4 * time.Second).Truncate(time.Second)
		weekday := int(boundary.Weekday())
		config := enabledCommissionPolicyConfig()
		config.Calendar.Weekday = &weekday
		config.Calendar.BoundaryTime = boundary.Format("15:04:05")
		var current commission.Policy
		read := f.call("GET", "/api/v1/admin/commission-policy", "", f.token, managedBrand, nil)
		mustStatus(t, read, 200)
		managedData(t, read, &current)
		mustStatus(t, f.call("PUT", "/api/v1/admin/commission-policy", "discovery-receipt-finance-01", f.token, managedBrand, commission.PolicyInput{Version: current.Version, Config: config, Reason: "enable actual discovery receipt fixture"}), 200)
	})
	ctx := context.Background()
	if wait := time.Until(boundary); wait > 0 {
		time.Sleep(wait)
	}
	if !order.PlacedAt.Before(boundary) {
		t.Fatal("fixture bet did not precede actual saved boundary")
	}
	_, err := f.pool.Exec(ctx, `CREATE FUNCTION test_reject_discovery_creation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='commission.cycle.create' THEN RAISE EXCEPTION 'test discovery creation outage'; END IF; RETURN NEW; END $$;
 CREATE TRIGGER test_reject_discovery_creation BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION test_reject_discovery_creation()`)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.pool.Exec(ctx, `DROP TRIGGER IF EXISTS test_reject_discovery_creation ON audit_logs;DROP FUNCTION IF EXISTS test_reject_discovery_creation()`)
	})
	s := commission.Service{DB: f.pool}
	if n, e := s.ProcessDiscovery(ctx, 100); e != nil || n != 1 {
		t.Fatalf("actual discovery failure processed=%d err=%v", n, e)
	}
	if _, err = f.pool.Exec(ctx, `DROP TRIGGER test_reject_discovery_creation ON audit_logs;DROP FUNCTION test_reject_discovery_creation()`); err != nil {
		t.Fatal(err)
	}
	listed := f.call("GET", commissionDiscoveryHTTPPath, "", f.token, managedBrand, nil)
	mustStatus(t, listed, 200)
	var page commission.DiscoveryPage
	managedData(t, listed, &page)
	if len(page.Items) != 1 || page.Items[0].ID != order.ID || page.Items[0].State != "failed" || page.Items[0].Version != 2 {
		t.Fatalf("real failure discovery read=%+v", page)
	}
	body := fmt.Sprintf(`{"version":%d,"reason":"retry after actual audit storage recovery"}`, page.Items[0].Version)
	path := commissionDiscoveryHTTPPath + "/" + order.ID + "/retry"
	key := "discovery-receipt-retry-01"
	first := f.rawCall("POST", path, key, f.token, managedBrand, body)
	mustStatus(t, first, 200)
	var original commission.Discovery
	managedData(t, first, &original)
	if original.State != "pending" || original.Version != 3 || original.CycleID != nil || original.LastAuditLogID == nil {
		t.Fatalf("invalid retry receipt=%+v", original)
	}
	if n, e := s.ProcessDiscovery(ctx, 100); e != nil || n != 1 {
		t.Fatalf("actual recovery processed=%d err=%v", n, e)
	}
	var state string
	if err = f.pool.QueryRow(ctx, `SELECT state FROM commission_discovery WHERE id=$1`, order.ID).Scan(&state); err != nil || state != "registered" {
		t.Fatalf("actual retry did not register: %s err=%v", state, err)
	}
	replay := f.rawCall("POST", path, key, f.token, managedBrand, body)
	mustStatus(t, replay, 200)
	var cached commission.Discovery
	managedData(t, replay, &cached)
	if cached.State != original.State || cached.Version != original.Version || cached.CycleID != nil || cached.LastAuditLogID == nil || *cached.LastAuditLogID != *original.LastAuditLogID {
		t.Fatalf("worker state replaced original retry receipt: %+v", cached)
	}
	mustStatus(t, f.rawCall("POST", path, key, f.token, managedBrand, "{ "+body[1:]), 409)
	if _, err = f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='commission.retry.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.rawCall("POST", path, key, f.token, managedBrand, body), 403)
	var retryAudits, cycles, credits int
	err = f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM audit_logs WHERE brand_id=$1 AND resource_id=$2 AND action='commission.discovery.retry'),
 (SELECT count(*) FROM commission_cycles WHERE brand_id=$1),(SELECT count(*) FROM point_ledger_entries WHERE brand_id=$1 AND entry_type='commission')`, managedBrand, order.ID).Scan(&retryAudits, &cycles, &credits)
	if err != nil || retryAudits != 1 || cycles != 1 || credits != 0 {
		t.Fatalf("retry duplicate economics/audit=%d/%d/%d err=%v", retryAudits, cycles, credits, err)
	}
}
