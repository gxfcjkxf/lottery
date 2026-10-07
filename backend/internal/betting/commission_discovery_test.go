package betting

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

func commissionDiscoveryRead(t *testing.T, f commissionBatchFixture, id string) commission.Discovery {
	t.Helper()
	tx, err := f.betting.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	discovery, err := f.service.DiscoveryTx(context.Background(), tx, f.betting.brand, id)
	if err != nil {
		t.Fatal(err)
	}
	return discovery
}

func commissionDiscoveryCallTx(t *testing.T, f commissionBatchFixture, call func(pgx.Tx) error) error {
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

func waitCommissionDiscoveriesDue(t *testing.T, f commissionBatchFixture, ctx context.Context) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		now := time.Now()
		latestDue := now
		for _, order := range f.orders {
			d := commissionDiscoveryRead(t, f, order.ID)
			if d.NextCheckAt.After(latestDue) {
				latestDue = d.NextCheckAt
			}
		}
		if !latestDue.After(now) {
			return
		}
		if !latestDue.Before(deadline) {
			t.Fatalf("discovery retry time %s exceeded bounded wait deadline %s", latestDue, deadline)
		}
		wait := time.Until(latestDue)
		if wait > 25*time.Millisecond {
			wait = 25 * time.Millisecond
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			t.Fatal(ctx.Err())
		case <-timer.C:
		}
	}
}

func processCommissionDiscoveriesUntilRegistered(t *testing.T, f commissionBatchFixture, ctx context.Context) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		registered := true
		for _, order := range f.orders {
			if d := commissionDiscoveryRead(t, f, order.ID); d.State != "registered" {
				registered = false
			}
		}
		if registered {
			return
		}
		if _, err := f.service.ProcessDiscovery(ctx, 100); err != nil {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			for _, order := range f.orders {
				t.Logf("discovery after bounded worker polling: %+v", commissionDiscoveryRead(t, f, order.ID))
			}
			t.Fatal("discoveries did not register within the bounded retry window")
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			t.Fatal(ctx.Err())
		case <-timer.C:
		}
	}
}

