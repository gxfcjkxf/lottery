package httpapi

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
)

type agentPolicy = agency.Policy
type agentNode = agency.Node
type agentRevision = agency.Revision

func agentGrantRoot(t *testing.T, f managementHTTP) {
	t.Helper()
	var role string
	if err := f.pool.QueryRow(context.Background(), `SELECT role_id::text FROM admin_account_roles WHERE account_id=$1 LIMIT 1`, f.root).Scan(&role); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"agent_policy.view.brand", "agent_policy.write.brand", "agent.view.brand", "agent.write.brand", "agent_policy.view.platform", "agent.view.platform"} {
		if _, err := f.pool.Exec(context.Background(), `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, key); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(context.Background(), `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2) ON CONFLICT DO NOTHING`, role, key); err != nil {
			t.Fatal(err)
		}
	}
}

func newNormalAgentMember(t *testing.T, f managementHTTP, suffix string) string {
	return newNormalAgentMemberInBrand(t, f, managedBrand, suffix)
}

func newNormalAgentMemberInBrand(t *testing.T, f managementHTTP, brand, suffix string) string {
	t.Helper()
	ctx := context.Background()
	user, member := ids.New(), ids.New()
	if _, err := f.pool.Exec(ctx, `INSERT INTO global_users(id,username) VALUES($1,$2)`, user, "agent_test_"+suffix); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version,status,terms_accepted) VALUES($1,$2,$3,'operator','dev-1','dev-1','normal',true)`, member, brand, user); err != nil {
		t.Fatal(err)
	}
	return member
}

func agentPolicyBody(enabled bool, maxDepth int, cap string) map[string]any {
	return map[string]any{"version": 1, "config": map[string]any{"enabled": enabled, "max_depth": maxDepth, "loss_ratio_cap": cap, "turnover_ratio_cap": cap, "mode": "loss", "cycle": "weekly"}, "reason": "agent policy test"}
}
func agentNodeBody(policyVersion int64, member string, parent *string, parentVersion *int64, ratio string) map[string]any {
	return map[string]any{"policy_version": policyVersion, "username": testdb.AgentUsername(member), "password": "agent123", "parent_id": parent, "parent_version": parentVersion, "config": map[string]any{"ratio": ratio, "mode": "loss", "status": "active", "can_create_children": true}, "reason": "agent node test"}
}

