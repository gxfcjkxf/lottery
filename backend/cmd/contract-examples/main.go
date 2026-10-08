// contract-examples emits deterministic JSON from real DTOs and the real rule
// engine. It never opens a database, creates sessions, or posts financial data.
package main

import (
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/branddomains"
	"github.com/gxfcjkxf/lottery/backend/internal/brandops"
	"github.com/gxfcjkxf/lottery/backend/internal/brandregistry"
	"github.com/gxfcjkxf/lottery/backend/internal/brandskin"
	"github.com/gxfcjkxf/lottery/backend/internal/commission"
	"github.com/gxfcjkxf/lottery/backend/internal/compliance"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/notification"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/reconciliation"
	"github.com/gxfcjkxf/lottery/backend/internal/reporting"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/gxfcjkxf/lottery/backend/internal/tenant"
	"github.com/gxfcjkxf/lottery/backend/internal/withdrawal"
)

func main() {
	const id = "11111111-1111-4111-8111-111111111111"
	now := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	startedAt := now.Add(time.Minute)
	checkedAt := now.Add(2 * time.Minute)
	consistent := "consistent"
	consistentFilter := "consistent"
	reconciliationAuditID := id
	noticeContent := notification.Content{En: notification.Copy{Title: "Credit recorded", Body: "Credit: {points}."}, ZhCN: notification.Copy{Title: "积分入账记录", Body: "入账：{points} 积分。"}}
	if err := notification.ValidateContent("recharge.confirmed", noticeContent); err != nil {
		log.Fatal(err)
	}
	noticePoints := "9223372036854775807"
	noticeAuditID := id
	complianceConfig := compliance.DefaultConfig()
	complianceConfig.IdentityEnabled = true
	complianceResult, complianceChecks, err := compliance.Evaluate(complianceConfig)
	if err != nil {
		log.Fatal(err)
	}
	complianceDecision := compliance.Decision{ID: id, BrandID: id, PolicyVersion: 2, Config: complianceConfig, Operation: "betting", Decision: complianceResult, Checks: complianceChecks, AdapterMode: "stub", CreatedBy: id, Reason: "explicit stub check", AuditLogID: id, CreatedAt: now}
	gateRecord := compliance.GateRecord{ID: id, BrandID: id, PolicyVersion: 2, Config: complianceConfig, Operation: "registration", Action: "register", Decision: complianceResult, Checks: complianceChecks, AdapterMode: "stub", ActorType: "anonymous", RequestID: id, AuditLogID: id, CreatedAt: now}
	match := 1
	in := rules.SimulationInput{
		Definition: rules.Definition{SchemaVersion: 1, Model: rules.Model{Type: "X_PLUS_Y", RegularPool: rules.Pool{Min: 1, Max: 49}, SpecialPool: rules.Pool{Min: 1, Max: 49}, RegularCount: 6, SpecialCount: 1}, Selection: rules.SelectionRule{Mode: "numbers", SpecialCount: 1}, UnitPoints: 1, PrizeTiers: []rules.Tier{{Code: "SPECIAL_MATCH", Condition: rules.Condition{Op: "equals", Field: "special_match", Value: &match}, Odds: "35", Exclusive: true}}, Rounding: "half_up", RoundingScope: "order", Limits: rules.Limits{MaxCombinations: 100, MaxMultiplier: 10}},
		Selection:  rules.Selection{Special: []int{7, 19, 31, 43}}, Draw: rules.Draw{Regular: []int{1, 2, 3, 4, 5, 6}, Special: []int{7}}, Multiplier: 2,
	}
	out, err := rules.Simulate(in)
	if err != nil {
		log.Fatal(err)
	}
	// This valid request omits nonapplicable model slots and optional limits.
	// Verify Go accepts it before exposing it as a schema regression example.
	sparse := json.RawMessage(`{"definition":{"schema_version":1,"model":{"model":"X_PLUS_Y","regular_pool":{"min":1,"max":49},"special_pool":{"min":1,"max":49},"regular_count":6,"special_count":1},"selection":{"mode":"numbers","special_count":1},"unit_points":"1","prize_tiers":[{"code":"SPECIAL_MATCH","condition":{"op":"equals","field":"special_match","value":1},"odds":"35","exclusive":true}],"rounding":"half_up","rounding_scope":"order","limits":{"max_combinations":100,"max_multiplier":"10"}},"selection":{"special":[7,19,31,43]},"draw":{"regular":[1,2,3,4,5,6],"special":[7]},"multiplier":"2"}`)
	var sparseInput rules.SimulationInput
	if err := json.Unmarshal(sparse, &sparseInput); err != nil {
		log.Fatal(err)
	}
	if _, err := rules.Simulate(sparseInput); err != nil {
		log.Fatal(err)
	}
	// Unlike sparse rule input, financial configuration replacements require
	// every field, including explicit nulls. Pointer types alone do not imply
	// that the HTTP decoder accepts omission.
	agentInput := agency.CreateInput{PolicyVersion: 1, MemberID: id, Config: agency.NodeConfig{Ratio: "0", Status: "active"}, Reason: "contract example"}
	agentJSON, err := json.Marshal(agentInput)
	if err != nil {
		log.Fatal(err)
	}
	var decodedAgent agency.CreateInput
	if err := json.Unmarshal(agentJSON, &decodedAgent); err != nil {
		log.Fatal(err)
	}
	var agentFields map[string]json.RawMessage
	if err := json.Unmarshal(agentJSON, &agentFields); err != nil {
		log.Fatal(err)
	}
	delete(agentFields, "parent_id")
	missingParent, err := json.Marshal(agentFields)
	if err != nil {
		log.Fatal(err)
	}
	if json.Unmarshal(missingParent, &decodedAgent) == nil {
		log.Fatal("agent decoder accepted omitted parent_id")
	}
	var gameWithdrawal withdrawal.GameConfig
	if json.Unmarshal([]byte(`{}`), &gameWithdrawal) == nil {
		log.Fatal("withdrawal decoder accepted omitted override")
	}
	if err := json.Unmarshal([]byte(`{"turnover_multiple":null}`), &gameWithdrawal); err != nil {
		log.Fatal(err)
	}
	before := points.Balance{}
	before[0][0] = 100
	delta := points.Balance{}
	delta[0][0] = -8
	after := points.Balance{}
	after[0][0] = 92
	entry := points.Entry{ID: id, BrandID: id, AccountID: id, MemberID: id, EntryType: "bet", ReferenceType: "order", ReferenceID: id, OperationKey: "contract-example", Reason: "contract example", ActorType: "user", ActorID: id, RequestID: "contract-example", Version: 2, Before: before, Delta: delta, After: after, Allocation: []points.Allocation{{Source: "recharge", State: "available", Points: 8}}, CreatedAt: now}
	reconciliationJob := reconciliation.Job{
		ID: id, BrandID: id, State: "running", Version: 1,
		TargetCount: "2", CheckedCount: "1", ConsistentCount: "1", RepairableCount: "0", CorruptCount: "0", FailedCount: "0", PendingCount: "1",
		CreatedBy: id, Reason: "monthly wallet observation", CreatedAt: now, StartedAt: &startedAt,
		CanRetry: false, CreationAuditLogID: id,
	}
	preview := points.RepairPreview{
		AccountID: id, MemberID: id, Version: 0, LedgerVersion: 0,
		Actual: map[string]map[string]points.Amount{
			"recharge": {"available": 11},
			"gift":     {"manual_frozen": 0},
		},
		Expected: points.Balance{}, Consistent: true, Issues: []string{}, Token: "contract-preview",
	}
	reconciliationCheckedTarget := reconciliation.Target{
		ID: "22222222-2222-4222-8222-222222222222", BrandID: id, JobID: id, AccountID: id, MemberID: id,
		State: "checked", Outcome: &consistent, Preview: &preview, AttemptCount: 1, CheckedAt: &checkedAt, AuditLogID: &reconciliationAuditID,
	}
	reconciliationPendingTarget := reconciliation.Target{
		ID: "33333333-3333-4333-8333-333333333333", BrandID: id, JobID: id, AccountID: id, MemberID: id,
		State: "pending",
	}
	reconciliationTargets := reconciliation.TargetPage{
		BrandID: id, JobID: id, Items: []reconciliation.Target{reconciliationCheckedTarget, reconciliationPendingTarget},
		TotalCount: "2", Limit: 20, Offset: 0, Outcome: &consistentFilter,
	}
	presentationConfig := brandskin.Config{}
	presentationEffective, err := brandskin.Resolve("Example", presentationConfig)
	if err != nil {
		log.Fatal(err)
	}
	presentation := brandskin.Record{BrandID: id, Version: 2, Status: "active", BaseName: "Example", Config: presentationConfig, Effective: presentationEffective, UpdatedAt: now, AuditLogID: id}
	publicTheme, err := json.Marshal(presentationEffective)
	if err != nil {
		log.Fatal(err)
	}
	values := map[string]any{
		"AdminBrandDomain":                 branddomains.Domain{ID: id, Domain: "brand.example.test", Enabled: true, Primary: true},
		"AdminBrandDomainRecord":           branddomains.Record{BrandID: id, Version: 2, Status: "active", Domains: []branddomains.Domain{{ID: id, Domain: "brand.example.test", Enabled: true, Primary: true}}, UpdatedAt: now},
		"AdminBrandDomainReceipt":          branddomains.Record{BrandID: id, Version: 2, Status: "active", Domains: []branddomains.Domain{{ID: id, Domain: "brand.example.test", Enabled: true, Primary: true}}, UpdatedAt: now, AuditLogID: id},
		"AdminBrandDomainCreateRequest":    branddomains.Input{Version: 1, Domain: "brand.example.test", Enabled: true, Primary: true, Reason: "contract example"},
		"AdminBrandDomainUpdateRequest":    branddomains.Input{Version: 1, Enabled: true, Primary: true, Reason: "contract example"},
		"AdminBrandDomainRevision":         branddomains.Revision{ID: id, BrandID: id, Version: 2, ChangedBy: id, Reason: "contract example", AuditLogID: id, CreatedAt: now, BeforeDomains: []branddomains.Domain{}, Domains: []branddomains.Domain{{ID: id, Domain: "brand.example.test", Enabled: true, Primary: true}}},
		"IdentityContextBrand":             tenant.Brand{ID: id, Code: "example", Name: presentationEffective.DisplayName, Status: "active", DefaultLocale: presentationEffective.DefaultLocale, Timezone: "Asia/Manila", Theme: publicTheme, ConfigVersion: 2},
		"AdminBrandPresentationConfig":     presentationConfig,
		"AdminBrandPresentationEffective":  presentationEffective,
		"AdminBrandPresentationRecord":     presentation,
		"AdminBrandPresentationReceipt":    presentation,
		"AdminBrandPresentationPutRequest": brandskin.Input{Version: 1, Config: presentationConfig, Reason: "contract example"},
		"AdminBrandPresentationRevision":   brandskin.Revision{ID: id, BrandID: id, Version: 2, Config: presentationConfig, Effective: presentationEffective, ChangedBy: id, Reason: "contract example", AuditLogID: id, CreatedAt: now},
		"AdminBrandOperation":              brandops.Record{BrandID: id, Version: 2, Name: "Example", Status: "paused", UpdatedAt: now, AuditLogID: id},
		"AdminBrandCreationInput":          brandregistry.Input{Code: "example_brand", Name: "Example", DefaultLocale: "en", Timezone: "UTC", Reason: "explicit creation"},
		"ComplianceConfig":                 complianceConfig,
		"ComplianceGateRecord":             gateRecord,
		"ComplianceGatesPage":              compliance.GatePage{BrandID: id, Items: []compliance.GateRecord{gateRecord}, Limit: 20, Offset: 0, TotalCount: "1"},
		"CompliancePolicyInput":            compliance.Input{Version: 1, Config: complianceConfig, Reason: "configure future check"},
		"CompliancePolicy":                 compliance.Policy{BrandID: id, Version: 2, Config: complianceConfig, UpdatedAt: now, AuditLogID: id},
		"ComplianceCheckInput":             compliance.CheckInput{Version: 2, Operation: "betting", Reason: "explicit stub check"},
		"ComplianceDecision":               complianceDecision,
		"ComplianceHistoryPage":            compliance.HistoryPage{BrandID: id, Items: []compliance.Revision{{ID: id, BrandID: id, Version: 1, Config: compliance.DefaultConfig(), Reason: "Initial disabled", CreatedAt: now}}, Limit: 20, Offset: 0, TotalCount: "1"},
		"ComplianceDecisionPage":           compliance.DecisionPage{BrandID: id, Items: []compliance.Decision{complianceDecision}, Limit: 20, Offset: 0, TotalCount: "1"},
		"AdminBrandCreationReceipt":        brandregistry.Receipt{ID: id, Code: "example_brand", Name: "Example", DefaultLocale: "en", Timezone: "UTC", Status: "paused", Version: 1, CreatedAt: now, AuditLogID: id},
		"AdminBrandOperationRevision":      brandops.Revision{ID: id, BrandID: id, Version: 2, PreviousStatus: "active", Status: "paused", ChangedBy: id, Reason: "contract example", AuditLogID: id, CreatedAt: now},
		"FinanceAgentCreateInput":          agentInput, "FinanceWithdrawalGameConfig": gameWithdrawal,
		"IdentityUser":   identity.User{ID: id, Status: "normal"},
		"IdentityMember": identity.Member{ID: id, BrandID: id, Status: "normal", JoinedAt: now},
		"FinanceBalance": before, "FinanceDeltaBalance": delta, "FinanceEntry": entry,
		"FinanceReconciliationJob":        reconciliationJob,
		"FinanceReconciliationJobPage":    reconciliation.JobPage{BrandID: id, Items: []reconciliation.Job{reconciliationJob}, TotalCount: "1", Limit: 20, Offset: 0},
		"FinanceReconciliationTarget":     reconciliationCheckedTarget,
		"FinanceReconciliationTargetPage": reconciliationTargets,
		"FinanceWallet":                   points.Wallet{AccountID: id, BrandID: id, MemberID: id, Version: 2, DisplayPoints: 92, AvailablePoints: 92, RechargePoints: 92, BySource: after},
		"FinanceReportBalances":           reporting.Balances{AccountCount: "2", AvailablePoints: "18000000000000000000", FrozenPoints: "0", WithdrawalPoints: "0", TotalPoints: "18000000000000000000"},
		"FinanceLedgerTotals":             reporting.LedgerTotals{EntryCount: "2", NetPoints: "-18000000000000000000", RechargePoints: "0", PrizeCreditPoints: "0", PrizeReversalPoints: "18000000000000000000", RefundPoints: "0"},
		"CommissionReport":                reporting.CommissionReport{BrandID: id, SnapshotAt: now, Timezone: "UTC", Query: reporting.CommissionQuery{From: now.Add(-24 * time.Hour), To: now, GroupBy: "day", Limit: 20}, Summary: reporting.CommissionTotals{EntryCount: "0", PaidEntryCount: "0", PaidPoints: "0", AdjustmentEntryCount: "0", AdjustmentCreditPoints: "0", AdjustmentDebitPoints: "0", CorrectionEntryCount: "0", CorrectionCreditPoints: "0", CorrectionDebitPoints: "0", NetPoints: "0"}, Items: []reporting.Group[reporting.CommissionTotals]{}, TotalGroups: "0"},
		"CommissionReportTotals":          reporting.CommissionTotals{EntryCount: "8", PaidEntryCount: "2", PaidPoints: "18000000000000000000", AdjustmentEntryCount: "1", AdjustmentCreditPoints: "0", AdjustmentDebitPoints: "2", CorrectionEntryCount: "5", CorrectionCreditPoints: "18000000000000000000", CorrectionDebitPoints: "27000000000000000000", NetPoints: "8999999999999999998"},
		"LotteryRuleDefinition":           in.Definition, "LotterySimulationInput": in, "LotterySimulationResult": out,
		"LotterySimulationInputSparse": sparse,
		"LotteryRuleSelection":         in.Selection, "LotteryRuleDraw": in.Draw,
		"LotteryRuleTier": in.Definition.PrizeTiers[0], "LotteryRuleCondition": in.Definition.PrizeTiers[0].Condition,
		"LotteryNotification":                 notification.Item{ID: id, BrandID: id, MemberID: id, EventType: "member.joined", TemplateKey: "member.joined", TemplateVersion: 1, Payload: notification.Payload{ResourceID: id}, CreatedAt: now},
		"LotteryNotificationSnapshot":         notification.Item{ID: id, BrandID: id, MemberID: id, EventType: "recharge.confirmed", TemplateKey: "recharge.confirmed", TemplateVersion: 2, Content: &noticeContent, Payload: notification.Payload{ResourceID: id, Points: &noticePoints}, CreatedAt: now},
		"LotteryNotificationTemplate":         notification.Template{BrandID: id, Key: "recharge.confirmed", Version: 2, Content: noticeContent, UpdatedAt: now, AuditLogID: &noticeAuditID},
		"LotteryNotificationTemplateRevision": notification.Revision{ID: id, BrandID: id, Key: "recharge.confirmed", Version: 2, Content: noticeContent, ChangedBy: &noticeAuditID, Reason: "contract example", AuditLogID: &noticeAuditID, CreatedAt: now},
	}
	for key, value := range workbenchExamples() {
		values[key] = value
	}
	for key, value := range withdrawalExamples() {
		values[key] = value
	}
	for key, value := range withdrawalReportExamples() {
		values[key] = value
	}
	for key, value := range rewardExamples() {
		values[key] = value
	}
	for key, value := range rewardReportExamples() {
		values[key] = value
	}
	stamp := time.Date(2026, 10, 8, 12, 34, 56, 123456789, time.UTC)
	credit, debit, net := "100", "100", "0"
	values["correction_policy"] = commission.CorrectionExecutionPolicy{BrandID: id, Version: 1, Enabled: false, UpdatedAt: stamp}
	values["correction_policy_updated"] = commission.CorrectionExecutionPolicy{BrandID: id, Version: 2, Enabled: true, AuditLogID: id, UpdatedAt: stamp}
	values["correction_plan"] = commission.CorrectionPlan{ID: id, BrandID: id, CycleID: id, PaymentID: id, RunID: id, State: "ready", PayoutMode: "mixed", Version: 3, EvidenceEpoch: "9223372036854775807", BeforePoints: "100", CalculatedPoints: "100", CreditPoints: &credit, DebitPoints: &debit, NetPoints: &net, TargetCount: "2", PlannedCount: "2", CreationAuditLogID: id, LastAuditLogID: id, CreatedAt: stamp, UpdatedAt: stamp}
	values["correction_plan_target"] = commission.CorrectionPlanTarget{ID: id, BrandID: id, PlanID: id, AgentID: id, MemberID: id, EarningID: func() *string { v := id; return &v }(), PointsBefore: 0, PointsAfter: 100, DeltaPoints: 100, CreationAuditLogID: id, CreatedAt: stamp}
	values["correction_execution"] = commission.CorrectionExecution{ID: id, BrandID: id, CycleID: id, PlanID: id, RunID: id, PayoutMode: "mixed", State: "awaiting_approval", Version: 1, PlanVersion: 3, EvidenceEpoch: "9223372036854775807", CreditPoints: "100", DebitPoints: "100", NetPoints: "0", AppliedCreditPoints: "0", AppliedDebitPoints: "0", TargetCount: "2", AppliedCount: "0", CreationAuditLogID: id, LastAuditLogID: id, CreatedAt: stamp, UpdatedAt: stamp}
	financialVersion := int64(1)
	ledger, targetAudit := id, id
	values["correction_execution_target"] = commission.CorrectionExecutionTarget{ID: id, BrandID: id, ExecutionID: id, PlanTargetID: id, AgentID: id, MemberID: id, PointsBefore: 100, PointsAfter: 0, DeltaPoints: -100, State: "applied", LedgerEntryID: &ledger, AuditLogID: &targetAudit, FinancialVersion: &financialVersion, CreatedAt: stamp, AppliedAt: &stamp}
	if err := json.NewEncoder(os.Stdout).Encode(values); err != nil {
		log.Fatal(err)
	}
}
