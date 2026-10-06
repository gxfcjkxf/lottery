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
	"github.com/gxfcjkxf/lottery/backend/internal/compliance"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/notification"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/reporting"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/gxfcjkxf/lottery/backend/internal/tenant"
	"github.com/gxfcjkxf/lottery/backend/internal/withdrawal"
)

func main() {
	const id = "11111111-1111-4111-8111-111111111111"
	now := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	complianceConfig := compliance.DefaultConfig()
	complianceConfig.IdentityEnabled = true
	complianceResult, complianceChecks, err := compliance.Evaluate(complianceConfig)
	if err != nil {
		log.Fatal(err)
	}
	complianceDecision := compliance.Decision{ID: id, BrandID: id, PolicyVersion: 2, Config: complianceConfig, Operation: "betting", Decision: complianceResult, Checks: complianceChecks, AdapterMode: "stub", CreatedBy: id, Reason: "explicit stub check", AuditLogID: id, CreatedAt: now}
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
		"FinanceWallet":         points.Wallet{AccountID: id, BrandID: id, MemberID: id, Version: 2, DisplayPoints: 92, AvailablePoints: 92, RechargePoints: 92, BySource: after},
		"FinanceReportBalances": reporting.Balances{AccountCount: "2", AvailablePoints: "18000000000000000000", FrozenPoints: "0", WithdrawalPoints: "0", TotalPoints: "18000000000000000000"},
		"FinanceLedgerTotals":   reporting.LedgerTotals{EntryCount: "2", NetPoints: "-18000000000000000000", RechargePoints: "0", PrizeCreditPoints: "0", PrizeReversalPoints: "18000000000000000000", RefundPoints: "0"},
		"LotteryRuleDefinition": in.Definition, "LotterySimulationInput": in, "LotterySimulationResult": out,
		"LotterySimulationInputSparse": sparse,
		"LotteryRuleSelection":         in.Selection, "LotteryRuleDraw": in.Draw,
		"LotteryRuleTier": in.Definition.PrizeTiers[0], "LotteryRuleCondition": in.Definition.PrizeTiers[0].Condition,
		"LotteryNotification": notification.Item{ID: id, BrandID: id, MemberID: id, EventType: "bet.placed", TemplateKey: "bet.placed", TemplateVersion: 1, Payload: notification.Payload{ResourceID: id}, CreatedAt: now},
	}
	if err := json.NewEncoder(os.Stdout).Encode(values); err != nil {
		log.Fatal(err)
	}
}