func TestAgentAdminPolicyTreeHistoryAndLimits(t *testing.T) {
	f := pointsFixture(t)
	agentGrantRoot(t, f.managementHTTP)
	beforeAccounts := 0
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM point_accounts`).Scan(&beforeAccounts); err != nil {
		t.Fatal(err)
	}
	defaultPolicy := f.call("GET", "/api/v1/admin/agent-policy", "", f.token, managedBrand, nil)
	if defaultPolicy.Code != 200 {
		t.Fatal(defaultPolicy.Code, defaultPolicy.Body.String())
	}
	var p agentPolicy
	managedData(t, defaultPolicy, &p)
	if !p.Config.Enabled || p.Config.MaxDepth != 5 || p.Config.LossRatioCap != "0" || p.Config.Mode != "loss" || p.Config.Cycle != "monthly" {
		t.Fatalf("bad default policy: %+v", p)
	}
	mustStatus(t, f.call("PUT", "/api/v1/admin/agent-policy", "agent-policy-disable-removed-001", f.token, managedBrand, agentPolicyBody(false, 5, "0")), 400)
	if got := f.call("PUT", "/api/v1/admin/agent-policy", "agent-policy-enable-001", f.token, managedBrand, agentPolicyBody(true, 3, "0.1")); got.Code != 200 {
		t.Fatal(got.Code, got.Body.String())
	}
	rootBody := agentNodeBody(2, f.memberID, nil, nil, "0.08")
	created := f.call("POST", "/api/v1/admin/agents", "agent-create-root-001", f.token, managedBrand, rootBody)
	if created.Code != 201 {
		t.Fatal(created.Code, created.Body.String())
	}
	var root agentNode
	managedData(t, created, &root)
	if root.Depth != 1 || len(root.Path) != 1 || root.EffectiveMode != "loss" || root.Version != 1 || root.PolicyVersion != 2 {
		t.Fatalf("bad root: %+v", root)
	}
	childMember := newNormalAgentMember(t, f.managementHTTP, "child")
	childBody := agentNodeBody(2, childMember, &root.ID, &root.Version, "0.04")
	childResp := f.call("POST", "/api/v1/admin/agents", "agent-create-child-001", f.token, managedBrand, childBody)
	if childResp.Code != 201 {
		t.Fatal(childResp.Code, childResp.Body.String())
	}
	var child agentNode
	managedData(t, childResp, &child)
	if child.Depth != 2 || len(child.Path) != 2 || child.ParentVersion == nil || *child.ParentVersion != 1 {
		t.Fatalf("bad child: %+v", child)
	}
	tooHigh := agentNodeBody(2, newNormalAgentMember(t, f.managementHTTP, "high"), &root.ID, &root.Version, "0.19")
	if resp := f.call("POST", "/api/v1/admin/agents", "agent-create-too-high-001", f.token, managedBrand, tooHigh); resp.Code != 409 {
		t.Fatalf("ratio above parent plus cap status=%d body=%s", resp.Code, resp.Body.String())
	}
	foreignMember := newNormalAgentMemberInBrand(t, f.managementHTTP, pointsBrandB, "foreign")
	if resp := f.call("POST", "/api/v1/admin/agents", "agent-create-foreign-member-001", f.token, managedBrand, agentNodeBody(2, foreignMember, nil, nil, "0.01")); resp.Code != 201 {
		t.Fatalf("cross-brand member status=%d body=%s", resp.Code, resp.Body.String())
	}
	missingParent := ids.New()
	if resp := f.call("POST", "/api/v1/admin/agents", "agent-create-foreign-parent-001", f.token, managedBrand, agentNodeBody(2, newNormalAgentMember(t, f.managementHTTP, "missingparent"), &missingParent, func() *int64 { v := int64(1); return &v }(), "0.01")); resp.Code != 404 {
		t.Fatalf("cross-brand or absent parent status=%d body=%s", resp.Code, resp.Body.String())
	}
	if resp := f.call("GET", "/api/v1/admin/agents/tree?parent_id="+root.ID, "", f.token, managedBrand, nil); resp.Code != 200 {
		t.Fatal(resp.Code, resp.Body.String())
	}
	if resp := f.call("GET", "/api/v1/admin/agents/tree?parent_id=0199a000-0000-7000-8000-000000000099", "", f.token, managedBrand, nil); resp.Code != 404 {
		t.Fatalf("false parent status=%d body=%s", resp.Code, resp.Body.String())
	}
	changed := map[string]any{"version": 1, "policy_version": 2, "parent_version": nil, "config": map[string]any{"ratio": "0.01", "mode": "loss", "status": "active", "can_create_children": true}, "reason": "lower parent"}
	if resp := f.call("PUT", "/api/v1/admin/agents/"+root.ID, "agent-update-parent-lower-001", f.token, managedBrand, changed); resp.Code != 409 {
		t.Fatalf("parent lowering invalidating child status=%d body=%s", resp.Code, resp.Body.String())
	}
	shallower := agentPolicyBody(true, 2, "0.1")
	shallower["version"] = 2
	if resp := f.call("PUT", "/api/v1/admin/agent-policy", "agent-policy-lower-depth-001", f.token, managedBrand, shallower); resp.Code != 200 {
		t.Fatalf("lower policy depth within existing tree status=%d body=%s", resp.Code, resp.Body.String())
	}
	childVersion := child.Version
	tooDeep := agentNodeBody(3, newNormalAgentMember(t, f.managementHTTP, "toodeep"), &child.ID, &childVersion, "0.02")
	if resp := f.call("POST", "/api/v1/admin/agents", "agent-create-too-deep-001", f.token, managedBrand, tooDeep); resp.Code != 409 {
		t.Fatalf("beyond policy max depth status=%d body=%s", resp.Code, resp.Body.String())
	}
	disableRoot := map[string]any{"version": 1, "policy_version": 3, "parent_version": nil, "config": map[string]any{"ratio": "0.08", "mode": "loss", "status": "disabled", "can_create_children": true}, "reason": "disable root for admission test"}
	if resp := f.call("PUT", "/api/v1/admin/agents/"+root.ID, "agent-disable-root-admission-001", f.token, managedBrand, disableRoot); resp.Code != 200 {
		t.Fatalf("disable root status=%d body=%s", resp.Code, resp.Body.String())
	}
	rootVersion := int64(2)
	blockedChild := agentNodeBody(3, newNormalAgentMember(t, f.managementHTTP, "inactiveparent"), &root.ID, &rootVersion, "0.02")
	if resp := f.call("POST", "/api/v1/admin/agents", "agent-create-inactive-parent-001", f.token, managedBrand, blockedChild); resp.Code != 409 {
		t.Fatalf("inactive parent admission status=%d body=%s", resp.Code, resp.Body.String())
	}
	history := f.call("GET", "/api/v1/admin/agents/"+child.ID+"/history", "", f.token, managedBrand, nil)
	if history.Code != 200 {
		t.Fatal(history.Code, history.Body.String())
	}
	var page struct {
		Items   []agentRevision `json:"items"`
		AgentID string          `json:"agent_id"`
	}
	managedData(t, history, &page)
	if page.AgentID != child.ID || len(page.Items) != 1 || page.Items[0].Version != 1 {
		t.Fatalf("child history mismatch: %+v", page)
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE agent_config_revisions SET reason='mutated' WHERE id=$1`, page.Items[0].ID); err == nil {
		t.Fatal("revision update unexpectedly succeeded")
	}
	afterAccounts := 0
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM point_accounts`).Scan(&afterAccounts); err != nil {
		t.Fatal(err)
	}
	if afterAccounts != beforeAccounts+3 {
		t.Fatalf("agent writes changed point-account count %d -> %d", beforeAccounts, afterAccounts)
	}
	mustStatus(t, f.call("GET", "/api/v1/admin/agent-policy/history", "", f.token, managedBrand, nil), 404)
	var historyCount int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM agent_config_revisions WHERE brand_id=$1 AND agent_id IS NULL`, managedBrand).Scan(&historyCount); err != nil || historyCount != 0 {
		t.Fatalf("brand policy history was recorded: %d %v", historyCount, err)
	}

}

