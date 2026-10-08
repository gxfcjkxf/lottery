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

const correctionExecutionTestBrand = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

func TestCorrectionExecutionDTOHasFrozenExactJSONShape(t *testing.T) {
	stamp := time.Date(2026, 10, 8, 1, 2, 3, 4, time.UTC)
	value := CorrectionExecution{
		ID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", BrandID: correctionExecutionTestBrand,
		CycleID: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", PlanID: "dddddddd-dddd-4ddd-8ddd-dddddddddddd",
		RunID: "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee", PayoutMode: PayoutMixed,
		State: CorrectionExecutionAwaitingApproval, Version: maxCycleVersion, PlanVersion: maxCycleVersion,
		EvidenceEpoch: "9007199254740993", CreditPoints: "9223372036854775808", DebitPoints: "7", NetPoints: "9223372036854775801",
		AppliedCreditPoints: "9007199254740993", AppliedDebitPoints: "0", TargetCount: "9007199254740994", AppliedCount: "9007199254740992",
		PausedPlanTargetID: nil, LastErrorCode: nil, CreationAuditLogID: "ffffffff-ffff-4fff-8fff-ffffffffffff",
		LastAuditLogID: "99999999-9999-4999-8999-999999999999", ApprovedBy: nil, ApprovalActorType: nil,
		ApprovalAuditLogID: nil, CycleHoldActive: true, CreatedAt: stamp, UpdatedAt: stamp,
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatal(err)
	}
	wantFields := []string{
		"id", "brand_id", "cycle_id", "plan_id", "run_id", "payout_mode", "state", "version", "plan_version", "evidence_epoch",
		"credit_points", "debit_points", "net_points", "applied_credit_points", "applied_debit_points", "target_count", "applied_count",
		"paused_plan_target_id", "last_error_code", "creation_audit_log_id", "last_audit_log_id", "approved_by", "approval_actor_type",
		"approval_audit_log_id", "cycle_hold_active", "created_at", "updated_at",
	}
	if len(object) != len(wantFields) {
		t.Fatalf("execution field count=%d want=%d JSON=%s", len(object), len(wantFields), raw)
	}
	for _, field := range wantFields {
		if _, ok := object[field]; !ok {
			t.Errorf("execution JSON missing %q: %s", field, raw)
		}
	}
	for field, want := range map[string]string{
		"version": "9007199254740991", "plan_version": "9007199254740991", "evidence_epoch": `"9007199254740993"`,
		"credit_points": `"9223372036854775808"`, "target_count": `"9007199254740994"`, "applied_count": `"9007199254740992"`,
		"payout_mode": `"mixed"`, "cycle_hold_active": "true", "approved_by": "null", "approval_actor_type": "null",
		"approval_audit_log_id": "null", "paused_plan_target_id": "null", "last_error_code": "null",
	} {
		if got := string(object[field]); got != want {
			t.Errorf("%s=%s want %s", field, got, want)
		}
	}
	for _, forbidden := range []string{"account_id", "wallet", "balance", "reason", "rule", "PointsBefore"} {
		if strings.Contains(string(raw), forbidden) {
			t.Errorf("execution leaked forbidden field %q: %s", forbidden, raw)
		}
	}

	approvedBy, actorType, approvalAudit := "10101010-1010-4010-8010-101010101010", "admin", "11111111-1111-4111-8111-111111111111"
	approved := value
	approved.ApprovedBy, approved.ApprovalActorType, approved.ApprovalAuditLogID = &approvedBy, &actorType, &approvalAudit
	approved.State = CorrectionExecutionApplying
	approvedRaw, err := json.Marshal(approved)
	if err != nil || !strings.Contains(string(approvedRaw), `"approved_by":"`+approvedBy+`"`) || !strings.Contains(string(approvedRaw), `"approval_actor_type":"admin"`) {
		t.Fatalf("approval witness missing from execution JSON: %s err=%v", approvedRaw, err)
	}
	if value.PayoutMode != PayoutMixed {
		t.Fatal("mixed source policy must remain mixed and be resolved by fresh manual approval")
	}

	target := CorrectionExecutionTarget{
		ID: "12121212-1212-4212-8212-121212121212", BrandID: correctionExecutionTestBrand,
		ExecutionID: value.ID, PlanTargetID: "13131313-1313-4313-8313-131313131313",
		AgentID: "14141414-1414-4414-8414-141414141414", MemberID: "15151515-1515-4515-8515-151515151515",
		PointsBefore: points.Amount(math.MaxInt64), PointsAfter: points.Amount(math.MaxInt64 - 1), DeltaPoints: -1,
		State: CorrectionExecutionTargetPending, LedgerEntryID: nil, AuditLogID: nil, FinancialVersion: nil,
		CreatedAt: stamp, AppliedAt: nil,
	}
	targetRaw, err := json.Marshal(target)
	if err != nil {
		t.Fatal(err)
	}
	var targetObject map[string]json.RawMessage
	if err := json.Unmarshal(targetRaw, &targetObject); err != nil {
		t.Fatal(err)
	}
	targetFields := []string{
		"id", "brand_id", "execution_id", "plan_target_id", "agent_id", "member_id", "points_before", "points_after", "delta_points",
		"state", "ledger_entry_id", "audit_log_id", "financial_version", "created_at", "applied_at",
	}
	if len(targetObject) != len(targetFields) {
		t.Fatalf("target field count=%d want=%d JSON=%s", len(targetObject), len(targetFields), targetRaw)
	}
	for _, field := range targetFields {
		if _, ok := targetObject[field]; !ok {
			t.Errorf("target JSON missing %q: %s", field, targetRaw)
		}
	}
	for _, field := range []string{`"points_before":"9223372036854775807"`, `"points_after":"9223372036854775806"`, `"delta_points":"-1"`, `"ledger_entry_id":null`, `"audit_log_id":null`, `"financial_version":null`, `"applied_at":null`} {
		if !strings.Contains(string(targetRaw), field) {
			t.Errorf("target JSON missing exact field %s: %s", field, targetRaw)
		}
	}
	for _, forbidden := range []string{"account_id", "wallet", "balance", "reason", "rule", "request_id"} {
		if strings.Contains(string(targetRaw), forbidden) {
			t.Errorf("target leaked forbidden field %q: %s", forbidden, targetRaw)
		}
	}
	ledgerID, auditID := "16161616-1616-4616-8616-161616161616", "17171717-1717-4717-8717-171717171717"
	financialVersion := int64(maxCycleVersion)
	appliedAt := stamp.Add(time.Second)
	target.State, target.LedgerEntryID, target.AuditLogID = CorrectionExecutionTargetApplied, &ledgerID, &auditID
	target.FinancialVersion, target.AppliedAt = &financialVersion, &appliedAt
	appliedRaw, err := json.Marshal(target)
	if err != nil || !strings.Contains(string(appliedRaw), `"financial_version":9007199254740991`) || !strings.Contains(string(appliedRaw), `"applied_at":"2026-10-08T01:02:04.000000004Z"`) {
		t.Fatalf("applied target witness serialization invalid: %s err=%v", appliedRaw, err)
	}

	pageRaw, err := json.Marshal(CorrectionExecutionPage{BrandID: correctionExecutionTestBrand, Items: []CorrectionExecution{value}, TotalCount: "9007199254740993", Limit: 20, Offset: 40})
	if err != nil || !strings.Contains(string(pageRaw), `"total_count":"9007199254740993"`) || !strings.Contains(string(pageRaw), `"limit":20`) {
		t.Fatalf("execution page serialization=%s err=%v", pageRaw, err)
	}
	targetPageRaw, err := json.Marshal(CorrectionExecutionTargetPage{BrandID: correctionExecutionTestBrand, ExecutionID: value.ID, Items: []CorrectionExecutionTarget{target}, TotalCount: "1", Limit: 20, Offset: 0})
	if err != nil || !strings.Contains(string(targetPageRaw), `"execution_id":"`+value.ID+`"`) {
		t.Fatalf("execution target page serialization=%s err=%v", targetPageRaw, err)
	}
}

