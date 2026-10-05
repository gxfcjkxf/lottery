package httpapi

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"testing"
)

func ruleFixture(t *testing.T) managementHTTP {
	t.Helper()
	f := managedFixture(t)
	_, err := f.pool.Exec(context.Background(), `INSERT INTO role_permissions(role_id,permission_key) SELECT ar.role_id,'rule.simulate.brand' FROM admin_account_roles ar WHERE ar.account_id=$1 ON CONFLICT DO NOTHING`, f.root)
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func ruleInput() map[string]any {
	return map[string]any{
		"definition": map[string]any{"schema_version": 1, "model": map[string]any{"model": "X_PLUS_Y", "regular_pool": map[string]any{"min": 1, "max": 49, "allow_repeat": false}, "special_pool": map[string]any{"min": 1, "max": 49, "allow_repeat": false}, "regular_count": 6, "special_count": 1}, "selection": map[string]any{"mode": "numbers", "regular_count": 0, "special_count": 1}, "unit_points": "1", "prize_tiers": []any{map[string]any{"code": "SPECIAL", "condition": map[string]any{"op": "equals", "field": "special_match", "value": 1}, "odds": "35", "exclusive": true}}, "rounding": "half_up", "rounding_scope": "order", "limits": map[string]any{"max_combinations": 100, "max_multiplier": "1000"}},
		"selection":  map[string]any{"special": []int{7, 19, 31, 43}}, "draw": map[string]any{"regular": []int{1, 2, 3, 4, 5, 6}, "special": []int{7}}, "multiplier": "2"}
}
func TestRuleSimulationHTTPRealCalculationReplayScopeAndNoFunds(t *testing.T) {
	f := ruleFixture(t)
	in := ruleInput()
	r := f.call("POST", "/api/v1/admin/rule-simulations", "rule-simulate-001", f.token, managedBrand, in)
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	var out rules.Simulation
	managedData(t, r, &out)
	if out.PrizePoints != 70 || out.BetPoints != 8 || out.CombinationCount != 4 || !out.Won {
		t.Fatal(out)
	}
	replay := f.call("POST", "/api/v1/admin/rule-simulations", "rule-simulate-001", f.token, managedBrand, in)
	if replay.Code != 200 {
		t.Fatal(replay.Code, replay.Body.String())
	}
	var audits, ledger int
	f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE action='rule.simulate'`).Scan(&audits)
	f.pool.QueryRow(context.Background(), `SELECT count(*) FROM point_ledger_entries`).Scan(&ledger)
	if audits != 1 || ledger != 0 {
		t.Fatal("simulation duplicated audit or changed funds", audits, ledger)
	}
	in["multiplier"] = "3"
	conflict := f.call("POST", "/api/v1/admin/rule-simulations", "rule-simulate-001", f.token, managedBrand, in)
	if conflict.Code != 409 {
		t.Fatal(conflict.Code, conflict.Body.String())
	}
	foreign := f.call("POST", "/api/v1/admin/rule-simulations", "rule-foreign-002", f.token, pointsBrandB, in)
	if foreign.Code != 403 {
		t.Fatal(foreign.Code, foreign.Body.String())
	}
	badOrigin := pointsCall(f, "POST", "/api/v1/admin/rule-simulations", "rule-origin-003", f.token, managedBrand, "https://evil.invalid", in)
	if badOrigin.Code != 403 {
		t.Fatal(badOrigin.Code, badOrigin.Body.String())
	}
	var role string
	f.pool.QueryRow(context.Background(), `SELECT role_id::text FROM admin_account_roles WHERE account_id=$1`, f.root).Scan(&role)
	f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='rule.simulate.brand'`, role)
	revoked := f.call("POST", "/api/v1/admin/rule-simulations", "rule-revoked-004", f.token, managedBrand, in)
	if revoked.Code != 403 {
		t.Fatal(revoked.Code, revoked.Body.String())
	}
}
func TestRuleSimulationHTTPRejectsExecutableUnknownAndUnboundedDefinitions(t *testing.T) {
	f := ruleFixture(t)
	for _, kind := range []string{"script", "unknown-field", "schema-version", "huge-multiplier", "out-of-range"} {
		in := ruleInput()
		definition := in["definition"].(map[string]any)
		switch kind {
		case "script":
			definition["script"] = "return true"
		case "unknown-field":
			definition["prize_tiers"] = []any{map[string]any{"code": "FORGED", "condition": map[string]any{"op": "equals", "field": "execute", "value": 1}, "odds": "35", "exclusive": true}}
		case "schema-version":
			definition["schema_version"] = 2
		case "huge-multiplier":
			in["multiplier"] = "1001"
		case "out-of-range":
			in["draw"] = map[string]any{"regular": []int{1, 2, 3, 4, 5, 6}, "special": []int{999}}
		}
		r := f.call("POST", "/api/v1/admin/rule-simulations", "invalid-rule-"+kind, f.token, managedBrand, in)
		if r.Code != 400 {
			t.Fatal(kind, r.Code, r.Body.String())
		}
	}
}