func TestCommissionDiscoveryAutoRegistersSavedWindowsAndSettlesWithoutPayment(t *testing.T) {
	f := newCommissionBatchFixtureWithWindow(t, 8*time.Second, 10*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if processed, err := f.service.ProcessDiscovery(ctx, 100); err != nil || processed != len(f.orders) {
		t.Fatalf("initial discovery processed=%d err=%v", processed, err)
	}
	for _, order := range f.orders {
		d := commissionDiscoveryRead(t, f, order.ID)
		if d.State != "pending" || d.Version != 1 || d.WindowFrom == nil || d.WindowTo == nil || !d.WindowTo.Equal(f.boundary) || !d.NextCheckAt.Equal(f.boundary) {
			t.Fatalf("future order discovery=%+v; expected a pending record scheduled at its saved boundary %s", d, f.boundary)
		}
	}
	var earlyCycles int
	if err := f.betting.db.QueryRow(ctx, `SELECT count(*) FROM commission_cycles WHERE brand_id=$1`, f.betting.brand).Scan(&earlyCycles); err != nil || earlyCycles != 0 {
		t.Fatalf("future window created commission cycles=%d err=%v", earlyCycles, err)
	}

	// Disable the mutable policy after placement. The enabled saved snapshots
	// must still be discovered using their original calendar.
	policy, err := f.service.Policy(ctx, f.betting.brand)
	if err != nil {
		t.Fatal(err)
	}
	if err = commissionDiscoveryCallTx(t, f, func(tx pgx.Tx) error {
		_, updateErr := f.service.Update(ctx, tx, f.betting.brand, f.actor, commission.PolicyInput{
			Version: policy.Version,
			Config:  commission.PolicyConfig{Enabled: false, PayoutMode: commission.PayoutManual},
			Reason:  "disable future commission snapshots while retaining placed bets",
		}, points.Metadata{ActorType: "admin", ActorID: f.actor.ID, RequestID: ids.New()})
		return updateErr
	}); err != nil {
		t.Fatal(err)
	}

	waitCommissionBoundary(t, f.boundary)
	var workers sync.WaitGroup
	workerErr := make(chan error, 3)
	for i := 0; i < 3; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, err := f.service.ProcessDiscovery(ctx, 1)
			workerErr <- err
		}()
	}
	workers.Wait()
	close(workerErr)
	for err := range workerErr {
		if err != nil {
			t.Fatal(err)
		}
	}
	processCommissionDiscoveriesUntilRegistered(t, f, ctx)

	var cycleID string
	if err = f.betting.db.QueryRow(ctx, `SELECT cycle_id::text FROM commission_discovery WHERE brand_id=$1 AND id=$2`, f.betting.brand, f.orders[0].ID).Scan(&cycleID); err != nil {
		t.Fatal(err)
	}
	for _, order := range f.orders {
		d := commissionDiscoveryRead(t, f, order.ID)
		if d.State != "registered" || d.CycleID == nil || *d.CycleID != cycleID || d.Version != 2 || !d.WindowTo.Equal(f.boundary) {
			t.Fatalf("discovery did not register against the saved window: %+v", d)
		}
	}
	var creationActor string
	var createdBy *string
	var cycleState string
	if err = f.betting.db.QueryRow(ctx, `SELECT creation_actor_type,created_by::text,state FROM commission_cycles WHERE brand_id=$1 AND id=$2`, f.betting.brand, cycleID).Scan(&creationActor, &createdBy, &cycleState); err != nil {
		t.Fatal(err)
	}
	if creationActor != "system" || createdBy != nil || cycleState != "enumerating" {
		t.Fatalf("automatic cycle provenance/state actor=%q created_by=%v state=%q", creationActor, createdBy, cycleState)
	}
	var cycleAuditActors int
	if err = f.betting.db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE brand_id=$1 AND resource_type='commission_cycle' AND resource_id=$2 AND action='commission.cycle.create' AND actor_type='system' AND actor_id IS NULL`, f.betting.brand, cycleID).Scan(&cycleAuditActors); err != nil || cycleAuditActors != 1 {
		t.Fatalf("system cycle creation audits=%d err=%v", cycleAuditActors, err)
	}
	var matchingCycles int
	if err = f.betting.db.QueryRow(ctx, `SELECT count(*) FROM commission_cycles WHERE brand_id=$1 AND window_from=(SELECT window_from FROM commission_cycles WHERE id=$2) AND window_to=(SELECT window_to FROM commission_cycles WHERE id=$2)`, f.betting.brand, cycleID).Scan(&matchingCycles); err != nil || matchingCycles != 1 {
		t.Fatalf("same-window cycle count=%d err=%v", matchingCycles, err)
	}

	advanceCommissionWorker(t, f, 10)
	cycle := readCommissionCycle(t, f, cycleID)
	firstDiscovery := commissionDiscoveryRead(t, f, f.orders[0].ID)
	if cycle.State != "waiting" || cycle.TargetCount != "3" || !cycle.ScanComplete ||
		firstDiscovery.WindowFrom == nil || !cycle.WindowFrom.Equal(*firstDiscovery.WindowFrom) || !cycle.WindowTo.Equal(f.boundary) ||
		cycle.Calendar.BoundaryTime != f.boundary.Format("15:04:05") {
		t.Fatalf("automatic cycle before settlement=%+v", cycle)
	}
	eligibilitySettle(t, f.betting)
	advanceCommissionWorker(t, f, 20)
	cycle = readCommissionCycle(t, f, cycleID)
	if cycle.State != "ready" || cycle.CalculatedCount != "3" || cycle.EarningCount != "1" {
		t.Fatalf("automatic cycle after real settlement=%+v", cycle)
	}
	var commissionEntries int
	if err = f.betting.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='commission' AND brand_id=$1`, f.betting.brand).Scan(&commissionEntries); err != nil || commissionEntries != 0 {
		t.Fatalf("discovery or calculation posted commission ledger entries=%d err=%v", commissionEntries, err)
	}
}

