package finance

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

func TestRechargeQuotaSharedConcurrentNoOverdraft(t *testing.T) {
	f := newFinanceFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE brand_recharge_quotas SET available=100 WHERE brand_id=$1`, f.brandID); err != nil {
		t.Fatal(err)
	}
	other := ids.New()
	if _, err := f.pool.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'unused')`, other, "quota_second_"+other); err != nil {
		t.Fatal(err)
	}
	orders := make([]Recharge, 2)
	for i := range orders {
		tx := financeTx(t, f.pool)
		var err error
		orders[i], err = f.service.CreateRecharge(ctx, tx, f.brandID, f.memberID, 7000, "", "", "", financeMeta(f.adminID))
		if err != nil {
			t.Fatal(err)
		}
		commitFinanceTx(t, tx)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i, order := range orders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, err := f.pool.Begin(ctx)
			if err != nil {
				results <- err
				return
			}
			defer tx.Rollback(ctx)
			actor := f.adminID
			if i == 1 {
				actor = other
			}
			_, err = f.service.ConfirmRecharge(ctx, tx, f.brandID, order.ID, 1, "", financeMeta(actor))
			if err == nil {
				err = tx.Commit(ctx)
			}
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	success, insufficient := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrRechargeQuota) {
			insufficient++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || insufficient != 1 {
		t.Fatalf("success=%d insufficient=%d", success, insufficient)
	}
	quota, err := f.service.ReadRechargeQuota(ctx, f.brandID)
	if err != nil || quota.Available != "30" {
		t.Fatalf("quota=%+v err=%v", quota, err)
	}
	wallet, err := f.service.Points.Read(ctx, f.brandID, f.memberID)
	if err != nil {
		t.Fatal(err)
	}
	_ = wallet
	var credit int64
	var count int
	if err = f.pool.QueryRow(ctx, `SELECT points FROM point_buckets WHERE brand_id=$1 AND account_id=$2 AND source='recharge' AND state='available'`, f.brandID, f.accountID).Scan(&credit); err != nil || credit != 70 {
		t.Fatalf("credit=%d err=%v", credit, err)
	}
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM brand_recharge_quota_entries WHERE brand_id=$1`, f.brandID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("entries=%d err=%v", count, err)
	}
	for _, order := range orders {
		tx := financeTx(t, f.pool)
		_, err = f.service.ConfirmRecharge(ctx, tx, f.otherBrand, order.ID, 1, "", financeMeta(other))
		if !errors.Is(err, points.ErrNotFound) {
			t.Fatalf("cross-brand error=%v", err)
		}
	}
}
func TestRechargeQuotaGrantRollbackAndAudit(t *testing.T) {
	f := newFinanceFixture(t)
	ctx := context.Background()
	meta := financeMeta(f.adminID)
	tx := financeTx(t, f.pool)
	out, err := f.service.GrantRechargeQuota(ctx, tx, f.brandID, 20000, meta)
	if err != nil || out.Available != "1000200" {
		t.Fatalf("quota=%+v err=%v", out, err)
	}
	commitFinanceTx(t, tx)
	entries, err := f.service.ListRechargeQuotaEntries(ctx, f.brandID, 10, 0)
	if err != nil || len(entries) != 1 || entries[0].Kind != "grant" || entries[0].BeforeAvailable != "1000000" || entries[0].AfterAvailable != "1000200" {
		t.Fatalf("entries=%+v err=%v", entries, err)
	}
	if _, err = f.pool.Exec(ctx, `UPDATE brand_recharge_quota_entries SET points=1 WHERE id=$1`, entries[0].ID); err == nil {
		t.Fatal("quota ledger is mutable")
	}
	tx = financeTx(t, f.pool)
	order, err := f.service.CreateRecharge(ctx, tx, f.brandID, f.memberID, 2000, "", "", "", meta)
	if err != nil {
		t.Fatal(err)
	}
	commitFinanceTx(t, tx)
	tx = financeTx(t, f.pool)
	if _, err = f.service.ConfirmRecharge(ctx, tx, f.brandID, order.ID, 1, "", meta); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	out, err = f.service.ReadRechargeQuota(ctx, f.brandID)
	if err != nil || out.Available != "1000200" {
		t.Fatalf("rollback quota=%+v err=%v", out, err)
	}
	var credits int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE reference_id=$1`, order.ID).Scan(&credits); err != nil || credits != 0 {
		t.Fatalf("rollback credit=%d err=%v", credits, err)
	}
}

