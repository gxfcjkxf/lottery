package database_test

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
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
	entry := ids.New()
	if _, err := p.Exec(ctx, `INSERT INTO point_ledger_entries(id,brand_id,account_id,member_id,version,request_hash,actor_type,entry_type,reference_type,operation_key,before_snapshot,delta_snapshot,after_snapshot,reason,request_id) VALUES($1,$2,$3,$4,1,repeat('a',64),'system','recharge','test','op-1','{}','{}','{}','test','req')`, entry, a, account, member); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, "UPDATE point_ledger_entries SET reason='tamper' WHERE id=$1", entry); err == nil {
		t.Fatal("ledger changed")
	}
}
