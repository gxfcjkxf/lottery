package database_test

import (
	"context"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

func TestCommissionReviewPreparationStopsAt74AndPreservesBusinessState(t *testing.T) {
	db := testdb.NewAtVersion(t, 73)
	ctx := context.Background()
	before := migrationStatusSnapshot(t, ctx, db)
	if err := database.PrepareCommissionHistoryReview(ctx, db); err != nil {
		t.Fatal(err)
	}
	after := migrationStatusSnapshot(t, ctx, db)
	before.Metadata = after.Metadata
	if before != after {
		t.Fatal("review preparation changed accounts, balances, ledger or audit")
	}
	var last string
	if err := db.QueryRow(ctx, `SELECT max(name) FROM schema_migrations`).Scan(&last); err != nil || last != "0074_commission_analysis_evidence.up.sql" {
		t.Fatalf("checkpoint=%s err=%v", last, err)
	}
	if err := database.CheckMigrations(ctx, db); err == nil {
		t.Fatal("partial review checkpoint was admitted as a serving schema")
	}
	if err := database.PrepareCommissionHistoryReview(ctx, db); err != nil {
		t.Fatal(err)
	}
	if again := migrationStatusSnapshot(t, ctx, db); after != again {
		t.Fatal("repeat preparation changed the checkpoint or business facts")
	}
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := database.CheckMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := database.PrepareCommissionHistoryReview(ctx, db); err == nil {
		t.Fatal("review preparation accepted a later current schema")
	}
}

func TestCommissionReviewPreparationRejectsUnknownPrefixWithoutWrites(t *testing.T) {
	db := testdb.NewAtVersion(t, 73)
	ctx := context.Background()
	if _, err := db.Exec(ctx, `UPDATE schema_migrations SET checksum='changed' WHERE name='0073_commission_manual_recalculation_policy.up.sql'`); err != nil {
		t.Fatal(err)
	}
	before := migrationStatusSnapshot(t, ctx, db)
	catalog := schemaCatalogSnapshot(t, ctx, db)
	if err := database.PrepareCommissionHistoryReview(ctx, db); err == nil {
		t.Fatal("damaged history was accepted")
	}
	if before != migrationStatusSnapshot(t, ctx, db) || catalog != schemaCatalogSnapshot(t, ctx, db) {
		t.Fatal("rejected preparation changed metadata, DDL or business state")
	}
	if err := database.PrepareCommissionHistoryReview(ctx, nil); err == nil {
		t.Fatal("nil database accepted")
	}
}

func TestCommissionReviewPreparationDDLFailureRollsBackEntireCheckpoint(t *testing.T) {
	db := testdb.NewAtVersion(t, 73)
	ctx := context.Background()
	// Force the third index statement to fail after the first two DDL writes.
	// This obstruction is confined to an owned random schema, not public.
	if _, err := db.Exec(ctx, `CREATE INDEX commission_analysis_ledger_binding ON point_ledger_entries(id)`); err != nil {
		t.Fatal(err)
	}
	before := migrationStatusSnapshot(t, ctx, db)
	var indexes string
	if err := db.QueryRow(ctx, `SELECT coalesce(string_agg(indexname,',' ORDER BY indexname),'') FROM pg_indexes WHERE schemaname=current_schema()`).Scan(&indexes); err != nil {
		t.Fatal(err)
	}
	if err := database.PrepareCommissionHistoryReview(ctx, db); err == nil {
		t.Fatal("obstructed DDL preparation succeeded")
	}
	var afterIndexes string
	if err := db.QueryRow(ctx, `SELECT coalesce(string_agg(indexname,',' ORDER BY indexname),'') FROM pg_indexes WHERE schemaname=current_schema()`).Scan(&afterIndexes); err != nil {
		t.Fatal(err)
	}
	if before != migrationStatusSnapshot(t, ctx, db) || indexes != afterIndexes {
		t.Fatal("failed preparation left partial indexes, metadata or financial changes")
	}
}