func TestRechargeQuotaReclaimOnlyUnusedAndPreservesMemberPoints(t *testing.T) {
	f := newFinanceFixture(t)
	ctx := context.Background()
	meta := financeMeta(f.adminID)
	if _, err := f.pool.Exec(ctx, `UPDATE brand_recharge_quotas SET available=0 WHERE brand_id=$1`, f.brandID); err != nil {
		t.Fatal(err)
	}
	tx := financeTx(t, f.pool)
	if _, err := f.service.GrantRechargeQuota(ctx, tx, f.brandID, 10000, meta); err != nil {
		t.Fatal(err)
	}
	commitFinanceTx(t, tx)
	tx = financeTx(t, f.pool)
	order, err := f.service.CreateRecharge(ctx, tx, f.brandID, f.memberID, 6000, "", "", "", meta)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.ConfirmRecharge(ctx, tx, f.brandID, order.ID, 1, "", meta); err != nil {
		t.Fatal(err)
	}
	commitFinanceTx(t, tx)
	tx = financeTx(t, f.pool)
	if _, err = f.service.ReclaimRechargeQuota(ctx, tx, f.brandID, 4100, meta); !errors.Is(err, ErrRechargeQuota) {
		t.Fatalf("oversized reclaim=%v", err)
	}
	_ = tx.Rollback(ctx)
	for _, amount := range []points.Amount{2500, 1500} {
		tx = financeTx(t, f.pool)
		if _, err = f.service.ReclaimRechargeQuota(ctx, tx, f.brandID, amount, meta); err != nil {
			t.Fatal(err)
		}
		commitFinanceTx(t, tx)
	}
	quota, err := f.service.ReadRechargeQuota(ctx, f.brandID)
	if err != nil || quota.Available != "0" {
		t.Fatalf("quota=%+v err=%v", quota, err)
	}
	var credited int64
	if err = f.pool.QueryRow(ctx, `SELECT points FROM point_buckets WHERE brand_id=$1 AND account_id=$2 AND source='recharge' AND state='available'`, f.brandID, f.accountID).Scan(&credited); err != nil || credited != 60 {
		t.Fatalf("member points=%d err=%v", credited, err)
	}
	entries, err := f.service.ListRechargeQuotaEntries(ctx, f.brandID, 10, 0)
	if err != nil || len(entries) != 4 {
		t.Fatalf("entries=%+v err=%v", entries, err)
	}
	if entries[0].Kind != "reclaim" || entries[0].BeforeAvailable != "15" || entries[0].AfterAvailable != "0" || entries[0].RechargeOrderID != nil {
		t.Fatalf("last reclaim=%+v", entries[0])
	}
	var auditCount int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE brand_id=$1 AND action='finance.recharge_quota.reclaim'`, f.brandID).Scan(&auditCount); err != nil || auditCount != 2 {
		t.Fatalf("audits=%d err=%v", auditCount, err)
	}
	tx = financeTx(t, f.pool)
	if _, err = f.service.ReclaimRechargeQuota(ctx, tx, f.brandID, 0, meta); !errors.Is(err, points.ErrInvalid) {
		t.Fatalf("zero reclaim=%v", err)
	}
}

func TestRechargeQuotaReclaimConcurrentWithMemberRecharge(t *testing.T) {
	f := newFinanceFixture(t)
	ctx := context.Background()
	meta := financeMeta(f.adminID)
	if _, err := f.pool.Exec(ctx, `UPDATE brand_recharge_quotas SET available=100 WHERE brand_id=$1`, f.brandID); err != nil {
		t.Fatal(err)
	}
	tx := financeTx(t, f.pool)
	order, err := f.service.CreateRecharge(ctx, tx, f.brandID, f.memberID, 7000, "", "", "", meta)
	if err != nil {
		t.Fatal(err)
	}
	commitFinanceTx(t, tx)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, reclaim := range []bool{true, false} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, e := f.pool.Begin(ctx)
			if e != nil {
				results <- e
				return
			}
			defer tx.Rollback(ctx)
			if reclaim {
				_, e = f.service.ReclaimRechargeQuota(ctx, tx, f.brandID, 7000, meta)
			} else {
				_, e = f.service.ConfirmRecharge(ctx, tx, f.brandID, order.ID, 1, "", meta)
			}
			if e == nil {
				e = tx.Commit(ctx)
			}
			results <- e
		}()
	}
	wg.Wait()
	close(results)
	success, denied := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else if errors.Is(e, ErrRechargeQuota) {
			denied++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || denied != 1 {
		t.Fatalf("success=%d denied=%d", success, denied)
	}
	quota, err := f.service.ReadRechargeQuota(ctx, f.brandID)
	if err != nil || quota.Available != "30" {
		t.Fatalf("quota=%+v err=%v", quota, err)
	}
	var entries int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM brand_recharge_quota_entries WHERE brand_id=$1`, f.brandID).Scan(&entries); err != nil || entries != 1 {
		t.Fatalf("entries=%d err=%v", entries, err)
	}
}

