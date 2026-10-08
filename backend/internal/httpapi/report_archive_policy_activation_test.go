package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/reportarchive"
)

const archiveActivationPath = "/api/v1/admin/report-archive-policy"

func activationRequest(f managementHTTP, key, token, brand, actor, body string) *httptest.ResponseRecorder {
	return archiveTaskRequest(f, "PUT", archiveActivationPath, key, token, brand, actor, body)
}

func activationBody(version int64, daily, monthly bool, reason string) string {
	return fmt.Sprintf(`{"version":%d,"daily_enabled":%t,"monthly_enabled":%t,"reason":%q}`, version, daily, monthly, reason)
}

func TestReportArchivePolicyActivationFirstStartsRetentionReplayAndNoBackfill(t *testing.T) {
	t.Parallel()
	pf := pointsFixture(t)
	f := pf.managementHTTP
	ctx := context.Background()
	grantArchiveTaskPermissions(t, f, "report_archive.view.brand", "report_archive_policy.write.brand")
	initial := archiveTaskRequest(f, "GET", archivePolicyPath, "", f.token, managedBrand, "", "")
	mustStatus(t, initial, 200)
	var defaultPolicy reportarchive.AutomaticPolicy
	managedData(t, initial, &defaultPolicy)
	if defaultPolicy.Version != 1 || defaultPolicy.DailyEnabled || defaultPolicy.MonthlyEnabled || defaultPolicy.DailyStartPeriod != nil || defaultPolicy.MonthlyStartPeriod != nil {
		t.Fatalf("automatic policy did not begin disabled with unset starts: %+v", defaultPolicy)
	}
	var walletBefore string
	if err := f.pool.QueryRow(ctx, `SELECT md5(jsonb_build_object('accounts',(SELECT jsonb_agg(to_jsonb(p) ORDER BY id) FROM point_accounts p),'buckets',(SELECT jsonb_agg(to_jsonb(p) ORDER BY account_id,source,state) FROM point_buckets p),'ledger',(SELECT jsonb_agg(to_jsonb(p) ORDER BY id) FROM point_ledger_entries p),'recharges',(SELECT jsonb_agg(to_jsonb(p) ORDER BY id) FROM recharge_orders p),'bets',(SELECT jsonb_agg(to_jsonb(p) ORDER BY id) FROM bet_orders p),'outbox',(SELECT jsonb_agg(to_jsonb(p) ORDER BY id) FROM outbox_events p))::text)`).Scan(&walletBefore); err != nil {
		t.Fatal(err)
	}
	var zone string
	var dbNow time.Time
	if err := f.pool.QueryRow(ctx, `SELECT timezone,statement_timestamp() FROM brands WHERE id=$1`, managedBrand).Scan(&zone, &dbNow); err != nil {
		t.Fatal(err)
	}
	wantDaily, wantMonthly, err := reportarchive.ActivationPeriods(dbNow, zone)
	if err != nil {
		t.Fatal(err)
	}
	activateBody := activationBody(1, true, true, "begin automatic report archives")
	first := activationRequest(f, "archive-policy-activate-first", f.token, managedBrand, f.root, activateBody)
	mustStatus(t, first, 200)
	var v2 reportarchive.AutomaticPolicy
	managedData(t, first, &v2)
	if v2.Version != 2 || !v2.DailyEnabled || !v2.MonthlyEnabled || v2.DailyStartPeriod == nil || *v2.DailyStartPeriod != wantDaily || v2.MonthlyStartPeriod == nil || *v2.MonthlyStartPeriod != wantMonthly || v2.Timezone != zone || v2.AuditLogID == nil {
		t.Fatalf("first activation policy = %+v, want DB-time local period starts", v2)
	}
	if n, err := (reportarchive.Service{DB: f.pool}).DiscoverAutomatic(ctx, 20); err != nil || n != 0 {
		t.Fatalf("activation must not backfill incomplete current periods: tasks=%d err=%v", n, err)
	}
	var taskCount int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM report_archive_automatic_tasks WHERE brand_id=$1`, managedBrand).Scan(&taskCount); err != nil || taskCount != 0 {
		t.Fatalf("activation created immediate tasks: count=%d err=%v", taskCount, err)
	}
	offBody := activationBody(2, false, false, "pause automatic report archives")
	off := activationRequest(f, "archive-policy-activate-off", f.token, managedBrand, f.root, offBody)
	mustStatus(t, off, 200)
	var v3 reportarchive.AutomaticPolicy
	managedData(t, off, &v3)
	if v3.Version != 3 || v3.DailyEnabled || v3.MonthlyEnabled || v3.DailyStartPeriod == nil || *v3.DailyStartPeriod != wantDaily || v3.MonthlyStartPeriod == nil || *v3.MonthlyStartPeriod != wantMonthly {
		t.Fatalf("disable lost first activation starts: %+v", v3)
	}
	reenableBody := activationBody(3, true, true, "resume automatic report archives")
	reenabled := activationRequest(f, "archive-policy-activate-on", f.token, managedBrand, f.root, reenableBody)
	mustStatus(t, reenabled, 200)
	var v4 reportarchive.AutomaticPolicy
	managedData(t, reenabled, &v4)
	if v4.Version != 4 || v4.DailyStartPeriod == nil || *v4.DailyStartPeriod != wantDaily || v4.MonthlyStartPeriod == nil || *v4.MonthlyStartPeriod != wantMonthly {
		t.Fatalf("reenable changed first activation starts: %+v", v4)
	}
	// The original idempotency receipt remains bound to the accepted result even
	// after later policy versions have been written.
	replay := activationRequest(f, "archive-policy-activate-first", f.token, managedBrand, f.root, activateBody)
	mustStatus(t, replay, 200)
	var replayed reportarchive.AutomaticPolicy
	managedData(t, replay, &replayed)
	if replayed.Version != 2 || replayed.AuditLogID == nil || *replayed.AuditLogID != *v2.AuditLogID {
		t.Fatalf("original activation replay returned current policy instead of original acknowledgment: %+v", replayed)
	}
	grantArchiveTaskPermissions(t, f, "report_archive.view.brand")
	var role string
	if err := f.pool.QueryRow(ctx, `SELECT role_id::text FROM admin_account_roles WHERE account_id=$1 LIMIT 1`, f.root).Scan(&role); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='report_archive_policy.write.brand'`, role); err != nil {
		t.Fatal(err)
	}
	deniedReplay := activationRequest(f, "archive-policy-activate-first", f.token, managedBrand, f.root, activateBody)
	mustStatus(t, deniedReplay, 403)
	var afterVersion int64
	if err := f.pool.QueryRow(ctx, `SELECT version FROM brand_report_archive_policies WHERE brand_id=$1`, managedBrand).Scan(&afterVersion); err != nil || afterVersion != 4 {
		t.Fatalf("revoked replay changed current policy: version=%d err=%v", afterVersion, err)
	}
	var walletAfter string
	if err := f.pool.QueryRow(ctx, `SELECT md5(jsonb_build_object('accounts',(SELECT jsonb_agg(to_jsonb(p) ORDER BY id) FROM point_accounts p),'buckets',(SELECT jsonb_agg(to_jsonb(p) ORDER BY account_id,source,state) FROM point_buckets p),'ledger',(SELECT jsonb_agg(to_jsonb(p) ORDER BY id) FROM point_ledger_entries p),'recharges',(SELECT jsonb_agg(to_jsonb(p) ORDER BY id) FROM recharge_orders p),'bets',(SELECT jsonb_agg(to_jsonb(p) ORDER BY id) FROM bet_orders p),'outbox',(SELECT jsonb_agg(to_jsonb(p) ORDER BY id) FROM outbox_events p))::text)`).Scan(&walletAfter); err != nil || walletBefore != walletAfter {
		t.Fatalf("policy activation changed wallet or financial records: before=%s after=%s err=%v", walletBefore, walletAfter, err)
	}
}

