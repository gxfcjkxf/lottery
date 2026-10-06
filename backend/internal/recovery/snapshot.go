package recovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TableDigest struct {
	Rows   int64  `json:"rows"`
	SHA256 string `json:"sha256"`
}

type Snapshot struct {
	Tables         map[string]TableDigest `json:"tables"`
	RowCount       int64                  `json:"row_count"`
	MigrationCount int64                  `json:"migration_count"`
	SHA256         string                 `json:"sha256"`
}

// SnapshotSchema compares complete row content, not counts alone. It is intended
// for bounded, synthetic recovery drills, not unbounded production table scans.
// The returned artifact contains digests only, never credential or member rows.
func SnapshotSchema(ctx context.Context, db *pgxpool.Pool, schema string) (Snapshot, error) {
	out := Snapshot{Tables: map[string]TableDigest{}}
	tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname=$1 ORDER BY tablename`, schema)
	if err != nil {
		return out, err
	}
	var tables []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			rows.Close()
			return out, err
		}
		tables = append(tables, name)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(tables) == 0 {
		return out, fmt.Errorf("recovery snapshot has no tables")
	}
	for _, table := range tables {
		var n int64
		var raw string
		query := `SELECT count(*),coalesce(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text),'[]'::jsonb)::text FROM ` + pgx.Identifier{schema, table}.Sanitize() + ` r`
		if err = tx.QueryRow(ctx, query).Scan(&n, &raw); err != nil {
			return out, err
		}
		hash := sha256.Sum256([]byte(raw))
		out.Tables[table] = TableDigest{Rows: n, SHA256: hex.EncodeToString(hash[:])}
		out.RowCount += n
		if table == "schema_migrations" {
			out.MigrationCount = n
		}
	}
	raw, err := json.Marshal(out.Tables)
	if err != nil {
		return out, err
	}
	hash := sha256.Sum256(raw)
	out.SHA256 = hex.EncodeToString(hash[:])
	return out, tx.Commit(ctx)
}
