package httpapi

import (
	"context"
	"fmt"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/reportarchive"
	"net/http/httptest"
	"testing"
)

const archiveActivationPath = "/api/v1/platform/report-archive-policy"

func activationRequest(f managementHTTP, key, token, brand, actor, body string) *httptest.ResponseRecorder {
	return archiveTaskRequest(f, "PUT", archiveActivationPath, key, token, brand, actor, body)
}
func activationBody(version int64, daily, monthly bool, reason string) string {
	return fmt.Sprintf(`{"version":%d,"daily_enabled":%t,"monthly_enabled":%t,"daily_execution_time":"07:30","daily_period_offset":2,"monthly_execution_day":5,"monthly_execution_time":"06:00","monthly_period_offset":2,"reason":%q}`, version, daily, monthly, reason)
}
func TestPlatformArchivePolicyInheritancePermissionsAndOriginalReceipt(t *testing.T) {
	f := managedFixture(t)
	op := securityOperator(t, f)
	ctx := context.Background()
	mustStatus(t, f.call("PUT", "/api/v1/admin/report-archive-policy", "brand-archive-settings-denied", f.token, managedBrand, map[string]any{}), 404)
	mustStatus(t, op.call("GET", archiveActivationPath, "", op.token, "", nil), 403)
	grantPlatformPermission(t, op, "platform_settings.view.platform", "platform_settings.write.platform")
	mustStatus(t, f.call("GET", archiveActivationPath, "", f.token, "", nil), 403)
	first := activationRequest(op, "global-archive-enable-first", op.token, "", op.root, activationBody(1, true, true, ""))
	mustStatus(t, first, 200)
	var saved reportarchive.PlatformPolicy
	managedData(t, first, &saved)
	if saved.Version != 2 || saved.MonthlyPeriodOffset != 2 || saved.MonthlyExecutionDay != 5 || saved.NextRuns.Monthly == nil {
		t.Fatal(saved)
	}
	var mismatches int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM brand_report_archive_policies b CROSS JOIN platform_report_archive_policy g WHERE b.daily_enabled<>g.daily_enabled OR b.monthly_enabled<>g.monthly_enabled OR b.daily_execution_time<>g.daily_execution_time OR b.monthly_execution_time<>g.monthly_execution_time OR b.daily_period_offset<>g.daily_period_offset OR b.monthly_period_offset<>g.monthly_period_offset OR b.monthly_execution_day<>g.monthly_execution_day`).Scan(&mismatches); err != nil || mismatches != 0 {
		t.Fatal(mismatches, err)
	}
	id := ids.New()
	if _, err := f.pool.Exec(ctx, `INSERT INTO brands(id,code,name,status) VALUES($1,$2,'Archive inheritance test','paused')`, id, "archive_"+id); err != nil {
		t.Fatal(err)
	}
	var enabled bool
	if err := f.pool.QueryRow(ctx, `SELECT daily_enabled AND monthly_enabled AND monthly_period_offset=2 AND monthly_execution_day=5 FROM brand_report_archive_policies WHERE brand_id=$1`, id).Scan(&enabled); err != nil || !enabled {
		t.Fatal(enabled, err)
	}
	mustStatus(t, activationRequest(op, "global-archive-disable-later", op.token, "", op.root, activationBody(2, false, false, "")), 200)
	replay := activationRequest(op, "global-archive-enable-first", op.token, "", op.root, activationBody(1, true, true, ""))
	mustStatus(t, replay, 200)
	var old reportarchive.PlatformPolicy
	managedData(t, replay, &old)
	if old.Version != 2 || !old.MonthlyEnabled || *old.NextRuns.Monthly != *saved.NextRuns.Monthly {
		t.Fatal("original receipt changed", old)
	}
	mustStatus(t, activationRequest(op, "global-archive-brand-input", op.token, managedBrand, op.root, activationBody(3, true, true, "")), 400)
	mustStatus(t, activationRequest(op, "global-archive-wrong-actor", op.token, "", f.root, activationBody(3, true, true, "")), 401)
}
func TestPlatformArchivePolicyPropagationFailureRollsBackAndCanRetry(t *testing.T) {
	f := managedFixture(t)
	op := securityOperator(t, f)
	ctx := context.Background()
	grantPlatformPermission(t, op, "platform_settings.view.platform", "platform_settings.write.platform")
	if _, err := f.pool.Exec(ctx, `CREATE FUNCTION reject_archive_global_propagation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='report_archive.policy.platform_sync' THEN RAISE EXCEPTION 'isolated propagation failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_archive_global_propagation BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_archive_global_propagation()`); err != nil {
		t.Fatal(err)
	}
	body := activationBody(1, true, true, "")
	mustStatus(t, activationRequest(op, "global-archive-atomic-retry", op.token, "", op.root, body), 503)
	var version int64
	if err := f.pool.QueryRow(ctx, `SELECT version FROM platform_report_archive_policy WHERE id=true`).Scan(&version); err != nil || version != 1 {
		t.Fatal(version, err)
	}
	var enabled int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM brand_report_archive_policies WHERE daily_enabled OR monthly_enabled`).Scan(&enabled); err != nil || enabled != 0 {
		t.Fatal(enabled, err)
	}
	if _, err := f.pool.Exec(ctx, `DROP TRIGGER reject_archive_global_propagation ON audit_logs;DROP FUNCTION reject_archive_global_propagation()`); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, activationRequest(op, "global-archive-atomic-retry", op.token, "", op.root, body), 200)
}
