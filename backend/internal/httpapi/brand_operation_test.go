package httpapi

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"testing"
)

func grantBrandOperation(t *testing.T, f managementHTTP) {
	t.Helper()
	for _, key := range []string{"brand_operation.view.brand", "brand_operation.write.brand"} {
		if _, err := f.pool.Exec(context.Background(), `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,$2 FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root, key); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBrandOperationHTTPPausePreservesLoginAndAuditFailureRollsBack(t *testing.T) {
	f := pointsFixture(t)
	grantBrandOperation(t, f.managementHTTP)
	const path = "/api/v1/admin/brand-operation"
	initial := f.call("GET", path, "", f.token, managedBrand, nil)
	mustStatus(t, initial, 200)
	var current struct {
		Version int64  `json:"version"`
		Status  string `json:"status"`
	}
	managedData(t, initial, &current)
	if _, err := f.pool.Exec(context.Background(), `CREATE FUNCTION fail_brand_operation_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='brand_operation.update' THEN RAISE EXCEPTION 'test audit failure'; END IF; RETURN NEW; END $$;CREATE TRIGGER fail_brand_operation_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION fail_brand_operation_audit()`); err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"version": current.Version, "status": "paused", "reason": "pause test"}
	mustStatus(t, f.call("PATCH", path, "brand-audit-fail-original-01", f.token, managedBrand, body), 503)
	check := f.call("GET", path, "", f.token, managedBrand, nil)
	mustStatus(t, check, 200)
	var unchanged struct {
		Version int64  `json:"version"`
		Status  string `json:"status"`
	}
	managedData(t, check, &unchanged)
	if unchanged != current {
		t.Fatal("failed audit changed status", unchanged, current)
	}
	if _, err := f.pool.Exec(context.Background(), `DROP TRIGGER fail_brand_operation_audit ON audit_logs`); err != nil {
		t.Fatal(err)
	}
	// A transient failure does not seal a false receipt; retry original intent.
	mustStatus(t, f.call("PATCH", path, "brand-audit-fail-original-01", f.token, managedBrand, body), 200)
	mustStatus(t, f.call("GET", "/api/v1/me", "", f.userToken, managedBrand, nil), 200)
	wallet := pointWallet(t, f)
	if wallet.DisplayPoints != 0 {
		t.Fatal(wallet)
	}
	login := f.call("POST", "/api/v1/auth/login", "brand-paused-login-01", "", managedBrand, map[string]string{"identifier": "points_member", "password": "points-member-password-2026"})
	mustStatus(t, login, 200)
	var auth identity.Authentication
	managedData(t, login, &auth)
	if auth.Member.ID != f.memberID || auth.AccessToken == "" {
		t.Fatal("paused login unavailable")
	}
	ctx := f.call("GET", "/api/v1/context", "", "", managedBrand, nil)
	mustStatus(t, ctx, 200)
	var contextData struct {
		Brand struct {
			Status string `json:"status"`
		} `json:"brand"`
	}
	managedData(t, ctx, &contextData)
	if contextData.Brand.Status != "paused" {
		t.Fatal(contextData)
	}
	preview := f.call("POST", "/api/v1/bet-previews", "", auth.AccessToken, managedBrand, betting.Input{PeriodID: managedBrand, PlayID: managedBrand, RuleVersionID: managedBrand, Selection: rules.Selection{Special: []int{7}}, Multiplier: 1})
	mustStatus(t, preview, 403)
	var denied struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(preview.Body.Bytes(), &denied); err != nil || denied.Error.Code != "BRAND_PAUSED" {
		t.Fatal(denied, err)
	}
}

func TestBrandOperationSuperAdministratorNeedsExplicitPlatformPermission(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	const path = "/api/v1/platform/brand-operation"
	const other = "0199a000-0000-7000-8000-000000000002"
	if _, err := f.pool.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	platformToken := platformAdminToken(t, f)
	mustStatus(t, f.call("GET", path, "", platformToken, other, nil), 403)
	grantPlatformPermission(t, f, "brand_operation.view.platform")
	initial := f.call("GET", path, "", platformToken, other, nil)
	mustStatus(t, initial, 200)
	var state struct {
		Version int64 `json:"version"`
	}
	managedData(t, initial, &state)
	body := map[string]any{"version": state.Version, "status": "paused", "reason": "explicit platform operator pause"}
	mustStatus(t, f.call("PATCH", path, "brand-super-no-write-01", platformToken, other, body), 403)
	role := grantPlatformPermission(t, f, "brand_operation.write.platform")
	paused := f.call("PATCH", path, "brand-super-write-01", platformToken, other, body)
	mustStatus(t, paused, 200)
	var after struct {
		Version int64  `json:"version"`
		Status  string `json:"status"`
		AuditID string `json:"audit_log_id"`
	}
	managedData(t, paused, &after)
	if after.Version != state.Version+1 || after.Status != "paused" || after.AuditID == "" {
		t.Fatal("platform pause missing version or audit", after)
	}
	replay := f.call("PATCH", path, "brand-super-write-01", platformToken, other, body)
	mustStatus(t, replay, 200)
	var receipt struct {
		Version int64  `json:"version"`
		AuditID string `json:"audit_log_id"`
	}
	managedData(t, replay, &receipt)
	if receipt.Version != after.Version || receipt.AuditID != after.AuditID {
		t.Fatal("replay changed operation", receipt)
	}
	mustStatus(t, f.call("PATCH", path, "brand-super-stale-01", platformToken, other, body), 409)
	resume := map[string]any{"version": after.Version, "status": "active", "reason": "platform resume"}
	mustStatus(t, f.call("PATCH", path, "brand-super-resume-01", platformToken, other, resume), 200)
	var status string
	if err := f.pool.QueryRow(ctx, `SELECT status FROM brands WHERE id=$1`, managedBrand).Scan(&status); err != nil || status != "active" {
		t.Fatal("platform pause affected another brand", status, err)
	}
	mustStatus(t, f.call("PATCH", "/api/v1/admin/brand-operation", "platform-brand-entry-01", platformToken, other, body), 403)
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='brand_operation.write.platform'`, role); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("PATCH", path, "brand-super-write-01", platformToken, other, body), 403)
}

