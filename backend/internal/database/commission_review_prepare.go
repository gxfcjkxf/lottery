package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/gxfcjkxf/lottery/backend/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PrepareCommissionHistoryReview is an explicit OFFLINE checkpoint, not a
// relaxed application migration. Only a genuine 0073 history can advance to
// the existing read-only 0074 evidence migration. Normal CheckMigrations still
// refuses API/worker admission until the complete migration set is installed.
// In particular, this does not bypass 0075's refusal of stale actual money.
func PrepareCommissionHistoryReview(ctx context.Context, db *pgxpool.Pool) error {
	if ctx == nil || db == nil {
		return errors.New("commission review preparation requires context and database")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SET LOCAL lock_timeout='5s'`); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(79001001)`); err != nil {
		return err
	}
	checkpoint, err := CheckCommissionReviewMigrationsTx(ctx, tx)
	if err != nil {
		return err
	}
	if checkpoint == commissionReviewMigration74 {
		return tx.Commit(ctx)
	}
	var schema string
	if err = tx.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		return err
	}
	// Explicit pg_temp-last prevents temp relations from shadowing the real
	// application's history or the migration's index targets. Keep app first so
	// the unchanged migration creates objects in the application, not pg_catalog.
	if _, err = tx.Exec(ctx, `SET LOCAL search_path TO `+pgx.Identifier{schema}.Sanitize()+`, pg_catalog, pg_temp`); err != nil {
		return err
	}
	body, err := migrations.Files.ReadFile(commissionReviewMigration74)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, string(body)); err != nil {
		return err
	}
	hash := sha256.Sum256(body)
	metadata := pgx.Identifier{schema, "schema_migrations"}.Sanitize()
	if _, err = tx.Exec(ctx, `INSERT INTO `+metadata+`(name,checksum) VALUES($1,$2)`, commissionReviewMigration74, hex.EncodeToString(hash[:])); err != nil {
		return err
	}
	if _, err = CheckCommissionReviewMigrationsTx(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
