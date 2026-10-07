package agency

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

func TestAgentConcurrentParentAndChildModeChangesStayUniform(t *testing.T) {
	s, a := fixture(t)
	ctx := context.Background()
	p := enable(t, s, a)
	root := create(t, s, a, p, nil, "0.08")
	loss := "loss"
	rootCfg := root.Config
	rootCfg.Mode = &loss
	root, err := updateNode(t, s, a, p, root, rootCfg)
	if err != nil {
		t.Fatal(err)
	}
	childMember := member(t, s, brandA)
	var child Node
	err = transaction(t, s, func(tx pgx.Tx) error {
		var e error
		child, e = s.Create(ctx, tx, brandA, a, CreateInput{
			PolicyVersion: p.Version, MemberID: childMember, ParentID: &root.ID, ParentVersion: &root.Version,
			Config: NodeConfig{Ratio: "0.04", Mode: &loss, Status: "active", CanCreateChildren: true},
			Reason: "create explicit child for mode race",
		}, metadata(a))
		return e
	})
	if err != nil {
		t.Fatal(err)
	}

	turnover := "turnover"
	parentConfig := root.Config
	parentConfig.Mode = &turnover
	childConfig := child.Config
	childConfig.Mode = nil
	start := make(chan struct{})
	type result struct {
		parent bool
		err    error
	}
	results := make(chan result, 2)
	run := func(isParent bool, id string, version int64, parentVersion *int64, config NodeConfig, reason string) {
		tx, beginErr := s.DB.Begin(ctx)
		if beginErr != nil {
			results <- result{parent: isParent, err: beginErr}
			return
		}
		<-start
		_, updateErr := s.Update(ctx, tx, brandA, a, id, UpdateInput{
			Version: version, PolicyVersion: p.Version, ParentVersion: parentVersion,
			Config: config, Reason: reason,
		}, metadata(a))
		if updateErr != nil {
			_ = tx.Rollback(ctx)
		} else {
			updateErr = tx.Commit(ctx)
		}
		results <- result{parent: isParent, err: updateErr}
	}
	go run(true, root.ID, root.Version, nil, parentConfig, "concurrent parent mode change")
	go run(false, child.ID, child.Version, child.ParentVersion, childConfig, "concurrent child mode inheritance")
	close(start)
	var parentErr, childErr error
	for range 2 {
		r := <-results
		if r.parent {
			parentErr = r.err
		} else {
			childErr = r.err
		}
	}
	if childErr != nil || parentErr != nil && !errors.Is(parentErr, ErrLimit) {
		t.Fatal("unexpected concurrent mode mutation results", parentErr, childErr)
	}
	currentRoot, err := s.Node(ctx, brandA, root.ID)
	if err != nil {
		t.Fatal(err)
	}
	currentChild, err := s.Node(ctx, brandA, child.ID)
	if err != nil || currentChild.EffectiveMode != currentRoot.EffectiveMode || currentChild.Config.Mode != nil {
		t.Fatal("concurrent updates left inconsistent effective modes", currentRoot, currentChild, err)
	}
}