func TestAgentPublicCanOnlyUpdateDirectChildAndRechecksSession(t *testing.T) {
	f := pointsFixture(t)
	agentGrantRoot(t, f.managementHTTP)
	if r := f.call("PUT", "/api/v1/admin/agent-policy", "agent-public-policy-001", f.token, managedBrand, agentPolicyBody(true, 3, "0.1")); r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	rootResp := f.call("POST", "/api/v1/admin/agents", "agent-public-root-001", f.token, managedBrand, agentNodeBody(2, f.memberID, nil, nil, "0.08"))
	if rootResp.Code != 201 {
		t.Fatal(rootResp.Code, rootResp.Body.String())
	}
	var root agentNode
	managedData(t, rootResp, &root)
	childMember := newNormalAgentMember(t, f.managementHTTP, "publicchild")
	childResp := f.call("POST", "/api/v1/admin/agents", "agent-public-child-001", f.token, managedBrand, agentNodeBody(2, childMember, &root.ID, &root.Version, "0.04"))
	if childResp.Code != 201 {
		t.Fatal(childResp.Code, childResp.Body.String())
	}
	var child agentNode
	managedData(t, childResp, &child)
	login := f.call("POST", "/api/v1/agent/login", "", "", managedBrand, identity.AgentLoginInput{Username: testdb.AgentUsername(f.memberID), Password: "agent123"})
	mustStatus(t, login, 200)
	var agentToken string
	for _, cookie := range login.Result().Cookies() {
		if cookie.Name == agentCookieName(managedBrand) {
			agentToken = cookie.Value
		}
	}
	path := "/api/v1/agent/children/" + child.ID + "/config"
	wrong := map[string]any{"version": 1, "policy_version": 2, "parent_version": 1, "ratio": "0.03", "mode": "turnover", "reason": "reject mode differing from parent"}
	var beforeRevisions, afterRevisions int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM agent_config_revisions WHERE agent_id=$1`, child.ID).Scan(&beforeRevisions); err != nil {
		t.Fatal(err)
	}
	if denied := pointsCall(f.managementHTTP, "PUT", path, "agent-user-wrong-mode-001", agentToken, managedBrand, "", wrong); denied.Code != 400 {
		t.Fatalf("mixed child mode accepted: %d %s", denied.Code, denied.Body.String())
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM agent_config_revisions WHERE agent_id=$1`, child.ID).Scan(&afterRevisions); err != nil || afterRevisions != beforeRevisions {
		t.Fatal("rejected child mode wrote a revision", beforeRevisions, afterRevisions, err)
	}
	body := map[string]any{"version": 1, "policy_version": 2, "parent_version": 1, "ratio": "0.03", "mode": "loss", "reason": "direct child adjustment"}
	updated := pointsCall(f.managementHTTP, "PUT", path, "agent-user-child-config-001", agentToken, managedBrand, "", body)
	if updated.Code != 200 {
		t.Fatal(updated.Code, updated.Body.String())
	}
	var got agentNode
	managedData(t, updated, &got)
	if got.Config.Ratio != "0.03" || got.Config.Mode == nil || *got.Config.Mode != "loss" || got.EffectiveMode != root.EffectiveMode || got.Config.Status != "active" || !got.Config.CanCreateChildren {
		t.Fatalf("unexpected user child update: %+v", got)
	}
	closedFields := map[string]any{"version": 2, "policy_version": 2, "parent_version": 1, "ratio": "0.03", "mode": "turnover", "status": "disabled", "reason": "attempt status change"}
	if invalid := pointsCall(f.managementHTTP, "PUT", path, "agent-user-status-field-001", agentToken, managedBrand, "", closedFields); invalid.Code != 400 {
		t.Fatalf("public status field accepted: status=%d body=%s", invalid.Code, invalid.Body.String())
	}
	self := pointsCall(f.managementHTTP, "PUT", "/api/v1/agent/children/"+root.ID+"/config", "agent-user-own-rate-001", agentToken, managedBrand, "", map[string]any{"version": 1, "policy_version": 2, "parent_version": 1, "ratio": "0.02", "mode": nil, "reason": "own rate"})
	if self.Code != 403 {
		t.Fatalf("self rate status=%d body=%s", self.Code, self.Body.String())
	}
	read := pointsCall(f.managementHTTP, "GET", "/api/v1/agent/me", "", agentToken, managedBrand, "", nil)
	if read.Code != 200 {
		t.Fatal(read.Code, read.Body.String())
	}
	var selfNode struct {
		Agent agentNode `json:"agent"`
	}
	managedData(t, read, &selfNode)
	if selfNode.Agent.ID != root.ID {
		t.Fatalf("unexpected /agent/me: %+v", selfNode)
	}
	rootUpdate := map[string]any{"version": 1, "policy_version": 2, "parent_version": nil, "config": map[string]any{"ratio": "0.08", "mode": "loss", "status": "disabled", "can_create_children": true}, "reason": "disable owner agent"}
	if disabled := f.call("PUT", "/api/v1/admin/agents/"+root.ID, "agent-disable-owner-001", f.token, managedBrand, rootUpdate); disabled.Code != 200 {
		t.Fatalf("disable owner agent status=%d body=%s", disabled.Code, disabled.Body.String())
	}
	ownerDisabledReplay := pointsCall(f.managementHTTP, "PUT", path, "agent-user-child-config-001", agentToken, managedBrand, "", body)
	if ownerDisabledReplay.Code != 401 {
		t.Fatalf("cached child receipt after owner deactivation status=%d body=%s", ownerDisabledReplay.Code, ownerDisabledReplay.Body.String())
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE brand_members SET status='disabled' WHERE id=$1`, f.memberID); err != nil {
		t.Fatal(err)
	}
	replay := pointsCall(f.managementHTTP, "PUT", path, "agent-user-child-config-001", agentToken, managedBrand, "", body)
	if replay.Code != 401 {
		t.Fatalf("revoked member replay status=%d body=%s", replay.Code, replay.Body.String())
	}
	var raw map[string]any
	if err := json.Unmarshal(replay.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
}

func TestAgentAdminIdempotencyFreshGrantWithoutPolicyHistory(t *testing.T) {
	f := pointsFixture(t)
	agentGrantRoot(t, f.managementHTTP)
	ctx := context.Background()
	var role string
	if err := f.pool.QueryRow(ctx, `SELECT role_id::text FROM admin_account_roles WHERE account_id=$1 LIMIT 1`, f.root).Scan(&role); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='agent_policy.write.brand'`, role); err != nil {
		t.Fatal(err)
	}
	body := agentPolicyBody(true, 3, "0.1")
	if denied := f.call("PUT", "/api/v1/admin/agent-policy", "agent-no-write-grant-001", f.token, managedBrand, body); denied.Code != 403 {
		t.Fatalf("without write grant status=%d body=%s", denied.Code, denied.Body.String())
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,'agent_policy.write.brand')`, role); err != nil {
		t.Fatal(err)
	}
	first := f.call("PUT", "/api/v1/admin/agent-policy", "agent-policy-replay-001", f.token, managedBrand, body)
	if first.Code != 200 {
		t.Fatal(first.Code, first.Body.String())
	}
	replay := f.call("PUT", "/api/v1/admin/agent-policy", "agent-policy-replay-001", f.token, managedBrand, body)
	if replay.Code != 200 {
		t.Fatalf("same-key replay status=%d body=%s", replay.Code, replay.Body.String())
	}
	changed := agentPolicyBody(true, 4, "0.1")
	if conflict := f.call("PUT", "/api/v1/admin/agent-policy", "agent-policy-replay-001", f.token, managedBrand, changed); conflict.Code != 409 {
		t.Fatalf("same key changed body status=%d body=%s", conflict.Code, conflict.Body.String())
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='agent_policy.write.brand'`, role); err != nil {
		t.Fatal(err)
	}
	if revoked := f.call("PUT", "/api/v1/admin/agent-policy", "agent-policy-replay-001", f.token, managedBrand, body); revoked.Code != 403 {
		t.Fatalf("cached response after grant revocation status=%d body=%s", revoked.Code, revoked.Body.String())
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,'agent_policy.write.brand')`, role); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `CREATE FUNCTION fail_agent_policy_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='agent.policy.update' THEN RAISE EXCEPTION 'test audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_agent_policy_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION fail_agent_policy_audit()`); err != nil {
		t.Fatal(err)
	}
	failing := agentPolicyBody(true, 4, "0.1")
	failing["version"] = 2
	if got := f.call("PUT", "/api/v1/admin/agent-policy", "agent-policy-audit-fail-001", f.token, managedBrand, failing); got.Code != 200 {
		t.Fatalf("current policy update depended on removed history: status=%d body=%s", got.Code, got.Body.String())
	}
	policy := f.call("GET", "/api/v1/admin/agent-policy", "", f.token, managedBrand, nil)
	if policy.Code != 200 {
		t.Fatal(policy.Code, policy.Body.String())
	}
	var current agentPolicy
	managedData(t, policy, &current)
	if current.Version != 3 || current.Config.MaxDepth != 4 {
		t.Fatalf("current policy was not replaced: %+v", current)
	}
	var audits int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE brand_id=$1 AND action='agent.policy.update'`, managedBrand).Scan(&audits); err != nil || audits != 0 {
		t.Fatalf("policy change audit was written: %d %v", audits, err)
	}
}

func TestAgentSuperAdminNeedsPlatformViewGrant(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	var brandRole string
	if err := f.pool.QueryRow(ctx, `SELECT role_id::text FROM admin_account_roles WHERE account_id=$1 LIMIT 1`, f.root).Scan(&brandRole); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"agent_policy.view.brand", "agent_policy.view.platform"} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, key); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='agent_policy.view.brand'`, brandRole); err != nil {
		t.Fatal(err)
	}
	platformRole := ids.New()
	if _, err := f.pool.Exec(ctx, `INSERT INTO roles(id,brand_id,code,name) VALUES($1,NULL,'agent_platform_reader','Agent platform reader')`, platformRole); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,'agent_policy.view.platform')`, platformRole); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, f.root, platformRole); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	platformToken := platformAdminToken(t, f)
	f.token = platformToken
	if got := f.call("GET", "/api/v1/platform/agent-policy", "", platformToken, managedBrand, nil); got.Code != 200 {
		t.Fatalf("platform-granted super read status=%d body=%s", got.Code, got.Body.String())
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='agent_policy.view.platform'`, platformRole); err != nil {
		t.Fatal(err)
	}
	if got := f.call("GET", "/api/v1/platform/agent-policy", "", platformToken, managedBrand, nil); got.Code != 403 {
		t.Fatalf("super read without platform grant status=%d body=%s", got.Code, got.Body.String())
	}
}

