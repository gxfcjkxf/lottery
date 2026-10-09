package database_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/gxfcjkxf/lottery/backend/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCommissionReviewMigrationAcceptsExactHistoricalPrefixes(t *testing.T) {
	for _, tc := range []struct {
		version int
		want    string
	}{
		{version: 73, want: "0073_commission_manual_recalculation_policy.up.sql"},
		{version: 74, want: "0074_commission_analysis_evidence.up.sql"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			db := testdb.NewAtVersion(t, tc.version)
			ctx := context.Background()
			before := commissionReviewStateSnapshot(t, ctx, db)

			tx, err := db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
			if err != nil {
				t.Fatal(err)
			}
			got, err := database.CheckCommissionReviewMigrationsTx(ctx, tx)
			_ = tx.Rollback(ctx)
			if err != nil {
				t.Fatalf("CheckCommissionReviewMigrationsTx(): %v", err)
			}
			if got != tc.want {
				t.Fatalf("CheckCommissionReviewMigrationsTx() = %q, want %q", got, tc.want)
			}
			if after := commissionReviewStateSnapshot(t, ctx, db); after != before {
				t.Fatalf("read-only migration check changed metadata or funds:\nbefore: %s\nafter:  %s", before, after)
			}
		})
	}
}

func TestCommissionReviewMigrationRejectsLatest75(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	before := commissionReviewStateSnapshot(t, ctx, db)
	tx, err := db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if got, err := database.CheckCommissionReviewMigrationsTx(ctx, tx); err == nil {
		t.Fatalf("CheckCommissionReviewMigrationsTx() = %q, nil error; migration 75 must be refused", got)
	}
	if after := commissionReviewStateSnapshot(t, ctx, db); after != before {
		t.Fatalf("rejected migration check changed metadata or funds:\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestCommissionReviewMigrationRejectsInvalidHistory(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(context.Context, *pgxpool.Pool) error
	}{
		{
			name: "hole",
			mutate: func(ctx context.Context, db *pgxpool.Pool) error {
				_, err := db.Exec(ctx, `DELETE FROM schema_migrations WHERE name='0001_foundation.up.sql'`)
				return err
			},
		},
		{
			name: "changed checksum",
			mutate: func(ctx context.Context, db *pgxpool.Pool) error {
				_, err := db.Exec(ctx, `UPDATE schema_migrations SET checksum='changed' WHERE name='0073_commission_manual_recalculation_policy.up.sql'`)
				return err
			},
		},
		{
			name: "foreign row",
			mutate: func(ctx context.Context, db *pgxpool.Pool) error {
				_, err := db.Exec(ctx, `INSERT INTO schema_migrations(name,checksum) VALUES('foreign_migration.up.sql','foreign')`)
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := testdb.NewAtVersion(t, 73)
			ctx := context.Background()
			if err := tc.mutate(ctx, db); err != nil {
				t.Fatal(err)
			}
			before := commissionReviewStateSnapshot(t, ctx, db)
			tx, err := db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if got, err := database.CheckCommissionReviewMigrationsTx(ctx, tx); err == nil {
				t.Fatalf("CheckCommissionReviewMigrationsTx() = %q, nil error; want invalid history rejected", got)
			}
			if after := commissionReviewStateSnapshot(t, ctx, db); after != before {
				t.Fatalf("rejected migration check changed metadata or funds:\nbefore: %s\nafter:  %s", before, after)
			}
		})
	}
}

func TestCommissionReviewMigrationRejectsTemporaryMetadataSpoof(t *testing.T) {
	db := testdb.NewAtVersion(t, 73)
	ctx := context.Background()
	config := db.Config()
	config.MaxConns, config.MinConns, config.MinIdleConns = 1, 0, 0
	probe, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()
	if _, err := probe.Exec(ctx, `CREATE TEMP TABLE schema_migrations(name text PRIMARY KEY, checksum text NOT NULL)`); err != nil {
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
		if _, err := probe.Exec(ctx, `INSERT INTO pg_temp.schema_migrations(name,checksum) VALUES($1,$2)`, name, hex.EncodeToString(sum[:])); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := probe.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if got, err := database.CheckCommissionReviewMigrationsTx(ctx, tx); err == nil {
		t.Fatalf("CheckCommissionReviewMigrationsTx() = %q, nil error; want temporary metadata spoof rejected", got)
	}
}

func TestCommissionReviewMigrationRejectsNilTxAndCanceledContext(t *testing.T) {
	if _, err := database.CheckCommissionReviewMigrationsTx(context.Background(), nil); err == nil {
		t.Fatal("CheckCommissionReviewMigrationsTx(nil tx) succeeded; want error")
	}
	db := testdb.NewAtVersion(t, 73)
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := database.CheckCommissionReviewMigrationsTx(ctx, tx); err == nil {
		t.Fatal("CheckCommissionReviewMigrationsTx(canceled context) succeeded; want error")
	}
}

func commissionReviewStateSnapshot(t *testing.T, ctx context.Context, db *pgxpool.Pool) string {
	t.Helper()
	return migrationMetadataSnapshot(t, ctx, db) + "\n" +
		jsonTableSnapshot(t, ctx, db, "point_accounts") + "\n" +
		jsonTableSnapshot(t, ctx, db, "point_ledger_entries")
}
