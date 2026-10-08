package commission

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

const correctionPlanTestBrand = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

func correctionString(value string) *string { return &value }
func correctionInt64(value int64) *int64    { return &value }

func TestCorrectionPlanDTOUsesFrozenSnakeCaseShape(t *testing.T) {
	stamp := time.Date(2026, 10, 8, 1, 2, 3, 4, time.UTC)
	plan := CorrectionPlan{
		ID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", BrandID: correctionPlanTestBrand,
		CycleID: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", PaymentID: "dddddddd-dddd-4ddd-8ddd-dddddddddddd",
		RunID: "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee", State: "ready", PayoutMode: PayoutManual,
		Version: maxCycleVersion, EvidenceEpoch: "9007199254740993", BeforePoints: "9223372036854775807",
		CalculatedPoints: "9007199254740993", CreditPoints: correctionString("1"), DebitPoints: correctionString("2"),
		NetPoints: correctionString("-1"), TargetCount: "9007199254740993", PlannedCount: "9007199254740992",
		CreationAuditLogID: "ffffffff-ffff-4fff-8fff-ffffffffffff", LastAuditLogID: "99999999-9999-4999-8999-999999999999",
		LastErrorCode: nil, CreatedAt: stamp, UpdatedAt: stamp,
	}
	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if err = json.Unmarshal(raw, &object); err != nil {
		t.Fatal(err)
	}
	wantFields := []string{"id", "brand_id", "cycle_id", "payment_id", "run_id", "state", "payout_mode", "version", "evidence_epoch", "before_points", "calculated_points", "credit_points", "debit_points", "net_points", "target_count", "planned_count", "creation_audit_log_id", "last_audit_log_id", "last_error_code", "created_at", "updated_at"}
	if len(object) != len(wantFields) {
		t.Fatalf("plan field count=%d want=%d JSON=%s", len(object), len(wantFields), raw)
	}
	for _, key := range wantFields {
		if _, ok := object[key]; !ok {
			t.Errorf("plan JSON missing %q: %s", key, raw)
		}
	}
	for key, want := range map[string]string{"version": "9007199254740991", "evidence_epoch": `"9007199254740993"`, "before_points": `"9223372036854775807"`, "credit_points": `"1"`, "net_points": `"-1"`, "last_error_code": "null"} {
		if got := string(object[key]); got != want {
			t.Errorf("%s=%s want %s", key, got, want)
		}
	}
	for _, forbidden := range []string{"account_id", "wallet", "ledger_entry_id", "PointsBefore", "OriginalPoints"} {
		if strings.Contains(string(raw), forbidden) {
			t.Errorf("plan leaked field %q: %s", forbidden, raw)
		}
	}
	blocked := plan
	blocked.State = CorrectionPlanBlocked
	blocked.LastErrorCode = correctionString("COMMISSION_CORRECTION_MANUAL_POLICY_UNRESOLVED")
	blocked.CreditPoints, blocked.DebitPoints, blocked.NetPoints = nil, nil, nil
	blockedRaw, err := json.Marshal(blocked)
	if err != nil || !strings.Contains(string(blockedRaw), `"credit_points":null`) || !strings.Contains(string(blockedRaw), `"debit_points":null`) || !strings.Contains(string(blockedRaw), `"net_points":null`) {
		t.Fatalf("manual-policy-blocked amounts must preserve explicit nulls: %s err=%v", blockedRaw, err)
	}

	target := CorrectionPlanTarget{
		ID: "12121212-1212-4212-8212-121212121212", BrandID: correctionPlanTestBrand,
		PlanID: plan.ID, AgentID: "13131313-1313-4313-8313-131313131313", MemberID: "14141414-1414-4414-8414-141414141414",
		OriginalTargetID: nil, EarningID: correctionString("15151515-1515-4515-8515-151515151515"), AdjustmentVersion: correctionInt64(maxCycleVersion),
		PointsBefore: points.Amount(math.MaxInt64), PointsAfter: points.Amount(math.MaxInt64 - 1), DeltaPoints: -1,
		CreationAuditLogID: plan.CreationAuditLogID, CreatedAt: stamp,
	}
	targetRaw, err := json.Marshal(target)
	if err != nil {
		t.Fatal(err)
	}
	var targetObject map[string]json.RawMessage
	if err = json.Unmarshal(targetRaw, &targetObject); err != nil {
		t.Fatal(err)
	}
	targetFields := []string{"id", "brand_id", "plan_id", "agent_id", "member_id", "original_target_id", "earning_id", "adjustment_version", "points_before", "points_after", "delta_points", "creation_audit_log_id", "created_at"}
	if len(targetObject) != len(targetFields) {
		t.Fatalf("target field count=%d want=%d JSON=%s", len(targetObject), len(targetFields), targetRaw)
	}
	for _, key := range targetFields {
		if _, ok := targetObject[key]; !ok {
			t.Errorf("target JSON missing %q: %s", key, targetRaw)
		}
	}
	for _, field := range []string{`"points_before":"9223372036854775807"`, `"points_after":"9223372036854775806"`, `"delta_points":"-1"`, `"original_target_id":null`, `"earning_id":"15151515-1515-4515-8515-151515151515"`, `"adjustment_version":9007199254740991`} {
		if !strings.Contains(string(targetRaw), field) {
			t.Errorf("target JSON missing %s: %s", field, targetRaw)
		}
	}
	for _, forbidden := range []string{"account_id", "wallet", "ledger_entry_id", "reason", "rule"} {
		if strings.Contains(string(targetRaw), forbidden) {
			t.Errorf("target leaked %q: %s", forbidden, targetRaw)
		}
	}

	planPage, err := json.Marshal(CorrectionPlanPage{BrandID: correctionPlanTestBrand, Items: []CorrectionPlan{plan}, TotalCount: "1", Limit: 20})
	if err != nil || !strings.Contains(string(planPage), `"total_count":"1"`) {
		t.Fatalf("plan page JSON=%s err=%v", planPage, err)
	}
	targetPage, err := json.Marshal(CorrectionPlanTargetPage{BrandID: correctionPlanTestBrand, PlanID: plan.ID, Items: []CorrectionPlanTarget{target}, TotalCount: "1", Limit: 20})
	if err != nil || !strings.Contains(string(targetPage), `"plan_id":"`+plan.ID+`"`) {
		t.Fatalf("target page JSON=%s err=%v", targetPage, err)
	}
}

