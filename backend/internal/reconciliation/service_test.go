package reconciliation

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
)

const testBrand = "0199a000-0000-7000-8000-000000000001"
const otherBrand = "0199a000-0000-7000-8000-000000000002"

func fixture(t *testing.T) (Service, access.Account) {
	t.Helper()
	s := Service{DB: testdb.New(t)}
	id := ids.New()
	if _, e := s.DB.Exec(context.Background(), `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'synthetic-only-hash')`, id, "reconcile_"+strings.ReplaceAll(id, "-", "")); e != nil {
		t.Fatal(e)
	}
	a := access.Account{ID: id, Type: access.AccountAdmin, BrandIDs: []string{testBrand, otherBrand}, Roles: []access.Role{{Permissions: []access.Permission{{Resource: "wallet", Action: "view", Scope: access.ScopeBrand}, {Resource: "wallet", Action: "reconcile", Scope: access.ScopeBrand}}}}}
	return s, a
}
func addMember(t *testing.T, s Service, brand string) (string, string) {
	t.Helper()
	u, m, a := ids.New(), ids.New(), ids.New()
	ctx := context.Background()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO global_users(id,username,password_hash) VALUES($1,$2,'synthetic-only-hash')`, []any{u, "recon_" + strings.ReplaceAll(u, "-", "")}},
		{`INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'domain','test','test')`, []any{m, brand, u}},
		{`INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, []any{a, brand, m}},
		{`INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT $1,$2,s,t FROM unnest(ARRAY['recharge','winning','gift'])s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal'])t`, []any{brand, a}},
	} {
		if _, e := s.DB.Exec(ctx, q.sql, q.args...); e != nil {
			t.Fatal(e)
		}
	}
	return m, a
}
func fund(t *testing.T, s Service, brand, member string, n int64) {
	t.Helper()
	ctx := context.Background()
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	var delta points.Balance
	delta[2][0] = points.Amount(n)
	_, e = (points.Store{DB: s.DB}).Post(ctx, tx, points.Change{BrandID: brand, MemberID: member, EntryType: "gift", ReferenceType: "reconciliation_test", OperationKey: "recon-" + ids.New(), Reason: "synthetic funding", ActorType: "system", RequestID: ids.New(), Delta: delta, Allocation: []points.Allocation{{Source: "gift", State: "available", Points: points.Amount(n)}}})
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
}
func create(t *testing.T, s Service, a access.Account, brand string) Job {
	t.Helper()
	ctx := context.Background()
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	j, e := s.Create(ctx, tx, brand, a, "check captured wallets", points.Metadata{ActorType: "admin", ActorID: a.ID, RequestID: ids.New()})
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	return j
}
func readJob(t *testing.T, s Service, brand, id string) Job {
	t.Helper()
	ctx := context.Background()
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	j, e := s.ReadTx(ctx, tx, brand, id)
	if e != nil {
		t.Fatal(e)
	}
	return j
}
func moneyDigest(t *testing.T, s Service) string {
	t.Helper()
	var hash string
	e := s.DB.QueryRow(context.Background(), `SELECT md5(jsonb_build_object('accounts',(SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM point_accounts a),'buckets',(SELECT jsonb_agg(to_jsonb(b) ORDER BY account_id,source,state) FROM point_buckets b),'entries',(SELECT jsonb_agg(to_jsonb(e) ORDER BY id) FROM point_ledger_entries e),'repairs',(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM point_balance_repairs r))::text)`).Scan(&hash)
	if e != nil {
		t.Fatal(e)
	}
	return hash
}
func TestChecksCaptureScopeAndNeverRepairOrRewriteMoney(t *testing.T) {
	s, a := fixture(t)
	ctx := context.Background()
	m1, _ := addMember(t, s, testBrand)
	m2, account2 := addMember(t, s, testBrand)
	m3, account3 := addMember(t, s, testBrand)
	fund(t, s, testBrand, m1, 11)
	fund(t, s, testBrand, m2, 7)
	fund(t, s, testBrand, m3, 5)
	if _, e := s.DB.Exec(ctx, `UPDATE point_buckets SET points=99 WHERE account_id=$1 AND source='gift' AND state='available';`, account2); e != nil {
		t.Fatal(e)
	}
	if _, e := s.DB.Exec(ctx, `DELETE FROM point_buckets WHERE account_id=$1 AND source='gift' AND state='withdrawal'`, account2); e != nil {
		t.Fatal(e)
	}
	// Fault injection only in testdb's owned schema: temporarily disable the
	// immutable UPDATE guard, corrupt one hash, then restore the guard before
	// running reconciliation. This simulates stored damage, not a normal write.
	var trigger string
	if e := s.DB.QueryRow(ctx, `SELECT tgname FROM pg_trigger WHERE tgrelid='point_ledger_entries'::regclass AND NOT tgisinternal AND (tgtype & 2)=2 AND (tgtype & 16)=16 LIMIT 1`).Scan(&trigger); e != nil {
		t.Fatal(e)
	}
	quoted := pgx.Identifier{trigger}.Sanitize()
	if _, e := s.DB.Exec(ctx, `ALTER TABLE point_ledger_entries DISABLE TRIGGER `+quoted); e != nil {
		t.Fatal(e)
	}
	_, e := s.DB.Exec(ctx, `UPDATE point_ledger_entries SET request_hash=repeat('0',64) WHERE account_id=$1`, account3)
	_, enableErr := s.DB.Exec(ctx, `ALTER TABLE point_ledger_entries ENABLE TRIGGER `+quoted)
	if e != nil || enableErr != nil {
		t.Fatal("owned corruption injection failed", e, enableErr)
	}
	j := create(t, s, a, testBrand)
	if j.TargetCount != "3" {
		t.Fatal(j)
	}
	addMember(t, s, testBrand) // Not part of the captured scope.
	before := moneyDigest(t, s)
	if _, e = s.Process(ctx, 20); e != nil {
		t.Fatal(e)
	}
	live := readJob(t, s, testBrand, j.ID)
	if live.State != "completed" || live.CheckedCount != "3" || live.ConsistentCount != "1" || live.RepairableCount != "1" || live.CorruptCount != "1" || live.PendingCount != "0" {
		t.Fatal(live)
	}
	if moneyDigest(t, s) != before {
		t.Fatal("check changed wallet/ledger/repair data")
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	p, e := s.TargetsTx(ctx, tx, testBrand, j.ID, "repairable", 20, 0)
	if e != nil || p.TotalCount != "1" || len(p.Items) != 1 || p.Items[0].Preview == nil || p.Items[0].Preview.Actual["gift"]["available"] != 99 || p.Items[0].Preview.Expected[2][0] != 7 {
		t.Fatal(p, e)
	}
	if _, e = s.ReadTx(ctx, tx, otherBrand, j.ID); !errors.Is(e, ErrNotFound) {
		t.Fatal("cross brand job leak", e)
	}
	for _, q := range []string{`UPDATE point_reconciliation_results SET outcome='consistent'`, `DELETE FROM point_reconciliation_results`, `DELETE FROM point_reconciliation_jobs`, `UPDATE point_reconciliation_targets SET state='pending' WHERE state='checked'`} {
		if _, e = s.DB.Exec(ctx, q); e == nil {
			t.Fatal("immutable evidence mutation allowed", q)
		}
	}
	lateMember, lateAccount := addMember(t, s, testBrand)
	if _, e = s.DB.Exec(ctx, `INSERT INTO point_reconciliation_targets(id,brand_id,job_id,account_id,member_id) VALUES($1,$2,$3,$4,$5)`, ids.New(), testBrand, j.ID, lateAccount, lateMember); e == nil {
		t.Fatal("late target insertion allowed")
	}
}
func TestHotWalletDoesNotBlockOtherBrandAndMultipleWorkersDoNotDuplicate(t *testing.T) {
	s, a := fixture(t)
	ctx := context.Background()
	_, busy := addMember(t, s, testBrand)
	addMember(t, s, otherBrand)
	first := create(t, s, a, testBrand)
	second := create(t, s, a, otherBrand)
	lock, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer lock.Rollback(ctx)
	if _, e = lock.Exec(ctx, `SELECT id FROM point_accounts WHERE id=$1 FOR UPDATE`, busy); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Process(ctx, 2); e != nil {
		t.Fatal(e)
	}
	if got := readJob(t, s, otherBrand, second.ID); got.State != "completed" {
		t.Fatal("hot peer blocked other brand", got)
	}
	if got := readJob(t, s, testBrand, first.ID); got.CheckedCount != "0" {
		t.Fatal(got)
	}
	if e = lock.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(ctx, `UPDATE point_reconciliation_targets SET next_check_at=clock_timestamp() WHERE job_id=$1`, first.ID); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := s.Process(ctx, 10); errs <- e }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if got := readJob(t, s, testBrand, first.ID); got.State != "completed" || got.CheckedCount != "1" {
		t.Fatal(got)
	}
	var count int
	if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM point_reconciliation_results WHERE job_id=$1`, first.ID).Scan(&count); e != nil || count != 1 {
		t.Fatal(count, e)
	}
}
func TestEmptyScopeCompletesAndJobCannotFakeCompletion(t *testing.T) {
	s, a := fixture(t)
	ctx := context.Background()
	j := create(t, s, a, testBrand)
	if _, e := s.Process(ctx, 1); e != nil {
		t.Fatal(e)
	}
	if got := readJob(t, s, testBrand, j.ID); got.State != "completed" || got.TargetCount != "0" || got.CompletedAt == nil {
		t.Fatal(got)
	}
	addMember(t, s, testBrand)
	j = create(t, s, a, testBrand)
	lateMember, lateAccount := addMember(t, s, testBrand)
	if _, e := s.DB.Exec(ctx, `INSERT INTO point_reconciliation_targets(id,brand_id,job_id,account_id,member_id) VALUES($1,$2,$3,$4,$5)`, ids.New(), testBrand, j.ID, lateAccount, lateMember); e == nil {
		t.Fatal("scope expanded after creation commit while job still pending/v1")
	}
	_, e := s.DB.Exec(ctx, `UPDATE point_reconciliation_jobs SET state='completed',version=version+1,started_at=clock_timestamp(),completed_at=clock_timestamp() WHERE id=$1`, j.ID)
	if e == nil {
		t.Fatal("unfinished targets falsely completed")
	}
}

func TestWorkerAuditFailureIsAtomicAndPausesWithFailureEvidence(t *testing.T) {
	s, a := fixture(t)
	ctx := context.Background()
	addMember(t, s, testBrand)
	j := create(t, s, a, testBrand)
	before := moneyDigest(t, s)
	_, e := s.DB.Exec(ctx, `CREATE FUNCTION reject_reconciliation_checked_audit() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN IF NEW.action='wallet.reconciliation.checked' THEN RAISE EXCEPTION 'injected';END IF;RETURN NEW;END$$;CREATE TRIGGER reject_reconciliation_checked_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_reconciliation_checked_audit()`)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Process(ctx, 1); e != nil {
		t.Fatal(e)
	}
	got := readJob(t, s, testBrand, j.ID)
	if got.State != "failed" || got.CheckedCount != "0" || got.FailedCount != "1" {
		t.Fatal(got)
	}
	var results, successAudits int
	if e = s.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM point_reconciliation_results),(SELECT count(*) FROM audit_logs WHERE action='wallet.reconciliation.checked')`).Scan(&results, &successAudits); e != nil || results != 0 || successAudits != 0 {
		t.Fatal(results, successAudits, e)
	}
	if moneyDigest(t, s) != before {
		t.Fatal("failed observation changed money")
	}
}

