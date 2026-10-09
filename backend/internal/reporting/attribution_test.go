package reporting_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/attribution"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/reporting"
	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func validAttributionQuery() reporting.AttributionQuery {
	return reporting.AttributionQuery{
		From:       time.Date(2026, 10, 1, 0, 0, 0, 123, time.UTC),
		To:         time.Date(2026, 10, 2, 0, 0, 0, 456, time.UTC),
		GroupBy:    "day",
		Limit:      20,
		Offset:     0,
		AgentScope: "direct",
	}
}

func TestAttributionQueryValidationRequiresClosedBoundedWindowAndGroup(t *testing.T) {
	query := validAttributionQuery()
	if err := query.Validate(); err != nil {
		t.Fatalf("valid query rejected: %v", err)
	}
	for _, edit := range []struct {
		name string
		fn   func(*reporting.AttributionQuery)
	}{
		{"zero from", func(q *reporting.AttributionQuery) { q.From = time.Time{} }},
		{"zero to", func(q *reporting.AttributionQuery) { q.To = time.Time{} }},
		{"empty window", func(q *reporting.AttributionQuery) { q.To = q.From }},
		{"reversed window", func(q *reporting.AttributionQuery) { q.To = q.From.Add(-time.Nanosecond) }},
		{"over 93 days", func(q *reporting.AttributionQuery) { q.To = q.From.Add(93*24*time.Hour + time.Nanosecond) }},
		{"missing group", func(q *reporting.AttributionQuery) { q.GroupBy = "" }},
		{"unknown group", func(q *reporting.AttributionQuery) { q.GroupBy = "current_agent" }},
		{"zero limit", func(q *reporting.AttributionQuery) { q.Limit = 0 }},
		{"over limit", func(q *reporting.AttributionQuery) { q.Limit = 101 }},
		{"negative offset", func(q *reporting.AttributionQuery) { q.Offset = -1 }},
		{"over offset", func(q *reporting.AttributionQuery) { q.Offset = 1000001 }},
		{"unknown scope", func(q *reporting.AttributionQuery) { q.AgentScope = "all" }},
		{"downline without agent", func(q *reporting.AttributionQuery) { q.AgentScope = "downline" }},
		{"bad game UUID", func(q *reporting.AttributionQuery) { v := "not-a-uuid"; q.GameID = &v }},
		{"bad member UUID", func(q *reporting.AttributionQuery) { v := "member"; q.MemberID = &v }},
		{"bad agent UUID", func(q *reporting.AttributionQuery) { v := "agent"; q.AgentID = &v }},
		{"bad join method", func(q *reporting.AttributionQuery) { v := "current"; q.JoinMethod = &v }},
		{"legacy join method", func(q *reporting.AttributionQuery) { v := "legacy"; q.JoinMethod = &v }},
	} {
		t.Run(edit.name, func(t *testing.T) {
			q := query
			edit.fn(&q)
			if err := q.Validate(); !errors.Is(err, reporting.ErrInvalid) {
				t.Fatalf("invalid query accepted: %+v, err=%v", q, err)
			}
		})
	}
}

func TestAttributionQueryValidationAcceptsEveryGroupAndHistoricalFilter(t *testing.T) {
	for _, group := range []string{"day", "game", "member", "agent", "join_method"} {
		t.Run(group, func(t *testing.T) {
			q := validAttributionQuery()
			q.GroupBy = group
			q.GameID = testAttributionStringPointer("0199a000-0000-7000-8000-000000000010")
			q.MemberID = testAttributionStringPointer("0199a000-0000-7000-8000-000000000011")
			q.AgentID = testAttributionStringPointer("0199a000-0000-7000-8000-000000000012")
			q.JoinMethod = testAttributionStringPointer("referral_code")
			if err := q.Validate(); err != nil {
				t.Fatalf("valid historical filter rejected: %v", err)
			}
		})
	}
	for _, method := range []string{"domain", "operator", "agent_code", "referral_code"} {
		q := validAttributionQuery()
		q.JoinMethod = testAttributionStringPointer(method)
		if err := q.Validate(); err != nil {
			t.Errorf("join method %q rejected: %v", method, err)
		}
	}
	q := validAttributionQuery()
	q.GroupBy = "agent"
	q.AgentScope = "downline"
	q.AgentID = testAttributionStringPointer("0199a000-0000-7000-8000-000000000012")
	q.Limit, q.Offset = 100, 1000000
	q.To = q.From.Add(93 * 24 * time.Hour)
	if err := q.Validate(); err != nil {
		t.Fatalf("inclusive valid boundary rejected: %v", err)
	}
}

