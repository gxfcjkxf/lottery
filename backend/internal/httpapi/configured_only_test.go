package httpapi

import (
	"strings"
	"testing"
)

func TestOnlyConfiguredBettingAndSettlementRoutesAreRegistered(t *testing.T) {
	found := map[string]bool{}
	for _, r := range RegisteredRoutes() {
		for _, old := range []string{"bet-previews", "bet-orders", "settlement-jobs", "settlement-context", "settlement-policy", "/corrections", "/mark-six/orders", "/canada/orders", "/game-settings"} {
			if strings.Contains(r.Path, old) {
				t.Fatal("retired real-money route", r)
			}
		}
		found[r.Method+" "+r.Path] = true
	}
	for _, route := range []string{"POST /api/v1/configured/orders", "POST /api/v1/configured/preview", "GET /api/v1/admin/configured-orders", "GET /api/v1/platform/configured-orders", "GET /api/v1/betting-history"} {
		if !found[route] {
			t.Fatal("missing configured route", route)
		}
	}
}
