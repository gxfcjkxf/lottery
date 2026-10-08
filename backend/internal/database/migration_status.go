package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"

	"github.com/gxfcjkxf/lottery/backend/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CheckMigrations verifies the complete embedded migration set and checksums
// against a single read-only database snapshot. It never applies migrations or
// repairs their metadata. Owned test fixtures use it before touching identities
// or funds so adding a migration cannot silently bypass their admission guard.
func CheckMigrations(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return fmt.Errorf("migration status requires a database")
	}
	names, err := fs.Glob(migrations.Files, "*.up.sql")
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return fmt.Errorf("embedded migration set is empty")
	}
	expected := make(map[string]string, len(names))
	for _, name := range names {
		body, err := migrations.Files.ReadFile(name)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(body)
		expected[name] = hex.EncodeToString(sum[:])
	}
	tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var schema string
	if err := tx.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		return err
	}
	// Implicit pg_temp lookup must not hide the configured application's real
	// history behind a counterfeit schema_migrations table on this connection.
	metadata := pgx.Identifier{schema, "schema_migrations"}.Sanitize()
	rows, err := tx.Query(ctx, `SELECT name,checksum FROM `+metadata+` ORDER BY name`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name, checksum string
		if err := rows.Scan(&name, &checksum); err != nil {
			return err
		}
		want, exists := expected[name]
		if !exists || checksum != want {
			return fmt.Errorf("migration status mismatch for %s", name)
		}
		delete(expected, name)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(expected) != 0 {
		return fmt.Errorf("migration status is incomplete")
	}
	return tx.Commit(ctx)
}
