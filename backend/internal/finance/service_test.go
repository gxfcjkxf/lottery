package finance

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

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
	if _, err := f.service.ListRecharges(ctx, f.brandID, "", 20, 1_000_001); !errors.Is(err, points.ErrInvalid) {
		t.Fatalf("oversized list offset error=%v, want points.ErrInvalid", err)
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

	secondGlobal, secondMember, secondAccount := ids.New(), ids.New(), ids.New()
	if _, err := f.pool.Exec(ctx, `INSERT INTO global_users(id,username,password_hash) VALUES($1,$2,'not-a-login-secret')`, secondGlobal, "finance_member_extra_"+strings.ReplaceAll(secondMember, "-", "")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO brand_members(id,brand_id,global_user_id,display_name,join_method,privacy_policy_version,service_terms_version)
		VALUES($1,$2,$3,'Second finance member','operator','1','1')`, secondMember, f.brandID, secondGlobal); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, secondAccount, f.brandID, secondMember); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"recharge", "winning", "gift"} {
		for _, state := range []string{"available", "manual_frozen", "system_frozen", "withdrawal"} {
			if _, err := f.pool.Exec(ctx, `INSERT INTO point_buckets(brand_id,account_id,source,state,points) VALUES($1,$2,$3,$4,0)`, f.brandID, secondAccount, source, state); err != nil {
				t.Fatal(err)
			}
		}
	}
	tx = financeTx(t, f.pool)
	second, err := f.service.CreateRecharge(ctx, tx, f.brandID, secondMember, 7, "second-proof", "second memo", "second request", meta)
	if err != nil {
		t.Fatal(err)
	}
	commitFinanceTx(t, tx)
	all, err := f.service.ListRecharges(ctx, f.brandID, "", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].MemberID == all[1].MemberID {
		t.Fatalf("brand-wide optional-member list mismatch: %+v", all)
	}
	secondOnly, err := f.service.ListRecharges(ctx, f.brandID, secondMember, 100, 0)
	if err != nil || len(secondOnly) != 1 || secondOnly[0].ID != second.ID {
		t.Fatalf("member-filtered list=%+v err=%v", secondOnly, err)
	}
}

func TestConfirmAndCancelRejectMaxInt64Version(t *testing.T) {
	f := newFinanceFixture(t)
	ctx := context.Background()
	tx := financeTx(t, f.pool)
	pending, err := f.service.CreateRecharge(ctx, tx, f.brandID, f.memberID, 15, "overflow-proof", "overflow remark", "create order", financeMeta(f.adminID))
	if err != nil {
		t.Fatal(err)
	}
	commitFinanceTx(t, tx)
	if _, err := f.pool.Exec(ctx, `UPDATE recharge_orders SET version=$2 WHERE id=$1`, pending.ID, int64(math.MaxInt64)); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"confirm", "cancel"} {
		tx = financeTx(t, f.pool)
		var opErr error
		if operation == "confirm" {
			_, opErr = f.service.ConfirmRecharge(ctx, tx, f.brandID, pending.ID, math.MaxInt64, "overflow version", financeMeta(f.adminID))
		} else {
			_, opErr = f.service.CancelRecharge(ctx, tx, f.brandID, pending.ID, math.MaxInt64, "overflow version", financeMeta(f.adminID))
		}
		_ = tx.Rollback(ctx)
		if !errors.Is(opErr, points.ErrOverflow) {
			t.Fatalf("%s at MaxInt64 error=%v, want points.ErrOverflow", operation, opErr)
		}
	}
	var state string
	var version int64
	if err := f.pool.QueryRow(ctx, `SELECT state,version FROM recharge_orders WHERE id=$1`, pending.ID).Scan(&state, &version); err != nil {
		t.Fatal(err)
	}
	if state != "pending" || version != math.MaxInt64 {
		t.Fatalf("overflow guard mutated order: state=%s version=%d", state, version)
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

func TestCancelPendingRechargeLeavesWalletUntouchedAndAuditsTransition(t *testing.T) {
	f := newFinanceFixture(t)
	ctx := context.Background()
	meta := financeMeta(f.adminID)
	tx := financeTx(t, f.pool)
	created, err := f.service.CreateRecharge(ctx, tx, f.brandID, f.memberID, 725, "immutable-proof", "immutable remark", "manual request", meta)
	if err != nil {
		t.Fatal(err)
	}
	commitFinanceTx(t, tx)

	tx = financeTx(t, f.pool)
	cancelled, err := f.service.CancelRecharge(ctx, tx, f.brandID, created.ID, created.Version, "request withdrawn", meta)
	if err != nil {
		t.Fatal(err)
	}
	commitFinanceTx(t, tx)
	if cancelled.State != "cancelled" || cancelled.Version != created.Version+1 || cancelled.Points != created.Points || cancelled.ProofReference != created.ProofReference || cancelled.Remark != created.Remark || cancelled.LedgerEntryID != "" || cancelled.ConfirmedBy != "" || cancelled.ConfirmedAt != nil || cancelled.AuditLogID == "" {
		t.Fatalf("unexpected cancelled recharge: before=%+v after=%+v", created, cancelled)
	}
	var ledgerCount, auditCount int
	var action, reason, snapshots string
	if err := f.pool.QueryRow(ctx, `SELECT count(*),string_agg(action,','),string_agg(reason,','),string_agg(COALESCE(before_json::text,'')||' '||COALESCE(after_json::text,''),' ')
		FROM audit_logs WHERE resource_id=$1`, created.ID).Scan(&auditCount, &action, &reason, &snapshots); err != nil {
		t.Fatal(err)
	}
	if auditCount != 2 || !containsFinance(action, "finance.recharge.create") || !containsFinance(action, "finance.recharge.cancel") || !containsFinance(reason, "request withdrawn") || !containsFinance(snapshots, `"state": "pending"`) || !containsFinance(snapshots, `"state": "cancelled"`) {
		t.Fatalf("cancel audit missing transition: count=%d action=%q reason=%q snapshots=%s", auditCount, action, reason, snapshots)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE reference_type='recharge' AND reference_id=$1`, created.ID).Scan(&ledgerCount); err != nil || ledgerCount != 0 {
		t.Fatalf("cancelled recharge ledger count=%d err=%v, want 0", ledgerCount, err)
	}
	tx = financeTx(t, f.pool)
	wallet, err := f.service.Points.Snapshot(ctx, tx, f.brandID, f.memberID)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	_ = tx.Rollback(ctx)
	if wallet.DisplayPoints != 0 || wallet.Version != 0 {
		t.Fatalf("cancellation changed wallet: %+v", wallet)
	}
}

