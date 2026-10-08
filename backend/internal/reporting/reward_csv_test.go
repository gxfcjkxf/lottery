package reporting

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"
)

func rewardCSVFixture() RewardReport {
	q := RewardQuery{From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), GroupBy: "order", Limit: 20}
	t := RewardTotals{EntryCount: "2", GrantEntryCount: "1", GrantPoints: "900719925474099312345", ReversalEntryCount: "1", ReversalPoints: "900719925474099312346", NetPoints: "-1"}
	key := "0199a000-0000-7000-8000-000000000002"
	return RewardReport{BrandID: rewardReportTestBrand, SnapshotAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), Timezone: "Asia/Singapore", Query: q, Summary: t, Items: []Group[RewardTotals]{{Key: key, Label: key, Totals: t}}, TotalGroups: "1"}
}

func TestRewardCSVHeadersExactArithmeticAndSignedFormulaSafety(t *testing.T) {
	r := rewardCSVFixture()
	body, err := RewardCSV(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) > CSVByteLimit || !strings.HasPrefix(string(body), "\xef\xbb\xbfrecord_type,") {
		t.Fatalf("bad export prefix/cap: %d", len(body))
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\xef\xbb\xbf"))).ReadAll()
	if err != nil || len(rows) != 3 {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
	want := strings.Join(rewardCSVFields, ",")
	if strings.Join(rows[0], ",") != want || rows[1][0] != "summary" || rows[1][16] != "'-1" || rows[2][9] != r.Items[0].Key {
		t.Fatalf("CSV contract mismatch: %v", rows)
	}
	var wire RewardQuery
	encoded, err := json.Marshal(r.Query)
	if err != nil || json.Unmarshal(encoded, &wire) != nil || wire.MemberID != nil || wire.OrderID != nil {
		t.Fatal("query JSON round trip changed null filters", err)
	}
	var envelope map[string]json.RawMessage
	if err = json.Unmarshal(mustJSON(t, r), &envelope); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"brand_id", "snapshot_at", "timezone", "query", "summary", "items", "total_groups"} {
		if _, ok := envelope[key]; !ok {
			t.Fatalf("report envelope is missing JSON field %q", key)
		}
	}
	var totalFields map[string]json.RawMessage
	if err = json.Unmarshal(envelope["summary"], &totalFields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"entry_count", "grant_entry_count", "grant_points", "reversal_entry_count", "reversal_points", "net_points"} {
		if _, ok := totalFields[key]; !ok {
			t.Fatalf("reward totals are missing JSON field %q", key)
		}
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestRewardOrdersCSVCompleteGroupsAndIntegrity(t *testing.T) {
	q := rewardCSVFixture().Query
	q.GroupBy = "state"
	totals := RewardOrderTotals{OrderCount: "3", OriginalPoints: "60", GrantedCount: "1", GrantedPoints: "10", PendingCount: "1", PendingPoints: "20", RevokedCount: "1", RevokedPoints: "30"}
	r := RewardOrderReport{BrandID: rewardReportTestBrand, SnapshotAt: time.Now().UTC(), Timezone: "Asia/Singapore", Query: q, Summary: totals, Items: []Group[RewardOrderTotals]{
		{Key: "granted", Label: "granted", Totals: RewardOrderTotals{OrderCount: "1", OriginalPoints: "10", GrantedCount: "1", GrantedPoints: "10", PendingCount: "0", PendingPoints: "0", RevokedCount: "0", RevokedPoints: "0"}},
		{Key: "revocation_pending", Label: "revocation_pending", Totals: RewardOrderTotals{OrderCount: "1", OriginalPoints: "20", GrantedCount: "0", GrantedPoints: "0", PendingCount: "1", PendingPoints: "20", RevokedCount: "0", RevokedPoints: "0"}},
		{Key: "revoked", Label: "revoked", Totals: RewardOrderTotals{OrderCount: "1", OriginalPoints: "30", GrantedCount: "0", GrantedPoints: "0", PendingCount: "0", PendingPoints: "0", RevokedCount: "1", RevokedPoints: "30"}},
	}, TotalGroups: "3"}
	body, err := RewardOrdersCSV(r)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\xef\xbb\xbf"))).ReadAll()
	if err != nil || len(rows) != 5 || strings.Join(rows[0], ",") != strings.Join(rewardOrdersCSVFields, ",") || rows[4][9] != "revoked" {
		t.Fatalf("order export lost final group: rows=%v err=%v", rows, err)
	}
	r.Items = r.Items[:2]
	if _, err = RewardOrdersCSV(r); err != ErrInvalid {
		t.Fatalf("truncated report accepted: %v", err)
	}
}

func TestRewardCSVRejectsBadTotalsKeysAndExportCaps(t *testing.T) {
	r := rewardCSVFixture()
	r.Summary.NetPoints = "0"
	if _, err := RewardCSV(r); err != ErrInvalid {
		t.Fatalf("inconsistent net accepted: %v", err)
	}
	r = rewardCSVFixture()
	r.Items[0].Label = "other"
	if _, err := RewardCSV(r); err != ErrInvalid {
		t.Fatalf("bad label accepted: %v", err)
	}
	r = rewardCSVFixture()
	r.Query.Offset = 1
	if _, err := RewardCSV(r); err != ErrInvalid {
		t.Fatalf("paged export accepted: %v", err)
	}
	r = rewardCSVFixture()
	r.Items = make([]Group[RewardTotals], ExportGroupLimit+1)
	if _, err := RewardCSV(r); err != ErrExportTooLarge {
		t.Fatalf("oversized group count: %v", err)
	}
	r = rewardCSVFixture()
	per := RewardTotals{EntryCount: "1", GrantEntryCount: "1", GrantPoints: strings.Repeat("9", 120), ReversalEntryCount: "0", ReversalPoints: "0", NetPoints: strings.Repeat("9", 120)}
	groupCount := 10000
	bigTotal := new(big.Int).Mul(mustBig(t, per.GrantPoints), big.NewInt(int64(groupCount))).String()
	r.Summary = RewardTotals{EntryCount: fmt.Sprint(groupCount), GrantEntryCount: fmt.Sprint(groupCount), GrantPoints: bigTotal, ReversalEntryCount: "0", ReversalPoints: "0", NetPoints: bigTotal}
	r.Items = make([]Group[RewardTotals], groupCount)
	for i := range r.Items {
		key := fmt.Sprintf("00000000-0000-0000-0000-%012x", i+1)
		r.Items[i] = Group[RewardTotals]{Key: key, Label: key, Totals: per}
	}
	r.TotalGroups = fmt.Sprint(groupCount)
	if body, err := RewardCSV(r); err != ErrExportTooLarge || body != nil {
		t.Fatalf("byte cap must reject without partial body: bytes=%d err=%v", len(body), err)
	}
}

func TestRewardCSVExactGroupLimitAndFinalRow(t *testing.T) {
	const count = ExportGroupLimit
	r := rewardCSVFixture()
	r.Summary = RewardTotals{EntryCount: fmt.Sprint(count), GrantEntryCount: fmt.Sprint(count), GrantPoints: fmt.Sprint(count), ReversalEntryCount: "0", ReversalPoints: "0", NetPoints: fmt.Sprint(count)}
	one := RewardTotals{EntryCount: "1", GrantEntryCount: "1", GrantPoints: "1", ReversalEntryCount: "0", ReversalPoints: "0", NetPoints: "1"}
	r.Items = make([]Group[RewardTotals], count)
	for i := range r.Items {
		key := fmt.Sprintf("00000000-0000-0000-0000-%012x", i+1)
		r.Items[i] = Group[RewardTotals]{Key: key, Label: key, Totals: one}
	}
	r.TotalGroups = fmt.Sprint(count)
	body, err := RewardCSV(r)
	if err != nil || len(body) > CSVByteLimit {
		t.Fatalf("exactly %d complete groups should fit: bytes=%d err=%v", count, len(body), err)
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\xef\xbb\xbf"))).ReadAll()
	wantLast := r.Items[count-1].Key
	if err != nil || len(rows) != count+2 || rows[len(rows)-1][0] != "group" || rows[len(rows)-1][9] != wantLast {
		t.Fatalf("exact group cap omitted final group: rows=%d last=%v err=%v", len(rows), rows[len(rows)-1], err)
	}

	q := r.Query
	q.GroupBy = "member"
	orderSummary := RewardOrderTotals{OrderCount: fmt.Sprint(count), OriginalPoints: fmt.Sprint(count), GrantedCount: fmt.Sprint(count), GrantedPoints: fmt.Sprint(count), PendingCount: "0", PendingPoints: "0", RevokedCount: "0", RevokedPoints: "0"}
	orderOne := RewardOrderTotals{OrderCount: "1", OriginalPoints: "1", GrantedCount: "1", GrantedPoints: "1", PendingCount: "0", PendingPoints: "0", RevokedCount: "0", RevokedPoints: "0"}
	or := RewardOrderReport{BrandID: r.BrandID, SnapshotAt: r.SnapshotAt, Timezone: r.Timezone, Query: q, Summary: orderSummary, Items: make([]Group[RewardOrderTotals], count), TotalGroups: fmt.Sprint(count)}
	for i := range or.Items {
		key := fmt.Sprintf("00000000-0000-0000-0000-%012x", i+1)
		or.Items[i] = Group[RewardOrderTotals]{Key: key, Label: key, Totals: orderOne}
	}
	body, err = RewardOrdersCSV(or)
	if err != nil || len(body) > CSVByteLimit {
		t.Fatalf("exactly %d complete order groups should fit: bytes=%d err=%v", count, len(body), err)
	}
	rows, err = csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\xef\xbb\xbf"))).ReadAll()
	if err != nil || len(rows) != count+2 || rows[len(rows)-1][0] != "group" || rows[len(rows)-1][9] != wantLast {
		t.Fatalf("exact order group cap omitted final group: rows=%d err=%v", len(rows), err)
	}
}

func TestRewardCSVRejectsNoncanonicalOrIncompleteEnvelopes(t *testing.T) {
	const unsafe = "9007199254740993"
	q := rewardCSVFixture().Query
	r := RewardReport{BrandID: rewardReportTestBrand, SnapshotAt: time.Now().UTC(), Timezone: "Asia/Singapore", Query: q,
		Summary: RewardTotals{EntryCount: unsafe, GrantEntryCount: unsafe, GrantPoints: unsafe, ReversalEntryCount: "0", ReversalPoints: "0", NetPoints: unsafe},
		Items:   []Group[RewardTotals]{{Key: "0199a000-0000-7000-8000-000000000002", Label: "0199a000-0000-7000-8000-000000000002", Totals: RewardTotals{EntryCount: unsafe, GrantEntryCount: unsafe, GrantPoints: unsafe, ReversalEntryCount: "0", ReversalPoints: "0", NetPoints: unsafe}}}, TotalGroups: "1"}
	if _, err := RewardCSV(r); err != nil {
		t.Fatalf("exact count above JS safe integer rejected: %v", err)
	}
	mutations := []struct {
		name string
		edit func(*RewardReport)
	}{
		{"noncanonical count", func(r *RewardReport) { r.Items[0].Totals.EntryCount = "09007199254740993" }},
		{"minimum points per entry", func(r *RewardReport) { r.Items[0].Totals.GrantPoints = "1" }},
		{"wrong group total", func(r *RewardReport) { r.TotalGroups = "2" }},
		{"uppercase machine key", func(r *RewardReport) {
			r.Items[0].Key = strings.ToUpper(r.Items[0].Key)
			r.Items[0].Label = r.Items[0].Key
		}},
		{"query day range", func(r *RewardReport) {
			r.Query.GroupBy = "day"
			r.Items[0].Key = "2027-01-01"
			r.Items[0].Label = r.Items[0].Key
		}},
		{"invalid timezone", func(r *RewardReport) { r.Timezone = "Mars/Olympus" }},
		{"invalid from/to", func(r *RewardReport) { r.Query.To = r.Query.From }},
		{"invalid snapshot year", func(r *RewardReport) { r.SnapshotAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) }},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			bad := r
			bad.Items = append([]Group[RewardTotals](nil), r.Items...)
			tc.edit(&bad)
			if _, err := RewardCSV(bad); err != ErrInvalid {
				t.Fatalf("invalid envelope accepted: %v", err)
			}
		})
	}

	base := rewardCSVFixture()
	base.Query.GroupBy = "order"
	base.Summary = RewardTotals{EntryCount: "2", GrantEntryCount: "1", GrantPoints: "2", ReversalEntryCount: "1", ReversalPoints: "3", NetPoints: "-1"}
	base.Items = []Group[RewardTotals]{
		{Key: "00000000-0000-0000-0000-000000000001", Label: "00000000-0000-0000-0000-000000000001", Totals: RewardTotals{EntryCount: "1", GrantEntryCount: "1", GrantPoints: "2", ReversalEntryCount: "0", ReversalPoints: "0", NetPoints: "2"}},
		{Key: "00000000-0000-0000-0000-000000000002", Label: "00000000-0000-0000-0000-000000000002", Totals: RewardTotals{EntryCount: "1", GrantEntryCount: "0", GrantPoints: "0", ReversalEntryCount: "1", ReversalPoints: "3", NetPoints: "-3"}},
	}
	base.TotalGroups = "2"
	base.Items[0], base.Items[1] = base.Items[1], base.Items[0]
	if _, err := RewardCSV(base); err != ErrInvalid {
		t.Fatalf("noncanonical C key order accepted: %v", err)
	}
}

func TestRewardOrdersCSVRejectsStateAndArithmeticMismatch(t *testing.T) {
	q := rewardCSVFixture().Query
	q.GroupBy = "state"
	oneGranted := RewardOrderTotals{OrderCount: "1", OriginalPoints: "5", GrantedCount: "1", GrantedPoints: "5", PendingCount: "0", PendingPoints: "0", RevokedCount: "0", RevokedPoints: "0"}
	r := RewardOrderReport{BrandID: rewardReportTestBrand, SnapshotAt: time.Now().UTC(), Timezone: "Asia/Singapore", Query: q, Summary: oneGranted,
		Items: []Group[RewardOrderTotals]{{Key: "revoked", Label: "revoked", Totals: oneGranted}}, TotalGroups: "1"}
	if _, err := RewardOrdersCSV(r); err != ErrInvalid {
		t.Fatalf("state group accepted totals for another state: %v", err)
	}
	r.Items[0].Key, r.Items[0].Label = "granted", "granted"
	r.Summary.OriginalPoints = "6"
	if _, err := RewardOrdersCSV(r); err != ErrInvalid {
		t.Fatalf("inconsistent original points accepted: %v", err)
	}
	const huge = "9007199254740993"
	r.Summary = RewardOrderTotals{OrderCount: huge, OriginalPoints: huge, GrantedCount: huge, GrantedPoints: huge, PendingCount: "0", PendingPoints: "0", RevokedCount: "0", RevokedPoints: "0"}
	r.Items = []Group[RewardOrderTotals]{{Key: "granted", Label: "granted", Totals: r.Summary}}
	if _, err := RewardOrdersCSV(r); err != nil {
		t.Fatalf("exact order count above JS safe integer rejected: %v", err)
	}
}

func TestRewardCSVDayKeysRespectTimezoneWindowAndEmptyEnvelope(t *testing.T) {
	r := rewardCSVFixture()
	r.Query.GroupBy = "day"
	r.Query.From = time.Date(2026, 1, 1, 23, 30, 0, 0, time.UTC)
	r.Query.To = time.Date(2026, 1, 2, 0, 30, 0, 0, time.UTC)
	r.Timezone = "Asia/Singapore"
	r.Items[0].Key, r.Items[0].Label = "2026-01-02", "2026-01-02"
	if _, err := RewardCSV(r); err != nil {
		t.Fatalf("local date boundary was rejected: %v", err)
	}
	r.Items[0].Key, r.Items[0].Label = "2026-01-01", "2026-01-01"
	if _, err := RewardCSV(r); err != ErrInvalid {
		t.Fatalf("UTC day accepted outside local date window: %v", err)
	}
	r.Items = nil
	r.TotalGroups = "0"
	r.Summary = RewardTotals{EntryCount: "0", GrantEntryCount: "0", GrantPoints: "0", ReversalEntryCount: "0", ReversalPoints: "0", NetPoints: "0"}
	if body, err := RewardCSV(r); err != nil || len(body) == 0 {
		t.Fatalf("empty summary envelope rejected: bytes=%d err=%v", len(body), err)
	}
	or := RewardOrderReport{BrandID: r.BrandID, SnapshotAt: r.SnapshotAt, Timezone: r.Timezone, Query: r.Query,
		Summary: RewardOrderTotals{OrderCount: "0", OriginalPoints: "0", GrantedCount: "0", GrantedPoints: "0", PendingCount: "0", PendingPoints: "0", RevokedCount: "0", RevokedPoints: "0"}, Items: []Group[RewardOrderTotals]{}, TotalGroups: "0"}
	if body, err := RewardOrdersCSV(or); err != nil || len(body) == 0 {
		t.Fatalf("empty order summary envelope rejected: bytes=%d err=%v", len(body), err)
	}
}
