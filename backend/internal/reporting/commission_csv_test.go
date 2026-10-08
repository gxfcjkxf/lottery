package reporting

import (
	"context"
	"encoding/csv"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func commissionCSVFixture() CommissionReport {
	q := CommissionQuery{From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), GroupBy: "agent", Limit: 20}
	totals := CommissionTotals{EntryCount: "2", PaidEntryCount: "1", PaidPoints: "900719925474099312345", AdjustmentEntryCount: "1", AdjustmentCreditPoints: "0", AdjustmentDebitPoints: "900719925474099312346", CorrectionEntryCount: "0", CorrectionCreditPoints: "0", CorrectionDebitPoints: "0", NetPoints: "-1"}
	return CommissionReport{BrandID: "0199a000-0000-7000-8000-000000000001", SnapshotAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), Timezone: "Asia/Singapore", Query: q, Summary: totals, Items: []Group[CommissionTotals]{{Key: "0199a000-0000-7000-8000-000000000002", Label: "0199a000-0000-7000-8000-000000000002", Totals: totals}}, TotalGroups: "1"}
}

func TestCommissionCSVExactSignedLedgerAndMetadata(t *testing.T) {
	report := commissionCSVFixture()
	body, err := CommissionCSV(report)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) > CSVByteLimit || !strings.HasPrefix(string(body), "\xef\xbb\xbfrecord_type,") {
		t.Fatal("missing BOM/header or byte cap", len(body))
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\xef\xbb\xbf"))).ReadAll()
	if err != nil || len(rows) != 3 {
		t.Fatal("bad CSV", err, len(rows))
	}
	if len(rows[0]) != 22 || rows[1][0] != "summary" || rows[1][12] != "2" || rows[1][13] != "1" || rows[1][14] != "900719925474099312345" || rows[1][18] != "0" || rows[1][19] != "0" || rows[1][20] != "0" || rows[1][21] != "'-1" {
		t.Fatal("summary changed exact totals", rows[1])
	}
	if rows[2][0] != "group" || rows[2][10] != "0199a000-0000-7000-8000-000000000002" || rows[2][11] != "0199a000-0000-7000-8000-000000000002" {
		t.Fatal("group metadata does not match the pinned contract", rows[2])
	}
}

func TestCommissionCSVAllowsSummaryOnlyEmptyReport(t *testing.T) {
	r := commissionCSVFixture()
	r.Summary = CommissionTotals{EntryCount: "0", PaidEntryCount: "0", PaidPoints: "0", AdjustmentEntryCount: "0", AdjustmentCreditPoints: "0", AdjustmentDebitPoints: "0", CorrectionEntryCount: "0", CorrectionCreditPoints: "0", CorrectionDebitPoints: "0", NetPoints: "0"}
	r.Items = nil
	r.TotalGroups = "0"
	body, err := CommissionCSV(r)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\xef\xbb\xbf"))).ReadAll()
	if err != nil || len(rows) != 2 || rows[1][0] != "summary" {
		t.Fatalf("empty export must contain only header and summary: rows=%d err=%v", len(rows), err)
	}
}

func TestCommissionCSVRejectsInvalidArithmeticAndTruncatedGroups(t *testing.T) {
	r := commissionCSVFixture()
	r.Summary.NetPoints = "0"
	if _, err := CommissionCSV(r); err != ErrInvalid {
		t.Fatalf("accepted inconsistent signed net: %v", err)
	}
	r = commissionCSVFixture()
	r.Items = nil
	if _, err := CommissionCSV(r); err != ErrInvalid {
		t.Fatalf("accepted summary not matching actual groups: %v", err)
	}
	r = commissionCSVFixture()
	r.Query.Offset = 1
	if _, err := CommissionCSV(r); err != ErrInvalid {
		t.Fatalf("accepted paged export: %v", err)
	}
	r = commissionCSVFixture()
	r.Items = make([]Group[CommissionTotals], ExportGroupLimit+1)
	if _, err := CommissionCSV(r); err != ErrExportTooLarge {
		t.Fatalf("expected group-limit error, got %v", err)
	}
}