func TestCancelRechargeRejectsDuplicateAndConfirmedOrders(t *testing.T) {
	f := newFinanceFixture(t)
	ctx := context.Background()
	meta := financeMeta(f.adminID)
	create := func(key string) Recharge {
		t.Helper()
		tx := financeTx(t, f.pool)
		r, err := f.service.CreateRecharge(ctx, tx, f.brandID, f.memberID, 10, "proof-"+key, "remark", "create reason", meta)
		if err != nil {
			t.Fatal(err)
		}
		commitFinanceTx(t, tx)
		return r
	}
	pending := create("pending-cancel")
	tx := financeTx(t, f.pool)
	if _, err := f.service.CancelRecharge(ctx, tx, f.brandID, pending.ID, pending.Version, "", meta); !errors.Is(err, points.ErrInvalid) {
		t.Fatalf("empty cancellation reason error=%v, want points.ErrInvalid", err)
	}
	_ = tx.Rollback(ctx)
	tx = financeTx(t, f.pool)
	if _, err := f.service.CancelRecharge(ctx, tx, f.brandID, pending.ID, pending.Version, "cancel", meta); err != nil {
		t.Fatal(err)
	}
	commitFinanceTx(t, tx)
	for _, version := range []int64{pending.Version, pending.Version + 1} {
		tx = financeTx(t, f.pool)
		_, err := f.service.CancelRecharge(ctx, tx, f.brandID, pending.ID, version, "duplicate cancel", meta)
		_ = tx.Rollback(ctx)
		if !errors.Is(err, points.ErrConflict) {
			t.Fatalf("duplicate cancellation version=%d error=%v, want points.ErrConflict", version, err)
		}
	}
	confirmedOrder := create("confirmed-cancel")
	tx = financeTx(t, f.pool)
	confirmed, err := f.service.ConfirmRecharge(ctx, tx, f.brandID, confirmedOrder.ID, confirmedOrder.Version, "verified", meta)
	if err != nil {
		t.Fatal(err)
	}
	commitFinanceTx(t, tx)
	tx = financeTx(t, f.pool)
	_, err = f.service.CancelRecharge(ctx, tx, f.brandID, confirmed.ID, confirmed.Version, "too late", meta)
	_ = tx.Rollback(ctx)
	if !errors.Is(err, points.ErrConflict) {
		t.Fatalf("confirmed recharge cancellation error=%v, want points.ErrConflict", err)
	}
}

