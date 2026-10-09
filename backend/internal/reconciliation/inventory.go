package reconciliation

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var ErrInventoryTooLarge = errors.New("brand inventory source cap exceeded")
var ErrInventoryInvalid = errors.New("invalid brand inventory observation")

// Fixed v1 coverage is separate from a captured wallet job and cannot extend
// that job's historical evidence.
var inventorySources = []string{
	"bet_order_exceptions",
	"bet_order_judgments",
	"bet_orders",
	"commission_adjustment_heads",
	"commission_adjustments",
	"commission_allocations",
	"commission_calculations",
	"commission_correction_balance_heads",
	"commission_correction_cycle_holds",
	"commission_correction_execution_steps",
	"commission_correction_execution_targets",
	"commission_correction_executions",
	"commission_correction_plan_steps",
	"commission_correction_plan_targets",
	"commission_correction_plans",
	"commission_cycle_steps",
	"commission_cycle_targets",
	"commission_cycles",
	"commission_earnings",
	"commission_payment_targets",
	"commission_payments",
	"commission_runs",
	"draw_correction_failures",
	"draw_correction_targets",
	"draw_corrections",
	"period_cancellation_targets",
	"period_cancellations",
	"point_accounts",
	"point_buckets",
	"point_ledger_entries",
	"recharge_orders",
	"reward_order_actions",
	"reward_orders",
	"settlement_calculations",
	"settlement_failures",
	"settlement_jobs",
	"settlement_targets",
	"withdrawal_operation_receipts",
	"withdrawal_order_transitions",
	"withdrawal_orders",
	"withdrawal_turnover_cycles",
}

func InventorySources() []string { return append([]string(nil), inventorySources...) }

type InventoryIssue struct {
	SourceTable  string `json:"source_table"`
	SourceID     string `json:"source_id"`
	Code         string `json:"code"`
	ReferenceKey string `json:"reference_key"`
	ParentTable  string `json:"parent_table"`
}
type InventoryCoverage struct {
	SourceTable    string `json:"source_table"`
	SourceRowCount string `json:"source_row_count"`
	ReferenceCount string `json:"reference_count"`
	IssueCount     string `json:"issue_count"`
}
type InventorySnapshot struct {
	BrandID         string              `json:"brand_id"`
	SnapshotAt      time.Time           `json:"snapshot_at"`
	SchemaVersion   int                 `json:"schema_version"`
	SourceRowCount  string              `json:"source_row_count"`
	ReferenceCount  string              `json:"reference_count"`
	IssueCount      string              `json:"issue_count"`
	IssuesTruncated bool                `json:"issues_truncated"`
	Consistent      bool                `json:"consistent"`
	Fingerprint     string              `json:"fingerprint"`
	Coverage        []InventoryCoverage `json:"coverage"`
	Issues          []InventoryIssue    `json:"issues"`
}

