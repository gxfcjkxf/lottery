package database_test

import (
	"context"

	"encoding/json"
	"errors"
	"fmt"

	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var inventorySources = []string{
	"bet_order_exceptions", "bet_order_judgments", "bet_orders",
	"commission_adjustment_heads", "commission_adjustments", "commission_allocations",
	"commission_calculations", "commission_correction_balance_heads",
	"commission_correction_cycle_holds", "commission_correction_execution_steps",
	"commission_correction_execution_targets", "commission_correction_executions",
	"commission_correction_plan_steps", "commission_correction_plan_targets",
	"commission_correction_plans", "commission_cycle_steps", "commission_cycle_targets",
	"commission_cycles", "commission_earnings", "commission_payment_targets",
	"commission_payments", "commission_runs", "draw_correction_failures",
	"draw_correction_targets", "draw_corrections", "period_cancellation_targets",
	"period_cancellations", "point_accounts", "point_buckets", "point_ledger_entries",
	"recharge_orders", "reward_order_actions", "reward_orders",
	"settlement_calculations", "settlement_failures", "settlement_jobs",
	"settlement_targets", "withdrawal_operation_receipts",
	"withdrawal_order_transitions", "withdrawal_orders", "withdrawal_turnover_cycles",
}

func TestBrandBusinessInventorySourceLimit(t *testing.T) {
	p := isolated(t)
	ctx := context.Background()
	if err := database.Migrate(ctx, p); err != nil {
		t.Fatal(err)
	}
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SET LOCAL session_replication_role = replica`); err != nil {
		t.Fatalf("test database cannot disable triggers for isolated corruption fixture: %v", err)
	}
	brandID := ids.New()
	if _, err = tx.Exec(ctx, `INSERT INTO brands(id,code,name,status) VALUES($1,'inventory-limit','Inventory limit','active')`, brandID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO point_buckets(brand_id,account_id,source,state,points)
		SELECT $1,md5(n::text)::uuid,'recharge','available',0 FROM generate_series(1,100001) n`, brandID); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT brand_business_inventory($1)`, brandID).Scan(&raw)
	if err == nil {
		t.Fatal("snapshot succeeded above the 100000 source-row limit")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("source-limit error is not a PostgreSQL error: %v", err)
	}
	if pgErr == nil || pgErr.Code != "54000" {
		t.Fatalf("source-limit SQLSTATE = %v, want 54000 (error %v)", pgErr, err)
	}
}

func TestBrandBusinessInventoryEmptySnapshot(t *testing.T) {
	p := isolated(t)
	ctx := context.Background()
	if err := database.Migrate(ctx, p); err != nil {
		t.Fatal(err)
	}
	brandID := ids.New()
	if _, err := p.Exec(ctx, `INSERT INTO brands(id,code,name,status) VALUES($1,'inventory-test','Inventory test','active')`, brandID); err != nil {
		t.Fatal(err)
	}
	var missing []byte
	if err := p.QueryRow(ctx, `SELECT brand_business_inventory($1)`, ids.New()).Scan(&missing); err != nil {
		t.Fatal(err)
	}
	if missing != nil {
		t.Fatalf("missing brand returned non-NULL snapshot: %s", missing)
	}
	var raw []byte
	if err := p.QueryRow(ctx, `SELECT brand_business_inventory($1)`, brandID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	wantFields := []string{"brand_id", "snapshot_at", "schema_version", "source_row_count", "reference_count", "issue_count", "issues_truncated", "consistent", "fingerprint", "coverage", "issues"}
	if len(got) != len(wantFields) {
		t.Fatalf("snapshot has %d fields, want %d: %s", len(got), len(wantFields), raw)
	}
	for _, field := range wantFields {
		if _, ok := got[field]; !ok {
			t.Errorf("snapshot missing field %q", field)
		}
	}
	for _, field := range []string{"source_row_count", "reference_count", "issue_count"} {
		var value string
		if err := json.Unmarshal(got[field], &value); err != nil || value != "0" {
			t.Errorf("%s = %s, want string \"0\" (err %v)", field, got[field], err)
		}
	}
	var version int
	if err := json.Unmarshal(got["schema_version"], &version); err != nil || version != 1 {
		t.Errorf("schema_version = %s, want 1 (err %v)", got["schema_version"], err)
	}
	var consistent, truncated bool
	_ = json.Unmarshal(got["consistent"], &consistent)
	_ = json.Unmarshal(got["issues_truncated"], &truncated)
	if !consistent || truncated {
		t.Errorf("empty snapshot consistent=%v issues_truncated=%v", consistent, truncated)
	}
	var coverage []struct {
		SourceTable    string `json:"source_table"`
		SourceRowCount string `json:"source_row_count"`
		ReferenceCount string `json:"reference_count"`
		IssueCount     string `json:"issue_count"`
	}
	if err := json.Unmarshal(got["coverage"], &coverage); err != nil {
		t.Fatal(err)
	}
	if len(coverage) != len(inventorySources) {
		t.Fatalf("coverage has %d entries, want %d", len(coverage), len(inventorySources))
	}
	for i, item := range coverage {
		if item.SourceTable != inventorySources[i] || item.SourceRowCount != "0" || item.ReferenceCount != "0" || item.IssueCount != "0" {
			t.Errorf("coverage[%d] = %+v, expected sorted zero entry for %s", i, item, inventorySources[i])
		}
	}
	var issues []json.RawMessage
	if err := json.Unmarshal(got["issues"], &issues); err != nil || len(issues) != 0 {
		t.Errorf("issues = %s, err %v; want empty array", got["issues"], err)
	}
	var volatile string
	var securityDefiner bool
	var config []string
	if err := p.QueryRow(ctx, `SELECT provolatile::text, prosecdef, coalesce(proconfig, ARRAY[]::text[])
		FROM pg_proc WHERE oid='brand_business_inventory(uuid)'::regprocedure`).Scan(&volatile, &securityDefiner, &config); err != nil {
		t.Fatal(err)
	}
	if volatile != "s" || securityDefiner {
		t.Errorf("routine volatility=%q security_definer=%v, want STABLE INVOKER", volatile, securityDefiner)
	}
	var appSchema string
	if err := p.QueryRow(ctx, `SELECT current_schema()`).Scan(&appSchema); err != nil {
		t.Fatal(err)
	}
	if len(config) != 1 || config[0] != "search_path=pg_catalog, "+appSchema+", pg_temp" {
		t.Errorf("routine config=%v, want the pinned current application schema", config)
	}
	var definition string
	if err := p.QueryRow(ctx, `SELECT pg_get_functiondef('brand_business_inventory(uuid)'::regprocedure)`).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(definition, "to_jsonb(p)") || strings.Contains(definition, "FOR v_row") ||
		!strings.Contains(definition, "AS MATERIALIZED") || !strings.Contains(definition, "p.brand_id=$1") {
		t.Errorf("installed function is not using the expected set-based native FK query")
	}
	tx, err := p.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var readonlyRaw []byte
	if err := tx.QueryRow(ctx, `SELECT brand_business_inventory($1)`, brandID).Scan(&readonlyRaw); err != nil {
		t.Errorf("read-only transaction snapshot: %v", err)
	}
}

func TestBrandBusinessInventorySettlementZeroPrizePointer(t *testing.T) {
	p := isolated(t)
	ctx := context.Background()
	if err := database.Migrate(ctx, p); err != nil {
		t.Fatal(err)
	}
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SET LOCAL session_replication_role = replica`); err != nil {
		t.Fatalf("test database cannot disable triggers for isolated corruption fixture: %v", err)
	}
	brandID := ids.New()
	if _, err = tx.Exec(ctx, `INSERT INTO brands(id,code,name,status) VALUES($1,'inventory-settlement-zero','Inventory settlement zero','active')`, brandID); err != nil {
		t.Fatal(err)
	}
	zeroJob, positiveJob := ids.New(), ids.New()
	zeroOrder, positiveOrder := ids.New(), ids.New()
	zeroPeriod, positivePeriod := ids.New(), ids.New()
	zeroCalc, positiveCalc := ids.New(), ids.New()
	for _, fixture := range []struct {
		job, order, period, calculation string
		prize                           int
	}{{zeroJob, zeroOrder, zeroPeriod, zeroCalc, 0}, {positiveJob, positiveOrder, positivePeriod, positiveCalc, 7}} {
		if _, err = tx.Exec(ctx, `INSERT INTO settlement_calculations
			(id,brand_id,job_id,period_id,order_id,order_version,definition_hash,draw_hash,won,prize_points,calculation)
			VALUES($1,$2,$3,$4,$5,1,'definition','draw',true,$6,'{}'::jsonb)`, fixture.calculation, brandID, fixture.job, fixture.period, fixture.order, fixture.prize); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO settlement_targets(brand_id,job_id,period_id,order_id,state,calculation_id)
			VALUES($1,$2,$3,$4,'paid',$5)`, brandID, fixture.job, fixture.period, fixture.order, fixture.calculation); err != nil {
			t.Fatal(err)
		}
	}
	var raw []byte
	if err = tx.QueryRow(ctx, `SELECT brand_business_inventory($1)`, brandID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Issues []struct {
			SourceID  string `json:"source_id"`
			Code      string `json:"code"`
			Reference string `json:"reference_key"`
		} `json:"issues"`
	}
	if err = json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	zeroID := zeroJob + "/" + zeroOrder
	positiveID := positiveJob + "/" + positiveOrder
	zeroRequired, positiveRequired := false, false
	for _, issue := range got.Issues {
		if issue.Reference != "payout_entry_id" || issue.Code != "MISSING_REQUIRED_LEDGER_REFERENCE" {
			continue
		}
		zeroRequired = zeroRequired || issue.SourceID == zeroID
		positiveRequired = positiveRequired || issue.SourceID == positiveID
	}
	if zeroRequired || !positiveRequired {
		t.Fatalf("paid settlement required-payout findings: zero=%v positive=%v", zeroRequired, positiveRequired)
	}
}

func TestBrandBusinessInventoryOrphansAndIssueTail(t *testing.T) {
	p := isolated(t)
	ctx := context.Background()
	if err := database.Migrate(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `ALTER TABLE point_buckets DROP CONSTRAINT point_buckets_brand_id_account_id_fkey`); err != nil {
		t.Fatal(err)
	}
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SET LOCAL session_replication_role = replica`); err != nil {
		t.Fatalf("test database cannot disable triggers for isolated corruption fixture: %v", err)
	}
	brandA, brandB := ids.New(), ids.New()
	for i, brand := range []string{brandA, brandB} {
		if _, err = tx.Exec(ctx, `INSERT INTO brands(id,code,name,status) VALUES($1,$2,$2,'active')`, brand, fmt.Sprintf("inventory-corrupt-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	// This account exists only under brand B. A bucket owned by brand A that
	// points to its ID must be treated as missing, even when a parent ID exists.
	crossBrandAccount := ids.New()
	if _, err = tx.Exec(ctx, `INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, crossBrandAccount, brandB, ids.New()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 105; i++ {
		if _, err = tx.Exec(ctx, `INSERT INTO point_buckets(brand_id,account_id,source,state,points) VALUES($1,$2,'recharge','available',0)`, brandA, ids.New()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO point_buckets(brand_id,account_id,source,state,points) VALUES($1,$2,'recharge','available',0)`, brandA, crossBrandAccount); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	if err = tx.QueryRow(ctx, `SELECT brand_business_inventory($1)`, brandA).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var got struct {
		SourceRowCount string `json:"source_row_count"`
		ReferenceCount string `json:"reference_count"`
		IssueCount     string `json:"issue_count"`
		Truncated      bool   `json:"issues_truncated"`
		Consistent     bool   `json:"consistent"`
		Fingerprint    string `json:"fingerprint"`
		Coverage       []struct {
			SourceTable string `json:"source_table"`
			Rows        string `json:"source_row_count"`
			References  string `json:"reference_count"`
			Issues      string `json:"issue_count"`
		} `json:"coverage"`
		Issues []struct {
			SourceTable string `json:"source_table"`
			SourceID    string `json:"source_id"`
			Code        string `json:"code"`
			Reference   string `json:"reference_key"`
			Parent      string `json:"parent_table"`
		} `json:"issues"`
	}
	if err = json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.SourceRowCount != "106" || got.ReferenceCount != "106" || got.IssueCount != "106" {
		t.Fatalf("counts source=%s refs=%s issues=%s, want 106/106/106", got.SourceRowCount, got.ReferenceCount, got.IssueCount)
	}
	if len(got.Coverage) != len(inventorySources) {
		t.Fatalf("coverage has %d entries, want %d", len(got.Coverage), len(inventorySources))
	}
	var coverageRows, coverageRefs, coverageIssues int64
	for i, item := range got.Coverage {
		if item.SourceTable != inventorySources[i] {
			t.Fatalf("coverage[%d] source=%q, want %q", i, item.SourceTable, inventorySources[i])
		}
		rows, e1 := strconv.ParseInt(item.Rows, 10, 64)
		refs, e2 := strconv.ParseInt(item.References, 10, 64)
		issues, e3 := strconv.ParseInt(item.Issues, 10, 64)
		if e1 != nil || e2 != nil || e3 != nil || issues > refs {
			t.Fatalf("coverage[%s] has invalid counts: %+v", item.SourceTable, item)
		}
		coverageRows += rows
		coverageRefs += refs
		coverageIssues += issues
	}
	if coverageRows != 106 || coverageRefs != 106 || coverageIssues != 106 {
		t.Fatalf("coverage totals=%d/%d/%d, want 106/106/106", coverageRows, coverageRefs, coverageIssues)
	}
	if !got.Truncated || got.Consistent || len(got.Issues) != 100 {
		t.Fatalf("truncated=%v consistent=%v returned issues=%d; want true/false/100", got.Truncated, got.Consistent, len(got.Issues))
	}
	if len(got.Fingerprint) != 64 {
		t.Fatalf("fingerprint length %d, want SHA-256 hex", len(got.Fingerprint))
	}
	if !sort.SliceIsSorted(got.Issues, func(i, j int) bool {
		a, b := got.Issues[i], got.Issues[j]
		if a.SourceTable != b.SourceTable {
			return a.SourceTable < b.SourceTable
		}
		if a.SourceID != b.SourceID {
			return a.SourceID < b.SourceID
		}
		if a.Reference != b.Reference {
			return a.Reference < b.Reference
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.Parent < b.Parent
	}) {
		t.Fatal("issues are not in Go SDK lexicographic field order")
	}
	for _, issue := range got.Issues {
		if issue.SourceTable != "point_buckets" || issue.Code != "MISSING_PARENT_REFERENCE" || issue.Reference != "point_buckets_brand_id_account_id_fkey" || issue.Parent != "point_accounts" {
			t.Fatalf("unexpected issue projection: %+v", issue)
		}
		if issue.SourceID == "" || len(issue.SourceID) > 500 || !strings.HasSuffix(issue.SourceID, "/recharge/available") {
			t.Fatalf("composite source_id was not stable PK components: %q", issue.SourceID)
		}
		for _, ch := range issue.SourceID {
			if !(ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || strings.ContainsRune("_.:/-", ch)) {
				t.Fatalf("source_id contains a disallowed character: %q", issue.SourceID)
			}
		}
	}
	var secondFingerprint string
	if err = tx.QueryRow(ctx, `SELECT brand_business_inventory($1)->>'fingerprint'`, brandA).Scan(&secondFingerprint); err != nil {
		t.Fatal(err)
	}
	if secondFingerprint != got.Fingerprint {
		t.Fatalf("fingerprint changed within the same data snapshot: %s != %s", secondFingerprint, got.Fingerprint)
	}
}
