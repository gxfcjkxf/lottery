package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/withdrawal"
)

func qualificationGet(f pointsHTTPFixture, path, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", "http://localhost"+path, nil)
	r.RemoteAddr = "192.0.2.83:12345"
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.http.ServeHTTP(w, r)
	return w
}

func qualificationFingerprint(t *testing.T, f pointsHTTPFixture) string {
	t.Helper()
	var value string
	if err := f.pool.QueryRow(context.Background(), `SELECT md5(jsonb_build_object('buckets',(SELECT jsonb_agg(to_jsonb(b) ORDER BY account_id,source,state) FROM point_buckets b),'ledger',(SELECT jsonb_agg(to_jsonb(l) ORDER BY id) FROM point_ledger_entries l),'orders',(SELECT jsonb_agg(to_jsonb(o) ORDER BY id) FROM withdrawal_orders o),'cycles',(SELECT jsonb_agg(to_jsonb(c) ORDER BY member_id) FROM withdrawal_turnover_cycles c),'receipts',(SELECT count(*) FROM idempotency_requests))::text)`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestWithdrawalQualificationHTTPReadOnlyScopeAndStrictRequests(t *testing.T) {
	f := pointsFixture(t)
	configureWithdrawalHTTP(t, f)
	f.http = withdrawalHTTP(t, f, withdrawal.TurnoverChecker{})
	before := qualificationFingerprint(t, f)
	for _, path := range []string{"/api/v1/withdrawal-qualification", "/api/v1/b/aurora/withdrawal-qualification"} {
		r := qualificationGet(f, path, f.userToken)
		mustStatus(t, r, 200)
		var q withdrawal.QualificationView
		managedData(t, r, &q)
		if q.MemberID != f.memberID || q.BrandID != managedBrand || q.BasePoints != 150 || q.ValidPoints != "0" || q.ValidOrderCount != "0" || q.CreditNumerator != "0" || q.CreditDenominator != "1" || q.MeetsTurnover || q.CycleFromVersion != "0" || q.CycleFromAt != nil {
			t.Fatal(q)
		}
		var envelope struct {
			Data map[string]json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(r.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if len(envelope.Data) != 13 {
			t.Fatalf("qualification DTO leaks unknown fields: %v", envelope.Data)
		}
	}
	for _, query := range []string{"?member_id=" + f.memberID, "?qualified=true", "?", "?cutoff_version=999", "?n=1&n=2"} {
		mustStatus(t, qualificationGet(f, "/api/v1/withdrawal-qualification"+query, f.userToken), 400)
	}
	r := httptest.NewRequest("GET", "http://localhost/api/v1/withdrawal-qualification", bytes.NewBufferString(`{"qualified":true}`))
	r.Header.Set("Authorization", "Bearer "+f.userToken)
	w := httptest.NewRecorder()
	f.http.ServeHTTP(w, r)
	mustStatus(t, w, 400)
	mustStatus(t, qualificationGet(f, "/api/v1/withdrawal-qualification", ""), 401)
	mustStatus(t, qualificationGet(f, "/api/v1/withdrawal-qualification", f.token), 401)
	mustStatus(t, qualificationGet(f, "/api/v1/b/harbor/withdrawal-qualification", f.userToken), 401)
	if got := qualificationFingerprint(t, f); got != before {
		t.Fatal("qualification reads mutated wallet/ledger/orders/cycle/receipts")
	}
	available := withdrawalHTTPAvailability(t, f)
	if !available.CanApply || !available.EligibilityConfigured {
		t.Fatal("configured entry availability must not be confused with turnover qualification", available)
	}
	mustStatus(t, withdrawalHTTPPost(f, "real-insufficient-turnover", available.ActorContext, withdrawalHTTPBody), 409)
	if pointWallet(t, f).WithdrawalPoints != 0 {
		t.Fatal("unqualified POST reserved funds")
	}
}

func realQualificationFixture(t *testing.T) (pointsHTTPFixture, betting.Order) {
	t.Helper()
	f, bet := settlementHTTPFixtureBeforeBet(t, func(f pointsHTTPFixture) {
		for _, key := range []string{"withdrawal_policy.write.brand", "withdrawal.view.brand", "withdrawal.approve.brand", "withdrawal.cancel.brand", "withdrawal.mark_paid.brand", "settlement_policy.write.brand", "settlement.run.brand", "settlement.view.brand"} {
			grantReportPermission(t, f.managementHTTP, key)
		}
		c := withdrawal.DefaultBrandConfig()
		c.Enabled = true
		c.TurnoverMultiple = "0.000001"
		mustStatus(t, f.call("PUT", "/api/v1/admin/withdrawal-policy", "real-turnover-policy", f.token, managedBrand, withdrawal.BrandInput{Version: 1, Config: c, Reason: "explicit real qualification test"}), 200)
	})
	f.http = withdrawalHTTP(t, f, withdrawal.TurnoverChecker{})
	mode := "automatic"
	mustStatus(t, f.call("PUT", "/api/v1/admin/settlement-policy", "real-withdraw-settlement-policy", f.token, managedBrand, betting.SettlementPolicyInput{Version: 1, Mode: &mode, Reason: "explicit payout to qualify actual stake"}), 200)
	var phase betting.PeriodSettlementContext
	response := f.call("GET", "/api/v1/admin/periods/"+bet.PeriodID+"/settlement-context", "", f.token, managedBrand, nil)
	mustStatus(t, response, 200)
	managedData(t, response, &phase)
	mustStatus(t, f.call("POST", "/api/v1/admin/periods/"+bet.PeriodID+"/settle", "real-withdraw-settle", f.token, managedBrand, betting.SettlementStartInput{Version: phase.PeriodVersion, PolicyVersion: 2, DrawResultID: *phase.DrawResultID, Reason: "settle actual qualifying stake"}), 201)
	if _, err := (betting.Service{DB: f.pool}).ProcessSettlements(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	return f, bet
}

func TestWithdrawalQualificationHTTPFreshRecheckReceiptAndSuccessfulCycle(t *testing.T) {
	f, _ := realQualificationFixture(t)
	r := qualificationGet(f, "/api/v1/withdrawal-qualification", f.userToken)
	mustStatus(t, r, 200)
	var q withdrawal.QualificationView
	managedData(t, r, &q)
	if !q.MeetsTurnover || q.BasePoints != 99 || q.ValidPoints != "1" || q.CreditNumerator != "1000000" {
		t.Fatal(q)
	}
	available := withdrawalHTTPAvailability(t, f)
	adjustPath := "/api/v1/admin/wallets/" + f.memberID + "/adjust"
	mustStatus(t, f.call("POST", adjustPath, "invalidate-real-qualification-preview", f.token, managedBrand, map[string]string{"source": "recharge", "delta": "1000001", "reason": "preview is stale after a real balance increase"}), 200)
	denied := withdrawalHTTPPost(f, "stale-preview-cannot-authorize", available.ActorContext, withdrawalHTTPBody)
	mustStatus(t, denied, 409)
	if pointWallet(t, f).WithdrawalPoints != 0 {
		t.Fatal("old positive preview bypassed fresh current-balance qualification")
	}
	mustStatus(t, f.call("POST", adjustPath, "restore-real-qualification-preview", f.token, managedBrand, map[string]string{"source": "recharge", "delta": "-1000001", "reason": "restore original test source balance with a ledger record"}), 200)
	first := withdrawalHTTPPost(f, "real-qualified-once", available.ActorContext, withdrawalHTTPBody)
	mustStatus(t, first, 201)
	var o withdrawal.OrderView
	managedData(t, first, &o)
	replay := withdrawalHTTPPost(f, "real-qualified-once", available.ActorContext, withdrawalHTTPBody)
	mustStatus(t, replay, 201)
	if !bytes.Equal(first.Body.Bytes(), replay.Body.Bytes()) {
		var repeated withdrawal.OrderView
		managedData(t, replay, &repeated)
		if repeated.ID != o.ID || repeated.ReserveEntryID != o.ReserveEntryID || repeated.State != o.State {
			t.Fatal("replay changed original receipt", repeated)
		}
	}
	if w := pointWallet(t, f); w.RechargePoints != 49 || w.WinningPoints != 10 || w.WithdrawalPoints != 50 {
		t.Fatal(w)
	}
	mustStatus(t, f.call("POST", "/api/v1/admin/withdrawals/"+o.ID+"/approve", "real-qualified-approve", f.token, managedBrand, withdrawal.ActionRequest{Version: 1, Reason: "review actual source reservation"}), 200)
	mustStatus(t, f.call("POST", "/api/v1/admin/withdrawals/"+o.ID+"/mark-paid", "real-qualified-paid", f.token, managedBrand, withdrawal.ActionRequest{Version: 2, Reason: "internal points completion only"}), 200)
	r = qualificationGet(f, "/api/v1/withdrawal-qualification", f.userToken)
	mustStatus(t, r, 200)
	managedData(t, r, &q)
	if q.MeetsTurnover || q.CreditNumerator != "0" || q.ValidOrderCount != "0" || q.BasePoints != 49 || q.CycleFromVersion != o.ReserveVersion || q.CycleFromAt == nil || !q.CycleFromAt.Equal(o.CreatedAt) {
		t.Fatal(q)
	}
	mustStatus(t, withdrawalHTTPPost(f, "real-next-cycle-denied", available.ActorContext, withdrawalHTTPBody), 409)
	if pointWallet(t, f).WithdrawalPoints != 0 {
		t.Fatal("old stake qualified a repeated cycle")
	}
}

func TestWithdrawalQualificationHTTPPeriodBusyIsRetryableAndDoesNotReserve(t *testing.T) {
	f, bet := realQualificationFixture(t)
	ctx := context.Background()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT id FROM periods WHERE id=$1 FOR UPDATE`, bet.PeriodID); err != nil {
		t.Fatal(err)
	}
	available := withdrawalHTTPAvailability(t, f)
	before := qualificationFingerprint(t, f)
	r := withdrawalHTTPPost(f, "busy-key-is-not-cached", available.ActorContext, withdrawalHTTPBody)
	mustStatus(t, r, 503)
	if got := qualificationFingerprint(t, f); got != before {
		t.Fatal("busy response left cached receipt or funds")
	}
	mustStatus(t, qualificationGet(f, "/api/v1/withdrawal-qualification", f.userToken), 503)
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, withdrawalHTTPPost(f, "busy-key-is-not-cached", available.ActorContext, withdrawalHTTPBody), 201)
}