// InventoryTx uses one read-only statement snapshot. Authorization and audit
// belong to the primary HTTP transaction, not this DTO.
func (s Service) InventoryTx(ctx context.Context, tx pgx.Tx, brand string) (InventorySnapshot, error) {
	var out InventorySnapshot
	if tx == nil || !uuid.MatchString(brand) {
		return out, ErrInvalid
	}
	var raw []byte
	err := tx.QueryRow(ctx, "SELECT brand_business_inventory($1::uuid)", brand).Scan(&raw)
	var pgerr *pgconn.PgError
	if errors.As(err, &pgerr) && pgerr.Code == "54000" {
		return out, ErrInventoryTooLarge
	}
	if err != nil {
		return out, err
	}
	if len(raw) == 0 {
		return out, ErrNotFound
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 11 {
		return out, ErrInventoryInvalid
	}
	for _, key := range []string{"brand_id", "snapshot_at", "schema_version", "source_row_count", "reference_count", "issue_count", "issues_truncated", "consistent", "fingerprint", "coverage", "issues"} {
		value, exists := fields[key]
		if !exists || string(value) == "null" {
			return out, ErrInventoryInvalid
		}
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&out) != nil || !validInventory(out, brand) {
		return InventorySnapshot{}, ErrInventoryInvalid
	}
	var tail any
	if decoder.Decode(&tail) != io.EOF {
		return InventorySnapshot{}, ErrInventoryInvalid
	}
	out.SnapshotAt = out.SnapshotAt.UTC()
	return out, nil
}

var inventoryCount = regexp.MustCompile("^(0|[1-9][0-9]*)$")
var inventoryDigest = regexp.MustCompile("^[0-9a-f]{64}$")
var inventoryKey = regexp.MustCompile("^[A-Za-z0-9_.:/-]{1,500}$")
var inventoryLabel = regexp.MustCompile("^[a-z][a-z0-9_]{0,62}$")

func inventoryNumber(s string) (*big.Int, bool) {
	if !inventoryCount.MatchString(s) || len(s) > 200 {
		return nil, false
	}
	n, ok := new(big.Int).SetString(s, 10)
	return n, ok
}
func validInventory(out InventorySnapshot, brand string) bool {
	if out.BrandID != brand || out.SchemaVersion != 1 || out.SnapshotAt.IsZero() || !inventoryDigest.MatchString(out.Fingerprint) ||
		len(out.Coverage) != len(inventorySources) || out.Issues == nil || len(out.Issues) > 100 {
		return false
	}
	source, ok := inventoryNumber(out.SourceRowCount)
	if !ok || source.Cmp(big.NewInt(100000)) > 0 {
		return false
	}
	refs, ok := inventoryNumber(out.ReferenceCount)
	if !ok {
		return false
	}
	issues, ok := inventoryNumber(out.IssueCount)
	if !ok || issues.Cmp(refs) > 0 {
		return false
	}
	if out.Consistent != (issues.Sign() == 0) || out.IssuesTruncated != (issues.Cmp(big.NewInt(100)) > 0) {
		return false
	}
	shown := issues
	if issues.Cmp(big.NewInt(100)) > 0 {
		shown = big.NewInt(100)
	}
	if shown.Cmp(big.NewInt(int64(len(out.Issues)))) != 0 {
		return false
	}
	sums := [3]*big.Int{new(big.Int), new(big.Int), new(big.Int)}
	perSource := map[string]*big.Int{}
	for i, c := range out.Coverage {
		if c.SourceTable != inventorySources[i] {
			return false
		}
		values := []string{c.SourceRowCount, c.ReferenceCount, c.IssueCount}
		parsed := make([]*big.Int, 3)
		for k, v := range values {
			n, ok := inventoryNumber(v)
			if !ok {
				return false
			}
			parsed[k] = n
			sums[k].Add(sums[k], n)
		}
		if parsed[2].Cmp(parsed[1]) > 0 || parsed[0].Sign() == 0 && parsed[1].Sign() != 0 {
			return false
		}
		perSource[c.SourceTable] = parsed[2]
	}
	if sums[0].Cmp(source) != 0 || sums[1].Cmp(refs) != 0 || sums[2].Cmp(issues) != 0 {
		return false
	}
	keys := make([]string, 0, len(out.Issues))
	shownCounts := map[string]int64{}
	for _, issue := range out.Issues {
		if _, ok := perSource[issue.SourceTable]; !ok {
			return false
		}
		if !inventoryKey.MatchString(issue.SourceID) || !inventoryLabel.MatchString(issue.ReferenceKey) || !inventoryLabel.MatchString(issue.ParentTable) ||
			issue.Code != "MISSING_PARENT_REFERENCE" && issue.Code != "MISSING_REQUIRED_LEDGER_REFERENCE" {
			return false
		}
		keys = append(keys, strings.Join([]string{issue.SourceTable, issue.SourceID, issue.ReferenceKey, issue.Code, issue.ParentTable}, "\x00"))
		shownCounts[issue.SourceTable]++
	}
	if !sort.StringsAreSorted(keys) {
		return false
	}
	for i := 1; i < len(keys); i++ {
		if keys[i] == keys[i-1] {
			return false
		}
	}
	for table, count := range shownCounts {
		if big.NewInt(count).Cmp(perSource[table]) > 0 {
			return false
		}
	}
	return true
}
