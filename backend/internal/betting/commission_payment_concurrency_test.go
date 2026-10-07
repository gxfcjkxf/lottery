package betting

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

func TestCommissionPaymentConcurrentWorkersWalletContentionAndDeferredStep(t *testing.T) {
	f := newCommissionBatchFixture(t)
	ctx := context.Background()
	p := prepareManualCommissionPayment(t, f)
	a := commissionPaymentActor(f)
	if err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, err := f.service.ApprovePaymentTx(ctx, tx, f.betting.brand, p.ID, a, commission.RetryCycleInput{Version: p.Version, Reason: "authorize concurrency verification"}, commissionPaymentMeta(a))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	p = commissionPaymentRead(t, f, p.ID)

	// Even a real ledger and real target audit are not enough without the job
	// step. This rejected commit must roll back all financial projections.
	tx, err := f.betting.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var earning string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM commission_earnings WHERE run_id=$1`, p.RunID).Scan(&earning); err != nil {
		t.Fatal(err)
	}
	target := ids.New()
	_, err = tx.Exec(ctx, `INSERT INTO commission_payment_targets(id,brand_id,payment_id,earning_id,points,member_id,agent_id,payment_version) VALUES($1,$2,$3,$4,1,$5,$6,$7)`, target, f.betting.brand, p.ID, earning, f.node.MemberID, f.node.ID, p.Version)
	if err != nil {
		t.Fatal(err)
	}
	var delta points.Balance
	delta[3][0] = 1
	entry, err := (points.Store{DB: f.betting.db}).Post(ctx, tx, points.Change{BrandID: f.betting.brand, MemberID: f.node.MemberID, EntryType: "commission", ReferenceType: "commission_payment_target", ReferenceID: target, OperationKey: "commission-payment:" + target, Reason: "attempt posting without a committed payment step", ActorType: "system", RequestID: ids.New(), Delta: delta, Allocation: []points.Allocation{{Source: "commission", State: "available", Points: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	log, err := audit.Append(ctx, tx, audit.Record{BrandID: f.betting.brand, ActorType: "system", Action: "commission.payment.target", ResourceType: "commission_payment_target", ResourceID: target, Reason: "target witness without payment step", RequestID: ids.New(), After: map[string]any{"points": "1", "ledger_entry_id": entry.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE commission_payment_targets SET state='paid',ledger_entry_id=$2,audit_log_id=$3,paid_at=clock_timestamp() WHERE id=$1`, target, entry.ID, log); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err == nil {
		t.Fatal("orphan payment target committed without payment version step")
	}
	w, err := (points.Store{DB: f.betting.db}).Read(ctx, f.betting.brand, f.node.MemberID)
	if err != nil || w.CommissionPoints != 0 {
		t.Fatal("orphan step changed wallet", w, err)
	}

	// Settlement's opposite wallet -> epoch lock order must not deadlock this
	// worker or misclassify ordinary wallet contention as a failed payout.
	lock, err := f.betting.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(ctx)
	var account string
	if err = lock.QueryRow(ctx, `SELECT id::text FROM point_accounts WHERE brand_id=$1 AND brand_member_id=$2 FOR UPDATE`, f.betting.brand, f.node.MemberID).Scan(&account); err != nil {
		t.Fatal(err)
	}
	if steps, e := f.service.ProcessPayments(ctx, 1); e != nil || steps != 0 {
		t.Fatalf("wallet busy became failure: steps=%d error=%v", steps, e)
	}
	if got := commissionPaymentRead(t, f, p.ID); got.State != "paying" || got.PaidCount != "0" {
		t.Fatal("busy payment changed financial state", got)
	}
	if err = lock.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = f.betting.db.Exec(ctx, `UPDATE commission_payments SET next_work_at=clock_timestamp() WHERE id=$1`, p.ID); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := f.service.ProcessPayments(ctx, 20)
			if e != nil && !errors.Is(e, commission.ErrBusy) {
				failures <- e
			}
		}()
	}
	wg.Wait()
	close(failures)
	for e := range failures {
		t.Fatal(e)
	}
	if _, err = f.service.ProcessPayments(ctx, 20); err != nil {
		t.Fatal(err)
	}
	got := commissionPaymentRead(t, f, p.ID)
	if got.State != "paid" || got.PaidCount != "1" || got.PaidPoints != "1" {
		t.Fatal("concurrent payment did not finish once", got)
	}
	var entries int
	if err = f.betting.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='commission'`).Scan(&entries); err != nil || entries != 1 {
		t.Fatal("duplicate credit", entries, err)
	}
	w, err = (points.Store{DB: f.betting.db}).Read(ctx, f.betting.brand, f.node.MemberID)
	if err != nil || w.CommissionPoints != 1 {
		t.Fatal("wrong beneficiary credit", w, err)
	}
}
