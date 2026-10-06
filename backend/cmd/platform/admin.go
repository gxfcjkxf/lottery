package main

import (
	"context"
	"errors"
	"flag"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/authcrypto"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"regexp"
	"strings"
)

// Bootstrap credentials come from the environment, not argv/history or defaults.
func createAdmin(ctx context.Context, db *pgxpool.Pool) error {
	flags := flag.NewFlagSet("create-admin", flag.ContinueOnError)
	username := flags.String("username", "", "new administrative username")
	brand := flags.String("brand", "", "brand code for brand operator")
	super := flags.Bool("super", false, "platform read-only user administration")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	*username = strings.ToLower(strings.TrimSpace(*username))
	if !regexp.MustCompile(`^[a-z][a-z0-9_]{2,31}$`).MatchString(*username) {
		return errors.New("invalid admin username")
	}
	password := os.Getenv("BOOTSTRAP_ADMIN_PASSWORD")
	if len(password) < 16 || len(password) > 128 {
		return errors.New("BOOTSTRAP_ADMIN_PASSWORD must contain 16-128 bytes")
	}
	hash, err := authcrypto.HashPassword(password)
	if err != nil {
		return err
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var brandID string
	if !*super {
		if *brand == "" {
			return errors.New("--brand required for a brand operator")
		}
		if err = tx.QueryRow(ctx, "SELECT id::text FROM brands WHERE code=$1", *brand).Scan(&brandID); err != nil {
			return errors.New("brand not found")
		}
	}
	admin, role := ids.New(), ids.New()
	if _, err = tx.Exec(ctx, "INSERT INTO admin_accounts(id,username,password_hash,is_super_admin) VALUES($1,$2,$3,$4)", admin, *username, hash, *super); err != nil {
		return errors.New("cannot create admin; account may already exist")
	}
	if _, err = tx.Exec(ctx, "INSERT INTO roles(id,brand_id,code,name,is_bootstrap) VALUES($1,NULLIF($2,'')::uuid,$3,$4,true)", role, brandID, "bootstrap_"+role, "Bootstrap administrator"); err != nil {
		return err
	}
	permissions := []string{"user.view.brand", "user.write.brand", "user.kick.brand", "user.password_reset.brand", "audit.view.brand", "brand.view.brand", "user.create.brand", "role.view.brand", "role.write.brand", "admin.view.brand", "admin.write.brand", "auth_config.view.brand", "auth_config.write.brand", "wallet.view.brand", "wallet.freeze.brand", "wallet.adjust.brand", "recharge.view.brand", "recharge.write.brand", "point_policy.view.brand", "point_policy.write.brand", "wallet.repair.brand"}
	if *super {
		permissions = []string{"user.view.platform", "audit.view.platform", "brand.view.platform", "role.view.platform", "role.write.platform", "admin.view.platform", "admin.write.platform", "auth_config.view.platform", "auth_config.write.platform", "wallet.view.platform", "recharge.view.platform", "point_policy.view.platform"}
	}
	if *super {
		permissions = append(permissions, "rule.simulate.platform")
		permissions = append(permissions, "game.view.platform", "rule.view.platform")
		permissions = append(permissions, "schedule.view.platform", "period.view.platform")
		permissions = append(permissions, "draw_source.view.platform", "draw.view.platform")
		permissions = append(permissions, "bet.view.platform", "bet_policy.view.platform")
		permissions = append(permissions, "withdrawal_policy.view.platform")
		permissions = append(permissions, "notification.view.platform")
		permissions = append(permissions, "report_betting.view.platform", "report_ledger.view.platform")
		permissions = append(permissions, "settlement.view.platform")
		permissions = append(permissions, "settlement_policy.view.platform")
	} else {
		permissions = append(permissions, "rule.simulate.brand")
		permissions = append(permissions, "game.view.brand", "game.write.brand", "rule.view.brand", "rule.write.brand", "rule.validate.brand", "rule.submit.brand", "rule.review.brand")
		permissions = append(permissions, "schedule.view.brand", "schedule.write.brand", "period.view.brand", "period.generate.brand", "period.cancel.brand", "period.cancel_retry.brand")
		permissions = append(permissions, "draw_source.view.brand", "draw_source.write.brand", "draw.view.brand", "draw.manual_create.brand")
		permissions = append(permissions, "bet.view.brand", "bet.cancel.brand", "bet_policy.view.brand", "bet_policy.write.brand")
		permissions = append(permissions, "bet.mark_abnormal.brand")
		permissions = append(permissions, "bet.judge_cancel.brand")
		permissions = append(permissions, "withdrawal_policy.view.brand", "withdrawal_policy.write.brand")
		permissions = append(permissions, "notification.view.brand", "notification.retry.brand")
		permissions = append(permissions, "report_betting.view.brand", "report_ledger.view.brand")
		permissions = append(permissions, "settlement.view.brand", "settlement.preview.brand")
		permissions = append(permissions, "settlement.run.brand", "settlement.approve.brand", "settlement.retry.brand", "settlement_policy.view.brand", "settlement_policy.write.brand")
		permissions = append(permissions, "draw.correct.brand", "draw.correction_retry.brand")
	}
	for _, permission := range permissions {
		if _, err = tx.Exec(ctx, "INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING", permission); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2)", role, permission); err != nil {
			return err
		}
	}
	if brandID != "" {
		if _, err = tx.Exec(ctx, "INSERT INTO admin_brand_scopes(account_id,brand_id) VALUES($1,$2)", admin, brandID); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, "INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)", admin, role); err != nil {
		return err
	}
	if _, err = audit.Append(ctx, tx, audit.Record{BrandID: brandID, ActorType: "system", Action: "admin.bootstrap", ResourceType: "admin", ResourceID: admin, Reason: "explicit server-owner bootstrap", RequestID: ids.New(), After: map[string]any{"super_admin": *super}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
