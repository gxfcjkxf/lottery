//go:build browserfixture

// This opt-in test fixture is excluded from normal builds and production images.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/reconciliation"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func safeFixtureURL(dsn, environment, confirm string) error {
	u, e := url.Parse(dsn)
	if e != nil || environment != "test" || confirm != "owned_synthetic_database" || u.Scheme != "postgres" && u.Scheme != "postgresql" || u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" || u.Path != "/lottery_reconciliation_browser" || u.Fragment != "" {
		return errors.New("explicit owned synthetic reconciliation database required")
	}
	if u.Port() != "5432" && u.Port() != "55549" {
		return errors.New("unexpected fixture loopback port")
	}
	if u.User == nil || u.User.Username() != "lottery_test" && u.User.Username() != "lottery_reconciliation_test" {
		return errors.New("unexpected fixture database user")
	}
	q, e := url.ParseQuery(u.RawQuery)
	if e != nil {
		return errors.New("invalid fixture query")
	}
	if len(q) != 1 || len(q["sslmode"]) != 1 || q.Get("sslmode") != "disable" {
		return errors.New("fixture connection overrides forbidden")
	}
	return nil
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "reconciliation fixture failed:", e.Error())
		os.Exit(1)
	}
}
func run() error {
	dsn := os.Getenv("DATABASE_URL")
	if e := safeFixtureURL(dsn, os.Getenv("APP_ENV"), os.Getenv("RECONCILIATION_FIXTURE_CONFIRM")); e != nil {
		return e
	}
	ctx := context.Background()
	p, e := pgxpool.New(ctx, dsn)
	if e != nil {
		return errors.New("fixture database unavailable")
	}
	defer p.Close()
	if len(os.Args) > 1 {
		if len(os.Args) != 2 || os.Args[1] != "verify" {
			return errors.New("unsupported owned fixture operation")
		}
		tx, err := p.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		if err != nil {
			return errors.New("owned verification unavailable")
		}
		defer tx.Rollback(ctx)
		hash := sha256.New()
		for _, table := range append(reconciliation.InventorySources(), "outbox_events") {
			var raw []byte
			query := `SELECT coalesce(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text),'[]'::jsonb) FROM ` + pgx.Identifier{"public", table}.Sanitize() + ` t`
			if err = tx.QueryRow(ctx, query).Scan(&raw); err != nil {
				return errors.New("owned financial verification failed")
			}
			_, _ = hash.Write([]byte(table + "\n"))
			_, _ = hash.Write(raw)
		}
		fmt.Println("economic_fingerprint=" + hex.EncodeToString(hash.Sum(nil)))
		return nil
	}
	var existing int
	if e = p.QueryRow(ctx, `SELECT count(*) FROM point_accounts`).Scan(&existing); e != nil {
		return errors.New("migrated fixture database required")
	}
	if existing != 0 {
		return errors.New("fixture database wallets must be empty")
	}
	const brand = "0199a000-0000-7000-8000-000000000002"
	accounts := []string{}
	for i, n := range []int64{11, 7, 5} {
		user, member, account := ids.New(), ids.New(), ids.New()
		tx, e := p.Begin(ctx)
		if e != nil {
			return errors.New("fixture transaction unavailable")
		}
		for _, q := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO global_users(id,username,password_hash) VALUES($1,$2,'non-login-synthetic-hash')`, []any{user, fmt.Sprintf("recon_user_%d", i+1)}},
			{`INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'domain','dev-1','dev-1')`, []any{member, brand, user}},
			{`INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, []any{account, brand, member}},
			{`INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT $1,$2,s,t FROM unnest(ARRAY['recharge','winning','gift'])s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal'])t`, []any{brand, account}},
		} {
			if _, e = tx.Exec(ctx, q.sql, q.args...); e != nil {
				_ = tx.Rollback(ctx)
				return errors.New("cannot create synthetic wallet")
			}
		}
		var delta points.Balance
		delta[2][0] = points.Amount(n)
		_, e = (points.Store{DB: p}).Post(ctx, tx, points.Change{BrandID: brand, MemberID: member, EntryType: "gift", ReferenceType: "reconciliation_browser_fixture", OperationKey: "recon-browser-" + ids.New(), Reason: "isolated synthetic browser funding", ActorType: "system", RequestID: ids.New(), Delta: delta, Allocation: []points.Allocation{{Source: "gift", State: "available", Points: points.Amount(n)}}})
		if e != nil {
			_ = tx.Rollback(ctx)
			return errors.New("cannot fund synthetic wallet")
		}
		if e = tx.Commit(ctx); e != nil {
			return errors.New("synthetic wallet commit failed")
		}
		accounts = append(accounts, account)
	}
	if _, e = p.Exec(ctx, `UPDATE point_buckets SET points=99 WHERE account_id=$1 AND source='gift' AND state='available'`, accounts[1]); e != nil {
		return errors.New("cannot inject owned balance difference")
	}
	if _, e = p.Exec(ctx, `DELETE FROM point_buckets WHERE account_id=$1 AND source='gift' AND state='withdrawal'`, accounts[1]); e != nil {
		return errors.New("cannot inject owned missing bucket")
	}
	var trigger string
	if e = p.QueryRow(ctx, `SELECT tgname FROM pg_trigger WHERE tgrelid='point_ledger_entries'::regclass AND NOT tgisinternal AND (tgtype & 2)=2 AND (tgtype & 16)=16 LIMIT 1`).Scan(&trigger); e != nil {
		return errors.New("fixture immutable guard missing")
	}
	quoted := pgx.Identifier{trigger}.Sanitize()
	tx, e := p.Begin(ctx)
	if e != nil {
		return errors.New("fixture damage transaction unavailable")
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `ALTER TABLE point_ledger_entries DISABLE TRIGGER `+quoted); e != nil {
		return errors.New("cannot inject owned ledger damage")
	}
	if _, e = tx.Exec(ctx, `UPDATE point_ledger_entries SET request_hash=$1 WHERE account_id=$2`, strings.Repeat("0", 64), accounts[2]); e != nil {
		return errors.New("cannot inject owned ledger hash difference")
	}
	if _, e = tx.Exec(ctx, `ALTER TABLE point_ledger_entries ENABLE TRIGGER `+quoted); e != nil {
		return errors.New("cannot restore fixture immutable guard")
	}
	if e = tx.Commit(ctx); e != nil {
		return errors.New("fixture damage commit failed")
	}
	// A separate corrupt child has no parent order, account or actual money
	// movement. Only this opt-in owned fixture may inject such a source. The
	// account-driven check cannot enumerate it; the brand inventory must do so.
	child, e := p.Begin(ctx)
	if e != nil {
		return errors.New("fixture orphan transaction unavailable")
	}
	defer child.Rollback(ctx)
	if _, e = child.Exec(ctx, `SET LOCAL session_replication_role=replica`); e != nil {
		return errors.New("owned orphan fixture privilege unavailable")
	}
	if _, e = child.Exec(ctx, `INSERT INTO reward_order_actions(id,brand_id,order_id,version,operation,state_after,actor_id,reason,audit_log_id)
	 VALUES($1,$2,$3,1,'grant','granted',$4,'owned orphan source without a real grant',$5)`, ids.New(), brand, ids.New(), ids.New(), ids.New()); e != nil {
		return errors.New("cannot inject owned parentless action")
	}
	if _, e = child.Exec(ctx, `SET LOCAL session_replication_role=origin`); e != nil {
		return errors.New("cannot restore owned orphan fixture guards")
	}
	if e = child.Commit(ctx); e != nil {
		return errors.New("owned orphan fixture commit failed")
	}
	fmt.Println("owned reconciliation fixture ready: 3 synthetic wallets and 1 parentless action; guards restored")
	return nil
}
