package commission

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

const batchBrandID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
const batchAnchorID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
const batchAdminID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"

func TestCreateCycleInputClosedAndValidated(t *testing.T) {
	valid := `{"anchor_order_id":"` + batchAnchorID + `","reason":"manual cycle"}`
	var input CreateCycleInput
	if err := json.Unmarshal([]byte(valid), &input); err != nil || input.Validate() != nil {
		t.Fatalf("valid input rejected: %+v, %v", input, err)
	}
	for _, raw := range []string{
		`{}`,
		`null`,
		valid + ` {}`,
		`{"anchor_order_id":"` + batchAnchorID + `","anchor_order_id":"` + batchAnchorID + `","reason":"x"}`,
		`{"anchor_order_id":"` + batchAnchorID + `","reason":"x","extra":true}`,
		`{"anchor_order_id":"` + strings.ToUpper(batchAnchorID) + `","reason":"x"}`,
		`{"anchor_order_id":"` + batchAnchorID + `","reason":" x"}`,
		`{"anchor_order_id":"` + batchAnchorID + `","reason":"x\u0000y"}`,
	} {
		var got CreateCycleInput
		if err := json.Unmarshal([]byte(raw), &got); err == nil {
			t.Errorf("invalid create input accepted: %s", raw)
		}
	}
	tooLong := strings.Repeat("x", 501)
	if (CreateCycleInput{AnchorOrderID: batchAnchorID, Reason: tooLong}).Validate() == nil {
		t.Fatal("reason longer than 500 bytes accepted")
	}
}

func TestRetryCycleInputSafeVersionAndClosedShape(t *testing.T) {
	var input RetryCycleInput
	if err := json.Unmarshal([]byte(`{"version":9007199254740991,"reason":"retry"}`), &input); err != nil || input.Validate() != nil {
		t.Fatalf("max safe version rejected: %+v, %v", input, err)
	}
	for _, raw := range []string{
		`{"version":0,"reason":"retry"}`,
		`{"version":9007199254740992,"reason":"retry"}`,
		`{"version":1.5,"reason":"retry"}`,
		`{"version":1,"reason":"retry","extra":true}`,
		`{"version":1,"reason":null}`,
	} {
		var got RetryCycleInput
		if err := json.Unmarshal([]byte(raw), &got); err == nil {
			t.Errorf("invalid retry input accepted: %s", raw)
		}
	}
}

func TestAllowedCycleRequiresViewAndExactBrandMutation(t *testing.T) {
	grant := func(action string, scope access.Scope, roleBrand string) access.Role {
		return access.Role{BrandID: roleBrand, Permissions: []access.Permission{{Resource: "commission", Action: action, Scope: scope}}}
	}
	brandViewer := access.Account{ID: batchAdminID, Type: access.AccountAdmin, BrandIDs: []string{batchBrandID}, Roles: []access.Role{grant("view", access.ScopeBrand, batchBrandID)}}
	if !AllowedCycle(brandViewer, batchBrandID, "view") || AllowedCycle(brandViewer, batchBrandID, "run") {
		t.Fatal("brand viewer action grants are incorrect")
	}
	brandRunner := brandViewer
	brandRunner.Roles = append(append([]access.Role(nil), brandViewer.Roles...), grant("run", access.ScopeBrand, batchBrandID), grant("retry", access.ScopeBrand, batchBrandID))
	if !AllowedCycle(brandRunner, batchBrandID, "run") || !AllowedCycle(brandRunner, batchBrandID, "retry") {
		t.Fatal("brand-scoped writer with view access was denied")
	}
	if AllowedCycle(brandRunner, "dddddddd-dddd-4ddd-8ddd-dddddddddddd", "run") {
		t.Fatal("brand permission crossed into another brand")
	}
	platformViewer := access.Account{ID: batchAdminID, Type: access.AccountAdmin, Roles: []access.Role{grant("view", access.ScopePlatform, "")}}
	if !AllowedCycle(platformViewer, batchBrandID, "view") || AllowedCycle(platformViewer, batchBrandID, "run") {
		t.Fatal("platform view incorrectly grants a mutation")
	}
	brandRunner.SuperAdmin = true
	if AllowedCycle(brandRunner, batchBrandID, "run") {
		t.Fatal("super administrator was allowed to mutate")
	}
}

func TestCycleReadDTOKeepsLargeCountsAsStringsAndOmitsInternals(t *testing.T) {
	cycle := Cycle{
		ID: batchAnchorID, BrandID: batchBrandID,
		WindowFrom:    time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		WindowTo:      time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		AnchorOrderID: batchAnchorID, Calendar: Calendar{Timezone: "UTC", Cycle: "monthly", BoundaryTime: "00:00:00", MonthDay: batchIntPtr(1), ShortMonth: "last_day"},
		Version: 9007199254740991, TargetCount: "9007199254740993", CalculatedCount: "0", EarningCount: "0", TotalPoints: "0",
	}
	raw, err := json.Marshal(cycle)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, `"target_count":"9007199254740993"`) || strings.Contains(text, "cursor") || strings.Contains(text, "rule_snapshot") {
		t.Fatalf("cycle JSON has unsafe count or internal field: %s", text)
	}
	earningRaw, err := json.Marshal(Earning{ExactAmount: ExactAmount{Numerator: "1", Denominator: "3"}, Points: points.Amount(7)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(earningRaw), `"exact_amount":{"numerator":"1","denominator":"3"}`) || !strings.Contains(string(earningRaw), `"points":"7"`) {
		t.Fatalf("earning JSON lost exact numeric representation: %s", earningRaw)
	}
}

func TestCyclePagingBounds(t *testing.T) {
	for _, tc := range []struct {
		brand         string
		limit, offset int
		want          bool
	}{
		{batchBrandID, 1, 0, true},
		{batchBrandID, 100, 1_000_000, true},
		{batchBrandID, 0, 0, false},
		{batchBrandID, 101, 0, false},
		{batchBrandID, 1, -1, false},
		{batchBrandID, 1, 1_000_001, false},
		{strings.ToUpper(batchBrandID), 1, 0, false},
	} {
		if got := validCyclePage(tc.brand, tc.limit, tc.offset); got != tc.want {
			t.Errorf("validCyclePage(%q,%d,%d)=%t want %t", tc.brand, tc.limit, tc.offset, got, tc.want)
		}
	}
}

func batchIntPtr(value int) *int { return &value }