func updateNode(t *testing.T, s Service, a access.Account, p Policy, n Node, cfg NodeConfig) (Node, error) {
	t.Helper()
	var out Node
	err := transaction(t, s, func(tx pgx.Tx) error {
		var e error
		out, e = s.Update(context.Background(), tx, brandA, a, n.ID, UpdateInput{
			Version: n.Version, PolicyVersion: p.Version, ParentVersion: n.ParentVersion,
			Config: cfg, Reason: "prepare mode concurrency fixture",
		}, metadata(a))
		return e
	})
	return out, err
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

func TestAgentEffectiveModeInheritanceAndGuards(t *testing.T) {
	s, a := fixture(t)
	ctx := context.Background()
	p := enable(t, s, a)
	turnover := "turnover"

	root := create(t, s, a, p, nil, "0.08")
	// Roots may override the brand default. Children may inherit or explicitly
	// repeat the parent's effective mode.
	rootCfg := root.Config
	rootCfg.Mode = &turnover
	var updatedRoot Node
	err := transaction(t, s, func(tx pgx.Tx) error {
		var e error
		updatedRoot, e = s.Update(ctx, tx, brandA, a, root.ID, UpdateInput{
			Version: root.Version, PolicyVersion: p.Version, Config: rootCfg,
			Reason: "set root mode override",
		}, metadata(a))
		return e
	})
	if err != nil || updatedRoot.EffectiveMode != turnover {
		t.Fatal(updatedRoot, err)
	}
	child := create(t, s, a, p, &updatedRoot, "0.04")
	if child.EffectiveMode != turnover || child.Config.Mode != nil {
		t.Fatal("inherited child mode", child)
	}
	equalChild := member(t, s, brandA)
	var explicitChild Node
	err = transaction(t, s, func(tx pgx.Tx) error {
		var e error
		explicitChild, e = s.Create(ctx, tx, brandA, a, CreateInput{
			PolicyVersion: p.Version, MemberID: equalChild, ParentID: &updatedRoot.ID,
			ParentVersion: &updatedRoot.Version,
			Config:        NodeConfig{Ratio: "0.03", Mode: &turnover, Status: "active", CanCreateChildren: true},
			Reason:        "create explicit equal mode child",
		}, metadata(a))
		return e
	})
	if err != nil || explicitChild.EffectiveMode != turnover {
		t.Fatal(explicitChild, err)
	}

	var revisionsBefore, auditsBefore int
	if err = s.DB.QueryRow(ctx, `SELECT count(*) FROM agent_config_revisions WHERE brand_id=$1`, brandA).Scan(&revisionsBefore); err != nil {
		t.Fatal(err)
	}
	if err = s.DB.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE brand_id=$1 AND resource_type IN ('agent_node','agent_policy')`, brandA).Scan(&auditsBefore); err != nil {
		t.Fatal(err)
	}
	wrong := "loss"
	badMember := member(t, s, brandA)
	err = transaction(t, s, func(tx pgx.Tx) error {
		_, e := s.Create(ctx, tx, brandA, a, CreateInput{
			PolicyVersion: p.Version, MemberID: badMember, ParentID: &updatedRoot.ID,
			ParentVersion: &updatedRoot.Version,
			Config:        NodeConfig{Ratio: "0.02", Mode: &wrong, Status: "active", CanCreateChildren: true},
			Reason:        "reject differing child mode",
		}, metadata(a))
		return e
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("differing child mode error=%v, want ErrInvalid", err)
	}
	childCfg := child.Config
	childCfg.Mode = &wrong
	err = transaction(t, s, func(tx pgx.Tx) error {
		_, e := s.Update(ctx, tx, brandA, a, child.ID, UpdateInput{
			Version: child.Version, PolicyVersion: p.Version, ParentVersion: child.ParentVersion,
			Config: childCfg, Reason: "reject differing child update",
		}, metadata(a))
		return e
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("differing child update error=%v, want ErrInvalid", err)
	}
	user := identity.Session{View: identity.View{
		User:   identity.User{ID: ids.New(), Status: "active"},
		Member: identity.Member{ID: updatedRoot.MemberID, BrandID: brandA, Status: "normal"},
	}}
	err = transaction(t, s, func(tx pgx.Tx) error {
		_, e := s.UpdateChild(ctx, tx, brandA, user, child.ID, ChildInput{
			Version: child.Version, PolicyVersion: p.Version, ParentVersion: updatedRoot.Version,
			Ratio: child.Config.Ratio, Mode: &wrong, Reason: "reject user child mode mismatch",
		}, points.Metadata{RequestID: ids.New()})
		return e
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("user child differing mode error=%v, want ErrInvalid", err)
	}
	rootCfg.Mode = nil
	err = transaction(t, s, func(tx pgx.Tx) error {
		_, e := s.Update(ctx, tx, brandA, a, updatedRoot.ID, UpdateInput{
			Version: updatedRoot.Version, PolicyVersion: p.Version, Config: rootCfg,
			Reason: "reject parent change that splits descendants",
		}, metadata(a))
		return e
	})
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("parent mode change error=%v, want ErrLimit", err)
	}
	var revisionsAfter, auditsAfter int
	if err = s.DB.QueryRow(ctx, `SELECT count(*) FROM agent_config_revisions WHERE brand_id=$1`, brandA).Scan(&revisionsAfter); err != nil || revisionsAfter != revisionsBefore {
		t.Fatal("rejected mode mutations wrote revisions", revisionsBefore, revisionsAfter, err)
	}
	if err = s.DB.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE brand_id=$1 AND resource_type IN ('agent_node','agent_policy')`, brandA).Scan(&auditsAfter); err != nil || auditsAfter != auditsBefore {
		t.Fatal("rejected mode mutations wrote audit records", auditsBefore, auditsAfter, err)
	}
	badID := ids.New()
	_, err = s.DB.Exec(ctx, `INSERT INTO agent_nodes(id,brand_id,member_id,parent_id,depth,path,config,created_by)
	 VALUES($1,$2,$3,$4,2,$5::uuid[],$6,$7)`, badID, brandA, badMember, updatedRoot.ID,
		append(append([]string{}, updatedRoot.Path...), badID),
		[]byte(`{"ratio":"0.02","mode":"loss","status":"active","can_create_children":true}`), a.ID)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != "agent_child_mode_matches_parent" {
		t.Fatalf("direct SQL child mode guard error=%v", err)
	}
	_, err = s.DB.Exec(ctx, `UPDATE agent_nodes SET version=version+1,config=jsonb_set(config,'{mode}','"loss"') WHERE id=$1`, child.ID)
	pgErr = nil
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != "agent_child_mode_matches_parent" {
		t.Fatalf("direct SQL child update mode guard error=%v", err)
	}
	_, err = s.DB.Exec(ctx, `UPDATE agent_nodes SET version=version+1,config=jsonb_set(config,'{mode}','null'::jsonb) WHERE id=$1`, updatedRoot.ID)
	pgErr = nil
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != "agent_descendant_mode_matches_parent" {
		t.Fatalf("direct SQL parent descendant mode guard error=%v", err)
	}
	leaf := create(t, s, a, p, &child, "0.02")
	legacyErr := transaction(t, s, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `ALTER TABLE agent_nodes DISABLE TRIGGER guarded_agent_node`); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `ALTER TABLE agent_nodes DISABLE TRIGGER guarded_agent_uniform_node_mode`); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `ALTER TABLE agent_nodes DISABLE TRIGGER agent_node_revision_required`); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `UPDATE agent_nodes SET config=jsonb_set(config,'{mode}','"loss"') WHERE id=$1`, child.ID); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `ALTER TABLE agent_nodes ENABLE TRIGGER guarded_agent_node`); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `ALTER TABLE agent_nodes ENABLE TRIGGER guarded_agent_uniform_node_mode`); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `ALTER TABLE agent_nodes ENABLE TRIGGER agent_node_revision_required`); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `SAVEPOINT legacy_insert_guard`); e != nil {
			return e
		}
		grandchildID, grandchildMember := ids.New(), member(t, s, brandA)
		_, insertErr := tx.Exec(ctx, `INSERT INTO agent_nodes(id,brand_id,member_id,parent_id,depth,path,config,created_by)
		 SELECT $1,$2,$3,parent.id,parent.depth+1,array_append(parent.path,$1::uuid),
		        '{"ratio":"0.01","mode":null,"status":"active","can_create_children":true}'::jsonb,$4
		 FROM agent_nodes parent WHERE parent.brand_id=$2 AND parent.id=$5`,
			grandchildID, brandA, grandchildMember, a.ID, child.ID)
		var insertPGErr *pgconn.PgError
		if !errors.As(insertErr, &insertPGErr) || insertPGErr.ConstraintName != "agent_ancestor_mode_chain_consistent" {
			return errors.New("direct SQL insert did not reject legacy mixed ancestor chain")
		}
		if _, e := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT legacy_insert_guard`); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `SAVEPOINT legacy_policy_guard`); e != nil {
			return e
		}
		_, policyErr := tx.Exec(ctx, `UPDATE brand_agent_policies SET version=version+1,config=jsonb_set(config,'{mode}','"turnover"') WHERE brand_id=$1`, brandA)
		var policyPGErr *pgconn.PgError
		if !errors.As(policyErr, &policyPGErr) || policyPGErr.ConstraintName != "agent_brand_mode_preserves_hierarchy" {
			return errors.New("direct SQL brand mode guard did not reject legacy mixed path")
		}
		if _, e := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT legacy_policy_guard`); e != nil {
			return e
		}
		cfg := leaf.Config
		cfg.Ratio = "0.019"
		_, e := s.Update(ctx, tx, brandA, a, leaf.ID, UpdateInput{
			Version: leaf.Version, PolicyVersion: p.Version, ParentVersion: leaf.ParentVersion,
			Config: cfg, Reason: "fail closed on legacy mixed ancestor chain",
		}, metadata(a))
		return e
	})
	if !errors.Is(legacyErr, ErrLimit) {
		t.Fatalf("legacy mixed path mutation error=%v, want ErrLimit", legacyErr)
	}
}

