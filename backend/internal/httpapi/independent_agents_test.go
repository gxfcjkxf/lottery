package httpapi

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestIndependentAgentsCreateLoginOwnershipAndChildren(t *testing.T) {
	f := managedFixture(t)
	agentGrantRoot(t, f)
	ctx := context.Background()
	existingMember := newNormalAgentMember(t, f, "existing")
	var before int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM brand_members`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	create := agency.CreateInput{PolicyVersion: 1, Username: "agent_test_existing", Password: "agent123", Config: agency.NodeConfig{Ratio: "0", Mode: new("loss"), Status: "active", CanCreateChildren: true}}
	debugTx, _ := f.pool.Begin(ctx)
	_, debugErr := (agency.Service{DB: f.pool}).Create(ctx, debugTx, managedBrand, access.Account{ID: f.root, Type: access.AccountAdmin, BrandIDs: []string{managedBrand}, Roles: []access.Role{{BrandID: managedBrand, Permissions: []access.Permission{{Resource: "agent", Action: "write", Scope: access.ScopeBrand}}}}}, create, points.Metadata{RequestID: ids.New()})
	_ = debugTx.Rollback(ctx)
	if debugErr != nil {
		t.Fatalf("direct provision: %+v", debugErr)
	}
	response := f.call("POST", "/api/v1/admin/agents", "independent-agent-create-001", f.token, managedBrand, create)
	mustStatus(t, response, 201)
	var root agency.Node
	managedData(t, response, &root)
	if root.MemberID == existingMember {
		t.Fatal("agent reused member identity")
	}
	var count int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM brand_members`).Scan(&count); err != nil || count != before {
		t.Fatal("creating agent created member", count, before, err)
	}
	mustStatus(t, f.call("POST", "/api/v1/admin/agents", "independent-agent-create-001", f.token, managedBrand, create), 201)
	login := f.call("POST", "/api/v1/b/aurora/agent/login", "", "", "", identity.AgentLoginInput{Username: "agent_test_existing", Password: "agent123"})
	mustStatus(t, login, 200)
	var token string
	for _, c := range login.Result().Cookies() {
		if c.Name == agentCookieName(managedBrand) {
			token = c.Value
		}
	}
	if token == "" {
		t.Fatal("no independent cookie")
	}
	mustStatus(t, f.call("GET", "/api/v1/b/aurora/agent/me", "", token, "", nil), 200)
	mustStatus(t, f.call("GET", "/api/v1/b/harbor/agent/me", "", token, "", nil), 401)
	mustStatus(t, f.call("GET", "/api/v1/b/aurora/me", "", token, "", nil), 401)
	mustStatus(t, f.call("GET", "/api/v1/b/aurora/agent/members/"+existingMember+"/bets", "", token, "", nil), 404)
	// Agent-created members are distinct member identities, assigned by the server to this agent.
	memberResult := f.call("POST", "/api/v1/b/aurora/agent/members", "agent-new-member-001", token, "", identity.OperatorInput{Username: "agent_new_member", Password: "member123"})
	mustStatus(t, memberResult, 201)
	var created struct {
		MemberID string `json:"member_id"`
	}
	managedData(t, memberResult, &created)
	var source string
	if err := f.pool.QueryRow(ctx, `SELECT attribution_snapshot->>'agent_id' FROM brand_members WHERE id=$1`, created.MemberID).Scan(&source); err != nil || source != root.ID {
		t.Fatal(source, err)
	}
	mustStatus(t, f.call("GET", "/api/v1/b/aurora/agent/members/"+created.MemberID+"/summary", "", token, "", nil), 200)
	mustStatus(t, f.call("GET", "/api/v1/b/aurora/agent/members/"+created.MemberID+"/bets", "", token, "", nil), 200)
	mustStatus(t, f.call("GET", "/api/v1/b/aurora/agent/members", "", token, "", nil), 200)
	child := create
	child.Username = "child_agent"
	child.Config.Mode = nil
	child.Config.CanCreateChildren = false
	staleChild := child
	staleVersion := int64(99)
	staleChild.ParentVersion = &staleVersion
	staleChild.Username = "stale_child"
	mustStatus(t, f.call("POST", "/api/v1/b/aurora/agent/children", "agent-stale-parent-001", token, "", staleChild), 409)
	childResult := f.call("POST", "/api/v1/b/aurora/agent/children", "agent-new-child-001", token, "", child)
	mustStatus(t, childResult, 201)
	var next agency.Node
	managedData(t, childResult, &next)
	if next.ParentID == nil || *next.ParentID != root.ID || next.Depth != 2 {
		t.Fatal(next)
	}
	wrong := ids.New()
	child.ParentID = &wrong
	child.Username = "wrong_parent"
	mustStatus(t, f.call("POST", "/api/v1/b/aurora/agent/children", "agent-wrong-parent-001", token, "", child), 403)
	loginChild := f.call("POST", "/api/v1/b/aurora/agent/login", "", "", "", identity.AgentLoginInput{Username: "child_agent", Password: "agent123"})
	mustStatus(t, loginChild, 200)
	var childToken string
	for _, c := range loginChild.Result().Cookies() {
		if c.Name == agentCookieName(managedBrand) {
			childToken = c.Value
		}
	}
	child.ParentID = nil
	child.Username = "forbidden_child"
	mustStatus(t, f.call("POST", "/api/v1/b/aurora/agent/children", "agent-no-children-001", childToken, "", child), 403)
	mustStatus(t, f.call("GET", "/api/v1/b/aurora/agent/members/"+created.MemberID+"/summary", "", childToken, "", nil), 404)
	// Sessions are independent of member logins; a new agent login replaces only the agent session.
	mustStatus(t, f.call("POST", "/api/v1/b/aurora/agent/login", "", "", "", identity.AgentLoginInput{Username: "agent_test_existing", Password: "agent123"}), 200)
	mustStatus(t, f.call("GET", "/api/v1/b/aurora/agent/me", "", token, "", nil), 401)
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	wallet, err := (points.Store{DB: f.pool}).Snapshot(ctx, tx, managedBrand, root.MemberID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(wallet)
	if len(raw) == 0 {
		t.Fatal("missing agent wallet")
	}
}

