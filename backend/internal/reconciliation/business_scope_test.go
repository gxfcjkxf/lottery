package reconciliation

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/finance"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

func createBusinessCheck(t *testing.T, s Service, actor access.Account) Job {
	t.Helper()
	ctx := context.Background()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	job, err := s.CreateScoped(ctx, tx, testBrand, actor, ScopeWalletAndBusiness, "check business and wallet evidence", points.Metadata{ActorType: "admin", ActorID: actor.ID, RequestID: ids.New()})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return job
}

func businessTargets(t *testing.T, s Service, job Job) []Target {
	t.Helper()
	ctx := context.Background()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	page, err := s.TargetsTx(ctx, tx, testBrand, job.ID, "", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	return page.Items
}

func postBusinessFixture(t *testing.T, s Service, member, kind string, count int) {
	t.Helper()
	ctx := context.Background()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var delta points.Balance
	delta[2][0] = 1
	for i := 0; i < count; i++ {
		_, err = (points.Store{DB: s.DB}).Post(ctx, tx, points.Change{BrandID: testBrand, MemberID: member, EntryType: kind, ReferenceType: "synthetic_business_check", OperationKey: "business-check-" + ids.New(), Reason: "synthetic integrity fixture", ActorType: "system", RequestID: ids.New(), Delta: delta, Allocation: []points.Allocation{{Source: "gift", State: "available", Points: 1}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestBusinessScopeSeparatesWalletTruthAndCountsAllUnknownIssues(t *testing.T) {
	s, actor := fixture(t)
	ctx := context.Background()
	member, _ := addMember(t, s, testBrand)
	postBusinessFixture(t, s, member, "adjustment", 1)
	unknownType := "Future:Kind." + strings.Repeat("X", 188)
	if len(unknownType) != 200 {
		t.Fatal("fixture does not exercise maximum ledger tag")
	}
	postBusinessFixture(t, s, member, unknownType, 101)
	walletJob := create(t, s, actor, testBrand)
	if _, err := s.Process(ctx, 20); err != nil {
		t.Fatal(err)
	}
	old := businessTargets(t, s, walletJob)
	if len(old) != 1 || old[0].CheckScope != ScopeWallet || old[0].BusinessPreview != nil || old[0].Outcome == nil || *old[0].Outcome != "consistent" {
		t.Fatalf("legacy wallet mode altered: %+v", old)
	}
	job := createBusinessCheck(t, s, actor)
	addMember(t, s, testBrand) // Captured scope must remain unchanged.
	before := moneyDigest(t, s)
	if _, err := s.Process(ctx, 20); err != nil {
		t.Fatal(err)
	}
	current := readJob(t, s, testBrand, job.ID)
	if current.State != "completed" || current.CheckedCount != "1" || current.CorruptCount != "1" || current.ConsistentCount != "0" {
		t.Fatalf("business issues falsely consistent or scope expanded: %+v", current)
	}
	targets := businessTargets(t, s, job)
	if len(targets) != 1 || targets[0].Preview == nil || !targets[0].Preview.Consistent || targets[0].Outcome == nil || *targets[0].Outcome != "corrupt" {
		t.Fatalf("wallet and business truth conflated: %+v", targets)
	}
	p := targets[0].BusinessPreview
	if p == nil || p.Consistent || p.IssueCount != "101" || p.LedgerEntryCount != "102" || p.BusinessReferenceCount != "1" || !p.IssuesTruncated || len(p.Issues) != 100 || len(p.Coverage) != 12 || len(p.Fingerprint) != 64 {
		t.Fatalf("incomplete business evidence: %+v", p)
	}
	for _, issue := range p.Issues {
		if issue.Code != "UNSUPPORTED_LEDGER_TYPE" || issue.EntryType == nil || *issue.EntryType != unknownType || issue.ResourceType != "ledger" {
			t.Fatalf("bad unknown-type evidence: %+v", issue)
		}
	}
	for _, coverage := range p.Coverage {
		if coverage.Family == "unknown" && (coverage.IssueCount != "101" || coverage.LedgerEntryCount != "101") {
			t.Fatal(coverage)
		}
	}
	if moneyDigest(t, s) != before {
		t.Fatal("business observation moved or repaired money")
	}
	if _, err := s.DB.Exec(ctx, `UPDATE point_reconciliation_jobs SET check_scope='wallet_and_business' WHERE id=$1`, walletJob.ID); err == nil {
		t.Fatal("historical wallet check relabelled as full coverage")
	}
}

func TestBusinessScopeAcceptsRealRechargeAndManualHistory(t *testing.T) {
	s, actor := fixture(t)
	ctx := context.Background()
	member, _ := addMember(t, s, testBrand)
	postBusinessFixture(t, s, member, "adjustment", 1)
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	service := finance.Service{DB: s.DB, Points: points.Store{DB: s.DB}}
	meta := points.Metadata{ActorType: "admin", ActorID: actor.ID, RequestID: ids.New()}
	r, err := service.CreateRecharge(ctx, tx, testBrand, member, 20, "synthetic source proof", "", "create authentic recharge", meta)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ConfirmRecharge(ctx, tx, testBrand, r.ID, r.Version, "confirm authentic recharge", meta); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	job := createBusinessCheck(t, s, actor)
	before := moneyDigest(t, s)
	if _, err = s.Process(ctx, 20); err != nil {
		t.Fatal(err)
	}
	targets := businessTargets(t, s, job)
	if len(targets) != 1 || targets[0].BusinessPreview == nil || !targets[0].BusinessPreview.Consistent || targets[0].BusinessPreview.BusinessReferenceCount != "2" || targets[0].BusinessPreview.LedgerEntryCount != "2" || *targets[0].Outcome != "consistent" {
		t.Fatalf("actual workflow rejected: %+v", targets)
	}
	if moneyDigest(t, s) != before {
		t.Fatal("read-only check changed financial history")
	}
}

func TestBusinessScopeRejectsForgedMatchingAuditAndPreview(t *testing.T) {
	s, actor := fixture(t)
	ctx := context.Background()
	member, account := addMember(t, s, testBrand)
	postBusinessFixture(t, s, member, "FutureKind", 1)
	job := createBusinessCheck(t, s, actor)
	target := businessTargets(t, s, job)[0]
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	preview, err := (points.Store{DB: s.DB}).PreviewRepairTx(ctx, tx, testBrand, member)
	if err != nil {
		t.Fatal(err)
	}
	var raw []byte
	if err = tx.QueryRow(ctx, `SELECT point_business_preview($1,$2)`, testBrand, account).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var fake BusinessPreview
	if err = json.Unmarshal(raw, &fake); err != nil {
		t.Fatal(err)
	}
	fake.Consistent = true
	fake.IssueCount = "0"
	fake.Issues = []BusinessIssue{}
	for i := range fake.Coverage {
		fake.Coverage[i].IssueCount = "0"
	}
	var at time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at); err != nil {
		t.Fatal(err)
	}
	auditID, err := audit.Append(ctx, tx, audit.Record{BrandID: testBrand, ActorType: "system", Action: "wallet.reconciliation.checked", ResourceType: "wallet_reconciliation_target", ResourceID: target.ID, RequestID: ids.New(), After: map[string]any{"job_id": job.ID, "account_id": account, "outcome": "consistent", "preview": preview, "business_preview": fake, "checked_at": at.UTC()}})
	if err != nil {
		t.Fatal(err)
	}
	walletRaw, _ := json.Marshal(preview)
	fakeRaw, _ := json.Marshal(fake)
	if _, err = tx.Exec(ctx, `INSERT INTO point_reconciliation_results(target_id,brand_id,job_id,outcome,preview,checked_at,audit_log_id,business_preview) VALUES($1,$2,$3,'consistent',$4,$5,$6,$7)`, target.ID, testBrand, job.ID, walletRaw, at, auditID, fakeRaw); err == nil {
		t.Fatal("matching forged audit and false full preview bypassed independent recomputation")
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	// A genuine business failure must not weaken the independent wallet proof.
	// Claim a wallet pass with a wrong expected balance and a correct business
	// preview; the combined corrupt outcome must still reject that lie.
	tx, err = s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var genuine BusinessPreview
	if err = json.Unmarshal(raw, &genuine); err != nil {
		t.Fatal(err)
	}
	preview.Expected = points.Balance{}
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at); err != nil {
		t.Fatal(err)
	}
	auditID, err = audit.Append(ctx, tx, audit.Record{BrandID: testBrand, ActorType: "system", Action: "wallet.reconciliation.checked", ResourceType: "wallet_reconciliation_target", ResourceID: target.ID, RequestID: ids.New(), After: map[string]any{"job_id": job.ID, "account_id": account, "outcome": "corrupt", "preview": preview, "business_preview": genuine, "checked_at": at.UTC()}})
	if err != nil {
		t.Fatal(err)
	}
	walletRaw, _ = json.Marshal(preview)
	if _, err = tx.Exec(ctx, `INSERT INTO point_reconciliation_results(target_id,brand_id,job_id,outcome,preview,checked_at,audit_log_id,business_preview) VALUES($1,$2,$3,'corrupt',$4,$5,$6,$7)`, target.ID, testBrand, job.ID, walletRaw, at, auditID, raw); err == nil {
		t.Fatal("business failure weakened wallet expected-balance proof")
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = s.DB.QueryRow(ctx, `SELECT count(*) FROM point_reconciliation_results WHERE job_id=$1`, job.ID).Scan(&n); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	if _, err = s.Process(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if got := readJob(t, s, testBrand, job.ID); got.State != "completed" || got.CorruptCount != "1" {
		t.Fatal(got)
	}
}

func TestBusinessScopeTechnicalFailureRequiresManualRetryAndKeepsPriorEvidence(t *testing.T) {
	s, actor := fixture(t)
	ctx := context.Background()
	addMember(t, s, testBrand)
	addMember(t, s, testBrand)
	job := createBusinessCheck(t, s, actor)
	before := moneyDigest(t, s)
	if _, err := s.Process(ctx, 1); err != nil {
		t.Fatal(err)
	}
	var originalEvidence, functionDefinition string
	if err := s.DB.QueryRow(ctx, `SELECT to_jsonb(r)::text FROM point_reconciliation_results r WHERE job_id=$1`, job.ID).Scan(&originalEvidence); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.QueryRow(ctx, `SELECT pg_get_functiondef('point_business_preview(uuid,uuid)'::regprocedure)`).Scan(&functionDefinition); err != nil {
		t.Fatal(err)
	}
	// Inject a diagnostic execution fault in this test's random schema only.
	if _, err := s.DB.Exec(ctx, `CREATE OR REPLACE FUNCTION point_business_preview(b uuid,account uuid) RETURNS jsonb LANGUAGE plpgsql STABLE AS $$ BEGIN RAISE EXCEPTION 'owned diagnostic fault'; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Process(ctx, 20); err != nil {
		t.Fatal(err)
	}
	failed := readJob(t, s, testBrand, job.ID)
	if failed.State != "failed" || failed.CheckedCount != "1" || failed.FailedCount != "1" || !failed.CanRetry || failed.CheckScope != ScopeWalletAndBusiness {
		t.Fatalf("full failure did not pause with prior success: %+v", failed)
	}
	if _, err := s.DB.Exec(ctx, functionDefinition); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Process(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if got := readJob(t, s, testBrand, job.ID); got.State != "failed" {
		t.Fatal("restoring infrastructure silently retried failure", got)
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = s.Retry(ctx, tx, testBrand, job.ID, actor, failed.Version, "explicitly retry diagnostic failure", points.Metadata{ActorType: "admin", ActorID: actor.ID, RequestID: ids.New()}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Process(ctx, 20); err != nil {
		t.Fatal(err)
	}
	got := readJob(t, s, testBrand, job.ID)
	if got.State != "completed" || got.CheckedCount != "2" || got.ConsistentCount != "2" || got.FailedCount != "0" || got.CheckScope != ScopeWalletAndBusiness {
		t.Fatal(got)
	}
	var retained bool
	if err = s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM point_reconciliation_results r WHERE job_id=$1 AND to_jsonb(r)::text=$2)`, job.ID, originalEvidence).Scan(&retained); err != nil || !retained {
		t.Fatal("manual retry rewrote earlier full observation", err)
	}
	if moneyDigest(t, s) != before {
		t.Fatal("failure or manual diagnostic retry moved money")
	}
}
