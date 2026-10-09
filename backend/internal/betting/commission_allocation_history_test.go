package betting

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

func runAllocationRead(t *testing.T, f commissionBatchFixture, cycle, run string, q commission.AllocationQuery) (commission.AllocationPage, error) {
	t.Helper()
	ctx := context.Background()
	tx, err := f.betting.db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	return f.service.AllocationsTx(ctx, tx, f.betting.brand, cycle, run, q)
}

func runEarningRead(t *testing.T, f commissionBatchFixture, cycle, run string) commission.RunEarningPage {
	t.Helper()
	ctx := context.Background()
	tx, err := f.betting.db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	out, err := f.service.EarningsForRunTx(ctx, tx, f.betting.brand, cycle, run, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCommissionAllocationHistoryUsesRealMultilevelSavedRatesAndExactPages(t *testing.T) {
	f := newCommissionBatchFixture(t)
	f, child := addActualChildAgentBeneficiary(t, f)
	waitCommissionBoundary(t, f.boundary)
	cycle := createCommissionCycle(t, f)
	eligibilitySettle(t, f.betting)
	advanceCommissionWorker(t, f, 30)
	cycle = readCommissionCycle(t, f, cycle.ID)
	if cycle.State != "ready" || cycle.CurrentRunID == nil {
		t.Fatal("real cycle not ready", cycle)
	}
	run := *cycle.CurrentRunID
	// Today's root configuration changes after the bet's immutable capture.
	agents := agency.Service{DB: f.betting.db}
	policy, err := agents.Policy(context.Background(), f.betting.brand)
	if err != nil {
		t.Fatal(err)
	}
	bettingTx(t, f.betting.db, func(tx pgx.Tx) error {
		cfg := f.node.Config
		cfg.Ratio = "0.2"
		cfg.Status = "disabled"
		_, e := agents.Update(context.Background(), tx, f.betting.brand, f.actor, f.node.ID, agency.UpdateInput{Version: f.node.Version, PolicyVersion: policy.Version, Config: cfg, Reason: "current node change must not alter saved allocation history"}, points.Metadata{ActorType: "admin", ActorID: f.actor.ID, RequestID: ids.New()})
		return e
	})
	before := commissionReportFingerprint(t, f)
	all, err := runAllocationRead(t, f, cycle.ID, run, commission.AllocationQuery{Limit: 100})
	if err != nil || all.TotalCount != "4" || len(all.Items) != 4 {
		t.Fatal("actual allocations", all, err)
	}
	for _, item := range all.Items {
		if item.RunID != run || item.Mode != "loss" || item.BasePoints != 1 || item.MemberID == item.BettorMemberID {
			t.Fatal("wrong saved identity/base", item)
		}
		if item.AgentID == f.node.ID && item.AgentRatio != "0.3" {
			t.Fatal("today's ratio replaced saved rate", item)
		}
	}
	for i := 0; i < 4; i++ {
		page, e := runAllocationRead(t, f, cycle.ID, run, commission.AllocationQuery{Limit: 1, Offset: i})
		if e != nil || page.TotalCount != "4" || len(page.Items) != 1 || page.Items[0] != all.Items[i] {
			t.Fatal("stable exact pagination", page, e)
		}
	}
	root := f.node.ID
	rootPage, err := runAllocationRead(t, f, cycle.ID, run, commission.AllocationQuery{Limit: 100, AgentID: &root})
	if err != nil || rootPage.TotalCount != "3" || len(rootPage.Items) != 3 {
		t.Fatal(rootPage, err)
	}
	childPage, err := runAllocationRead(t, f, cycle.ID, run, commission.AllocationQuery{Limit: 100, AgentID: &child})
	if err != nil || childPage.TotalCount != "1" || len(childPage.Items) != 1 || childPage.Items[0].DifferenceRatio != "0.1" || childPage.Items[0].ExactAmount != (commission.ExactAmount{Numerator: "1", Denominator: "10"}) {
		t.Fatal(childPage, err)
	}
	childOrder := f.orders[len(f.orders)-1].ID
	orderPage, err := runAllocationRead(t, f, cycle.ID, run, commission.AllocationQuery{Limit: 100, OrderID: &childOrder})
	if err != nil || orderPage.TotalCount != "2" {
		t.Fatal(orderPage, err)
	}
	cancelled := f.orders[2].ID
	empty, err := runAllocationRead(t, f, cycle.ID, run, commission.AllocationQuery{Limit: 100, OrderID: &cancelled})
	if err != nil || empty.TotalCount != "0" || empty.Items == nil || len(empty.Items) != 0 {
		t.Fatal("cancelled order must have no allocation", empty, err)
	}
	missing := ids.New()
	for _, q := range []commission.AllocationQuery{{Limit: 20, AgentID: &missing}, {Limit: 20, OrderID: &missing}} {
		if _, e := runAllocationRead(t, f, cycle.ID, run, q); !errors.Is(e, commission.ErrNotFound) {
			t.Fatal("unknown filter did not reject", e)
		}
	}
	earnings := runEarningRead(t, f, cycle.ID, run)
	if earnings.RunID != run || earnings.TotalCount != "2" || len(earnings.Items) != 2 {
		t.Fatal(earnings)
	}
	for _, earning := range earnings.Items {
		if earning.AgentID == root && (earning.ExactAmount != (commission.ExactAmount{Numerator: "4", Denominator: "5"}) || earning.Points != 1) {
			t.Fatal("whole-cycle rounding lost", earning)
		}
		if earning.AgentID == child && earning.Points != 0 {
			t.Fatal("per-order rounding produced a child point", earning)
		}
	}
	encoded, _ := json.Marshal(all)
	for _, private := range []string{"rule_snapshot", "agent_path", "account_id", "financial_policy"} {
		if strings.Contains(string(encoded), private) {
			t.Fatal("private evidence leaked", private)
		}
	}
	if after := commissionReportFingerprint(t, f); after != before {
		t.Fatal("pure history reads changed financial, audit or event evidence")
	}
}

func TestCommissionAllocationHistoryRetainsOldAndReplacementRunsWithoutPayment(t *testing.T) {
	f, payment, _ := createAndPayAutomaticCorrectionFixture(t)
	old, err := runAllocationRead(t, f, payment.CycleID, payment.RunID, commission.AllocationQuery{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	oldEarnings := runEarningRead(t, f, payment.CycleID, payment.RunID)
	correctCommissionRunToZero(t, f, payment.CycleID, payment.RunID)
	current := readCommissionCycle(t, f, payment.CycleID)
	if current.CurrentRunID == nil || *current.CurrentRunID == payment.RunID {
		t.Fatal("replacement generation not created", current)
	}
	before := commissionReportFingerprint(t, f)
	again, err := runAllocationRead(t, f, payment.CycleID, payment.RunID, commission.AllocationQuery{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	oldJSON, _ := json.Marshal(old)
	againJSON, _ := json.Marshal(again)
	if string(oldJSON) != string(againJSON) {
		t.Fatal("historical allocation changed with replacement run")
	}
	againEarnings := runEarningRead(t, f, payment.CycleID, payment.RunID)
	oldEarningJSON, _ := json.Marshal(oldEarnings)
	againEarningJSON, _ := json.Marshal(againEarnings)
	if string(oldEarningJSON) != string(againEarningJSON) {
		t.Fatal("historical earnings silently substituted current run")
	}
	newPage, err := runAllocationRead(t, f, payment.CycleID, *current.CurrentRunID, commission.AllocationQuery{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if newPage.TotalCount != old.TotalCount {
		t.Fatal("replacement zero allocations disappeared", newPage)
	}
	for _, item := range newPage.Items {
		if item.BasePoints != 0 || item.ExactAmount != (commission.ExactAmount{Numerator: "0", Denominator: "1"}) {
			t.Fatal("new final generation not reflected", item)
		}
	}
	if after := commissionReportFingerprint(t, f); after != before {
		t.Fatal("history inspection moved points or changed original evidence")
	}
}
