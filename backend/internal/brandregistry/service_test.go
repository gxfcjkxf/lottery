package brandregistry

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func fixture(t *testing.T) (*pgxpool.Pool, access.Account) {
	t.Helper()
	db := testdb.New(t)
	id := ids.New()
	if _, err := db.Exec(context.Background(), `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,'registry_operator','not-a-login-hash')`, id); err != nil {
		t.Fatal(err)
	}
	return db, access.Account{ID: id, Type: access.AccountAdmin, Roles: []access.Role{{Permissions: []access.Permission{{Resource: "brand", Action: "create", Scope: access.ScopePlatform}}}}}
}

func input() Input {
	return Input{Code: "new_brand", Name: "新品牌", DefaultLocale: "zh-CN", Timezone: "Asia/Manila", Reason: "initialize independent brand"}
}

func create(t *testing.T, db *pgxpool.Pool, actor access.Account, in Input) (Receipt, error) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	out, err := Create(ctx, tx, actor, in, points.Metadata{RequestID: ids.New()})
	if err == nil {
		err = tx.Commit(ctx)
	}
	return out, err
}

func TestCreationInitializesIsolatedConfigurationWithoutFinancialOrAccountEffects(t *testing.T) {
	db, actor := fixture(t)
	ctx := context.Background()
	var before string
	if err := db.QueryRow(ctx, `SELECT md5(jsonb_agg(to_jsonb(b) ORDER BY id)::text) FROM brands b`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	in := input()
	out, err := create(t, db, actor, in)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "paused" || out.Version != 1 || out.AuditLogID == "" || out.Code != in.Code || out.Name != in.Name || out.DefaultLocale != in.DefaultLocale || out.Timezone != in.Timezone {
		t.Fatal("invalid creation receipt", out)
	}
	var configurations, scope, financial, creation int
	var disabled bool
	var mode *string
	var locale string
	err = db.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM brand_point_policies WHERE brand_id=$1)+(SELECT count(*) FROM brand_bet_policies WHERE brand_id=$1)+(SELECT count(*) FROM brand_withdrawal_policies WHERE brand_id=$1)+(SELECT count(*) FROM brand_settlement_policies WHERE brand_id=$1)+(SELECT count(*) FROM brand_agent_policies WHERE brand_id=$1)+(SELECT count(*) FROM brand_presentations WHERE brand_id=$1),
 (SELECT count(*) FROM admin_brand_scopes WHERE brand_id=$1),
 (SELECT count(*) FROM point_accounts WHERE brand_id=$1)+(SELECT count(*) FROM point_ledger_entries WHERE brand_id=$1)+(SELECT count(*) FROM bet_orders WHERE brand_id=$1)+(SELECT count(*) FROM brand_members WHERE brand_id=$1)+(SELECT count(*) FROM games WHERE brand_id=$1)+(SELECT count(*) FROM brand_domains WHERE brand_id=$1),
 (SELECT count(*) FROM brand_creation_records WHERE brand_id=$1),
 (SELECT (config->>'enabled')::boolean FROM brand_agent_policies WHERE brand_id=$1),
 (SELECT mode FROM brand_settlement_policies WHERE brand_id=$1),
 (SELECT config->>'default_locale' FROM brand_presentations WHERE brand_id=$1)`, out.ID).Scan(&configurations, &scope, &financial, &creation, &disabled, &mode, &locale)
	if err != nil || configurations != 6 || scope != 0 || financial != 0 || creation != 1 || disabled || mode != nil || locale != in.DefaultLocale {
		t.Fatalf("initialization configs=%d scopes=%d effects=%d history=%d agent=%v mode=%v locale=%s err=%v", configurations, scope, financial, creation, disabled, mode, locale, err)
	}
	var after string
	if err = db.QueryRow(ctx, `SELECT md5(jsonb_agg(to_jsonb(b) ORDER BY id)::text) FROM brands b WHERE id<>$1`, out.ID).Scan(&after); err != nil || after != before {
		t.Fatal("old brands changed", err)
	}
	for _, q := range []string{`UPDATE brand_creation_records SET reason='rewrite' WHERE brand_id=$1`, `DELETE FROM brand_creation_records WHERE brand_id=$1`} {
		if _, err = db.Exec(ctx, q, out.ID); err == nil {
			t.Fatal("creation history changed")
		}
	}
	if _, err = create(t, db, actor, in); !errors.Is(err, ErrCodeConflict) {
		t.Fatal("duplicate code accepted", err)
	}
}

func TestPermissionAndRollbackDoNotLeavePartialBrand(t *testing.T) {
	db, actor := fixture(t)
	in := input()
	denied := actor
	denied.SuperAdmin = true
	denied.Roles = nil
	if _, err := create(t, db, denied, in); !errors.Is(err, ErrDenied) {
		t.Fatal("super flag bypass", err)
	}
	denied.Roles = []access.Role{{BrandID: "0199a000-0000-7000-8000-000000000001", Permissions: actor.Roles[0].Permissions}}
	if _, err := create(t, db, denied, in); !errors.Is(err, ErrDenied) {
		t.Fatal("brand role supplied platform grant", err)
	}
	actor.ID = ids.New() // Missing persistent admin: final record FK must fail atomically.
	if _, err := create(t, db, actor, in); err == nil {
		t.Fatal("missing persistent creator accepted")
	}
	var n int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM brands WHERE code=$1`, in.Code).Scan(&n); err != nil || n != 0 {
		t.Fatal("partial brand survived rollback", n, err)
	}
}

