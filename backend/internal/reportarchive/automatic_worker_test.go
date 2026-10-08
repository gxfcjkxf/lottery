package reportarchive

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
)

func automaticActor(a access.Account) access.Account {
	a.Roles = append(a.Roles, access.Role{BrandID: testBrand, Permissions: []access.Permission{{Resource: "report_archive_policy", Action: "write", Scope: access.ScopeBrand}, {Resource: "report_archive_task", Action: "retry", Scope: access.ScopeBrand}}})
	return a
}
func automaticPolicy(t *testing.T, s Service, a access.Account, in AutomaticPolicyInput) AutomaticPolicy {
	t.Helper()
	ctx := context.Background()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	p, err := s.UpdateAutomaticPolicyTx(ctx, tx, testBrand, automaticActor(a), in, archiveMeta(a))
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return p
}
func automaticPast(t *testing.T, s Service, a access.Account, days int) AutomaticPolicy {
	key := automaticBrandNow(t, s).AddDate(0, 0, -days).Format("2006-01-02")
	return automaticPolicy(t, s, a, AutomaticPolicyInput{Version: 1, DailyEnabled: true, DailyStartPeriod: &key, Reason: "explicit saved start for isolated core proof"})
}

// Discovery uses the database clock and brand civil calendar, not the test
// process's UTC date. Before 00:00 UTC these can already be different days.
func automaticBrandNow(t *testing.T, s Service) time.Time {
	t.Helper()
	var at time.Time
	var zone string
	if err := s.DB.QueryRow(context.Background(), `SELECT statement_timestamp(),timezone FROM brands WHERE id=$1`, testBrand).Scan(&at, &zone); err != nil {
		t.Fatal(err)
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		t.Fatal(err)
	}
	return at.In(loc)
}
func automaticTasks(t *testing.T, s Service) []AutomaticTask {
	t.Helper()
	ctx := context.Background()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	p, err := s.AutomaticTasksTx(ctx, tx, testBrand, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	return p.Items
}

func TestAutomaticArchiveDefaultDisabledDiscoveryVersionsAndActualSnapshot(t *testing.T) {
	t.Parallel()
	s, a := archiveFixture(t)
	ctx := context.Background()
	archiveFund(t, s)
	money := archiveMoney(t, s)
	if n, e := s.DiscoverAutomatic(ctx, 20); e != nil || n != 0 {
		t.Fatal(n, e)
	}
	automaticPast(t, s, a, 2)
	if n, e := s.DiscoverAutomatic(ctx, 1); e != nil || n != 1 {
		t.Fatal("bounded first discovery", n, e)
	}
	if n, e := s.DiscoverAutomatic(ctx, 20); e != nil || n != 1 {
		t.Fatal("remaining window", n, e)
	}
	if n, e := s.DiscoverAutomatic(ctx, 20); e != nil || n != 0 {
		t.Fatal("duplicate discovery", n, e)
	}
	if n, e := s.ProcessAutomatic(ctx, 20); e != nil || n != 2 {
		t.Fatal("actual processing", n, e)
	}
	for _, task := range automaticTasks(t, s) {
		if task.State != "completed" || task.ArchiveID == nil || task.AttemptCount != 1 || task.Version != 2 {
			t.Fatal("task completion", task)
		}
		r := archiveRead(t, s, *task.ArchiveID)
		if r.CreatedBy != nil || r.Automation == nil || r.Automation.TaskID != task.ID || r.Automation.PolicyVersion != 2 || r.Revision != 1 || r.Snapshot.WalletSnapshot.Balances.AvailablePoints != "13" {
			t.Fatal("system source/snapshot", r)
		}
		if r.Snapshot.Ledger.EntryCount != "0" {
			t.Fatal("present-day posting shifted into past period", r.Snapshot.Ledger)
		}
	}
	if n, e := s.ProcessAutomatic(ctx, 20); e != nil || n != 0 {
		t.Fatal("repeated processing", n, e)
	}
	if archiveMoney(t, s) != money {
		t.Fatal("report jobs moved money")
	}
	task := automaticTasks(t, s)[0]
	r := archiveRead(t, s, *task.ArchiveID)
	next := archiveCreate(t, s, a, Input{Kind: r.Window.Kind, PeriodKey: r.Window.PeriodKey, ExpectedRevision: 1, Reason: "manual later observation of automatic first version"})
	if next.Automation != nil || next.CreatedBy == nil || *next.CreatedBy != a.ID || next.PreviousID == nil || *next.PreviousID != r.ID || next.Window != r.Window {
		t.Fatal("manual append lost automatic original", next)
	}
}

func TestAutomaticArchiveMonthlyConcurrentDiscoveryAndDisabledBrand(t *testing.T) {
	t.Parallel()
	s, a := archiveFixture(t)
	ctx := context.Background()
	archiveFund(t, s)
	money := archiveMoney(t, s)
	// Start at the previous calendar month, not a fixed number of elapsed days.
	now := automaticBrandNow(t, s)
	key := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, -1, 0).Format("2006-01")
	automaticPolicy(t, s, a, AutomaticPolicyInput{Version: 1, MonthlyEnabled: true, MonthlyStartPeriod: &key, Reason: "explicit monthly core proof"})
	var wg sync.WaitGroup
	results := make(chan int, 2)
	failures := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := s.DiscoverAutomatic(ctx, 20)
			results <- n
			failures <- err
		}()
	}
	wg.Wait()
	if err := <-failures; err != nil {
		t.Fatal(err)
	}
	if err := <-failures; err != nil {
		t.Fatal(err)
	}
	if total := <-results + <-results; total != 1 {
		t.Fatal("concurrent discovery duplicated month", total)
	}
	if _, err := s.DB.Exec(ctx, `UPDATE brands SET status='disabled' WHERE id=$1`, testBrand); err != nil {
		t.Fatal(err)
	}
	if n, err := s.ProcessAutomatic(ctx, 20); err != nil || n != 0 {
		t.Fatal("disabled brand processed archive", n, err)
	}
	if tasks := automaticTasks(t, s); len(tasks) != 1 || tasks[0].State != "pending" {
		t.Fatal("disabled brand discarded queue", tasks)
	}
	if _, err := s.DB.Exec(ctx, `UPDATE brands SET status='paused' WHERE id=$1`, testBrand); err != nil {
		t.Fatal(err)
	}
	if n, err := s.ProcessAutomatic(ctx, 20); err != nil || n != 1 {
		t.Fatal("paused brand refused reporting", n, err)
	}
	task := automaticTasks(t, s)[0]
	if task.Window.Kind != Monthly || task.Window.PeriodKey != key || task.ArchiveID == nil || task.State != "completed" || archiveMoney(t, s) != money {
		t.Fatal("monthly capture changed money or scope", task)
	}
	r := archiveRead(t, s, *task.ArchiveID)
	if r.Snapshot.WalletSnapshot.Balances.AvailablePoints != "13" || r.Snapshot.Ledger.EntryCount != "0" {
		t.Fatal("monthly snapshot time basis", r.Snapshot)
	}
}
func TestAutomaticArchiveExistingManualSkippedAndPolicyPausePreservesQueue(t *testing.T) {
	t.Parallel()
	s, a := archiveFixture(t)
	ctx := context.Background()
	manual := archiveCreate(t, s, a, pastInput())
	key := manual.Window.PeriodKey
	p := automaticPolicy(t, s, a, AutomaticPolicyInput{Version: 1, DailyEnabled: true, DailyStartPeriod: &key, Reason: "existing retained archive core proof"})
	if n, e := s.DiscoverAutomatic(ctx, 1); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	p = automaticPolicy(t, s, a, AutomaticPolicyInput{Version: p.Version, DailyStartPeriod: p.DailyStartPeriod, Reason: "pause automatic processing"})
	if n, e := s.ProcessAutomatic(ctx, 20); e != nil || n != 0 {
		t.Fatal("disabled policy processed", n, e)
	}
	p = automaticPolicy(t, s, a, AutomaticPolicyInput{Version: p.Version, DailyEnabled: true, DailyStartPeriod: p.DailyStartPeriod, Reason: "resume pending saved scopes"})
	if n, e := s.ProcessAutomatic(ctx, 20); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	rows := automaticTasks(t, s)
	if len(rows) != 1 || rows[0].State != "skipped" || rows[0].ArchiveID == nil || *rows[0].ArchiveID != manual.ID {
		t.Fatal("manual overwritten", rows)
	}
	var count int
	if e := s.DB.QueryRow(ctx, `SELECT count(*) FROM report_archives WHERE brand_id=$1 AND kind=$2 AND period_key=$3`, testBrand, manual.Window.Kind, key).Scan(&count); e != nil || count != 1 {
		t.Fatal("automatic duplicate archive", count, e)
	}
}

