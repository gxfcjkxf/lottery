package httpapi

import (
	"context"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/attribution"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
)

func TestCommissionCorrectionHTTPRealPlanRetryApprovalPauseContinueAndExecutionRetryReceipts(t *testing.T) {
	ctx := context.Background()
	var boundary time.Time
	var beneficiary string
	f, order := settlementHTTPFixtureBeforeBetMutating(t, func(f *pointsHTTPFixture) {
		beneficiary = f.memberID
		grantCommissionCycle(t, f.managementHTTP, "view", "run")
		grantCommissionPolicy(t, f.managementHTTP)
		grantCommissionPaymentPolicyWriter(t, f.managementHTTP)
		grantCorrectionHTTP(t, f.managementHTTP, "commission_correction_policy.write.brand", "commission_correction.retry.brand", "commission_correction.approve.brand", "commission_correction.continue.brand", "commission_correction.execute_retry.brand", "commission_payment.approve.brand")
		agentGrantRoot(t, f.managementHTTP)
		grantJoinCodeRoot(t, f.managementHTTP)
		mustStatus(t, f.call("PUT", "/api/v1/admin/agent-policy", "correction-http-agency-01", f.token, managedBrand, map[string]any{"version": 1, "config": map[string]any{"enabled": true, "max_depth": 3, "ratio_cap": "1", "mode": "loss", "cycle": "weekly"}, "reason": "real correction beneficiary policy"}), 200)
		r := f.call("POST", "/api/v1/admin/agents", "correction-http-agent-01", f.token, managedBrand, agentNodeBody(2, beneficiary, nil, nil, "1"))
		mustStatus(t, r, 201)
		var node agentNode
		managedData(t, r, &node)
		r = f.call("POST", "/api/v1/admin/join-codes", "correction-http-code-01", f.token, managedBrand, map[string]any{"kind": "agent", "owner_member_id": beneficiary, "agent_id": node.ID, "starts_at": nil, "expires_at": nil, "reason": "join a real commission beneficiary"})
		mustStatus(t, r, 201)
		var code attribution.Code
		managedData(t, r, &code)
		r = f.call("POST", "/api/v1/auth/register", "correction-http-player-01", "", managedBrand, map[string]string{"username": "correction_http_" + strings.ReplaceAll(ids.New(), "-", "")[:12], "password": "correction-http-member-password-2026", "privacy_policy_version": "dev-1", "service_terms_version": "dev-1", "agent_code": code.Code})
		mustStatus(t, r, 201)
		var user struct {
			AccessToken string `json:"access_token"`
			Member      struct {
				ID string `json:"id"`
			} `json:"member"`
		}
		managedData(t, r, &user)
		f.memberID, f.userToken = user.Member.ID, user.AccessToken
		recharge := pointRecharge(t, *f, "100", "fund a real attributed bettor", "correction-http-funding-01")
		mustStatus(t, f.call("POST", "/api/v1/admin/recharges/"+recharge.ID+"/confirm", "correction-http-confirm-01", f.token, managedBrand, map[string]any{"version": 1, "reason": "confirm actual stake funding"}), 200)
		boundary = time.Now().UTC().Add(10 * time.Second).Truncate(time.Second)
		weekday := int(boundary.Weekday())
		config := enabledCommissionPolicyConfig()
		config.Calendar.Weekday = &weekday
		config.Calendar.BoundaryTime = boundary.Format("15:04:05")
		r = f.call("GET", commissionPolicyPath, "", f.token, managedBrand, nil)
		mustStatus(t, r, 200)
		var p commission.Policy
		managedData(t, r, &p)
		mustStatus(t, f.call("PUT", commissionPolicyPath, "correction-http-economics-01", f.token, managedBrand, commission.PolicyInput{Version: p.Version, Config: config, Reason: "capture real manual commission economics"}), 200)
		mustStatus(t, paymentRawCall(f.managementHTTP, "PUT", commissionPaymentPolicyHTTPPath, "correction-http-payment-gate-01", f.token, managedBrand, `{"version":1,"enabled":true,"reason":"explicit original commission payout opt-in"}`), 200)
	}, rules.Draw{Regular: []int{}, Special: []int{}, Digits: []int{1, 2, 2}})
	if !order.PlacedAt.Before(boundary) {
		t.Fatal("real bet missed its immutable calendar boundary")
	}
	if delay := time.Until(boundary); delay > 0 {
		time.Sleep(delay)
	}
	grantCorrectionHTTP(t, f.managementHTTP, "settlement.view.brand", "settlement.run.brand", "settlement_policy.view.brand", "settlement_policy.write.brand", "draw.view.brand", "draw.correct.brand")
	mode := "automatic"
	mustStatus(t, f.call("PUT", "/api/v1/admin/settlement-policy", "correction-http-settlement-policy-01", f.token, managedBrand, betting.SettlementPolicyInput{Version: 1, Mode: &mode, Reason: "actual original loss settlement"}), 200)
	bs := betting.Service{DB: f.pool}
	c, err := bs.PeriodSettlementContext(ctx, managedBrand, order.PeriodID)
	if err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("POST", "/api/v1/admin/periods/"+order.PeriodID+"/settle", "correction-http-original-settle-01", f.token, managedBrand, betting.SettlementStartInput{Version: c.PeriodVersion, PolicyVersion: 2, DrawResultID: *c.DrawResultID, Reason: "settle actual original losing bet"}), 201)
	if _, err = bs.ProcessSettlements(ctx, 100); err != nil {
		t.Fatal(err)
	}
	r := f.call("POST", commissionCyclesPath, "correction-http-cycle-01", f.token, managedBrand, map[string]string{"anchor_order_id": order.ID, "reason": "actual closed original cycle"})
	mustStatus(t, r, 201)
	var cycle commission.Cycle
	managedData(t, r, &cycle)
	cs := commission.Service{DB: f.pool}
	if _, err = cs.ProcessCycles(ctx, 100); err != nil {
		t.Fatal(err)
	}
	if _, err = cs.ProcessPayments(ctx, 100); err != nil {
		t.Fatal(err)
	}
	r = f.call("GET", commissionPaymentsHTTPPath, "", f.token, managedBrand, nil)
	mustStatus(t, r, 200)
	var payments commission.PaymentPage
	managedData(t, r, &payments)
	if len(payments.Items) != 1 || payments.Items[0].State != "awaiting_approval" {
		t.Fatalf("actual manual payment not created: %+v", payments)
	}
	pay := payments.Items[0]
	mustStatus(t, paymentRawCall(f.managementHTTP, "POST", commissionPaymentsHTTPPath+"/"+pay.ID+"/approve", "correction-http-original-approve-01", f.token, managedBrand, `{"version":1,"reason":"approve actual original one-point commission"}`), 200)
	if _, err = cs.ProcessPayments(ctx, 100); err != nil {
		t.Fatal(err)
	}
	store := points.Store{DB: f.pool}
	wallet, err := store.Read(ctx, managedBrand, beneficiary)
	if err != nil || wallet.CommissionPoints != 1 {
		t.Fatalf("actual original commission wallet absent: %+v err=%v", wallet, err)
	}
	cc, err := bs.CorrectionContext(ctx, managedBrand, order.PeriodID)
	if err != nil {
		t.Fatal(err)
	}
	pv := int64(2)
	mustStatus(t, f.call("POST", "/api/v1/admin/draw-results/"+*cc.DrawResultID+"/correct", "correction-http-winning-redraw-01", f.token, managedBrand, betting.CorrectionInput{Version: cc.PeriodVersion, PolicyVersion: &pv, Result: rules.Draw{Regular: []int{}, Special: []int{}, Digits: []int{1, 2, 1}}, Reason: "verified winning replacement makes loss commission zero"}), 201)
	if _, err = bs.ProcessCorrections(ctx, 100); err != nil {
		t.Fatal(err)
	}
	if _, err = bs.ProcessSettlements(ctx, 100); err != nil {
		t.Fatal(err)
	}
	if _, err = cs.ProcessCycles(ctx, 100); err != nil {
		t.Fatal(err)
	}
	if _, err = cs.ProcessPayments(ctx, 100); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `CREATE FUNCTION fail_http_correction_page() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='commission.correction_plan.page' THEN RAISE EXCEPTION 'test plan page audit outage'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_http_correction_page BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION fail_http_correction_page()`); err != nil {
		t.Fatal(err)
	}
	if _, err = cs.ProcessCorrectionPlans(ctx, 100); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `DROP TRIGGER fail_http_correction_page ON audit_logs; DROP FUNCTION fail_http_correction_page()`); err != nil {
		t.Fatal(err)
	}
	plansPath := correctionHTTPPrefix + correctionPlansPath
	executionsPath := correctionHTTPPrefix + correctionExecutionsPath
	r = correctionCall(f.managementHTTP, "GET", plansPath, "", f.token, managedBrand, "")
	mustStatus(t, r, 200)
	var plans commission.CorrectionPlanPage
	managedData(t, r, &plans)
	if len(plans.Items) != 1 || plans.Items[0].State != "failed" {
		t.Fatalf("real page fault not retryable: %+v", plans)
	}
	plan := plans.Items[0]
	planBody := `{"version":` + strconv.FormatInt(plan.Version, 10) + `,"reason":"retry failed preparation without financial approval"}`
	planReceipt := correctionCall(f.managementHTTP, "POST", plansPath+"/"+plan.ID+"/retry", "correction-http-plan-retry-01", f.token, managedBrand, planBody)
	mustStatus(t, planReceipt, 200)
	if _, err = cs.ProcessCorrectionPlans(ctx, 100); err != nil {
		t.Fatal(err)
	}
	if replay := correctionCall(f.managementHTTP, "POST", plansPath+"/"+plan.ID+"/retry", "correction-http-plan-retry-01", f.token, managedBrand, planBody); replay.Body.String() != planReceipt.Body.String() {
		var a, b commission.CorrectionPlan
		managedData(t, replay, &a)
		managedData(t, planReceipt, &b)
		if !reflect.DeepEqual(a, b) {
			t.Fatal("plan retry receipt changed after readiness")
		}
	}
	r = correctionCall(f.managementHTTP, "GET", plansPath+"/"+plan.ID, "", f.token, managedBrand, "")
	mustStatus(t, r, 200)
	managedData(t, r, &plan)
	if plan.State != "ready" || plan.DebitPoints == nil || *plan.DebitPoints != "1" {
		t.Fatalf("actual corrected difference wrong: %+v", plan)
	}
	mustStatus(t, correctionCall(f.managementHTTP, "GET", plansPath+"/"+plan.ID+"/targets", "", f.token, managedBrand, ""), 200)
	mustStatus(t, correctionCall(f.managementHTTP, "PUT", correctionHTTPPrefix+correctionPolicyPath, "correction-http-financial-opt-in-01", f.token, managedBrand, `{"version":1,"enabled":true,"reason":"explicit new financial correction opt-in"}`), 200)
	if _, err = cs.ProcessCorrectionExecutions(ctx, 100); err != nil {
		t.Fatal(err)
	}
	r = correctionCall(f.managementHTTP, "GET", executionsPath, "", f.token, managedBrand, "")
	mustStatus(t, r, 200)
	var jobs commission.CorrectionExecutionPage
	managedData(t, r, &jobs)
	if len(jobs.Items) != 1 || jobs.Items[0].State != "awaiting_approval" {
		t.Fatal("old approval authorized new correction", jobs)
	}
	x := jobs.Items[0]
	actionPath := executionsPath + "/" + x.ID
	approveBody := `{"version":1,"reason":"fresh approval of actual one-point reclaim"}`
	approved := correctionCall(f.managementHTTP, "POST", actionPath+"/approve", "correction-http-new-approve-01", f.token, managedBrand, approveBody)
	mustStatus(t, approved, 200)
	var approval commission.CorrectionExecution
	managedData(t, approved, &approval)
	wallet, err = store.Read(ctx, managedBrand, beneficiary)
	if err != nil {
		t.Fatal(err)
	}
	freeze := f.call("POST", "/api/v1/admin/wallets/"+beneficiary+"/freeze", "correction-http-real-freeze-01", f.token, managedBrand, map[string]string{"points": strconv.FormatInt(int64(wallet.AvailablePoints), 10), "reason": "actual full available hold, not an artificial balance edit"})
	mustStatus(t, freeze, 200)
	var freezeEntry points.Entry
	managedData(t, freeze, &freezeEntry)
	if _, err = cs.ProcessCorrectionExecutions(ctx, 100); err != nil {
		t.Fatal(err)
	}
	r = correctionCall(f.managementHTTP, "GET", actionPath, "", f.token, managedBrand, "")
	mustStatus(t, r, 200)
	managedData(t, r, &x)
	if x.State != "paused" || !x.CycleHoldActive {
		t.Fatalf("real C shortage not paused: %+v", x)
	}
	mustStatus(t, f.call("POST", "/api/v1/admin/wallets/"+beneficiary+"/unfreeze", "correction-http-real-unfreeze-01", f.token, managedBrand, map[string]string{"entry_id": freezeEntry.ID, "reason": "operator restores original available sources"}), 200)
	if n, err := cs.ProcessCorrectionExecutions(ctx, 100); err != nil || n != 0 {
		t.Fatalf("funding resumed financial work without continue: n=%d err=%v", n, err)
	}
	continueBody := `{"version":` + strconv.FormatInt(x.Version, 10) + `,"reason":"explicitly release current cycle hold"}`
	continued := correctionCall(f.managementHTTP, "POST", actionPath+"/continue", "correction-http-continue-01", f.token, managedBrand, continueBody)
	mustStatus(t, continued, 200)
	if _, err = f.pool.Exec(ctx, `CREATE FUNCTION fail_http_correction_target() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='commission.correction_execution.target' THEN RAISE EXCEPTION 'test target audit outage'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_http_correction_target BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION fail_http_correction_target()`); err != nil {
		t.Fatal(err)
	}
	if _, err = cs.ProcessCorrectionExecutions(ctx, 100); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `DROP TRIGGER fail_http_correction_target ON audit_logs; DROP FUNCTION fail_http_correction_target()`); err != nil {
		t.Fatal(err)
	}
	r = correctionCall(f.managementHTTP, "GET", actionPath, "", f.token, managedBrand, "")
	mustStatus(t, r, 200)
	managedData(t, r, &x)
	if x.State != "failed" || x.AppliedCount != "0" {
		t.Fatalf("actual target fault did not roll back: %+v", x)
	}
	retryBody := `{"version":` + strconv.FormatInt(x.Version, 10) + `,"reason":"explicitly retry repaired target audit"}`
	retried := correctionCall(f.managementHTTP, "POST", actionPath+"/retry", "correction-http-execute-retry-01", f.token, managedBrand, retryBody)
	mustStatus(t, retried, 200)
	if _, err = cs.ProcessCorrectionExecutions(ctx, 100); err != nil {
		t.Fatal(err)
	}
	r = correctionCall(f.managementHTTP, "GET", actionPath, "", f.token, managedBrand, "")
	mustStatus(t, r, 200)
	managedData(t, r, &x)
	if x.State != "completed" || x.AppliedDebitPoints != "1" || x.AppliedCount != "1" {
		t.Fatalf("actual financial correction not completed: %+v", x)
	}
	for _, entry := range []struct {
		suffix, key, body string
		first             *httptest.ResponseRecorder
	}{{"approve", "correction-http-new-approve-01", approveBody, approved}, {"continue", "correction-http-continue-01", continueBody, continued}, {"retry", "correction-http-execute-retry-01", retryBody, retried}} {
		r = correctionCall(f.managementHTTP, "POST", actionPath+"/"+entry.suffix, entry.key, f.token, managedBrand, entry.body)
		mustStatus(t, r, 200)
		var old, replay commission.CorrectionExecution
		managedData(t, entry.first, &old)
		managedData(t, r, &replay)
		if !reflect.DeepEqual(old, replay) || replay.State != "applying" {
			t.Fatalf("%s receipt replaced by current completion: original=%+v replay=%+v", entry.suffix, old, replay)
		}
	}
	r = correctionCall(f.managementHTTP, "GET", actionPath+"/targets", "", f.token, managedBrand, "")
	mustStatus(t, r, 200)
	var targets commission.CorrectionExecutionTargetPage
	managedData(t, r, &targets)
	if len(targets.Items) != 1 || targets.Items[0].DeltaPoints != -1 || targets.Items[0].LedgerEntryID == nil {
		t.Fatalf("actual target financial witness absent: %+v", targets)
	}
	wallet, err = store.Read(ctx, managedBrand, beneficiary)
	if err != nil || wallet.CommissionPoints != 0 || wallet.RechargePoints != 100 {
		t.Fatalf("reclaim touched non-commission source or failed: %+v err=%v", wallet, err)
	}
	if _, err = f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='commission_correction.approve.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, correctionCall(f.managementHTTP, "POST", actionPath+"/approve", "correction-http-new-approve-01", f.token, managedBrand, approveBody), 403)
	var n int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type='commission_correction'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("receipt replay appended financial postings: n=%d err=%v", n, err)
	}
}
