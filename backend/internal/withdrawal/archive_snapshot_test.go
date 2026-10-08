package withdrawal

import (
	"context"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/reporting"
)

func TestArchiveSnapshotWithdrawalsMatchesPaidAndRejectedOrders(t *testing.T) {
	f := newOrderIntegrationFixture(t, true, true)
	ctx := context.Background()
	policyService := Service{DB: f.db}
	policy, err := policyService.BrandPolicy(ctx, orderTestBrand)
	if err != nil {
		t.Fatal(err)
	}
	policyConfig := policy.Config
	policyConfig.AllowedSources = []string{"recharge", "winning", "gift", "commission"}
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = policyService.UpdateBrand(ctx, tx, orderTestBrand, f.admin,
		BrandInput{Version: policy.Version, Config: policyConfig, Reason: "authorize four-source archive fixture"},
		points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	allocation := []points.Allocation{
		{Source: "recharge", State: "available", Points: 10},
		{Source: "winning", State: "available", Points: 10},
		{Source: "gift", State: "available", Points: 10},
		{Source: "commission", State: "available", Points: 10},
	}
	f.fund(t, allocation)
	paid := createOrder(t, f, 40, "archive-paid-four-source", allocation)
	meta := points.Metadata{ActorType: "admin", ActorID: f.adminID, RequestID: ids.New()}
	if _, err := orderAction(t, f, orderID(t, paid), "approve", "archive-paid-approve", f.admin,
		ActionInput{Version: 1, ClientKey: "archive-paid-approve", Reason: "review actual withdrawal"}, meta); err != nil {
		t.Fatal(err)
	}
	if got, err := orderAction(t, f, orderID(t, paid), "mark_paid", "archive-paid-settle", f.admin,
		ActionInput{Version: 2, ClientKey: "archive-paid-settle", Reason: "record actual payment"}, meta); err != nil || got.State != "paid" {
		t.Fatalf("actual withdrawal payment=%+v err=%v", got, err)
	}

	f.fund(t, allocation)
	rejected := createOrder(t, f, 40, "archive-rejected-four-source", allocation)
	if got, err := orderAction(t, f, orderID(t, rejected), "reject", "archive-reject", f.admin,
		ActionInput{Version: 1, ClientKey: "archive-reject", Reason: "reject actual withdrawal"}, meta); err != nil || got.State != "rejected" {
		t.Fatalf("actual withdrawal rejection=%+v err=%v", got, err)
	}

	from, to := paid.CreatedAt.UTC().Add(-time.Hour), rejected.CreatedAt.UTC().Add(time.Hour)
	reports := reporting.Service{DB: f.db}
	query := reporting.Query{From: from, To: to, GroupBy: "state", Limit: 20}
	withdrawalReport, err := reports.Withdrawal(ctx, orderTestBrand, query)
	if err != nil {
		t.Fatal(err)
	}
	ledgerQuery := reporting.Query{From: from, To: to, GroupBy: "entry_type", Limit: 20}
	ledgerReport, err := reports.Ledger(ctx, orderTestBrand, ledgerQuery)
	if err != nil {
		t.Fatal(err)
	}
	tx, err = f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := reports.CaptureArchive(ctx, tx, orderTestBrand, from, to)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if snapshot.Withdrawals != withdrawalReport.Summary || snapshot.Ledger != ledgerReport.Summary {
		t.Fatalf("archive differs from live reports: withdrawals=%+v/%+v ledger=%+v/%+v", snapshot.Withdrawals, withdrawalReport.Summary, snapshot.Ledger, ledgerReport.Summary)
	}
	if snapshot.Withdrawals.OrderCount != "2" || snapshot.Withdrawals.PaidCount != "1" || snapshot.Withdrawals.RejectedCount != "1" || snapshot.Withdrawals.PaidPoints != "40" || snapshot.Withdrawals.RejectedPoints != "40" {
		t.Fatalf("archive omitted actual paid/rejected withdrawal states: %+v", snapshot.Withdrawals)
	}
	var paidEntry *reporting.Group[reporting.LedgerTotals]
	for i := range ledgerReport.Items {
		if ledgerReport.Items[i].Key == "withdrawal_paid" {
			paidEntry = &ledgerReport.Items[i]
			break
		}
	}
	if paidEntry == nil || paidEntry.Totals.NetPoints != "-40" {
		t.Fatalf("ledger report did not retain the actual negative payment posting: %+v", ledgerReport.Items)
	}
	if !snapshot.To.Equal(to) || !snapshot.SnapshotAt.Before(snapshot.To) {
		t.Fatalf("future half-open bound was not captured as a present snapshot: snapshot_at=%s to=%s", snapshot.SnapshotAt, snapshot.To)
	}
}