func TestAutomaticArchiveSkippedCivilDateKeepsPrecedingPeriod(t *testing.T) {
	t.Parallel()
	s, a := archiveFixture(t)
	ctx := context.Background()
	if _, err := s.DB.Exec(ctx, `UPDATE brands SET timezone='Pacific/Apia' WHERE id=$1`, testBrand); err != nil {
		t.Fatal(err)
	}
	key := "2011-12-29"
	automaticPolicy(t, s, a, AutomaticPolicyInput{Version: 1, DailyEnabled: true, DailyStartPeriod: &key, Reason: "synthetic skipped civil date proof"})
	if n, err := s.DiscoverAutomatic(ctx, 2); err != nil || n != 2 {
		t.Fatal("skipped-date bounded discovery", n, err)
	}
	if n, err := s.ProcessAutomatic(ctx, 2); err != nil || n != 2 {
		t.Fatal("skipped-date processing", n, err)
	}
	tasks := automaticTasks(t, s)
	keys := make(map[string]bool)
	for _, task := range tasks {
		keys[task.Window.PeriodKey] = true
		if task.State != "completed" || task.ArchiveID == nil {
			t.Fatal(task)
		}
	}
	if !keys["2011-12-29"] || !keys["2011-12-31"] || keys["2011-12-30"] {
		t.Fatal("valid date lost or nonexistent date manufactured", keys)
	}
	// The manually requested first version uses the same corrected boundaries.
	manual := archiveCreate(t, s, a, Input{Kind: Daily, PeriodKey: "2011-12-28", Reason: "manual boundary parity"})
	if manual.Window.To.UTC().Format(time.RFC3339) != "2011-12-29T10:00:00Z" {
		t.Fatal(manual.Window)
	}
	manual = archiveCreate(t, s, a, Input{Kind: Monthly, PeriodKey: "2011-12", Reason: "monthly boundary parity"})
	window, err := ResolveWindow(Monthly, "2011-12", "Pacific/Apia")
	if err != nil || !manual.Window.From.Equal(window.From) || !manual.Window.To.Equal(window.To) || manual.Window.Timezone != window.Timezone {
		t.Fatal(manual.Window, window, err)
	}
}
func TestAutomaticArchiveFailureRollbackNeedsExplicitRetryAndConcurrentWorkers(t *testing.T) {
	t.Parallel()
	s, a := archiveFixture(t)
	ctx := context.Background()
	automaticPast(t, s, a, 1)
	if _, e := s.DiscoverAutomatic(ctx, 20); e != nil {
		t.Fatal(e)
	}
	if _, e := s.DB.Exec(ctx, `CREATE FUNCTION fail_automatic_archive_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='report_archive.create.automatic' THEN RAISE EXCEPTION 'synthetic automatic capture failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_automatic_archive_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION fail_automatic_archive_audit()`); e != nil {
		t.Fatal(e)
	}
	if n, e := s.ProcessAutomatic(ctx, 20); e != nil || n != 0 {
		t.Fatal(n, e)
	}
	task := automaticTasks(t, s)[0]
	if task.State != "failed" || task.AttemptCount != 1 || task.LastErrorCode == nil || task.ArchiveID != nil {
		t.Fatal("failed attempt not atomic", task)
	}
	if _, e := s.DB.Exec(ctx, `DROP TRIGGER fail_automatic_archive_audit ON audit_logs`); e != nil {
		t.Fatal(e)
	}
	if n, e := s.ProcessAutomatic(ctx, 20); e != nil || n != 0 {
		t.Fatal("failure auto-retried", n, e)
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	meta := archiveMeta(a)
	meta.IP = "127.0.0.1"
	retry, e := s.RetryAutomaticTaskTx(ctx, tx, testBrand, task.ID, automaticActor(a), task.Version, "explicit technical retry", meta)
	if e != nil {
		t.Fatal(e)
	}
	if retry.State != "pending" || retry.AttemptCount != 1 {
		t.Fatal(retry)
	}
	var auditIP, auditRequest string
	if e = tx.QueryRow(ctx, `SELECT ip_address,request_id FROM audit_logs WHERE id=$1`, retry.LastAuditLogID).Scan(&auditIP, &auditRequest); e != nil || auditIP != meta.IP || auditRequest != meta.RequestID {
		t.Fatal("retry audit lost operator metadata", auditIP, auditRequest, e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.ProcessAutomatic(ctx, 20); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	task = automaticTasks(t, s)[0]
	if task.State != "completed" || task.AttemptCount != 2 || task.Version != 4 {
		t.Fatal("retry/concurrency", task)
	}
	var count int
	if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM report_archives`).Scan(&count); e != nil || count != 1 {
		t.Fatal("duplicate system archive", count, e)
	}
}
func TestAutomaticArchivePolicyAuditAndPermissionsFailClosed(t *testing.T) {
	t.Parallel()
	s, a := archiveFixture(t)
	ctx := context.Background()
	key := automaticBrandNow(t, s).Format("2006-01-02")
	in := AutomaticPolicyInput{Version: 1, DailyEnabled: true, DailyStartPeriod: &key, Reason: "current activation core proof"}
	platformViewOnly := automaticActor(a)
	platformViewOnly.Roles = []access.Role{{BrandID: testBrand, Permissions: []access.Permission{{Resource: "report_archive_policy", Action: "write", Scope: access.ScopeBrand}}}, {Permissions: []access.Permission{{Resource: "report_archive", Action: "view", Scope: access.ScopePlatform}}}}
	for _, actor := range []access.Account{a, platformViewOnly, func() access.Account { v := automaticActor(a); v.SuperAdmin = true; return v }()} {
		tx, e := s.DB.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		_, e = s.UpdateAutomaticPolicyTx(ctx, tx, testBrand, actor, in, archiveMeta(actor))
		tx.Rollback(ctx)
		if !errors.Is(e, ErrDenied) {
			t.Fatal("unauthorized policy", e)
		}
	}
	if _, e := s.DB.Exec(ctx, `CREATE FUNCTION fail_auto_policy_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='report_archive.policy.update' THEN RAISE EXCEPTION 'policy audit outage'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_auto_policy_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION fail_auto_policy_audit()`); e != nil {
		t.Fatal(e)
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.UpdateAutomaticPolicyTx(ctx, tx, testBrand, automaticActor(a), in, archiveMeta(a))
	tx.Rollback(ctx)
	if e == nil {
		t.Fatal("policy survived failed audit")
	}
	var version int
	var cursors int
	if e = s.DB.QueryRow(ctx, `SELECT version,(SELECT count(*) FROM report_archive_automatic_cursors) FROM brand_report_archive_policies WHERE brand_id=$1`, testBrand).Scan(&version, &cursors); e != nil || version != 1 || cursors != 0 {
		t.Fatal(version, cursors, e)
	}
}

func TestAutomaticArchiveDeferredLinkRejectsAnUnfinishedCapture(t *testing.T) {
	t.Parallel()
	s, a := archiveFixture(t)
	ctx := context.Background()
	archiveFund(t, s)
	before := archiveMoney(t, s)
	automaticPast(t, s, a, 1)
	if _, err := s.DiscoverAutomatic(ctx, 20); err != nil {
		t.Fatal(err)
	}
	// The earlier guard validates the attempted finish; this isolated fault
	// then drops it. Deferred evidence must still refuse the orphan archive.
	if _, err := s.DB.Exec(ctx, `CREATE FUNCTION zz_drop_archive_finish() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.state='completed' THEN RETURN OLD; END IF; RETURN NEW; END $$; CREATE TRIGGER zz_drop_archive_finish BEFORE UPDATE ON report_archive_automatic_tasks FOR EACH ROW EXECUTE FUNCTION zz_drop_archive_finish()`); err != nil {
		t.Fatal(err)
	}
	if n, err := s.ProcessAutomatic(ctx, 20); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	var archives, captureAudits int
	if err := s.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM report_archives),(SELECT count(*) FROM audit_logs WHERE action='report_archive.create.automatic')`).Scan(&archives, &captureAudits); err != nil || archives != 0 || captureAudits != 0 {
		t.Fatal("partial automatic capture committed", archives, captureAudits, err)
	}
	task := automaticTasks(t, s)[0]
	if task.State != "failed" || task.ArchiveID != nil || archiveMoney(t, s) != before {
		t.Fatal("orphan completion or money changed", task)
	}
}