func TestAllowedCorrectionExecutionSeparatesViewAndFinancialActions(t *testing.T) {
	role := func(roleBrand, resource, action string, scope access.Scope) access.Role {
		return access.Role{BrandID: roleBrand, Permissions: []access.Permission{{Resource: resource, Action: action, Scope: scope}}}
	}
	viewBrand := role(correctionExecutionTestBrand, "commission", "view", access.ScopeBrand)
	viewer := access.Account{ID: "22222222-2222-4222-8222-222222222222", Type: access.AccountAdmin, BrandIDs: []string{correctionExecutionTestBrand}, Roles: []access.Role{viewBrand}}
	if !AllowedCorrectionExecution(viewer, correctionExecutionTestBrand, "view") {
		t.Fatal("explicit brand commission viewer denied view")
	}
	for _, action := range []string{"approve", "continue", "retry", "", "unknown", "execute_retry"} {
		if AllowedCorrectionExecution(viewer, correctionExecutionTestBrand, action) {
			t.Errorf("view-only account allowed %q", action)
		}
	}

	for _, tc := range []struct {
		action, permission string
	}{
		{"approve", "approve"}, {"continue", "continue"}, {"retry", "execute_retry"},
	} {
		writer := viewer
		writer.Roles = append(append([]access.Role(nil), viewer.Roles...), role(correctionExecutionTestBrand, "commission_correction", tc.permission, access.ScopeBrand))
		if !AllowedCorrectionExecution(writer, correctionExecutionTestBrand, tc.action) {
			t.Errorf("explicit %s right with view denied %s", tc.permission, tc.action)
		}
		for _, other := range []string{"approve", "continue", "retry"} {
			if other != tc.action && AllowedCorrectionExecution(writer, correctionExecutionTestBrand, other) {
				t.Errorf("%s permission also authorized %s", tc.permission, other)
			}
		}
		if AllowedCorrectionExecution(writer, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", tc.action) {
			t.Errorf("%s right crossed brand scope", tc.action)
		}
	}
	policyWriter := viewer
	policyWriter.Roles = append(append([]access.Role(nil), viewer.Roles...), role(correctionExecutionTestBrand, "commission_correction_policy", "write", access.ScopeBrand))
	if !AllowedCorrectionExecution(policyWriter, correctionExecutionTestBrand, "policy_write") {
		t.Fatal("explicit correction policy writer with commission view denied")
	}
	if AllowedCorrectionExecution(policyWriter, correctionExecutionTestBrand, "approve") ||
		AllowedCorrectionExecution(policyWriter, correctionExecutionTestBrand, "continue") ||
		AllowedCorrectionExecution(policyWriter, correctionExecutionTestBrand, "retry") {
		t.Fatal("policy write permission leaked into execution actions")
	}
	if AllowedCorrectionExecution(viewer, correctionExecutionTestBrand, "policy_write") {
		t.Fatal("commission view alone authorized policy write")
	}
	policyWithoutView := access.Account{ID: viewer.ID, Type: access.AccountAdmin, BrandIDs: []string{correctionExecutionTestBrand}, Roles: []access.Role{
		role(correctionExecutionTestBrand, "commission_correction_policy", "write", access.ScopeBrand),
	}}
	if AllowedCorrectionExecution(policyWithoutView, correctionExecutionTestBrand, "policy_write") {
		t.Fatal("policy write without explicit commission view authorized")
	}
	policySuper := policyWriter
	policySuper.SuperAdmin = true
	if AllowedCorrectionExecution(policySuper, correctionExecutionTestBrand, "policy_write") {
		t.Fatal("super administrator authorized correction policy write")
	}
	policyUser := policyWriter
	policyUser.Type = access.AccountUser
	if AllowedCorrectionExecution(policyUser, correctionExecutionTestBrand, "policy_write") {
		t.Fatal("non-admin authorized correction policy write")
	}
	policyCrossBrand := viewer
	policyCrossBrand.Roles = append(append([]access.Role(nil), viewer.Roles...), role("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "commission_correction_policy", "write", access.ScopeBrand))
	if AllowedCorrectionExecution(policyCrossBrand, correctionExecutionTestBrand, "policy_write") {
		t.Fatal("policy write grant for another brand crossed scope")
	}

	platformViewer := access.Account{ID: viewer.ID, Type: access.AccountAdmin, Roles: []access.Role{role("", "commission", "view", access.ScopePlatform)}}
	if !AllowedCorrectionExecution(platformViewer, correctionExecutionTestBrand, "view") || AllowedCorrectionExecution(platformViewer, correctionExecutionTestBrand, "approve") {
		t.Fatal("platform view should permit viewing only")
	}
	platformWithBrandApproval := platformViewer
	platformWithBrandApproval.BrandIDs = []string{correctionExecutionTestBrand}
	platformWithBrandApproval.Roles = append(platformWithBrandApproval.Roles, role(correctionExecutionTestBrand, "commission_correction", "approve", access.ScopeBrand))
	if !AllowedCorrectionExecution(platformWithBrandApproval, correctionExecutionTestBrand, "approve") {
		t.Fatal("platform viewer with explicit brand approval right should be authorized")
	}

	// The legacy correction-plan retry grant is preparation-only and must never
	// stand in for the financial execution retry permission.
	legacyRetry := viewer
	legacyRetry.Roles = append(append([]access.Role(nil), viewer.Roles...), role(correctionExecutionTestBrand, "commission_correction", "retry", access.ScopeBrand))
	if AllowedCorrectionExecution(legacyRetry, correctionExecutionTestBrand, "retry") {
		t.Fatal("commission_correction.retry.brand authorized execution retry")
	}

	noView := access.Account{ID: viewer.ID, Type: access.AccountAdmin, BrandIDs: []string{correctionExecutionTestBrand}, Roles: []access.Role{
		role(correctionExecutionTestBrand, "commission_correction", "approve", access.ScopeBrand),
	}}
	if AllowedCorrectionExecution(noView, correctionExecutionTestBrand, "approve") {
		t.Fatal("brand approval right without explicit commission view authorized action")
	}
	super := platformWithBrandApproval
	super.SuperAdmin = true
	if AllowedCorrectionExecution(super, correctionExecutionTestBrand, "approve") {
		t.Fatal("super administrator authorized financial approval")
	}
	user := platformWithBrandApproval
	user.Type = access.AccountUser
	if AllowedCorrectionExecution(user, correctionExecutionTestBrand, "approve") {
		t.Fatal("non-admin account authorized financial approval")
	}
	for _, malformed := range []string{"", "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaz", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"} {
		if AllowedCorrectionExecution(platformViewer, malformed, "view") || AllowedCorrectionExecution(platformWithBrandApproval, malformed, "approve") {
			t.Errorf("malformed/noncanonical brand %q authorized", malformed)
		}
	}
}

func TestCorrectionExecutionPolicyMatchesExistingPolicyProjectionAndInput(t *testing.T) {
	stamp := time.Date(2026, 10, 8, 1, 2, 3, 4, time.UTC)
	policy := CorrectionExecutionPolicy{
		BrandID: correctionExecutionTestBrand, Version: maxCycleVersion, Enabled: false,
		AuditLogID: "18181818-1818-4818-8818-181818181818", UpdatedAt: stamp,
	}
	raw, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 5 {
		t.Fatalf("correction policy has unexpected JSON shape: %s", raw)
	}
	for key, want := range map[string]string{
		"brand_id": `"` + correctionExecutionTestBrand + `"`, "version": "9007199254740991",
		"enabled": "false", "audit_log_id": `"18181818-1818-4818-8818-181818181818"`,
		"updated_at": `"2026-10-08T01:02:03.000000004Z"`,
	} {
		if got := string(fields[key]); got != want {
			t.Errorf("policy %s=%s want %s", key, got, want)
		}
	}
	var input PaymentPolicyInput
	if err := json.Unmarshal([]byte(`{"version":3,"enabled":true,"reason":"explicit correction gate opt-in"}`), &input); err != nil || input.Validate() != nil {
		t.Fatalf("existing closed policy input cannot be reused: %+v err=%v", input, err)
	}
	for _, raw := range []string{
		`{"version":3,"enabled":true,"reason":"explicit correction gate opt-in","retroactive":true}`,
		`{"version":3,"enabled":true,"reason":"explicit correction gate opt-in","execute":true}`,
	} {
		var invalid PaymentPolicyInput
		if err := json.Unmarshal([]byte(raw), &invalid); err == nil {
			t.Errorf("existing closed policy input accepted execution expansion: %s", raw)
		}
	}
}

func TestCorrectionExecutionErrorsAndStatesAreDistinct(t *testing.T) {
	if ErrCorrectionExecutionState == nil || ErrCorrectionExecutionVersion == nil || ErrCorrectionExecutionEvidence == nil {
		t.Fatal("execution errors must be defined")
	}
	if ErrCorrectionExecutionState == ErrCorrectionExecutionVersion || ErrCorrectionExecutionState == ErrCorrectionExecutionEvidence || ErrCorrectionExecutionVersion == ErrCorrectionExecutionEvidence {
		t.Fatal("execution conflict errors must be distinct")
	}
	if CorrectionExecutionAwaitingApproval != "awaiting_approval" || CorrectionExecutionApplying != "applying" ||
		CorrectionExecutionCompleted != "completed" || CorrectionExecutionPaused != "paused" ||
		CorrectionExecutionFailed != "failed" || CorrectionExecutionStale != "stale" ||
		CorrectionExecutionTargetPending != "pending" || CorrectionExecutionTargetApplied != "applied" {
		t.Fatal("frozen correction execution state values changed")
	}
}
