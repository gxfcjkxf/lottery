package httpapi

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

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

func TestBrandPaymentPolicyIsReadOnlyAndPaymentActionsBindActor(t *testing.T) {
	f := managedFixture(t)
	grantCommissionPaymentPolicyWriter(t, f)
	mustStatus(t, f.call("GET", commissionPaymentPolicyHTTPPath, "", "", managedBrand, nil), 401)
	mustStatus(t, f.call("GET", commissionPaymentPolicyHTTPPath, "", f.token, managedBrand, nil), 200)
	mustStatus(t, paymentRawCall(f, "PUT", commissionPaymentPolicyHTTPPath, "brand-denied", f.token, managedBrand, `{"version":1,"enabled":true}`), 403)
	for _, action := range []string{"approve", "retry"} {
		if _, err := f.pool.Exec(context.Background(), `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,$2 FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root, "commission_payment."+action+".brand"); err != nil {
			t.Fatal(err)
		}
		mustStatus(t, paymentRawCall(f, "POST", commissionPaymentsHTTPPath+"/"+ids.New()+"/"+action, "actor-"+action, f.token, managedBrand, `{"version":1,"reason":""}`, ids.New()), 401)
	}
}
