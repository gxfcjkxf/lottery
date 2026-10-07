package betting

import (
	"context"
	"errors"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

func TestCommissionPaymentMixedOriginalModesRemainBlocked(t *testing.T) {
	f := newCommissionBatchFixture(t)
	ctx := context.Background()
	policy, err := f.service.Policy(ctx, f.betting.brand)
	if err != nil {
		t.Fatal(err)
	}
	config := policy.Config
	config.PayoutMode = commission.PayoutAutomatic
	if err = commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, e := f.service.Update(ctx, tx, f.betting.brand, f.actor, commission.PolicyInput{Version: policy.Version, Config: config, Reason: "capture different mode only on a new genuine bet"}, commissionPaymentMeta(f.actor))
		return e
	}); err != nil {
		t.Fatal(err)
	}
	input := f.betting.input
	input.Selection = rules.Selection{Digits: [][]int{{0}, {0}, {0}}}
	order, err := placeBettingOrder(t, f.betting, input, "commission-mixed-new-automatic-order")
	if err != nil {
		t.Fatal(err)
	}
	if !order.PlacedAt.Before(f.boundary) {
		t.Fatal("new automatic bet missed original cycle boundary")
	}
	waitCommissionBoundary(t, f.boundary)
	cycle := createCommissionCycle(t, f)
	eligibilitySettle(t, f.betting)
	advanceCommissionWorker(t, f, 30)
	cycle = readCommissionCycle(t, f, cycle.ID)
	if cycle.State != "ready" || cycle.TotalPoints != "1" {
		t.Fatal("mixed-mode genuine bets did not finalize", cycle)
	}
	actor := commissionPaymentActor(f)
	gate := commissionPaymentPolicy(t, f)
	if err = updateCommissionPaymentPolicy(t, f, actor, true, gate.Version); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.ProcessPayments(ctx, 20); err != nil {
		t.Fatal(err)
	}
	page := commissionPayments(t, f)
	if len(page.Items) != 1 {
		t.Fatal("mixed cycle did not register exactly one decision", page)
	}
	p := page.Items[0]
	if p.State != "blocked" || p.PayoutMode != "mixed" || p.PaidCount != "0" || p.PaidPoints != "0" || p.LastErrorCode == nil || *p.LastErrorCode != "COMMISSION_PAYMENT_MODE_UNRESOLVED" {
		t.Fatal("unconfirmed mixed mode was defaulted", p)
	}
	for _, action := range []string{"approve", "retry"} {
		err = commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
			var e error
			in := commission.RetryCycleInput{Version: p.Version, Reason: "cannot bypass unconfirmed mixed mode"}
			if action == "approve" {
				_, e = f.service.ApprovePaymentTx(ctx, tx, f.betting.brand, p.ID, actor, in, commissionPaymentMeta(actor))
			} else {
				_, e = f.service.RetryPaymentTx(ctx, tx, f.betting.brand, p.ID, actor, in, commissionPaymentMeta(actor))
			}
			return e
		})
		if !errors.Is(err, commission.ErrPaymentState) {
			t.Fatalf("mixed-mode %s=%v", action, err)
		}
	}
	var credits int
	if err = f.betting.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='commission'`).Scan(&credits); err != nil || credits != 0 {
		t.Fatal("mixed decision minted points", credits, err)
	}
	wallet, err := (points.Store{DB: f.betting.db}).Read(ctx, f.betting.brand, f.node.MemberID)
	if err != nil || wallet.CommissionPoints != 0 {
		t.Fatal("mixed decision mutated beneficiary wallet", wallet, err)
	}
}
