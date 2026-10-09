// Package testdb provides isolated PostgreSQL schemas; never point tests at production.
package testdb

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"strings"
	"testing"
)

func New(t *testing.T) *pgxpool.Pool {
	return newDatabase(t, true)
}

// NewUnseeded creates an owned, fully migrated schema without development
// brands. First-brand tests must start empty, not truncate immutable history.
func NewUnseeded(t *testing.T) *pgxpool.Pool {
	return newDatabase(t, false)
}

func newDatabase(t *testing.T, seed bool) *pgxpool.Pool {
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
	err = database.Migrate(ctx, p)
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
