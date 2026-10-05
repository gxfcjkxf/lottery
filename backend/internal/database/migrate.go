package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/gxfcjkxf/lottery/backend/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"io/fs"
	"sort"
	"strings"
)

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Serialize concurrent deployers using a transaction-scoped advisory lock.
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(79001001)"); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	names, err := fs.Glob(migrations.Files, "*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		body, err := migrations.Files.ReadFile(name)
		if err != nil {
			return err
		}
		hash := sha256.Sum256(body)
		checksum := hex.EncodeToString(hash[:])
		var stored string
		err = tx.QueryRow(ctx, "SELECT checksum FROM schema_migrations WHERE name=$1", name).Scan(&stored)
		if err == nil {
			if stored != checksum {
				return fmt.Errorf("migration %s changed after application", name)
			}
			continue
		}
		if err != pgx.ErrNoRows {
			return err
		}
		if _, err = tx.Exec(ctx, string(body)); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err = tx.Exec(ctx, "INSERT INTO schema_migrations(name,checksum) VALUES($1,$2)", name, checksum); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
