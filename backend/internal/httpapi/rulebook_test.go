package httpapi

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"net/http/httptest"
	"testing"
)

func bookFixture(t *testing.T) (managementHTTP, string) {
	t.Helper()
	f := managedFixture(t)
	ctx := context.Background()
	var role string
	if e := f.pool.QueryRow(ctx, `SELECT role_id::text FROM admin_account_roles WHERE account_id=$1`, f.root).Scan(&role); e != nil {
		t.Fatal(e)
	}
	for _, key := range []string{"game.view.brand", "game.write.brand", "rule.view.brand", "rule.write.brand", "rule.validate.brand", "rule.submit.brand", "rule.review.brand"} {
		if _, e := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2)`, role, key); e != nil {
			t.Fatal(e)
		}
	}
	id := ids.New()
	if _, e := f.pool.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) SELECT $1,'book_reviewer',password_hash FROM admin_accounts WHERE id=$2`, id, f.root); e != nil {
		t.Fatal(e)
	}
	if _, e := f.pool.Exec(ctx, `INSERT INTO admin_brand_scopes(account_id,brand_id) VALUES($1,$2)`, id, managedBrand); e != nil {
		t.Fatal(e)
	}
	if _, e := f.pool.Exec(ctx, `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, id, role); e != nil {
		t.Fatal(e)
	}
	r := f.call("POST", "/api/v1/admin/auth/login", "book-reviewer-login", "", managedBrand, map[string]string{"identifier": "book_reviewer", "password": "root-test-password-2026"})
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	var auth identity.AdminAuthentication
	managedData(t, r, &auth)
	return f, auth.AccessToken
}
func mustStatus(t *testing.T, r *httptest.ResponseRecorder, status int) {
	t.Helper()
	if r.Code != status {
		t.Fatal(r.Code, r.Body.String())
	}
}
func TestRuleBookHTTPCompleteWorkflowReplayAndImmutableHistory(t *testing.T) {
	f, _ := bookFixture(t)
	in := ruleInput()
	definition := in["definition"]
	body := map[string]any{"code": "six_one", "name": "Six plus one", "model": definition.(map[string]any)["model"], "timezone": "Asia/Manila", "reason": "game setup"}
	r := f.call("POST", "/api/v1/admin/games", "book-game-create", f.token, managedBrand, body)
	mustStatus(t, r, 201)
	var g rulebook.Game
	managedData(t, r, &g)
	replay := f.call("POST", "/api/v1/admin/games", "book-game-create", f.token, managedBrand, body)
	mustStatus(t, replay, 201)
	var same rulebook.Game
	managedData(t, replay, &same)
	if same.ID != g.ID {
		t.Fatal("duplicated game")
	}
	r = f.call("POST", "/api/v1/admin/games/"+g.ID+"/plays", "book-play-create", f.token, managedBrand, map[string]any{"code": "special", "name": "Special number", "reason": "play setup"})
	mustStatus(t, r, 201)
	var p rulebook.Play
	managedData(t, r, &p)
	r = f.call("POST", "/api/v1/admin/rule-versions", "book-draft-create", f.token, managedBrand, map[string]any{"play_id": p.ID, "definition": definition, "effect_mode": "immediate", "reason": "rule draft"})
	mustStatus(t, r, 201)
	var v rulebook.Version
	managedData(t, r, &v)
	action := func(path, key, token string, body any) *httptest.ResponseRecorder {
		return f.call("POST", "/api/v1/admin/rule-versions/"+v.ID+"/"+path, key, token, managedBrand, body)
	}
	r = action("submit-review", "book-no-validation", f.token, map[string]any{"version": v.Version, "reason": "not validated"})
	mustStatus(t, r, 409)
	cases := []any{map[string]any{"name": "special candidate multiple", "selection": in["selection"], "draw": in["draw"], "multiplier": "2", "expected_bet_points": "8", "expected_prize_points": "999", "expected_won": true}}
	r = action("validate", "book-invalid-expectation", f.token, map[string]any{"version": v.Version, "cases": cases, "reason": "mismatch test"})
	mustStatus(t, r, 200)
	managedData(t, r, &v)
	if v.Validation == nil || v.Validation.Passed {
		t.Fatal(v)
	}
	r = action("submit-review", "book-failed-validation", f.token, map[string]any{"version": v.Version, "reason": "should not submit"})
	mustStatus(t, r, 409)
	cases[0].(map[string]any)["expected_prize_points"] = "70"
	r = action("validate", "book-good-validate", f.token, map[string]any{"version": v.Version, "cases": cases, "reason": "validated test"})
	mustStatus(t, r, 200)
	managedData(t, r, &v)
	r = action("submit-review", "book-submit-review", f.token, map[string]any{"version": v.Version, "reason": "ready for review"})
	mustStatus(t, r, 200)
	managedData(t, r, &v)
	r = action("approve", "book-warning-needs-ack", f.token, map[string]any{"version": v.Version, "reason": "without acknowledgement"})
	mustStatus(t, r, 409)
	approvedBody := map[string]any{"version": v.Version, "reason": "brand reviewed", "warnings_acknowledged": true}
	r = action("approve", "book-approved-001", f.token, approvedBody)
	mustStatus(t, r, 200)
	managedData(t, r, &v)
	if v.Status != "active" || v.ReviewedBy != f.root || v.CreatedBy != f.root {
		t.Fatal(v)
	}
	mustStatus(t, action("approve", "book-approved-001", f.token, approvedBody), 200)
	var n int
	if e := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE action='rule.review.approve'`).Scan(&n); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	r = f.call("PUT", "/api/v1/admin/rule-versions/"+v.ID, "book-active-edit", f.token, managedBrand, map[string]any{"version": v.Version, "definition": definition, "effect_mode": "immediate", "reason": "must not rewrite"})
	mustStatus(t, r, 409)
	r = f.call("GET", "/api/v1/admin/plays/"+p.ID+"/rule-versions", "", f.token, managedBrand, nil)
	mustStatus(t, r, 200)
	var history struct {
		Versions []rulebook.Version `json:"versions"`
	}
	managedData(t, r, &history)
	if len(history.Versions) != 1 || history.Versions[0].Status != "active" {
		t.Fatal(history)
	}
	mustStatus(t, f.call("GET", "/api/v1/admin/rule-versions/"+v.ID, "", f.token, pointsBrandB, nil), 403)
	forged := pointsCall(f, "POST", "/api/v1/admin/rule-versions/"+v.ID+"/clone", "book-origin-forged", f.token, managedBrand, "https://evil.invalid", map[string]any{"effect_mode": "immediate", "reason": "forged"})
	mustStatus(t, forged, 403)
	var ledger int
	if e := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM point_ledger_entries`).Scan(&ledger); e != nil || ledger != 0 {
		t.Fatal("ruleworkflow changed funds", ledger, e)
	}
}
func TestRuleBookHTTPExplicitRevocationAndSuperReadonly(t *testing.T) {
	f, _ := bookFixture(t)
	ctx := context.Background()
	_, e := f.pool.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root)
	if e != nil {
		t.Fatal(e)
	}
	// Super-admin remains denied even if mistakenly assigned brand write grants.
	r := f.call("POST", "/api/v1/admin/games", "book-super-write", f.token, managedBrand, map[string]any{})
	mustStatus(t, r, 403)
	mustStatus(t, f.call("GET", "/api/v1/admin/games", "", f.token, managedBrand, nil), 403)
	platformToken := platformAdminToken(t, f)
	mustStatus(t, f.call("GET", "/api/v1/platform/games", "", platformToken, managedBrand, nil), 403)
	grantPlatformPermission(t, f, "game.view.platform")
	mustStatus(t, f.call("GET", "/api/v1/platform/games", "", platformToken, managedBrand, nil), 200)
	if _, e = f.pool.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=false WHERE id=$1`, f.root); e != nil {
		t.Fatal(e)
	}
	if _, e = f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN (SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='game.write.brand'`, f.root); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("POST", "/api/v1/admin/games", "book-revoked-write", f.token, managedBrand, map[string]any{}), 403)
	bad := ruleInput()
	raw, _ := json.Marshal(bad["definition"])
	var definition map[string]any
	json.Unmarshal(raw, &definition)
	definition["script"] = "execute"
	r = f.call("POST", "/api/v1/admin/rule-versions", "book-unknown-code", f.token, managedBrand, map[string]any{"play_id": ids.New(), "definition": definition, "effect_mode": "immediate", "reason": "bad"})
	mustStatus(t, r, 400)
}