func TestCommissionCSVCorrectionTotalsUseExactSignedArithmetic(t *testing.T) {
	r := commissionCSVFixture()
	totals := CommissionTotals{
		EntryCount: "3", PaidEntryCount: "1", PaidPoints: "5",
		AdjustmentEntryCount: "1", AdjustmentCreditPoints: "7", AdjustmentDebitPoints: "3",
		CorrectionEntryCount: "1", CorrectionCreditPoints: "11", CorrectionDebitPoints: "19", NetPoints: "1",
	}
	r.Summary, r.Items[0].Totals = totals, totals
	body, err := CommissionCSV(r)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\xef\xbb\xbf"))).ReadAll()
	if err != nil || rows[1][18] != "1" || rows[1][19] != "11" || rows[1][20] != "19" || rows[1][21] != "1" {
		t.Fatalf("correction fields or net arithmetic drifted: rows=%v err=%v", rows, err)
	}
	totals.CorrectionEntryCount = "0"
	r.Summary, r.Items[0].Totals = totals, totals
	if _, err := CommissionCSV(r); err != ErrInvalid {
		t.Fatalf("accepted entry count that omits a correction posting: %v", err)
	}
	totals.CorrectionEntryCount = "1"
	totals.CorrectionCreditPoints = "-1"
	r.Summary, r.Items[0].Totals = totals, totals
	if _, err := CommissionCSV(r); err != ErrInvalid {
		t.Fatalf("accepted negative correction credit: %v", err)
	}
}

func TestCommissionCSVExactGroupLimitAndByteCap(t *testing.T) {
	r := commissionCSVFixture()
	zero := CommissionTotals{EntryCount: "0", PaidEntryCount: "0", PaidPoints: "0", AdjustmentEntryCount: "0", AdjustmentCreditPoints: "0", AdjustmentDebitPoints: "0", CorrectionEntryCount: "0", CorrectionCreditPoints: "0", CorrectionDebitPoints: "0", NetPoints: "0"}
	r.Summary = zero
	r.Items = make([]Group[CommissionTotals], ExportGroupLimit)
	for i := range r.Items {
		key := fmt.Sprintf("00000000-0000-0000-0000-%012x", i+1)
		r.Items[i] = Group[CommissionTotals]{Key: key, Label: key, Totals: zero}
	}
	r.TotalGroups = fmt.Sprint(ExportGroupLimit)
	body, err := CommissionCSV(r)
	if err != nil || len(body) > CSVByteLimit {
		t.Fatalf("exactly %d groups must export wholly within the byte cap: bytes=%d err=%v", ExportGroupLimit, len(body), err)
	}
	r.Items = append(r.Items, Group[CommissionTotals]{Key: "ffffffff-ffff-ffff-ffff-ffffffffffff", Label: "ffffffff-ffff-ffff-ffff-ffffffffffff", Totals: zero})
	r.TotalGroups = fmt.Sprint(len(r.Items))
	if _, err = CommissionCSV(r); err != ErrExportTooLarge {
		t.Fatalf("%d groups must exceed export limit: %v", ExportGroupLimit+1, err)
	}
}

func TestCommissionCSVRejectsByteLimitWithoutPartialBody(t *testing.T) {
	r := commissionCSVFixture()
	point := strings.Repeat("9", 101)
	groupTotals := CommissionTotals{EntryCount: "2", PaidEntryCount: "1", PaidPoints: point, AdjustmentEntryCount: "1", AdjustmentCreditPoints: point, AdjustmentDebitPoints: point, CorrectionEntryCount: "0", CorrectionCreditPoints: "0", CorrectionDebitPoints: "0", NetPoints: point}
	count := ExportGroupLimit
	r.Items = make([]Group[CommissionTotals], count)
	r.Summary = CommissionTotals{EntryCount: fmt.Sprint(2 * count), PaidEntryCount: fmt.Sprint(count), PaidPoints: new(big.Int).Mul(mustBig(t, point), big.NewInt(int64(count))).String(), AdjustmentEntryCount: fmt.Sprint(count), AdjustmentCreditPoints: new(big.Int).Mul(mustBig(t, point), big.NewInt(int64(count))).String(), AdjustmentDebitPoints: new(big.Int).Mul(mustBig(t, point), big.NewInt(int64(count))).String(), CorrectionEntryCount: "0", CorrectionCreditPoints: "0", CorrectionDebitPoints: "0", NetPoints: new(big.Int).Mul(mustBig(t, point), big.NewInt(int64(count))).String()}
	for i := range r.Items {
		key := fmt.Sprintf("00000000-0000-0000-0000-%012x", i+1)
		r.Items[i] = Group[CommissionTotals]{Key: key, Label: key, Totals: groupTotals}
	}
	r.TotalGroups = fmt.Sprint(count)
	if body, err := CommissionCSV(r); err != ErrExportTooLarge || body != nil {
		t.Fatalf("oversized CSV must return no partial bytes: bytes=%d err=%v", len(body), err)
	}
}

