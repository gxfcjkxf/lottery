package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/tenant"
	"github.com/gxfcjkxf/lottery/backend/internal/withdrawal"
	"github.com/jackc/pgx/v5"
)

type withdrawalHTTPChecker struct{ delay time.Duration }

func (c withdrawalHTTPChecker) Check(ctx context.Context, _ pgx.Tx, _ withdrawal.EligibilityInput) (withdrawal.EligibilityDecision, error) {
	if c.delay > 0 {
		timer := time.NewTimer(c.delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return withdrawal.EligibilityDecision{}, ctx.Err()
		}
	}
	return withdrawal.EligibilityDecision{Allowed: true, Evidence: json.RawMessage(`{"test_adapter":true,"private_internal_note":"never expose qualification internals"}`)}, nil
}
func withdrawalHTTP(t *testing.T, f pointsHTTPFixture, checker withdrawal.EligibilityChecker) http.Handler {
	t.Helper()
	users, err := identity.New(f.pool)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := mutation.New(f.pool, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	return New(Dependencies{Brands: tenant.Store{DB: f.pool}, Ready: f.pool.Ping, Identity: users, Mutations: engine, Admins: adminsys.Store{DB: f.pool}, WithdrawalEligibility: checker, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
}
func configureWithdrawalHTTP(t *testing.T, f pointsHTTPFixture) {
	t.Helper()
	for _, key := range []string{"withdrawal_policy.write.brand", "withdrawal.view.brand", "withdrawal.approve.brand", "withdrawal.reject.brand", "withdrawal.cancel.brand", "withdrawal.fail.brand", "withdrawal.mark_paid.brand"} {
		grantReportPermission(t, f.managementHTTP, key)
	}
	r := f.call("PUT", "/api/v1/admin/withdrawal-policy", "withdraw-http-policy", f.token, managedBrand, map[string]any{"version": 1, "config": map[string]any{"enabled": true, "min_points": "1", "max_points": nil, "allowed_sources": []string{"recharge", "winning", "gift"}, "review_mode": "manual", "turnover_multiple": "1"}, "reason": "isolated HTTP fixture"})
	mustStatus(t, r, 200)
	order := pointRecharge(t, f, "150", "HTTP withdrawal fixture funding", "withdraw-http-recharge")
	mustStatus(t, f.call("POST", "/api/v1/admin/recharges/"+order.ID+"/confirm", "withdraw-http-fund-confirm", f.token, managedBrand, map[string]any{"version": order.Version, "reason": "isolated synthetic deposit"}), 200)
}
func withdrawalHTTPAvailability(t *testing.T, f pointsHTTPFixture) withdrawal.AvailabilityView {
	t.Helper()
	r := f.call("GET", "/api/v1/withdrawal-availability", "", f.userToken, managedBrand, nil)
	mustStatus(t, r, 200)
	var out withdrawal.AvailabilityView
	managedData(t, r, &out)
	return out
}
func withdrawalHTTPPost(f pointsHTTPFixture, key, actorContext, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "http://localhost/api/v1/withdrawals", bytes.NewBufferString(body))
	r.RemoteAddr = "192.0.2.54:12345"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+f.userToken)
	r.Header.Set("Idempotency-Key", key)
	r.Header.Set("X-Withdrawal-Actor-Context", actorContext)
	w := httptest.NewRecorder()
	f.http.ServeHTTP(w, r)
	return w
}

const withdrawalHTTPBody = `{"points":"50","source_allocation":[{"source":"recharge","state":"available","points":"50"}]}`

func withdrawalHTTPCreate(t *testing.T, f pointsHTTPFixture, key string) withdrawal.OrderView {
	t.Helper()
	available := withdrawalHTTPAvailability(t, f)
	r := withdrawalHTTPPost(f, key, available.ActorContext, withdrawalHTTPBody)
	mustStatus(t, r, 201)
	var o withdrawal.OrderView
	managedData(t, r, &o)
	return o
}

func TestWithdrawalHTTPDefaultQualificationIsClosedAndClientCannotOverrideIt(t *testing.T) {
	f := pointsFixture(t)
	configureWithdrawalHTTP(t, f)
	available := withdrawalHTTPAvailability(t, f)
	if available.CanApply || available.EligibilityConfigured || available.ReasonCode != "WITHDRAWAL_ELIGIBILITY_NOT_CONFIGURED" || len(available.ActorContext) != 64 {
		t.Fatalf("default availability wrong: %+v", available)
	}
	rejected := withdrawalHTTPPost(f, "default-qualification", ""+available.ActorContext, withdrawalHTTPBody)
	mustStatus(t, rejected, 409)
	if !strings.Contains(rejected.Body.String(), "WITHDRAWAL_ELIGIBILITY_NOT_CONFIGURED") {
		t.Fatal(rejected.Body.String())
	}
	mustStatus(t, withdrawalHTTPPost(f, "missing-actor-proof", "", withdrawalHTTPBody), 403)
	mustStatus(t, withdrawalHTTPPost(f, "wrong-actor-proof", strings.Repeat("0", 64), withdrawalHTTPBody), 403)
	for _, raw := range []string{
		strings.Replace(withdrawalHTTPBody, `"points":"50"`, `"points":"50","points":"50"`, 1),
		strings.Replace(withdrawalHTTPBody, `"source":"recharge"`, `"source":"recharge","source":"winning"`, 1),
		strings.Replace(withdrawalHTTPBody, `"state":"available"`, `"state":"available","qualified":true`, 1),
		strings.Replace(withdrawalHTTPBody, `"points":"50"`, `"points":50`, 1),
		strings.Replace(withdrawalHTTPBody, `"points":"50"`, `"points":"50","member_id":"`+f.memberID+`"`, 1),
		strings.Replace(withdrawalHTTPBody, `"points":"50"`, `"points":"50","eligibility_evidence":{}`, 1),
	} {
		mustStatus(t, withdrawalHTTPPost(f, ids.New(), available.ActorContext, raw), 400)
	}
	wallet := pointWallet(t, f)
	if wallet.AvailablePoints != 150 || wallet.WithdrawalPoints != 0 {
		t.Fatal("closed qualification changed wallet", wallet)
	}
	var n int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM withdrawal_orders`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("unexpected orders=%d %v", n, err)
	}
}

func TestWithdrawalHTTPApplicationApprovalPaidAndOriginalReceipts(t *testing.T) {
	f := pointsFixture(t)
	configureWithdrawalHTTP(t, f)
	f.http = withdrawalHTTP(t, f, withdrawalHTTPChecker{})
	available := withdrawalHTTPAvailability(t, f)
	created := withdrawalHTTPPost(f, "http-application-original", available.ActorContext, withdrawalHTTPBody)
	mustStatus(t, created, 201)
	var order withdrawal.OrderView
	managedData(t, created, &order)
	wallet := pointWallet(t, f)
	if wallet.AvailablePoints != 100 || wallet.WithdrawalPoints != 50 {
		t.Fatal(wallet)
	}
	if order.Points != 50 || order.State != "reviewing" || order.ReleaseEntryID != nil || order.ReserveVersion == "" {
		t.Fatal(order)
	}
	approveBody := map[string]any{"version": 1, "reason": "internal confidential review note"}
	approve := f.call("POST", "/api/v1/admin/withdrawals/"+order.ID+"/approve", "http-approve-original", f.token, managedBrand, approveBody)
	mustStatus(t, approve, 200)
	var approved withdrawal.OrderView
	managedData(t, approve, &approved)
	if approved.State != "processing" || approved.Version != 2 {
		t.Fatal(approved)
	}
	mustStatus(t, f.call("POST", "/api/v1/admin/withdrawals/"+order.ID+"/mark-paid", "http-paid-original", f.token, managedBrand, map[string]any{"version": 2, "reason": "internal-only simulated payout"}), 200)
	wallet = pointWallet(t, f)
	if wallet.AvailablePoints != 100 || wallet.WithdrawalPoints != 0 {
		t.Fatal(wallet)
	}
	replay := withdrawalHTTPPost(f, "http-application-original", available.ActorContext, withdrawalHTTPBody)
	mustStatus(t, replay, 201)
	var original withdrawal.OrderView
	managedData(t, replay, &original)
	if original.State != "reviewing" || original.Version != 1 || original.ID != order.ID {
		t.Fatal("cached creation returned live state", original)
	}
	approveReplay := f.call("POST", "/api/v1/admin/withdrawals/"+order.ID+"/approve", "http-approve-original", f.token, managedBrand, approveBody)
	mustStatus(t, approveReplay, 200)
	var originalApproval withdrawal.OrderView
	managedData(t, approveReplay, &originalApproval)
	if originalApproval.State != "processing" || originalApproval.Version != 2 {
		t.Fatal(originalApproval)
	}
	live := f.call("GET", "/api/v1/withdrawals/"+order.ID, "", f.userToken, managedBrand, nil)
	mustStatus(t, live, 200)
	var current withdrawal.OrderView
	managedData(t, live, &current)
	if current.State != "paid" || current.DecisionReason != "" {
		t.Fatal(current)
	}
	history := f.call("GET", "/api/v1/withdrawals/"+order.ID+"/history", "", f.userToken, managedBrand, nil)
	mustStatus(t, history, 200)
	for _, response := range []*httptest.ResponseRecorder{created, live, history, f.call("GET", "/api/v1/withdrawals", "", f.userToken, managedBrand, nil)} {
		for _, secret := range []string{"private_internal_note", "eligibility_evidence", "policy_snapshot", "actor_id", "client_key", "internal confidential", "internal-only"} {
			if strings.Contains(response.Body.String(), secret) {
				t.Fatalf("user response leaked %q: %s", secret, response.Body.String())
			}
		}
	}
	mustStatus(t, withdrawalHTTPPost(f, "http-application-original", available.ActorContext, strings.ReplaceAll(withdrawalHTTPBody, `"50"`, `"49"`)), 409)
	var n int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM point_ledger_entries WHERE reference_type='withdrawal'`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("duplicate financial writes %d %v", n, err)
	}
}