func TestManualAdjustmentAndRechargeShareQuotaConcurrently(t *testing.T) {
	f := newFinanceFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE brand_recharge_quotas SET available=100 WHERE brand_id=$1`, f.brandID); err != nil {
		t.Fatal(err)
	}
	other := ids.New()
	if _, err := f.pool.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'unused')`, other, "quota_adjust_"+other); err != nil {
		t.Fatal(err)
	}
	tx := financeTx(t, f.pool)
	order, err := f.service.CreateRecharge(ctx, tx, f.brandID, f.memberID, 7000, "", "", "", financeMeta(f.adminID))
	if err != nil {
		t.Fatal(err)
	}
	commitFinanceTx(t, tx)
	var delta points.Balance
	delta[2][0] = 7000
	change := points.Change{BrandID: f.brandID, MemberID: f.memberID, EntryType: "adjust", ReferenceType: "manual", ReferenceID: f.memberID, OperationKey: "quota-adjust-concurrent", ActorType: "admin", ActorID: other, RequestID: "quota-adjust-concurrent", Delta: delta, Allocation: []points.Allocation{{Source: "gift", State: "available", Points: 7000}}}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, adjust := range []bool{true, false} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, err := f.pool.Begin(ctx)
			if err != nil {
				results <- err
				return
			}
			defer tx.Rollback(ctx)
			if adjust {
				_, err = f.service.PostManualAdjustment(ctx, tx, change)
			} else {
				_, err = f.service.ConfirmRecharge(ctx, tx, f.brandID, order.ID, 1, "", financeMeta(f.adminID))
			}
			if err == nil {
				err = tx.Commit(ctx)
			}
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	success, insufficient := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrRechargeQuota) {
			insufficient++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || insufficient != 1 {
		t.Fatalf("success=%d insufficient=%d", success, insufficient)
	}
	quota, err := f.service.ReadRechargeQuota(ctx, f.brandID)
	if err != nil || quota.Available != "30" {
		t.Fatal(quota, err)
	}
	wallet, err := f.service.Points.Read(ctx, f.brandID, f.memberID)
	if err != nil || wallet.AvailablePoints != 7000 {
		t.Fatal(wallet, err)
	}
}

