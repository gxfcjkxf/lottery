package betting

import (
	"context"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/commission"
)

func TestCommissionCorrectionPlanPositiveUnpaidDifferenceUsesRealRoundedEarnings(t *testing.T) {
	t.Parallel()
	f := newAutomaticCommissionBatchFixture(t)
	f, child := addActualChildAgentBeneficiary(t, f, 10)
	ctx := context.Background()
	waitCommissionBoundary(t, f.boundary)
	cycle := createCommissionCycle(t, f)
	eligibilitySettle(t, f.betting)
	advanceCommissionWorker(t, f, 40)
	cycle = readCommissionCycle(t, f, cycle.ID)
	if cycle.State != "ready" || cycle.TotalPoints != "4" {
		t.Fatalf("real parent/child rounded earnings: %+v", cycle)
	}
	gate := commissionPaymentPolicy(t, f)
	if err := updateCommissionPaymentPolicy(t, f, commissionPaymentActor(f), true, gate.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessPayments(ctx, 1); err != nil {
		t.Fatal(err)
	}
	payment := commissionPayments(t, f).Items[0]
	if _, err := f.service.ProcessPayments(ctx, 1); err != nil {
		t.Fatal(err)
	}
	payment = commissionPaymentRead(t, f, payment.ID)
	if payment.PaidCount != "1" || payment.PaidPoints != "3" || payment.State != "paying" {
		t.Fatalf("original actual partial credit: %+v", payment)
	}
	// A different real draw that leaves all purchases losing creates a new
	// calculation generation. The unpaid child still deserves its rounded 1.
	c := createFixtureCorrection(t, f.betting, correctedDigits(1, 2, 2))
	if _, err := f.betting.service.ProcessCorrections(ctx, 20); err != nil {
		t.Fatal(err)
	}
	var err error
	c, err = f.betting.service.Correction(ctx, f.betting.brand, c.ID)
	if err != nil || c.NewJobID == nil {
		t.Fatalf("real new settlement: %+v %v", c, err)
	}
	if _, err = f.betting.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	advanceCommissionWorker(t, f, 40)
	if _, err = f.service.ProcessPayments(ctx, 20); err != nil {
		t.Fatal(err)
	}
	before := correctionPlanSnapshot(t, f, payment.ID)
	if _, err = f.service.ProcessCorrectionPlans(ctx, 20); err != nil {
		t.Fatal(err)
	}
	plan := correctionPlansRead(t, f).Items[0]
	if plan.State != "ready" || plan.BeforePoints != "3" || plan.CalculatedPoints != "4" || plan.CreditPoints == nil || *plan.CreditPoints != "1" || plan.DebitPoints == nil || *plan.DebitPoints != "0" || plan.NetPoints == nil || *plan.NetPoints != "1" {
		t.Fatalf("positive correction plan cannot resend original 3: %+v", plan)
	}
	page := correctionPlanTargetsRead(t, f, plan.ID)
	var target *commission.CorrectionPlanTarget
	for i := range page.Items {
		if page.Items[i].AgentID == child {
			target = &page.Items[i]
		}
	}
	if page.TotalCount != "2" || target == nil || target.OriginalTargetID != nil || target.AdjustmentVersion != nil || target.PointsBefore != 0 || target.PointsAfter != 1 || target.DeltaPoints != 1 {
		t.Fatalf("unpaid beneficiary was omitted or treated as already paid: %+v", page)
	}
	if after := correctionPlanSnapshot(t, f, payment.ID); after != before {
		t.Fatal("positive difference planning changed financial facts")
	}
}
