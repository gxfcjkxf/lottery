package betting

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

func commissionAdjustmentActor(f commissionBatchFixture) access.Account {
	a := commissionPaymentActor(f)
	for i := range a.Roles {
		if a.Roles[i].BrandID == f.betting.brand {
			a.Roles[i].Permissions = append(a.Roles[i].Permissions,
				access.Permission{Resource: "commission_adjustment", Action: "write", Scope: access.ScopeBrand})
		}
	}
	return a
}

func payManualCommissionForAdjustment(t *testing.T, f commissionBatchFixture, payment commission.Payment) commission.Payment {
	t.Helper()
	ctx := context.Background()
	a := commissionAdjustmentActor(f)
	if payment.State == "awaiting_approval" {
		if err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
			_, err := f.service.ApprovePaymentTx(ctx, tx, f.betting.brand, payment.ID, a,
				commission.RetryCycleInput{Version: payment.Version, Reason: "approve before commission adjustment"}, commissionPaymentMeta(a))
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 10; i++ {
		if _, err := f.service.ProcessPayments(ctx, 20); err != nil {
			t.Fatal(err)
		}
		payment = commissionPaymentRead(t, f, payment.ID)
		if payment.State == "paid" {
			return payment
		}
		if payment.State == "failed" {
			code := ""
			if payment.LastErrorCode != nil {
				code = *payment.LastErrorCode
			}
			t.Fatalf("manual payment failed before adjustment: state=%s code=%s paid=%s/%s", payment.State, code, payment.PaidPoints, payment.TotalPoints)
		}
		if _, err := f.betting.db.Exec(ctx, `UPDATE commission_payments SET next_work_at=clock_timestamp() WHERE brand_id=$1 AND id=$2`, f.betting.brand, payment.ID); err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("manual payment did not reach paid before adjustment: %+v", payment)
	return payment
}

func createPendingCommissionTarget(t *testing.T, f commissionBatchFixture, payment commission.Payment) string {
	t.Helper()
	targetID := ids.New()
	ctx := context.Background()
	tx, err := f.betting.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO commission_payment_targets(id,brand_id,payment_id,earning_id,points,member_id,agent_id,payment_version)
	 SELECT $1,e.brand_id,$2,e.id,e.points,e.member_id,e.agent_id,$3 FROM commission_earnings e WHERE e.run_id=$4`, targetID, payment.ID, payment.Version, payment.RunID)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return targetID
}

func commissionTargets(t *testing.T, f commissionBatchFixture, paymentID string) commission.TargetPage {
	t.Helper()
	tx, err := f.betting.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	page, err := f.service.PaymentTargetsTx(context.Background(), tx, f.betting.brand, paymentID, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func commissionWalletBySource(t *testing.T, f commissionBatchFixture, memberID string) points.Balance {
	t.Helper()
	wallet, err := (points.Store{DB: f.betting.db}).Read(context.Background(), f.betting.brand, memberID)
	if err != nil {
		t.Fatal(err)
	}
	return wallet.BySource
}

func adjustCommission(t *testing.T, f commissionBatchFixture, brand string, target commission.Target, actor access.Account, version int64, after points.Amount) (commission.Adjustment, error) {
	t.Helper()
	var adjustment commission.Adjustment
	err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		var err error
		adjustment, err = f.service.AdjustCommissionTx(context.Background(), tx, brand, target.ID, actor,
			commission.AdjustmentInput{Version: version, Points: after, Reason: "manual correction integration test"}, commissionPaymentMeta(actor))
		return err
	})
	return adjustment, err
}

type commissionAdjustmentSnapshot struct {
	Version  int64
	Points   points.Amount
	LastID   string
	Records  int
	Ledgers  int
	Audits   int
	Outboxes int
	Wallet   points.Balance
}

func readCommissionAdjustmentSnapshot(t *testing.T, f commissionBatchFixture, target commission.Target) commissionAdjustmentSnapshot {
	t.Helper()
	var snapshot commissionAdjustmentSnapshot
	ctx := context.Background()
	if err := f.betting.db.QueryRow(ctx, `SELECT h.version,h.points,coalesce(h.last_adjustment_id::text,'') FROM commission_adjustment_heads h WHERE h.brand_id=$1 AND h.target_id=$2`, f.betting.brand, target.ID).Scan(&snapshot.Version, &snapshot.Points, &snapshot.LastID); err != nil {
		t.Fatal(err)
	}
	if err := f.betting.db.QueryRow(ctx, `SELECT
	 (SELECT count(*) FROM commission_adjustments WHERE brand_id=$1 AND target_id=$2),
	 (SELECT count(*) FROM point_ledger_entries WHERE brand_id=$1 AND member_id=$3 AND entry_type='commission_adjustment'),
	 (SELECT count(*) FROM audit_logs WHERE brand_id=$1 AND action='commission.adjustment.create' AND after_json->>'target_id'=$2::text),
	 (SELECT count(*) FROM outbox_events WHERE brand_id=$1 AND event_type='commission.adjusted' AND payload->>'target_id'=$2::text)`, f.betting.brand, target.ID, target.MemberID).
		Scan(&snapshot.Records, &snapshot.Ledgers, &snapshot.Audits, &snapshot.Outboxes); err != nil {
		t.Fatal(err)
	}
	snapshot.Wallet = commissionWalletBySource(t, f, target.MemberID)
	return snapshot
}

func rejectAdjustmentTransactionWithoutWrites(t *testing.T, f commissionBatchFixture, target commission.Target, attack func(pgx.Tx) error) {
	t.Helper()
	ctx := context.Background()
	before := readCommissionAdjustmentSnapshot(t, f, target)
	tx, err := f.betting.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = attack(tx)
	if err == nil {
		err = tx.Commit(ctx)
	} else {
		_ = tx.Rollback(ctx)
	}
	if err == nil {
		t.Fatal("adversarial SQL transaction committed")
	}
	_ = tx.Rollback(ctx)
	after := readCommissionAdjustmentSnapshot(t, f, target)
	if after != before {
		t.Fatalf("rejected SQL transaction changed adjustment state:\nbefore=%+v\nafter=%+v", before, after)
	}
}

func insertDirectCommissionAdjustment(ctx context.Context, tx pgx.Tx, f commissionBatchFixture, target commission.Target, actor access.Account, after points.Amount) (string, error) {
	var version int64
	var before points.Amount
	var policyVersion int64
	if err := tx.QueryRow(ctx, `SELECT version,points FROM commission_adjustment_heads WHERE brand_id=$1 AND target_id=$2 FOR UPDATE`, f.betting.brand, target.ID).Scan(&version, &before); err != nil {
		return "", err
	}
	if err := tx.QueryRow(ctx, `SELECT version FROM brand_point_policies WHERE brand_id=$1`, f.betting.brand).Scan(&policyVersion); err != nil {
		return "", err
	}
	id := ids.New()
	reason := "direct SQL adversarial invariant test"
	delta := after - before
	logID, err := audit.Append(ctx, tx, audit.Record{
		BrandID: f.betting.brand, ActorType: "admin", ActorID: actor.ID, Action: "commission.adjustment.create",
		ResourceType: "commission_adjustment", ResourceID: id, Reason: reason, RequestID: ids.New(),
		Before: map[string]any{"version": version, "points": before},
		After:  map[string]any{"version": version + 1, "points": after, "delta_points": delta, "target_id": target.ID, "payment_id": target.PaymentID},
	})
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `INSERT INTO commission_adjustments(id,brand_id,target_id,payment_id,version,points_before,points_after,delta_points,audit_log_id,created_by,reason,point_policy_version)
	 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, id, f.betting.brand, target.ID, target.PaymentID, version+1, before, after, delta, logID, actor.ID, reason, policyVersion)
	return id, err
}

func saveCommissionPointCaps(t *testing.T, f commissionBatchFixture, actor access.Account, version int64, maxBalance, maxAdjustment *points.Amount) points.Policy {
	t.Helper()
	ctx := context.Background()
	tx, err := f.betting.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := (points.Store{DB: f.betting.db}).UpdatePolicy(ctx, tx, f.betting.brand, points.PolicyInput{
		Version: version, MaxBalancePoints: maxBalance, MaxRechargePoints: nil, MaxAdjustmentPoints: maxAdjustment,
		Reason: "commission adjustment integration point cap",
	}, points.Metadata{ActorType: "admin", ActorID: actor.ID, RequestID: ids.New()})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return updated
}

func TestCommissionAdjustmentPaidTargetMovesOnlyCommissionAvailableAndPreservesEvidence(t *testing.T) {
	f := newCommissionBatchFixture(t)
	ctx := context.Background()
	a := commissionAdjustmentActor(f)
	payment := payManualCommissionForAdjustment(t, f, prepareManualCommissionPayment(t, f))
	page := commissionTargets(t, f, payment.ID)
	if len(page.Items) != 1 {
		t.Fatalf("paid payment targets=%+v; want one target", page.Items)
	}
	target := page.Items[0]
	if target.State != "paid" || target.PaidAt == nil || target.LedgerEntryID == nil || target.AdjustmentVersion == nil || *target.AdjustmentVersion != 1 || target.AdjustedPoints == nil || *target.AdjustedPoints != 1 || target.LastAdjustmentID != nil {
		t.Fatalf("initial paid target projection=%+v", target)
	}
	var originalLedger, originalEarning string
	if err := f.betting.db.QueryRow(ctx, `SELECT md5(row_to_json(l)::text) FROM point_ledger_entries l WHERE l.id=$1`, *target.LedgerEntryID).Scan(&originalLedger); err != nil {
		t.Fatal(err)
	}
	if err := f.betting.db.QueryRow(ctx, `SELECT md5(row_to_json(e)::text) FROM commission_earnings e WHERE e.id=$1`, target.EarningID).Scan(&originalEarning); err != nil {
		t.Fatal(err)
	}
	if wallet := commissionWalletBySource(t, f, target.MemberID); wallet[3][0] != 1 {
		t.Fatalf("original payment did not credit one commission point: %+v", wallet)
	}
	if _, err := f.betting.db.Exec(ctx, `CREATE FUNCTION test_reject_commission_adjustment_outbox() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.event_type='commission.adjusted' THEN RAISE EXCEPTION 'test commission adjustment notification failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER test_reject_commission_adjustment_outbox BEFORE INSERT ON outbox_events FOR EACH ROW EXECUTE FUNCTION test_reject_commission_adjustment_outbox()`); err != nil {
		t.Fatal(err)
	}
	if _, err := adjustCommission(t, f, f.betting.brand, target, a, 1, 3); err == nil {
		t.Fatal("adjustment committed despite injected outbox failure")
	}
	if _, err := f.betting.db.Exec(ctx, `DROP TRIGGER test_reject_commission_adjustment_outbox ON outbox_events; DROP FUNCTION test_reject_commission_adjustment_outbox()`); err != nil {
		t.Fatal(err)
	}
	var rolledBackAdjustments int
	if err := f.betting.db.QueryRow(ctx, `SELECT count(*) FROM commission_adjustments WHERE target_id=$1`, target.ID).Scan(&rolledBackAdjustments); err != nil || rolledBackAdjustments != 0 {
		t.Fatalf("outbox failure left adjustment records=%d err=%v", rolledBackAdjustments, err)
	}
	if wallet := commissionWalletBySource(t, f, target.MemberID); wallet[3][0] != 1 {
		t.Fatalf("outbox failure left a correction in the wallet: %+v", wallet)
	}

	up, err := adjustCommission(t, f, f.betting.brand, target, a, *target.AdjustmentVersion, 3)
	if err != nil || up.Version != 2 || up.PointsBefore != 1 || up.PointsAfter != 3 || up.DeltaPoints != 2 || up.LedgerEntryID == "" || up.AuditLogID == "" || up.CreatedBy != a.ID {
		t.Fatalf("upward adjustment=%+v err=%v", up, err)
	}
	if wallet := commissionWalletBySource(t, f, target.MemberID); wallet[3][0] != 3 {
		t.Fatalf("upward correction wallet=%+v; want commission available 3", wallet)
	}
	down, err := adjustCommission(t, f, f.betting.brand, target, a, up.Version, 0)
	if err != nil || down.Version != 3 || down.PointsBefore != 3 || down.PointsAfter != 0 || down.DeltaPoints != -3 || down.LedgerEntryID == "" {
		t.Fatalf("downward adjustment=%+v err=%v", down, err)
	}
	if wallet := commissionWalletBySource(t, f, target.MemberID); wallet[3][0] != 0 {
		t.Fatalf("downward correction wallet=%+v; want commission available zero", wallet)
	}
	if got, err := adjustCommission(t, f, f.betting.brand, target, a, up.Version, 2); !errors.Is(err, commission.ErrAdjustmentVersion) {
		t.Fatalf("stale target adjustment=%+v err=%v; want version conflict", got, err)
	}
	var records, ledger, audits, adjustmentNotices int
	if err := f.betting.db.QueryRow(ctx, `SELECT
	 (SELECT count(*) FROM commission_adjustments WHERE target_id=$1),
		 (SELECT count(*) FROM point_ledger_entries l JOIN commission_adjustments a ON a.ledger_entry_id=l.id WHERE a.target_id=$1 AND l.entry_type='commission_adjustment'),
		 (SELECT count(*) FROM audit_logs WHERE action='commission.adjustment.create' AND resource_type='commission_adjustment' AND resource_id::text IN(SELECT id::text FROM commission_adjustments WHERE target_id=$1)),
		 (SELECT count(*) FROM outbox_events WHERE event_type='commission.adjusted' AND payload->>'target_id'=$1::text)`, target.ID).
		Scan(&records, &ledger, &audits, &adjustmentNotices); err != nil {
		t.Fatal(err)
	}
	if records != 2 || ledger != 2 || audits != 2 || adjustmentNotices != 2 {
		t.Fatalf("adjustment writes records=%d ledger=%d audits=%d notices=%d; want 2 each", records, ledger, audits, adjustmentNotices)
	}
	var ledgerAfter, earningAfter string
	if err := f.betting.db.QueryRow(ctx, `SELECT md5(row_to_json(l)::text) FROM point_ledger_entries l WHERE l.id=$1`, *target.LedgerEntryID).Scan(&ledgerAfter); err != nil {
		t.Fatal(err)
	}
	if err := f.betting.db.QueryRow(ctx, `SELECT md5(row_to_json(e)::text) FROM commission_earnings e WHERE e.id=$1`, target.EarningID).Scan(&earningAfter); err != nil {
		t.Fatal(err)
	}
	if originalLedger != ledgerAfter || originalEarning != earningAfter {
		t.Fatalf("manual correction rewrote original paid evidence: ledger %s->%s earning %s->%s", originalLedger, ledgerAfter, originalEarning, earningAfter)
	}
	latest := commissionTargets(t, f, payment.ID)
	got := latest.Items[0]
	if got.AdjustmentVersion == nil || *got.AdjustmentVersion != 3 || got.AdjustedPoints == nil || *got.AdjustedPoints != 0 || got.LastAdjustmentID == nil || *got.LastAdjustmentID != down.ID || got.LedgerEntryID == nil || *got.LedgerEntryID != *target.LedgerEntryID {
		t.Fatalf("final target projection=%+v", got)
	}
	historyTx, err := f.betting.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	history, err := f.service.AdjustmentsTx(ctx, historyTx, f.betting.brand, target.ID, 100, 0)
	_ = historyTx.Rollback(ctx)
	if err != nil || len(history.Items) != 2 || history.Items[0].Version != 3 || history.Items[1].Version != 2 {
		t.Fatalf("adjustment history=%+v err=%v", history, err)
	}
}

func TestCommissionAdjustmentRejectsInsufficientCommissionAndConcurrentStaleWriters(t *testing.T) {
	f := newCommissionBatchFixture(t)
	ctx := context.Background()
	a := commissionAdjustmentActor(f)
	payment := payManualCommissionForAdjustment(t, f, prepareManualCommissionPayment(t, f))
	target := commissionTargets(t, f, payment.ID).Items[0]
	// Spend the only commission point while leaving the original betting
	// recharge balance available. A correction must not consume that source.
	var delta points.Balance
	delta[0][0] = 9
	delta[2][0] = 7
	fundTx, err := f.betting.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (points.Store{DB: f.betting.db}).Post(ctx, fundTx, points.Change{
		BrandID: f.betting.brand, MemberID: target.MemberID, EntryType: "adjustment", ReferenceType: "commission_adjustment_test",
		OperationKey: "fund-other-sources-" + ids.New(), Reason: "fund other sources for commission underflow assertion",
		ActorType: "admin", ActorID: a.ID, RequestID: ids.New(), Delta: delta,
		Allocation: []points.Allocation{{Source: "recharge", State: "available", Points: 9}, {Source: "gift", State: "available", Points: 7}},
	})
	if err != nil {
		_ = fundTx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = fundTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	delta = points.Balance{}
	delta[3][0] = -1
	spendTx, err := f.betting.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (points.Store{DB: f.betting.db}).Post(ctx, spendTx, points.Change{
		BrandID: f.betting.brand, MemberID: target.MemberID, EntryType: "adjustment", ReferenceType: "commission_adjustment_test",
		OperationKey: "spend-commission-" + ids.New(), Reason: "consume commission source for correction underflow test",
		ActorType: "admin", ActorID: a.ID, RequestID: ids.New(), Delta: delta,
		Allocation: []points.Allocation{{Source: "commission", State: "available", Points: 1}},
	})
	if err != nil {
		_ = spendTx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = spendTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	wallet := commissionWalletBySource(t, f, target.MemberID)
	if wallet[3][0] != 0 || wallet[0][0] != 9 || wallet[2][0] != 7 {
		t.Fatalf("underflow fixture must have R/G but no commission after spending C: %+v", wallet)
	}
	if _, err := adjustCommission(t, f, f.betting.brand, target, a, 1, 0); !errors.Is(err, commission.ErrAdjustmentInsufficient) {
		t.Fatalf("downward correction error=%v; want insufficient commission source", err)
	}
	if got := commissionWalletBySource(t, f, target.MemberID); got != wallet {
		t.Fatalf("rejected correction moved another wallet source: before=%+v after=%+v", wallet, got)
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := adjustCommission(t, f, f.betting.brand, target, a, 1, 2)
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	wins := 0
	busy := 0
	for err := range results {
		if err == nil {
			wins++
		} else if errors.Is(err, commission.ErrBusy) {
			busy++
		} else if !errors.Is(err, commission.ErrAdjustmentVersion) {
			t.Fatalf("same-version concurrent adjustment error=%v; want one version conflict", err)
		}
	}
	if wins != 1 {
		t.Fatalf("same-version concurrent adjustments successes=%d; want exactly one", wins)
	}
	if busy > 0 {
		if _, err := adjustCommission(t, f, f.betting.brand, target, a, 1, 2); !errors.Is(err, commission.ErrAdjustmentVersion) {
			t.Fatalf("same-head retry after NOWAIT contention error=%v; want version conflict", err)
		}
	}
}

func TestCommissionAdjustmentRequiresPaidTargetDedicatedPermissionAndOpenGate(t *testing.T) {
	f := newCommissionBatchFixture(t)
	ctx := context.Background()
	writer := commissionAdjustmentActor(f)
	payment := prepareManualCommissionPayment(t, f)
	if err := commissionPaymentCallTx(t, f, func(tx pgx.Tx) error {
		_, err := f.service.ApprovePaymentTx(ctx, tx, f.betting.brand, payment.ID, writer,
			commission.RetryCycleInput{Version: payment.Version, Reason: "approve to create pending target rejection case"}, commissionPaymentMeta(writer))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	payment = commissionPaymentRead(t, f, payment.ID)
	pendingID := createPendingCommissionTarget(t, f, payment)
	pending := commissionTargets(t, f, payment.ID).Items[0]
	if pending.ID != pendingID || pending.State != "pending" {
		t.Fatalf("pending target fixture=%+v", pending)
	}
	if _, err := adjustCommission(t, f, f.betting.brand, pending, writer, 1, 2); !errors.Is(err, commission.ErrAdjustmentState) {
		t.Fatalf("pending target adjustment error=%v; want state rejection", err)
	}
	viewer := writer
	for i := range viewer.Roles {
		if viewer.Roles[i].BrandID == f.betting.brand {
			viewer.Roles[i].Permissions = []access.Permission{{Resource: "commission", Action: "view", Scope: access.ScopeBrand}}
		}
	}
	if _, err := adjustCommission(t, f, f.betting.brand, pending, viewer, 1, 2); !errors.Is(err, commission.ErrDenied) {
		t.Fatalf("viewer adjustment error=%v; want denied", err)
	}
	super := writer
	super.SuperAdmin = true
	if _, err := adjustCommission(t, f, f.betting.brand, pending, super, 1, 2); !errors.Is(err, commission.ErrDenied) {
		t.Fatalf("super-admin adjustment error=%v; want denied", err)
	}
	foreign := writer
	foreign.BrandIDs = append(foreign.BrandIDs, storeTestOtherBrand)
	foreign.Roles = append(foreign.Roles, access.Role{BrandID: storeTestOtherBrand, Permissions: []access.Permission{
		{Resource: "commission", Action: "view", Scope: access.ScopeBrand},
		{Resource: "commission_adjustment", Action: "write", Scope: access.ScopeBrand},
	}})
	if _, err := adjustCommission(t, f, storeTestOtherBrand, pending, foreign, 1, 2); err == nil {
		t.Fatal("cross-brand adjustment was accepted")
	}

	paid := payManualCommissionForAdjustment(t, f, payment)
	target := commissionTargets(t, f, paid.ID).Items[0]
	policy := commissionPaymentPolicy(t, f)
	if err := updateCommissionPaymentPolicy(t, f, writer, false, policy.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := adjustCommission(t, f, f.betting.brand, target, writer, 1, 2); !errors.Is(err, commission.ErrAdjustmentState) {
		t.Fatalf("closed-gate adjustment error=%v; want state rejection", err)
	}
	if err := updateCommissionPaymentPolicy(t, f, writer, true, policy.Version+1); err != nil {
		t.Fatal(err)
	}
}

func TestCommissionAdjustmentBlockedOrChangedEpochIsFinanciallyInert(t *testing.T) {
	f := newCommissionBatchFixture(t)
	ctx := context.Background()
	a := commissionAdjustmentActor(f)
	payment := payManualCommissionForAdjustment(t, f, prepareManualCommissionPayment(t, f))
	target := commissionTargets(t, f, payment.ID).Items[0]
	if _, err := f.betting.db.Exec(ctx, `UPDATE commission_cycles SET evidence_epoch=evidence_epoch+1 WHERE brand_id=$1 AND id=$2`, f.betting.brand, payment.CycleID); err != nil {
		t.Fatal(err)
	}
	beforeChangedEpoch := readCommissionAdjustmentSnapshot(t, f, target)
	if _, err := adjustCommission(t, f, f.betting.brand, target, a, 1, 2); !errors.Is(err, commission.ErrPaymentEvidence) {
		t.Fatalf("changed-epoch post-paid adjustment error=%v; want invalid evidence", err)
	}
	if after := readCommissionAdjustmentSnapshot(t, f, target); after != beforeChangedEpoch {
		t.Fatalf("changed-epoch rejection wrote adjustment evidence:\nbefore=%+v\nafter=%+v", beforeChangedEpoch, after)
	}
	if _, err := f.betting.db.Exec(ctx, `UPDATE commission_payments SET next_work_at=clock_timestamp() WHERE brand_id=$1 AND id=$2`, f.betting.brand, payment.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessPayments(ctx, 20); err != nil {
		t.Fatal(err)
	}
	blocked := commissionPaymentRead(t, f, payment.ID)
	if blocked.State != "blocked" || blocked.PaidCount != "1" || blocked.PaidPoints != "1" {
		t.Fatalf("epoch change did not block the already-paid payment: %+v", blocked)
	}
	beforeBlocked := readCommissionAdjustmentSnapshot(t, f, target)
	if _, err := adjustCommission(t, f, f.betting.brand, target, a, 1, 2); !errors.Is(err, commission.ErrAdjustmentState) {
		t.Fatalf("blocked post-paid adjustment error=%v; want state rejection", err)
	}
	if after := readCommissionAdjustmentSnapshot(t, f, target); after != beforeBlocked {
		t.Fatalf("blocked-payment rejection wrote adjustment evidence:\nbefore=%+v\nafter=%+v", beforeBlocked, after)
	}
}

func TestCommissionAdjustmentSQLGuardsRejectMissingAndForgedEvidence(t *testing.T) {
	f := newCommissionBatchFixture(t)
	a := commissionAdjustmentActor(f)
	payment := payManualCommissionForAdjustment(t, f, prepareManualCommissionPayment(t, f))
	target := commissionTargets(t, f, payment.ID).Items[0]
	ctx := context.Background()

	// A head cannot be created for a target that does not exist.
	rejectAdjustmentTransactionWithoutWrites(t, f, target, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO commission_adjustment_heads(target_id,brand_id,version,points) VALUES($1,$2,1,1)`, ids.New(), f.betting.brand)
		return err
	})

	// A well-audited adjustment receipt without its ledger row cannot commit.
	rejectAdjustmentTransactionWithoutWrites(t, f, target, func(tx pgx.Tx) error {
		_, err := insertDirectCommissionAdjustment(ctx, tx, f, target, a, 2)
		return err
	})

	// A correct low-level ledger post still cannot commit unless the adjustment
	// row is bound back to that ledger entry in the same transaction.
	rejectAdjustmentTransactionWithoutWrites(t, f, target, func(tx pgx.Tx) error {
		adjustmentID, err := insertDirectCommissionAdjustment(ctx, tx, f, target, a, 2)
		if err != nil {
			return err
		}
		var delta points.Balance
		delta[3][0] = 1
		_, err = (points.Store{DB: f.betting.db}).Post(ctx, tx, points.Change{
			BrandID: f.betting.brand, MemberID: target.MemberID, EntryType: "commission_adjustment", ReferenceType: "commission_adjustment",
			ReferenceID: adjustmentID, OperationKey: "commission-adjustment:" + adjustmentID, Reason: "unbound adjustment ledger adversarial test",
			ActorType: "admin", ActorID: a.ID, RequestID: ids.New(), Delta: delta,
			Allocation: []points.Allocation{{Source: "commission", State: "available", Points: 1}},
		})
		return err
	})

	// The same branded business reference cannot smuggle a credit from a
	// different source through the low-level points ledger store.
	rejectAdjustmentTransactionWithoutWrites(t, f, target, func(tx pgx.Tx) error {
		adjustmentID, err := insertDirectCommissionAdjustment(ctx, tx, f, target, a, 2)
		if err != nil {
			return err
		}
		var delta points.Balance
		delta[1][0] = 1
		_, err = (points.Store{DB: f.betting.db}).Post(ctx, tx, points.Change{
			BrandID: f.betting.brand, MemberID: target.MemberID, EntryType: "commission_adjustment", ReferenceType: "commission_adjustment",
			ReferenceID: adjustmentID, OperationKey: "commission-adjustment:" + adjustmentID, Reason: "wrong source adjustment ledger adversarial test",
			ActorType: "admin", ActorID: a.ID, RequestID: ids.New(), Delta: delta,
			Allocation: []points.Allocation{{Source: "winning", State: "available", Points: 1}},
		})
		return err
	})
}

func TestCommissionAdjustmentHonorsCurrentPointAdjustmentAndBalanceCaps(t *testing.T) {
	f := newCommissionBatchFixture(t)
	a := commissionAdjustmentActor(f)
	payment := payManualCommissionForAdjustment(t, f, prepareManualCommissionPayment(t, f))
	target := commissionTargets(t, f, payment.ID).Items[0]
	ctx := context.Background()
	pointStore := points.Store{DB: f.betting.db}
	policy, err := pointStore.ReadPolicy(ctx, f.betting.brand)
	if err != nil {
		t.Fatal(err)
	}
	maxAdjustment := points.Amount(1)
	policy = saveCommissionPointCaps(t, f, a, policy.Version, nil, &maxAdjustment)
	beforeAdjustmentCap := readCommissionAdjustmentSnapshot(t, f, target)
	if _, err := adjustCommission(t, f, f.betting.brand, target, a, 1, 3); !errors.Is(err, points.ErrPolicyLimit) {
		t.Fatalf("over-cap adjustment error=%v; want point policy limit", err)
	}
	if after := readCommissionAdjustmentSnapshot(t, f, target); after != beforeAdjustmentCap {
		t.Fatalf("max_adjustment_points rejection changed financial state:\nbefore=%+v\nafter=%+v", beforeAdjustmentCap, after)
	}

	policy = saveCommissionPointCaps(t, f, a, policy.Version, nil, nil)
	wallet := commissionWalletBySource(t, f, target.MemberID)
	var currentTotal points.Amount
	for _, source := range wallet {
		for _, bucket := range source {
			currentTotal += bucket
		}
	}
	maxBalance := currentTotal + 1 // the requested +2 would exceed this cap
	policy = saveCommissionPointCaps(t, f, a, policy.Version, &maxBalance, nil)
	_ = policy
	beforeBalanceCap := readCommissionAdjustmentSnapshot(t, f, target)
	if _, err := adjustCommission(t, f, f.betting.brand, target, a, 1, 3); !errors.Is(err, points.ErrPolicyLimit) {
		t.Fatalf("over-cap net balance error=%v; want point policy limit", err)
	}
	if after := readCommissionAdjustmentSnapshot(t, f, target); after != beforeBalanceCap {
		t.Fatalf("max_balance_points rejection changed financial state:\nbefore=%+v\nafter=%+v", beforeBalanceCap, after)
	}
}
