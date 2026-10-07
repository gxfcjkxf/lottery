package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
)

const commissionPaymentPolicyHTTPPath = "/api/v1/admin/commission-payment-policy"
const commissionPaymentsHTTPPath = "/api/v1/admin/commission-payments"

func paymentRawCall(f managementHTTP, method, path, key, token, brand, body string, actors ...string) *httptest.ResponseRecorder {
	actor := f.root
	if len(actors) == 1 {
		actor = actors[0]
	}
	r := httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(body))
	r.RemoteAddr = "192.0.2.55:12345"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", key)
	r.Header.Set("X-Brand-ID", brand)
	r.Header.Set("X-Commission-Payment-Actor-ID", actor)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.http.ServeHTTP(w, r)
	return w
}

func TestCommissionPaymentActorBindingPrecedesNewWriteAndReceiptReplay(t *testing.T) {
	f := managedFixture(t)
	grantCommissionPaymentPolicyWriter(t, f)
	ctx := context.Background()
	body := `{"version":1,"enabled":true,"reason":"bind immutable financial intent to original actor"}`
	for _, actor := range []string{"", ids.New()} {
		out := paymentRawCall(f, "PUT", commissionPaymentPolicyHTTPPath, "actor-bound-policy-01", f.token, managedBrand, body, actor)
		want := 401
		if actor == "" {
			want = 400
		}
		mustStatus(t, out, want)
	}
	var version, receipts int64
	if err := f.pool.QueryRow(ctx, `SELECT version FROM brand_commission_payment_policies WHERE brand_id=$1`, managedBrand).Scan(&version); err != nil || version != 1 {
		t.Fatal("wrong actor changed policy", version, err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM idempotency_requests WHERE operation='admin.commission_payment_policy.update'`).Scan(&receipts); err != nil || receipts != 0 {
		t.Fatal("wrong actor cached financial intent", receipts, err)
	}
	first := paymentRawCall(f, "PUT", commissionPaymentPolicyHTTPPath, "actor-bound-policy-01", f.token, managedBrand, body)
	mustStatus(t, first, 200)
	wrong := paymentRawCall(f, "PUT", commissionPaymentPolicyHTTPPath, "actor-bound-policy-01", f.token, managedBrand, body, ids.New())
	mustStatus(t, wrong, 401)
	correct := paymentRawCall(f, "PUT", commissionPaymentPolicyHTTPPath, "actor-bound-policy-01", f.token, managedBrand, body)
	mustStatus(t, correct, 200)
	var original, replayed commission.PaymentPolicy
	managedData(t, first, &original)
	managedData(t, correct, &replayed)
	if original != replayed {
		t.Fatal("bound replay replaced original receipt", original, replayed)
	}
	for _, action := range []string{"approve", "retry"} {
		permission := "commission_payment." + action + ".brand"
		if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,$2 FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root, permission); err != nil {
			t.Fatal(err)
		}
		out := paymentRawCall(f, "POST", commissionPaymentsHTTPPath+"/"+ids.New()+"/"+action, "actor-bound-"+action+"-01", f.token, managedBrand, `{"version":1,"reason":"wrong account cannot even reach task lookup"}`, ids.New())
		mustStatus(t, out, 401)
	}
}

func grantCommissionPaymentPolicyWriter(t *testing.T, f managementHTTP) {
	t.Helper()
	ctx := context.Background()
	for _, key := range []string{"commission.view.brand", "commission_payment_policy.write.brand"} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, key); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,$2 FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root, key); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCommissionPaymentHTTPRequiresAdminSessionAndClosedBodies(t *testing.T) {
	f := managedFixture(t)
	for _, path := range []string{commissionPaymentPolicyHTTPPath, commissionPaymentsHTTPPath} {
		if got := f.call("GET", path, "", "", managedBrand, nil); got.Code != 401 {
			t.Errorf("unauthenticated GET %s status=%d body=%s", path, got.Code, got.Body.String())
		}
	}
	if got := paymentRawCall(f, "POST", commissionPaymentsHTTPPath+"/0199a000-0000-7000-8000-000000000099/approve", "payment-no-session-01", "", managedBrand, `{"version":1,"reason":"approve payout"}`); got.Code != 401 {
		t.Fatalf("unauthenticated approve status=%d body=%s", got.Code, got.Body.String())
	}

	for _, raw := range []string{
		`{}`, `null`,
		`{"version":1,"enabled":true}`,
		`{"version":1,"enabled":null,"reason":"enable payments"}`,
		`{"version":1,"enabled":true,"reason":"enable payments","extra":true}`,
	} {
		got := paymentRawCall(f, "PUT", commissionPaymentPolicyHTTPPath, "payment-policy-invalid-01", f.token, managedBrand, raw)
		if got.Code != 400 {
			t.Errorf("invalid policy body accepted: status=%d body=%s raw=%s", got.Code, got.Body.String(), raw)
		}
	}
	for _, raw := range []string{`{}`, `{"version":1,"reason":null}`, `{"version":1,"reason":"approve","extra":1}`} {
		got := paymentRawCall(f, "POST", commissionPaymentsHTTPPath+"/0199a000-0000-7000-8000-000000000099/retry", "payment-action-invalid-01", f.token, managedBrand, raw)
		if got.Code != 400 {
			t.Errorf("invalid action body accepted: status=%d body=%s raw=%s", got.Code, got.Body.String(), raw)
		}
	}
}

func TestCommissionPaymentHTTPRequiresCommissionViewAndPaymentMutationPermission(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	policyRead := f.call("GET", commissionPaymentPolicyHTTPPath, "", f.token, managedBrand, nil)
	if policyRead.Code != 403 {
		t.Fatalf("read without commission.view status=%d body=%s", policyRead.Code, policyRead.Body.String())
	}
	grantCommissionCycle(t, f, "view")

	for _, action := range []string{"approve", "retry"} {
		path := commissionPaymentsHTTPPath + "/0199a000-0000-7000-8000-000000000099/" + action
		got := paymentRawCall(f, "POST", path, "payment-auth-deny-"+action+"-01", f.token, managedBrand, `{"version":1,"reason":"must have payment permission"}`)
		if got.Code != 403 {
			t.Errorf("%s without payment permission status=%d body=%s", action, got.Code, got.Body.String())
		}
	}
	policyWrite := paymentRawCall(f, "PUT", commissionPaymentPolicyHTTPPath, "payment-policy-auth-deny-01", f.token, managedBrand, `{"version":1,"enabled":true,"reason":"must have policy permission"}`)
	if policyWrite.Code != 403 {
		t.Fatalf("policy write without dedicated permission status=%d body=%s", policyWrite.Code, policyWrite.Body.String())
	}

	var denials int
	err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='access.denied' AND after_json->>'attempted_brand'=$1 AND after_json->>'permission' IN ('commission_payment.approve','commission_payment.retry','commission_payment_policy.write')`, managedBrand).Scan(&denials)
	if err != nil || denials != 3 {
		t.Fatalf("payment permission denials not audited: count=%d err=%v", denials, err)
	}
}

func TestCommissionPaymentPolicyReplayRechecksRevokedPermission(t *testing.T) {
	f := managedFixture(t)
	grantCommissionPaymentPolicyWriter(t, f)
	key := "payment-policy-revoke-replay-01"
	body := `{"version":1,"enabled":true,"reason":"enable payment processing"}`
	first := paymentRawCall(f, "PUT", commissionPaymentPolicyHTTPPath, key, f.token, managedBrand, body)
	mustStatus(t, first, 200)
	var updated commission.PaymentPolicy
	managedData(t, first, &updated)
	if updated.Version != 2 || !updated.Enabled {
		t.Fatalf("unexpected original policy receipt: %+v", updated)
	}
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE role_id IN (SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='commission_payment_policy.write.brand'`, f.root); err != nil {
		t.Fatal(err)
	}
	replay := paymentRawCall(f, "PUT", commissionPaymentPolicyHTTPPath, key, f.token, managedBrand, body)
	mustStatus(t, replay, 403)
	var version int64
	if err := f.pool.QueryRow(context.Background(), `SELECT version FROM brand_commission_payment_policies WHERE brand_id=$1`, managedBrand).Scan(&version); err != nil || version != 2 {
		t.Fatalf("revoked replay changed policy: version=%d err=%v", version, err)
	}
}

func TestCommissionPaymentPolicyBusyDoesNotCacheReceipt(t *testing.T) {
	f := managedFixture(t)
	grantCommissionPaymentPolicyWriter(t, f)
	ctx := context.Background()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT id FROM brands WHERE id=$1 FOR UPDATE`, managedBrand); err != nil {
		t.Fatal(err)
	}
	key := "payment-policy-brand-busy-01"
	body := `{"version":1,"enabled":true,"reason":"enable after brand lock"}`
	busy := paymentRawCall(f, "PUT", commissionPaymentPolicyHTTPPath, key, f.token, managedBrand, body)
	mustStatus(t, busy, 503)
	var envelope struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err = json.Unmarshal(busy.Body.Bytes(), &envelope); err != nil || envelope.Error == nil || envelope.Error.Code != "COMMISSION_PAYMENT_BUSY" {
		t.Fatalf("brand-lock contention did not return payment busy: %s err=%v", busy.Body.String(), err)
	}
	var receipts int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM idempotency_requests WHERE brand_id=$1 AND actor_id=$2 AND operation='admin.commission_payment_policy.update' AND key=$3`, managedBrand, f.root, key).Scan(&receipts); err != nil || receipts != 0 {
		t.Fatalf("busy response cached a receipt: count=%d err=%v", receipts, err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	retried := paymentRawCall(f, "PUT", commissionPaymentPolicyHTTPPath, key, f.token, managedBrand, body)
	mustStatus(t, retried, 200)
}

func TestCommissionPaymentPolicyDisabledBrandRejectsReceiptReplay(t *testing.T) {
	f := managedFixture(t)
	grantCommissionPaymentPolicyWriter(t, f)
	key := "payment-policy-disabled-replay-01"
	body := `{"version":1,"enabled":true,"reason":"enable before brand pause"}`
	first := paymentRawCall(f, "PUT", commissionPaymentPolicyHTTPPath, key, f.token, managedBrand, body)
	mustStatus(t, first, 200)
	if _, err := f.pool.Exec(context.Background(), `UPDATE brands SET status='disabled' WHERE id=$1`, managedBrand); err != nil {
		t.Fatal(err)
	}
	replay := paymentRawCall(f, "PUT", commissionPaymentPolicyHTTPPath, key, f.token, managedBrand, body)
	mustStatus(t, replay, 409)
	var envelope struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(replay.Body.Bytes(), &envelope); err != nil || envelope.Error == nil || envelope.Error.Code != "COMMISSION_PAYMENT_STATE_CONFLICT" {
		t.Fatalf("disabled brand replay was not refused as state conflict: %s err=%v", replay.Body.String(), err)
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE brands SET status='active' WHERE id=$1`, managedBrand); err != nil {
		t.Fatal(err)
	}
}