func TestReportArchivePolicyActivationStrictInputAndAuthorizationGates(t *testing.T) {
	t.Parallel()
	f := managedFixture(t)
	grantArchiveTaskPermissions(t, f, "report_archive.view.brand", "report_archive_policy.write.brand")
	invalid := []string{
		`{}`, `null`, `{"version":null,"daily_enabled":true,"monthly_enabled":false,"reason":"valid reason"}`,
		`{"version":1,"daily_enabled":true,"monthly_enabled":false,"reason":"valid reason","extra":true}`,
		`{"version":1,"version":1,"daily_enabled":true,"monthly_enabled":false,"reason":"valid reason"}`,
		`{"version":1,"daily_enabled":true,"monthly_enabled":false}`,
		`{"version":1,"daily_enabled":true,"monthly_enabled":false,"reason":" leading"}`,
		`{"version":1,"daily_enabled":true,"monthly_enabled":false,"reason":"line\nbreak"}`,
		`{"version":0,"daily_enabled":true,"monthly_enabled":false,"reason":"valid reason"}`,
		`{"version":1,"daily_enabled":1,"monthly_enabled":false,"reason":"valid reason"}`,
		`{"version":1,"daily_enabled":true,"monthly_enabled":null,"reason":"valid reason"}`,
		`{"version":1,"daily_enabled":true,"monthly_enabled":false,"reason":"valid reason"} {}`,
		`{"version":1,"daily_enabled":true,"monthly_enabled":false,"reason":"valid reason","daily_start_period":"2020-01-01"}`,
		`{"version":1,"daily_enabled":true,"monthly_enabled":false,"reason":"valid reason","timezone":"UTC"}`,
	}
	for i, body := range invalid {
		r := activationRequest(f, fmt.Sprintf("archive-policy-activation-invalid-%02d", i), f.token, managedBrand, f.root, body)
		mustStatus(t, r, 400)
		if code := archiveTaskCode(t, r); code != "REQUEST_INVALID" {
			t.Errorf("invalid activation body #%d returned %q: %s", i, code, r.Body.String())
		}
	}
	for _, tc := range []struct {
		name, token, brand, actor string
		want                      int
	}{
		{name: "missing session", brand: managedBrand, actor: f.root, want: 401},
		{name: "cross brand", token: f.token, brand: pointsBrandB, actor: f.root, want: 403},
		{name: "actor mismatch", token: f.token, brand: managedBrand, actor: ids.New(), want: 401},
		{name: "missing actor", token: f.token, brand: managedBrand, want: 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := activationRequest(f, "archive-policy-activation-auth-"+strings.ReplaceAll(tc.name, " ", "-"), tc.token, tc.brand, tc.actor,
				activationBody(1, true, false, "activation authorization check"))
			mustStatus(t, r, tc.want)
		})
	}
	var role string
	if err := f.pool.QueryRow(context.Background(), `SELECT role_id::text FROM admin_account_roles WHERE account_id=$1 LIMIT 1`, f.root).Scan(&role); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='report_archive_policy.write.brand'`, role); err != nil {
		t.Fatal(err)
	}
	missingGrant := activationRequest(f, "archive-policy-activation-no-write-grant", f.token, managedBrand, f.root, activationBody(1, true, false, "write permission required"))
	mustStatus(t, missingGrant, 403)
	grantArchiveTaskPermissions(t, f, "report_archive_policy.write.brand")
	stale := activationRequest(f, "archive-policy-activation-stale-version", f.token, managedBrand, f.root, activationBody(2, true, false, "stale policy version"))
	mustStatus(t, stale, 409)
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	super := activationRequest(f, "archive-policy-activation-super", f.token, managedBrand, f.root, activationBody(1, true, false, "super-admin cannot activate"))
	mustStatus(t, super, 403)
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=false WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE brands SET status='paused' WHERE id=$1`, managedBrand); err != nil {
		t.Fatal(err)
	}
	paused := activationRequest(f, "archive-policy-activation-paused", f.token, managedBrand, f.root, activationBody(1, false, false, "paused brand policy remains manageable"))
	mustStatus(t, paused, 200)
	var pausedPolicy reportarchive.AutomaticPolicy
	managedData(t, paused, &pausedPolicy)
	if pausedPolicy.Version != 2 || pausedPolicy.DailyEnabled || pausedPolicy.MonthlyEnabled || pausedPolicy.DailyStartPeriod != nil || pausedPolicy.MonthlyStartPeriod != nil {
		t.Fatalf("paused brand policy update changed activation state unexpectedly: %+v", pausedPolicy)
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE brands SET status='disabled' WHERE id=$1`, managedBrand); err != nil {
		t.Fatal(err)
	}
	disabled := activationRequest(f, "archive-policy-activation-disabled", f.token, managedBrand, f.root, activationBody(2, true, false, "disabled brand cannot activate"))
	mustStatus(t, disabled, 409)
	var version int64
	var dailyStart, monthlyStart *string
	if err := f.pool.QueryRow(context.Background(), `SELECT version,daily_start_period,monthly_start_period FROM brand_report_archive_policies WHERE brand_id=$1`, managedBrand).Scan(&version, &dailyStart, &monthlyStart); err != nil {
		t.Fatal(err)
	}
	if version != 2 || dailyStart != nil || monthlyStart != nil {
		t.Fatalf("rejected state/authorization mutated default policy: version=%d starts=(%v,%v)", version, dailyStart, monthlyStart)
	}
}

func TestReportArchivePolicyActivationFailureIsAtomic(t *testing.T) {
	t.Parallel()
	f := managedFixture(t)
	ctx := context.Background()
	grantArchiveTaskPermissions(t, f, "report_archive.view.brand", "report_archive_policy.write.brand")
	if _, err := f.pool.Exec(ctx, `CREATE FUNCTION fail_report_archive_activation_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='report_archive.policy.update' THEN RAISE EXCEPTION 'isolated activation audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_report_archive_activation_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION fail_report_archive_activation_audit()`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = f.pool.Exec(ctx, `DROP TRIGGER IF EXISTS fail_report_archive_activation_audit ON audit_logs; DROP FUNCTION IF EXISTS fail_report_archive_activation_audit()`)
	}()
	failed := activationRequest(f, "archive-policy-activation-audit-fail", f.token, managedBrand, f.root, activationBody(1, true, true, "activation audit rollback proof"))
	mustStatus(t, failed, 503)
	var version, cursors, audits, receipts int
	if err := f.pool.QueryRow(ctx, `SELECT version FROM brand_report_archive_policies WHERE brand_id=$1`, managedBrand).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM report_archive_automatic_cursors WHERE brand_id=$1`, managedBrand).Scan(&cursors); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='report_archive.policy.update' AND brand_id=$1`, managedBrand).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM idempotency_requests WHERE key='archive-policy-activation-audit-fail'`).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if version != 1 || cursors != 0 || audits != 0 || receipts != 0 {
		t.Fatalf("failed activation left partial state: version=%d cursors=%d audits=%d receipts=%d", version, cursors, audits, receipts)
	}
}

func TestReportArchivePolicyActivationUnseenKindsRemainUnset(t *testing.T) {
	t.Parallel()
	f := managedFixture(t)
	grantArchiveTaskPermissions(t, f, "report_archive.view.brand", "report_archive_policy.write.brand")
	first := activationRequest(f, "archive-policy-activation-daily-only", f.token, managedBrand, f.root, activationBody(1, true, false, "activate daily archives only"))
	mustStatus(t, first, 200)
	var policy reportarchive.AutomaticPolicy
	managedData(t, first, &policy)
	if policy.DailyStartPeriod == nil || policy.MonthlyStartPeriod != nil {
		t.Fatalf("disabled never-enabled monthly kind acquired a start: %+v", policy)
	}
	off := activationRequest(f, "archive-policy-activation-daily-off", f.token, managedBrand, f.root, activationBody(2, false, false, "turn daily archives off"))
	mustStatus(t, off, 200)
	var disabled reportarchive.AutomaticPolicy
	managedData(t, off, &disabled)
	if disabled.DailyStartPeriod == nil || *disabled.DailyStartPeriod != *policy.DailyStartPeriod || disabled.MonthlyStartPeriod != nil {
		t.Fatalf("off state did not retain only previously activated start: %+v", disabled)
	}
}

func TestReportArchivePolicyActivationResponseIsOriginalAndCompleteJSON(t *testing.T) {
	f := managedFixture(t)
	grantArchiveTaskPermissions(t, f, "report_archive.view.brand", "report_archive_policy.write.brand")
	r := activationRequest(f, "archive-policy-activation-json-shape", f.token, managedBrand, f.root, activationBody(1, false, false, "keep automation disabled"))
	mustStatus(t, r, 200)
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(envelope.Data, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"brand_id", "version", "daily_enabled", "monthly_enabled", "daily_start_period", "monthly_start_period", "timezone", "audit_log_id", "updated_at"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("activation acknowledgment missing original policy field %q: %s", key, envelope.Data)
		}
	}
}

func TestReportArchivePolicyActivationBusyPolicyRowDoesNotCacheAndCanRetry(t *testing.T) {
	t.Parallel()
	f := managedFixture(t)
	ctx := context.Background()
	grantArchiveTaskPermissions(t, f, "report_archive.view.brand", "report_archive_policy.write.brand")
	key := "archive-policy-activation-busy-row"
	body := activationBody(1, true, true, "retry after policy lock is released")
	lock, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(ctx)
	if _, err = lock.Exec(ctx, `SELECT brand_id FROM brand_report_archive_policies WHERE brand_id=$1 FOR UPDATE`, managedBrand); err != nil {
		t.Fatal(err)
	}
	busy := activationRequest(f, key, f.token, managedBrand, f.root, body)
	mustStatus(t, busy, 503)
	var receipts int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM idempotency_requests WHERE key=$1`, key).Scan(&receipts); err != nil || receipts != 0 {
		t.Fatalf("lock-busy activation cached an acknowledgment: count=%d err=%v", receipts, err)
	}
	if err = lock.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	retried := activationRequest(f, key, f.token, managedBrand, f.root, body)
	mustStatus(t, retried, 200)
	var policy reportarchive.AutomaticPolicy
	managedData(t, retried, &policy)
	if policy.Version != 2 || !policy.DailyEnabled || !policy.MonthlyEnabled || policy.DailyStartPeriod == nil || policy.MonthlyStartPeriod == nil {
		t.Fatalf("same-key retry after lock release did not activate policy: %+v", policy)
	}
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM idempotency_requests WHERE key=$1`, key).Scan(&receipts); err != nil || receipts != 1 {
		t.Fatalf("successful retry acknowledgment count=%d err=%v, want 1", receipts, err)
	}
}

func TestReportArchivePolicyActivationAuditWaitRechecksExpiredSessionAndRollsBack(t *testing.T) {
	t.Parallel()
	f := managedFixture(t)
	ctx := context.Background()
	grantArchiveTaskPermissions(t, f, "report_archive.view.brand", "report_archive_policy.write.brand")
	const advisoryKey int64 = 77165010
	lock, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(ctx)
	if _, err = lock.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, advisoryKey); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `CREATE FUNCTION wait_report_archive_policy_activation_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='report_archive.policy.update' THEN PERFORM pg_advisory_xact_lock(77165010); END IF; RETURN NEW; END $$; CREATE TRIGGER wait_report_archive_policy_activation_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION wait_report_archive_policy_activation_audit()`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = f.pool.Exec(ctx, `DROP TRIGGER IF EXISTS wait_report_archive_policy_activation_audit ON audit_logs; DROP FUNCTION IF EXISTS wait_report_archive_policy_activation_audit()`)
	}()
	key := "archive-policy-activation-expired-session"
	result := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		result <- activationRequest(f, key, f.token, managedBrand, f.root, activationBody(1, true, true, "session expires during audit wait"))
	}()
	waited := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err = f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks waiting JOIN pg_locks held USING(locktype,classid,objid,objsubid) WHERE waiting.locktype='advisory' AND NOT waiting.granted AND held.granted AND held.pid=$1 AND waiting.pid<>pg_backend_pid())`, lock.Conn().PgConn().PID()).Scan(&waited); err != nil {
			t.Fatal(err)
		}
		if waited {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !waited {
		t.Fatal("activation request did not reach its policy audit wait")
	}
	expireSingleAdminSession(t, f)
	if err = lock.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case response := <-result:
		mustStatus(t, response, 401)
		if archiveTaskCode(t, response) != "AUTH_SESSION_REVOKED" {
			t.Fatalf("expired activation session returned wrong error: %s", response.Body.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("activation request remained blocked after audit wait was released")
	}
	var version, cursors, audits, receipts int
	if err = f.pool.QueryRow(ctx, `SELECT version FROM brand_report_archive_policies WHERE brand_id=$1`, managedBrand).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM report_archive_automatic_cursors WHERE brand_id=$1`, managedBrand).Scan(&cursors); err != nil {
		t.Fatal(err)
	}
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='report_archive.policy.update' AND brand_id=$1`, managedBrand).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM idempotency_requests WHERE key=$1`, key).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if version != 1 || cursors != 0 || audits != 0 || receipts != 0 {
		t.Fatalf("expired-session activation committed partial state: version=%d cursors=%d audits=%d receipts=%d", version, cursors, audits, receipts)
	}
}

func expireSingleAdminSession(t *testing.T, f managementHTTP) {
	t.Helper()
	result, err := f.pool.Exec(context.Background(), `UPDATE sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE admin_id=$1 AND revoked_at IS NULL`, f.root)
	if err != nil {
		t.Fatal(err)
	}
	if result.RowsAffected() != 1 {
		t.Fatalf("expired %d active admin sessions, want exactly one", result.RowsAffected())
	}
}
