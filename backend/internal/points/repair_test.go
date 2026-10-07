package points

import (
	"context"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"testing"
)

func repairActor(t *testing.T, p *pgxpool.Pool) Metadata {
	t.Helper()
	id := ids.New()
	_, err := p.Exec(context.Background(), `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only-opaque-hash')`, id, "repair_"+id)
	if err != nil {
		t.Fatal(err)
	}
	return Metadata{ActorType: "admin", ActorID: id, RequestID: ids.New(), IP: "192.0.2.9"}
}
func TestRepairBalanceRebuildsOnlyIntactLedgerAndRetainsEvidence(t *testing.T) {
	p, s, member := fixture(t)
	ctx := context.Background()
	credit(t, p, s, member, 0, 100)
	meta := repairActor(t, p)
	var account string
	p.QueryRow(ctx, `SELECT id::text FROM point_accounts WHERE brand_member_id=$1`, member).Scan(&account)
	if _, err := p.Exec(ctx, `UPDATE point_buckets SET points=999 WHERE account_id=$1 AND source='recharge' AND state='available'`, account); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `DELETE FROM point_buckets WHERE account_id=$1 AND source='commission' AND state='withdrawal'`, account); err != nil {
		t.Fatal(err)
	}
	preview, err := s.PreviewRepair(ctx, testBrand, member)
	if err != nil || !preview.Repairable || preview.Consistent || preview.Expected[0][0] != 100 {
		t.Fatal(preview, err)
	}
	tx, _ := p.Begin(ctx)
	record, err := s.RepairBalance(ctx, tx, testBrand, member, preview.Version, preview.Token, "rebuild projection from original ledger", meta)
	if err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if record.ID == "" || record.AuditLogID == "" || record.Preview.Actual["recharge"]["available"] != 999 {
		t.Fatal(record)
	}
	wallet, err := s.Read(ctx, testBrand, member)
	if err != nil || wallet.DisplayPoints != 100 || wallet.Version != 1 {
		t.Fatal(wallet, err)
	}
	var bucketCount int
	var commissionWithdrawal int64
	if err = p.QueryRow(ctx, `SELECT count(*) FROM point_buckets WHERE brand_id=$1 AND account_id=$2`, testBrand, account).Scan(&bucketCount); err != nil {
		t.Fatal(err)
	}
	if err = p.QueryRow(ctx, `SELECT points FROM point_buckets WHERE brand_id=$1 AND account_id=$2 AND source='commission' AND state='withdrawal'`, testBrand, account).Scan(&commissionWithdrawal); err != nil {
		t.Fatal(err)
	}
	if bucketCount != 16 || commissionWithdrawal != 0 {
		t.Fatalf("repair did not restore strict 16-bucket projection: count=%d commission withdrawal=%d", bucketCount, commissionWithdrawal)
	}
	reconciled, err := s.Reconcile(ctx, testBrand, member)
	if err != nil || !reconciled.Consistent || reconciled.EntryCount != 1 {
		t.Fatal(reconciled, err)
	}
	for _, q := range []string{`UPDATE point_balance_repairs SET reason='rewrite' WHERE id=$1`, `DELETE FROM point_balance_repairs WHERE id=$1`} {
		if _, err = p.Exec(ctx, q, record.ID); err == nil {
			t.Fatal("repair evidence mutable")
		}
	}
	tx, _ = p.Begin(ctx)
	_, err = s.RepairBalance(ctx, tx, testBrand, member, preview.Version, preview.Token, "stale retry", meta)
	tx.Rollback(ctx)
	if !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	credit(t, p, s, member, 2, 5)
	wallet, err = s.Read(ctx, testBrand, member)
	if err != nil || wallet.AvailablePoints != 105 {
		t.Fatal(wallet, err)
	}
	// The derived version is repaired from the intact chain too, without
	// appending a fictitious economic entry to fill the version gap.
	if _, err = p.Exec(ctx, `UPDATE point_accounts SET version=99 WHERE id=$1`, account); err != nil {
		t.Fatal(err)
	}
	preview, err = s.PreviewRepair(ctx, testBrand, member)
	if err != nil || !preview.Repairable || preview.Version != 99 || preview.LedgerVersion != 2 {
		t.Fatal(preview, err)
	}
	tx, _ = p.Begin(ctx)
	_, err = s.RepairBalance(ctx, tx, testBrand, member, preview.Version, preview.Token, "restore derived version", meta)
	if err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	reconciled, err = s.Reconcile(ctx, testBrand, member)
	if err != nil || !reconciled.Consistent || reconciled.EntryCount != 2 || reconciled.Version != 2 {
		t.Fatal(reconciled, err)
	}
}
func TestRepairStaleObservationRollbackAndInvalidLedgerRefusal(t *testing.T) {
	p, s, member := fixture(t)
	ctx := context.Background()
	credit(t, p, s, member, 0, 10)
	meta := repairActor(t, p)
	p.Exec(ctx, `UPDATE point_buckets SET points=19 WHERE account_id IN(SELECT id FROM point_accounts WHERE brand_member_id=$1) AND source='recharge' AND state='available'`, member)
	preview, err := s.PreviewRepair(ctx, testBrand, member)
	if err != nil {
		t.Fatal(err)
	}
	p.Exec(ctx, `UPDATE point_buckets SET points=20 WHERE account_id=$1 AND source='recharge' AND state='available'`, preview.AccountID)
	tx, _ := p.Begin(ctx)
	_, err = s.RepairBalance(ctx, tx, testBrand, member, preview.Version, preview.Token, "changed balance observation", meta)
	tx.Rollback(ctx)
	if !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	preview, err = s.PreviewRepair(ctx, testBrand, member)
	if err != nil {
		t.Fatal(err)
	}
	tx, _ = p.Begin(ctx)
	_, err = s.RepairBalance(ctx, tx, testBrand, member, preview.Version, preview.Token, "rollback repair", meta)
	if err != nil {
		t.Fatal(err)
	}
	tx.Rollback(ctx)
	wallet, err := s.Read(ctx, testBrand, member)
	if err != nil || wallet.AvailablePoints != 20 {
		t.Fatal(wallet, err)
	}
	var repairCount int
	p.QueryRow(ctx, `SELECT count(*) FROM point_balance_repairs`).Scan(&repairCount)
	if repairCount != 0 {
		t.Fatal("rollback retained repair", repairCount)
	}
	// A fabricated historical entry has no matching request hash. Do not turn it
	// into money merely because its snapshots look plausible.
	legacyFaultTx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, trigger := range []string{"ledger_four_source_snapshot", "ledger_four_source_projection"} {
		if _, err = legacyFaultTx.Exec(ctx, `ALTER TABLE point_ledger_entries DISABLE TRIGGER `+trigger); err != nil {
			legacyFaultTx.Rollback(ctx)
			t.Fatal(err)
		}
	}
	_, err = legacyFaultTx.Exec(ctx, `INSERT INTO point_ledger_entries(id,brand_id,account_id,member_id,version,entry_type,reference_type,operation_key,before_snapshot,delta_snapshot,after_snapshot,source_allocation,reason,actor_type,request_id,request_hash)
	 SELECT $1,brand_id,account_id,member_id,2,entry_type,reference_type,'fabricated',after_snapshot,delta_snapshot,after_snapshot,source_allocation,reason,actor_type,request_id,$2 FROM point_ledger_entries WHERE member_id=$3`, ids.New(), strings.Repeat("0", 64), member)
	if err != nil {
		legacyFaultTx.Rollback(ctx)
		t.Fatal(err)
	}
	for _, trigger := range []string{"ledger_four_source_snapshot", "ledger_four_source_projection"} {
		if _, err = legacyFaultTx.Exec(ctx, `ALTER TABLE point_ledger_entries ENABLE TRIGGER `+trigger); err != nil {
			legacyFaultTx.Rollback(ctx)
			t.Fatal(err)
		}
	}
	if err = legacyFaultTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	preview, err = s.PreviewRepair(ctx, testBrand, member)
	if err != nil || preview.Repairable {
		t.Fatal(preview, err)
	}
	tx, _ = p.Begin(ctx)
	_, err = s.RepairBalance(ctx, tx, testBrand, member, preview.Version, preview.Token, "refuse fabricated ledger", meta)
	tx.Rollback(ctx)
	if !errors.Is(err, ErrCorrupt) {
		t.Fatal(err)
	}
	if _, err = s.PreviewRepair(ctx, "0199a000-0000-7000-8000-000000000002", member); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func TestRepairRestoresMissingNonzeroCommissionBucket(t *testing.T) {
	p, s, member := fixture(t)
	ctx := context.Background()
	credit(t, p, s, member, 3, 19)
	meta := repairActor(t, p)
	if _, err := p.Exec(ctx, `DELETE FROM point_buckets WHERE account_id=(SELECT id FROM point_accounts WHERE brand_member_id=$1) AND source='commission' AND state='available'`, member); err != nil {
		t.Fatal(err)
	}
	preview, err := s.PreviewRepair(ctx, testBrand, member)
	if err != nil || !preview.Repairable || preview.Expected[3][0] != 19 {
		t.Fatalf("missing commission preview=%+v err=%v", preview, err)
	}
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RepairBalance(ctx, tx, testBrand, member, preview.Version, preview.Token, "restore missing commission projection", meta); err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	wallet, err := s.Read(ctx, testBrand, member)
	if err != nil || wallet.CommissionPoints != 19 || wallet.BySource[3][0] != 19 {
		t.Fatalf("repaired commission wallet=%+v err=%v", wallet, err)
	}
	var bucketCount int
	if err = p.QueryRow(ctx, `SELECT count(*) FROM point_buckets WHERE account_id=(SELECT id FROM point_accounts WHERE brand_member_id=$1)`, member).Scan(&bucketCount); err != nil || bucketCount != 16 {
		t.Fatalf("repaired commission bucket count=%d err=%v", bucketCount, err)
	}
}
