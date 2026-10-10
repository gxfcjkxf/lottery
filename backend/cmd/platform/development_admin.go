package main

import (
	"context"
	"errors"

	"github.com/gxfcjkxf/lottery/backend/internal/authcrypto"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The known trial credential is never installed in test or production.
func seedDevelopmentAdmin(ctx context.Context, db *pgxpool.Pool, environment string) error {
	if environment != "development" {
		return nil
	}
	hash, err := authcrypto.HashDevelopmentAdminPassword()
	if err != nil {
		return err
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var admin string
	err = tx.QueryRow(ctx, `INSERT INTO admin_accounts(id,username,password_hash,is_super_admin)
		VALUES($1,'admin',$2,true) ON CONFLICT(username) DO NOTHING RETURNING id::text`, ids.New(), hash).Scan(&admin)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // Existing credentials and grants belong to their owner.
	}
	if err != nil {
		return err
	}
	if err := grantBootstrapAdmin(ctx, tx, admin, "", true, "development seed bootstrap"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
