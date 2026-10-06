package branddomains

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/tenant"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"testing"
	"time"
)

const brand = "0199a000-0000-7000-8000-000000000002"

func fixture(t *testing.T) (Service, access.Account) {
	t.Helper()
	p := testdb.New(t)
	id := ids.New()
	_, e := p.Exec(context.Background(), `INSERT INTO admin_accounts(id,username,password_hash)VALUES($1,$2,'test-only')`, id, "domains_"+id[:8])
	if e != nil {
		t.Fatal(e)
	}
	return Service{DB: p}, access.Account{ID: id, Type: access.AccountAdmin, BrandIDs: []string{brand}, Roles: []access.Role{{BrandID: brand, Permissions: []access.Permission{{Resource: "brand_domains", Action: "write", Scope: access.ScopeBrand}}}}}
}

func TestDomainGlobalReservationAndConcurrentVersion(t *testing.T) {
	s, a := fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, e := s.DB.Exec(ctx, `INSERT INTO platform_domains(domain)VALUES('platform.example.test')`); e != nil {
		t.Fatal(e)
	}
	r, e := s.Read(ctx, brand)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = change(t, s, a, "", Input{Version: r.Version, Domain: "platform.example.test", Enabled: true, Reason: "reserved platform host"}); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	results := make(chan error, 2)
	for _, host := range []string{"first.example.test", "second.example.test"} {
		go func(host string) {
			tx, e := s.DB.Begin(ctx)
			if e != nil {
				results <- e
				return
			}
			defer tx.Rollback(ctx)
			_, e = s.Change(ctx, tx, brand, a, "", Input{Version: r.Version, Domain: host, Enabled: true, Primary: true, Reason: "race brand shared version"}, points.Metadata{ActorType: "admin", ActorID: a.ID, RequestID: ids.New()})
			if e == nil {
				e = tx.Commit(ctx)
			}
			results <- e
		}(host)
	}
	success, conflicts := 0, 0
	for i := 0; i < 2; i++ {
		select {
		case e := <-results:
			if e == nil {
				success++
			} else if errors.Is(e, ErrVersion) {
				conflicts++
			} else {
				t.Fatal(e)
			}
		case <-ctx.Done():
			t.Fatal("concurrent domain changes did not finish", ctx.Err())
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatal(success, conflicts)
	}
	current, e := s.Read(ctx, brand)
	if e != nil || current.Version != r.Version+1 || len(current.Domains) != len(r.Domains)+1 {
		t.Fatal(current, e)
	}
	other := "0199a000-0000-7000-8000-000000000001"
	a.BrandIDs = append(a.BrandIDs, other)
	a.Roles = append(a.Roles, access.Role{BrandID: other, Permissions: []access.Permission{{Resource: "brand_domains", Action: "write", Scope: access.ScopeBrand}}})
	for _, b := range []string{brand, other} {
		go func(b string) {
			tx, e := s.DB.Begin(ctx)
			if e != nil {
				results <- e
				return
			}
			defer tx.Rollback(ctx)
			before, e := s.Read(ctx, b)
			if e == nil {
				_, e = s.Change(ctx, tx, b, a, "", Input{Version: before.Version, Domain: "race.example.test", Enabled: true, Reason: "global namespace race"}, points.Metadata{ActorType: "admin", ActorID: a.ID, RequestID: ids.New()})
			}
			if e == nil {
				e = tx.Commit(ctx)
			}
			results <- e
		}(b)
	}
	success, conflicts = 0, 0
	for i := 0; i < 2; i++ {
		select {
		case e := <-results:
			if e == nil {
				success++
			} else if errors.Is(e, ErrConflict) {
				conflicts++
			} else {
				t.Fatal(e)
			}
		case <-ctx.Done():
			t.Fatal("global namespace race did not finish", ctx.Err())
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatal(success, conflicts)
	}
	var count int
	if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM brand_domains WHERE domain='race.example.test'`).Scan(&count); e != nil || count != 1 {
		t.Fatal(count, e)
	}
}
func change(t *testing.T, s Service, a access.Account, id string, in Input) (Record, error) {
	t.Helper()
	ctx := context.Background()
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	out, e := s.Change(ctx, tx, brand, a, id, in, points.Metadata{ActorType: "admin", ActorID: a.ID, RequestID: ids.New()})
	if e != nil {
		return out, e
	}
	return out, tx.Commit(ctx)
}
func TestDomainInputClosedAndCanonical(t *testing.T) {
	for _, host := range []string{"brand.example.com", "a.example.test", "xn--brand-123.example"} {
		if !ValidDomain(host) {
			t.Fatal(host)
		}
	}
	for _, host := range []string{"localhost", "a.localhost", "127.0.0.1", "127.1", "https://example.com", "a.example:443", "a.example.", "A.example", "*.example.com", "a..example", "-a.example", "a.internal", "a.local", "a.123", "a.example/path"} {
		if ValidDomain(host) {
			t.Fatal("accepted", host)
		}
	}
	for _, raw := range []string{`{"version":1,"domain":"a.example","enabled":true,"is_primary":false,"reason":"test"}`, `{"version":1,"enabled":true,"is_primary":false,"reason":"test"}`} {
		var in Input
		if json.Unmarshal([]byte(raw), &in) != nil {
			t.Fatal(raw)
		}
	}
	for _, raw := range []string{`{"version":1,"enabled":null,"is_primary":false,"reason":"test"}`, `{"version":1,"enabled":true,"reason":"test"}`, `{"version":1,"version":1,"enabled":true,"is_primary":false,"reason":"test"}`, `{"version":1,"enabled":true,"is_primary":false,"reason":"test","extra":1}`} {
		var in Input
		if json.Unmarshal([]byte(raw), &in) == nil {
			t.Fatal("accepted", raw)
		}
	}
}
func TestDomainChangesResolveAndPreserveAuditedSnapshots(t *testing.T) {
	s, a := fixture(t)
	ctx := context.Background()
	before, e := s.Read(ctx, brand)
	if e != nil {
		t.Fatal(e)
	}
	next, e := change(t, s, a, "", Input{Version: before.Version, Domain: "new.example.test", Enabled: true, Primary: true, Reason: "bind new primary"})
	if e != nil || next.Version != before.Version+1 || next.AuditLogID == "" {
		t.Fatal(next, e)
	}
	var added Domain
	primary := 0
	for _, d := range next.Domains {
		if d.Domain == "new.example.test" {
			added = d
		}
		if d.Primary && d.Enabled {
			primary++
		}
	}
	if primary != 1 || added.ID == "" {
		t.Fatal(next)
	}
	resolved, e := (tenant.Store{DB: s.DB}).Resolve(ctx, "new.example.test:443", "")
	if e != nil || resolved.ID != brand {
		t.Fatal(resolved, e)
	}
	history, e := s.History(ctx, brand, 20, 0)
	if e != nil || len(history) != 1 || len(history[0].BeforeDomains) != len(before.Domains) || len(history[0].Domains) != len(next.Domains) {
		t.Fatal(history, e)
	}
	off, e := change(t, s, a, added.ID, Input{Version: next.Version, Enabled: false, Primary: false, Reason: "disable binding"})
	if e != nil || off.Version != next.Version+1 {
		t.Fatal(off, e)
	}
	if _, e = (tenant.Store{DB: s.DB}).Resolve(ctx, "new.example.test", ""); !errors.Is(e, tenant.ErrNotFound) {
		t.Fatal(e)
	}
	if _, e = change(t, s, a, "", Input{Version: off.Version, Domain: "new.example.test", Reason: "reserved disabled domain"}); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	if _, e = change(t, s, a, added.ID, Input{Version: off.Version, Domain: "rename.example.test", Reason: "rename forbidden"}); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(ctx, `DELETE FROM brand_domains WHERE id=$1`, added.ID); e == nil {
		t.Fatal("hard delete allowed")
	}
	if _, e = s.DB.Exec(ctx, `UPDATE brand_domains SET domain='changed.example.test' WHERE id=$1`, added.ID); e == nil {
		t.Fatal("rename allowed")
	}
	if _, e = s.DB.Exec(ctx, `UPDATE brand_domains SET enabled=true WHERE id=$1`, added.ID); e == nil {
		t.Fatal("unaudited update allowed")
	}
	if _, e = s.DB.Exec(ctx, `UPDATE brand_domains SET brand_id='0199a000-0000-7000-8000-000000000001' WHERE id=$1`, added.ID); e == nil {
		t.Fatal("brand reassignment allowed")
	}
	for _, sql := range []string{`UPDATE brand_domain_revisions SET reason='rewritten' WHERE brand_id=$1`, `DELETE FROM brand_domain_revisions WHERE brand_id=$1`} {
		if _, e = s.DB.Exec(ctx, sql, brand); e == nil {
			t.Fatal("domain history modified", sql)
		}
	}
	var ledger, orders int
	_ = s.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM point_ledger_entries),(SELECT count(*) FROM bet_orders)`).Scan(&ledger, &orders)
	if ledger != 0 || orders != 0 {
		t.Fatal("financial mutation", ledger, orders)
	}
}

func TestDomainBindingLimitCountsDisabledReservations(t *testing.T) {
	s, a := fixture(t)
	ctx := context.Background()
	r, e := s.Read(ctx, brand)
	if e != nil {
		t.Fatal(e)
	}
	for i := len(r.Domains); i < 100; i++ {
		if _, e = s.DB.Exec(ctx, `INSERT INTO brand_domains(id,brand_id,domain,enabled)VALUES($1,$2,$3,false)`, ids.New(), brand, fmt.Sprintf("reserved-%d.example.test", i)); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = change(t, s, a, "", Input{Version: r.Version, Domain: "overflow.example.test", Enabled: true, Reason: "respect reservation limit"}); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	after, e := s.Read(ctx, brand)
	if e != nil || after.Version != r.Version || len(after.Domains) != 100 {
		t.Fatal(after, e)
	}
}
func TestDomainAuthorizationVersionStateAndRollback(t *testing.T) {
	s, a := fixture(t)
	ctx := context.Background()
	r, e := s.Read(ctx, brand)
	if e != nil {
		t.Fatal(e)
	}
	in := Input{Version: r.Version, Domain: "safe.example.test", Enabled: true, Reason: "bind"}
	foreign := a
	foreign.BrandIDs = []string{"0199a000-0000-7000-8000-000000000001"}
	foreign.Roles = nil
	foreign.SuperAdmin = true
	if _, e = change(t, s, foreign, "", in); !errors.Is(e, ErrDenied) {
		t.Fatal(e)
	}
	in.Version++
	if _, e = change(t, s, a, "", in); !errors.Is(e, ErrVersion) {
		t.Fatal(e)
	}
	in.Version = r.Version
	in.Domain = "harbor.localhost"
	if _, e = change(t, s, a, "", in); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	in.Domain = "safe.example.test"
	if _, e = s.DB.Exec(ctx, `CREATE FUNCTION reject_domain_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='brand_domains.update' THEN RAISE EXCEPTION 'audit rejected';END IF;RETURN NEW;END $$;CREATE TRIGGER reject_domain_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_domain_audit()`); e != nil {
		t.Fatal(e)
	}
	if _, e = change(t, s, a, "", in); e == nil {
		t.Fatal("audit failure ignored")
	}
	same, e := s.Read(ctx, brand)
	if e != nil || same.Version != r.Version || len(same.Domains) != len(r.Domains) {
		t.Fatal(same, e)
	}
	if _, e = s.DB.Exec(ctx, `UPDATE brands SET status='disabled' WHERE id=$1`, brand); e != nil {
		t.Fatal(e)
	}
	if _, e = change(t, s, a, "", in); !errors.Is(e, ErrState) {
		t.Fatal(e)
	}
}
