package betting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/attribution"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

type commissionBatchFixture struct {
	betting  bettingFixture
	service  commission.Service
	actor    access.Account
	node     agency.Node
	orders   []Order
	boundary time.Time
}

func newCommissionBatchFixture(t *testing.T) commissionBatchFixture {
	t.Helper()
	return newCommissionBatchFixtureWithWindow(t, 20*time.Second, 22*time.Second)
}

func newCommissionBatchFixtureWithWindow(t *testing.T, betWindow, drawWindow time.Duration) commissionBatchFixture {
	t.Helper()
	f := newBettingFixtureWithWindow(t, storeTestBrand, betWindow, drawWindow)
	ctx := context.Background()
	adminID := f.version.CreatedBy
	actor := access.Account{
		ID: adminID, Type: access.AccountAdmin, BrandIDs: []string{f.brand},
		Roles: []access.Role{{BrandID: f.brand, Permissions: []access.Permission{
			{Resource: "agent_policy", Action: "write", Scope: access.ScopeBrand},
			{Resource: "agent", Action: "write", Scope: access.ScopeBrand},
			{Resource: "join_code", Action: "write", Scope: access.ScopeBrand},
			{Resource: "commission_policy", Action: "write", Scope: access.ScopeBrand},
			{Resource: "commission", Action: "view", Scope: access.ScopeBrand},
			{Resource: "commission", Action: "run", Scope: access.ScopeBrand},
			{Resource: "commission", Action: "retry", Scope: access.ScopeBrand},
		}}},
	}
	agents := agency.Service{DB: f.db}
	financial := commission.Service{DB: f.db}
	codes := attribution.Service{DB: f.db}
	var policy agency.Policy
	var root agency.Node
	var code attribution.Code
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var err error
		policy, err = agents.SavePolicy(ctx, tx, f.brand, actor, agency.PolicyInput{
			Version: 1,
			Config:  agency.PolicyConfig{Enabled: true, MaxDepth: 3, RatioCap: "0.3", Mode: "loss", Cycle: "weekly"},
			Reason:  "commission batch integration policy",
		}, points.Metadata{RequestID: ids.New()})
		return err
	})
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var err error
		root, err = agents.Create(ctx, tx, f.brand, actor, agency.CreateInput{
			PolicyVersion: policy.Version, MemberID: f.member,
			Config: agency.NodeConfig{Ratio: "0.3", Status: "active", CanCreateChildren: true},
			Reason: "commission batch root beneficiary",
		}, points.Metadata{RequestID: ids.New()})
		return err
	})
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var err error
		code, err = codes.Create(ctx, tx, f.brand, actor, attribution.CreateInput{
			Kind: "agent", OwnerMemberID: f.member, AgentID: &root.ID, Reason: "commission batch actual registration code",
		}, points.Metadata{RequestID: ids.New()})
		return err
	})
	users, err := identity.New(f.db)
	if err != nil {
		t.Fatal(err)
	}
	var session identity.Authentication
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		out, err := users.Register(ctx, tx, f.brand, identity.RegisterInput{
			Username: "bet_batch_" + strings.ReplaceAll(ids.New(), "-", "")[:12],
			Password: "test-commission-batch-password", Privacy: "dev-1", Terms: "dev-1", AgentCode: code.Code,
		}, identity.Metadata{Domain: "aurora.localhost"})
		if err == nil {
			if out.Status != 201 {
				t.Fatalf("commission batch register status=%d error=%+v", out.Status, out.Error)
			}
			err = json.Unmarshal(out.Data, &session)
		}
		return err
	})
	f.user, err = users.Authenticate(ctx, f.brand, session.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	f.member, f.token = session.Member.ID, session.AccessToken

	boundary := f.period.BetEndAt.UTC().Truncate(time.Second).Add(-3 * time.Second)
	weekday := int(boundary.Weekday())
	calendar := commission.Calendar{Timezone: "UTC", Cycle: "weekly", BoundaryTime: boundary.Format("15:04:05"), Weekday: &weekday}
	current, err := financial.Policy(ctx, f.brand)
	if err != nil {
		t.Fatal(err)
	}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var updateErr error
		_, updateErr = financial.Update(ctx, tx, f.brand, actor, commission.PolicyInput{
			Version: current.Version,
			Config:  commission.PolicyConfig{Enabled: true, Calendar: &calendar, PayoutMode: commission.PayoutManual},
			Reason:  "enable real weekly commission batch boundary",
		}, points.Metadata{ActorType: "admin", ActorID: actor.ID, RequestID: ids.New()})
		return updateErr
	})
	fundBettingWallet(t, f, 10)
	in := f.input
	in.Selection = rules.Selection{Digits: [][]int{{0}, {0}, {0}}}
	first, err := placeBettingOrder(t, f, in, "commission-batch-lost-one")
	if err != nil {
		t.Fatal(err)
	}
	second, err := placeBettingOrder(t, f, in, "commission-batch-lost-two")
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := placeBettingOrder(t, f, in, "commission-batch-cancelled")
	if err != nil {
		t.Fatal(err)
	}
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		_, cancelErr := f.service.CancelAdmin(ctx, tx, f.brand, judgeActor(f), cancelled.ID, cancelled.Version, "cancelled stake excluded from commission", policyMeta(actor.ID))
		return cancelErr
	})
	for _, order := range []Order{first, second, cancelled} {
		if !order.PlacedAt.Before(boundary) {
			t.Fatalf("order %s was placed at %s, not before weekly boundary %s", order.ID, order.PlacedAt, boundary)
		}
	}
	return commissionBatchFixture{betting: f, service: financial, actor: actor, node: root, orders: []Order{first, second, cancelled}, boundary: boundary}
}