// This opt-in fixture owns a temporary schema and its seeded test brands in an
// explicitly named test database. Closing the fixture drops the entire schema.
func TestIndependentAgentBrowserFixture(t *testing.T) {
	if os.Getenv("AGENT_BROWSER_FIXTURE_FILE") == "" {
		t.Skip("opt-in isolated browser fixture")
	}
	_ = os.Remove(os.Getenv("AGENT_BROWSER_FIXTURE_FILE") + ".stop")
	f := managedFixture(t)
	agentGrantRoot(t, f)
	input := agency.CreateInput{PolicyVersion: 1, Username: "portal_reviewer", Password: "agent123", Config: agency.NodeConfig{Ratio: "0", Mode: new("loss"), Status: "active", CanCreateChildren: true}}
	mustStatus(t, f.call("POST", "/api/v1/admin/agents", "browser-fixture-root-001", f.token, managedBrand, input), 201)

	for _, device := range []string{"android", "android_landscape", "ios", "ios_landscape", "ios_320", "desktop"} {
		for _, lang := range []string{"zh", "en"} {
			in := input
			in.Username = "review_" + device + "_" + lang
			rootResult := f.call("POST", "/api/v1/admin/agents", "browser-"+device+"-"+lang+"-001", f.token, managedBrand, in)
			mustStatus(t, rootResult, 201)
			var root agency.Node
			managedData(t, rootResult, &root)
			seed := in
			seed.Username = "seed_" + device + "_" + lang
			seed.ParentID = &root.ID
			seed.ParentVersion = &root.Version
			seed.Config.Mode = nil
			seed.Config.CanCreateChildren = false
			mustStatus(t, f.call("POST", "/api/v1/admin/agents", "seed-"+device+"-"+lang, f.token, managedBrand, seed), 201)
			login := f.call("POST", "/api/v1/b/aurora/agent/login", "", "", "", identity.AgentLoginInput{Username: seed.Username, Password: "agent123"})
			mustStatus(t, login, 200)
			var token string
			for _, c := range login.Result().Cookies() {
				if c.Name == agentCookieName(managedBrand) {
					token = c.Value
				}
			}
			mustStatus(t, f.call("POST", "/api/v1/b/aurora/agent/members", "desc-"+device+"-"+lang, token, "", identity.OperatorInput{Username: "desc_" + device + "_" + lang, Password: "member123"}), 201)
		}
	}
	server := httptest.NewServer(f.http)
	defer server.Close()
	file := os.Getenv("AGENT_BROWSER_FIXTURE_FILE")
	raw, _ := json.Marshal(map[string]string{"url": server.URL, "database": f.pool.Config().ConnConfig.Database, "brand": "aurora"})
	if err := os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(file)
	timer := time.NewTimer(4 * time.Minute)
	defer timer.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-timer.C:
			t.Fatal("browser fixture timed out")
		case <-ticker.C:
			if _, err := os.Stat(file + ".stop"); err == nil {
				os.Remove(file + ".stop")
				return
			}
		}
	}
}