func TestAttributionTotalsJSONIsFlatAndUsesExactDecimalStrings(t *testing.T) {
	totals := testAttributionTotals()
	totals.OrderCount = "18446744073709551616"
	encoded, err := json.Marshal(totals)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if err = json.Unmarshal(encoded, &object); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"order_count", "stake_points", "placed_count", "won_count", "lost_count", "abnormal_count",
		"cancelled_count", "refund_points", "settled_stake_points", "unfinalized_stake_points",
		"abnormal_stake_points", "current_prize_points", "correction_open_count",
		"final_lost_stake_points",
	}
	if len(object) != len(want) {
		t.Fatalf("totals JSON has %d fields, want exactly %d: %s", len(object), len(want), encoded)
	}
	for _, key := range want {
		if _, ok := object[key]; !ok {
			t.Errorf("missing flattened total %q: %s", key, encoded)
		}
	}
	if string(object["order_count"]) != `"18446744073709551616"` {
		t.Fatalf("large count did not remain a decimal string: %s", object["order_count"])
	}
}

func TestAttributionEmptyReportIsReadOnlyAndValidatesHistoricalScope(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	brand := "0199a000-0000-7000-8000-000000000001"
	service := reporting.Service{DB: db}
	query := validAttributionQuery()
	query.From = time.Now().UTC().Add(-time.Hour)
	query.To = time.Now().UTC().Add(time.Hour)
	query.GameID, query.MemberID, query.AgentID, query.JoinMethod = nil, nil, nil, nil
	before := attributionReadOnlyCounts(t, db)
	report, err := service.Attribution(ctx, brand, query)
	if err != nil {
		t.Fatal(err)
	}
	if report.BrandID != brand || report.SnapshotAt.IsZero() || report.Timezone == "" ||
		report.Query != query || report.TotalGroups != "0" || len(report.Items) != 0 || report.Items == nil ||
		report.Summary.OrderCount != "0" || report.Summary.StakePoints != "0" ||
		report.Summary.FinalLostStakePoints != "0" {
		t.Fatalf("unexpected empty attribution report: %+v", report)
	}
	if after := attributionReadOnlyCounts(t, db); after != before {
		t.Fatalf("attribution report changed database business/audit state: before=%v after=%v", before, after)
	}
	query.AgentID = testAttributionStringPointer("ffffffff-ffff-4fff-8fff-ffffffffffff")
	if _, err = service.Attribution(ctx, brand, query); !errors.Is(err, reporting.ErrNotFound) {
		t.Fatalf("unknown historical agent scope should be not found: %v", err)
	}
	if after := attributionReadOnlyCounts(t, db); after != before {
		t.Fatalf("invalid scope read changed database business/audit state: before=%v after=%v", before, after)
	}
}

func attributionReadOnlyCounts(t *testing.T, db *pgxpool.Pool) [5]int64 {
	t.Helper()
	var counts [5]int64
	err := db.QueryRow(context.Background(), `SELECT
	 (SELECT count(*) FROM point_ledger_entries),
	 (SELECT count(*) FROM point_buckets),
	 (SELECT count(*) FROM commission_payments),
	 (SELECT count(*) FROM audit_logs),
	 (SELECT count(*) FROM bet_orders)`).Scan(&counts[0], &counts[1], &counts[2], &counts[3], &counts[4])
	if err != nil {
		t.Fatal(err)
	}
	return counts
}

func testAttributionStringPointer(value string) *string { return &value }

func testAttributionTotals() reporting.AttributionTotals {
	return reporting.AttributionTotals{
		BettingTotals: reporting.BettingTotals{
			OrderCount: "0", StakePoints: "0", PlacedCount: "0", WonCount: "0", LostCount: "0",
			AbnormalCount: "0", CancelledCount: "0", RefundPoints: "0", SettledStakePoints: "0",
			UnfinalizedStakePoints: "0", AbnormalStakePoints: "0", CurrentPrizePoints: "0", CorrectionOpenCount: "0",
		},
		FinalLostStakePoints: "0",
	}
}

const attributionIntegrationBrand = "0199a000-0000-7000-8000-000000000001"

