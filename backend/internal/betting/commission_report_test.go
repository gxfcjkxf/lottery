package betting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/agency"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/reporting"

	"github.com/jackc/pgx/v5"
)

func commissionReportQuery(now time.Time, group string) reporting.CommissionQuery {
	return reporting.CommissionQuery{
		From: now.Add(-24 * time.Hour), To: now.Add(24 * time.Hour),
		GroupBy: group, Limit: 20,
	}
}

func commissionReportFingerprint(t *testing.T, f commissionBatchFixture) string {
	t.Helper()
	var fingerprint string
	err := f.betting.db.QueryRow(context.Background(), `SELECT md5(jsonb_build_object(
	 'accounts',coalesce((SELECT jsonb_agg(to_jsonb(x) ORDER BY x.id) FROM point_accounts x WHERE x.brand_id=$1),'[]'::jsonb),
	 'buckets',coalesce((SELECT jsonb_agg(to_jsonb(x) ORDER BY x.account_id,x.source,x.state) FROM point_buckets x WHERE x.brand_id=$1),'[]'::jsonb),
	 'ledger',coalesce((SELECT jsonb_agg(to_jsonb(x) ORDER BY x.created_at,x.id) FROM point_ledger_entries x WHERE x.brand_id=$1),'[]'::jsonb),
	 'audits',coalesce((SELECT jsonb_agg(to_jsonb(x) ORDER BY x.id) FROM audit_logs x WHERE x.brand_id=$1),'[]'::jsonb),
	 'outbox',coalesce((SELECT jsonb_agg(to_jsonb(x) ORDER BY x.id) FROM outbox_events x WHERE x.brand_id=$1),'[]'::jsonb)
	)::text)`, f.betting.brand).Scan(&fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	return fingerprint
}

func commissionReportFinancialFingerprint(t *testing.T, f commissionBatchFixture) string {
	t.Helper()
	var fingerprint string
	err := f.betting.db.QueryRow(context.Background(), `SELECT md5(jsonb_build_object(
	 'accounts',coalesce((SELECT jsonb_agg(to_jsonb(x) ORDER BY x.id) FROM point_accounts x WHERE x.brand_id=$1),'[]'::jsonb),
	 'buckets',coalesce((SELECT jsonb_agg(to_jsonb(x) ORDER BY x.account_id,x.source,x.state) FROM point_buckets x WHERE x.brand_id=$1),'[]'::jsonb),
	 'ledger',coalesce((SELECT jsonb_agg(to_jsonb(x) ORDER BY x.created_at,x.id) FROM point_ledger_entries x WHERE x.brand_id=$1),'[]'::jsonb)
	)::text)`, f.betting.brand).Scan(&fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	return fingerprint
}

func assertCommissionReportTotals(t *testing.T, got reporting.CommissionTotals, entryCount, paidCount, paidPoints, adjustmentCount, adjustmentCredits, adjustmentDebits, net string) {
	t.Helper()
	want := reporting.CommissionTotals{
		EntryCount: entryCount, PaidEntryCount: paidCount, PaidPoints: paidPoints,
		AdjustmentEntryCount: adjustmentCount, AdjustmentCreditPoints: adjustmentCredits,
		AdjustmentDebitPoints: adjustmentDebits, CorrectionEntryCount: "0", CorrectionCreditPoints: "0", CorrectionDebitPoints: "0", NetPoints: net,
	}
	if got != want {
		t.Fatalf("commission totals = %+v, want %+v", got, want)
	}
}

func TestCommissionReportUsesActualPostingLedgerAndPreservesHistoricalFacts(t *testing.T) {
	f := newCommissionBatchFixture(t)
	ctx := context.Background()
	reports := reporting.Service{DB: f.betting.db}
	query := commissionReportQuery(time.Now().UTC(), "day")

	// Calculate the cycle through the worker while payout remains disabled.
	// A forecast/earning exists now, but no actual commission posting does.
	waitCommissionBoundary(t, f.boundary)
	cycle := createCommissionCycle(t, f)
	eligibilitySettle(t, f.betting)
	advanceCommissionWorker(t, f, 30)
	cycle = readCommissionCycle(t, f, cycle.ID)
	if cycle.State != "ready" || cycle.TotalPoints != "1" || cycle.EarningCount != "1" {
		t.Fatalf("fixture did not create the expected calculated but unpaid earning: %+v", cycle)
	}
	beforeRead := commissionReportFingerprint(t, f)
	calculated, err := reports.Commission(ctx, f.betting.brand, query)
	if err != nil {
		t.Fatal(err)
	}
	assertCommissionReportTotals(t, calculated.Summary, "0", "0", "0", "0", "0", "0", "0")
	if calculated.TotalGroups != "0" || len(calculated.Items) != 0 {
		t.Fatalf("calculated but unpaid earning appeared in actual commission report: %+v", calculated)
	}
	if afterRead := commissionReportFingerprint(t, f); afterRead != beforeRead {
		t.Fatal("commission report read mutated accounts, ledger, audit, or outbox")
	}

	policy := commissionPaymentPolicy(t, f)
	if policy.Enabled {
		t.Fatalf("manual payout gate unexpectedly enabled: %+v", policy)
	}
	if err = updateCommissionPaymentPolicy(t, f, commissionPaymentActor(f), true, policy.Version); err != nil {
		t.Fatal(err)
	}
	if steps, processErr := f.service.ProcessPayments(ctx, 20); processErr != nil || steps == 0 {
		t.Fatalf("payment worker failed to register actual target: steps=%d err=%v", steps, processErr)
	}
	payments := commissionPayments(t, f)
	if len(payments.Items) != 1 || payments.Items[0].State != "awaiting_approval" {
		t.Fatalf("manual payment was not registered for approval: %+v", payments.Items)
	}
	paid := payManualCommissionForAdjustment(t, f, payments.Items[0])
	if paid.State != "paid" || paid.PaidPoints != "1" || paid.PaidCount != "1" {
		t.Fatalf("fixture payment did not post exactly one point: %+v", paid)
	}
	targets := commissionTargets(t, f, paid.ID)
	if len(targets.Items) != 1 || targets.Items[0].LedgerEntryID == nil || targets.Items[0].AdjustmentVersion == nil {
		t.Fatalf("paid target missing its actual ledger binding: %+v", targets)
	}
	target := targets.Items[0]

	actor := commissionAdjustmentActor(f)
	up, err := adjustCommission(t, f, f.betting.brand, target, actor, *target.AdjustmentVersion, 3)
	if err != nil || up.DeltaPoints != 2 || up.PointsAfter != 3 {
		t.Fatalf("actual upward target adjustment = %+v, err=%v", up, err)
	}
	down, err := adjustCommission(t, f, f.betting.brand, target, actor, up.Version, 0)
	if err != nil || down.DeltaPoints != -3 || down.PointsAfter != 0 {
		t.Fatalf("actual downward target adjustment = %+v, err=%v", down, err)
	}

	// Current agent status is mutable policy. It cannot rewrite beneficiaries
	// already saved on the payment target and its ledger entries.
	agents := agency.Service{DB: f.betting.db}
	agentPolicy, err := agents.Policy(ctx, f.betting.brand)
	if err != nil {
		t.Fatal(err)
	}
	if err = commissionBatchCallTx(t, f, func(tx pgx.Tx) error {
		_, updateErr := agents.Update(ctx, tx, f.betting.brand, f.actor, f.node.ID, agency.UpdateInput{
			Version: f.node.Version, PolicyVersion: agentPolicy.Version,
			Config: agency.NodeConfig{Ratio: "0.2", Status: "disabled", CanCreateChildren: false},
			Reason: "report keeps historical beneficiary after current agent is disabled",
		}, points.Metadata{RequestID: ids.New()})
		return updateErr
	}); err != nil {
		t.Fatal(err)
	}

	var paidAt, upAt, downAt time.Time
	if err := f.betting.db.QueryRow(ctx, `SELECT created_at FROM point_ledger_entries WHERE id=$1`, *target.LedgerEntryID).Scan(&paidAt); err != nil {
		t.Fatal(err)
	}
	if err := f.betting.db.QueryRow(ctx, `SELECT created_at FROM point_ledger_entries WHERE id=$1`, up.LedgerEntryID).Scan(&upAt); err != nil {
		t.Fatal(err)
	}
	if err := f.betting.db.QueryRow(ctx, `SELECT created_at FROM point_ledger_entries WHERE id=$1`, down.LedgerEntryID).Scan(&downAt); err != nil {
		t.Fatal(err)
	}
	query.AgentID, query.MemberID, query.CycleID = &target.AgentID, &target.MemberID, &cycle.ID

	// Exercise exact half-open ledger-time selection, including sub-microsecond
	// bounds that PostgreSQL must round upward to its timestamp precision.
	narrow := query
	narrow.From, narrow.To = paidAt, upAt
	paidOnly, err := reports.Commission(ctx, f.betting.brand, narrow)
	if err != nil {
		t.Fatal(err)
	}
	assertCommissionReportTotals(t, paidOnly.Summary, "1", "1", "1", "0", "0", "0", "1")
	narrow.From, narrow.To = upAt, downAt
	upOnly, err := reports.Commission(ctx, f.betting.brand, narrow)
	if err != nil {
		t.Fatal(err)
	}
	assertCommissionReportTotals(t, upOnly.Summary, "1", "0", "0", "1", "2", "0", "2")
	narrow.From, narrow.To = downAt, downAt.Add(100*time.Nanosecond)
	downIncluded, err := reports.Commission(ctx, f.betting.brand, narrow)
	if err != nil {
		t.Fatal(err)
	}
	assertCommissionReportTotals(t, downIncluded.Summary, "1", "0", "0", "1", "0", "3", "-3")
	narrow.From, narrow.To = downAt.Add(100*time.Nanosecond), downAt.Add(2*time.Microsecond)
	downExcluded, err := reports.Commission(ctx, f.betting.brand, narrow)
	if err != nil {
		t.Fatal(err)
	}
	assertCommissionReportTotals(t, downExcluded.Summary, "0", "0", "0", "0", "0", "0", "0")

	query.From, query.To = paidAt, downAt.Add(time.Microsecond)
	query.GroupBy = "day"
	beforeReports := commissionReportFingerprint(t, f)
	day, err := reports.Commission(ctx, f.betting.brand, query)
	if err != nil {
		t.Fatal(err)
	}
	assertCommissionReportTotals(t, day.Summary, "3", "1", "1", "2", "2", "3", "0")
	if day.TotalGroups != "1" || len(day.Items) != 1 || day.Items[0].Key != paidAt.In(mustCommissionLocation(t, day.Timezone)).Format("2006-01-02") {
		t.Fatalf("day grouping did not contain the actual postings: %+v", day)
	}
	assertCommissionReportTotals(t, day.Items[0].Totals, "3", "1", "1", "2", "2", "3", "0")

	for _, group := range []struct{ name, key string }{{"agent", f.node.ID}, {"cycle", cycle.ID}} {
		groupQuery := query
		groupQuery.GroupBy = group.name
		report, reportErr := reports.Commission(ctx, f.betting.brand, groupQuery)
		if reportErr != nil || report.TotalGroups != "1" || len(report.Items) != 1 || report.Items[0].Key != group.key {
			t.Fatalf("%s grouping lost actual ledger rows: report=%+v err=%v", group.name, report, reportErr)
		}
		assertCommissionReportTotals(t, report.Summary, "3", "1", "1", "2", "2", "3", "0")
	}
	pageQuery := query
	pageQuery.Limit, pageQuery.Offset = 1, 1
	emptyPage, err := reports.Commission(ctx, f.betting.brand, pageQuery)
	if err != nil || len(emptyPage.Items) != 0 || emptyPage.TotalGroups != "1" || emptyPage.Summary != day.Summary {
		t.Fatalf("empty offset page lost complete count or summary: report=%+v err=%v", emptyPage, err)
	}
	emptyQuery := query
	emptyQuery.From, emptyQuery.To = downAt.Add(time.Hour), downAt.Add(2*time.Hour)
	empty, err := reports.Commission(ctx, f.betting.brand, emptyQuery)
	if err != nil || len(empty.Items) != 0 || empty.TotalGroups != "0" || empty.Summary.EntryCount != "0" {
		t.Fatalf("valid empty posting window was reported as missing: report=%+v err=%v", empty, err)
	}
	foreign := ids.New()
	for _, filter := range []struct {
		name string
		set  func(*reporting.CommissionQuery)
	}{{"agent", func(q *reporting.CommissionQuery) { q.AgentID = &foreign }}, {"member", func(q *reporting.CommissionQuery) { q.MemberID = &foreign }}, {"cycle", func(q *reporting.CommissionQuery) { q.CycleID = &foreign }}} {
		bad := query
		filter.set(&bad)
		if _, err := reports.Commission(ctx, f.betting.brand, bad); !errors.Is(err, reporting.ErrNotFound) {
			t.Fatalf("unknown %s filter error=%v, want ErrNotFound", filter.name, err)
		}
	}
	for _, filter := range []struct {
		name string
		set  func(*reporting.CommissionQuery)
	}{{"agent", func(q *reporting.CommissionQuery) { q.AgentID = &target.AgentID }}, {"member", func(q *reporting.CommissionQuery) { q.MemberID = &target.MemberID }}, {"cycle", func(q *reporting.CommissionQuery) { q.CycleID = &cycle.ID }}} {
		crossBrand := query
		crossBrand.AgentID, crossBrand.MemberID, crossBrand.CycleID = nil, nil, nil
		filter.set(&crossBrand)
		if _, err := reports.Commission(ctx, storeTestOtherBrand, crossBrand); !errors.Is(err, reporting.ErrNotFound) {
			t.Fatalf("cross-brand %s filter error=%v, want ErrNotFound", filter.name, err)
		}
	}
	if afterReports := commissionReportFingerprint(t, f); afterReports != beforeReports {
		t.Fatal("commission report reads mutated accounts, ledger, audit, or outbox")
	}

	// Advancing the evidence epoch blocks a fully paid payment, but the report
	// continues to describe its historical posted point facts.
	financialBeforeBlock := commissionReportFinancialFingerprint(t, f)
	if _, err := f.betting.db.Exec(ctx, `UPDATE commission_cycles SET evidence_epoch=evidence_epoch+1 WHERE brand_id=$1 AND id=$2`, f.betting.brand, cycle.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.betting.db.Exec(ctx, `UPDATE commission_payments SET next_work_at=clock_timestamp() WHERE brand_id=$1 AND id=$2`, f.betting.brand, paid.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ProcessPayments(ctx, 20); err != nil {
		t.Fatal(err)
	}
	blocked := commissionPaymentRead(t, f, paid.ID)
	if blocked.State != "blocked" || blocked.PaidPoints != "1" || blocked.PaidCount != "1" {
		t.Fatalf("changed evidence did not block the stale paid payment: %+v", blocked)
	}
	var posted int
	if err := f.betting.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE brand_id=$1 AND entry_type='commission'`, f.betting.brand).Scan(&posted); err != nil || posted != 1 {
		t.Fatalf("blocking stale payment changed actual paid ledger count: count=%d err=%v", posted, err)
	}
	if financialAfterBlock := commissionReportFinancialFingerprint(t, f); financialAfterBlock != financialBeforeBlock {
		t.Fatal("blocking stale paid payment changed wallet accounts, buckets, or point ledger")
	}
	stillHistorical, err := reports.Commission(ctx, f.betting.brand, query)
	if err != nil {
		t.Fatal(err)
	}
	assertCommissionReportTotals(t, stillHistorical.Summary, "3", "1", "1", "2", "2", "3", "0")
}

func mustCommissionLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	location, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return location
}
