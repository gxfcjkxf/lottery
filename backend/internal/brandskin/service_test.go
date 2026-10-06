package brandskin

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"testing"
)

const testBrand = "0199a000-0000-7000-8000-000000000002"

func fixture(t *testing.T) (Service, access.Account) {
	t.Helper()
	p := testdb.New(t)
	id := ids.New()
	if _, e := p.Exec(context.Background(), `INSERT INTO admin_accounts(id,username,password_hash)VALUES($1,$2,'test-only')`, id, "skin_"+id[:8]); e != nil {
		t.Fatal(e)
	}
	return Service{DB: p}, access.Account{ID: id, Type: access.AccountAdmin, BrandIDs: []string{testBrand}, Roles: []access.Role{{BrandID: testBrand, Permissions: []access.Permission{{Resource: "brand_presentation", Action: "write", Scope: access.ScopeBrand}}}}}
}
func change(t *testing.T, s Service, a access.Account, in Input) (Record, error) {
	t.Helper()
	ctx := context.Background()
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	out, e := s.Update(ctx, tx, testBrand, a, in, points.Metadata{ActorType: "admin", ActorID: a.ID, RequestID: ids.New()})
	if e != nil {
		return out, e
	}
	e = tx.Commit(ctx)
	return out, e
}
func TestPresentationPersistsAuditedSnapshotsWithoutFinancialChanges(t *testing.T) {
	s, a := fixture(t)
	ctx := context.Background()
	initial, e := s.Read(ctx, testBrand)
	if e != nil {
		t.Fatal(e)
	}
	if initial.Config.PrimaryColor == nil || *initial.Config.PrimaryColor != "#274c75" || initial.Effective.PrimaryColor != "#274c75" {
		t.Fatal(initial)
	}
	name, color, locale := "月湾展示", "#123456", "zh-CN"
	cfg := initial.Config
	cfg.DisplayName = &name
	cfg.PrimaryColor = &color
	cfg.DefaultLocale = &locale
	next, e := change(t, s, a, Input{Version: initial.Version, Config: cfg, Reason: "publish audited brand presentation"})
	if e != nil || next.Version != initial.Version+1 || next.AuditLogID == "" || next.Effective.DisplayName != name {
		t.Fatal(next, e)
	}
	history, e := s.History(ctx, testBrand, 20, 0)
	if e != nil || len(history) != 1 || history[0].ChangedBy != a.ID || history[0].Effective.PrimaryColor != color {
		t.Fatal(history, e)
	}
	if _, e = change(t, s, a, Input{Version: initial.Version, Config: cfg, Reason: "stale version"}); !errors.Is(e, ErrVersion) {
		t.Fatal(e)
	}
	// Auth/operation and presentation share the version; historical snapshots do not change.
	if _, e = s.DB.Exec(ctx, `UPDATE brands SET config_version=config_version+1 WHERE id=$1`, testBrand); e != nil {
		t.Fatal(e)
	}
	current, e := s.Read(ctx, testBrand)
	if e != nil || current.Version != next.Version+1 || current.AuditLogID != "" {
		t.Fatal(current, e)
	}
	reset, e := change(t, s, a, Input{Version: current.Version, Config: Config{}, Reason: "restore platform inheritance"})
	if e != nil || reset.Effective.PrimaryColor != "#20594c" || reset.Effective.DisplayName != "Harbor" {
		t.Fatal(reset, e)
	}
	old, e := s.History(ctx, testBrand, 20, 1)
	if e != nil || len(old) != 1 || old[0].Config.PrimaryColor == nil || *old[0].Config.PrimaryColor != color {
		t.Fatal(old, e)
	}
	for _, query := range []string{`UPDATE brand_presentation_revisions SET reason='changed'`, `DELETE FROM brand_presentation_revisions`, `DELETE FROM brand_presentations WHERE brand_id='0199a000-0000-7000-8000-000000000002'`} {
		if _, e = s.DB.Exec(ctx, query); e == nil {
			t.Fatal("immutable presentation evidence was changed")
		}
	}
	var funds int
	if e = s.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM point_ledger_entries)+(SELECT count(*) FROM bet_orders)`).Scan(&funds); e != nil || funds != 0 {
		t.Fatal(funds, e)
	}
}
func TestPresentationRejectsForeignScopeDisabledAndAuditFailure(t *testing.T) {
	s, a := fixture(t)
	ctx := context.Background()
	initial, e := s.Read(ctx, testBrand)
	if e != nil {
		t.Fatal(e)
	}
	outsider := a
	outsider.BrandIDs = []string{"0199a000-0000-7000-8000-000000000001"}
	if _, e = change(t, s, outsider, Input{Version: initial.Version, Config: initial.Config, Reason: "foreign scope"}); !errors.Is(e, ErrDenied) {
		t.Fatal(e)
	}
	if Allowed(access.Account{ID: a.ID, Type: access.AccountAdmin, SuperAdmin: true}, testBrand, "write") {
		t.Fatal("implicit super permission")
	}
	if _, e = s.DB.Exec(ctx, `CREATE FUNCTION fail_skin_audit()RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='brand_presentation.update' THEN RAISE EXCEPTION 'test audit failure';END IF;RETURN NEW;END $$;CREATE TRIGGER fail_skin_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION fail_skin_audit()`); e != nil {
		t.Fatal(e)
	}
	if _, e = change(t, s, a, Input{Version: initial.Version, Config: Config{}, Reason: "audit rollback"}); e == nil {
		t.Fatal("audit failure accepted")
	}
	unchanged, e := s.Read(ctx, testBrand)
	if e != nil || unchanged.Version != initial.Version || unchanged.Config.PrimaryColor == nil {
		t.Fatal(unchanged, e)
	}
	if _, e = s.DB.Exec(ctx, `UPDATE brands SET status='disabled' WHERE id=$1`, testBrand); e != nil {
		t.Fatal(e)
	}
	if _, e = change(t, s, a, Input{Version: initial.Version, Config: Config{}, Reason: "disabled write"}); !errors.Is(e, ErrState) {
		t.Fatal(e)
	}
}
func TestPresentationDatabaseRequiresMatchingHistory(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `UPDATE brands SET config_version=config_version+1 WHERE id=$1`, testBrand); e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(Config{})
	if _, e = tx.Exec(ctx, `UPDATE brand_presentations SET config=$2,version=version+1 WHERE brand_id=$1`, testBrand, raw); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e == nil {
		t.Fatal("unaudited presentation committed")
	}
}
func TestPresentationSameVersionHasOneWinner(t *testing.T) {
	s, a := fixture(t)
	ctx := context.Background()
	before, e := s.Read(ctx, testBrand)
	if e != nil {
		t.Fatal(e)
	}
	first, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer first.Rollback(ctx)
	if _, e = s.Update(ctx, first, testBrand, a, Input{Version: before.Version, Config: Config{}, Reason: "first publisher"}, points.Metadata{ActorType: "admin", ActorID: a.ID, RequestID: ids.New()}); e != nil {
		t.Fatal(e)
	}
	second, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer second.Rollback(ctx)
	result := make(chan error, 1)
	go func() {
		_, e := s.Update(ctx, second, testBrand, a, Input{Version: before.Version, Config: before.Config, Reason: "second publisher"}, points.Metadata{ActorType: "admin", ActorID: a.ID, RequestID: ids.New()})
		result <- e
	}()
	if e = first.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if e = <-result; !errors.Is(e, ErrVersion) {
		t.Fatal(e)
	}
}
