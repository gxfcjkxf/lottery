package notification

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/events"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Build the genuine previous schema, seed a legacy notification under its real
// constraints, then upgrade through the normal migration runner. Never disable
// an immutability trigger or rewrite an old row to manufacture compatibility.
func notificationSchemaBefore(t *testing.T, stopBefore string) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	admin, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	schema := "test_template_upgrade_" + strings.ReplaceAll(ids.New(), "-", "")
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, e = admin.Exec(ctx, "CREATE SCHEMA "+quoted); e != nil {
		t.Fatal(e)
	}
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		t.Fatal(e)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		db.Close()
		_, e := admin.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE")
		admin.Close()
		if e != nil {
			t.Error(e)
		}
	})
	tx, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(79001001);CREATE TABLE schema_migrations(name text PRIMARY KEY,checksum text NOT NULL,applied_at timestamptz NOT NULL DEFAULT now())`); e != nil {
		t.Fatal(e)
	}
	names, e := fs.Glob(migrations.Files, "*.up.sql")
	if e != nil {
		t.Fatal(e)
	}
	sort.Strings(names)
	for _, name := range names {
		if name >= stopBefore {
			break
		}
		body, e := migrations.Files.ReadFile(name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = tx.Exec(ctx, string(body)); e != nil {
			t.Fatal(name, e)
		}
		hash := sha256.Sum256(body)
		if _, e = tx.Exec(ctx, `INSERT INTO schema_migrations(name,checksum)VALUES($1,$2)`, name, hex.EncodeToString(hash[:])); e != nil {
			t.Fatal(e)
		}
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if e = database.Seed(ctx, db, "test"); e != nil {
		t.Fatal(e)
	}
	return db
}

func TestTemplateMigrationPreservesLegacyNotificationAndSnapshotsNewMessages(t *testing.T) {
	db := notificationSchemaBefore(t, "0037_")
	ctx := context.Background()
	user, member, event, notificationID := ids.New(), ids.New(), ids.New(), ids.New()
	if _, e := db.Exec(ctx, `INSERT INTO global_users(id,username)VALUES($1,$2)`, user, "template_upgrade_"+user); e != nil {
		t.Fatal(e)
	}
	if _, e := db.Exec(ctx, `INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version)VALUES($1,$2,$3,'domain','dev-1','dev-1')`, member, brand, user); e != nil {
		t.Fatal(e)
	}
	if _, e := db.Exec(ctx, `INSERT INTO outbox_events(id,brand_id,event_type,aggregate_id,payload) VALUES($1,$2,'member.joined',$3::uuid,jsonb_build_object('member_id',$3::uuid::text,'resource_id',$3::uuid::text,'points',NULL))`, event, brand, member); e != nil {
		t.Fatal(e)
	}
	if _, e := db.Exec(ctx, `INSERT INTO notifications(id,brand_id,member_id,event_id,event_type,template_key,template_version,payload)VALUES($1,$2,$3::uuid,$4,'member.joined','member.joined',1,jsonb_build_object('resource_id',$3::uuid::text,'points',NULL))`, notificationID, brand, member, event); e != nil {
		t.Fatal(e)
	}
	var before, after string
	var e error
	if e = db.QueryRow(ctx, `SELECT to_jsonb(n)::text FROM notifications n WHERE id=$1`, notificationID).Scan(&before); e != nil {
		t.Fatal(e)
	}
	if e = database.Migrate(ctx, db); e != nil {
		t.Fatal(e)
	}
	if e = database.Migrate(ctx, db); e != nil {
		t.Fatal("repeat migration", e)
	}
	if e = db.QueryRow(ctx, `SELECT (to_jsonb(n)-'content')::text FROM notifications n WHERE id=$1`, notificationID).Scan(&after); e != nil || before != after {
		t.Fatal("legacy fields changed", e, before, after)
	}
	s := Service{DB: db}
	page, e := s.List(ctx, brand, member, 20, 0)
	if e != nil || len(page.Items) != 1 || page.Items[0].Content != nil || page.Items[0].TemplateVersion != 1 {
		t.Fatal("legacy renderer lost", page, e)
	}
	// The queue can re-observe the old event without replacing its legacy content.
	if _, e = s.Process(ctx, 20); e != nil {
		t.Fatal(e)
	}
	write, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = events.Append(ctx, write, brand, "member.joined", member, member, nil); e != nil {
		t.Fatal(e)
	}
	if e = write.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Process(ctx, 20); e != nil {
		t.Fatal(e)
	}
	page, e = s.List(ctx, brand, member, 20, 0)
	if e != nil || len(page.Items) != 2 {
		t.Fatal(page, e)
	}
	if page.Items[0].Content == nil || page.Items[1].Content != nil || page.Items[1].ID != notificationID {
		t.Fatal("new snapshot or old legacy content incorrect", page)
	}
}
