package httpapi

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/reconciliation"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const inventoryHTTPPath = "/api/v1/admin/reconciliations/business-inventory"

func inventoryHTTPMoney(t *testing.T, f reconciliationHTTPFixture) string {
	t.Helper()
	var out string
	err := f.pool.QueryRow(context.Background(), `SELECT md5(jsonb_build_object(
 'accounts',(SELECT coalesce(jsonb_agg(to_jsonb(p) ORDER BY id),'[]') FROM point_accounts p),
 'buckets',(SELECT coalesce(jsonb_agg(to_jsonb(p) ORDER BY account_id,source,state),'[]') FROM point_buckets p),
 'ledger',(SELECT coalesce(jsonb_agg(to_jsonb(p) ORDER BY id),'[]') FROM point_ledger_entries p),
 'outbox',(SELECT coalesce(jsonb_agg(to_jsonb(p) ORDER BY id),'[]') FROM outbox_events p))::text)`).Scan(&out)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestBrandBusinessInventoryHTTPReadOnlyViewGrantAndStrictInputs(t *testing.T) {
	f := newReconciliationHTTPFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key<>'wallet.view.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	before := inventoryHTTPMoney(t, f)
	response := f.call("GET", inventoryHTTPPath, "", f.token, managedBrand, nil)
	mustStatus(t, response, 200)
	var out reconciliation.InventorySnapshot
	managedData(t, response, &out)
	if !out.Consistent || out.IssueCount != "0" || len(out.Coverage) != 41 || out.SourceRowCount == "0" {
		t.Fatalf("genuine view-only snapshot %+v", out)
	}
	if inventoryHTTPMoney(t, f) != before {
		t.Fatal("inventory changed financial facts")
	}
	var audits int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='wallet.business_inventory.view' AND after_json->>'fingerprint'=$1 AND after_json->>'success'='true'`, out.Fingerprint).Scan(&audits); err != nil || audits != 1 {
		t.Fatal("missing committed observation audit", audits, err)
	}
	for _, path := range []string{inventoryHTTPPath + "?", inventoryHTTPPath + "?limit=1", inventoryHTTPPath + "?brand_id=" + managedBrand} {
		mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 400)
	}
	for _, body := range []string{"null", "{}", " "} {
		mustStatus(t, reconciliationRawCall(f, "GET", inventoryHTTPPath, "", body), 400)
	}
	mustStatus(t, f.call("GET", inventoryHTTPPath, "", f.token, "", nil), 400)
	mustStatus(t, f.call("GET", inventoryHTTPPath, "", "", managedBrand, nil), 401)
}

func TestBrandBusinessInventoryHTTPSourceAndAuditFailuresCloseWithoutPartialEvidence(t *testing.T) {
	f := newReconciliationHTTPFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `ALTER FUNCTION brand_business_inventory(uuid) RENAME TO owned_hidden_brand_business_inventory`); err != nil {
		t.Fatal(err)
	}
	response := f.call("GET", inventoryHTTPPath, "", f.token, managedBrand, nil)
	mustStatus(t, response, 503)
	if strings.Contains(response.Body.String(), "source_row_count") || strings.Contains(response.Body.String(), "owned_hidden") {
		t.Fatal("partial or internal source failure")
	}
	var failures int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='wallet.business_inventory.read_failed' AND after_json->>'attempted_brand'=$1`, managedBrand).Scan(&failures); err != nil || failures != 1 {
		t.Fatal(failures, err)
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='wallet.view.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("GET", inventoryHTTPPath, "", f.token, managedBrand, nil), 403)
	if _, err := f.pool.Exec(ctx, `ALTER FUNCTION owned_hidden_brand_business_inventory(uuid) RENAME TO brand_business_inventory`); err != nil {
		t.Fatal(err)
	}
	var role string
	if err := f.pool.QueryRow(ctx, `SELECT role_id::text FROM admin_account_roles WHERE account_id=$1 LIMIT 1`, f.root).Scan(&role); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,'wallet.view.brand')`, role); err != nil {
		t.Fatal(err)
	}
	before := inventoryHTTPMoney(t, f)
	if _, err := f.pool.Exec(ctx, `CREATE FUNCTION fail_inventory_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='wallet.business_inventory.view' THEN RAISE EXCEPTION 'owned audit outage'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_inventory_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION fail_inventory_audit()`); err != nil {
		t.Fatal(err)
	}
	response = f.call("GET", inventoryHTTPPath, "", f.token, managedBrand, nil)
	mustStatus(t, response, 503)
	if strings.Contains(response.Body.String(), "fingerprint") {
		t.Fatal("audit failure released observation")
	}
	if inventoryHTTPMoney(t, f) != before {
		t.Fatal("failure wrote financial facts")
	}
}

func TestBrandBusinessInventoryHTTPRechecksSessionAfterAuditWait(t *testing.T) {
	f := newReconciliationHTTPFixture(t)
	ctx := context.Background()
	lock, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(ctx)
	const key int64 = 77159102
	if _, err = lock.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, key); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `CREATE FUNCTION wait_inventory_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='wallet.business_inventory.view' THEN PERFORM pg_advisory_xact_lock(77159102); END IF; RETURN NEW; END $$; CREATE TRIGGER wait_inventory_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION wait_inventory_audit()`); err != nil {
		t.Fatal(err)
	}
	response := make(chan *httptest.ResponseRecorder, 1)
	go func() { response <- f.call("GET", inventoryHTTPPath, "", f.token, managedBrand, nil) }()
	waited := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err = f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks waiting JOIN pg_locks held USING(locktype,classid,objid,objsubid) WHERE waiting.locktype='advisory' AND NOT waiting.granted AND held.granted AND held.pid=$1 AND waiting.pid<>pg_backend_pid())`, lock.Conn().PgConn().PID()).Scan(&waited); err != nil {
			t.Fatal(err)
		}
		if waited {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !waited {
		t.Fatal("request did not reach audit wait")
	}
	if _, err = f.pool.Exec(ctx, `UPDATE sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE admin_id=$1 AND revoked_at IS NULL`, f.root); err != nil {
		t.Fatal(err)
	}
	if err = lock.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-response:
		mustStatus(t, r, 401)
		if strings.Contains(r.Body.String(), "fingerprint") {
			t.Fatal("expired session received observation")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("request did not leave audit wait")
	}
	var audits int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='wallet.business_inventory.view'`).Scan(&audits); err != nil || audits != 0 {
		t.Fatal("expired view committed audit", audits, err)
	}
}
