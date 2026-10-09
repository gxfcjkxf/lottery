package database_test

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/tenant"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"strings"
	"testing"
)

func isolated(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run real PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := "test_" + strings.ReplaceAll(ids.New(), "-", "")
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	c, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	c.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, e := admin.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE")
		if e != nil {
			t.Error(e)
		}
		admin.Close()
	})
	return pool
}
func TestMigrationsAndSeedRepeatable(t *testing.T) {
	p := isolated(t)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if err := database.Migrate(ctx, p); err != nil {
			t.Fatal(err)
		}
		if err := database.Seed(ctx, p, "test"); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM schema_migrations WHERE name='0001_baseline.up.sql'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("single baseline count %d err %v", count, err)
	}
	if err := p.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("unexpected incremental migration history: %d err %v", count, err)
	}
	if err := p.QueryRow(ctx, "SELECT count(*) FROM brands").Scan(&count); err != nil || count != 2 {
		t.Fatalf("seed count %d err %v", count, err)
	}
	s := tenant.Store{DB: p}
	a, err := s.Resolve(ctx, "LOCALHOST:5173", "")
	if err != nil || a.Code != "aurora" {
		t.Fatalf("domain resolve: %+v %v", a, err)
	}
	b, err := s.Resolve(ctx, "localhost", "harbor")
	if err != nil || b.Code != "harbor" || b.ID == a.ID {
		t.Fatal("platform path isolation", err)
	}
	if _, err = s.Resolve(ctx, "unconfigured.test", "harbor"); err != tenant.ErrNotFound {
		t.Fatal("untrusted host path resolution", err)
	}
	if _, err = s.Resolve(ctx, "localhost", "unknown"); err != tenant.ErrNotFound {
		t.Fatal("unknown path brand", err)
	}
	if err = database.Seed(ctx, p, "production"); err == nil {
		t.Fatal("production seed was allowed")
	}
}

func TestBaselineRefusesOldMigrationHistoryWithoutChangingIt(t *testing.T) {
	p := isolated(t)
	ctx := context.Background()
	if _, err := p.Exec(ctx, `CREATE TABLE schema_migrations(name text PRIMARY KEY,checksum text NOT NULL,applied_at timestamptz NOT NULL DEFAULT now()); INSERT INTO schema_migrations(name,checksum) VALUES('0041_withdrawal_reports.up.sql','old-test-only')`); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, p); err == nil {
		t.Fatal("old migration history was accepted")
	}
	var count int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM schema_migrations WHERE name='0041_withdrawal_reports.up.sql' AND checksum='old-test-only'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("old metadata changed", count, err)
	}
	var brands *string
	if err := p.QueryRow(ctx, `SELECT to_regclass('brands')::text`).Scan(&brands); err != nil || brands != nil {
		t.Fatal("rejected baseline created business tables", brands, err)
	}
}
func TestBrandForeignKeysAndAppendOnly(t *testing.T) {
	p := isolated(t)
	ctx := context.Background()
	if err := database.Migrate(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := database.Seed(ctx, p, "test"); err != nil {
		t.Fatal(err)
	}
	a := "0199a000-0000-7000-8000-000000000001"
	b := "0199a000-0000-7000-8000-000000000002"
	user, member, account := ids.New(), ids.New(), ids.New()
	if _, err := p.Exec(ctx, "INSERT INTO global_users(id,username) VALUES($1,'test_member')", user); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'domain','1','1')`, member, a, user); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, "INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)", account, b, member); err == nil {
		t.Fatal("cross-brand account accepted")
	}
	if _, err := p.Exec(ctx, "INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)", account, a, member); err != nil {
		t.Fatal(err)
	}
	otherUser := ids.New()
	if _, err := p.Exec(ctx, "INSERT INTO global_users(id,username) VALUES($1,'other_member')", otherUser); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `INSERT INTO sessions(id,token_hash,user_id,member_id,brand_id,expires_at) VALUES($1,'wrong-user',$2,$3,$4,now()+interval '1 hour')`, ids.New(), otherUser, member, a); err == nil {
		t.Fatal("session using another member's global identity accepted")
	}
	if _, err := p.Exec(ctx, `INSERT INTO sessions(id,token_hash,user_id,member_id,brand_id,expires_at) VALUES($1,'matching-user',$2,$3,$4,now()+interval '1 hour')`, ids.New(), user, member, a); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `INSERT INTO point_buckets(brand_id,account_id,source,state,points) VALUES($1,$2,'recharge','available',-1)`, a, account); err == nil {
		t.Fatal("negative bucket accepted")
	}
	logID := ids.New()
	if _, err := p.Exec(ctx, `INSERT INTO audit_logs(id,brand_id,actor_type,action,resource_type,request_id) VALUES($1,$2,'system','seed','test','req-test')`, logID, a); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, "DELETE FROM audit_logs WHERE id=$1", logID); err == nil {
		t.Fatal("audit deleted")
	}
	if _, err := p.Exec(ctx, `INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT $1,$2,s,t FROM unnest(ARRAY['recharge','winning','gift']) s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal']) t`, a, account); err != nil {
		t.Fatal(err)
	}
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var delta points.Balance
	delta[0][0] = 1
	entry, err := (points.Store{DB: p}).Post(ctx, tx, points.Change{BrandID: a, MemberID: member, EntryType: "adjustment", ReferenceType: "test", OperationKey: "op-1", Reason: "test valid append-only ledger", ActorType: "system", RequestID: "req", Delta: delta, Allocation: []points.Allocation{{Source: "recharge", State: "available", Points: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, "UPDATE point_ledger_entries SET reason='tamper' WHERE id=$1", entry.ID); err == nil {
		t.Fatal("ledger changed")
	}
}