func TestComputeCorrectionDeltaUsesSafeSignedInt64Math(t *testing.T) {
	tests := []struct {
		name   string
		before points.Amount
		after  points.Amount
		want   points.Amount
		bad    bool
	}{
		{"same", 17, 17, 0, false},
		{"credit", 10, 15, 5, false},
		{"debit", 15, 10, -5, false},
		{"maximum credit", 0, points.Amount(math.MaxInt64), points.Amount(math.MaxInt64), false},
		{"maximum debit", points.Amount(math.MaxInt64), 0, points.Amount(-math.MaxInt64), false},
		{"negative before", -1, 0, 0, true},
		{"negative after", 0, -1, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ComputeCorrectionDelta(tc.before, tc.after)
			if tc.bad {
				if err != ErrInvalid {
					t.Fatalf("err=%v want ErrInvalid", err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("delta=%d err=%v want=%d", got, err, tc.want)
			}
		})
	}
}

func TestMergeCorrectionModeAllModePairsAndRejectsUnknown(t *testing.T) {
	modes := []string{PayoutManual, PayoutAutomatic, PayoutMixed, PayoutNone}
	want := map[string]map[string]string{
		PayoutManual:    {PayoutManual: PayoutManual, PayoutAutomatic: PayoutMixed, PayoutMixed: PayoutMixed, PayoutNone: PayoutManual},
		PayoutAutomatic: {PayoutManual: PayoutMixed, PayoutAutomatic: PayoutAutomatic, PayoutMixed: PayoutMixed, PayoutNone: PayoutAutomatic},
		PayoutMixed:     {PayoutManual: PayoutMixed, PayoutAutomatic: PayoutMixed, PayoutMixed: PayoutMixed, PayoutNone: PayoutMixed},
		PayoutNone:      {PayoutManual: PayoutManual, PayoutAutomatic: PayoutAutomatic, PayoutMixed: PayoutMixed, PayoutNone: PayoutNone},
	}
	for _, original := range modes {
		for _, current := range modes {
			got, err := MergeCorrectionMode(original, current)
			if err != nil || got != want[original][current] {
				t.Errorf("MergeCorrectionMode(%q,%q)=(%q,%v), want %q", original, current, got, err, want[original][current])
			}
		}
	}
	for _, tc := range [][2]string{{"unknown", PayoutManual}, {PayoutManual, "automatic-ish"}, {"", "none"}} {
		if _, err := MergeCorrectionMode(tc[0], tc[1]); err != ErrInvalid {
			t.Errorf("unknown mode pair %q/%q err=%v", tc[0], tc[1], err)
		}
	}
}

func TestAllowedCorrectionPlanSeparatesViewAndBrandRetry(t *testing.T) {
	role := func(brand, resource, action string, scope access.Scope) access.Role {
		return access.Role{BrandID: brand, Permissions: []access.Permission{{Resource: resource, Action: action, Scope: scope}}}
	}
	brandView := access.Account{ID: batchAdminID, Type: access.AccountAdmin, BrandIDs: []string{correctionPlanTestBrand}, Roles: []access.Role{role(correctionPlanTestBrand, "commission", "view", access.ScopeBrand)}}
	if !AllowedCorrectionPlan(brandView, correctionPlanTestBrand, "view") || AllowedCorrectionPlan(brandView, correctionPlanTestBrand, "retry") {
		t.Fatal("view-only brand role must read but cannot retry")
	}
	retryRole := role(correctionPlanTestBrand, "commission_correction", "retry", access.ScopeBrand)
	writer := brandView
	writer.Roles = append(append([]access.Role(nil), brandView.Roles...), retryRole)
	if !AllowedCorrectionPlan(writer, correctionPlanTestBrand, "retry") {
		t.Fatal("authorized ordinary brand reviewer denied retry")
	}
	if AllowedCorrectionPlan(writer, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "retry") {
		t.Fatal("retry grant crossed brands")
	}
	if AllowedCorrectionPlan(writer, correctionPlanTestBrand, "approve") {
		t.Fatal("unknown correction action allowed")
	}
	writer.SuperAdmin = true
	if AllowedCorrectionPlan(writer, correctionPlanTestBrand, "retry") {
		t.Fatal("super-admin allowed to retry")
	}
	writer.SuperAdmin, writer.Type = false, access.AccountUser
	if AllowedCorrectionPlan(writer, correctionPlanTestBrand, "retry") {
		t.Fatal("non-admin allowed to retry")
	}

	platformViewer := access.Account{ID: batchAdminID, Type: access.AccountAdmin, Roles: []access.Role{role("", "commission", "view", access.ScopePlatform)}}
	if !AllowedCorrectionPlan(platformViewer, correctionPlanTestBrand, "view") || AllowedCorrectionPlan(platformViewer, correctionPlanTestBrand, "retry") {
		t.Fatal("platform view must allow read but not retry")
	}
	platformPlusRetry := platformViewer
	platformPlusRetry.BrandIDs = []string{correctionPlanTestBrand}
	platformPlusRetry.Roles = append(platformPlusRetry.Roles, retryRole)
	if !AllowedCorrectionPlan(platformPlusRetry, correctionPlanTestBrand, "retry") {
		t.Fatal("platform view plus brand retry grant should authorize ordinary admin")
	}
	if AllowedCorrectionPlan(platformViewer, strings.ToUpper(correctionPlanTestBrand), "view") {
		t.Fatal("noncanonical brand accepted")
	}
}

func TestCorrectionPlanRetryUsesExistingClosedCycleInput(t *testing.T) {
	var input RetryCycleInput
	if err := json.Unmarshal([]byte(`{"version":7,"reason":"retry evidence scan"}`), &input); err != nil || input.Validate() != nil {
		t.Fatalf("valid existing retry input rejected: %+v err=%v", input, err)
	}
	if err := json.Unmarshal([]byte(`{"version":7,"reason":"retry evidence scan","correction":true}`), &input); err == nil {
		t.Fatal("existing closed retry input accepted an unknown field")
	}
}
