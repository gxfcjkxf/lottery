package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/commission"
)

const commissionPolicyPath = "/api/v1/admin/commission-policy"

func grantCommissionPolicy(t *testing.T, f managementHTTP) {
	t.Helper()
	ctx := context.Background()
	for _, key := range []string{"commission_policy.view.brand", "commission_policy.write.brand"} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, key); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,$2 FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root, key); err != nil {
			t.Fatal(err)
		}
	}
}

func enabledCommissionPolicyConfig() commission.PolicyConfig {
	weekday := 1
	return commission.PolicyConfig{
		Enabled: true,
		Calendar: &commission.Calendar{
			Timezone: "UTC", Cycle: "weekly", BoundaryTime: "00:00:00", Weekday: &weekday,
		},
		PayoutMode: commission.PayoutManual,
	}
}

func TestCommissionPolicyHTTPAuthorizationReplayWithoutHistory(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	grantCommissionPolicy(t, f)
	agentGrantRoot(t, f)
	if got := f.call("PUT", "/api/v1/admin/agent-policy", "commission-agent-weekly-01", f.token, managedBrand, agentPolicyBody(true, 3, "0.1")); got.Code != 200 {
		t.Fatalf("enable weekly agency evidence: %d %s", got.Code, got.Body.String())
	}
	member := newNormalAgentMember(t, f, "commission-disabled-agent")
	createdAgent := f.call("POST", "/api/v1/admin/agents", "commission-agent-create-001", f.token, managedBrand, agentNodeBody(2, member, nil, nil, "0.05"))
	mustStatus(t, createdAgent, 201)
	var node agentNode
	managedData(t, createdAgent, &node)
	disableAgent := map[string]any{
		"version": node.Version, "policy_version": node.PolicyVersion, "parent_version": nil,
		"config": map[string]any{"ratio": "0.05", "mode": "loss", "status": "disabled", "can_create_children": true},
		"reason": "manually disable agent",
	}
	mustStatus(t, f.call("PUT", "/api/v1/admin/agents/"+node.ID, "commission-agent-disable-001", f.token, managedBrand, disableAgent), 200)

	initialResponse := f.call("GET", commissionPolicyPath, "", f.token, managedBrand, nil)
	mustStatus(t, initialResponse, 200)
	var initial commission.Policy
	managedData(t, initialResponse, &initial)
	if initial.Version != 1 || initial.Config.Enabled || initial.RevisionID == "" {
		t.Fatalf("unexpected initial policy: %+v", initial)
	}

	body := commission.PolicyInput{Version: initial.Version, Config: enabledCommissionPolicyConfig(), Reason: "enable weekly commission policy"}
	first := f.call("PUT", commissionPolicyPath, "commission-policy-update-001", f.token, managedBrand, body)
	mustStatus(t, first, 200)
	var updated commission.Policy
	managedData(t, first, &updated)
	if updated.Version != 2 || !updated.Config.Enabled || updated.Config.Calendar == nil || updated.Config.Calendar.Cycle != "weekly" {
		t.Fatalf("unexpected updated policy: %+v", updated)
	}

	replay := f.call("PUT", commissionPolicyPath, "commission-policy-update-001", f.token, managedBrand, body)
	mustStatus(t, replay, 200)
	var repeated commission.Policy
	managedData(t, replay, &repeated)
	if repeated.Version != updated.Version || repeated.RevisionID != updated.RevisionID {
		t.Fatalf("replay was not stable: %+v vs %+v", repeated, updated)
	}
	changed := body
	changed.Reason = "different request under same key"
	mustStatus(t, f.call("PUT", commissionPolicyPath, "commission-policy-update-001", f.token, managedBrand, changed), 409)
	mustStatus(t, f.call("PUT", commissionPolicyPath, "commission-policy-stale-001", f.token, managedBrand, body), 409)
	mustStatus(t, f.call("GET", commissionPolicyPath, "", f.token, "0199a000-0000-7000-8000-000000000002", nil), 403)

	mustStatus(t, f.call("GET", commissionPolicyPath+"/history", "", f.token, managedBrand, nil), 404)
	mustStatus(t, f.call("GET", "/api/v1/admin/agent-policy/history", "", f.token, managedBrand, nil), 404)

	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN (SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='commission_policy.write.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("PUT", commissionPolicyPath, "commission-policy-update-001", f.token, managedBrand, body), 403)
	if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,'commission_policy.write.brand' FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN (SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='commission_policy.view.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	platformViewer := grantPlatformPermission(t, f, "commission_policy.view.platform")
	mustStatus(t, f.call("GET", commissionPolicyPath, "", f.token, managedBrand, nil), 403)
	if _, err := f.pool.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	platformToken := platformAdminToken(t, f)
	f.token = platformToken
	platformPath := strings.Replace(commissionPolicyPath, "/api/v1/admin", "/api/v1/platform", 1)
	mustStatus(t, f.call("GET", platformPath, "", platformToken, managedBrand, nil), 200)
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='commission_policy.view.platform'`, platformViewer); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("GET", platformPath, "", platformToken, managedBrand, nil), 403)
	if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,'commission_policy.view.brand' FROM admin_account_roles WHERE account_id=$1 AND role_id IN (SELECT id FROM roles WHERE brand_id=$2) ON CONFLICT DO NOTHING`, f.root, managedBrand); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("GET", platformPath, "", platformToken, managedBrand, nil), 403)
	grantPlatformPermission(t, f, "commission_policy.view.platform")
	mustStatus(t, f.call("GET", commissionPolicyPath, "", f.token, managedBrand, nil), 403)
	mustStatus(t, f.call("GET", platformPath, "", platformToken, managedBrand, nil), 200)
	mustStatus(t, f.call("PUT", commissionPolicyPath, "commission-policy-update-001", f.token, managedBrand, body), 403)

	var updates int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE brand_id=$1 AND action='commission.policy.update'`, managedBrand).Scan(&updates); err != nil {
		t.Fatal(err)
	}
	if updates != 0 {
		t.Fatalf("policy history was recorded: %d", updates)
	}
}

func TestCommissionPolicyHTTPStrictBodyAndEvidenceRollback(t *testing.T) {
	f := managedFixture(t)
	grantCommissionPolicy(t, f)
	agentGrantRoot(t, f)
	if got := f.call("PUT", "/api/v1/admin/agent-policy", "commission-agent-weekly-02", f.token, managedBrand, agentPolicyBody(true, 3, "0.1")); got.Code != 200 {
		t.Fatal(got.Code, got.Body.String())
	}
	invalidBody := `{"version":1,"version":1,"config":{"enabled":true,"calendar":{"timezone":"UTC","cycle":"weekly","boundary_time":"00:00:00","weekday":1,"month_day":null,"short_month":""},"payout_mode":"manual"},"reason":"duplicate key"}`
	mustStatus(t, f.rawCall("PUT", commissionPolicyPath, "commission-policy-invalid-001", f.token, managedBrand, invalidBody), 400)

	wrongCalendar := enabledCommissionPolicyConfig()
	wrongCalendar.Calendar.Cycle = "monthly"
	day := 1
	wrongCalendar.Calendar.Weekday = nil
	wrongCalendar.Calendar.MonthDay = &day
	wrongCalendar.Calendar.ShortMonth = "last_day"
	wrong := commission.PolicyInput{Version: 2, Config: wrongCalendar, Reason: "reject stale policy version"}
	mustStatus(t, f.call("PUT", commissionPolicyPath, "commission-policy-evidence-001", f.token, managedBrand, wrong), 409)
	var version, updates int
	if err := f.pool.QueryRow(context.Background(), `SELECT (SELECT version FROM brand_commission_policies WHERE brand_id=$1),(SELECT count(*) FROM audit_logs WHERE brand_id=$1 AND action='commission.policy.update')`, managedBrand).Scan(&version, &updates); err != nil {
		t.Fatal(err)
	}
	if version != 1 || updates != 0 {
		t.Fatalf("rejected evidence write was not rolled back: version=%d updates=%d", version, updates)
	}

	// Verify the HTTP response remains the standard envelope on a cached rejection.
	raw := f.call("PUT", commissionPolicyPath, "commission-policy-evidence-001", f.token, managedBrand, wrong)
	mustStatus(t, raw, 409)
	var envelope struct {
		Success bool `json:"success"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw.Body.Bytes(), &envelope); err != nil || envelope.Success || envelope.Error.Code != "COMMISSION_POLICY_VERSION_CONFLICT" {
		t.Fatalf("unexpected rejection envelope: %s (%v)", raw.Body.String(), err)
	}

	valid := commission.PolicyInput{Version: 1, Config: enabledCommissionPolicyConfig(), Reason: "retry after audit subsystem recovers"}
	if _, err := f.pool.Exec(context.Background(), `CREATE FUNCTION fail_commission_policy_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='commission.policy.update' THEN RAISE EXCEPTION 'test audit failure'; END IF; RETURN NEW; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `CREATE TRIGGER fail_commission_policy_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION fail_commission_policy_audit()`); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("PUT", commissionPolicyPath, "commission-policy-audit-retry-001", f.token, managedBrand, valid), 200)
	if err := f.pool.QueryRow(context.Background(), `SELECT (SELECT version FROM brand_commission_policies WHERE brand_id=$1),(SELECT count(*) FROM audit_logs WHERE brand_id=$1 AND action='commission.policy.update')`, managedBrand).Scan(&version, &updates); err != nil {
		t.Fatal(err)
	}
	if version != 2 || updates != 0 {
		t.Fatalf("policy update unexpectedly depended on history: version=%d updates=%d", version, updates)
	}
	if _, err := f.pool.Exec(context.Background(), `DROP TRIGGER fail_commission_policy_audit ON audit_logs`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `DROP FUNCTION fail_commission_policy_audit()`); err != nil {
		t.Fatal(err)
	}
	retried := f.call("PUT", commissionPolicyPath, "commission-policy-audit-retry-001", f.token, managedBrand, valid)
	mustStatus(t, retried, 200)
	var committed commission.Policy
	managedData(t, retried, &committed)
	if committed.Version != 2 {
		t.Fatalf("retry after transient audit failure was not executed: %+v", committed)
	}
}

func TestCommissionPolicyCanEnableOnDefaultAgentRules(t *testing.T) {
	f := managedFixture(t)
	grantCommissionPolicy(t, f)
	day := 1
	config := commission.PolicyConfig{Enabled: true, Calendar: &commission.Calendar{Timezone: "UTC", Cycle: "monthly", BoundaryTime: "00:00:00", MonthDay: &day, ShortMonth: "last_day"}, PayoutMode: commission.PayoutManual}
	mustStatus(t, f.call("PUT", commissionPolicyPath, "commission-default-agency-enable-001", f.token, managedBrand, commission.PolicyInput{Version: 1, Config: config}), 200)
	mustStatus(t, f.call("PUT", commissionPolicyPath, "commission-default-agency-disable-001", f.token, managedBrand, commission.PolicyInput{Version: 2, Config: commission.DefaultPolicyConfig()}), 200)
	var version int
	var enabled bool
	if err := f.pool.QueryRow(context.Background(), `SELECT version,(config->>'enabled')::boolean FROM brand_agent_policies WHERE brand_id=$1`, managedBrand).Scan(&version, &enabled); err != nil || version != 1 || !enabled {
		t.Fatalf("commission toggle changed agent rules: %d %v %v", version, enabled, err)
	}
}

func TestCommissionCalendarSynchronizesAgentCycleWithoutSecondSetting(t *testing.T) {
	f := managedFixture(t)
	grantCommissionPolicy(t, f)
	config := enabledCommissionPolicyConfig()
	mustStatus(t, f.call("PUT", commissionPolicyPath, "single-calendar-weekly-001", f.token, managedBrand, commission.PolicyInput{Version: 1, Config: config}), 200)
	var cycle string
	var version int
	read := func() {
		t.Helper()
		if err := f.pool.QueryRow(context.Background(), `SELECT config->>'cycle',version FROM brand_agent_policies WHERE brand_id=$1`, managedBrand).Scan(&cycle, &version); err != nil {
			t.Fatal(err)
		}
	}
	read()
	if cycle != "weekly" || version != 2 {
		t.Fatalf("derived agent cycle: %s %d", cycle, version)
	}
	// The source remains the financial calendar even while commission is disabled.
	config.Enabled = false
	mustStatus(t, f.call("PUT", commissionPolicyPath, "single-calendar-disabled-001", f.token, managedBrand, commission.PolicyInput{Version: 2, Config: config}), 200)
	agentGrantRoot(t, f)
	body := agentPolicyBody(true, 5, "0")
	body["version"] = 2
	body["config"].(map[string]any)["cycle"] = "monthly"
	mustStatus(t, f.call("PUT", "/api/v1/admin/agent-policy", "reject-second-cycle-setting-001", f.token, managedBrand, body), 409)
	day := 1
	config.Calendar = &commission.Calendar{Timezone: "UTC", Cycle: "monthly", BoundaryTime: "00:00:00", MonthDay: &day, ShortMonth: "last_day"}
	mustStatus(t, f.call("PUT", commissionPolicyPath, "single-calendar-monthly-001", f.token, managedBrand, commission.PolicyInput{Version: 3, Config: config}), 200)
	read()
	if cycle != "monthly" || version != 3 {
		t.Fatalf("derived monthly cycle: %s %d", cycle, version)
	}
}