func TestManualAdjustmentReplayWithExhaustedQuota(t *testing.T) {
	f := newFinanceFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE brand_recharge_quotas SET available=20 WHERE brand_id=$1`, f.brandID); err != nil {
		t.Fatal(err)
	}
	var delta points.Balance
	delta[2][0] = 2000
	change := points.Change{BrandID: f.brandID, MemberID: f.memberID, EntryType: "adjust", ReferenceType: "manual", ReferenceID: f.memberID, OperationKey: "quota-adjust-replay", ActorType: "admin", ActorID: f.adminID, RequestID: "quota-adjust-replay", Delta: delta, Allocation: []points.Allocation{{Source: "gift", State: "available", Points: 2000}}}
	for i := 0; i < 2; i++ {
		tx := financeTx(t, f.pool)
		if _, err := f.service.PostManualAdjustment(ctx, tx, change); err != nil {
			t.Fatal(err)
		}
		commitFinanceTx(t, tx)
	}
	quota, err := f.service.ReadRechargeQuota(ctx, f.brandID)
	if err != nil || quota.Available != "0" {
		t.Fatal(quota, err)
	}
	entries, err := f.service.ListRechargeQuotaEntries(ctx, f.brandID, 10, 0)
	if err != nil || len(entries) != 1 || entries[0].Kind != "manual_adjustment" {
		t.Fatal(entries, err)
	}
	wallet, err := f.service.Points.Read(ctx, f.brandID, f.memberID)
	if err != nil || wallet.AvailablePoints != 2000 {
		t.Fatal(wallet, err)
	}
}

func TestRechargeQuotaEntriesIdentifyMemberDestinations(t *testing.T) {
	f := newFinanceFixture(t)
	ctx := context.Background()
	meta := financeMeta(f.adminID)
	tx := financeTx(t, f.pool)
	if _, err := f.service.GrantRechargeQuota(ctx, tx, f.brandID, 10000, meta); err != nil {
		t.Fatal(err)
	}
	order, err := f.service.CreateRecharge(ctx, tx, f.brandID, f.memberID, 2000, "", "", "", meta)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.ConfirmRecharge(ctx, tx, f.brandID, order.ID, 1, "", meta); err != nil {
		t.Fatal(err)
	}
	var delta points.Balance
	delta[2][0] = 1000
	_, err = f.service.PostManualAdjustment(ctx, tx, points.Change{BrandID: f.brandID, MemberID: f.memberID, EntryType: "adjust", ReferenceType: "manual", ReferenceID: f.memberID, OperationKey: "quota-destination", ActorType: "admin", ActorID: f.adminID, RequestID: "quota-destination", Delta: delta, Allocation: []points.Allocation{{Source: "gift", State: "available", Points: 1000}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.ReclaimRechargeQuota(ctx, tx, f.brandID, 1000, meta); err != nil {
		t.Fatal(err)
	}
	commitFinanceTx(t, tx)
	entries, err := f.service.ListRechargeQuotaEntries(ctx, f.brandID, 10, 0)
	if err != nil || len(entries) != 4 {
		t.Fatalf("entries=%+v err=%v", entries, err)
	}
	for _, entry := range entries {
		switch entry.Kind {
		case "manual_recharge", "manual_adjustment":
			if entry.MemberID == nil || *entry.MemberID != f.memberID || entry.MemberAccount == nil || *entry.MemberAccount != "finance_member_"+f.memberID[:8] {
				t.Fatalf("destination=%+v", entry)
			}
		default:
			if entry.MemberID != nil || entry.MemberAccount != nil {
				t.Fatalf("platform flow has member: %+v", entry)
			}
		}
	}
	other, err := f.service.ListRechargeQuotaEntries(ctx, f.otherBrand, 10, 0)
	if err != nil || len(other) != 0 {
		t.Fatalf("cross-brand entries=%+v err=%v", other, err)
	}
}
