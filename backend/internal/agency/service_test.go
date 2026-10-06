package agency

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
	"sync"
	"testing"
)

const brandA = "0199a000-0000-7000-8000-000000000001"
const brandB = "0199a000-0000-7000-8000-000000000002"

func fixture(t *testing.T) (Service, access.Account) {
	t.Helper()
	db := testdb.New(t)
	a := access.Account{ID: ids.New(), Type: access.AccountAdmin, BrandIDs: []string{brandA}, Roles: []access.Role{{BrandID: brandA, Permissions: []access.Permission{{Resource: "agent", Action: "write", Scope: access.ScopeBrand}, {Resource: "agent_policy", Action: "write", Scope: access.ScopeBrand}}}}}
	if _, e := db.Exec(context.Background(), `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-unused-agent-hash')`, a.ID, "agent_"+a.ID); e != nil {
		t.Fatal(e)
	}
	return Service{DB: db}, a
}
func member(t *testing.T, s Service, brand string) string {
	t.Helper()
	u, m := ids.New(), ids.New()
	if _, e := s.DB.Exec(context.Background(), `INSERT INTO global_users(id,username) VALUES($1,$2)`, u, "agency_"+u); e != nil {
		t.Fatal(e)
	}
	if _, e := s.DB.Exec(context.Background(), `INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'domain','dev-1','dev-1')`, m, brand, u); e != nil {
		t.Fatal(e)
	}
	return m
}
func transaction(t *testing.T, s Service, f func(pgx.Tx) error) error {
	t.Helper()
	tx, e := s.DB.Begin(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(context.Background())
	if e = f(tx); e != nil {
		return e
	}
	return tx.Commit(context.Background())
}
func metadata(a access.Account) points.Metadata {
	return points.Metadata{ActorType: "admin", ActorID: a.ID, RequestID: ids.New()}
}
func enable(t *testing.T, s Service, a access.Account) Policy {
	t.Helper()
	var p Policy
	e := transaction(t, s, func(tx pgx.Tx) error {
		var e error
		p, e = s.SavePolicy(context.Background(), tx, brandA, a, PolicyInput{Version: 1, Config: PolicyConfig{Enabled: true, MaxDepth: 3, RatioCap: "0.1", Mode: "loss", Cycle: "weekly"}, Reason: "enable isolated agent fixture"}, metadata(a))
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func create(t *testing.T, s Service, a access.Account, p Policy, parent *Node, r string) Node {
	t.Helper()
	var n Node
	in := CreateInput{PolicyVersion: p.Version, MemberID: member(t, s, brandA), Config: NodeConfig{Ratio: r, Status: "active", CanCreateChildren: true}, Reason: "create scoped agent fixture"}
	if parent != nil {
		in.ParentID = &parent.ID
		in.ParentVersion = &parent.Version
	}
	e := transaction(t, s, func(tx pgx.Tx) error {
		var e error
		n, e = s.Create(context.Background(), tx, brandA, a, in, metadata(a))
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	return n
}
func TestAgentHierarchyBoundsHistoryAndDatabaseGuards(t *testing.T) {
	s, a := fixture(t)
	ctx := context.Background()
	p, e := s.Policy(ctx, brandA)
	if e != nil || p.Config.Enabled || p.Config.RatioCap != "0" || p.Version != 1 {
		t.Fatal(p, e)
	}
	p = enable(t, s, a)
	root := create(t, s, a, p, nil, "0.08")
	child := create(t, s, a, p, &root, "0.04")
	leaf := create(t, s, a, p, &child, "0.03")
	tree, e := s.Tree(ctx, brandA, &root.ID, 20, 0)
	if e != nil || tree.TotalCount != "1" || len(tree.Items) != 1 || tree.Items[0].ID != child.ID || len(leaf.Path) != 3 {
		t.Fatal(tree, leaf, e)
	}
	for _, tc := range []struct {
		node Node
		cfg  NodeConfig
		err  error
	}{{root, NodeConfig{Ratio: "0.02", Status: "active", CanCreateChildren: true}, ErrLimit}, {child, NodeConfig{Ratio: "0.09", Status: "active", CanCreateChildren: true}, ErrLimit}} {
		e := transaction(t, s, func(tx pgx.Tx) error {
			_, e := s.Update(ctx, tx, brandA, a, tc.node.ID, UpdateInput{Version: tc.node.Version, PolicyVersion: p.Version, ParentVersion: tc.node.ParentVersion, Config: tc.cfg, Reason: "reject invalid hierarchy ratio"}, metadata(a))
			return e
		})
		if !errors.Is(e, tc.err) {
			t.Fatal(e)
		}
	}
	for _, cfg := range []PolicyConfig{{Enabled: true, MaxDepth: 2, RatioCap: "0.1", Mode: "loss", Cycle: "weekly"}, {Enabled: true, MaxDepth: 3, RatioCap: "0.01", Mode: "loss", Cycle: "weekly"}} {
		e := transaction(t, s, func(tx pgx.Tx) error {
			_, e := s.SavePolicy(ctx, tx, brandA, a, PolicyInput{Version: p.Version, Config: cfg, Reason: "reject invalid brand shrink"}, metadata(a))
			return e
		})
		if !errors.Is(e, ErrLimit) {
			t.Fatal(e)
		}
	}
	h, e := s.History(ctx, brandA, &child.ID, 20, 0)
	if e != nil || len(h.Items) != 1 || h.Items[0].AuditLogID == nil {
		t.Fatal(h, e)
	}
	if _, e = s.DB.Exec(ctx, `UPDATE agent_config_revisions SET reason='overwrite' WHERE agent_id=$1`, child.ID); e == nil {
		t.Fatal("history rewrite allowed")
	}
	if _, e = s.DB.Exec(ctx, `UPDATE agent_nodes SET parent_id=NULL,depth=1,path=ARRAY[id],version=version+1 WHERE id=$1`, child.ID); e == nil {
		t.Fatal("reparent bypass allowed")
	}
	if _, e = s.DB.Exec(ctx, `UPDATE agent_nodes SET version=version+1,config=jsonb_set(config,'{ratio}','"0.031"') WHERE id=$1`, leaf.ID); e == nil {
		t.Fatal("unaudited update allowed")
	}
	if _, e = s.DB.Exec(ctx, `UPDATE agent_nodes SET version=version+1,config=jsonb_set(config,'{ratio}','"0.09"') WHERE id=$1`, child.ID); e == nil {
		t.Fatal("SQL parent ceiling bypass")
	}
	var posted int
	if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries`).Scan(&posted); e != nil || posted != 0 {
		t.Fatal("agent config posted money", posted, e)
	}
}
func TestAgentConcurrentParentShrinkAndChildIncreaseCannotViolateCap(t *testing.T) {
	s, a := fixture(t)
	p := enable(t, s, a)
	root := create(t, s, a, p, nil, "0.08")
	child := create(t, s, a, p, &root, "0.04")
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, n := range []Node{root, child} {
		wg.Add(1)
		go func(n Node) {
			defer wg.Done()
			cfg := n.Config
			if n.ID == root.ID {
				cfg.Ratio = "0.05"
			} else {
				cfg.Ratio = "0.07"
			}
			results <- transaction(t, s, func(tx pgx.Tx) error {
				_, e := s.Update(context.Background(), tx, brandA, a, n.ID, UpdateInput{Version: n.Version, PolicyVersion: p.Version, ParentVersion: n.ParentVersion, Config: cfg, Reason: "concurrent cap invariant"}, metadata(a))
				return e
			})
		}(n)
	}
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		} else if !errors.Is(e, ErrVersion) && !errors.Is(e, ErrLimit) {
			t.Fatal(e)
		}
	}
	if success != 1 {
		t.Fatal("expected one compatible update", success)
	}
	current, e := s.Node(context.Background(), brandA, root.ID)
	if e != nil {
		t.Fatal(e)
	}
	currentChild, e := s.Node(context.Background(), brandA, child.ID)
	if e != nil {
		t.Fatal(e)
	}
	pr, _ := RatioMicros(current.Config.Ratio)
	cr, _ := RatioMicros(currentChild.Config.Ratio)
	if cr > pr {
		t.Fatal(current, currentChild)
	}
}
func TestAgentRatioAndClosedInput(t *testing.T) {
	for _, v := range []string{"0", "1", "0.25", "0.000001", "0.999999"} {
		if _, e := RatioMicros(v); e != nil {
			t.Fatal(v, e)
		}
	}
	for _, v := range []string{"1.01", "0.0", "0.250", "+0.1", "0.0000001", "00"} {
		if _, e := RatioMicros(v); e == nil {
			t.Fatal(v)
		}
	}
	for _, raw := range []string{`{"version":1,"config":{"enabled":null,"max_depth":5,"ratio_cap":"0","mode":"loss","cycle":"weekly"},"reason":"x"}`, `{"version":1,"version":2,"config":{},"reason":"x"}`, `{"version":1,"config":{"enabled":false,"max_depth":5,"ratio_cap":"0","mode":"loss","cycle":"weekly"},"reason":"x","extra":true}`} {
		var in PolicyInput
		if json.Unmarshal([]byte(raw), &in) == nil {
			t.Fatal(raw)
		}
	}
}