func TestBrandOperationHTTPHistoryFreshReplayAndIsolation(t *testing.T) {
	f := managedFixture(t)
	const path = "/api/v1/admin/brand-operation"
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 403)
	grantBrandOperation(t, f)
	initial := f.call("GET", path, "", f.token, managedBrand, nil)
	mustStatus(t, initial, 200)
	var current struct {
		BrandID string `json:"brand_id"`
		Version int64  `json:"version"`
		Status  string `json:"status"`
	}
	managedData(t, initial, &current)
	if current.BrandID != managedBrand || current.Status != "active" || current.Version < 1 {
		t.Fatal(current)
	}
	body := map[string]any{"version": current.Version, "status": "paused", "reason": "pause new bets while preserving existing workflows"}
	first := f.call("PATCH", path, "brand-pause-first-001", f.token, managedBrand, body)
	mustStatus(t, first, 200)
	var paused struct {
		BrandID string `json:"brand_id"`
		Version int64  `json:"version"`
		Status  string `json:"status"`
		AuditID string `json:"audit_log_id"`
	}
	managedData(t, first, &paused)
	if paused.Version != current.Version+1 || paused.Status != "paused" || paused.AuditID == "" {
		t.Fatal(paused)
	}
	replay := f.call("PATCH", path, "brand-pause-first-001", f.token, managedBrand, body)
	mustStatus(t, replay, 200)
	var repeated any
	var original any
	managedData(t, replay, &repeated)
	managedData(t, first, &original)
	rawRepeated, _ := json.Marshal(repeated)
	rawOriginal, _ := json.Marshal(original)
	if string(rawRepeated) != string(rawOriginal) {
		t.Fatal("cached receipt changed", repeated, original)
	}
	changed := map[string]any{"version": current.Version, "status": "paused", "reason": "different intent"}
	mustStatus(t, f.call("PATCH", path, "brand-pause-first-001", f.token, managedBrand, changed), 409)
	mustStatus(t, f.call("PATCH", path, "brand-pause-stale-001", f.token, managedBrand, body), 409)
	mustStatus(t, f.call("GET", path, "", f.token, "0199a000-0000-7000-8000-000000000002", nil), 403)
	mustStatus(t, f.call("GET", path+"/history?limit=101", "", f.token, managedBrand, nil), 400)
	mustStatus(t, f.call("GET", path+"/history?limit=2&limit=3", "", f.token, managedBrand, nil), 400)
	mustStatus(t, f.call("GET", path+"/history?unknown=x", "", f.token, managedBrand, nil), 400)
	mustStatus(t, f.rawCall("PATCH", path, "brand-pause-duplicate-01", f.token, managedBrand, `{"version":2,"version":2,"status":"paused","reason":"invalid"}`), 400)
	mustStatus(t, f.call("PATCH", path, "brand-pause-disabled-01", f.token, managedBrand, map[string]any{"version": paused.Version, "status": "disabled", "reason": "not offered"}), 400)
	resume := f.call("PATCH", path, "brand-resume-first-001", f.token, managedBrand, map[string]any{"version": paused.Version, "status": "active", "reason": "resume new bets"})
	mustStatus(t, resume, 200)
	// The old successful receipt remains the original pause, not current state.
	mustStatus(t, f.call("PATCH", path, "brand-pause-first-001", f.token, managedBrand, body), 200)
	latest := f.call("GET", path, "", f.token, managedBrand, nil)
	mustStatus(t, latest, 200)
	managedData(t, latest, &current)
	if current.Status != "active" || current.Version != paused.Version+1 {
		t.Fatal(current)
	}
	history := f.call("GET", path+"/history?limit=20&offset=0", "", f.token, managedBrand, nil)
	mustStatus(t, history, 200)
	var page struct {
		Items []struct {
			Version   int64  `json:"version"`
			Status    string `json:"status"`
			ChangedBy string `json:"changed_by"`
		} `json:"items"`
	}
	managedData(t, history, &page)
	if len(page.Items) != 2 || page.Items[0].Status != "active" || page.Items[1].Status != "paused" || page.Items[0].ChangedBy != f.root {
		t.Fatal(page)
	}
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='brand_operation.write.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("PATCH", path, "brand-pause-first-001", f.token, managedBrand, body), 403)
	var ledger, orders int
	if err := f.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM point_ledger_entries),(SELECT count(*) FROM bet_orders)`).Scan(&ledger, &orders); err != nil || ledger != 0 || orders != 0 {
		t.Fatal(ledger, orders, err)
	}
}
