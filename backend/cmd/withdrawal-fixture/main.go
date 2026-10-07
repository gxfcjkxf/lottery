//go:build browserfixture

// Creates only explicitly owned synthetic data; never part of production builds.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/authcrypto"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/withdrawal"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func safeFixtureURL(raw, environment, confirmation string) error {
	u, err := url.Parse(raw)
	if err != nil || environment != "test" || confirmation != "owned_synthetic_database" || u.Scheme != "postgres" && u.Scheme != "postgresql" || u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" || u.Port() != "55432" && u.Port() != "5432" || u.Path != "/lottery_withdrawal_ui_s8" && u.Path != "/lottery_withdrawal_ui_s8_followup" && u.Path != "/lottery_withdrawal_ui_s8_verified" || u.Fragment != "" || u.User == nil || u.User.Username() != "lottery_test" {
		return errors.New("explicit owned synthetic withdrawal database required")
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(q) != 1 || len(q["sslmode"]) != 1 || q.Get("sslmode") != "disable" {
		return errors.New("fixture connection overrides forbidden")
	}
	return nil
}

type checker struct{}

func (checker) Check(context.Context, pgx.Tx, withdrawal.EligibilityInput) (withdrawal.EligibilityDecision, error) {
	return withdrawal.EligibilityDecision{Allowed: true, Evidence: json.RawMessage(`{"test_adapter":true,"qualification_algorithm":"synthetic fixture only"}`)}, nil
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "withdrawal fixture failed:", err.Error())
		os.Exit(1)
	}
}
func run() error {
	dsn := os.Getenv("DATABASE_URL")
	if err := safeFixtureURL(dsn, os.Getenv("APP_ENV"), os.Getenv("WITHDRAWAL_FIXTURE_CONFIRM")); err != nil {
		return err
	}
	adminPassword, userPassword := os.Getenv("WITHDRAWAL_FIXTURE_ADMIN_PASSWORD"), os.Getenv("WITHDRAWAL_FIXTURE_USER_PASSWORD")
	if len(adminPassword) < 16 || len(userPassword) < 16 {
		return errors.New("synthetic fixture passwords required")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return errors.New("fixture database unavailable")
	}
	defer db.Close()
	if err = database.Migrate(ctx, db); err != nil {
		return err
	}
	if err = database.Seed(ctx, db, "test"); err != nil {
		return err
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(80401001)`); err != nil {
		return err
	}
	var existing int
	if err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM global_users)+(SELECT count(*) FROM admin_accounts)+(SELECT count(*) FROM point_accounts)`).Scan(&existing); err != nil {
		return err
	}
	if existing != 0 {
		return errors.New("fixture database identities and wallets must be empty")
	}
	adminHash, err := authcrypto.HashPassword(adminPassword)
	if err != nil {
		return errors.New("fixture administrator hash failed")
	}
	userHash, err := authcrypto.HashPassword(userPassword)
	if err != nil {
		return errors.New("fixture user hash failed")
	}
	admin := ids.New()
	if _, err = tx.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,'withdraw_admin',$2)`, admin, adminHash); err != nil {
		return err
	}
	// Base directory grants are bootstrap-defined, not all migration-registered.
	// Register them explicitly only for this owned synthetic operator fixture.
	for _, key := range []string{"brand.view.brand", "user.view.brand", "user.create.brand", "user.write.brand", "user.kick.brand", "user.password_reset.brand", "audit.view.brand", "role.view.brand", "role.write.brand"} {
		if _, err = tx.Exec(ctx, `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, key); err != nil {
			return err
		}
	}
	for _, brand := range []string{"0199a000-0000-7000-8000-000000000001", "0199a000-0000-7000-8000-000000000002"} {
		role := ids.New()
		if _, err = tx.Exec(ctx, `INSERT INTO roles(id,brand_id,code,name) VALUES($1,$2,'withdrawal_fixture','Synthetic withdrawal operator')`, role, brand); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) SELECT $1,key FROM permissions WHERE key LIKE '%.brand'`, role); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO admin_brand_scopes(account_id,brand_id) VALUES($1,$2)`, admin, brand); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, admin, role); err != nil {
			return err
		}
	}
	actor, err := (adminsys.Store{DB: db}).LockAdminAccess(ctx, tx, admin, false)
	if err != nil {
		return err
	}
	const brand = "0199a000-0000-7000-8000-000000000001"
	policy := withdrawal.DefaultBrandConfig()
	policy.Enabled = true
	if _, err = (withdrawal.Service{DB: db}).UpdateBrand(ctx, tx, brand, actor, withdrawal.BrandInput{Version: 1, Config: policy, Reason: "synthetic UI fixture; eligibility adapter not enabled in API"}, points.Metadata{ActorType: "admin", ActorID: admin, RequestID: ids.New()}); err != nil {
		return err
	}
	type result struct{ Username, MemberID, OrderID string }
	out := []result{}
	for _, username := range []string{"withdraw_user", "withdraw_paid_user"} {
		user, member, account := ids.New(), ids.New(), ids.New()
		queries := []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO global_users(id,username,password_hash) VALUES($1,$2,$3)`, []any{user, username, userHash}},
			{`INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version,terms_accepted) VALUES($1,$2,$3,'domain','dev-1','dev-1',true)`, []any{member, brand, user}},
			{`INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, []any{account, brand, member}},
			{`INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT $1,$2,s,t FROM unnest(ARRAY['recharge','winning','gift','commission'])s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal'])t ON CONFLICT DO NOTHING`, []any{brand, account}},
		}
		for _, q := range queries {
			if _, err = tx.Exec(ctx, q.sql, q.args...); err != nil {
				return err
			}
		}
		funding := []points.Allocation{{Source: "recharge", State: "available", Points: 100}, {Source: "winning", State: "available", Points: 60}, {Source: "gift", State: "available", Points: 40}}
		delta, e := points.AllocationDelta(funding, "", "available")
		if e != nil {
			return e
		}
		store := points.Store{DB: db}
		if _, err = store.Post(ctx, tx, points.Change{BrandID: brand, MemberID: member, EntryType: "adjust", ReferenceType: "test_fixture", OperationKey: "withdrawal-ui-fund:" + member, Reason: "owned synthetic browser funding", ActorType: "system", RequestID: ids.New(), Delta: delta, Allocation: funding}); err != nil {
			return err
		}
		allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 40}, {Source: "winning", State: "available", Points: 25}, {Source: "gift", State: "available", Points: 15}}
		order, e := (withdrawal.OrderService{DB: db, Points: store, Eligibility: checker{}}).Create(ctx, tx, brand, member, withdrawal.OrderInput{Points: 80, SourceAllocation: allocation, ClientKey: "synthetic-initial-request"}, points.Metadata{ActorType: "user", ActorID: user, RequestID: ids.New()})
		if e != nil {
			return e
		}
		out = append(out, result{username, member, order.ID})
	}
	// Caller creates new independent browser-owned intents through normal APIs.
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(out)
}