func TestAttributionReportUsesSavedAgentChainAcrossGamesAndSettlementCorrection(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	users, err := identity.New(db)
	if err != nil {
		t.Fatal(err)
	}
	adminID, reviewerID := ids.New(), ids.New()
	for i, id := range []string{adminID, reviewerID} {
		if _, err = db.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'test-only')`, id, "attrib_report_admin_"+id[:8]+string(rune('a'+i))); err != nil {
			t.Fatal(err)
		}
	}
	actor := attributionReportActor(adminID, attributionIntegrationBrand)
	reviewer := attributionReportActor(reviewerID, attributionIntegrationBrand)
	metadata := func() points.Metadata { return points.Metadata{RequestID: ids.New()} }
	register := func(agentCode string) identity.Authentication {
		t.Helper()
		var result identity.Authentication
		attributionReportTx(t, db, func(tx pgx.Tx) error {
			username := "attrib_" + strings.ReplaceAll(ids.New(), "-", "")[20:]
			out, e := users.Register(ctx, tx, attributionIntegrationBrand, identity.RegisterInput{
				Username: username, Password: "test-attribution-password",
				Privacy: "dev-1", Terms: "dev-1", AgentCode: agentCode,
			}, identity.Metadata{Domain: "aurora.localhost"})
			if e != nil {
				return e
			}
			if out.Status != 201 {
				var diagnostic struct {
					Code      string `json:"code"`
					ErrorCode string `json:"error_code"`
				}
				_ = json.Unmarshal(out.Data, &diagnostic)
				code := diagnostic.Code
				if code == "" {
					code = diagnostic.ErrorCode
				}
				if code == "" {
					code = "unavailable"
				}
				return fmt.Errorf("attribution fixture registration failed: status=%d code=%s", out.Status, code)
			}
			return json.Unmarshal(out.Data, &result)
		})
		return result
	}
	adminMember := register("")
	agents := agency.Service{DB: db}
	codes := attribution.Service{DB: db}
	var policy agency.Policy
	var root agency.Node
	var rootCode attribution.Code
	attributionReportTx(t, db, func(tx pgx.Tx) error {
		var e error
		policy, e = agents.SavePolicy(ctx, tx, attributionIntegrationBrand, actor, agency.PolicyInput{
			Version: 1, Config: agency.PolicyConfig{Enabled: true, MaxDepth: 3, RatioCap: "0.1", Mode: "loss", Cycle: "weekly"},
			Reason: "attribution report integration policy",
		}, metadata())
		return e
	})
	attributionReportTx(t, db, func(tx pgx.Tx) error {
		var e error
		root, e = agents.Create(ctx, tx, attributionIntegrationBrand, actor, agency.CreateInput{
			PolicyVersion: policy.Version, MemberID: adminMember.Member.ID,
			Config: agency.NodeConfig{Ratio: "0.08", Status: "active", CanCreateChildren: true},
			Reason: "attribution report root agent",
		}, metadata())
		return e
	})
	attributionReportTx(t, db, func(tx pgx.Tx) error {
		var e error
		rootCode, e = codes.Create(ctx, tx, attributionIntegrationBrand, actor, attribution.CreateInput{
			Kind: "agent", OwnerMemberID: adminMember.Member.ID, AgentID: &root.ID, Reason: "attribution report root code",
		}, metadata())
		return e
	})
	childAgentMember := register(rootCode.Code)
	var child agency.Node
	parentID, parentVersion := root.ID, root.Version
	attributionReportTx(t, db, func(tx pgx.Tx) error {
		var e error
		child, e = agents.Create(ctx, tx, attributionIntegrationBrand, actor, agency.CreateInput{
			PolicyVersion: policy.Version, MemberID: childAgentMember.Member.ID,
			ParentID: &parentID, ParentVersion: &parentVersion,
			Config: agency.NodeConfig{Ratio: "0.04", Status: "active", CanCreateChildren: false},
			Reason: "attribution report child agent",
		}, metadata())
		return e
	})
	var childCode attribution.Code
	attributionReportTx(t, db, func(tx pgx.Tx) error {
		var e error
		childCode, e = codes.Create(ctx, tx, attributionIntegrationBrand, actor, attribution.CreateInput{
			Kind: "agent", OwnerMemberID: childAgentMember.Member.ID, AgentID: &child.ID, Reason: "attribution report child code",
		}, metadata())
		return e
	})
	rootPlayer := register(rootCode.Code)
	childPlayer := register(childCode.Code)

	games, inputs := createAttributionReportGames(t, db, actor, reviewer)
	betService := betting.Service{DB: db}
	fundAttributionReportMember(t, db, rootPlayer.Member.ID, 10)
	fundAttributionReportMember(t, db, childPlayer.Member.ID, 10)
	rootSession, err := users.Authenticate(ctx, attributionIntegrationBrand, rootPlayer.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	childSession, err := users.Authenticate(ctx, attributionIntegrationBrand, childPlayer.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	rootWin, err := attributionReportPlace(t, db, betService, rootSession, inputs[0], "root-win")
	if err != nil {
		t.Fatal(err)
	}
	rootCancelled, err := attributionReportPlace(t, db, betService, rootSession, inputs[0], "root-cancel")
	if err != nil {
		t.Fatal(err)
	}
	rootAbnormal, err := attributionReportPlace(t, db, betService, rootSession, inputs[0], "root-abnormal")
	if err != nil {
		t.Fatal(err)
	}
	childWin, err := attributionReportPlace(t, db, betService, childSession, inputs[1], "child-win-before-correction")
	if err != nil {
		t.Fatal(err)
	}
	attributionReportTx(t, db, func(tx pgx.Tx) error {
		_, e := betService.CancelAdmin(ctx, tx, attributionIntegrationBrand, actor, rootCancelled.ID, rootCancelled.Version, "report exclusion cancellation", metadata())
		return e
	})
	attributionReportTx(t, db, func(tx pgx.Tx) error {
		_, e := betService.MarkAbnormal(ctx, tx, attributionIntegrationBrand, actor, rootAbnormal.ID, rootAbnormal.Version, "report exclusion abnormal", metadata())
		return e
	})
	for _, game := range games {
		attributionReportSettle(t, db, betService, actor, game)
	}
	correctionContext, err := betService.CorrectionContext(ctx, attributionIntegrationBrand, games[1].PeriodID)
	if err != nil || correctionContext.DrawResultID == nil {
		t.Fatalf("child draw correction context: %+v %v", correctionContext, err)
	}
	policyVersion := correctionContext.PolicyVersion
	var correction betting.Correction
	attributionReportTx(t, db, func(tx pgx.Tx) error {
		var e error
		correction, e = betService.CreateCorrection(ctx, tx, attributionIntegrationBrand, actor, *correctionContext.DrawResultID, betting.CorrectionInput{
			Version: correctionContext.PeriodVersion, PolicyVersion: &policyVersion,
			Result: rules.Draw{Digits: []int{1, 2, 2}}, Reason: "verified attribution report result correction",
		}, metadata())
		return e
	})
	if correction.ID == "" {
		t.Fatal("correction was not created from the actual draw witness")
	}
	if _, err = betService.ProcessCorrections(ctx, 20); err != nil {
		t.Fatal(err)
	}
	if _, err = betService.ProcessSettlements(ctx, 20); err != nil {
		t.Fatal(err)
	}
	correctedOrder, err := betService.Order(ctx, attributionIntegrationBrand, childPlayer.Member.ID, childWin.ID)
	if err != nil || correctedOrder.Status != "lost" {
		t.Fatalf("real corrected child order not lost: %+v %v", correctedOrder, err)
	}
	var savedSnapshot []byte
	if err = db.QueryRow(ctx, `SELECT attribution_snapshot FROM bet_orders WHERE id=$1`, childWin.ID).Scan(&savedSnapshot); err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Member struct {
			AgentID    string `json:"agent_id"`
			JoinMethod string `json:"join_method"`
		} `json:"member_attribution"`
		AgentConfigs []struct {
			ID string `json:"id"`
		} `json:"agent_configs_at_bet"`
	}
	if err = json.Unmarshal(savedSnapshot, &saved); err != nil || saved.Member.AgentID != child.ID || saved.Member.JoinMethod != "agent_code" ||
		len(saved.AgentConfigs) != 2 || saved.AgentConfigs[0].ID != root.ID || saved.AgentConfigs[1].ID != child.ID {
		t.Fatalf("placement did not save the actual child path and source: %+v err=%v", saved, err)
	}

	// A real current agent edit after placement must not rewrite saved provenance.
	disabled := child.Config
	disabled.Status = "disabled"
	disabled.CanCreateChildren = false
	attributionReportTx(t, db, func(tx pgx.Tx) error {
		var e error
		child, e = agents.Update(ctx, tx, attributionIntegrationBrand, actor, child.ID, agency.UpdateInput{
			Version: child.Version, PolicyVersion: policy.Version, ParentVersion: child.ParentVersion, Config: disabled, Reason: "disable child after historical stakes",
		}, metadata())
		return e
	})
	var afterDisableSnapshot []byte
	if err = db.QueryRow(ctx, `SELECT attribution_snapshot FROM bet_orders WHERE id=$1`, childWin.ID).Scan(&afterDisableSnapshot); err != nil || string(afterDisableSnapshot) != string(savedSnapshot) {
		t.Fatalf("current child-node edit rewrote the immutable placed-order attribution: %v", err)
	}

	reportService := reporting.Service{DB: db}
	from := time.Now().UTC().Add(-time.Hour)
	to := time.Now().UTC().Add(time.Hour)
	query := reporting.AttributionQuery{From: from, To: to, GroupBy: "agent", Limit: 20, AgentScope: "downline", AgentID: &root.ID}
	beforeRead := attributionBusinessFingerprint(t, db)
	report, err := reportService.Attribution(ctx, attributionIntegrationBrand, query)
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.OrderCount != "4" || report.Summary.StakePoints != "4" || report.Summary.CancelledCount != "1" ||
		report.Summary.RefundPoints != "1" || report.Summary.AbnormalCount != "1" || report.Summary.AbnormalStakePoints != "1" ||
		report.Summary.WonCount != "1" || report.Summary.LostCount != "1" || report.Summary.SettledStakePoints != "2" ||
		report.Summary.FinalLostStakePoints != "1" || report.Summary.CurrentPrizePoints != "10" || report.Summary.CorrectionOpenCount != "0" ||
		report.TotalGroups != "2" || len(report.Items) != 2 {
		t.Fatalf("wrong snapshot-based multi-agent projection: %+v", report)
	}
	countsByAgent := map[string]string{report.Items[0].Key: report.Items[0].Totals.OrderCount, report.Items[1].Key: report.Items[1].Totals.OrderCount}
	if len(countsByAgent) != 2 || countsByAgent[root.ID] != "3" || countsByAgent[child.ID] != "1" {
		t.Fatalf("downline agent grouping duplicated or reassigned orders: %+v", report.Items)
	}
	query.AgentScope = "direct"
	direct, err := reportService.Attribution(ctx, attributionIntegrationBrand, query)
	if err != nil || direct.Summary.OrderCount != "3" || direct.TotalGroups != "1" {
		t.Fatalf("direct agent filter included child: %+v %v", direct, err)
	}
	query.GroupBy = "game"
	query.GameID = &games[0].GameID
	byGame, err := reportService.Attribution(ctx, attributionIntegrationBrand, query)
	if err != nil || byGame.Summary.OrderCount != "3" || byGame.TotalGroups != "1" || len(byGame.Items) != 1 || byGame.Items[0].Key != games[0].GameID {
		t.Fatalf("cross-game game filter failed: %+v %v", byGame, err)
	}
	query.GameID = nil
	query.AgentScope = "downline"
	query.GroupBy = "join_method"
	bySource, err := reportService.Attribution(ctx, attributionIntegrationBrand, query)
	if err != nil || bySource.Summary.OrderCount != "4" || bySource.TotalGroups != "1" || bySource.Items[0].Key != "agent_code" {
		t.Fatalf("saved join source was not preserved: %+v %v", bySource, err)
	}

	exactPlacedAt := rootWin.PlacedAt
	exactWindow := reporting.AttributionQuery{From: exactPlacedAt, To: exactPlacedAt.Add(time.Nanosecond), GroupBy: "member", Limit: 20, AgentScope: "direct", MemberID: &rootPlayer.Member.ID}
	exact, err := reportService.Attribution(ctx, attributionIntegrationBrand, exactWindow)
	if err != nil || exact.Summary.OrderCount != "1" {
		t.Fatalf("half-open sub-microsecond window lost exact placed order: %+v %v", exact, err)
	}
	exactWindow.From = exactPlacedAt.Add(time.Nanosecond)
	exactWindow.To = exactPlacedAt.Add(2 * time.Nanosecond)
	excluded, err := reportService.Attribution(ctx, attributionIntegrationBrand, exactWindow)
	if err != nil || excluded.Summary.OrderCount != "0" || len(excluded.Items) != 0 {
		t.Fatalf("nonoverlapping sub-microsecond order window included placement: %+v %v", excluded, err)
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	exported, err := reportService.AttributionExport(ctx, tx, attributionIntegrationBrand, reporting.AttributionQuery{
		From: from, To: to, GroupBy: "agent", Limit: 20, AgentScope: "downline", AgentID: &root.ID,
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = reporting.AttributionCSV(exported); err != nil {
		t.Fatalf("full historical export failed: %v", err)
	}
	if afterRead := attributionBusinessFingerprint(t, db); afterRead != beforeRead {
		t.Fatal("attribution reads/export changed ledger, wallets, payments, or audit history")
	}
	var commissionEntries int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE entry_type IN ('commission','commission_adjustment','commission_correction')`).Scan(&commissionEntries); err != nil || commissionEntries != 0 {
		t.Fatalf("operational report fixture unexpectedly posted commission: %d %v", commissionEntries, err)
	}
}

