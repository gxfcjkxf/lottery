package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"

	"github.com/gxfcjkxf/lottery/backend/migrations"
	"github.com/jackc/pgx/v5"
)

const (
	commissionReviewMigration73 = "0073_commission_manual_recalculation_policy.up.sql"
	commissionReviewMigration74 = "0074_commission_analysis_evidence.up.sql"
)

type commissionReviewMigration struct {
	name     string
	checksum string
}

// CheckCommissionReviewMigrationsTx admits commission review only when the
// supplied transaction sees the exact embedded migration history through
// migration 73 or 74. It performs no writes and leaves transaction ownership
// with the caller.
func CheckCommissionReviewMigrationsTx(ctx context.Context, tx pgx.Tx) (string, error) {
	if ctx == nil {
		return "", fmt.Errorf("commission review migration check requires a context")
	}
	if tx == nil {
		return "", fmt.Errorf("commission review migration check requires a transaction")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	migrations73, migrations74, err := commissionReviewMigrationPrefixes()
	if err != nil {
		return "", err
	}

	var schema string
	var isTemporarySchema bool
	var hasTemporaryMetadata bool
	if err := tx.QueryRow(ctx, `
		SELECT n.nspname,
		       n.oid = pg_catalog.pg_my_temp_schema(),
	       pg_catalog.to_regclass('pg_temp.schema_migrations') IS NOT NULL
		FROM pg_catalog.pg_namespace n
		WHERE n.nspname = current_schema()
	`).Scan(&schema, &isTemporarySchema, &hasTemporaryMetadata); err != nil {
		return "", fmt.Errorf("read commission review schema: %w", err)
	}
	if schema == "" || isTemporarySchema || hasTemporaryMetadata {
		return "", fmt.Errorf("commission review migration metadata must be in the current application schema")
	}

	metadata := pgx.Identifier{schema, "schema_migrations"}.Sanitize()
	rows, err := tx.Query(ctx, `SELECT name, checksum FROM `+metadata+` ORDER BY name`)
	if err != nil {
		return "", fmt.Errorf("read commission review migration metadata: %w", err)
	}
	defer rows.Close()
	actual := make([]commissionReviewMigration, 0)
	for rows.Next() {
		var entry commissionReviewMigration
		if err := rows.Scan(&entry.name, &entry.checksum); err != nil {
			return "", err
		}
		actual = append(actual, entry)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	for _, candidate := range []struct {
		name    string
		entries []commissionReviewMigration
	}{{commissionReviewMigration73, migrations73}, {commissionReviewMigration74, migrations74}} {
		if commissionReviewMigrationRowsMatch(actual, candidate.entries) {
			return candidate.name, nil
		}
	}
	return "", fmt.Errorf("commission review requires the exact embedded migration history through 0073 or 0074")
}

func commissionReviewMigrationRowsMatch(actual, expected []commissionReviewMigration) bool {
	if len(actual) != len(expected) {
		return false
	}
	for i := range expected {
		if actual[i] != expected[i] {
			return false
		}
	}
	return true
}

func commissionReviewMigrationPrefixes() ([]commissionReviewMigration, []commissionReviewMigration, error) {
	names, err := fs.Glob(migrations.Files, "*.up.sql")
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(names)
	var through73, through74 []commissionReviewMigration
	found73, found74 := false, false
	for _, name := range names {
		body, err := migrations.Files.ReadFile(name)
		if err != nil {
			return nil, nil, err
		}
		sum := sha256.Sum256(body)
		entry := commissionReviewMigration{name: name, checksum: hex.EncodeToString(sum[:])}
		if !found74 {
			through74 = append(through74, entry)
			if name == commissionReviewMigration74 {
				found74 = true
			}
		}
		if !found73 {
			through73 = append(through73, entry)
			if name == commissionReviewMigration73 {
				found73 = true
			}
		}
	}
	if !found73 || !found74 {
		return nil, nil, fmt.Errorf("embedded commission review migration prefix is incomplete")
	}
	return through73, through74, nil
}
