package httpapi

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRewardReportAuditFailureWithholdsJSONAndCSV(t *testing.T) {
	f := newRewardReportFixture(t)
	grantRewardReportPermission(t, f.managementHTTP, "report_reward.view.brand")
	grantRewardReportPermission(t, f.managementHTTP, "report_reward.export.brand")
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `CREATE FUNCTION reject_reward_report_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action IN ('report.rewards.view','report.rewards.export','report.reward_orders.view','report.reward_orders.export') THEN RAISE EXCEPTION 'injected reward report audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_reward_report_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_reward_report_audit()`); err != nil {
		t.Fatal(err)
	}
	from, to := reportWindow()
	for _, path := range []string{
		rewardReportQueryURL(rewardReportJSONPath, from, to, "day"),
		rewardReportQueryURL(rewardReportCSVPath, from, to, "day"),
		rewardReportQueryURL(rewardOrdersReportPath, from, to, "state"),
		rewardReportQueryURL(rewardOrdersReportCSVPath, from, to, "state"),
	} {
		out := rewardReportCall(f.managementHTTP, "GET", path, f.token, managedBrand, "")
		mustStatus(t, out, 503)
		if strings.Contains(out.Body.String(), "grant_points") || strings.Contains(out.Body.String(), "original_points") || strings.Contains(out.Body.String(), f.memberID) || strings.Contains(out.Header().Get("Content-Type"), "text/csv") || out.Header().Get("X-Report-Audit-ID") != "" || out.Header().Get("Content-Disposition") != "" {
			t.Fatalf("unaudited reward report leaked data: %s %s", out.Header(), out.Body.String())
		}
	}
	var audits int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE actor_id=$1 AND action LIKE 'report.reward%'`, f.root).Scan(&audits); err != nil || audits != 0 {
		t.Fatalf("failed audit transaction persisted %d report audit rows, err=%v", audits, err)
	}
}

func TestRewardReportSessionExpiryWhileAuditWaitsRollsBackAndWithholdsData(t *testing.T) {
	for _, tc := range []struct {
		name, path string
	}{
		{"json", rewardReportJSONPath},
		{"csv", rewardReportCSVPath},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := rewardFixture(t)
			grantRewardReportPermission(t, f.managementHTTP, "report_reward.view.brand")
			grantRewardReportPermission(t, f.managementHTTP, "report_reward.export.brand")
			const lockKey int64 = 77118001
			ctx := context.Background()
			unlock, err := f.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer unlock.Rollback(ctx)
			if _, err := unlock.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, lockKey); err != nil {
				t.Fatal(err)
			}
			if _, err := f.pool.Exec(ctx, `CREATE FUNCTION wait_reward_report_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action IN ('report.rewards.view','report.rewards.export') THEN PERFORM pg_advisory_xact_lock(77118001); END IF; RETURN NEW; END $$; CREATE TRIGGER wait_reward_report_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION wait_reward_report_audit()`); err != nil {
				t.Fatal(err)
			}
			from, to := reportWindow()
			path := rewardReportQueryURL(tc.path, from, to, "day")
			result := make(chan *httptest.ResponseRecorder, 1)
			go func() { result <- rewardReportCall(f.managementHTTP, "GET", path, f.token, managedBrand, "") }()
			waited := false
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				if err := f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks waiting JOIN pg_locks held USING(locktype,classid,objid,objsubid) WHERE waiting.locktype='advisory' AND NOT waiting.granted AND held.granted AND held.pid=$1 AND waiting.pid<>pg_backend_pid())`, unlock.Conn().PgConn().PID()).Scan(&waited); err != nil {
					t.Fatal(err)
				}
				if waited {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			if !waited {
				t.Fatal("reward report did not wait inside its audit insert")
			}
			if _, err := f.pool.Exec(ctx, `UPDATE sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE admin_id=$1 AND revoked_at IS NULL`, f.root); err != nil {
				t.Fatal(err)
			}
			if err := unlock.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			var out *httptest.ResponseRecorder
			select {
			case out = <-result:
			case <-time.After(3 * time.Second):
				t.Fatal("reward report remained blocked after audit lock released")
			}
			mustStatus(t, out, 401)
			if strings.Contains(out.Body.String(), "grant_points") || strings.Contains(out.Body.String(), "order_count") || strings.Contains(out.Header().Get("Content-Type"), "text/csv") || out.Header().Get("X-Report-Audit-ID") != "" || out.Header().Get("Content-Disposition") != "" {
				t.Fatalf("expired session received report/audit response: %s %s", out.Header(), out.Body.String())
			}
			var audits int
			if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE actor_id=$1 AND action IN('report.rewards.view','report.rewards.export')`, f.root).Scan(&audits); err != nil || audits != 0 {
				t.Fatalf("session-expired audit transaction was not rolled back: count=%d err=%v", audits, err)
			}
		})
	}
}

func TestRewardReportForeignScopeAndSuperAdminRequireExplicitReportRights(t *testing.T) {
	f := rewardFixture(t)
	from, to := reportWindow()
	path := rewardReportQueryURL(rewardReportJSONPath, from, to, "day")
	grantRewardReportPermission(t, f.managementHTTP, "report_reward.view.brand")
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, rewardReportCall(f.managementHTTP, "GET", path, f.token, pointsBrandB, ""), 403)
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=false WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='report_reward.view.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	grantRewardReportPermission(t, f.managementHTTP, "report_reward.view.platform")
	mustStatus(t, rewardReportCall(f.managementHTTP, "GET", path, f.token, pointsBrandB, ""), 200)
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='report_reward.view.platform'`, f.root); err != nil {
		t.Fatal(err)
	}
	grantRewardReportPermission(t, f.managementHTTP, "report_reward.view.brand")
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, rewardReportCall(f.managementHTTP, "GET", path, f.token, managedBrand, ""), 200)
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=false WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
}
