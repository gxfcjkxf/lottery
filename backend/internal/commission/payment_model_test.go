package commission

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
)

const paymentBrandID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

func TestPaymentPolicyInputIsClosedAndRequiresSafeVersion(t *testing.T) {
	var input PaymentPolicyInput
	if err := json.Unmarshal([]byte(`{"version":9007199254740991,"enabled":false,"reason":"keep payouts disabled"}`), &input); err != nil || input.Validate() != nil {
		t.Fatalf("valid explicit disabled policy rejected: %+v, %v", input, err)
	}
	for _, raw := range []string{
		`{}`, `null`,
		`{"version":1,"enabled":true}`, // reason is required
		`{"version":0,"enabled":true,"reason":"enable"}`,
		`{"version":9007199254740992,"enabled":true,"reason":"enable"}`,
		`{"version":1.5,"enabled":true,"reason":"enable"}`,
		`{"version":1,"enabled":null,"reason":"enable"}`,
		`{"version":1,"enabled":true,"reason":null}`,
		`{"version":1,"enabled":true,"reason":"enable","extra":1}`,
		`{"version":1,"version":1,"enabled":true,"reason":"enable"}`,
	} {
		var got PaymentPolicyInput
		if err := json.Unmarshal([]byte(raw), &got); err == nil {
			t.Errorf("invalid payment policy input accepted: %s", raw)
		}
	}
	if (PaymentPolicyInput{Version: 1, Enabled: true, Reason: strings.Repeat("x", 501)}).Validate() == nil {
		t.Fatal("oversized audit reason accepted")
	}
}

func TestAllowedPaymentRequiresViewAndExactMutationScope(t *testing.T) {
	permission := func(resource, action string, scope access.Scope, brand string) access.Role {
		return access.Role{BrandID: brand, Permissions: []access.Permission{{Resource: resource, Action: action, Scope: scope}}}
	}
	viewer := access.Account{ID: batchAdminID, Type: access.AccountAdmin, BrandIDs: []string{paymentBrandID}, Roles: []access.Role{
		permission("commission", "view", access.ScopeBrand, paymentBrandID),
	}}
	if !AllowedPayment(viewer, paymentBrandID, "view") {
		t.Fatal("brand viewer denied payment read")
	}
	for _, action := range []string{"approve", "retry", "policy_write"} {
		if AllowedPayment(viewer, paymentBrandID, action) {
			t.Errorf("view-only account allowed %s", action)
		}
	}

	writer := viewer
	writer.Roles = append(append([]access.Role(nil), viewer.Roles...),
		permission("commission_payment", "approve", access.ScopeBrand, paymentBrandID),
		permission("commission_payment", "retry", access.ScopeBrand, paymentBrandID),
		permission("commission_payment_policy", "write", access.ScopeBrand, paymentBrandID),
	)
	for _, action := range []string{"approve", "retry", "policy_write"} {
		if !AllowedPayment(writer, paymentBrandID, action) {
			t.Errorf("proper brand writer denied %s", action)
		}
		if AllowedPayment(writer, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", action) {
			t.Errorf("%s permission crossed brands", action)
		}
	}

	platformViewer := access.Account{ID: batchAdminID, Type: access.AccountAdmin, Roles: []access.Role{
		permission("commission", "view", access.ScopePlatform, ""),
	}}
	if !AllowedPayment(platformViewer, paymentBrandID, "view") || AllowedPayment(platformViewer, paymentBrandID, "approve") {
		t.Fatal("platform view must not grant payout mutations")
	}
	writer.SuperAdmin = true
	if AllowedPayment(writer, paymentBrandID, "approve") || AllowedPayment(writer, paymentBrandID, "policy_write") {
		t.Fatal("super-admin was allowed to mutate payment state")
	}
	writer.SuperAdmin = false
	writer.Type = access.AccountUser
	if AllowedPayment(writer, paymentBrandID, "retry") {
		t.Fatal("non-admin account was allowed to retry payment")
	}
}

func TestPaymentProjectionUsesExactDecimalStringsAndSnakeCase(t *testing.T) {
	stamp := time.Date(2026, 10, 7, 1, 2, 3, 0, time.UTC)
	value := Payment{
		ID: batchAnchorID, BrandID: paymentBrandID, CycleID: batchAnchorID, RunID: batchAdminID,
		State: "pending", PayoutMode: PayoutManual, Version: maxCycleVersion, EvidenceEpoch: "9007199254740993",
		TotalPoints: "9007199254740993", PaidPoints: "9007199254740992", TargetCount: "9007199254740994", PaidCount: "9007199254740991",
		CreationAuditLogID: batchAdminID, CreatedAt: stamp, UpdatedAt: stamp,
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, field := range []string{`"brand_id"`, `"cycle_id"`, `"run_id"`, `"payout_mode"`, `"evidence_epoch":"9007199254740993"`, `"total_points":"9007199254740993"`, `"paid_points":"9007199254740992"`, `"target_count":"9007199254740994"`, `"paid_count":"9007199254740991"`, `"creation_audit_log_id"`} {
		if !strings.Contains(text, field) {
			t.Errorf("payment projection missing %s: %s", field, text)
		}
	}
	if strings.Contains(text, "BrandID") || strings.Contains(text, "account_id") || strings.Contains(text, "agent_id") {
		t.Fatalf("payment projection leaked internal or non-snake-case data: %s", text)
	}
}

func TestPaymentPagingBounds(t *testing.T) {
	for _, tc := range []struct {
		brand         string
		limit, offset int
		want          bool
	}{
		{paymentBrandID, 1, 0, true},
		{paymentBrandID, 100, 1_000_000, true},
		{paymentBrandID, 0, 0, false},
		{paymentBrandID, 101, 0, false},
		{paymentBrandID, 20, -1, false},
		{paymentBrandID, 20, 1_000_001, false},
		{strings.ToUpper(paymentBrandID), 20, 0, false},
	} {
		if got := validPaymentPage(tc.brand, tc.limit, tc.offset); got != tc.want {
			t.Errorf("validPaymentPage(%q,%d,%d)=%t want %t", tc.brand, tc.limit, tc.offset, got, tc.want)
		}
	}
}
