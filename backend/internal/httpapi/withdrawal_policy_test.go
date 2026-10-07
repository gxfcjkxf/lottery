package httpapi

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/withdrawal"
)

func grantWithdrawalPolicy(t *testing.T, f managementHTTP) {
	t.Helper()
	for _, key := range []string{"withdrawal_policy.view.brand", "withdrawal_policy.write.brand", "withdrawal_policy.view.platform"} {
		if _, e := f.pool.Exec(context.Background(), `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,$2 FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root, key); e != nil {
			t.Fatal(e)
		}
	}
}
func TestWithdrawalPolicyHTTPHistoryCheckedReplayAndNoFinancialMutation(t *testing.T) {
	f, g := periodFixture(t)
	base := "/api/v1/admin/withdrawal-policy"
	gameBase := "/api/v1/admin/games/" + g.ID + "/withdrawal-policy"
	ctx := context.Background()
	mustStatus(t, f.call("GET", base, "", f.token, managedBrand, nil), 403)
	grantWithdrawalPolicy(t, f)
	initial := f.call("GET", base, "", f.token, managedBrand, nil)
	mustStatus(t, initial, 200)
	var current withdrawal.BrandPolicy
	managedData(t, initial, &current)
	if current.Version != 1 || current.Config.Enabled {
		t.Fatal(current)
	}
	config := current.Config
	config.Enabled = true
	config.TurnoverMultiple = "2.5"
	body := withdrawal.BrandInput{Version: 1, Config: config, Reason: "configured withdrawal rules"}
	first := f.call("PUT", base, "withdraw-policy-first-01", f.token, managedBrand, body)
	mustStatus(t, first, 200)
	var updated withdrawal.BrandPolicy
	managedData(t, first, &updated)
	if updated.Version != 2 || updated.Config.TurnoverMultiple != "2.5" || updated.AuditLogID == "" {
		t.Fatal(updated)
	}
	replay := f.call("PUT", base, "withdraw-policy-first-01", f.token, managedBrand, body)
	mustStatus(t, replay, 200)
	var repeated withdrawal.BrandPolicy
	managedData(t, replay, &repeated)
	if repeated.Version != 2 || repeated.AuditLogID != updated.AuditLogID {
		t.Fatal(repeated)
	}
	changed := body
	changed.Reason = "different operation body"
	mustStatus(t, f.call("PUT", base, "withdraw-policy-first-01", f.token, managedBrand, changed), 409)
	mustStatus(t, f.call("PUT", base, "withdraw-policy-stale-01", f.token, managedBrand, body), 409)
	zero := "0"
	gameBody := withdrawal.GameInput{Version: 1, Config: withdrawal.GameConfig{TurnoverMultiple: &zero}, Reason: "explicit game override"}
	r := f.call("PUT", gameBase, "withdraw-game-first-01", f.token, managedBrand, gameBody)
	mustStatus(t, r, 200)
	var gamePolicy withdrawal.GamePolicy
	managedData(t, r, &gamePolicy)
	if gamePolicy.Effective.Source != "game" || gamePolicy.Effective.TurnoverMultiple != "0" || gamePolicy.Effective.BrandVersion != 2 || gamePolicy.Version != 2 {
		t.Fatal(gamePolicy)
	}
	for _, path := range []string{base + "/history", gameBase + "/history"} {
		r = f.call("GET", path, "", f.token, managedBrand, nil)
		mustStatus(t, r, 200)
		var history struct {
			Items []withdrawal.Revision `json:"items"`
		}
		managedData(t, r, &history)
		if len(history.Items) != 2 || history.Items[0].ChangedBy != f.root || history.Items[1].ChangedBy != "" {
			t.Fatal(history)
		}
	}
	mustStatus(t, f.call("GET", base+"/history?limit=101", "", f.token, managedBrand, nil), 400)
	mustStatus(t, f.call("GET", gameBase, "", f.token, "0199a000-0000-7000-8000-000000000002", nil), 403)
	raw, _ := json.Marshal(body)
	duplicate := string(raw[:len(raw)-1]) + `,"version":1}`
	mustStatus(t, f.rawCall("PUT", base, "withdraw-duplicate-field", f.token, managedBrand, duplicate), 400)
	if _, e := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN (SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='withdrawal_policy.write.brand'`, f.root); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("PUT", base, "withdraw-policy-first-01", f.token, managedBrand, body), 403)
	grantWithdrawalPolicy(t, f)
	if _, e := f.pool.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("GET", base, "", f.token, managedBrand, nil), 200)
	mustStatus(t, f.call("PUT", base, "withdraw-policy-first-01", f.token, managedBrand, body), 403)
	var entries, orders, audits int
	if e := f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM point_ledger_entries),(SELECT count(*) FROM bet_orders),(SELECT count(*) FROM audit_logs WHERE action IN ('withdrawal_policy.brand.update','withdrawal_policy.game.update'))`).Scan(&entries, &orders, &audits); e != nil || entries != 0 || orders != 0 || audits != 2 {
		t.Fatal(entries, orders, audits, e)
	}
	// Enabling policy cannot replace a member session or formal qualification.
	mustStatus(t, f.call("POST", "/api/v1/withdrawals", "withdraw-no-user-session", "", managedBrand, map[string]any{"points": "1", "source_allocation": []map[string]string{{"source": "recharge", "state": "available", "points": "1"}}}), 401)
}
