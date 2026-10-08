// Package testdb provides isolated PostgreSQL schemas; never point tests at production.
package testdb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"io/fs"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func New(t *testing.T) *pgxpool.Pool {
	return newDatabase(t, 0, true)
}

// NewUnseeded creates an owned, fully migrated schema without development
// brands. First-brand tests must start empty, not truncate immutable history.
func NewUnseeded(t *testing.T) *pgxpool.Pool {
	return newDatabase(t, 0, false)
}

// NewAtVersion creates an owned random schema using the actual historical
// migration files. It is only for upgrade verification, never a downgrade.
func NewAtVersion(t *testing.T, version int) *pgxpool.Pool {
	t.Helper()
	if version < 1 {
		t.Fatal("historical migration version must be positive")
	}
	return newDatabase(t, version, true)
}

func newDatabase(t *testing.T, version int, seed bool) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required for real PostgreSQL tests")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "test_" + strings.ReplaceAll(ids.New(), "-", "")
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	p, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		p.Close()
		_, err := admin.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE")
		admin.Close()
		if err != nil {
			t.Error(err)
		}
	})
	if version == 0 {
		err = database.Migrate(ctx, p)
	} else {
		err = migrateHistorical(ctx, p, version)
	}
	if err != nil {
		t.Fatal(err)
	}
	if seed {
		if err = database.Seed(ctx, p, "test"); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func migrateHistorical(ctx context.Context, p *pgxpool.Pool, version int) error {
	tx, err := p.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `CREATE TABLE schema_migrations(name text PRIMARY KEY,checksum text NOT NULL,applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	names, err := fs.Glob(migrations.Files, "*.up.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	last := 0
	for _, name := range names {
		v, e := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		if e != nil {
			return e
		}
		if v > version {
			break
		}
		body, e := migrations.Files.ReadFile(name)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, string(body)); e != nil {
			return fmt.Errorf("historical %s: %w", name, e)
		}
		hash := sha256.Sum256(body)
		if _, e = tx.Exec(ctx, `INSERT INTO schema_migrations(name,checksum) VALUES($1,$2)`, name, hex.EncodeToString(hash[:])); e != nil {
			return e
		}
		last = v
	}
	if last != version {
		return fmt.Errorf("historical migration %d unavailable", version)
	}
	return tx.Commit(ctx)
}