func TestConcurrentSameCodeCreatesExactlyOneBrandAndWitness(t *testing.T) {
	db, actor := fixture(t)
	ctx := context.Background()
	start := make(chan struct{})
	results := make(chan error, 8)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			tx, err := db.Begin(ctx)
			if err != nil {
				results <- err
				return
			}
			defer tx.Rollback(ctx)
			_, err = Create(ctx, tx, actor, input(), points.Metadata{RequestID: ids.New()})
			if err == nil {
				err = tx.Commit(ctx)
			}
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrCodeConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	var records, audits int
	if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM brand_creation_records),(SELECT count(*) FROM audit_logs WHERE action='brand.create')`).Scan(&records, &audits); err != nil || success != 1 || conflict != 7 || records != 1 || audits != 1 {
		t.Fatalf("create race success=%d conflict=%d records=%d audits=%d err=%v", success, conflict, records, audits, err)
	}
}

func TestCreationInputIsClosedAndRequiresValidNamedTimezone(t *testing.T) {
	good := `{"code":"fresh","name":"Fresh","default_locale":"en","timezone":"Asia/Manila","reason":"new brand"}`
	var out Input
	if err := json.Unmarshal([]byte(good), &out); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`null`, `[]`, `{"code":"fresh"}`,
		`{"code":"fresh","code":"other","name":"Fresh","default_locale":"en","timezone":"UTC","reason":"new"}`,
		`{"code":"fresh","name":null,"default_locale":"en","timezone":"UTC","reason":"new"}`,
		`{"code":"fresh","name":"Fresh","default_locale":"en","timezone":"UTC","reason":"new","status":"active"}`,
		`{"code":"FRESH","name":"Fresh","default_locale":"en","timezone":"UTC","reason":"new"}`,
		`{"code":"fresh","name":" Fresh","default_locale":"en","timezone":"UTC","reason":"new"}`,
		`{"code":"fresh","name":"Fresh","default_locale":"fr","timezone":"UTC","reason":"new"}`,
		`{"code":"fresh","name":"Fresh","default_locale":"en","timezone":"Local","reason":"new"}`,
		`{"code":"fresh","name":"Fresh","default_locale":"en","timezone":"Bad/Zone","reason":"new"}`,
		`{"code":"fresh","name":"Fresh","default_locale":"en","timezone":"UTC","reason":"new\nline"}`,
		good + ` {}`,
	} {
		t.Run(raw, func(t *testing.T) {
			var in Input
			if err := json.Unmarshal([]byte(raw), &in); err == nil {
				t.Fatal("invalid creation input accepted")
			}
		})
	}
	in := input()
	in.Name = string(make([]byte, 121))
	if Valid(in) {
		t.Fatal("oversize/control name accepted")
	}
}
