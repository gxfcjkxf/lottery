package commission

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

const adjustmentBrandID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

func TestAdjustmentInputIsClosedAndValidatesNetAmount(t *testing.T) {
	var input AdjustmentInput
	if err := json.Unmarshal([]byte(`{"version":9007199254740991,"points":"9223372036854775807","reason":"manual review"}`), &input); err != nil || input.Validate() != nil {
		t.Fatalf("valid maximum adjustment input rejected: %+v, %v", input, err)
	}
	for _, raw := range []string{
		`{}`, `null`,
		`{"version":1,"points":"0"}`, // reason required
		`{"version":0,"points":"1","reason":"manual"}`,
		`{"version":9007199254740992,"points":"1","reason":"manual"}`,
		`{"version":1.5,"points":"1","reason":"manual"}`,
		`{"version":1,"points":1,"reason":"manual"}`,
		`{"version":1,"points":null,"reason":"manual"}`,
		`{"version":1,"points":"-1","reason":"manual"}`,
		`{"version":1,"points":"9223372036854775808","reason":"manual"}`,
		`{"version":1,"points":"01","reason":"manual"}`,
		`{"version":1,"points":"1","reason":null}`,
		`{"version":1,"points":"1","reason":" manual"}`,
		`{"version":1,"points":"1","reason":"manual ","extra":true}`,
		`{"version":1,"version":1,"points":"1","reason":"manual"}`,
	} {
		var got AdjustmentInput
		if err := json.Unmarshal([]byte(raw), &got); err == nil {
			t.Errorf("invalid adjustment input accepted: %s", raw)
		}
	}
	if (AdjustmentInput{Version: 1, Points: 1, Reason: strings.Repeat("界", 167)}).Validate() == nil {
		t.Fatal("reason longer than 500 UTF-8 bytes accepted")
	}
	if (AdjustmentInput{Version: 1, Points: 1, Reason: "x\x00y"}).Validate() == nil {
		t.Fatal("reason containing NUL accepted")
	}
}

func TestAllowedAdjustmentRequiresViewAndDedicatedBrandWrite(t *testing.T) {
	permission := func(resource, action string, scope access.Scope, brand string) access.Role {
		return access.Role{BrandID: brand, Permissions: []access.Permission{{Resource: resource, Action: action, Scope: scope}}}
	}
	viewer := access.Account{ID: batchAdminID, Type: access.AccountAdmin, BrandIDs: []string{adjustmentBrandID}, Roles: []access.Role{
		permission("commission", "view", access.ScopeBrand, adjustmentBrandID),
	}}
	if !AllowedAdjustment(viewer, adjustmentBrandID, "view") || AllowedAdjustment(viewer, adjustmentBrandID, "write") {
		t.Fatal("brand view should permit read only")
	}

	writer := viewer
	writer.Roles = append(append([]access.Role(nil), viewer.Roles...),
		permission("commission_adjustment", "write", access.ScopeBrand, adjustmentBrandID))
	if !AllowedAdjustment(writer, adjustmentBrandID, "write") {
		t.Fatal("proper brand adjustment writer denied")
	}
	if AllowedAdjustment(writer, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "write") {
		t.Fatal("adjustment permission crossed brands")
	}
	writer.SuperAdmin = true
	if AllowedAdjustment(writer, adjustmentBrandID, "write") {
		t.Fatal("super-admin was allowed to adjust")
	}
	writer.SuperAdmin, writer.Type = false, access.AccountUser
	if AllowedAdjustment(writer, adjustmentBrandID, "write") {
		t.Fatal("non-admin was allowed to adjust")
	}
	platform := access.Account{ID: batchAdminID, Type: access.AccountAdmin, Roles: []access.Role{
		permission("commission", "view", access.ScopePlatform, ""),
	}}
	if !AllowedAdjustment(platform, adjustmentBrandID, "view") || AllowedAdjustment(platform, adjustmentBrandID, "write") {
		t.Fatal("platform view must not grant brand adjustment writes")
	}
	cycleWriter := viewer
	cycleWriter.Roles = append(append([]access.Role(nil), viewer.Roles...), permission("commission", "run", access.ScopeBrand, adjustmentBrandID))
	if AllowedAdjustment(cycleWriter, adjustmentBrandID, "write") {
		t.Fatal("commission.run must not grant adjustment writes")
	}
}

func TestAdjustmentDTOUsesSanitizedSnakeCaseAndExactAmounts(t *testing.T) {
	value := Adjustment{
		ID: batchAnchorID, BrandID: adjustmentBrandID, TargetID: batchAdminID, PaymentID: batchAnchorID,
		Version: maxCycleVersion, PointsBefore: points.Amount(9007199254740993), PointsAfter: points.Amount(9007199254740994),
		DeltaPoints: 1, LedgerEntryID: batchAdminID, AuditLogID: batchAdminID, CreatedBy: batchAdminID,
		Reason: "manual review", PointPolicyVersion: "9007199254740993",
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, field := range []string{`"points_before":"9007199254740993"`, `"points_after":"9007199254740994"`, `"delta_points":"1"`, `"point_policy_version":"9007199254740993"`, `"target_id"`} {
		if !strings.Contains(text, field) {
			t.Errorf("adjustment projection missing %s: %s", field, text)
		}
	}
	for _, forbidden := range []string{"account_id", "wallet", "rule", "PointsBefore"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("adjustment projection contains forbidden/internal field %q: %s", forbidden, text)
		}
	}

	target, err := json.Marshal(Target{ID: batchAnchorID, OriginalPoints: 9, State: "pending"})
	if err != nil {
		t.Fatal(err)
	}
	var projected map[string]json.RawMessage
	if err := json.Unmarshal(target, &projected); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"adjustment_version", "adjusted_points", "last_adjustment_id"} {
		if string(projected[name]) != "null" {
			t.Errorf("headless target %s should serialize null, got %s", name, projected[name])
		}
	}
}

func TestValidAdjustmentPageBounds(t *testing.T) {
	for _, tc := range []struct {
		brand         string
		limit, offset int
		want          bool
	}{
		{adjustmentBrandID, 1, 0, true},
		{adjustmentBrandID, 100, 1_000_000, true},
		{adjustmentBrandID, 0, 0, false},
		{adjustmentBrandID, 101, 0, false},
		{adjustmentBrandID, 20, -1, false},
		{adjustmentBrandID, 20, 1_000_001, false},
		{strings.ToUpper(adjustmentBrandID), 20, 0, false},
	} {
		if got := validAdjustmentPage(tc.brand, tc.limit, tc.offset); got != tc.want {
			t.Errorf("validAdjustmentPage(%q,%d,%d)=%t want %t", tc.brand, tc.limit, tc.offset, got, tc.want)
		}
	}
}