func TestCommissionDiscoveryLinksToExistingManualRegistration(t *testing.T) {
	f := newCommissionBatchFixtureWithWindow(t, 8*time.Second, 10*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	waitCommissionBoundary(t, f.boundary)
	mutex, err := f.betting.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var lockedBrand string
	if err = mutex.QueryRow(ctx, `SELECT brand_id::text FROM brand_commission_policies WHERE brand_id=$1 FOR UPDATE`, f.betting.brand).Scan(&lockedBrand); err != nil {
		_ = mutex.Rollback(ctx)
		t.Fatal(err)
	}
	if _, processErr := f.service.ProcessDiscovery(ctx, 100); processErr != nil {
		_ = mutex.Rollback(ctx)
		t.Fatalf("discovery during financial mutex err=%v", processErr)
	}
	for _, order := range f.orders {
		d := commissionDiscoveryRead(t, f, order.ID)
		if d.State != "pending" || d.LastErrorCode != nil || !d.NextCheckAt.After(time.Now()) {
			_ = mutex.Rollback(ctx)
			t.Fatalf("financial mutex contention failed to defer pending discovery: %+v", d)
		}
	}
	var cyclesDuringMutex int
	if err = f.betting.db.QueryRow(ctx, `SELECT count(*) FROM commission_cycles WHERE brand_id=$1`, f.betting.brand).Scan(&cyclesDuringMutex); err != nil || cyclesDuringMutex != 0 {
		_ = mutex.Rollback(ctx)
		t.Fatalf("financial mutex contention created cycles=%d err=%v", cyclesDuringMutex, err)
	}
	if err = mutex.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	waitCommissionDiscoveriesDue(t, f, ctx)
	manual := createCommissionCycle(t, f)
	if processed, err := f.service.ProcessDiscovery(ctx, 100); err != nil || processed != len(f.orders) {
		t.Fatalf("discovery processed=%d err=%v", processed, err)
	}
	for _, order := range f.orders {
		d := commissionDiscoveryRead(t, f, order.ID)
		if d.State != "registered" || d.CycleID == nil || *d.CycleID != manual.ID {
			t.Fatalf("manual window registration was not linked: order=%s discovery=%+v manual=%s", order.ID, d, manual.ID)
		}
	}
	var count, actorCount int
	if err := f.betting.db.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE creation_actor_type='admin' AND created_by=$2)
	 FROM commission_cycles WHERE brand_id=$1 AND window_from=$3 AND window_to=$4`, f.betting.brand, f.actor.ID, manual.WindowFrom, manual.WindowTo).Scan(&count, &actorCount); err != nil || count != 1 || actorCount != 1 {
		t.Fatalf("manual same-window cycles=%d actor-owned=%d err=%v", count, actorCount, err)
	}
	var commissionEntries int
	if err := f.betting.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='commission' AND brand_id=$1`, f.betting.brand).Scan(&commissionEntries); err != nil || commissionEntries != 0 {
		t.Fatalf("registration posted commission ledger entries=%d err=%v", commissionEntries, err)
	}
}

