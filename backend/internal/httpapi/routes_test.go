package httpapi

import (
	"strings"
	"testing"
)

func TestRegisteredRoutesInventoryUsesRealRouter(t *testing.T) {
	routes := RegisteredRoutes()
	seen := map[string]bool{}
	for _, r := range routes {
		key := r.Method + " " + r.Path
		if seen[key] {
			t.Fatal("duplicate", key)
		}
		seen[key] = true
		if r.Method == "" || !strings.HasPrefix(r.Path, "/") {
			t.Fatal(r)
		}
	}
	for _, key := range []string{"GET /health/ready", "POST /api/v1/auth/register", "POST /api/v1/b/{brandCode}/bet-orders", "POST /api/v1/admin/join-codes", "GET /api/v1/admin/reports/ledger", "POST /api/v1/withdrawals", "GET /api/v1/b/{brandCode}/withdrawal-availability", "POST /api/v1/admin/withdrawals/{id}/mark-paid"} {
		if !seen[key] {
			t.Fatal("missing", key)
		}
	}
	for _, key := range []string{"POST /api/v1/admin/commissions/pay", "GET /"} {
		if seen[key] {
			t.Fatal("unimplemented operation exposed", key)
		}
	}
	if len(routes) < 100 {
		t.Fatal("incomplete inventory", len(routes))
	}
}
