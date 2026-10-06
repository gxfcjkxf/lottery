package betting

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/attribution"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"strings"
	"testing"
)

func TestBetAttributionUsesImmutableJoinAndSubmissionConfig(t *testing.T) {
	f := newBettingFixture(t, storeTestBrand)
	ctx := context.Background()
	admin := f.version.CreatedBy
	a := access.Account{ID: admin, Type: access.AccountAdmin, BrandIDs: []string{f.brand}, Roles: []access.Role{{BrandID: f.brand, Permissions: []access.Permission{{Resource: "agent_policy", Action: "write", Scope: access.ScopeBrand}, {Resource: "agent", Action: "write", Scope: access.ScopeBrand}, {Resource: "join_code", Action: "write", Scope: access.ScopeBrand}}}}}
	agents := agency.Service{DB: f.db}
	codes := attribution.Service{DB: f.db}
	var policy agency.Policy
	var node agency.Node
	var code attribution.Code
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		policy, e = agents.SavePolicy(ctx, tx, f.brand, a, agency.PolicyInput{Version: 1, Config: agency.PolicyConfig{Enabled: true, MaxDepth: 3, RatioCap: "0.1", Mode: "loss", Cycle: "weekly"}, Reason: "attribution fixture policy"}, points.Metadata{RequestID: ids.New()})
		return e
	})
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		node, e = agents.Create(ctx, tx, f.brand, a, agency.CreateInput{PolicyVersion: policy.Version, MemberID: f.member, Config: agency.NodeConfig{Ratio: "0.08", Status: "active", CanCreateChildren: true}, Reason: "attribution fixture root"}, points.Metadata{RequestID: ids.New()})
		return e
	})
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		code, e = codes.Create(ctx, tx, f.brand, a, attribution.CreateInput{Kind: "agent", OwnerMemberID: f.member, AgentID: &node.ID, Reason: "attribution fixture join code"}, points.Metadata{RequestID: ids.New()})
		return e
	})
	users, e := identity.New(f.db)
	if e != nil {
		t.Fatal(e)
	}
	var session identity.Authentication
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		out, e := users.Register(ctx, tx, f.brand, identity.RegisterInput{Username: "bet_attrib_" + strings.ReplaceAll(ids.New(), "-", "")[:12], Password: "test-attribution-password", Privacy: "dev-1", Terms: "dev-1", AgentCode: code.Code}, identity.Metadata{Domain: "aurora.localhost"})
		if e == nil {
			if out.Status != 201 {
				t.Fatalf("register=%+v", out)
			}
			e = json.Unmarshal(out.Data, &session)
		}
		return e
	})
	f.user, e = users.Authenticate(ctx, f.brand, session.AccessToken)
	if e != nil {
		t.Fatal(e)
	}
	f.member, f.token = session.Member.ID, session.AccessToken
	fundBettingWallet(t, f, 20)
	first, e := placeBettingOrder(t, f, f.input, "attribution-bet-first")
	if e != nil {
		t.Fatal(e)
	}
	read := func(id string) []byte {
		var raw []byte
		if e := f.db.QueryRow(ctx, `SELECT attribution_snapshot FROM bet_orders WHERE id=$1`, id).Scan(&raw); e != nil {
			t.Fatal(e)
		}
		return raw
	}
	original := read(first.ID)
	var old struct {
		Member  map[string]any `json:"member_attribution"`
		Configs []struct {
			ID      string         `json:"id"`
			Version int64          `json:"version"`
			Config  map[string]any `json:"config"`
		} `json:"agent_configs_at_bet"`
		Commission *json.RawMessage `json:"commission_policy"`
	}
	if e = json.Unmarshal(original, &old); e != nil || len(old.Configs) != 1 || old.Configs[0].ID != node.ID || old.Configs[0].Version != 1 || old.Configs[0].Config["ratio"] != "0.08" || old.Member["code_id"] != code.ID || old.Commission != nil {
		t.Fatalf("old=%+v error=%v", old, e)
	}
	cfg := node.Config
	cfg.Ratio = "0.06"
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var e error
		node, e = agents.Update(ctx, tx, f.brand, a, node.ID, agency.UpdateInput{Version: node.Version, PolicyVersion: policy.Version, Config: cfg, Reason: "new stakes use new ratio"}, points.Metadata{RequestID: ids.New()})
		return e
	})
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, e := codes.Update(ctx, tx, f.brand, a, code.ID, attribution.UpdateInput{Version: code.Version, Status: "disabled", Reason: "existing member remains attributed"}, points.Metadata{RequestID: ids.New()})
		return e
	})
	second, e := placeBettingOrder(t, f, f.input, "attribution-bet-second")
	if e != nil {
		t.Fatal(e)
	}
	var next struct {
		Configs []struct {
			Version int64          `json:"version"`
			Config  map[string]any `json:"config"`
		} `json:"agent_configs_at_bet"`
		Member map[string]any `json:"member_attribution"`
	}
	if e = json.Unmarshal(read(second.ID), &next); e != nil || len(next.Configs) != 1 || next.Configs[0].Version != 2 || next.Configs[0].Config["ratio"] != "0.06" || next.Member["code_version"] != float64(1) {
		t.Fatal(next, e)
	}
	if string(read(first.ID)) != string(original) {
		t.Fatal("old stake config rewritten")
	}
	if _, e = f.db.Exec(ctx, `UPDATE bet_orders SET attribution_snapshot='{}'::jsonb,version=version+1 WHERE id=$1`, first.ID); e == nil {
		t.Fatal("attribution rewrite accepted")
	}
	public, _ := json.Marshal(second)
	if strings.Contains(string(public), "agent_configs_at_bet") || strings.Contains(string(public), "member_attribution") {
		t.Fatal("private chain exposed in user order")
	}
	var commissions int
	if e = f.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type NOT IN('adjustment','bet')`).Scan(&commissions); e != nil || commissions != 0 {
		t.Fatal("configuration paid funds", commissions, e)
	}
}