func TestWithdrawalHTTPOwnershipBrandQueriesAndFreshAdminPermissions(t *testing.T) {
	f := pointsFixture(t)
	configureWithdrawalHTTP(t, f)
	f.http = withdrawalHTTP(t, f, withdrawalHTTPChecker{})
	order := withdrawalHTTPCreate(t, f, "scope-original-application")
	for _, path := range []string{"/api/v1/withdrawals?member_id=" + f.memberID, "/api/v1/withdrawals?limit=1&limit=2", "/api/v1/withdrawals?state=unknown", "/api/v1/withdrawals?", "/api/v1/withdrawals/" + order.ID + "?x=1"} {
		mustStatus(t, f.call("GET", path, "", f.userToken, managedBrand, nil), 400)
	}
	mustStatus(t, f.call("GET", "/api/v1/b/harbor/withdrawals/"+order.ID, "", f.userToken, managedBrand, nil), 401)
	mustStatus(t, f.call("GET", "/api/v1/withdrawals/"+ids.New(), "", f.userToken, managedBrand, nil), 404)
	mustStatus(t, f.call("GET", "/api/v1/admin/withdrawals", "", f.token, pointsBrandB, nil), 403)
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE permission_key='withdrawal.approve.brand' AND role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1)`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("POST", "/api/v1/admin/withdrawals/"+order.ID+"/approve", "revoked-approval-operation", f.token, managedBrand, map[string]any{"version": 1, "reason": "should be denied"}), 403)
	mustStatus(t, f.call("GET", "/api/v1/admin/withdrawals/"+order.ID, "", f.token, managedBrand, nil), 200)
	mustStatus(t, f.call("GET", "/api/v1/admin/withdrawals?member_id="+ids.New(), "", f.token, managedBrand, nil), 404)
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	grantReportPermission(t, f.managementHTTP, "withdrawal.approve.brand")
	mustStatus(t, f.call("POST", "/api/v1/admin/withdrawals/"+order.ID+"/approve", "super-denied-approval", f.token, managedBrand, map[string]any{"version": 1, "reason": "super write denied"}), 403)
}

func TestWithdrawalHTTPMandatoryReadAuditFailureWithholdsData(t *testing.T) {
	f := pointsFixture(t)
	configureWithdrawalHTTP(t, f)
	f.http = withdrawalHTTP(t, f, withdrawalHTTPChecker{})
	order := withdrawalHTTPCreate(t, f, "read-audit-application")
	if _, err := f.pool.Exec(context.Background(), `CREATE FUNCTION reject_withdrawal_read_audit() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN IF NEW.action='withdrawal.read' THEN RAISE EXCEPTION 'injected audit failure'; END IF; RETURN NEW; END$$; CREATE TRIGGER reject_withdrawal_read_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_withdrawal_read_audit()`); err != nil {
		t.Fatal(err)
	}
	r := f.call("GET", "/api/v1/admin/withdrawals/"+order.ID, "", f.token, managedBrand, nil)
	mustStatus(t, r, 503)
	if strings.Contains(r.Body.String(), `"data"`) {
		t.Fatal("audit failure leaked data", r.Body.String())
	}
}

func TestWithdrawalHTTPChangedUserContextCannotReplayOrReadAnotherMember(t *testing.T) {
	f := pointsFixture(t)
	configureWithdrawalHTTP(t, f)
	f.http = withdrawalHTTP(t, f, withdrawalHTTPChecker{})
	oldContext := withdrawalHTTPAvailability(t, f).ActorContext
	order := withdrawalHTTPCreate(t, f, "first-member-intent")
	created := f.call("POST", "/api/v1/admin/users", "second-withdrawal-member", f.token, managedBrand, map[string]string{"username": "withdrawal_other", "password": "withdrawal-other-test-password-2026", "reason": "separate synthetic account"})
	mustStatus(t, created, 201)
	var member struct {
		MemberID string `json:"member_id"`
	}
	managedData(t, created, &member)
	login := f.call("POST", "/api/v1/auth/login", "second-withdrawal-login", "", managedBrand, map[string]string{"identifier": "withdrawal_other", "password": "withdrawal-other-test-password-2026", "privacy_policy_version": "dev-1", "service_terms_version": "dev-1"})
	mustStatus(t, login, 200)
	var auth identity.Authentication
	managedData(t, login, &auth)
	f.userToken = auth.AccessToken
	f.memberID = member.MemberID
	if withdrawalHTTPAvailability(t, f).ActorContext == oldContext {
		t.Fatal("actor context did not bind distinct member")
	}
	r := withdrawalHTTPPost(f, "first-member-intent", oldContext, withdrawalHTTPBody)
	mustStatus(t, r, 403)
	if !strings.Contains(r.Body.String(), "WITHDRAWAL_CONFIRMATION_ACCOUNT_CHANGED") {
		t.Fatal(r.Body.String())
	}
	mustStatus(t, f.call("GET", "/api/v1/withdrawals/"+order.ID, "", f.userToken, managedBrand, nil), 404)
	mustStatus(t, f.call("GET", "/api/v1/withdrawals/"+order.ID+"/history", "", f.userToken, managedBrand, nil), 404)
	list := f.call("GET", "/api/v1/withdrawals", "", f.userToken, managedBrand, nil)
	mustStatus(t, list, 200)
	var page withdrawal.OrderPage
	managedData(t, list, &page)
	if len(page.Items) != 0 {
		t.Fatal("foreign member records leaked", page)
	}
}

func TestWithdrawalHTTPForeignOriginClosedActionsAndNoImplicitPlatformRead(t *testing.T) {
	f := pointsFixture(t)
	configureWithdrawalHTTP(t, f)
	f.http = withdrawalHTTP(t, f, withdrawalHTTPChecker{})
	order := withdrawalHTTPCreate(t, f, "origin-application")
	path := "/api/v1/admin/withdrawals/" + order.ID + "/approve"
	request := httptest.NewRequest("POST", "http://localhost"+path, strings.NewReader(`{"version":1,"reason":"review"}`))
	request.Header.Set("Authorization", "Bearer "+f.token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Brand-ID", managedBrand)
	request.Header.Set("Idempotency-Key", "foreign-origin-approve")
	request.Header.Set("Origin", "http://attacker.example")
	response := httptest.NewRecorder()
	f.http.ServeHTTP(response, request)
	mustStatus(t, response, 403)
	for _, raw := range []string{`{"version":1,"version":1,"reason":"review"}`, `{"version":1,"reason":"review","points":"99"}`, `{"version":1,"reason":"review","client_key":"invented-key"}`} {
		r := httptest.NewRequest("POST", "http://localhost"+path, strings.NewReader(raw))
		r.Header.Set("Authorization", "Bearer "+f.token)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Brand-ID", managedBrand)
		r.Header.Set("Idempotency-Key", ids.New())
		w := httptest.NewRecorder()
		f.http.ServeHTTP(w, r)
		mustStatus(t, w, 400)
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("GET", "/api/v1/admin/withdrawals", "", f.token, pointsBrandB, nil), 403)
	grantReportPermission(t, f.managementHTTP, "withdrawal.view.platform")
	mustStatus(t, f.call("GET", "/api/v1/admin/withdrawals", "", f.token, pointsBrandB, nil), 200)
}

func TestWithdrawalHTTPExpiryDuringQualificationRollsBackReservation(t *testing.T) {
	f := pointsFixture(t)
	configureWithdrawalHTTP(t, f)
	f.http = withdrawalHTTP(t, f, withdrawalHTTPChecker{delay: 180 * time.Millisecond})
	available := withdrawalHTTPAvailability(t, f)
	if _, err := f.pool.Exec(context.Background(), `UPDATE sessions SET expires_at=clock_timestamp()+interval '100 milliseconds' WHERE member_id=$1 AND revoked_at IS NULL`, f.memberID); err != nil {
		t.Fatal(err)
	}
	r := withdrawalHTTPPost(f, "expired-qualification-intent", available.ActorContext, withdrawalHTTPBody)
	mustStatus(t, r, 401)
	w, err := (points.Store{DB: f.pool}).Read(context.Background(), managedBrand, f.memberID)
	if err != nil || w.AvailablePoints != 150 || w.WithdrawalPoints != 0 {
		t.Fatal(w, err)
	}
	var n int
	if err = f.pool.QueryRow(context.Background(), `SELECT count(*) FROM withdrawal_orders`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("expired application left order %d %v", n, err)
	}
}
