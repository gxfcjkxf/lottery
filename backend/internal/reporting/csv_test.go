package reporting

import (
	"encoding/csv"
	"strings"
	"testing"
	"time"
)

func csvFixture() Report[BettingTotals] {
	at := time.Date(2026, 10, 7, 0, 0, 0, 123456000, time.UTC)
	total := BettingTotals{OrderCount: "1", StakePoints: "18000000000000000000", PlacedCount: "1", WonCount: "0", LostCount: "0", AbnormalCount: "0", CancelledCount: "0", RefundPoints: "0", SettledStakePoints: "0", UnfinalizedStakePoints: "18000000000000000000", AbnormalStakePoints: "0", CurrentPrizePoints: "0", CorrectionOpenCount: "0"}
	return Report[BettingTotals]{BrandID: testBrand, SnapshotAt: at, Timezone: "Asia/Manila", Query: Query{From: at.Add(-time.Hour), To: at.Add(time.Hour), GroupBy: "game", Limit: 20}, Summary: total, TotalGroups: "1", Items: []Group[BettingTotals]{{Key: testBrand, Label: "  =SUM(1,2)\nquoted \"game\"", Totals: total}}}
}
func TestCSVExportExactMetricsFormulaSafetyAndCompleteShape(t *testing.T) {
	r := csvFixture()
	body, e := BettingCSV(r)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.HasPrefix(string(body), "\xef\xbb\xbf") {
		t.Fatal("UTF8 BOM missing")
	}
	rows, e := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\xef\xbb\xbf"))).ReadAll()
	if e != nil {
		t.Fatal(e)
	}
	if len(rows) != 3 || len(rows[0]) != 24 || rows[0][0] != "record_type" || rows[1][0] != "summary" || rows[2][0] != "group" || rows[2][10] != "'"+r.Items[0].Label || rows[1][12] != "18000000000000000000" {
		t.Fatal(rows)
	}
	if rows[2][4] != r.Query.From.Format(time.RFC3339Nano) || rows[2][5] != r.Query.To.Format(time.RFC3339Nano) {
		t.Fatal("filter/time precision lost")
	}
	r.TotalGroups = "2"
	if _, e = BettingCSV(r); e == nil {
		t.Fatal("truncated group list accepted")
	}
	r = csvFixture()
	r.Summary.StakePoints = "1e18"
	if _, e = BettingCSV(r); e == nil {
		t.Fatal("noncanonical number accepted")
	}
	r = csvFixture()
	r.Summary.StakePoints = "1" + strings.Repeat("0", 180)
	r.Items[0].Totals.StakePoints = r.Summary.StakePoints
	if _, e = BettingCSV(r); e != nil {
		t.Fatal("arbitrary precision string rejected", e)
	}
}
func TestLedgerCSVSeparatesBalancesNegativePostingsAndSafety(t *testing.T) {
	at := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	totals := LedgerTotals{EntryCount: "1", NetPoints: "-9000000000000000000", RechargePoints: "0", PrizeCreditPoints: "0", PrizeReversalPoints: "9000000000000000000", RefundPoints: "0"}
	r := LedgerReport{Report: Report[LedgerTotals]{BrandID: testBrand, SnapshotAt: at, Timezone: "UTC", Query: Query{From: at.Add(-time.Hour), To: at.Add(time.Hour), GroupBy: "entry_type", Limit: 20}, Summary: totals, TotalGroups: "1", Items: []Group[LedgerTotals]{{Key: "-danger", Label: "-danger", Totals: totals}}}, Balances: Balances{AccountCount: "2", AvailablePoints: "18000000000000000000", FrozenPoints: "0", WithdrawalPoints: "0", TotalPoints: "18000000000000000000"}}
	body, e := LedgerCSV(r)
	if e != nil {
		t.Fatal(e)
	}
	rows, e := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\xef\xbb\xbf"))).ReadAll()
	if e != nil {
		t.Fatal(e)
	}
	if len(rows) != 4 || len(rows[0]) != 22 || rows[1][12] != "-9000000000000000000" || rows[2][0] != "balances" || rows[2][18] != "18000000000000000000" || rows[3][9] != "'-danger" {
		t.Fatal(rows)
	}
	for _, bad := range []string{"-0", "+1", "1.0"} {
		r.Summary.NetPoints = bad
		if _, e = LedgerCSV(r); e == nil {
			t.Fatal("noncanonical signed number accepted")
		}
	}
}
func TestCSVExportFailsClosedBeforeHugeOrMalformedContent(t *testing.T) {
	r := csvFixture()
	r.Items[0].Label = strings.Repeat("A", CSVByteLimit)
	if _, e := BettingCSV(r); e != ErrExportTooLarge {
		t.Fatal("oversized body not rejected", e)
	}
	for _, label := range []string{"\x00bad", string([]byte{0xff}), "valid"} {
		r = csvFixture()
		r.Items[0].Label = label
		_, e := BettingCSV(r)
		if label == "valid" && e != nil {
			t.Fatal(e)
		}
		if label != "valid" && e == nil {
			t.Fatal("unsafe encoding accepted")
		}
	}
	r = csvFixture()
	r.Query.Offset = 20
	if _, e := BettingCSV(r); e == nil {
		t.Fatal("paginated CSV mislabeled complete")
	}
	r = csvFixture()
	r.TotalGroups = "010"
	if _, e := BettingCSV(r); e == nil {
		t.Fatal("malformed group count accepted")
	}
}

func TestWithdrawalCSVIsExactFormulaSafeAndContainsNoGameFilter(t *testing.T) {
	at := time.Date(2026, 10, 7, 0, 0, 0, 123456000, time.UTC)
	member := "0199a000-0000-7000-8000-000000000007"
	totals := WithdrawalTotals{OrderCount: "1", RequestedPoints: "18000000000000000000", ReviewingCount: "1", ReviewingPoints: "18000000000000000000", ProcessingCount: "0", ProcessingPoints: "0", PaidCount: "0", PaidPoints: "0", RejectedCount: "0", RejectedPoints: "0", FailedCount: "0", FailedPoints: "0", CancelledCount: "0", CancelledPoints: "0"}
	r := WithdrawalReport{BrandID: testBrand, SnapshotAt: at, Timezone: "Asia/Singapore", Query: Query{From: at.Add(-time.Hour), To: at.Add(time.Hour), GroupBy: "day", Limit: 20, MemberID: &member}, Summary: totals, TotalGroups: "1", Items: []Group[WithdrawalTotals]{{Key: "2026-10-07", Label: "=SUM(A1:A2)", Totals: totals}}}
	body, err := WithdrawalCSV(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(body), "\xef\xbb\xbf") || strings.Contains(string(body), "game_id") {
		t.Fatal("BOM missing or game filter leaked")
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\xef\xbb\xbf"))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || len(rows[0]) != 24 || rows[1][0] != "summary" || rows[1][7] != member || rows[2][0] != "group" || rows[2][7] != member || rows[2][9] != "'=SUM(A1:A2)" || rows[2][11] != "18000000000000000000" {
		t.Fatal(rows)
	}
	r.Query.Offset = 20
	if _, err = WithdrawalCSV(r); err == nil {
		t.Fatal("paginated export accepted")
	}
}