func TestAgentListAllLevelsPaginationAndBrandPermission(t *testing.T) {
	f := pointsFixture(t)
	agentGrantRoot(t, f.managementHTTP)
	if resp := f.call("PUT", "/api/v1/admin/agent-policy", "agent-list-policy", f.token, managedBrand, agentPolicyBody(true, 3, "0.1")); resp.Code != 200 {
		t.Fatal(resp.Code, resp.Body.String())
	}
	var root, child agentNode
	managedData(t, f.call("POST", "/api/v1/admin/agents", "agent-list-root", f.token, managedBrand, agentNodeBody(2, f.memberID, nil, nil, "0.08")), &root)
	childMember := newNormalAgentMember(t, f.managementHTTP, "listchild")
	managedData(t, f.call("POST", "/api/v1/admin/agents", "agent-list-child", f.token, managedBrand, agentNodeBody(2, childMember, &root.ID, &root.Version, "0.04")), &child)
	var page agency.AgentList
	managedData(t, f.call("GET", "/api/v1/admin/agents?limit=1&offset=1", "", f.token, managedBrand, nil), &page)
	if page.BrandID != managedBrand || page.TotalCount != "2" || len(page.Items) != 1 || page.Items[0].ID != child.ID || page.Items[0].Depth != 2 {
		t.Fatalf("bad agent list page: %+v", page)
	}

	// Search all levels before pagination, including a child outside the first page.
	managedData(t, f.call("GET", "/api/v1/admin/agents?q="+strings.ToUpper(testdb.AgentUsername(childMember))+"&limit=1&offset=0", "", f.token, managedBrand, nil), &page)
	if page.TotalCount != "1" || len(page.Items) != 1 || page.Items[0].ID != child.ID || page.Items[0].Username != testdb.AgentUsername(childMember) {
		t.Fatalf("bad search: %+v", page)
	}
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE session_activity SET last_seen_at=now()-interval '6 minutes'`); err != nil {
		t.Fatal(err)
	}
	login := f.call("POST", "/api/v1/agent/login", "", "", managedBrand, identity.AgentLoginInput{Username: testdb.AgentUsername(childMember), Password: "agent123"})
	mustStatus(t, login, 200)
	managedData(t, f.call("GET", "/api/v1/admin/agents?online=true", "", f.token, managedBrand, nil), &page)
	if page.TotalCount != "1" || page.Items[0].ID != child.ID || !page.Items[0].Online {
		t.Fatalf("bad presence: %+v", page)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE agent_sessions SET revoked_at=now() WHERE agent_account_id=$1`, child.MemberID); err != nil {
		t.Fatal(err)
	}
	managedData(t, f.call("GET", "/api/v1/admin/agents?online=true", "", f.token, managedBrand, nil), &page)
	if page.TotalCount != "0" {
		t.Fatalf("revoked session online: %+v", page)
	}
	managedData(t, f.call("GET", "/api/v1/admin/agents?online=false", "", f.token, managedBrand, nil), &page)
	if page.TotalCount != "2" {
		t.Fatalf("offline count: %+v", page)
	}
	for _, suffix := range []string{"online=1", "online=true&online=false", "unknown=x"} {
		if got := f.call("GET", "/api/v1/admin/agents?"+suffix, "", f.token, managedBrand, nil); got.Code != 400 {
			t.Fatal(got.Code, got.Body.String())
		}
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE permission_key IN ('wallet.view.brand','wallet.view.platform')`); err != nil {
		t.Fatal(err)
	}
	managedData(t, f.call("GET", "/api/v1/admin/agents", "", f.token, managedBrand, nil), &page)
	for _, item := range page.Items {
		if item.DisplayPoints != nil {
			t.Fatal("wallet permission bypass")
		}
	}
	if resp := f.call("GET", "/api/v1/admin/agents", "", f.token, pointsBrandB, nil); resp.Code != 403 {
		t.Fatalf("foreign brand list: %d %s", resp.Code, resp.Body.String())
	}
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE permission_key='agent.view.brand'`); err != nil {
		t.Fatal(err)
	}
	if resp := f.call("GET", "/api/v1/admin/agents", "", f.token, managedBrand, nil); resp.Code != 403 {
		t.Fatalf("list without agent grant: %d %s", resp.Code, resp.Body.String())
	}
}