func mustBig(t *testing.T, value string) *big.Int {
	t.Helper()
	n, ok := new(big.Int).SetString(value, 10)
	if !ok {
		t.Fatalf("invalid test integer %q", value)
	}
	return n
}

func TestCommissionCSVRejectsMalformedMetadata(t *testing.T) {
	mutations := []struct {
		name string
		edit func(*CommissionReport)
	}{
		{"brand", func(r *CommissionReport) { r.BrandID = "not-a-uuid" }},
		{"snapshot", func(r *CommissionReport) { r.SnapshotAt = time.Time{} }},
		{"timezone", func(r *CommissionReport) { r.Timezone = "Local" }},
		{"group count", func(r *CommissionReport) { r.TotalGroups = "2" }},
		{"label mismatch", func(r *CommissionReport) { r.Items[0].Label = "other" }},
		{"invalid key", func(r *CommissionReport) { r.Items[0].Key = "not-a-uuid" }},
		{"noncanonical signed total", func(r *CommissionReport) { r.Summary.NetPoints = "-0" }},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			r := commissionCSVFixture()
			tc.edit(&r)
			if _, err := CommissionCSV(r); err != ErrInvalid {
				t.Fatalf("malformed metadata accepted: %v", err)
			}
		})
	}
}

func TestCommissionCSVArithmeticUsesUnboundedIntegers(t *testing.T) {
	n := new(big.Int).SetUint64(^uint64(0))
	if n.String() != "18446744073709551615" {
		t.Fatal(n)
	}
	r := commissionCSVFixture()
	r.Summary.EntryCount = "18446744073709551616"
	r.Summary.PaidEntryCount = "18446744073709551615"
	r.Summary.AdjustmentEntryCount = "1"
	r.Summary.PaidPoints = "18446744073709551615"
	r.Summary.AdjustmentDebitPoints = "18446744073709551616"
	r.Summary.NetPoints = "-1"
	r.Items[0].Totals = r.Summary
	if _, err := CommissionCSV(r); err != nil {
		t.Fatalf("rejected exact unbounded arithmetic: %v", err)
	}
}

type commissionExportCapRow struct{}

func (commissionExportCapRow) Scan(dest ...any) error {
	*dest[0].(*time.Time) = time.Now().UTC()
	*dest[1].(*string) = "Asia/Singapore"
	*dest[2].(*bool) = true
	*dest[3].(*[]byte) = []byte(`{"entry_count":"0","paid_entry_count":"0","paid_points":"0","adjustment_entry_count":"0","adjustment_credit_points":"0","adjustment_debit_points":"0","correction_entry_count":"0","correction_credit_points":"0","correction_debit_points":"0","net_points":"0"}`)
	*dest[4].(*[]byte) = []byte("[]")
	*dest[5].(*string) = fmt.Sprint(ExportGroupLimit + 1)
	return nil
}

type commissionExportCapRunner struct{}

func (commissionExportCapRunner) QueryRow(context.Context, string, ...any) pgx.Row {
	return commissionExportCapRow{}
}

func TestCommissionExportServiceRejectsGroupLimitPlusOne(t *testing.T) {
	now := time.Now().UTC()
	q := CommissionQuery{From: now.Add(-time.Hour), To: now, GroupBy: "day", Limit: 20}
	out, err := (Service{}).commission(context.Background(), commissionExportCapRunner{}, "0199a000-0000-7000-8000-000000000001", q, ExportGroupLimit+1, 0, true)
	if err != ErrExportTooLarge || out.TotalGroups != fmt.Sprint(ExportGroupLimit+1) || len(out.Items) != 0 {
		t.Fatalf("service returned a partial or accepted oversized export: groups=%s rows=%d err=%v", out.TotalGroups, len(out.Items), err)
	}
}
