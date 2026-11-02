package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestCommissionListFilterStrictParameters(t *testing.T) {
	for _, path := range []string{
		"?state=unknown", "?state=ready&state=failed", "?from=2026-10-01T00:00:00Z", "?to=2026-10-02T00:00:00Z",
		"?from=2026-10-03T00:00:00Z&to=2026-10-02T00:00:00Z", "?agent_id=bad", "?cycle_id=bad", "?limit=01", "?offset=-1", "?unknown=1", "?state=", "?",
	} {
		_, _, _, ok := commissionListFilter(httptest.NewRequest("GET", "/commission-cycles"+path, nil), "cycle")
		if ok {
			t.Errorf("accepted %s", path)
		}
	}
	for _, kind := range []string{"cycle", "payment", "target"} {
		_, _, _, ok := commissionListFilter(httptest.NewRequest("GET", "/commission?limit=10&offset=20&agent_id=11111111-1111-4111-8111-111111111111", nil), kind)
		if !ok {
			t.Errorf("rejected valid %s filter", kind)
		}
	}
	for _, kind := range []string{"cycle", "payment"} {
		_, _, f, ok := commissionListFilter(httptest.NewRequest("GET", "/commission?from=2026-10-01T00:00:00%2B08:00&to=2026-10-03T00:00:00%2B08:00", nil), kind)
		if !ok || f.From.UTC().Format("2006-01-02T15:04:05Z") != "2026-09-30T16:00:00Z" {
			t.Errorf("range %s: %+v", kind, f)
		}
	}
	for _, query := range []string{"?state=needs_attention", "?cycle_id=11111111-1111-4111-8111-111111111111", "?from=2026-10-01T00:00:00Z&to=2026-10-02T00:00:00Z"} {
		_, _, _, ok := commissionListFilter(httptest.NewRequest("GET", "/targets"+query, nil), "target")
		if ok {
			t.Errorf("target accepted %s", query)
		}
	}
	r := httptest.NewRequest("GET", "/commission", nil)
	r.ContentLength = 1
	if _, _, _, ok := commissionListFilter(r, "cycle"); ok {
		t.Error("accepted GET body")
	}
}

func TestCommissionPaymentReadDetailsRequireBrandPermission(t *testing.T) {
	f := managedFixture(t)
	for _, suffix := range []string{"beneficiaries", "history"} {
		path := "/api/v1/admin/commission-payments/11111111-1111-4111-8111-111111111111/" + suffix
		mustStatus(t, f.call("GET", path, "", "", managedBrand, nil), 401)
		mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 403)
	}
	grantCommissionCycle(t, f, "view")
	for _, suffix := range []string{"beneficiaries", "history"} {
		path := "/api/v1/admin/commission-payments/11111111-1111-4111-8111-111111111111/" + suffix
		mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 404)
		mustStatus(t, f.call("GET", path+"?unknown=1", "", f.token, managedBrand, nil), 400)
	}
}

func TestCommissionCorrectionCasesRequireBrandPermission(t *testing.T) {
	f := managedFixture(t)
	path := "/api/v1/admin/commission-correction-cases"
	mustStatus(t, f.call("GET", path, "", "", managedBrand, nil), 401)
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 403)
	grantCommissionCycle(t, f, "view")
	mustStatus(t, f.call("GET", path+"?state=pending", "", f.token, managedBrand, nil), 200)
	for _, q := range []string{"?state=ready", "?cycle_id=bad", "?state=pending&state=history", "?unknown=1", "?from=2026-01-01T00:00:00Z"} {
		mustStatus(t, f.call("GET", path+q, "", f.token, managedBrand, nil), 400)
	}
}
