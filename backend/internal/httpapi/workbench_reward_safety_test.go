package httpapi

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/workbench"
)

func TestWorkbenchRewardSectionUsesIndependentRewardViewPermissionAndRealOrders(t *testing.T) {
	f := newRewardReportFixture(t)
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='reward.view.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	response := f.call("GET", workbenchPath, "", f.token, managedBrand, nil)
	mustStatus(t, response, 200)
	var snapshot workbench.Snapshot
	managedData(t, response, &snapshot)
	if snapshot.Rewards.Status != "forbidden" || snapshot.Rewards.Data != nil {
		t.Fatalf("reward grant/revoke rights implied workbench read: %+v", snapshot.Rewards)
	}
	grantRewardPermission(t, f.managementHTTP, "reward.view.brand")
	response = f.call("GET", workbenchPath, "", f.token, managedBrand, nil)
	mustStatus(t, response, 200)
	managedData(t, response, &snapshot)
	rewards := snapshot.Rewards.Data
	if snapshot.Rewards.Status != "ready" || rewards == nil || rewards.GrantedCount != "1" || rewards.PendingCount != "1" || rewards.RevokedCount != "1" {
		t.Fatalf("independent reward.view did not expose real granted/pending/revoked counts: %+v", snapshot.Rewards)
	}
	if strings.Contains(response.Body.String(), f.memberID) || strings.Contains(response.Body.String(), f.grantedID) || strings.Contains(response.Body.String(), f.revokedID) || strings.Contains(response.Body.String(), f.pendingID) {
		t.Fatal("workbench reward summary leaked member or order identifiers")
	}
}

func TestWorkbenchSessionExpiryWhileAuditWaitsRollsBackAndWithholdsSnapshot(t *testing.T) {
	f := managedFixture(t)
	grantRewardPermission(t, f, "reward.view.brand")
	const lockKey int64 = 77118002
	ctx := context.Background()
	unlock, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock.Rollback(ctx)
	if _, err := unlock.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, lockKey); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `CREATE FUNCTION wait_workbench_reward_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='workbench.view' THEN PERFORM pg_advisory_xact_lock(77118002); END IF; RETURN NEW; END $$; CREATE TRIGGER wait_workbench_reward_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION wait_workbench_reward_audit()`); err != nil {
		t.Fatal(err)
	}
	result := make(chan *httptest.ResponseRecorder, 1)
	go func() { result <- f.call("GET", workbenchPath, "", f.token, managedBrand, nil) }()
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
		t.Fatal("workbench did not wait inside its audit insert")
	}
	if _, err := f.pool.Exec(ctx, `UPDATE sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE admin_id=$1 AND revoked_at IS NULL`, f.root); err != nil {
		t.Fatal(err)
	}
	if err := unlock.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var response *httptest.ResponseRecorder
	select {
	case response = <-result:
	case <-time.After(3 * time.Second):
		t.Fatal("workbench remained blocked after audit lock release")
	}
	mustStatus(t, response, 401)
	if strings.Contains(response.Body.String(), `"snapshot_at"`) || strings.Contains(response.Body.String(), `"rewards"`) || strings.Contains(response.Body.String(), managedBrand) {
		t.Fatalf("expired session received workbench DTO: %s", response.Body.String())
	}
	var audits int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE actor_id=$1 AND action='workbench.view'`, f.root).Scan(&audits); err != nil || audits != 0 {
		t.Fatalf("session-expired workbench audit transaction was not rolled back: count=%d err=%v", audits, err)
	}
}
