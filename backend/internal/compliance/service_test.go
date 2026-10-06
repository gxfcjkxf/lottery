package compliance

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
)

const testBrand = "0199a000-0000-7000-8000-000000000001"

func actor(t *testing.T, s Service) access.Account {
	t.Helper()
	id := ids.New()
	if _, e := s.DB.Exec(context.Background(), `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'not-a-login-hash')`, id, "compliance_"+id); e != nil {
		t.Fatal(e)
	}
	return access.Account{ID: id, Type: access.AccountAdmin, BrandIDs: []string{testBrand}, Roles: []access.Role{{BrandID: testBrand, Permissions: []access.Permission{{Resource: "compliance_policy", Action: "view", Scope: access.ScopeBrand}, {Resource: "compliance_policy", Action: "write", Scope: access.ScopeBrand}, {Resource: "compliance_check", Action: "run", Scope: access.ScopeBrand}}}}}
}
func update(t *testing.T, s Service, a access.Account, in Input) (Policy, error) {
	t.Helper()
	ctx := context.Background()
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	v, e := s.Update(ctx, tx, testBrand, a, in, points.Metadata{ActorType: "admin", ActorID: a.ID, RequestID: ids.New()})
	if e == nil {
		e = tx.Commit(ctx)
	}
	return v, e
}
func check(t *testing.T, s Service, a access.Account, in CheckInput) (Decision, error) {
	t.Helper()
	ctx := context.Background()
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	v, e := s.Check(ctx, tx, testBrand, a, in, points.Metadata{ActorType: "admin", ActorID: a.ID, RequestID: ids.New()})
	if e == nil {
		e = tx.Commit(ctx)
	}
	return v, e
}
func financial(t *testing.T, s Service) string {
	t.Helper()
	var v string
	if e := s.DB.QueryRow(context.Background(), `SELECT md5(jsonb_build_object('accounts',(SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM point_accounts a),'buckets',(SELECT jsonb_agg(to_jsonb(b) ORDER BY account_id,source,state) FROM point_buckets b),'ledger',(SELECT jsonb_agg(to_jsonb(l) ORDER BY id) FROM point_ledger_entries l),'orders',(SELECT jsonb_agg(to_jsonb(o) ORDER BY id) FROM bet_orders o),'members',(SELECT jsonb_agg(to_jsonb(m) ORDER BY id) FROM brand_members m))::text)`).Scan(&v); e != nil {
		t.Fatal(e)
	}
	return v
}
func TestDefaultDisabledAndExplicitStubRecordsHaveNoBusinessEffects(t *testing.T) {
	s := Service{DB: testdb.New(t)}
	ctx := context.Background()
	a := actor(t, s)
	before := financial(t, s)
	p, e := s.Read(ctx, testBrand)
	if e != nil || p.Version != 1 || p.Config.AgeEnabled || p.Config.RegionEnabled || p.Config.IdentityEnabled || p.Config.MinimumAge != nil || p.Config.AllowedCountries == nil || p.AuditLogID != "" {
		t.Fatal(p, e)
	}
	d, e := check(t, s, a, CheckInput{Version: 1, Operation: "betting", Reason: "Explicit disabled adapter check"})
	if e != nil || d.Decision != "allow" || d.AdapterMode != "stub" || len(d.Checks) != 3 || d.AuditLogID == "" {
		t.Fatal(d, e)
	}
	age := 21
	cfg := DefaultConfig()
	cfg.AgeEnabled = true
	cfg.MinimumAge = &age
	cfg.RegionEnabled = true
	cfg.AllowedCountries = []string{"PH", "US"}
	cfg.IdentityEnabled = true
	p, e = update(t, s, a, Input{Version: 1, Config: cfg, Reason: "Configure future checks"})
	if e != nil || p.Version != 2 || p.AuditLogID == "" {
		t.Fatal(p, e)
	}
	d, e = check(t, s, a, CheckInput{Version: 2, Operation: "registration", Reason: "Explicit unconfigured adapter check"})
	if e != nil || d.Decision != "review" || d.PolicyVersion != 2 {
		t.Fatal(d, e)
	}
	for _, c := range d.Checks {
		if c.Decision != "review" || c.ReasonCode != "ADAPTER_NOT_CONFIGURED" {
			t.Fatal(c)
		}
	}
	h, e := s.History(ctx, testBrand, 20, 0)
	if e != nil || h.TotalCount != "2" || len(h.Items) != 2 || h.Items[0].Version != 2 || h.Items[1].ChangedBy != nil || h.Items[1].AuditLogID != nil {
		t.Fatal(h, e)
	}
	ds, e := s.Decisions(ctx, testBrand, "registration", 20, 0)
	if e != nil || ds.TotalCount != "1" || len(ds.Items) != 1 || ds.Items[0].ID != d.ID {
		t.Fatal(ds, e)
	}
	if after := financial(t, s); after != before {
		t.Fatal("configuration/check changed financial or member facts")
	}
	for _, q := range []string{`UPDATE compliance_decisions SET decision='allow' WHERE id=$1`, `DELETE FROM compliance_decisions WHERE id=$1`} {
		if _, e = s.DB.Exec(ctx, q, d.ID); e == nil {
			t.Fatal("decision rewritten")
		}
	}
	if _, e = s.DB.Exec(ctx, `UPDATE compliance_policy_revisions SET reason='rewrite' WHERE brand_id=$1`, testBrand); e == nil {
		t.Fatal("revision rewritten")
	}
	if _, e = s.DB.Exec(ctx, `UPDATE brand_compliance_policies SET version=version+1 WHERE brand_id=$1`, testBrand); e == nil {
		t.Fatal("unaudited policy committed")
	}
}
func TestPermissionsVersionsAndAuditFailureAreAtomic(t *testing.T) {
	s := Service{DB: testdb.New(t)}
	a := actor(t, s)
	ctx := context.Background()
	cfg := DefaultConfig()
	cfg.IdentityEnabled = true
	in := Input{Version: 1, Config: cfg, Reason: "Enable identity adapter configuration"}
	denied := a
	denied.SuperAdmin = true
	if _, e := update(t, s, denied, in); !errors.Is(e, ErrDenied) {
		t.Fatal("super wrote", e)
	}
	if _, e := check(t, s, denied, CheckInput{Version: 1, Operation: "betting", Reason: "should refuse"}); !errors.Is(e, ErrDenied) {
		t.Fatal("super ran", e)
	}
	denied = a
	denied.BrandIDs = nil
	if _, e := update(t, s, denied, in); !errors.Is(e, ErrDenied) {
		t.Fatal("no brand scope wrote", e)
	}
	missing := a
	missing.ID = ids.New()
	if _, e := update(t, s, missing, in); e == nil {
		t.Fatal("missing persistent actor succeeded")
	}
	p, e := s.Read(ctx, testBrand)
	if e != nil || p.Version != 1 || p.Config.IdentityEnabled {
		t.Fatal("partial policy survived", p, e)
	}
	if _, e = update(t, s, a, in); e != nil {
		t.Fatal(e)
	}
	if _, e = update(t, s, a, in); !errors.Is(e, ErrVersion) {
		t.Fatal("stale update succeeded", e)
	}
	if _, e = check(t, s, a, CheckInput{Version: 1, Operation: "betting", Reason: "stale read"}); !errors.Is(e, ErrVersion) {
		t.Fatal("stale check succeeded", e)
	}
	if _, e = s.DB.Exec(ctx, `UPDATE brands SET status='disabled' WHERE id=$1`, testBrand); e != nil {
		t.Fatal(e)
	}
	in.Version = 2
	if _, e = update(t, s, a, in); !errors.Is(e, ErrState) {
		t.Fatal("disabled wrote", e)
	}
	if _, e = check(t, s, a, CheckInput{Version: 2, Operation: "betting", Reason: "disabled check"}); !errors.Is(e, ErrState) {
		t.Fatal("disabled ran", e)
	}
	if _, e = s.Read(ctx, testBrand); e != nil {
		t.Fatal("disabled brand not readable", e)
	}
}
func TestConcurrentPolicyUpdateHasOneAuditedWinner(t *testing.T) {
	s := Service{DB: testdb.New(t)}
	a := actor(t, s)
	var won atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg := DefaultConfig()
			cfg.IdentityEnabled = true
			_, e := update(t, s, a, Input{Version: 1, Config: cfg, Reason: "Concurrent policy update"})
			if e == nil {
				won.Add(1)
			} else if !errors.Is(e, ErrVersion) {
				t.Errorf("unexpected update error %v", e)
			}
		}()
	}
	wg.Wait()
	if won.Load() != 1 {
		t.Fatal(won.Load())
	}
	var revisions, audits int
	if e := s.DB.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM compliance_policy_revisions WHERE brand_id=$1),(SELECT count(*) FROM audit_logs WHERE brand_id=$1 AND action='compliance.policy.update')`, testBrand).Scan(&revisions, &audits); e != nil || revisions != 2 || audits != 1 {
		t.Fatal(revisions, audits, e)
	}
}
func TestSQLValidatorRestorePathAndNewBrandInitialization(t *testing.T) {
	s := Service{DB: testdb.New(t)}
	ctx := context.Background()
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	var schema string
	if e = tx.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, `SET LOCAL search_path=''`); e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(DefaultConfig())
	var valid bool
	if e = tx.QueryRow(ctx, "SELECT "+pgx.Identifier{schema, "valid_compliance_config"}.Sanitize()+"($1::jsonb)", raw).Scan(&valid); e != nil || !valid {
		t.Fatal("empty restore path invalid", e)
	}
	for _, bad := range []string{`{}`, `{"age_enabled":true,"minimum_age":null,"region_enabled":false,"allowed_countries":[],"identity_enabled":false}`, `{"age_enabled":false,"minimum_age":null,"region_enabled":false,"allowed_countries":["US","PH"],"identity_enabled":false}`, `{"age_enabled":false,"minimum_age":null,"region_enabled":false,"allowed_countries":["PH","PH"],"identity_enabled":false}`} {
		if e = tx.QueryRow(ctx, "SELECT "+pgx.Identifier{schema, "valid_compliance_config"}.Sanitize()+"($1::jsonb)", bad).Scan(&valid); e != nil || valid {
			t.Fatal("SQL accepted invalid config", bad, e)
		}
	}
	id := ids.New()
	if _, e = tx.Exec(ctx, "INSERT INTO "+pgx.Identifier{schema, "brands"}.Sanitize()+"(id,code,name,status) VALUES($1,'compliance_new','New','paused')", id); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	p, e := s.Read(ctx, id)
	if e != nil || p.Version != 1 || p.Config.IdentityEnabled {
		t.Fatal(p, e)
	}
	h, e := s.History(ctx, id, 20, 0)
	if e != nil || h.TotalCount != "1" {
		t.Fatal(h, e)
	}
}
