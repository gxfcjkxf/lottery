package httpapi

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/attribution"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
)

const commissionAdjustmentHTTPPath = "/api/v1/admin/commission-payment-targets/0199a000-0000-7000-8000-000000000099/adjustments"

func grantCommissionAdjustment(t *testing.T, f managementHTTP, permissions ...string) {
	t.Helper()
	ctx := context.Background()
	for _, key := range append([]string{"commission.view.brand", "commission_adjustment.write.brand"}, permissions...) {
		if _, err := f.pool.Exec(ctx, `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, key); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,$2 FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root, key); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCommissionAdjustmentHTTPRequiresDedicatedPermissionAndAuditsDenial(t *testing.T) {
	f := managedFixture(t)
	write := paymentRawCall(f, "POST", commissionAdjustmentHTTPPath, "adjust-denied-permission-01", f.token, managedBrand,
		`{"version":1,"points":"1","reason":"permission rejection test"}`)
	if write.Code != 403 {
		t.Fatalf("adjustment without dedicated write permission status=%d body=%s", write.Code, write.Body.String())
	}
	var denials int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE action='access.denied' AND after_json->>'attempted_brand'=$1 AND after_json->>'permission'='commission_adjustment.write'`, managedBrand).Scan(&denials); err != nil || denials != 1 {
		t.Fatalf("adjustment denial audit count=%d err=%v", denials, err)
	}
	var receipts int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM idempotency_requests WHERE operation='admin.commission_adjustment.create'`).Scan(&receipts); err != nil || receipts != 0 {
		t.Fatalf("denied adjustment stored receipt count=%d err=%v", receipts, err)
	}
}

func TestCommissionAdjustmentHTTPActorBindingPrecedesBusinessStateAndReceipt(t *testing.T) {
	f := managedFixture(t)
	grantCommissionAdjustment(t, f)
	body := `{"version":1,"points":"1","reason":"bind correction to reviewed administrator"}`
	key := "adjustment-actor-binding-01"
	for _, actor := range []string{"", ids.New()} {
		out := paymentRawCall(f, "POST", commissionAdjustmentHTTPPath, key, f.token, managedBrand, body, actor)
		if out.Code != 401 {
			t.Fatalf("wrong/missing actor reached target lookup: status=%d body=%s", out.Code, out.Body.String())
		}
	}
	var receipts int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM idempotency_requests WHERE operation='admin.commission_adjustment.create' AND key=$1`, key).Scan(&receipts); err != nil || receipts != 0 {
		t.Fatalf("wrong actor cached correction intent count=%d err=%v", receipts, err)
	}

	// The authenticated actor gets past the security check; the disabled gate
	// proves only the correctly bound request reaches business processing.
	valid := paymentRawCall(f, "POST", commissionAdjustmentHTTPPath, key, f.token, managedBrand, body)
	if valid.Code != 409 {
		t.Fatalf("correct actor did not reach the closed-gate business check: status=%d body=%s", valid.Code, valid.Body.String())
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM idempotency_requests WHERE operation='admin.commission_adjustment.create' AND key=$1`, key).Scan(&receipts); err != nil || receipts != 1 {
		t.Fatalf("valid actor result receipt count=%d err=%v", receipts, err)
	}
}

func TestCommissionAdjustmentHTTPRejectsClosedBodyAndMalformedPagination(t *testing.T) {
	f := managedFixture(t)
	for _, body := range []string{
		`{}`, `null`,
		`{"version":1,"points":"1"}`,
		`{"version":1,"points":"1","reason":"manual","ignored":true}`,
		`{"version":1,"version":1,"points":"1","reason":"manual"}`,
	} {
		out := paymentRawCall(f, "POST", commissionAdjustmentHTTPPath, "adjustment-closed-body-01", f.token, managedBrand, body)
		if out.Code != 400 {
			t.Errorf("invalid closed adjustment body accepted: status=%d body=%s raw=%s", out.Code, out.Body.String(), body)
		}
	}
	for _, query := range []string{"?limit=0", "?limit=20&limit=30", "?cursor=opaque"} {
		out := f.call("GET", "/api/v1/admin/commission-payments/"+ids.New()+"/targets"+query, "", f.token, managedBrand, nil)
		if out.Code != 400 {
			t.Errorf("invalid target page query accepted: status=%d body=%s query=%s", out.Code, out.Body.String(), query)
		}
	}
}

func TestCommissionAdjustmentHTTPCommitReplayAfterHeadAdvanceAndHistoricalReadOnDisabledBrand(t *testing.T) {
	var boundary time.Time
	f, order := settlementHTTPFixtureBeforeBetMutating(t, func(f *pointsHTTPFixture) {
		grantCommissionCycle(t, f.managementHTTP, "view", "run")
		grantCommissionPolicy(t, f.managementHTTP)
		grantCommissionPaymentPolicyWriter(t, f.managementHTTP)
		grantCommissionAdjustment(t, f.managementHTTP)
		agentGrantRoot(t, f.managementHTTP)
		mustStatus(t, f.call("PUT", "/api/v1/admin/agent-policy", "adjustment-http-agent-policy-01", f.token, managedBrand,
			map[string]any{"version": 1, "config": map[string]any{"enabled": true, "max_depth": 3, "ratio_cap": "1", "mode": "turnover", "cycle": "weekly"}, "reason": "enable actual adjustment beneficiary"}), 200)
		agentResponse := f.call("POST", "/api/v1/admin/agents", "adjustment-http-agent-create-01", f.token, managedBrand,
			agentNodeBody(2, f.memberID, nil, nil, "1"))
		mustStatus(t, agentResponse, 201)
		var agent agentNode
		managedData(t, agentResponse, &agent)
		grantJoinCodeRoot(t, f.managementHTTP)
		codeResponse := f.call("POST", "/api/v1/admin/join-codes", "adjustment-http-agent-code-01", f.token, managedBrand,
			map[string]any{"kind": "agent", "owner_member_id": f.memberID, "agent_id": agent.ID, "starts_at": nil, "expires_at": nil, "reason": "attribute actual adjustment test bettor"})
		mustStatus(t, codeResponse, 201)
		var code attribution.Code
		managedData(t, codeResponse, &code)
		registered := f.call("POST", "/api/v1/auth/register", "adjustment-http-agent-member-01", "", managedBrand,
			map[string]string{"username": "adjustment_http_" + strings.ReplaceAll(ids.New(), "-", "")[:12], "password": "adjustment-http-member-password-2026", "privacy_policy_version": "dev-1", "service_terms_version": "dev-1", "agent_code": code.Code})
		mustStatus(t, registered, 201)
		var user struct {
			AccessToken string `json:"access_token"`
			Member      struct {
				ID string `json:"id"`
			} `json:"member"`
		}
		managedData(t, registered, &user)
		f.memberID, f.userToken = user.Member.ID, user.AccessToken
		recharge := pointRecharge(t, *f, "100", "fund attributed commission bettor", "adjustment-http-member-fund")
		mustStatus(t, f.call("POST", "/api/v1/admin/recharges/"+recharge.ID+"/confirm", "adjustment-http-member-fund-confirm", f.token, managedBrand,
			map[string]any{"version": 1, "reason": "verify attributed commission test funding"}), 200)

		boundary = time.Now().UTC().Add(10 * time.Second).Truncate(time.Second)
		weekday := int(boundary.Weekday())
		config := enabledCommissionPolicyConfig()
		config.Calendar.Weekday = &weekday
		config.Calendar.BoundaryTime = boundary.Format("15:04:05")
		policyResponse := f.call("GET", commissionPolicyPath, "", f.token, managedBrand, nil)
		mustStatus(t, policyResponse, 200)
		var policy commission.Policy
		managedData(t, policyResponse, &policy)
		mustStatus(t, f.call("PUT", commissionPolicyPath, "adjustment-http-commission-policy-01", f.token, managedBrand,
			commission.PolicyInput{Version: policy.Version, Config: config, Reason: "enable actual paid commission target"}), 200)

		gateResponse := f.call("GET", commissionPaymentPolicyHTTPPath, "", f.token, managedBrand, nil)
		mustStatus(t, gateResponse, 200)
		var gate commission.PaymentPolicy
		managedData(t, gateResponse, &gate)
		mustStatus(t, paymentRawCall(f.managementHTTP, "PUT", commissionPaymentPolicyHTTPPath, "adjustment-http-payment-gate-01", f.token, managedBrand,
			`{"version":`+strconv.FormatInt(gate.Version, 10)+`,"enabled":true,"reason":"enable actual manual commission payout"}`), 200)
		if _, err := f.pool.Exec(context.Background(), `INSERT INTO permissions(key) VALUES('commission_payment.approve.brand') ON CONFLICT DO NOTHING`); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(context.Background(), `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,'commission_payment.approve.brand' FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root); err != nil {
			t.Fatal(err)
		}
	})
	if !order.PlacedAt.Before(boundary) {
		t.Fatalf("genuine adjustment anchor is outside the configured cycle: placed=%s boundary=%s", order.PlacedAt, boundary)
	}
	if wait := time.Until(boundary); wait > 0 {
		time.Sleep(wait)
	}
	for _, permission := range []string{"settlement.view.brand", "settlement.run.brand", "settlement_policy.view.brand", "settlement_policy.write.brand"} {
		if _, err := f.pool.Exec(context.Background(), `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, permission); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(context.Background(), `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,$2 FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root, permission); err != nil {
			t.Fatal(err)
		}
	}
	var settlementPolicy betting.SettlementPolicy
	settlementPolicyResponse := f.call("GET", "/api/v1/admin/settlement-policy", "", f.token, managedBrand, nil)
	mustStatus(t, settlementPolicyResponse, 200)
	managedData(t, settlementPolicyResponse, &settlementPolicy)
	mode := "automatic"
	mustStatus(t, f.call("PUT", "/api/v1/admin/settlement-policy", "adjustment-http-settlement-policy-01", f.token, managedBrand,
		betting.SettlementPolicyInput{Version: settlementPolicy.Version, Mode: &mode, Reason: "finalize actual commission anchor"}), 200)
	settlementContextResponse := f.call("GET", "/api/v1/admin/periods/"+order.PeriodID+"/settlement-context", "", f.token, managedBrand, nil)
	mustStatus(t, settlementContextResponse, 200)
	var settlementContext betting.PeriodSettlementContext
	managedData(t, settlementContextResponse, &settlementContext)
	settlementBody := betting.SettlementStartInput{Version: settlementContext.PeriodVersion, PolicyVersion: settlementPolicy.Version + 1, DrawResultID: *settlementContext.DrawResultID, Reason: "finalize actual commission anchor"}
	mustStatus(t, f.call("POST", "/api/v1/admin/periods/"+order.PeriodID+"/settle", "adjustment-http-settlement-create-01", f.token, managedBrand, settlementBody), 201)
	if _, err := (betting.Service{DB: f.pool}).ProcessSettlements(context.Background(), 100); err != nil {
		t.Fatal(err)
	}

	cycleResponse := f.call("POST", commissionCyclesPath, "adjustment-http-cycle-create-01", f.token, managedBrand,
		map[string]string{"anchor_order_id": order.ID, "reason": "create current closed cycle for adjustment HTTP test"})
	mustStatus(t, cycleResponse, 201)
	var cycle commission.Cycle
	managedData(t, cycleResponse, &cycle)
	service := commission.Service{DB: f.pool}
	for i := 0; i < 15 && cycle.State != "ready"; i++ {
		if _, err := service.ProcessCycles(context.Background(), 100); err != nil {
			t.Fatal(err)
		}
		read := f.call("GET", commissionCyclesPath+"/"+cycle.ID, "", f.token, managedBrand, nil)
		mustStatus(t, read, 200)
		managedData(t, read, &cycle)
		if cycle.State != "ready" {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if cycle.State != "ready" {
		t.Fatalf("cycle did not produce current earnings: %+v", cycle)
	}
	if _, err := service.ProcessPayments(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	paymentsResponse := f.call("GET", commissionPaymentsHTTPPath, "", f.token, managedBrand, nil)
	mustStatus(t, paymentsResponse, 200)
	var payments commission.PaymentPage
	managedData(t, paymentsResponse, &payments)
	if len(payments.Items) != 1 {
		t.Fatalf("expected one actual commission payment, got %+v", payments)
	}
	payment := payments.Items[0]
	if payment.State == "awaiting_approval" {
		approved := paymentRawCall(f.managementHTTP, "POST", commissionPaymentsHTTPPath+"/"+payment.ID+"/approve", "adjustment-http-payment-approve-01", f.token, managedBrand,
			`{"version":1,"reason":"approve actual payout before correction"}`)
		mustStatus(t, approved, 200)
	}
	for i := 0; i < 10; i++ {
		if _, err := service.ProcessPayments(context.Background(), 100); err != nil {
			t.Fatal(err)
		}
		read := f.call("GET", commissionPaymentsHTTPPath+"/"+payment.ID, "", f.token, managedBrand, nil)
		mustStatus(t, read, 200)
		managedData(t, read, &payment)
		if payment.State == "paid" {
			break
		}
	}
	if payment.State != "paid" {
		t.Fatalf("actual payment did not finish: %+v", payment)
	}
	targetsResponse := f.call("GET", commissionPaymentsHTTPPath+"/"+payment.ID+"/targets", "", f.token, managedBrand, nil)
	mustStatus(t, targetsResponse, 200)
	var targets commission.TargetPage
	managedData(t, targetsResponse, &targets)
	if len(targets.Items) != 1 || targets.Items[0].State != "paid" || targets.Items[0].AdjustmentVersion == nil || *targets.Items[0].AdjustmentVersion != 1 {
		t.Fatalf("expected one paid target with initialized head: %+v", targets)
	}
	target := targets.Items[0]
	path := "/api/v1/admin/commission-payment-targets/" + target.ID + "/adjustments"
	firstBody := `{"version":1,"points":"2","reason":"first actual HTTP commission correction"}`
	firstKey := "adjustment-http-actual-create-01"
	firstResponse := paymentRawCall(f.managementHTTP, "POST", path, firstKey, f.token, managedBrand, firstBody)
	mustStatus(t, firstResponse, 201)
	var first commission.Adjustment
	managedData(t, firstResponse, &first)
	if first.Version != 2 || first.PointsBefore != target.OriginalPoints || first.PointsAfter != 2 || first.LedgerEntryID == "" || first.CreatedBy != f.root {
		t.Fatalf("actual HTTP correction did not commit a full receipt: %+v", first)
	}

	secondBody := `{"version":2,"points":"3","reason":"advance actual commission adjustment head"}`
	second := paymentRawCall(f.managementHTTP, "POST", path, "adjustment-http-actual-create-02", f.token, managedBrand, secondBody)
	mustStatus(t, second, 201)
	var advanced commission.Adjustment
	managedData(t, second, &advanced)
	if advanced.Version != 3 || advanced.PointsBefore != 2 || advanced.PointsAfter != 3 {
		t.Fatalf("second actual correction did not advance head: %+v", advanced)
	}

	replay := paymentRawCall(f.managementHTTP, "POST", path, firstKey, f.token, managedBrand, firstBody)
	mustStatus(t, replay, 201)
	var replayed commission.Adjustment
	managedData(t, replay, &replayed)
	if replayed.ID != first.ID || replayed.Version != 2 || replayed.PointsAfter != 2 || replayed.LedgerEntryID != first.LedgerEntryID {
		t.Fatalf("replay returned current head instead of immutable original receipt: replay=%+v original=%+v", replayed, first)
	}
	if wrongActor := paymentRawCall(f.managementHTTP, "POST", path, firstKey, f.token, managedBrand, firstBody, ids.New()); wrongActor.Code != 401 {
		t.Fatalf("receipt replay accepted a different actor: status=%d body=%s", wrongActor.Code, wrongActor.Body.String())
	}

	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='commission_adjustment.write.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	if revoked := paymentRawCall(f.managementHTTP, "POST", path, firstKey, f.token, managedBrand, firstBody); revoked.Code != 403 {
		t.Fatalf("receipt replay ignored revoked adjustment role: status=%d body=%s", revoked.Code, revoked.Body.String())
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,'commission_adjustment.write.brand' FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE admin_accounts SET status='disabled' WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	if disabledActor := paymentRawCall(f.managementHTTP, "POST", path, firstKey, f.token, managedBrand, firstBody); disabledActor.Code != 401 {
		t.Fatalf("receipt replay ignored disabled administrator account: status=%d body=%s", disabledActor.Code, disabledActor.Body.String())
	}
	if _, err := f.pool.Exec(ctx, `UPDATE admin_accounts SET status='active' WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}

	if _, err := f.pool.Exec(ctx, `UPDATE brands SET status='disabled' WHERE id=$1`, managedBrand); err != nil {
		t.Fatal(err)
	}
	defer f.pool.Exec(ctx, `UPDATE brands SET status='active' WHERE id=$1`, managedBrand)
	for _, readPath := range []string{commissionPaymentsHTTPPath + "/" + payment.ID + "/targets", path} {
		read := f.call("GET", readPath, "", f.token, managedBrand, nil)
		mustStatus(t, read, 200)
	}
	disabledWrite := paymentRawCall(f.managementHTTP, "POST", path, "adjustment-disabled-brand-write-01", f.token, managedBrand,
		`{"version":3,"points":"4","reason":"must remain read-only while brand disabled"}`)
	if disabledWrite.Code != 409 {
		t.Fatalf("disabled brand allowed adjustment write: status=%d body=%s", disabledWrite.Code, disabledWrite.Body.String())
	}
}