func TestAgentWholeLineModeChangeIdempotencyAndBrandIsolation(t *testing.T) {
	f := pointsFixture(t)
	agentGrantRoot(t, f.managementHTTP)
	ctx := context.Background()
	mustStatus(t, f.call("PUT", "/api/v1/admin/agent-policy", "mode-policy-enable", f.token, managedBrand, agentPolicyBody(true, 3, "0.1")), 200)
	rootResp := f.call("POST", "/api/v1/admin/agents", "mode-root-create", f.token, managedBrand, agentNodeBody(2, f.memberID, nil, nil, "0.08"))
	mustStatus(t, rootResp, 201)
	var root agentNode
	managedData(t, rootResp, &root)
	childMember := newNormalAgentMember(t, f.managementHTTP, "mode_child_"+ids.New())
	childResp := f.call("POST", "/api/v1/admin/agents", "mode-child-create", f.token, managedBrand, agentNodeBody(2, childMember, &root.ID, &root.Version, "0.04"))
	mustStatus(t, childResp, 201)
	var child agentNode
	managedData(t, childResp, &child)
	route := "/api/v1/admin/agents/" + root.ID + "/mode-change"
	scope := f.call("GET", route, "", f.token, managedBrand, nil)
	mustStatus(t, scope, 200)
	var line agency.ModeChangeScope
	managedData(t, scope, &line)
	if len(line.Items) != 2 {
		t.Fatal(line)
	}
	in := agency.ModeChangeInput{PolicyVersion: 2, Mode: "turnover", Items: []agency.ModeRatioInput{{AgentID: root.ID, Version: root.Version, Ratio: "0.03"}, {AgentID: child.ID, Version: child.Version, Ratio: "0.02"}}}
	missingCalendar := f.call("POST", route, "mode-missing-calendar", f.token, managedBrand, in)
	mustStatus(t, missingCalendar, 409)
	if _, err := f.pool.Exec(ctx, `UPDATE brand_commission_policies SET version=version+1,config='{"enabled":false,"calendar":{"timezone":"Asia/Singapore","cycle":"weekly","boundary_time":"00:00:00","weekday":1,"month_day":null,"short_month":""},"payout_mode":"manual"}' WHERE brand_id=$1`, managedBrand); err != nil {
		t.Fatal(err)
	}
	first := f.call("POST", route, "mode-schedule-once", f.token, managedBrand, in)
	mustStatus(t, first, 200)
	retry := f.call("POST", route, "mode-schedule-once", f.token, managedBrand, in)
	mustStatus(t, retry, 200)
	var one, two agency.ModeChangeReceipt
	managedData(t, first, &one)
	managedData(t, retry, &two)
	if one.ID != two.ID || !one.EffectiveAt.Equal(two.EffectiveAt) {
		t.Fatal(one, two)
	}
	different := in
	different.Items = append([]agency.ModeRatioInput{}, in.Items...)
	different.Items[0].Ratio = "0.04"
	mustStatus(t, f.call("POST", route, "mode-schedule-once", f.token, managedBrand, different), 409)
	mustStatus(t, f.call("GET", route, "", f.token, pointsBrandB, nil), 403)
	mustStatus(t, f.call("POST", route, "mode-foreign-brand", f.token, pointsBrandB, in), 403)
	var count int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM agent_mode_changes WHERE brand_id=$1`, managedBrand).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	nodeResp := f.call("GET", "/api/v1/admin/agents/"+child.ID, "", f.token, managedBrand, nil)
	mustStatus(t, nodeResp, 200)
	var current agentNode
	managedData(t, nodeResp, &current)
	if current.EffectiveMode != "loss" || current.Config.Ratio != "0.04" || current.PendingChange == nil || current.PendingChange.Ratio != "0.02" {
		t.Fatal(current)
	}
}