func waitCommissionBoundary(t *testing.T, boundary time.Time) {
	t.Helper()
	if wait := time.Until(boundary); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		<-timer.C
	}
	for time.Now().Before(boundary) {
		time.Sleep(time.Millisecond)
	}
}

func commissionBatchCallTx(t *testing.T, f commissionBatchFixture, call func(pgx.Tx) error) error {
	t.Helper()
	tx, err := f.betting.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if err = call(tx); err != nil {
		return err
	}
	return tx.Commit(context.Background())
}

func readCommissionCycle(t *testing.T, f commissionBatchFixture, id string) commission.Cycle {
	t.Helper()
	tx, err := f.betting.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	cycle, err := f.service.CycleTx(context.Background(), tx, f.betting.brand, id)
	if err != nil {
		t.Fatal(err)
	}
	return cycle
}

func advanceCommissionWorker(t *testing.T, f commissionBatchFixture, steps int) {
	t.Helper()
	if _, err := f.betting.db.Exec(context.Background(), `UPDATE commission_cycles SET next_work_at=clock_timestamp() WHERE brand_id=$1`, f.betting.brand); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessCycles(context.Background(), steps); err != nil {
		t.Fatal(err)
	}
}

func createCommissionCycle(t *testing.T, f commissionBatchFixture) commission.Cycle {
	t.Helper()
	var cycle commission.Cycle
	err := commissionBatchCallTx(t, f, func(tx pgx.Tx) error {
		var err error
		cycle, err = f.service.CreateCycleTx(context.Background(), tx, f.betting.brand, f.actor,
			commission.CreateCycleInput{AnchorOrderID: f.orders[0].ID, Reason: "close actual weekly commission window"},
			points.Metadata{ActorType: "admin", ActorID: f.actor.ID, RequestID: ids.New()})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return cycle
}

func commissionBatchPersistence(t *testing.T, f commissionBatchFixture, cycleID string) (string, string) {
	t.Helper()
	var calculations, allocations string
	err := f.betting.db.QueryRow(context.Background(), `SELECT
	 coalesce(jsonb_agg(to_jsonb(c) ORDER BY c.order_id)::text,'[]'),
	 coalesce((SELECT jsonb_agg(to_jsonb(a) ORDER BY a.order_id,a.agent_id)::text FROM commission_allocations a WHERE a.cycle_id=$1),'[]')
	 FROM commission_calculations c WHERE c.cycle_id=$1`, cycleID).Scan(&calculations, &allocations)
	if err != nil {
		t.Fatal(err)
	}
	return calculations, allocations
}

func TestCommissionBatchRealWindowWaitsThenCalculatesAndInvalidatesOnCorrection(t *testing.T) {
	f := newCommissionBatchFixture(t)
	ctx := context.Background()
	// The policy is captured at placement. Updating the mutable agent after the
	// wagers cannot alter the saved 0.3 loss allocation.
	disabled := agency.NodeConfig{Ratio: "0.2", Status: "disabled", CanCreateChildren: false}
	agents := agency.Service{DB: f.betting.db}
	policy, err := agents.Policy(ctx, f.betting.brand)
	if err != nil {
		t.Fatal(err)
	}
	if err = commissionBatchCallTx(t, f, func(tx pgx.Tx) error {
		var updateErr error
		_, updateErr = agents.Update(ctx, tx, f.betting.brand, f.actor, f.node.ID, agency.UpdateInput{
			Version: f.node.Version, PolicyVersion: policy.Version, Config: disabled,
			Reason: "later live agent state must not rewrite placed wagers",
		}, points.Metadata{RequestID: ids.New()})
		return updateErr
	}); err != nil {
		t.Fatal(err)
	}
	waitCommissionBoundary(t, f.boundary)
	cycle := createCommissionCycle(t, f)
	viewer := f.actor
	viewer.Roles = []access.Role{{BrandID: f.betting.brand, Permissions: []access.Permission{{Resource: "commission", Action: "view", Scope: access.ScopeBrand}}}}
	err = commissionBatchCallTx(t, f, func(tx pgx.Tx) error {
		_, callErr := f.service.CreateCycleTx(ctx, tx, f.betting.brand, viewer,
			commission.CreateCycleInput{AnchorOrderID: f.orders[0].ID, Reason: "viewer cannot close another cycle"},
			points.Metadata{ActorType: "admin", ActorID: viewer.ID, RequestID: ids.New()})
		return callErr
	})
	if !errors.Is(err, commission.ErrDenied) {
		t.Fatalf("view-only commission actor create error=%v, want denied", err)
	}
	super := f.actor
	super.SuperAdmin = true
	err = commissionBatchCallTx(t, f, func(tx pgx.Tx) error {
		_, callErr := f.service.CreateCycleTx(ctx, tx, f.betting.brand, super,
			commission.CreateCycleInput{AnchorOrderID: f.orders[0].ID, Reason: "superadmin mutation denied"},
			points.Metadata{ActorType: "admin", ActorID: super.ID, RequestID: ids.New()})
		return callErr
	})
	if !errors.Is(err, commission.ErrDenied) {
		t.Fatalf("superadmin commission create error=%v, want denied", err)
	}

	advanceCommissionWorker(t, f, 10)
	cycle = readCommissionCycle(t, f, cycle.ID)
	if cycle.State != "waiting" || cycle.TargetCount != "3" || !cycle.ScanComplete || cycle.CurrentRunID != nil || cycle.CalculatedCount != "0" || cycle.EarningCount != "0" {
		t.Fatalf("cycle with three unsettled targets=%+v", cycle)
	}
	if got := walletBySource(t, f.betting); got[0][0] != 8 {
		t.Fatalf("unexpected wallet change before settlement/commission work: %+v", got)
	}
	var commissionsBefore int
	if err = f.betting.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='commission'`).Scan(&commissionsBefore); err != nil || commissionsBefore != 0 {
		t.Fatalf("commission ledger before settlement=%d err=%v", commissionsBefore, err)
	}

	eligibilitySettle(t, f.betting)
	advanceCommissionWorker(t, f, 20)
	cycle = readCommissionCycle(t, f, cycle.ID)
	if cycle.State != "ready" || !cycle.EvidenceCurrent || cycle.TargetCount != "3" || cycle.CalculatedCount != "3" || cycle.EarningCount != "1" || cycle.TotalPoints != "1" || cycle.CurrentGeneration == nil || *cycle.CurrentGeneration != "1" {
		t.Fatalf("completed commission cycle=%+v", cycle)
	}
	var calcCount, allocationCount int
	var snapshot []byte
	if err = f.betting.db.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE reason='eligible'),
	 (SELECT rule_snapshot FROM commission_calculations WHERE cycle_id=$1 AND order_id=$2)
	 FROM commission_calculations WHERE cycle_id=$1`, cycle.ID, f.orders[0].ID).Scan(&calcCount, &allocationCount, &snapshot); err != nil {
		t.Fatal(err)
	}
	if calcCount != 3 || allocationCount != 2 {
		t.Fatalf("persisted calculations=%d eligible=%d; want 3 and 2", calcCount, allocationCount)
	}
	if !bytes.Contains(snapshot, []byte(`"ratio": "0.3"`)) || bytes.Contains(snapshot, []byte(`"ratio": "0.2"`)) {
		t.Fatalf("calculation did not preserve placement snapshot: %s", snapshot)
	}
	var exactNumerator, exactDenominator string
	var earned points.Amount
	if err = f.betting.db.QueryRow(ctx, `SELECT numerator,denominator,points FROM commission_earnings WHERE cycle_id=$1 AND run_id=$2`, cycle.ID, *cycle.CurrentRunID).Scan(&exactNumerator, &exactDenominator, &earned); err != nil {
		t.Fatal(err)
	}
	if exactNumerator != "3" || exactDenominator != "5" || earned != 1 {
		t.Fatalf("aggregate exact=%s/%s rounded=%d; want 3/5 rounded once to 1", exactNumerator, exactDenominator, earned)
	}
	var beneficiary string
	if err = f.betting.db.QueryRow(ctx, `SELECT member_id::text FROM commission_earnings WHERE cycle_id=$1 AND run_id=$2`, cycle.ID, *cycle.CurrentRunID).Scan(&beneficiary); err != nil || beneficiary != f.node.MemberID {
		t.Fatalf("earning beneficiary=%s want root member %s err=%v", beneficiary, f.node.MemberID, err)
	}
	var cancelledReason string
	if err = f.betting.db.QueryRow(ctx, `SELECT reason FROM commission_calculations WHERE cycle_id=$1 AND order_id=$2`, cycle.ID, f.orders[2].ID).Scan(&cancelledReason); err != nil || cancelledReason != "cancelled" {
		t.Fatalf("cancelled calculation reason=%q err=%v", cancelledReason, err)
	}
	cycleJSON, err := json.Marshal(cycle)
	if err != nil || bytes.Contains(cycleJSON, []byte("rule_snapshot")) || bytes.Contains(cycleJSON, []byte("cursor")) {
		t.Fatalf("cycle DTO exposed internal calculation evidence: %s err=%v", cycleJSON, err)
	}
	tx, err := f.betting.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cycles, listErr := f.service.CyclesTx(ctx, tx, f.betting.brand, 100, 0)
	if listErr != nil || len(cycles.Items) != 1 || cycles.Items[0].ID != cycle.ID {
		_ = tx.Rollback(ctx)
		t.Fatalf("brand-scoped cycle list=%+v err=%v", cycles, listErr)
	}
	earnings, readErr := f.service.EarningsTx(ctx, tx, f.betting.brand, cycle.ID, 100, 0)
	_ = tx.Rollback(ctx)
	if readErr != nil || len(earnings.Items) != 1 {
		t.Fatalf("public earnings read=%+v err=%v", earnings, readErr)
	}
	earningsJSON, err := json.Marshal(earnings)
	if err != nil || bytes.Contains(earningsJSON, []byte("rule_snapshot")) || bytes.Contains(earningsJSON, []byte("attribution")) {
		t.Fatalf("earnings DTO exposed private evidence: %s err=%v", earningsJSON, err)
	}
	if processed, processErr := f.service.ProcessCycles(ctx, 20); processErr != nil || processed != 0 {
		t.Fatalf("ready cycle rerun processed=%d err=%v", processed, processErr)
	}
	if calc, alloc := commissionBatchPersistence(t, f, cycle.ID); calc == "" || alloc == "" {
		t.Fatal("ready cycle has no persisted calculations/allocations")
	}
	beforeCalculations, beforeAllocations := commissionBatchPersistence(t, f, cycle.ID)
	beforeWallet := walletBySource(t, f.betting)
	var beforeLedger int
	if err = f.betting.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries`).Scan(&beforeLedger); err != nil {
		t.Fatal(err)
	}
	err = commissionBatchCallTx(t, f, func(tx pgx.Tx) error {
		_, callErr := f.service.CreateCycleTx(ctx, tx, f.betting.brand, f.actor,
			commission.CreateCycleInput{AnchorOrderID: f.orders[0].ID, Reason: "duplicate closed window rejected"},
			points.Metadata{ActorType: "admin", ActorID: f.actor.ID, RequestID: ids.New()})
		return callErr
	})
	if !errors.Is(err, commission.ErrCycleState) {
		t.Fatalf("duplicate same-window create error=%v, want state conflict", err)
	}
	if afterCalc, afterAlloc := commissionBatchPersistence(t, f, cycle.ID); afterCalc != beforeCalculations || afterAlloc != beforeAllocations {
		t.Fatal("ready cycle rerun or duplicate create changed saved economics")
	}
	var afterLedger int
	if err = f.betting.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries`).Scan(&afterLedger); err != nil || afterLedger != beforeLedger || walletBySource(t, f.betting) != beforeWallet {
		t.Fatalf("commission calculation posted wallet ledger: count %d->%d err=%v", beforeLedger, afterLedger, err)
	}

	oldRun := *cycle.CurrentRunID
	oldGeneration := *cycle.CurrentGeneration
	oldEpoch := *cycle.EvidenceEpoch
	correction := createFixtureCorrection(t, f.betting, correctedDigits(0, 0, 0))
	if correction.State != "reversing" {
		t.Fatalf("loss-to-win correction state=%s", correction.State)
	}
	if _, err = f.betting.service.ProcessCorrections(ctx, 20); err != nil {
		t.Fatal(err)
	}
	correction, err = f.betting.service.Correction(ctx, f.betting.brand, correction.ID)
	if err != nil || correction.State != "resettling" || correction.NewJobID == nil {
		t.Fatalf("correction did not create real new settlement generation: %+v err=%v", correction, err)
	}
	if _, err = f.betting.service.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	correctedOrder, err := f.betting.service.Order(ctx, f.betting.brand, f.betting.member, f.orders[0].ID)
	if err != nil || correctedOrder.Status != "won" {
		t.Fatalf("corrected lost order state=%s err=%v", correctedOrder.Status, err)
	}
	var epochAfter int64
	if err = f.betting.db.QueryRow(ctx, `SELECT evidence_epoch FROM commission_cycles WHERE brand_id=$1 AND id=$2`, f.betting.brand, cycle.ID).Scan(&epochAfter); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(epochAfter) == oldEpoch {
		t.Fatalf("settlement correction did not advance evidence epoch %s", oldEpoch)
	}
	stale := readCommissionCycle(t, f, cycle.ID)
	if stale.EvidenceCurrent || stale.CurrentRunID == nil || *stale.CurrentRunID != oldRun {
		t.Fatalf("corrected evidence must flag the retained old run stale: %+v", stale)
	}
	advanceCommissionWorker(t, f, 30)
	cycle = readCommissionCycle(t, f, cycle.ID)
	if cycle.State != "ready" || !cycle.EvidenceCurrent || cycle.CurrentGeneration == nil || *cycle.CurrentGeneration == oldGeneration || cycle.TotalPoints != "0" {
		t.Fatalf("corrected commission generation=%+v, previous generation=%s", cycle, oldGeneration)
	}
	oldCalcs, oldAllocs := commissionBatchPersistenceForRun(t, f, cycle.ID, oldRun)
	if oldCalcs != 3 || oldAllocs != 2 {
		t.Fatalf("correction rewrote previous calculation run: calculations=%d allocations=%d", oldCalcs, oldAllocs)
	}
	historyTx, err := f.betting.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	runs, runErr := f.service.RunsTx(ctx, historyTx, f.betting.brand, cycle.ID, 100, 0)
	oldHistory, historyErr := f.service.CalculationsTx(ctx, historyTx, f.betting.brand, cycle.ID, oldRun, 100, 0)
	newHistory, newHistoryErr := f.service.CalculationsTx(ctx, historyTx, f.betting.brand, cycle.ID, *cycle.CurrentRunID, 100, 0)
	_ = historyTx.Rollback(ctx)
	if runErr != nil || len(runs.Items) != 2 || runs.TotalCount != "2" || runs.Items[0].ID != *cycle.CurrentRunID || runs.Items[1].ID != oldRun || runs.Items[1].State != "abandoned" {
		t.Fatalf("preserved run history=%+v err=%v", runs, runErr)
	}
	if historyErr != nil || newHistoryErr != nil || oldHistory.TotalCount != "3" || newHistory.TotalCount != "3" || len(oldHistory.Items) != 3 || len(newHistory.Items) != 3 {
		t.Fatalf("explicit old/new calculation history=%+v/%+v errs=%v/%v", oldHistory, newHistory, historyErr, newHistoryErr)
	}
	var oldBase, newBase points.Amount
	for _, item := range oldHistory.Items {
		oldBase += item.BasePoints
		if item.RunID != oldRun {
			t.Fatal("old history silently substituted current run")
		}
	}
	for _, item := range newHistory.Items {
		newBase += item.BasePoints
	}
	if oldBase != 2 || newBase != 0 {
		t.Fatalf("history economic evidence changed: old base=%d new base=%d", oldBase, newBase)
	}
	historyJSON, err := json.Marshal(oldHistory)
	if err != nil || bytes.Contains(historyJSON, []byte("rule_snapshot")) || bytes.Contains(historyJSON, []byte("agent_path")) || bytes.Contains(historyJSON, []byte("account_id")) {
		t.Fatal("calculation history exposed private witness data")
	}
	var currentLosses, commissionEntries int
	if err = f.betting.db.QueryRow(ctx, `SELECT count(*) FILTER (WHERE base_points>0),count(*) FILTER (WHERE reason='eligible') FROM commission_calculations WHERE cycle_id=$1 AND run_id=$2`, cycle.ID, *cycle.CurrentRunID).Scan(&currentLosses, &commissionEntries); err != nil || currentLosses != 0 || commissionEntries != 2 {
		t.Fatalf("new corrected run loss/eligible calculations=%d/%d err=%v", currentLosses, commissionEntries, err)
	}
	if err = f.betting.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='commission'`).Scan(&commissionsBefore); err != nil || commissionsBefore != 0 {
		t.Fatalf("calculation worker credited commission points: entries=%d err=%v", commissionsBefore, err)
	}
	readyCycle := cycle
	readyCalculations, readyAllocations := commissionBatchPersistence(t, f, cycle.ID)
	var readyEarnings string
	var readyRunState string
	if err = f.betting.db.QueryRow(ctx, `SELECT r.state,coalesce(jsonb_agg(to_jsonb(e) ORDER BY e.agent_id)::text,'[]') FROM commission_runs r LEFT JOIN commission_earnings e ON e.run_id=r.id WHERE r.id=$1 GROUP BY r.id`, *cycle.CurrentRunID).Scan(&readyRunState, &readyEarnings); err != nil || readyRunState != "ready" {
		t.Fatalf("snapshot current run state=%s earnings=%s err=%v", readyRunState, readyEarnings, err)
	}
	settleEmptyCommissionBatchPeriod(t, f.betting, cycle.WindowTo)
	if processed, processErr := f.service.ProcessCycles(ctx, 20); processErr != nil || processed != 0 {
		t.Fatalf("unrelated future period reprocessed ready cycle: steps=%d err=%v", processed, processErr)
	}
	afterUnrelated := readCommissionCycle(t, f, cycle.ID)
	afterCalc, afterAlloc := commissionBatchPersistence(t, f, cycle.ID)
	if !reflect.DeepEqual(afterUnrelated, readyCycle) || afterCalc != readyCalculations || afterAlloc != readyAllocations {
		t.Fatalf("unrelated period changed ready cycle/run: before=%+v after=%+v calculations=%s/%s allocations=%s/%s", readyCycle, afterUnrelated, readyCalculations, afterCalc, readyAllocations, afterAlloc)
	}
	var afterRunState, afterEarnings string
	if err = f.betting.db.QueryRow(ctx, `SELECT r.state,coalesce(jsonb_agg(to_jsonb(e) ORDER BY e.agent_id)::text,'[]') FROM commission_runs r LEFT JOIN commission_earnings e ON e.run_id=r.id WHERE r.id=$1 GROUP BY r.id`, *cycle.CurrentRunID).Scan(&afterRunState, &afterEarnings); err != nil || afterRunState != readyRunState || afterEarnings != readyEarnings {
		t.Fatalf("unrelated period changed ready earning generation: state %s->%s earnings %s->%s err=%v", readyRunState, afterRunState, readyEarnings, afterEarnings, err)
	}
}

func commissionBatchPersistenceForRun(t *testing.T, f commissionBatchFixture, cycleID, runID string) (int, int) {
	t.Helper()
	var calculations, allocations int
	if err := f.betting.db.QueryRow(context.Background(), `SELECT
	 (SELECT count(*) FROM commission_calculations WHERE cycle_id=$1 AND run_id=$2),
	 (SELECT count(*) FROM commission_allocations WHERE cycle_id=$1 AND run_id=$2)`, cycleID, runID).Scan(&calculations, &allocations); err != nil {
		t.Fatal(err)
	}
	return calculations, allocations
}

func settleCommissionBatchFixture(t *testing.T, f bettingFixture) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, err := (rulebook.Store{DB: f.db}).Tick(ctx); err != nil {
			t.Fatal(err)
		}
		var state string
		if err := f.db.QueryRow(ctx, `SELECT status FROM periods WHERE id=$1`, f.period.ID).Scan(&state); err != nil {
			t.Fatal(err)
		}
		if state == "waiting_draw" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("period did not reach natural draw time; state=%s", state)
		}
		time.Sleep(20 * time.Millisecond)
	}
	var periodVersion int64
	if err := f.db.QueryRow(ctx, `SELECT version FROM periods WHERE id=$1`, f.period.ID).Scan(&periodVersion); err != nil {
		t.Fatal(err)
	}
	publicManual(t, f, f.period.ID, periodVersion, f.period.PeriodNo, rules.Draw{Digits: []int{1, 2, 1}}, f.period.DrawAt)
	actor := settlementActor(f)
	automatic := "automatic"
	policy, err := f.service.SettlementPolicy(ctx, f.brand)
	if err != nil {
		t.Fatal(err)
	}
	if policy.Mode == nil || *policy.Mode != automatic {
		policy = setSettlementMode(t, f, actor, policy.Version, &automatic)
	}
	startFixtureSettlement(t, f, actor, policy.Version)
	deadline = time.Now().Add(20 * time.Second)
	for {
		if _, err = f.service.ProcessSettlements(ctx, 100); err != nil {
			t.Fatal(err)
		}
		var state string
		if err = f.db.QueryRow(ctx, `SELECT status FROM periods WHERE id=$1`, f.period.ID).Scan(&state); err != nil {
			t.Fatal(err)
		}
		if state == "settled" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("real settlement worker did not finish the 103-order period; state=%s", state)
		}
	}
}

func settleEmptyCommissionBatchPeriod(t *testing.T, f bettingFixture, after time.Time) {
	t.Helper()
	ctx := context.Background()
	store := rulebook.Store{DB: f.db}
	start := time.Now().UTC().Add(-time.Second)
	end := time.Now().UTC().Add(time.Second)
	drawAt := time.Now().UTC().Add(3 * time.Second)
	if !start.After(after) {
		t.Fatalf("new empty period start %s is not after closed cycle window %s", start, after)
	}
	periodNo := "commission-empty-" + strings.ReplaceAll(ids.New(), "-", "")[:12]
	var empty rulebook.Period
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var err error
		empty, err = store.OpenPeriod(ctx, tx, f.brand, f.game.ID, periodNo, start, end, drawAt)
		return err
	})
	f.period = empty
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := store.Tick(ctx); err != nil {
			t.Fatal(err)
		}
		if err := f.db.QueryRow(ctx, `SELECT status FROM periods WHERE brand_id=$1 AND id=$2`, f.brand, empty.ID).Scan(&empty.Status); err != nil {
			t.Fatal(err)
		}
		if empty.Status == "waiting_draw" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("new empty period did not reach natural draw time; state=%s", empty.Status)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := f.db.QueryRow(ctx, `SELECT version FROM periods WHERE brand_id=$1 AND id=$2`, f.brand, empty.ID).Scan(&empty.Version); err != nil {
		t.Fatal(err)
	}
	publicManual(t, f, empty.ID, empty.Version, periodNo, rules.Draw{Digits: []int{2, 3, 4}}, drawAt)
	actor := settlementActor(f)
	automatic := "automatic"
	policy, err := f.service.SettlementPolicy(ctx, f.brand)
	if err != nil {
		t.Fatal(err)
	}
	if policy.Mode == nil || *policy.Mode != automatic {
		policy = setSettlementMode(t, f, actor, policy.Version, &automatic)
	}
	startFixtureSettlement(t, f, actor, policy.Version)
	for {
		if _, err = f.service.ProcessSettlements(ctx, 20); err != nil {
			t.Fatal(err)
		}
		if err = f.db.QueryRow(ctx, `SELECT status FROM periods WHERE brand_id=$1 AND id=$2`, f.brand, empty.ID).Scan(&empty.Status); err != nil {
			t.Fatal(err)
		}
		if empty.Status == "settled" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("empty future period did not settle through the real worker; state=%s", empty.Status)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestCommissionBatchCalculationPageFailureIsAuditedAndRetryable(t *testing.T) {
	f := newCommissionBatchFixture(t)
	waitCommissionBoundary(t, f.boundary)
	cycle := createCommissionCycle(t, f)
	eligibilitySettle(t, f.betting)
	if _, err := f.betting.db.Exec(context.Background(), `CREATE FUNCTION test_reject_commission_calculation_page() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='commission.cycle.calculation_page' THEN RAISE EXCEPTION 'test commission calculation page outage'; END IF; RETURN NEW; END $$; CREATE TRIGGER test_reject_commission_calculation_page BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION test_reject_commission_calculation_page()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.betting.db.Exec(context.Background(), `DROP TRIGGER IF EXISTS test_reject_commission_calculation_page ON audit_logs; DROP FUNCTION IF EXISTS test_reject_commission_calculation_page()`)
	})
	advanceCommissionWorker(t, f, 10)
	cycle = readCommissionCycle(t, f, cycle.ID)
	if cycle.State != "failed" || cycle.LastErrorCode == nil || *cycle.LastErrorCode != "COMMISSION_CALCULATION_FAILED" {
		t.Fatalf("audit failure did not persist failed cycle state: %+v", cycle)
	}
	viewer := f.actor
	viewer.Roles = []access.Role{{BrandID: f.betting.brand, Permissions: []access.Permission{{Resource: "commission", Action: "view", Scope: access.ScopeBrand}}}}
	if err := commissionBatchCallTx(t, f, func(tx pgx.Tx) error {
		_, retryErr := f.service.RetryCycleTx(context.Background(), tx, f.betting.brand, cycle.ID, viewer,
			commission.RetryCycleInput{Version: cycle.Version, Reason: "viewer cannot retry a failed cycle"},
			points.Metadata{ActorType: "admin", ActorID: viewer.ID, RequestID: ids.New()})
		return retryErr
	}); !errors.Is(err, commission.ErrDenied) {
		t.Fatalf("view-only commission actor retry error=%v, want denied", err)
	}
	var calculations, allocations int
	if err := f.betting.db.QueryRow(context.Background(), `SELECT
	 (SELECT count(*) FROM commission_calculations WHERE cycle_id=$1),
	 (SELECT count(*) FROM commission_allocations WHERE cycle_id=$1)`, cycle.ID).Scan(&calculations, &allocations); err != nil || calculations != 0 || allocations != 0 {
		t.Fatalf("failed page left partial calculation evidence: calculations=%d allocations=%d err=%v", calculations, allocations, err)
	}
	if _, err := f.betting.db.Exec(context.Background(), `DROP TRIGGER test_reject_commission_calculation_page ON audit_logs; DROP FUNCTION test_reject_commission_calculation_page()`); err != nil {
		t.Fatal(err)
	}
	if err := commissionBatchCallTx(t, f, func(tx pgx.Tx) error {
		_, retryErr := f.service.RetryCycleTx(context.Background(), tx, f.betting.brand, cycle.ID, f.actor,
			commission.RetryCycleInput{Version: cycle.Version, Reason: "retry after audit storage recovery"},
			points.Metadata{ActorType: "admin", ActorID: f.actor.ID, RequestID: ids.New()})
		return retryErr
	}); err != nil {
		t.Fatal(err)
	}
	advanceCommissionWorker(t, f, 20)
	cycle = readCommissionCycle(t, f, cycle.ID)
	if cycle.State != "ready" || cycle.CalculatedCount != "3" || cycle.EarningCount != "1" || cycle.TotalPoints != "1" {
		t.Fatalf("manual retry did not recover complete cycle: %+v", cycle)
	}
	calculationCount, allocationCount := commissionBatchPersistenceForRun(t, f, cycle.ID, *cycle.CurrentRunID)
	if calculationCount != 3 || allocationCount != 2 {
		t.Fatalf("retried final calculations=%d allocations=%d, want 3 and 2", calculationCount, allocationCount)
	}
}

