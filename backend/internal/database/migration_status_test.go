package database_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"reflect"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/gxfcjkxf/lottery/backend/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCheckMigrationsCurrentSchemaIsReadOnly(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	before := migrationStatusSnapshot(t, ctx, db)

	if err := database.CheckMigrations(ctx, db); err != nil {
		t.Fatalf("CheckMigrations(current schema): %v", err)
	}

	after := migrationStatusSnapshot(t, ctx, db)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("CheckMigrations changed database state:\nbefore: %+v\nafter:  %+v", before, after)
	}
}

func TestCheckMigrationsRejectsHistoricalSchemaWithoutMigrating(t *testing.T) {
	db := testdb.NewAtVersion(t, 53)
	ctx := context.Background()
	before := migrationStatusSnapshot(t, ctx, db)
	schemaBefore := schemaCatalogSnapshot(t, ctx, db)

	if err := database.CheckMigrations(ctx, db); err == nil {
		t.Fatal("CheckMigrations accepted schema at migration 53; want incomplete-set error")
	}

	after := migrationStatusSnapshot(t, ctx, db)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("rejected historical schema metadata changed:\nbefore: %+v\nafter:  %+v", before, after)
	}
	if schemaAfter := schemaCatalogSnapshot(t, ctx, db); schemaAfter != schemaBefore {
		t.Fatalf("rejected historical schema was migrated:\nbefore: %s\nafter:  %s", schemaBefore, schemaAfter)
	}
}

func TestCheckMigrationsRejectsInconsistentMetadataWithoutChangingIt(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*testing.T, context.Context, *pgxpool.Pool)
	}{
		{
			name: "missing historical metadata",
			mutate: func(t *testing.T, ctx context.Context, db *pgxpool.Pool) {
				t.Helper()
				var name string
				if err := db.QueryRow(ctx, `SELECT name FROM schema_migrations ORDER BY name LIMIT 1`).Scan(&name); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(ctx, `DELETE FROM schema_migrations WHERE name=$1`, name); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "extra future metadata",
			mutate: func(t *testing.T, ctx context.Context, db *pgxpool.Pool) {
				t.Helper()
				if _, err := db.Exec(ctx, `INSERT INTO schema_migrations(name,checksum) VALUES('9999_future.up.sql','future')`); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "incorrect checksum",
			mutate: func(t *testing.T, ctx context.Context, db *pgxpool.Pool) {
				t.Helper()
				var name string
				if err := db.QueryRow(ctx, `SELECT name FROM schema_migrations ORDER BY name LIMIT 1`).Scan(&name); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(ctx, `UPDATE schema_migrations SET checksum='incorrect-checksum' WHERE name=$1`, name); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := testdb.New(t)
			ctx := context.Background()
			tc.mutate(t, ctx, db)
			before := migrationMetadataSnapshot(t, ctx, db)

			if err := database.CheckMigrations(ctx, db); err == nil {
				t.Fatal("CheckMigrations accepted inconsistent metadata")
			}

			if after := migrationMetadataSnapshot(t, ctx, db); after != before {
				t.Fatalf("CheckMigrations changed inconsistent migration metadata:\nbefore: %s\nafter:  %s", before, after)
			}
		})
	}
}

func TestCheckMigrationsRejectsNilPoolAndCanceledContext(t *testing.T) {
	if err := database.CheckMigrations(context.Background(), nil); err == nil {
		t.Fatal("CheckMigrations(nil pool) succeeded; want error")
	}

	db := testdb.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := database.CheckMigrations(ctx, db); err == nil {
		t.Fatal("CheckMigrations(canceled context) succeeded; want error")
	}
}

func TestCheckMigrationsRejectsTemporaryMetadataSpoofing(t *testing.T) {
	db := testdb.NewAtVersion(t, 53)
	ctx := context.Background()
	// Force the check onto the connection that owns the counterfeit temp table.
	config := db.Config()
	config.MaxConns, config.MinConns, config.MinIdleConns = 1, 0, 0
	probe, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()
	before := migrationMetadataSnapshot(t, ctx, db)
	if _, err = probe.Exec(ctx, `CREATE TEMP TABLE schema_migrations(name text PRIMARY KEY,checksum text NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	names, err := fs.Glob(migrations.Files, "*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		body, err := migrations.Files.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(body)
		if _, err = probe.Exec(ctx, `INSERT INTO pg_temp.schema_migrations VALUES($1,$2)`, name, hex.EncodeToString(sum[:])); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.CheckMigrations(ctx, probe); err == nil {
		t.Fatal("counterfeit temporary metadata hid the real historical schema")
	}
	if after := migrationMetadataSnapshot(t, ctx, db); after != before {
		t.Fatal("spoofed status check changed the real migration metadata")
	}
}

type migrationSnapshot struct {
	Metadata string
	Wallet   string
	Buckets  string
	Ledger   string
	Audit    string
}

func migrationStatusSnapshot(t *testing.T, ctx context.Context, db *pgxpool.Pool) migrationSnapshot {
	t.Helper()
	return migrationSnapshot{
		Metadata: migrationMetadataSnapshot(t, ctx, db),
		Wallet:   jsonTableSnapshot(t, ctx, db, "point_accounts"),
		Buckets:  jsonTableSnapshot(t, ctx, db, "point_buckets"),
		Ledger:   jsonTableSnapshot(t, ctx, db, "point_ledger_entries"),
		Audit:    jsonTableSnapshot(t, ctx, db, "audit_logs"),
	}
}

func migrationMetadataSnapshot(t *testing.T, ctx context.Context, db *pgxpool.Pool) string {
	t.Helper()
	return jsonTableSnapshot(t, ctx, db, "schema_migrations")
}

func jsonTableSnapshot(t *testing.T, ctx context.Context, db *pgxpool.Pool, table string) string {
	t.Helper()
	var snapshot string
	query := `SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text)::text, '[]') FROM ` + pgx.Identifier{table}.Sanitize() + ` AS r`
	if err := db.QueryRow(ctx, query).Scan(&snapshot); err != nil {
		t.Fatalf("snapshot %s: %v", table, err)
	}
	return snapshot
}

func schemaCatalogSnapshot(t *testing.T, ctx context.Context, db *pgxpool.Pool) string {
	t.Helper()
	var snapshot string
	err := db.QueryRow(ctx, `
		SELECT COALESCE(jsonb_agg(jsonb_build_object(
			'relation', c.relname,
			'kind', c.relkind,
			'column', a.attname,
			'type', pg_catalog.format_type(a.atttypid, a.atttypmod),
			'not_null', a.attnotnull
		) ORDER BY c.relname, a.attnum)::text, '[]')
		FROM pg_catalog.pg_class c
		JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
		LEFT JOIN pg_catalog.pg_attribute a ON a.attrelid=c.oid AND a.attnum > 0 AND NOT a.attisdropped
		WHERE n.nspname=current_schema() AND c.relkind IN ('r','p','v','m','S')`).Scan(&snapshot)
	if err != nil {
		t.Fatalf("snapshot schema catalog: %v", err)
	}
	return snapshot
}