type attributionReportGame struct {
	GameID, PeriodID string
	PeriodNo         string
	Version          int64
	DrawAt           time.Time
}

func attributionReportActor(id, brand string) access.Account {
	permissions := []access.Permission{
		{Resource: "rule", Action: "write", Scope: access.ScopeBrand}, {Resource: "rule", Action: "validate", Scope: access.ScopeBrand},
		{Resource: "rule", Action: "submit", Scope: access.ScopeBrand}, {Resource: "rule", Action: "review", Scope: access.ScopeBrand},
		{Resource: "game", Action: "write", Scope: access.ScopeBrand}, {Resource: "agent_policy", Action: "write", Scope: access.ScopeBrand},
		{Resource: "agent", Action: "write", Scope: access.ScopeBrand}, {Resource: "join_code", Action: "write", Scope: access.ScopeBrand},
		{Resource: "bet", Action: "cancel", Scope: access.ScopeBrand}, {Resource: "bet", Action: "mark_abnormal", Scope: access.ScopeBrand},
		{Resource: "draw", Action: "manual_create", Scope: access.ScopeBrand}, {Resource: "draw", Action: "correct", Scope: access.ScopeBrand},
		{Resource: "settlement_policy", Action: "write", Scope: access.ScopeBrand}, {Resource: "settlement", Action: "run", Scope: access.ScopeBrand},
	}
	return access.Account{ID: id, Type: access.AccountAdmin, BrandIDs: []string{brand}, Roles: []access.Role{{BrandID: brand, Permissions: permissions}}}
}

