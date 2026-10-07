package points

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func addSafetyMember(t *testing.T, p *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()
	userID, memberID, accountID := ids.New(), ids.New(), ids.New()
	queries := []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO global_users(id,username,password_hash) VALUES($1,$2,'test-only-hash')", []any{userID, "safety_" + strings.ReplaceAll(userID, "-", "")}},
		{"INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'domain','dev-1','dev-1')", []any{memberID, testBrand, userID}},
		{"INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)", []any{accountID, testBrand, memberID}},
		{"INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT $1,$2,s,t FROM unnest(ARRAY['recharge','winning','gift','commission']) s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal']) t ON CONFLICT(brand_id,account_id,source,state) DO NOTHING", []any{testBrand, accountID}},
	}
	for _, q := range queries {
		if _, err := p.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	var bucketCount int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM point_buckets WHERE brand_id=$1 AND account_id=$2`, testBrand, accountID).Scan(&bucketCount); err != nil || bucketCount != 16 {
		t.Fatalf("fixture bucket count=%d err=%v", bucketCount, err)
	}
	return memberID
}

func TestPostRejectsAllocationThatClaimsAnotherSource(t *testing.T) {
	p, s, member := fixture(t)
	ctx := context.Background()
	var delta Balance
	delta[0][0] = 9
	c := change(member, "fake-source-allocation", delta, []Allocation{{Source: "winning", State: "available", Points: 9}})
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Post(ctx, tx, c)
	_ = tx.Rollback(ctx)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("fake source allocation error=%v, want ErrInvalid", err)
	}
	var entries, audits int
	if err := p.QueryRow(ctx, "SELECT count(*) FROM point_ledger_entries WHERE operation_key=$1", c.OperationKey).Scan(&entries); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE action='points.adjustment'").Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if entries != 0 || audits != 0 {
		t.Fatalf("rejected allocation wrote records: entries=%d audits=%d", entries, audits)
	}
}

func TestConcurrentSameOperationKeyProducesOneLedgerAndAudit(t *testing.T) {
	p, s, member := fixture(t)
	ctx := context.Background()
	var delta Balance
	delta[0][0] = 17
	base := change(member, "safety-same-operation", delta, []Allocation{{Source: "recharge", State: "available", Points: 17}})
	const workers = 20
	var wg sync.WaitGroup
	idsOut := make(chan string, workers)
	errs := make(chan error, workers)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			tx, err := p.Begin(ctx)
			if err != nil {
				errs <- err
				return
			}
			defer tx.Rollback(ctx)
			c := base
			c.RequestID = ids.New()
			entry, err := s.Post(ctx, tx, c)
			if err != nil {
				errs <- err
				return
			}
			if err = tx.Commit(ctx); err != nil {
				errs <- err
				return
			}
			idsOut <- entry.ID
		}()
	}
	close(start)
	wg.Wait()
	close(idsOut)
	close(errs)
	for err := range errs {
		t.Errorf("same-key replay returned error: %v", err)
	}
	var entryID string
	for got := range idsOut {
		if entryID == "" {
			entryID = got
		} else if got != entryID {
			t.Errorf("replay returned another ledger entry: %s != %s", got, entryID)
		}
	}
	if entryID == "" {
		t.Fatal("no concurrent operation committed")
	}
	var entryCount, auditCount int
	if err := p.QueryRow(ctx, "SELECT count(*) FROM point_ledger_entries WHERE brand_id=$1 AND operation_key=$2", testBrand, base.OperationKey).Scan(&entryCount); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE action='points.adjustment' AND resource_id=(SELECT id FROM point_accounts WHERE brand_member_id=$1)", member).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if entryCount != 1 || auditCount != 1 {
		t.Fatalf("same-key concurrency wrote entry/audit counts %d/%d", entryCount, auditCount)
	}
}

func TestCrossAccountConcurrentOperationKeyMapsUniqueRaceToConflict(t *testing.T) {
	p, s, memberA := fixture(t)
	memberB := addSafetyMember(t, p)
	ctx := context.Background()
	var deltaA, deltaB Balance
	deltaA[0][0] = 11
	deltaB[0][0] = 13
	key := "safety-cross-account-key"
	cA := change(memberA, key, deltaA, []Allocation{{Source: "recharge", State: "available", Points: 11}})
	cB := change(memberB, key, deltaB, []Allocation{{Source: "recharge", State: "available", Points: 13}})
	txA, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Post(ctx, txA, cA); err != nil {
		_ = txA.Rollback(ctx)
		t.Fatal(err)
	}
	defer txA.Rollback(ctx)
	txB, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer txB.Rollback(ctx)
	result := make(chan error, 1)
	go func() {
		_, postErr := s.Post(ctx, txB, cB)
		result <- postErr
	}()

	deadline := time.Now().Add(5 * time.Second)
	waiting := false
	for time.Now().Before(deadline) {
		var found bool
		err = p.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM pg_stat_activity
			WHERE pid<>pg_backend_pid() AND state='active' AND wait_event_type='Lock'
			AND query ILIKE '%INSERT INTO point_ledger_entries%'
		)`).Scan(&found)
		if err != nil {
			_ = txA.Rollback(ctx)
			t.Fatal(err)
		}
		if found {
			waiting = true
			break
		}
		select {
		case postErr := <-result:
			_ = txA.Rollback(ctx)
			t.Fatalf("second insert returned before first commit (err=%v), expected unique-index wait", postErr)
		case <-time.After(10 * time.Millisecond):
		}
	}
	if !waiting {
		_ = txA.Rollback(ctx)
		select {
		case <-result:
		case <-time.After(time.Second):
		}
		t.Fatal("second account did not wait on the uncommitted brand operation key")
	}
	if err = txA.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var secondErr error
	select {
	case secondErr = <-result:
	case <-time.After(5 * time.Second):
		t.Fatal("second Post did not finish after the first commit")
	}
	if !errors.Is(secondErr, ErrConflict) {
		var pgErr *pgconn.PgError
		if errors.As(secondErr, &pgErr) {
			t.Fatalf("cross-account operation key race returned PostgreSQL %s (%s), want ErrConflict", pgErr.Code, pgErr.ConstraintName)
		}
		t.Fatalf("cross-account operation key race error=%v, want ErrConflict", secondErr)
	}
	var count int
	if err = p.QueryRow(ctx, "SELECT count(*) FROM point_ledger_entries WHERE brand_id=$1 AND operation_key=$2", testBrand, key).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("cross-account key produced %d ledger entries, want one", count)
	}
	var audits int
	if err = p.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE action='points.adjustment' AND resource_id IN (SELECT id FROM point_accounts WHERE brand_member_id IN ($1,$2))", memberA, memberB).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("cross-account key produced %d point audits, want one", audits)
	}
}

