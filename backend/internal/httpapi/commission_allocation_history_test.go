package httpapi

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestCommissionAllocationQueryStrictParameters(t *testing.T) {
	const id = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	for _, query := range []string{"", "?limit=1&offset=1000000", "?agent_id=" + id, "?order_id=" + id, "?agent_id=" + id + "&order_id=" + id} {
		if _, ok := commissionAllocationQuery(httptest.NewRequest("GET", "/owned"+query, nil), true); !ok {
			t.Fatal("valid query rejected", query)
		}
	}
	for _, query := range []string{"?", "?limit=0", "?limit=101", "?offset=-1", "?offset=1000001", "?limit=01", "?limit=%2B1", "?offset=00", "?limit=1&limit=2", "?agent_id=", "?agent_id=bad", "?unknown=1", "?agent_id=AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA", "?x=%zz", "?limit=1;offset=2"} {
		if _, ok := commissionAllocationQuery(httptest.NewRequest("GET", "/owned"+query, nil), true); ok {
			t.Fatal("invalid query accepted", query)
		}
	}
	if _, ok := commissionAllocationQuery(httptest.NewRequest("GET", "/owned?agent_id="+id, nil), false); ok {
		t.Fatal("historical earning query accepted allocation-only filter")
	}
}

func TestCommissionAllocationHTTPAuthorizationNotFoundAndAuditFailure(t *testing.T) {
	f := managedFixture(t)
	const id = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	base := commissionCyclesPath + "/" + id + "/runs/" + id
	get := func(path, token string) *httptest.ResponseRecorder {
		return f.rawCall("GET", path, "", token, managedBrand, "")
	}
	for _, kind := range []string{"earnings", "allocations"} {
		mustStatus(t, get(base+"/"+kind, ""), 401)
		mustStatus(t, get(base+"/"+kind, f.token), 403)
	}
	grantCommissionCycle(t, f, "view")
	for _, kind := range []string{"earnings", "allocations"} {
		mustStatus(t, get(base+"/"+kind, f.token), 404)
		mustStatus(t, get(base+"/"+kind+"?limit=01", f.token), 400)
		mustStatus(t, get(base+"/"+kind+"?unexpected=1", f.token), 400)
		mustStatus(t, f.rawCall("GET", base+"/"+kind, "", f.token, managedBrand, "null"), 400)
	}
	var audited int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE brand_id=$1 AND action='commission.cycle.run_allocations' AND after_json->>'cycle_id'=$2 AND after_json->>'run_id'=$2 AND after_json->>'limit'='20'`, managedBrand, id).Scan(&audited); err != nil || audited != 1 {
		t.Fatal("run query evidence not committed", audited, err)
	}
	if _, err := f.pool.Exec(context.Background(), `CREATE FUNCTION reject_allocation_query_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action LIKE 'commission.cycle.run_%' THEN RAISE EXCEPTION 'owned audit outage'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_allocation_query_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_allocation_query_audit()`); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"earnings", "allocations"} {
		mustStatus(t, get(base+"/"+kind, f.token), 503)
	}
}