func TestRetryPreservesPriorObservationAndOriginalStartTime(t *testing.T) {
	s, a := fixture(t)
	ctx := context.Background()
	addMember(t, s, testBrand)
	addMember(t, s, testBrand)
	j := create(t, s, a, testBrand)
	if _, e := s.Process(ctx, 1); e != nil {
		t.Fatal(e)
	}
	running := readJob(t, s, testBrand, j.ID)
	if running.StartedAt == nil || running.CheckedCount != "1" {
		t.Fatal(running)
	}
	var target, checked, hash string
	if e := s.DB.QueryRow(ctx, `SELECT id::text FROM point_reconciliation_targets WHERE job_id=$1 AND state='pending'`, j.ID).Scan(&target); e != nil {
		t.Fatal(e)
	}
	if e := s.DB.QueryRow(ctx, `SELECT target_id::text,md5(to_jsonb(r)::text) FROM point_reconciliation_results r WHERE job_id=$1`, j.ID).Scan(&checked, &hash); e != nil {
		t.Fatal(e)
	}
	if e := s.recordFailure(ctx, testBrand, j.ID, target, 0); e != nil {
		t.Fatal(e)
	}
	failed := readJob(t, s, testBrand, j.ID)
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	retried, e := s.Retry(ctx, tx, testBrand, j.ID, a, failed.Version, "resume pending targets only", points.Metadata{ActorType: "admin", ActorID: a.ID, RequestID: ids.New()})
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if retried.StartedAt == nil || !retried.StartedAt.Equal(*running.StartedAt) || retried.State != "pending" || retried.CheckedCount != "1" || retried.PendingCount != "1" || retried.FailedCount != "0" {
		t.Fatal(retried)
	}
	if _, e = s.Process(ctx, 20); e != nil {
		t.Fatal(e)
	}
	var after string
	if e = s.DB.QueryRow(ctx, `SELECT md5(to_jsonb(r)::text) FROM point_reconciliation_results r WHERE target_id=$1`, checked).Scan(&after); e != nil || after != hash {
		t.Fatal("retry changed old observation", e)
	}
}
func TestReadTimeoutPausesForManualRetryAndPreservesCheckedTargets(t *testing.T) {
	s, a := fixture(t)
	ctx := context.Background()
	member, _ := addMember(t, s, testBrand)
	fund(t, s, testBrand, member, 3)
	j := create(t, s, a, testBrand)
	lock, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer lock.Rollback(ctx)
	if _, e = lock.Exec(ctx, `LOCK TABLE point_ledger_entries IN ACCESS EXCLUSIVE MODE`); e != nil {
		t.Fatal(e)
	}
	started := time.Now()
	_, e = s.Process(ctx, 1)
	if e != nil {
		t.Fatal(e)
	}
	if time.Since(started) > 5*time.Second {
		t.Fatal("unbounded check")
	}
	if e = lock.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	failed := readJob(t, s, testBrand, j.ID)
	if failed.State != "failed" || !failed.CanRetry || failed.FailedCount != "1" {
		t.Fatal(failed)
	}
	if n, e := s.Process(ctx, 20); e != nil || n != 0 {
		t.Fatal("failed check automatically retried", n, e)
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	_, e = s.Retry(ctx, tx, testBrand, j.ID, a, failed.Version, "inspect timeout and resume", points.Metadata{ActorType: "admin", ActorID: a.ID, RequestID: ids.New()})
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Process(ctx, 20); e != nil {
		t.Fatal(e)
	}
	got := readJob(t, s, testBrand, j.ID)
	if got.State != "completed" || got.ConsistentCount != "1" || got.FailedCount != "0" {
		t.Fatal(got)
	}
	var failures, retries, attempts int
	if e = s.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM point_reconciliation_failures),(SELECT count(*) FROM point_reconciliation_retries),(SELECT attempt_count FROM point_reconciliation_targets WHERE job_id=$1)`, j.ID).Scan(&failures, &retries, &attempts); e != nil || failures != 1 || retries != 1 || attempts != 2 {
		t.Fatal(failures, retries, attempts, e)
	}
}
