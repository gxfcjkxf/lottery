package finance

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	financeBrandA = "0199a000-0000-7000-8000-000000000001"
	financeBrandB = "0199a000-0000-7000-8000-000000000002"
)

type financeFixture struct {
	pool       *pgxpool.Pool
	service    Service
	brandID    string
	otherBrand string
	memberID   string
	adminID    string
	accountID  string
}

func newFinanceFixture(t *testing.T) financeFixture {
	t.Helper()
	ctx := context.Background()
	p := testdb.New(t)
	memberID, globalID, adminID, accountID := ids.New(), ids.New(), ids.New(), ids.New()
	if _, err := p.Exec(ctx, `INSERT INTO global_users(id,username,password_hash) VALUES($1,$2,'not-a-login-secret')`, globalID, "finance_member_"+memberID[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `INSERT INTO brand_members(id,brand_id,global_user_id,display_name,join_method,privacy_policy_version,service_terms_version)
		VALUES($1,$2,$3,'Finance member','operator','1','1')`, memberID, financeBrandA, globalID); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'not-a-password-hash')`, adminID, "finance_admin_"+adminID[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, accountID, financeBrandA, memberID); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"recharge", "winning", "gift"} {
		for _, state := range []string{"available", "manual_frozen", "system_frozen", "withdrawal"} {
			if _, err := p.Exec(ctx, `INSERT INTO point_buckets(brand_id,account_id,source,state,points) VALUES($1,$2,$3,$4,0)`, financeBrandA, accountID, source, state); err != nil {
				t.Fatal(err)
			}
		}
	}
	return financeFixture{pool: p, service: Service{DB: p, Points: points.Store{DB: p}}, brandID: financeBrandA, otherBrand: financeBrandB, memberID: memberID, adminID: adminID, accountID: accountID}
}

func financeTx(t *testing.T, p *pgxpool.Pool) pgx.Tx {
	t.Helper()
	tx, err := p.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}

func commitFinanceTx(t *testing.T, tx pgx.Tx) {
	t.Helper()
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func financeMeta(actor string) points.Metadata {
	return points.Metadata{ActorType: "admin", ActorID: actor, RequestID: "finance-integration-test", IP: "127.0.0.1"}
}

func TestRechargeCreateConfirmCreditsExactlyOnce(t *testing.T) {
	f := newFinanceFixture(t)
	ctx := context.Background()
	meta := financeMeta(f.adminID)
	tx := financeTx(t, f.pool)
	created, err := f.service.CreateRecharge(ctx, tx, f.brandID, f.memberID, points.Amount(725), "bank slip ref", "manual top-up", "verified deposit", meta)
	if err != nil {
		t.Fatal(err)
	}
	commitFinanceTx(t, tx)
	if created.ID == "" || created.BrandID != f.brandID || created.MemberID != f.memberID || created.AccountID != f.accountID || created.Points != "725" || created.State != "pending" || created.Version != 1 || created.LedgerEntryID != "" {
		t.Fatalf("unexpected pending recharge: %+v", created)
	}
	var ledgerBefore int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE reference_type='recharge' AND reference_id=$1`, created.ID).Scan(&ledgerBefore); err != nil || ledgerBefore != 0 {
		t.Fatalf("pending recharge wrote ledger entry count=%d err=%v", ledgerBefore, err)
	}

	tx = financeTx(t, f.pool)
	confirmed, err := f.service.ConfirmRecharge(ctx, tx, f.brandID, created.ID, created.Version, "statement matched", meta)
	if err != nil {
		t.Fatal(err)
	}
	commitFinanceTx(t, tx)
	if confirmed.State != "confirmed" || confirmed.Version != created.Version+1 || confirmed.LedgerEntryID == "" {
		t.Fatalf("unexpected confirmed recharge: %+v", confirmed)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE reference_type='recharge' AND reference_id=$1`, created.ID).Scan(&ledgerBefore); err != nil || ledgerBefore != 1 {
		t.Fatalf("confirmed recharge ledger count=%d err=%v, want exactly one", ledgerBefore, err)
	}
	var entryType, operationKey, reason string
	var delta, allocation []byte
	var entryIP string
	if err := f.pool.QueryRow(ctx, `SELECT l.entry_type,l.operation_key,l.reason,a.ip_address,l.delta_snapshot,l.source_allocation FROM point_ledger_entries l JOIN audit_logs a ON a.resource_id=l.account_id AND a.action='points.recharge' AND a.request_id=l.request_id WHERE l.id=$1`, confirmed.LedgerEntryID).Scan(&entryType, &operationKey, &reason, &entryIP, &delta, &allocation); err != nil {
		t.Fatal(err)
	}
	if entryType != "recharge" || operationKey != "recharge-confirm:"+created.ID || reason != "statement matched" || entryIP != "127.0.0.1" || !containsFinance(string(delta), `"725"`) || !containsFinance(string(allocation), `"available"`) {
		t.Fatalf("ledger entry does not match recharge contract: type=%q key=%q reason=%q ip=%q delta=%s allocation=%s", entryType, operationKey, reason, entryIP, delta, allocation)
	}
	tx = financeTx(t, f.pool)
	wallet, err := f.service.Points.Snapshot(ctx, tx, f.brandID, f.memberID)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	_ = tx.Rollback(ctx)
	if wallet.MemberID != f.memberID || wallet.BrandID != f.brandID || wallet.AccountID != f.accountID || wallet.BySource[0][0] != points.Amount(725) {
		t.Fatalf("confirmed points missing from recharge/available bucket: %+v", wallet)
	}

	// A later request key or version cannot apply the same business order twice.
	tx = financeTx(t, f.pool)
	_, err = f.service.ConfirmRecharge(ctx, tx, f.brandID, created.ID, confirmed.Version, "duplicate confirmation", points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: "second-idempotency-key"})
	_ = tx.Rollback(ctx)
	if !errors.Is(err, points.ErrConflict) {
		t.Fatalf("second confirmation error=%v, want points.ErrConflict", err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE reference_type='recharge' AND reference_id=$1`, created.ID).Scan(&ledgerBefore); err != nil || ledgerBefore != 1 {
		t.Fatalf("duplicate confirmation changed ledger count=%d err=%v", ledgerBefore, err)
	}
}