func TestAgentBrandModeChangesPreserveExistingChains(t *testing.T) {
	s, a := fixture(t)
	ctx := context.Background()
	p := enable(t, s, a)
	root := create(t, s, a, p, nil, "0.08")
	child := create(t, s, a, p, &root, "0.04")
	explicitRoot := create(t, s, a, p, nil, "0.08")
	loss := "loss"
	explicitMember := member(t, s, brandA)
	var explicitChild Node
	err := transaction(t, s, func(tx pgx.Tx) error {
		var e error
		explicitChild, e = s.Create(ctx, tx, brandA, a, CreateInput{
			PolicyVersion: p.Version, MemberID: explicitMember, ParentID: &explicitRoot.ID,
			ParentVersion: &explicitRoot.Version,
			Config:        NodeConfig{Ratio: "0.03", Mode: &loss, Status: "active", CanCreateChildren: true},
			Reason:        "create homogeneous explicit child mode",
		}, metadata(a))
		return e
	})
	if err != nil || explicitChild.EffectiveMode != "loss" {
		t.Fatal(explicitChild, err)
	}
	turnover := PolicyConfig{Enabled: true, MaxDepth: 3, RatioCap: "0.1", Mode: "turnover", Cycle: "weekly"}
	var revisionsBefore int
	if err = s.DB.QueryRow(ctx, `SELECT count(*) FROM agent_config_revisions WHERE brand_id=$1`, brandA).Scan(&revisionsBefore); err != nil {
		t.Fatal(err)
	}
	err = transaction(t, s, func(tx pgx.Tx) error {
		var e error
		_, e = s.SavePolicy(ctx, tx, brandA, a, PolicyInput{Version: p.Version, Config: turnover, Reason: "reject mode change that splits explicit child"}, metadata(a))
		return e
	})
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("opposite brand mode with explicit homogeneous child error=%v, want ErrLimit", err)
	}
	var revisionsAfter int
	if err = s.DB.QueryRow(ctx, `SELECT count(*) FROM agent_config_revisions WHERE brand_id=$1`, brandA).Scan(&revisionsAfter); err != nil || revisionsAfter != revisionsBefore {
		t.Fatal("rejected brand mode change wrote revisions", revisionsBefore, revisionsAfter, err)
	}
	childCfg := explicitChild.Config
	childCfg.Mode = nil
	var inheritedExplicitChild Node
	err = transaction(t, s, func(tx pgx.Tx) error {
		var e error
		inheritedExplicitChild, e = s.Update(ctx, tx, brandA, a, explicitChild.ID, UpdateInput{
			Version: explicitChild.Version, PolicyVersion: p.Version, ParentVersion: explicitChild.ParentVersion,
			Config: childCfg, Reason: "return child to inherited mode",
		}, metadata(a))
		return e
	})
	if err != nil || inheritedExplicitChild.EffectiveMode != "loss" {
		t.Fatal(inheritedExplicitChild, err)
	}
	var next Policy
	err = transaction(t, s, func(tx pgx.Tx) error {
		var e error
		next, e = s.SavePolicy(ctx, tx, brandA, a, PolicyInput{Version: p.Version, Config: turnover, Reason: "change inherited brand mode"}, metadata(a))
		return e
	})
	if err != nil {
		t.Fatal("homogeneous inherited hierarchy should follow the brand default", err)
	}
	n, err := s.Node(ctx, brandA, child.ID)
	if err != nil || n.EffectiveMode != "turnover" {
		t.Fatal(n, err)
	}
	explicitAfter, err := s.Node(ctx, brandA, explicitChild.ID)
	if err != nil || explicitAfter.EffectiveMode != "turnover" {
		t.Fatal(explicitAfter, err)
	}

	// A root override shields its inherited subtree from a later default change.
	rootCfg := root.Config
	rootCfg.Mode = &loss
	var overridden Node
	err = transaction(t, s, func(tx pgx.Tx) error {
		var e error
		overridden, e = s.Update(ctx, tx, brandA, a, root.ID, UpdateInput{Version: root.Version, PolicyVersion: next.Version, Config: rootCfg, Reason: "override root mode"}, metadata(a))
		return e
	})
	if err != nil || overridden.EffectiveMode != "loss" {
		t.Fatal(overridden, err)
	}
	changed := turnover
	changed.Mode = "loss"
	var final Policy
	err = transaction(t, s, func(tx pgx.Tx) error {
		var e error
		final, e = s.SavePolicy(ctx, tx, brandA, a, PolicyInput{Version: next.Version, Config: changed, Reason: "change brand mode under root override"}, metadata(a))
		return e
	})
	if err != nil || final.Config.Mode != "loss" {
		t.Fatal(final, err)
	}
	childAfter, err := s.Node(ctx, brandA, child.ID)
	if err != nil || childAfter.EffectiveMode != "loss" {
		t.Fatal("brand default change affected explicit root subtree", childAfter, err)
	}
}