func TestConcurrentConfirmAndCancelOnlyOneWins(t *testing.T) {
	f := newFinanceFixture(t)
	ctx := context.Background()
	meta := financeMeta(f.adminID)
	tx := financeTx(t, f.pool)
	created, err := f.service.CreateRecharge(ctx, tx, f.brandID, f.memberID, 37, "race-proof", "race remark", "race create", meta)
	if err != nil {
		t.Fatal(err)
	}
	commitFinanceTx(t, tx)
	type outcome struct {
		action string
		state  string
		err    error
	}
	results := make(chan outcome, 2)
	var start sync.WaitGroup
	start.Add(1)
	for _, action := range []string{"confirm", "cancel"} {
		action := action
		go func() {
			start.Wait()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			tx, err := f.pool.Begin(ctx)
			if err != nil {
				results <- outcome{action: action, err: err}
				return
			}
			var record Recharge
			if action == "confirm" {
				record, err = f.service.ConfirmRecharge(ctx, tx, f.brandID, created.ID, created.Version, "race confirmation", meta)
			} else {
				record, err = f.service.CancelRecharge(ctx, tx, f.brandID, created.ID, created.Version, "race cancellation", meta)
			}
			if err == nil {
				err = tx.Commit(ctx)
			} else {
				_ = tx.Rollback(ctx)
			}
			results <- outcome{action: action, state: record.State, err: err}
		}()
	}
	start.Done()
	var outcomes []outcome
	for i := 0; i < 2; i++ {
		select {
		case result := <-results:
			outcomes = append(outcomes, result)
		case <-time.After(20 * time.Second):
			t.Fatal("confirm/cancel race timed out")
		}
	}
	winners, conflicts := 0, 0
	for _, result := range outcomes {
		if result.err == nil {
			winners++
			if result.state != "confirmed" && result.state != "cancelled" {
				t.Fatalf("unexpected winning state: %+v", result)
			}
		} else if errors.Is(result.err, points.ErrConflict) {
			conflicts++
		} else {
			t.Fatalf("unexpected race error: %+v", result)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("confirm/cancel race: winners=%d conflicts=%d results=%+v", winners, conflicts, outcomes)
	}
	var state string
	var ledgerCount, auditCount int
	if err := f.pool.QueryRow(ctx, `SELECT state FROM recharge_orders WHERE id=$1`, created.ID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE reference_type='recharge' AND reference_id=$1`, created.ID).Scan(&ledgerCount); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE resource_id=$1`, created.ID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	wantLedger := 0
	if state == "confirmed" {
		wantLedger = 1
	} else if state != "cancelled" {
		t.Fatalf("race left invalid recharge state %q", state)
	}
	if ledgerCount != wantLedger || auditCount != 2 {
		t.Fatalf("race final state=%s ledger=%d audits=%d; want ledger=%d audits=2", state, ledgerCount, auditCount, wantLedger)
	}
}

func TestRechargePolicyIsCheckedAtCreateAndConfirm(t *testing.T) {
	f := newFinanceFixture(t)
	ctx := context.Background()
	meta := financeMeta(f.adminID)
	if _, err := f.pool.Exec(ctx, `UPDATE brand_point_policies SET max_recharge_points=50 WHERE brand_id=$1`, f.brandID); err != nil {
		t.Fatal(err)
	}
	tx := financeTx(t, f.pool)
	_, err := f.service.CreateRecharge(ctx, tx, f.brandID, f.memberID, 51, "too-large", "remark", "over limit", meta)
	_ = tx.Rollback(ctx)
	if !errors.Is(err, points.ErrPolicyLimit) {
		t.Fatalf("over-limit create error=%v, want points.ErrPolicyLimit", err)
	}
	tx = financeTx(t, f.pool)
	pending, err := f.service.CreateRecharge(ctx, tx, f.brandID, f.memberID, 50, "within-limit", "remark", "within limit", meta)
	if err != nil {
		t.Fatal(err)
	}
	commitFinanceTx(t, tx)
	if _, err := f.pool.Exec(ctx, `UPDATE brand_point_policies SET max_recharge_points=49,version=version+1 WHERE brand_id=$1`, f.brandID); err != nil {
		t.Fatal(err)
	}
	tx = financeTx(t, f.pool)
	_, err = f.service.ConfirmRecharge(ctx, tx, f.brandID, pending.ID, pending.Version, "policy changed", meta)
	_ = tx.Rollback(ctx)
	if !errors.Is(err, points.ErrPolicyLimit) {
		t.Fatalf("confirmation after policy update error=%v, want points.ErrPolicyLimit", err)
	}
	var state string
	var version int64
	if err := f.pool.QueryRow(ctx, `SELECT state,version FROM recharge_orders WHERE id=$1`, pending.ID).Scan(&state, &version); err != nil {
		t.Fatal(err)
	}
	if state != "pending" || version != pending.Version {
		t.Fatalf("failed policy confirmation mutated recharge: state=%s version=%d", state, version)
	}
}

func containsFinance(value, fragment string) bool {
	return strings.Contains(value, fragment)
}
