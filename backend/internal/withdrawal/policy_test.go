package withdrawal

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
)

const policyBrand = "0199a000-0000-7000-8000-000000000001"
const policyOther = "0199a000-0000-7000-8000-000000000002"

func fixture(t *testing.T) (Service, access.Account, string) {
	t.Helper()
	db := testdb.New(t)
	ctx := context.Background()
	id := ids.New()
	if _, e := db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only')`, id, "withdraw_"+id[:8]); e != nil {
		t.Fatal(e)
	}
	a := access.Account{ID: id, Type: access.AccountAdmin, BrandIDs: []string{policyBrand}, Roles: []access.Role{{BrandID: policyBrand, Permissions: []access.Permission{{Resource: "withdrawal_policy", Action: "write", Scope: access.ScopeBrand}, {Resource: "game", Action: "write", Scope: access.ScopeBrand}}}}}
	var game rulebook.Game
	transact(t, Service{DB: db}, func(tx pgx.Tx) error {
		var e error
		game, e = (rulebook.Store{DB: db}).CreateGame(ctx, tx, policyBrand, a, "withdraw_policy_digits", "Withdrawal policy model", rules.Model{Type: "DIGITS_0_9", Length: 3, AllowRepeat: true, Ordered: true}, "UTC", "policy fixture", metadata(a.ID))
		return e
	})
	return Service{DB: db}, a, game.ID
}
func metadata(id string) points.Metadata {
	return points.Metadata{ActorType: "admin", ActorID: id, RequestID: ids.New()}
}
func transact(t *testing.T, s Service, fn func(pgx.Tx) error) {
	t.Helper()
	ctx := context.Background()
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if e = fn(tx); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
}
func str(v string) *string { return &v }
func TestConfigParsingIsExactClosedAndCanonical(t *testing.T) {
	for _, v := range []string{"1", "2.5", "0.000001", "999999.999999", "1000000"} {
		if !ValidMultiple(v) {
			t.Errorf("valid N rejected %q", v)
		}
	}
	for _, v := range []string{"", "0", "01", "+1", "-1", "1.0", "1.50", ".1", "1.", "1e2", "0.0000001", "1000000.000001", " 2", "1/2"} {
		if ValidMultiple(v) {
			t.Errorf("invalid N accepted %q", v)
		}
	}
	zeroBrandConfig := DefaultBrandConfig()
	zeroBrandConfig.TurnoverMultiple = "0"
	if ValidateBrandConfig(zeroBrandConfig) == nil || ValidateGameConfig(GameConfig{TurnoverMultiple: str("0")}) == nil {
		t.Fatal("new policy validation accepted zero N")
	}
	if ValidateGameConfig(GameConfig{}) != nil {
		t.Fatal("blank game override must continue to inherit")
	}
	raw := `{"version":1,"config":{"enabled":false,"min_points":"1","max_points":null,"allowed_sources":["recharge","winning","gift"],"review_mode":"manual","turnover_multiple":"1"},"reason":"configured"}`
	var in BrandInput
	if e := json.Unmarshal([]byte(raw), &in); e != nil {
		t.Fatal(e)
	}
	if got := DefaultBrandConfig().AllowedSources; strings.Join(got, ",") != "recharge,winning,gift" {
		t.Fatalf("default authorization changed: %v", got)
	}
	commissionPolicy := strings.Replace(raw, `"recharge","winning","gift"`, `"commission"`, 1)
	var commissionInput BrandInput
	if e := json.Unmarshal([]byte(commissionPolicy), &commissionInput); e != nil || len(commissionInput.Config.AllowedSources) != 1 || commissionInput.Config.AllowedSources[0] != "commission" {
		t.Fatalf("explicit commission authorization rejected: %+v err=%v", commissionInput, e)
	}
	fourSourcePolicy := strings.Replace(raw, `"recharge","winning","gift"`, `"recharge","winning","gift","commission"`, 1)
	if e := json.Unmarshal([]byte(fourSourcePolicy), &commissionInput); e != nil || len(commissionInput.Config.AllowedSources) != 4 {
		t.Fatalf("four-source authorization rejected: %+v err=%v", commissionInput, e)
	}
	fiveSourcePolicy := strings.Replace(fourSourcePolicy, `"commission"`, `"commission","extra"`, 1)
	if json.Unmarshal([]byte(fiveSourcePolicy), &commissionInput) == nil {
		t.Fatal("oversized allowed source policy accepted")
	}
	legacyBrandConfig := `{"enabled":false,"min_points":"1","max_points":null,"allowed_sources":["recharge","winning","gift"],"review_mode":"manual","turnover_multiple":"0"}`
	var savedBrand BrandConfig
	if e := json.Unmarshal([]byte(legacyBrandConfig), &savedBrand); e != nil || savedBrand.TurnoverMultiple != "0" {
		t.Fatalf("legacy zero brand config must remain readable: %+v, %v", savedBrand, e)
	}
	legacyBrand := strings.Replace(raw, `"turnover_multiple":"1"`, `"turnover_multiple":"0"`, 1)
	var rejectedBrand BrandInput
	if json.Unmarshal([]byte(legacyBrand), &rejectedBrand) == nil {
		t.Fatal("new brand write accepted zero N")
	}
	for _, bad := range []string{strings.Replace(raw, `"enabled":false`, `"enabled":null`, 1), strings.Replace(raw, `"max_points":null,`, "", 1), strings.Replace(raw, `"version":1`, `"version":1,"version":2`, 1), strings.Replace(raw, `"enabled":false`, `"enabled":false,"enabled":true`, 1), strings.Replace(raw, `"review_mode":"manual"`, `"review_mode":"manual","hidden":true`, 1), strings.Replace(raw, `"min_points":"1"`, `"min_points":1`, 1), strings.Replace(raw, `"allowed_sources":["recharge","winning","gift"]`, `"allowed_sources":["gift","gift"]`, 1), strings.Replace(raw, `"min_points":"1"`, `"min_points":"9223372036854775808"`, 1)} {
		var got BrandInput
		if json.Unmarshal([]byte(bad), &got) == nil {
			t.Fatalf("bad config accepted %s", bad)
		}
	}
	for _, raw := range []string{`{}`, `{"turnover_multiple":"1","turnover_multiple":null}`, `{"turnover_multiple":1}`, `{"turnover_multiple":null,"unknown":0}`} {
		var c GameConfig
		if json.Unmarshal([]byte(raw), &c) == nil {
			t.Fatal(raw)
		}
	}
	for _, raw := range []string{`{"turnover_multiple":null}`, `{"turnover_multiple":"0"}`, `{"turnover_multiple":"2.5"}`} {
		var c GameConfig
		if e := json.Unmarshal([]byte(raw), &c); e != nil {
			t.Fatal(raw, e)
		}
		if strings.Contains(raw, `"0"`) {
			write := `{"version":1,"config":` + raw + `,"reason":"legacy"}`
			var input GameInput
			if json.Unmarshal([]byte(write), &input) == nil {
				t.Fatal("new game policy write accepted zero N")
			}
		}
	}
}
func TestPoliciesInitializeResolveExactOverridesAndKeepImmutableHistory(t *testing.T) {
	s, a, game := fixture(t)
	ctx := context.Background()
	b, e := s.BrandPolicy(ctx, policyBrand)
	if e != nil || b.Version != 1 || !reflect.DeepEqual(b.Config, DefaultBrandConfig()) {
		t.Fatal(b, e)
	}
	g, e := s.GamePolicy(ctx, policyBrand, game)
	if e != nil || g.Config.TurnoverMultiple != nil || g.Effective.Source != "brand" || g.Effective.TurnoverMultiple != "1" {
		t.Fatal(g, e)
	}
	c := b.Config
	c.Enabled = true
	c.MinPoints = 10
	cap := points.Amount(100)
	c.MaxPoints = &cap
	c.TurnoverMultiple = "2.5"
	c.AllowedSources = []string{"recharge", "winning"}
	transact(t, s, func(tx pgx.Tx) error {
		var e error
		b, e = s.UpdateBrand(ctx, tx, policyBrand, a, BrandInput{Version: 1, Config: c, Reason: "first brand policy"}, metadata(a.ID))
		return e
	})
	if b.Version != 2 || b.AuditLogID == "" {
		t.Fatal(b)
	}
	transact(t, s, func(tx pgx.Tx) error {
		var e error
		g, e = s.UpdateGame(ctx, tx, policyBrand, game, a, GameInput{Version: 1, Config: GameConfig{TurnoverMultiple: str("3")}, Reason: "explicit positive override"}, metadata(a.ID))
		return e
	})
	if g.Effective.Source != "game" || g.Effective.TurnoverMultiple != "3" || g.Effective.BrandVersion != 2 || g.Version != 2 {
		t.Fatal(g)
	}
	c.TurnoverMultiple = "4.5"
	transact(t, s, func(tx pgx.Tx) error {
		var e error
		b, e = s.UpdateBrand(ctx, tx, policyBrand, a, BrandInput{Version: 2, Config: c, Reason: "new default preserves override"}, metadata(a.ID))
		return e
	})
	g, e = s.GamePolicy(ctx, policyBrand, game)
	if e != nil || g.Effective.TurnoverMultiple != "3" || g.Effective.BrandVersion != 3 {
		t.Fatal(g, e)
	}
	transact(t, s, func(tx pgx.Tx) error {
		var e error
		g, e = s.UpdateGame(ctx, tx, policyBrand, game, a, GameInput{Version: 2, Config: GameConfig{}, Reason: "return to brand inheritance"}, metadata(a.ID))
		return e
	})
	if g.Effective.Source != "brand" || g.Effective.TurnoverMultiple != "4.5" || g.Version != 3 {
		t.Fatal(g)
	}
	for _, scope := range []string{"", game} {
		h, e := s.History(ctx, policyBrand, scope, 50, 0)
		if e != nil || len(h) != 3 || h[0].Version != 3 || h[2].Version != 1 || h[2].ChangedBy != "" || h[0].ChangedBy != a.ID {
			t.Fatal(h, e)
		}
		p, e := s.History(ctx, policyBrand, scope, 1, 1)
		if e != nil || len(p) != 1 || p[0].ID != h[1].ID {
			t.Fatal(p, e)
		}
	}
	h, e := s.History(ctx, policyBrand, "", 50, 0)
	if e != nil {
		t.Fatal(e)
	}
	var old BrandConfig
	if e = json.Unmarshal(h[1].Config, &old); e != nil || old.TurnoverMultiple != "2.5" {
		t.Fatal(old, e)
	}
	var entries, orders int
	if e = s.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM point_ledger_entries),(SELECT count(*) FROM bet_orders)`).Scan(&entries, &orders); e != nil || entries != 0 || orders != 0 {
		t.Fatal(entries, orders, e)
	}
	if _, e = s.GamePolicy(ctx, policyOther, game); !errors.Is(e, ErrNotFound) {
		t.Fatal("cross brand", e)
	}
	if _, e = s.History(ctx, policyOther, game, 50, 0); !errors.Is(e, ErrNotFound) {
		t.Fatal("cross history", e)
	}
}
func TestPolicyConcurrentWritersHaveOneWinnerAndRollbackAuditFailure(t *testing.T) {
	s, a, _ := fixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, n := range []string{"2", "3"} {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			tx, e := s.DB.Begin(ctx)
			if e != nil {
				results <- e
				return
			}
			defer tx.Rollback(ctx)
			c := DefaultBrandConfig()
			c.TurnoverMultiple = n
			_, e = s.UpdateBrand(ctx, tx, policyBrand, a, BrandInput{Version: 1, Config: c, Reason: "concurrent update"}, metadata(a.ID))
			if e == nil {
				e = tx.Commit(ctx)
			}
			results <- e
		}(n)
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else if errors.Is(e, ErrVersion) {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal(success, conflict)
	}
	before, e := s.BrandPolicy(ctx, policyBrand)
	if e != nil {
		t.Fatal(e)
	}
	h, e := s.History(ctx, policyBrand, "", 50, 0)
	if e != nil || len(h) != 2 {
		t.Fatal(h, e)
	}
	if _, e = s.DB.Exec(ctx, `CREATE FUNCTION fail_withdraw_policy_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='withdrawal_policy.brand.update' THEN RAISE EXCEPTION 'policy audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_withdraw_policy_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION fail_withdraw_policy_audit()`); e != nil {
		t.Fatal(e)
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	c := DefaultBrandConfig()
	c.TurnoverMultiple = "5"
	_, e = s.UpdateBrand(ctx, tx, policyBrand, a, BrandInput{Version: before.Version, Config: c, Reason: "must rollback"}, metadata(a.ID))
	_ = tx.Rollback(ctx)
	if e == nil {
		t.Fatal("audit failure not propagated")
	}
	after, e := s.BrandPolicy(ctx, policyBrand)
	if e != nil || !reflect.DeepEqual(before, after) {
		t.Fatal(before, after, e)
	}
	h, e = s.History(ctx, policyBrand, "", 50, 0)
	if e != nil || len(h) != 2 {
		t.Fatal(h, e)
	}
}
func TestDatabaseRejectsOrphanRevisionUnwitnessedConfigAndHistoryMutation(t *testing.T) {
	s, a, game := fixture(t)
	ctx := context.Background()
	raw, _ := json.Marshal(DefaultBrandConfig())
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	_, e = tx.Exec(ctx, `INSERT INTO withdrawal_policy_revisions(brand_id,version,config,changed_by,reason) VALUES($1,2,$2,$3,'orphan revision')`, policyBrand, raw, a.ID)
	if e != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e == nil {
		t.Fatal("orphan committed")
	}
	tx, e = s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	_, e = tx.Exec(ctx, `UPDATE brand_withdrawal_policies SET version=version+1 WHERE brand_id=$1`, policyBrand)
	if e != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e == nil {
		t.Fatal("config committed without revision")
	}
	for _, q := range []string{`UPDATE withdrawal_policy_revisions SET reason='rewritten' WHERE brand_id=$1`, `DELETE FROM withdrawal_policy_revisions WHERE brand_id=$1`, `DELETE FROM brand_withdrawal_policies WHERE brand_id=$1`, `UPDATE game_withdrawal_policies SET brand_id='0199a000-0000-7000-8000-000000000002',version=version+1 WHERE brand_id=$1`} {
		if _, e = s.DB.Exec(ctx, q, policyBrand); e == nil {
			t.Fatal("guard bypass", q)
		}
	}
	for _, bad := range []string{`{"enabled":true}`, `{"enabled":false,"min_points":"1","max_points":null,"allowed_sources":["gift","gift"],"review_mode":"manual","turnover_multiple":"1"}`, `{"enabled":false,"min_points":"10","max_points":"9","allowed_sources":["gift"],"review_mode":"manual","turnover_multiple":"1"}`, `{"enabled":false,"min_points":"1","max_points":null,"allowed_sources":["gift"],"review_mode":"manual","turnover_multiple":"1.0"}`} {
		if _, e = s.DB.Exec(ctx, `UPDATE brand_withdrawal_policies SET config=$2,version=version+1 WHERE brand_id=$1`, policyBrand, bad); e == nil {
			t.Fatal("bad persisted config accepted", bad)
		}
	}
	b, e := s.BrandPolicy(ctx, policyBrand)
	if e != nil || b.Version != 1 {
		t.Fatal(b, e)
	}
	g, e := s.GamePolicy(ctx, policyBrand, game)
	if e != nil || g.Version != 1 {
		t.Fatal(g, e)
	}
}
func TestPolicyWriteDeniesSuperMissingGrantForeignScopeAndStaleVersions(t *testing.T) {
	s, a, game := fixture(t)
	ctx := context.Background()
	in := BrandInput{Version: 1, Config: DefaultBrandConfig(), Reason: "checked update"}
	for _, actor := range []access.Account{func() access.Account { v := a; v.SuperAdmin = true; return v }(), {ID: a.ID, Type: access.AccountAdmin, BrandIDs: []string{policyBrand}}, func() access.Account {
		v := a
		v.BrandIDs = []string{policyOther}
		v.Roles = []access.Role{{BrandID: policyOther, Permissions: []access.Permission{{Resource: "withdrawal_policy", Action: "write", Scope: access.ScopeBrand}}}}
		return v
	}()} {
		tx, e := s.DB.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		_, e = s.UpdateBrand(ctx, tx, policyBrand, actor, in, metadata(actor.ID))
		_ = tx.Rollback(ctx)
		if !errors.Is(e, ErrDenied) {
			t.Fatal(e)
		}
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	in.Version = 2
	_, e = s.UpdateBrand(ctx, tx, policyBrand, a, in, metadata(a.ID))
	_ = tx.Rollback(ctx)
	if !errors.Is(e, ErrVersion) {
		t.Fatal(e)
	}
	tx, e = s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.UpdateGame(ctx, tx, policyBrand, game, a, GameInput{Version: 2, Reason: "stale", Config: GameConfig{}}, metadata(a.ID))
	_ = tx.Rollback(ctx)
	if !errors.Is(e, ErrVersion) {
		t.Fatal(e)
	}
}