func createAttributionReportGames(t *testing.T, db *pgxpool.Pool, actor, reviewer access.Account) ([]attributionReportGame, []betting.Input) {
	t.Helper()
	ctx := context.Background()
	store := rulebook.Store{DB: db}
	definition := rules.Definition{
		SchemaVersion: 1, Model: rules.Model{Type: "DIGITS_0_9", Length: 3, AllowRepeat: true, Ordered: true},
		Selection: rules.SelectionRule{Mode: "numbers"}, UnitPoints: 1,
		PrizeTiers: []rules.Tier{{Code: "EXACT", Condition: rules.Condition{Op: "equals", Field: "position_match", Value: intForAttributionReport(3)}, Odds: "10", Exclusive: true}},
		Rounding:   "half_up", RoundingScope: "order", Limits: rules.Limits{MaxCombinations: 100, MaxMultiplier: 1000},
	}
	games := make([]attributionReportGame, 2)
	inputs := make([]betting.Input, 2)
	for i := range games {
		gameCode := fmt.Sprintf("attrib_%d", i+1)
		var game rulebook.Game
		attributionReportTx(t, db, func(tx pgx.Tx) error {
			var e error
			game, e = store.CreateGame(ctx, tx, attributionIntegrationBrand, actor, gameCode, "Attribution Game "+gameCode, definition.Model, "UTC", "attribution report cross-game fixture", points.Metadata{RequestID: ids.New()})
			return e
		})
		var play rulebook.Play
		attributionReportTx(t, db, func(tx pgx.Tx) error {
			var e error
			play, e = store.CreatePlay(ctx, tx, attributionIntegrationBrand, actor, game.ID, "exact", "Exact", "attribution report fixture play", points.Metadata{RequestID: ids.New()})
			return e
		})
		var version rulebook.Version
		attributionReportTx(t, db, func(tx pgx.Tx) error {
			var e error
			version, e = store.CreateVersion(ctx, tx, attributionIntegrationBrand, actor, play.ID, definition, "immediate", "attribution report fixture rule", points.Metadata{RequestID: ids.New()})
			return e
		})
		attributionReportTx(t, db, func(tx pgx.Tx) error {
			var e error
			version, e = store.Validate(ctx, tx, attributionIntegrationBrand, actor, version.ID, version.Version, []rules.ValidationCase{{
				Name: "exact", Selection: rules.Selection{Digits: [][]int{{1}, {2}, {1}}}, Draw: rules.Draw{Digits: []int{1, 2, 1}}, Multiplier: 1,
				ExpectedBetPoints: amountForAttributionReport(1), ExpectedPrizePoints: amountForAttributionReport(10), ExpectedWon: boolForAttributionReport(true),
			}}, "validate attribution report rule", points.Metadata{RequestID: ids.New()})
			return e
		})
		attributionReportTx(t, db, func(tx pgx.Tx) error {
			var e error
			version, e = store.Submit(ctx, tx, attributionIntegrationBrand, actor, version.ID, version.Version, "submit attribution report rule", points.Metadata{RequestID: ids.New()})
			return e
		})
		attributionReportTx(t, db, func(tx pgx.Tx) error {
			var e error
			version, e = store.Review(ctx, tx, attributionIntegrationBrand, reviewer, version.ID, version.Version, true, true, "approve attribution report rule", points.Metadata{RequestID: ids.New()})
			return e
		})
		now := time.Now().UTC()
		periodNo := fmt.Sprintf("attrib-%d-%s", i+1, ids.New()[:8])
		var period rulebook.Period
		attributionReportTx(t, db, func(tx pgx.Tx) error {
			var e error
			period, e = store.OpenPeriod(ctx, tx, attributionIntegrationBrand, game.ID, periodNo, now.Add(-time.Second), now.Add(3*time.Second), now.Add(3100*time.Millisecond))
			return e
		})
		brandPolicy, e := (betting.Service{DB: db}).BrandPolicy(ctx, attributionIntegrationBrand)
		if e != nil {
			t.Fatal(e)
		}
		gamePolicy, e := (betting.Service{DB: db}).GamePolicy(ctx, attributionIntegrationBrand, game.ID)
		if e != nil {
			t.Fatal(e)
		}
		games[i] = attributionReportGame{GameID: game.ID, PeriodID: period.ID, PeriodNo: period.PeriodNo, DrawAt: period.DrawAt}
		inputs[i] = betting.Input{PeriodID: period.ID, PlayID: play.ID, RuleVersionID: version.ID,
			Selection: rules.Selection{Digits: [][]int{{1}, {2}, {1}}}, Multiplier: 1,
			PolicyVersions: &betting.PolicyVersions{Brand: brandPolicy.Version, Game: gamePolicy.Version}}
	}
	return games, inputs
}