func TestSnapshotAndLockedSnapshotSerializeRowLocks(t *testing.T) {
	p, s, member := fixture(t)
	ctx := context.Background()
	sharedTx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Snapshot(ctx, sharedTx, testBrand, member); err != nil {
		_ = sharedTx.Rollback(ctx)
		t.Fatal(err)
	}
	lockedTx, err := p.Begin(ctx)
	if err != nil {
		_ = sharedTx.Rollback(ctx)
		t.Fatal(err)
	}
	defer lockedTx.Rollback(ctx)
	locked := make(chan error, 1)
	go func() {
		_, lockErr := s.LockedSnapshot(ctx, lockedTx, testBrand, member)
		locked <- lockErr
	}()
	select {
	case err = <-locked:
		_ = sharedTx.Rollback(ctx)
		t.Fatalf("exclusive snapshot passed an outstanding shared snapshot: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err = sharedTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-locked:
		if err != nil {
			t.Fatalf("locked snapshot after shared release: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("locked snapshot remained blocked after shared transaction committed")
	}
	if err = lockedTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestLedgerMaxInt64BoundaryRejectsAggregateOverflow(t *testing.T) {
	p, s, member := fixture(t)
	ctx := context.Background()
	credit(t, p, s, member, 0, Amount(math.MaxInt64))
	before, err := s.Read(ctx, testBrand, member)
	if err != nil || before.BySource[0][0] != Amount(math.MaxInt64) || before.Version != 1 {
		t.Fatalf("MaxInt64 credit did not persist exactly: wallet=%+v err=%v", before, err)
	}
	var delta Balance
	delta[2][0] = 1
	c := change(member, "safety-maxint64-plus-one", delta, []Allocation{{Source: "gift", State: "available", Points: 1}})
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Post(ctx, tx, c)
	_ = tx.Rollback(ctx)
	if !errors.Is(err, ErrOverflow) {
		t.Fatalf("aggregate MaxInt64+1 error=%v, want ErrOverflow", err)
	}
	after, err := s.Read(ctx, testBrand, member)
	if err != nil || after.BySource != before.BySource || after.Version != before.Version {
		t.Fatalf("overflow attempt altered balance/version: before=%+v after=%+v err=%v", before, after, err)
	}
}

func TestSystemFreezeAndWithdrawalRoundTripTotals(t *testing.T) {
	p, s, member := fixture(t)
	ctx := context.Background()
	credit(t, p, s, member, 0, 40)
	credit(t, p, s, member, 2, 10)
	initial, err := s.Read(ctx, testBrand, member)
	if err != nil || initial.DisplayPoints != 50 {
		t.Fatalf("initial wallet=%+v err=%v", initial, err)
	}
	freezeAlloc, err := initial.BySource.Allocate(20, "available")
	if err != nil {
		t.Fatal(err)
	}
	freezeDelta, err := AllocationDelta(freezeAlloc, "available", "system_frozen")
	if err != nil {
		t.Fatal(err)
	}
	freeze := change(member, "safety-system-freeze", freezeDelta, freezeAlloc)
	freeze.EntryType = "freeze"
	freezeEntry := post(t, p, s, freeze)
	withdrawWallet, err := s.Read(ctx, testBrand, member)
	if err != nil || withdrawWallet.AvailablePoints != 30 || withdrawWallet.SystemFrozenPoints != 20 || withdrawWallet.ManualFrozenPoints != 0 || withdrawWallet.FrozenPoints != 20 || withdrawWallet.DisplayPoints != 50 || withdrawWallet.WithdrawalPoints != 0 {
		t.Fatalf("system-frozen wallet totals=%+v err=%v", withdrawWallet, err)
	}
	withdrawAlloc, err := withdrawWallet.BySource.Allocate(7, "system_frozen")
	if err != nil {
		t.Fatal(err)
	}
	withdrawDelta, err := AllocationDelta(withdrawAlloc, "system_frozen", "withdrawal")
	if err != nil {
		t.Fatal(err)
	}
	withdraw := change(member, "safety-withdrawal-transfer", withdrawDelta, withdrawAlloc)
	withdraw.EntryType = "withdrawal_transfer"
	withdrawEntry := post(t, p, s, withdraw)
	inWithdrawal, err := s.Read(ctx, testBrand, member)
	if err != nil {
		t.Fatal(err)
	}
	rechargeTotal, err := inWithdrawal.BySource.SourceTotal(0)
	if err != nil || inWithdrawal.SystemFrozenPoints != 13 || inWithdrawal.WithdrawalPoints != 7 || inWithdrawal.FrozenPoints != 13 || inWithdrawal.DisplayPoints != 43 || rechargeTotal != 40 {
		t.Fatalf("withdrawal wallet totals=%+v err=%v", inWithdrawal, err)
	}
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Reverse(ctx, tx, testBrand, member, withdrawEntry.ID, "safety-withdrawal-reverse", "restore withdrawal transfer", Metadata{ActorType: "system", RequestID: ids.New()}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	backFromWithdrawal, err := s.Read(ctx, testBrand, member)
	if err != nil || backFromWithdrawal.BySource != withdrawWallet.BySource || backFromWithdrawal.DisplayPoints != 50 || backFromWithdrawal.WithdrawalPoints != 0 {
		t.Fatalf("withdrawal reversal failed to restore: wallet=%+v err=%v", backFromWithdrawal, err)
	}
	tx, err = p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Reverse(ctx, tx, testBrand, member, freezeEntry.ID, "safety-freeze-reverse", "restore original available points", Metadata{ActorType: "system", RequestID: ids.New()}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	restored, err := s.Read(ctx, testBrand, member)
	if err != nil || restored.BySource != initial.BySource || restored.DisplayPoints != 50 || restored.FrozenPoints != 0 || restored.WithdrawalPoints != 0 {
		t.Fatalf("system freeze round trip failed: wallet=%+v err=%v", restored, err)
	}
}