func TestRechargeValidationBrandIsolationAndList(t *testing.T) {
	f := newFinanceFixture(t)
	ctx := context.Background()
	meta := financeMeta(f.adminID)
	for _, tc := range []struct {
		name   string
		amount points.Amount
		proof  string
		remark string
		reason string
	}{{"zero amount", 0, "ref", "note", "reason"}, {"negative amount", -1, "ref", "note", "reason"}, {"empty reason", 1, "ref", "note", "  "}, {"long proof", 1, string(make([]byte, 501)), "note", "reason"}, {"long remark", 1, "ref", string(make([]byte, 2001)), "reason"}} {
		t.Run(tc.name, func(t *testing.T) {
			tx := financeTx(t, f.pool)
			_, err := f.service.CreateRecharge(ctx, tx, f.brandID, f.memberID, tc.amount, tc.proof, tc.remark, tc.reason, meta)
			_ = tx.Rollback(ctx)
			if !errors.Is(err, points.ErrInvalid) {
				t.Fatalf("invalid recharge error=%v, want points.ErrInvalid", err)
			}
		})
	}
	tx := financeTx(t, f.pool)
	_, err := f.service.CreateRecharge(ctx, tx, f.otherBrand, f.memberID, 10, "ref", "note", "wrong brand", meta)
	_ = tx.Rollback(ctx)
	if !errors.Is(err, points.ErrNotFound) {
		t.Fatalf("cross-brand recharge error=%v, want points.ErrNotFound", err)
	}
	if _, err := f.service.ListRecharges(ctx, f.otherBrand, f.memberID, 20, 0); !errors.Is(err, points.ErrNotFound) {
		t.Fatalf("cross-brand list error=%v, want points.ErrNotFound", err)
	}
	if _, err := f.service.ListRecharges(ctx, f.brandID, f.memberID, 101, 0); !errors.Is(err, points.ErrInvalid) {
		t.Fatalf("invalid list page error=%v, want points.ErrInvalid", err)
	}
	tx = financeTx(t, f.pool)
	pending, err := f.service.CreateRecharge(ctx, tx, f.brandID, f.memberID, 42, "proof-42", "memo-42", "valid reason", meta)
	if err != nil {
		t.Fatal(err)
	}
	commitFinanceTx(t, tx)
	rows, err := f.service.ListRecharges(ctx, f.brandID, f.memberID, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != pending.ID || rows[0].Points != "42" || rows[0].ProofReference != "proof-42" || rows[0].Remark != "memo-42" {
		t.Fatalf("brand-member recharge list mismatch: %+v", rows)
	}
}

func TestRechargeAuditsDoNotContainPointLedgerSecrets(t *testing.T) {
	f := newFinanceFixture(t)
	ctx := context.Background()
	tx := financeTx(t, f.pool)
	recharge, err := f.service.CreateRecharge(ctx, tx, f.brandID, f.memberID, 90, "proof-ref", "manual remark", "deposit verified", financeMeta(f.adminID))
	if err != nil {
		t.Fatal(err)
	}
	commitFinanceTx(t, tx)
	tx = financeTx(t, f.pool)
	confirmed, err := f.service.ConfirmRecharge(ctx, tx, f.brandID, recharge.ID, recharge.Version, "finance matched", financeMeta(f.adminID))
	if err != nil {
		t.Fatal(err)
	}
	commitFinanceTx(t, tx)
	var count int
	var snapshots string
	if err := f.pool.QueryRow(ctx, `SELECT count(*),string_agg(action||':'||COALESCE(before_json::text,'')||COALESCE(after_json::text,''),' ') FROM audit_logs WHERE resource_id=$1`, recharge.ID).Scan(&count, &snapshots); err != nil {
		t.Fatal(err)
	}
	if count != 2 || !containsFinance(snapshots, "finance.recharge.create") || !containsFinance(snapshots, "finance.recharge.confirm") || !containsFinance(snapshots, confirmed.LedgerEntryID) {
		t.Fatalf("business audits are incomplete: count=%d snapshot=%s", count, snapshots)
	}
}

func containsFinance(value, fragment string) bool {
	return strings.Contains(value, fragment)
}