func attributionReportTx(t *testing.T, db *pgxpool.Pool, fn func(pgx.Tx) error) {
	t.Helper()
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if err = fn(tx); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func attributionReportPlace(t *testing.T, db *pgxpool.Pool, service betting.Service, session identity.Session, input betting.Input, key string) (betting.Order, error) {
	t.Helper()
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	order, err := service.Place(context.Background(), tx, attributionIntegrationBrand, session, input, key+"-"+ids.New(), points.Metadata{RequestID: ids.New()})
	if err != nil {
		_ = tx.Rollback(context.Background())
		return order, err
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	return order, nil
}

func fundAttributionReportMember(t *testing.T, db *pgxpool.Pool, member string, amount points.Amount) {
	t.Helper()
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var delta points.Balance
	delta[0][0] = amount
	_, err = (points.Store{DB: db}).Post(context.Background(), tx, points.Change{
		BrandID: attributionIntegrationBrand, MemberID: member, EntryType: "adjustment", ReferenceType: "attribution_report_fixture",
		OperationKey: "attribution-report-fund-" + ids.New(), Reason: "fund actual attribution report bets",
		ActorType: "system", RequestID: ids.New(), Delta: delta,
		Allocation: []points.Allocation{{Source: "recharge", State: "available", Points: amount}},
	})
	if err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func attributionReportSettle(t *testing.T, db *pgxpool.Pool, service betting.Service, actor access.Account, game attributionReportGame) {
	t.Helper()
	ctx := context.Background()
	store := rulebook.Store{DB: db}
	deadline := time.Now().Add(8 * time.Second)
	for {
		if _, err := store.Tick(ctx); err != nil {
			t.Fatal(err)
		}
		var ready bool
		if err := db.QueryRow(ctx, `SELECT status='waiting_draw' FROM periods WHERE id=$1`, game.PeriodID).Scan(&ready); err != nil {
			t.Fatal(err)
		}
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("attribution report fixture period did not reach draw")
		}
		time.Sleep(20 * time.Millisecond)
	}
	var version int64
	if err := db.QueryRow(ctx, `SELECT version FROM periods WHERE id=$1`, game.PeriodID).Scan(&version); err != nil {
		t.Fatal(err)
	}
	var draw rulebook.DrawResult
	attributionReportTx(t, db, func(tx pgx.Tx) error {
		var e error
		draw, e = store.ManualDraw(ctx, tx, attributionIntegrationBrand, actor, game.PeriodID, version, game.PeriodNo, rules.Draw{Digits: []int{1, 2, 1}}, game.DrawAt, "verified actual attribution report draw", points.Metadata{RequestID: ids.New()})
		return e
	})
	policy, err := service.SettlementPolicy(ctx, attributionIntegrationBrand)
	if err != nil {
		t.Fatal(err)
	}
	automatic := "automatic"
	attributionReportTx(t, db, func(tx pgx.Tx) error {
		var e error
		policy, e = service.SaveSettlementPolicy(ctx, tx, attributionIntegrationBrand, actor, betting.SettlementPolicyInput{
			Version: policy.Version, Mode: &automatic, Reason: "automatic settlement for attribution report evidence",
		}, points.Metadata{RequestID: ids.New()})
		return e
	})
	settlementContext, err := service.PeriodSettlementContext(ctx, attributionIntegrationBrand, game.PeriodID)
	if err != nil || settlementContext.DrawResultID == nil || *settlementContext.DrawResultID != draw.ID {
		t.Fatalf("actual draw not available as settlement witness: %+v %v", settlementContext, err)
	}
	attributionReportTx(t, db, func(tx pgx.Tx) error {
		_, e := service.StartSettlement(ctx, tx, attributionIntegrationBrand, actor, game.PeriodID, betting.SettlementStartInput{
			Version: settlementContext.PeriodVersion, PolicyVersion: policy.Version, DrawResultID: draw.ID, Reason: "settle attribution report evidence",
		}, points.Metadata{RequestID: ids.New()})
		return e
	})
	if _, err = service.ProcessSettlements(ctx, 50); err != nil {
		t.Fatal(err)
	}
}

func attributionBusinessFingerprint(t *testing.T, db *pgxpool.Pool) string {
	t.Helper()
	var fingerprint string
	err := db.QueryRow(context.Background(), `SELECT md5(jsonb_build_object(
	 'accounts',coalesce((SELECT jsonb_agg(to_jsonb(x) ORDER BY x.id) FROM point_accounts x WHERE x.brand_id=$1),'[]'::jsonb),
	 'buckets',coalesce((SELECT jsonb_agg(to_jsonb(x) ORDER BY x.account_id,x.source,x.state) FROM point_buckets x WHERE x.brand_id=$1),'[]'::jsonb),
	 'ledger',coalesce((SELECT jsonb_agg(to_jsonb(x) ORDER BY x.created_at,x.id) FROM point_ledger_entries x WHERE x.brand_id=$1),'[]'::jsonb),
	 'payments',coalesce((SELECT jsonb_agg(to_jsonb(x) ORDER BY x.id) FROM commission_payments x WHERE x.brand_id=$1),'[]'::jsonb),
	 'audits',coalesce((SELECT jsonb_agg(to_jsonb(x) ORDER BY x.id) FROM audit_logs x WHERE x.brand_id=$1),'[]'::jsonb)
	)::text)`, attributionIntegrationBrand).Scan(&fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	return fingerprint
}

func intForAttributionReport(value int) *int                        { return &value }
func amountForAttributionReport(value points.Amount) *points.Amount { return &value }
func boolForAttributionReport(value bool) *bool                     { return &value }