func TestCommissionBatchOverHundredOrdersUsesRealManifestAndConcurrentWorkerPages(t *testing.T) {
	f := newCommissionBatchFixtureWithWindow(t, 45*time.Second, 47*time.Second)
	ctx := context.Background()
	// These later wagers capture the then-current 0.2 disabled node policy;
	// the three fixture orders retain their original 0.3 active snapshots.
	agents := agency.Service{DB: f.betting.db}
	policy, err := agents.Policy(ctx, f.betting.brand)
	if err != nil {
		t.Fatal(err)
	}
	var updated agency.Node
	if err = commissionBatchCallTx(t, f, func(tx pgx.Tx) error {
		var updateErr error
		updated, updateErr = agents.Update(ctx, tx, f.betting.brand, f.actor, f.node.ID, agency.UpdateInput{
			Version: f.node.Version, PolicyVersion: policy.Version,
			Config: agency.NodeConfig{Ratio: "0.2", Status: "disabled", CanCreateChildren: false},
			Reason: "later wagers capture disabled node at ratio 0.2",
		}, points.Metadata{RequestID: ids.New()})
		return updateErr
	}); err != nil {
		t.Fatal(err)
	}
	f.node = updated
	fundBettingWallet(t, f.betting, 120)
	in := f.betting.input
	in.Selection = rules.Selection{Digits: [][]int{{0}, {0}, {0}}}
	for i := 0; i < 100; i++ {
		order, placeErr := placeBettingOrder(t, f.betting, in, fmt.Sprintf("commission-batch-page-%03d", i))
		if placeErr != nil {
			t.Fatalf("place real pagination wager %d: %v", i, placeErr)
		}
		if !order.PlacedAt.Before(f.boundary) {
			t.Fatalf("pagination wager %d at %s crossed real cycle boundary %s", i, order.PlacedAt, f.boundary)
		}
		f.orders = append(f.orders, order)
	}
	waitCommissionBoundary(t, f.boundary)
	cycle := createCommissionCycle(t, f)
	if n, processErr := f.service.ProcessCycles(ctx, 1); processErr != nil || n != 1 {
		t.Fatalf("first manifest page processed=%d err=%v", n, processErr)
	}
	cycle = readCommissionCycle(t, f, cycle.ID)
	if cycle.State != "enumerating" || cycle.TargetCount != "100" || cycle.ScanComplete {
		t.Fatalf("first real manifest page=%+v; want 100 orders and incomplete scan", cycle)
	}
	if n, processErr := f.service.ProcessCycles(ctx, 1); processErr != nil || n != 1 {
		t.Fatalf("second manifest page processed=%d err=%v", n, processErr)
	}
	cycle = readCommissionCycle(t, f, cycle.ID)
	if cycle.State != "waiting" || cycle.TargetCount != "103" || !cycle.ScanComplete {
		t.Fatalf("complete real manifest=%+v; want 103 orders", cycle)
	}

	settleCommissionBatchFixture(t, f.betting)
	var settledOrders, nonFinalOrders int
	if err = f.betting.db.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE status NOT IN('won','lost','abnormal','bet_cancelled','judged_cancelled')) FROM bet_orders WHERE brand_id=$1 AND period_id=$2`, f.betting.brand, f.betting.period.ID).Scan(&settledOrders, &nonFinalOrders); err != nil || settledOrders != 103 || nonFinalOrders != 0 {
		t.Fatalf("natural settlement status counts total=%d nonfinal=%d err=%v", settledOrders, nonFinalOrders, err)
	}
	advanceCommissionWorker(t, f, 1) // waiting -> calculating after natural settlement
	cycle = readCommissionCycle(t, f, cycle.ID)
	if cycle.State != "calculating" || cycle.CurrentRunID == nil {
		t.Fatalf("settled manifest did not start calculation run: %+v", cycle)
	}
	walletBefore := walletBySource(t, f.betting)
	var ledgerBefore int
	if err = f.betting.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries`).Scan(&ledgerBefore); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	errCh := make(chan error, 2)
	workerCtx, cancelWorkers := context.WithTimeout(ctx, 30*time.Second)
	defer cancelWorkers()
	var workers sync.WaitGroup
	for i := 0; i < 2; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			for step := 0; step < 70; step++ {
				if _, processErr := f.service.ProcessCycles(workerCtx, 1); processErr != nil {
					errCh <- processErr
					return
				}
				var state string
				if queryErr := f.betting.db.QueryRow(ctx, `SELECT state FROM commission_cycles WHERE brand_id=$1 AND id=$2`, f.betting.brand, cycle.ID).Scan(&state); queryErr != nil {
					errCh <- queryErr
					return
				}
				if state == "ready" {
					return
				}
				if state == "failed" {
					var errorCode string
					if queryErr := f.betting.db.QueryRow(ctx, `SELECT coalesce(last_error_code,'') FROM commission_cycles WHERE brand_id=$1 AND id=$2`, f.betting.brand, cycle.ID).Scan(&errorCode); queryErr != nil {
						errCh <- queryErr
					} else {
						errCh <- fmt.Errorf("commission worker persisted failed state with code %q", errorCode)
					}
					return
				}
				time.Sleep(100 * time.Millisecond)
			}
			var state, errorCode, runState string
			var calculations, allocations int
			queryErr := f.betting.db.QueryRow(ctx, `SELECT c.state,coalesce(c.last_error_code,''),coalesce(r.state,''),
			 (SELECT count(*) FROM commission_calculations x WHERE x.cycle_id=c.id AND x.run_id=c.current_run_id),
			 (SELECT count(*) FROM commission_allocations a WHERE a.cycle_id=c.id AND a.run_id=c.current_run_id)
			 FROM commission_cycles c LEFT JOIN commission_runs r ON r.id=c.current_run_id WHERE c.brand_id=$1 AND c.id=$2`, f.betting.brand, cycle.ID).Scan(&state, &errorCode, &runState, &calculations, &allocations)
			if queryErr != nil {
				errCh <- queryErr
			} else {
				errCh <- fmt.Errorf("bounded workers did not reach ready: state=%s error_code=%s run_state=%s calculations=%d allocations=%d", state, errorCode, runState, calculations, allocations)
			}
		}()
	}
	close(start)
	workers.Wait()
	close(errCh)
	for workerErr := range errCh {
		t.Fatal(workerErr)
	}
	cycle = readCommissionCycle(t, f, cycle.ID)
	if cycle.State != "ready" || cycle.TargetCount != "103" || cycle.CalculatedCount != "103" || cycle.EarningCount != "1" || cycle.TotalPoints != "21" || cycle.CurrentGeneration == nil || *cycle.CurrentGeneration != "1" {
		t.Fatalf("completed paginated cycle=%+v", cycle)
	}
	var calculationCount, uniqueOrders, eligibleCount, allocationCount int
	if err = f.betting.db.QueryRow(ctx, `SELECT count(*),count(DISTINCT order_id),count(*) FILTER (WHERE reason='eligible'),
	 (SELECT count(*) FROM commission_allocations WHERE cycle_id=$1 AND run_id=$2)
	 FROM commission_calculations WHERE cycle_id=$1 AND run_id=$2`, cycle.ID, *cycle.CurrentRunID).Scan(&calculationCount, &uniqueOrders, &eligibleCount, &allocationCount); err != nil {
		t.Fatal(err)
	}
	if calculationCount != 103 || uniqueOrders != 103 || eligibleCount != 102 || allocationCount != 102 {
		t.Fatalf("persisted multi-page calculations=%d unique_orders=%d eligible=%d allocations=%d; want 103/103/102/102", calculationCount, uniqueOrders, eligibleCount, allocationCount)
	}
	var exactNumerator, exactDenominator string
	var earned points.Amount
	var earningAgent, earningMember string
	if err = f.betting.db.QueryRow(ctx, `SELECT numerator,denominator,points,agent_id::text,member_id::text FROM commission_earnings WHERE cycle_id=$1 AND run_id=$2`, cycle.ID, *cycle.CurrentRunID).Scan(&exactNumerator, &exactDenominator, &earned, &earningAgent, &earningMember); err != nil {
		t.Fatal(err)
	}
	if exactNumerator != "103" || exactDenominator != "5" || earned != 21 || earningAgent != f.node.ID || earningMember != f.node.MemberID {
		t.Fatalf("aggregated earnings=%s/%s => %d agent=%s member=%s; want 103/5 => 21 for root agent/member", exactNumerator, exactDenominator, earned, earningAgent, earningMember)
	}
	var evidenceEpoch int64
	if err = f.betting.db.QueryRow(ctx, `SELECT evidence_epoch FROM commission_cycles WHERE brand_id=$1 AND id=$2`, f.betting.brand, cycle.ID).Scan(&evidenceEpoch); err != nil {
		t.Fatal(err)
	}
	if cycle.EvidenceEpoch == nil || *cycle.EvidenceEpoch != fmt.Sprint(evidenceEpoch) {
		t.Fatalf("ready run epoch=%v current evidence epoch=%d", cycle.EvidenceEpoch, evidenceEpoch)
	}
	var ledgerAfter int
	if err = f.betting.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries`).Scan(&ledgerAfter); err != nil {
		t.Fatal(err)
	}
	if ledgerAfter != ledgerBefore || walletBySource(t, f.betting) != walletBefore {
		t.Fatalf("concurrent calculation workers changed financial ledger/wallet: ledger %d->%d wallet %+v->%+v", ledgerBefore, ledgerAfter, walletBefore, walletBySource(t, f.betting))
	}
}