func TestCommissionDiscoveryFailureRollsBackAndAllowsGuardedManualRetry(t *testing.T) {
	f := newCommissionBatchFixtureWithWindow(t, 8*time.Second, 10*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	waitCommissionBoundary(t, f.boundary)
	if _, err := f.betting.db.Exec(ctx, `CREATE FUNCTION test_reject_commission_discovery_cycle_audit() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN IF NEW.action='commission.cycle.create' THEN RAISE EXCEPTION 'test discovery audit storage outage'; END IF; RETURN NEW; END $$;
CREATE TRIGGER test_reject_commission_discovery_cycle_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION test_reject_commission_discovery_cycle_audit()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.betting.db.Exec(context.Background(), `DROP TRIGGER IF EXISTS test_reject_commission_discovery_cycle_audit ON audit_logs; DROP FUNCTION IF EXISTS test_reject_commission_discovery_cycle_audit()`)
	})
	if processed, err := f.service.ProcessDiscovery(ctx, 100); err != nil || processed != len(f.orders) {
		t.Fatalf("failed discovery step processed=%d err=%v", processed, err)
	}
	d := commissionDiscoveryRead(t, f, f.orders[0].ID)
	if d.State != "failed" || d.Version != 2 || d.LastErrorCode == nil || *d.LastErrorCode != "COMMISSION_DISCOVERY_FAILED" {
		t.Fatalf("audit-trigger failure was not recorded separately: %+v", d)
	}
	var cycleCount, cycleAuditCount int
	if err := f.betting.db.QueryRow(ctx, `SELECT count(*) FROM commission_cycles WHERE brand_id=$1`, f.betting.brand).Scan(&cycleCount); err != nil || cycleCount != 0 {
		t.Fatalf("failed registration left a partial cycle count=%d err=%v", cycleCount, err)
	}
	if err := f.betting.db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE brand_id=$1 AND resource_type='commission_discovery' AND action='commission.discovery.failure' AND actor_type='system' AND actor_id IS NULL`, f.betting.brand).Scan(&cycleAuditCount); err != nil || cycleAuditCount != len(f.orders) {
		t.Fatalf("failure transition audit count=%d err=%v", cycleAuditCount, err)
	}

	stale := commission.RetryCycleInput{Version: d.Version - 1, Reason: "stale discovery retry version"}
	err := commissionDiscoveryCallTx(t, f, func(tx pgx.Tx) error {
		_, retryErr := f.service.RetryDiscoveryTx(ctx, tx, f.betting.brand, f.orders[0].ID, f.actor, stale,
			points.Metadata{ActorType: "admin", ActorID: f.actor.ID, RequestID: ids.New()})
		return retryErr
	})
	if !errors.Is(err, commission.ErrCycleVersion) {
		t.Fatalf("stale discovery retry error=%v, want version conflict", err)
	}
	super := f.actor
	super.SuperAdmin = true
	err = commissionDiscoveryCallTx(t, f, func(tx pgx.Tx) error {
		_, retryErr := f.service.RetryDiscoveryTx(ctx, tx, f.betting.brand, f.orders[0].ID, super,
			commission.RetryCycleInput{Version: d.Version, Reason: "superadmin discovery retry denied"},
			points.Metadata{ActorType: "admin", ActorID: super.ID, RequestID: ids.New()})
		return retryErr
	})
	if !errors.Is(err, commission.ErrDenied) {
		t.Fatalf("superadmin discovery retry error=%v, want denied", err)
	}
	if _, err = f.betting.db.Exec(ctx, `DROP TRIGGER test_reject_commission_discovery_cycle_audit ON audit_logs; DROP FUNCTION test_reject_commission_discovery_cycle_audit()`); err != nil {
		t.Fatal(err)
	}
	if err = commissionDiscoveryCallTx(t, f, func(tx pgx.Tx) error {
		_, retryErr := f.service.RetryDiscoveryTx(ctx, tx, f.betting.brand, f.orders[0].ID, f.actor,
			commission.RetryCycleInput{Version: d.Version, Reason: "retry after audit storage recovery"},
			points.Metadata{ActorType: "admin", ActorID: f.actor.ID, RequestID: ids.New()})
		return retryErr
	}); err != nil {
		t.Fatal(err)
	}
	if processed, err := f.service.ProcessDiscovery(ctx, 100); err != nil || processed < 1 {
		t.Fatalf("manual discovery retry processed=%d err=%v", processed, err)
	}
	retried := commissionDiscoveryRead(t, f, f.orders[0].ID)
	if retried.State != "registered" || retried.Version != 4 || retried.CycleID == nil {
		t.Fatalf("manual retry did not register the saved window: %+v", retried)
	}
	if err = f.betting.db.QueryRow(ctx, `SELECT count(*) FROM commission_cycles WHERE brand_id=$1`, f.betting.brand).Scan(&cycleCount); err != nil || cycleCount != 1 {
		t.Fatalf("retry created cycle count=%d err=%v", cycleCount, err)
	}
}
