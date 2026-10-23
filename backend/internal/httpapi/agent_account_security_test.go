package httpapi

import (
	"context"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
)

func TestAgentPasswordAndCommissionStayInIndependentAccountScope(t *testing.T) {
	f := pointsFixture(t)
	agentGrantRoot(t, f.managementHTTP)
	ctx := context.Background()
	create := func(username string) agency.Node {
		t.Helper()
		out := f.call("POST", "/api/v1/admin/agents", "security-create-"+username, f.token, managedBrand, agency.CreateInput{PolicyVersion: 1, Username: username, Password: "agent123", Config: agency.NodeConfig{Mode: new("loss"), Ratio: "0", Status: "active"}})
		mustStatus(t, out, 201)
		var node agency.Node
		managedData(t, out, &node)
		return node
	}
	login := func(username, password string) string {
		t.Helper()
		out := f.call("POST", "/api/v1/b/aurora/agent/login", "", "", "", identity.AgentLoginInput{Username: username, Password: password})
		mustStatus(t, out, 200)
		for _, c := range out.Result().Cookies() {
			if c.Name == agentCookieName(managedBrand) {
				return c.Value
			}
		}
		t.Fatal("agent cookie missing")
		return ""
	}
	own, other := create("security_agent"), create("security_other")
	for _, seed := range []struct{ member, source, amount, key string }{{own.MemberID, "commission", "7", "own-commission"}, {other.MemberID, "commission", "11", "other-commission"}, {own.MemberID, "recharge", "100", "own-recharge"}} {
		mustStatus(t, f.call("POST", "/api/v1/admin/wallets/"+seed.member+"/adjust", seed.key, f.token, managedBrand, map[string]string{"source": seed.source, "delta": seed.amount, "reason": "independent commission scope fixture"}), 200)
	}
	token := login("security_agent", "agent123")
	result := f.call("GET", "/api/v1/b/aurora/agent/commission?limit=20&offset=0", "", token, "", nil)
	mustStatus(t, result, 200)
	var postings struct {
		Total    string `json:"total_count"`
		Credited string `json:"credited_points"`
		Debited  string `json:"debited_points"`
		Items    []struct {
			Points string `json:"points"`
		} `json:"items"`
	}
	managedData(t, result, &postings)
	if postings.Total != "1" || postings.Credited != "7" || postings.Debited != "0" || len(postings.Items) != 1 || postings.Items[0].Points != "7" {
		t.Fatalf("commission scope leaked or mixed sources: %+v", postings)
	}
	for _, private := range []string{other.MemberID, "source_allocation", "request_id\":\"test", "password_hash", "delta_snapshot"} {
		if strings.Contains(result.Body.String(), private) {
			t.Fatalf("private commission evidence exposed: %s", private)
		}
	}
	mustStatus(t, f.call("GET", "/api/v1/b/harbor/agent/commission", "", token, "", nil), 401)
	mustStatus(t, f.call("GET", "/api/v1/b/aurora/agent/commission?member_id="+other.MemberID, "", token, "", nil), 400)
	path := "/api/v1/b/aurora/agent/password"
	body := identity.AgentPasswordInput{CurrentPassword: "agent123", NewPassword: "changed456"}
	mustStatus(t, f.call("POST", path, "password-wrong-old", token, "", identity.AgentPasswordInput{CurrentPassword: "wrong123", NewPassword: "changed456"}), 400)
	mustStatus(t, f.call("GET", "/api/v1/b/aurora/agent/me", "", token, "", nil), 200)
	mustStatus(t, f.call("POST", path, "password-invalid-new", token, "", identity.AgentPasswordInput{CurrentPassword: "agent123", NewPassword: "short"}), 400)
	mustStatus(t, pointsCall(f.managementHTTP, "POST", path, "password-csrf", token, "", "https://foreign.example", body), 403)
	if _, err := f.pool.Exec(ctx, `CREATE FUNCTION reject_agent_password_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='agent.password_change' THEN RAISE EXCEPTION 'owned test audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_agent_password_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_agent_password_audit()`); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("POST", path, "password-rollback-retry", token, "", body), 503)
	mustStatus(t, f.call("GET", "/api/v1/b/aurora/agent/me", "", token, "", nil), 200)
	if _, err := f.pool.Exec(ctx, `DROP TRIGGER reject_agent_password_audit ON audit_logs`); err != nil {
		t.Fatal(err)
	}
	changed := f.call("POST", path, "password-rollback-retry", token, "", body)
	mustStatus(t, changed, 200)
	var audits, active int
	if err := f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM audit_logs WHERE brand_id=$1 AND action='agent.password_change' AND actor_type='agent' AND actor_id=$2 AND before_json='{"password":"redacted"}'::jsonb AND after_json='{"password":"redacted"}'::jsonb),(SELECT count(*) FROM agent_sessions WHERE brand_id=$1 AND agent_account_id=$2 AND revoked_at IS NULL)`, managedBrand, own.MemberID).Scan(&audits, &active); err != nil || audits != 1 || active != 0 {
		t.Fatal("password audit or session revocation missing", audits, active, err)
	}
	mustStatus(t, f.call("POST", path, "password-rollback-retry", token, "", body), 401)
	mustStatus(t, f.call("GET", "/api/v1/b/aurora/agent/me", "", token, "", nil), 401)
	mustStatus(t, f.call("POST", "/api/v1/b/aurora/agent/login", "", "", "", identity.AgentLoginInput{Username: "security_agent", Password: "agent123"}), 401)
	newToken := login("security_agent", "changed456")
	mustStatus(t, f.call("GET", "/api/v1/b/aurora/agent/me", "", newToken, "", nil), 200)
	_ = login("security_other", "agent123")
	var ledgerCount int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries`).Scan(&ledgerCount); err != nil || ledgerCount != 3 {
		t.Fatal("password changed funds", ledgerCount, err)
	}
}
