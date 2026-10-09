package reporting

import (
	"encoding/csv"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"
)

var attributionCSVHeader = []string{
	"record_type", "brand_id", "snapshot_at", "timezone", "from", "to", "group_by",
	"game_id", "member_id", "agent_id", "agent_scope", "join_method", "key", "label",
	"order_count", "stake_points", "placed_count", "won_count", "lost_count",
	"abnormal_count", "cancelled_count", "refund_points", "settled_stake_points",
	"unfinalized_stake_points", "abnormal_stake_points", "current_prize_points",
	"correction_open_count", "final_lost_stake_points", "legacy_attribution_count",
}

func attributionCSVFixture() AttributionReport {
	query := AttributionQuery{
		From:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		To:         time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		GroupBy:    "game",
		Limit:      20,
		AgentScope: "downline",
		GameID:     attributionStringPointer("0199a000-0000-7000-8000-000000000020"),
		MemberID:   attributionStringPointer("0199a000-0000-7000-8000-000000000021"),
		AgentID:    attributionStringPointer("0199a000-0000-7000-8000-000000000012"),
		JoinMethod: attributionStringPointer("agent_code"),
	}
	totals := attributionTestTotals()
	totals.OrderCount = "2"
	totals.PlacedCount = "1"
	totals.LostCount = "1"
	totals.StakePoints = "900719925474099312345"
	totals.SettledStakePoints = "400000000000000000000"
	totals.UnfinalizedStakePoints = "500719925474099312345"
	totals.FinalLostStakePoints = "400000000000000000000"
	totals.LegacyAttributionCount = "0"
	return AttributionReport{
		BrandID:    "0199a000-0000-7000-8000-000000000001",
		SnapshotAt: time.Date(2026, 1, 2, 3, 4, 5, 678901234, time.UTC),
		Timezone:   "Asia/Singapore",
		Query:      query,
		Summary:    totals,
		Items: []Group[AttributionTotals]{{
			Key:    "0199a000-0000-7000-8000-000000000020",
			Label:  "=SUM(1,1)",
			Totals: totals,
		}},
		TotalGroups: "1",
	}
}

func attributionTestTotals() AttributionTotals {
	return AttributionTotals{
		BettingTotals: BettingTotals{
			OrderCount: "0", StakePoints: "0", PlacedCount: "0", WonCount: "0", LostCount: "0",
			AbnormalCount: "0", CancelledCount: "0", RefundPoints: "0", SettledStakePoints: "0",
			UnfinalizedStakePoints: "0", AbnormalStakePoints: "0", CurrentPrizePoints: "0",
			CorrectionOpenCount: "0",
		},
		FinalLostStakePoints: "0", LegacyAttributionCount: "0",
	}
}

func attributionCSVRows(t *testing.T, body []byte) [][]string {
	t.Helper()
	if !strings.HasPrefix(string(body), "\xef\xbb\xbf") {
		t.Fatal("CSV is missing UTF-8 BOM")
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\xef\xbb\xbf"))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestAttributionCSVHasExactClosedHeaderAndEchoesAllMetadata(t *testing.T) {
	r := attributionCSVFixture()
	body, err := AttributionCSV(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) > CSVByteLimit {
		t.Fatalf("CSV exceeds byte cap: %d", len(body))
	}
	if !strings.HasSuffix(string(body), "\r\n") {
		t.Fatal("CSV records must use CRLF")
	}
	rows := attributionCSVRows(t, body)
	if len(rows) != 3 || len(rows[0]) != 29 {
		t.Fatalf("expected header, summary, and one group with 29 columns; got %d rows and header width %d", len(rows), len(rows[0]))
	}
	for i, want := range attributionCSVHeader {
		if rows[0][i] != want {
			t.Errorf("header[%d]=%q, want %q", i, rows[0][i], want)
		}
	}
	if rows[1][0] != "summary" || rows[2][0] != "group" {
		t.Fatalf("expected summary followed by group, got %q and %q", rows[1][0], rows[2][0])
	}
	if rows[1][1] != r.BrandID || rows[1][2] != r.SnapshotAt.UTC().Format(time.RFC3339Nano) ||
		rows[1][3] != r.Timezone || rows[1][4] != r.Query.From.UTC().Format(time.RFC3339Nano) ||
		rows[1][5] != r.Query.To.UTC().Format(time.RFC3339Nano) || rows[1][6] != r.Query.GroupBy ||
		rows[1][7] != *r.Query.GameID || rows[1][8] != *r.Query.MemberID || rows[1][9] != *r.Query.AgentID ||
		rows[1][10] != "downline" || rows[1][11] != "agent_code" || rows[1][12] != "" || rows[1][13] != "" {
		t.Fatalf("CSV metadata did not echo the complete query in the specified order: %v", rows[1][:14])
	}
	if rows[1][14] != "2" || rows[1][15] != "900719925474099312345" || rows[1][18] != "1" ||
		rows[1][22] != "400000000000000000000" || rows[1][23] != "500719925474099312345" ||
		rows[1][27] != "400000000000000000000" || rows[1][28] != "0" {
		t.Fatalf("summary metrics changed or lost exact integer precision: %v", rows[1][14:])
	}
	if rows[2][12] != r.Items[0].Key || rows[2][13] != "'=SUM(1,1)" {
		t.Fatalf("group key or formula-safe label changed: %v", rows[2][12:14])
	}
}

func TestAttributionCSVRequiresWholeGroupsAndExactSummarySums(t *testing.T) {
	r := attributionCSVFixture()
	r.Summary.StakePoints = "900719925474099312346"
	if body, err := AttributionCSV(r); err != ErrInvalid || body != nil {
		t.Fatalf("accepted group sums that differ from the summary: bytes=%d err=%v", len(body), err)
	}
	r = attributionCSVFixture()
	r.Items = nil
	if body, err := AttributionCSV(r); err != ErrInvalid || body != nil {
		t.Fatalf("accepted an export missing a group: bytes=%d err=%v", len(body), err)
	}
	r = attributionCSVFixture()
	r.Query.Offset = 1
	if _, err := AttributionCSV(r); err != ErrInvalid {
		t.Fatalf("accepted paginated export: %v", err)
	}
	r = attributionCSVFixture()
	r.TotalGroups = "2"
	if _, err := AttributionCSV(r); err != ErrInvalid {
		t.Fatalf("accepted group count not matching the complete export: %v", err)
	}
}

func TestAttributionAgentCSVKeepsNoAgentAndLegacyAsSeparateStableKeys(t *testing.T) {
	r := attributionCSVFixture()
	r.Query.GroupBy = "agent"
	r.Query.AgentID, r.Query.AgentScope, r.Query.JoinMethod = nil, "direct", nil
	r.Query.GameID, r.Query.MemberID = nil, nil
	uuid := "0199a000-0000-7000-8000-000000000099"
	lost := attributionTestTotals()
	lost.OrderCount, lost.LostCount, lost.StakePoints = "1", "1", "1"
	lost.SettledStakePoints, lost.FinalLostStakePoints = "1", "1"
	legacyLost := lost
	legacyLost.LegacyAttributionCount = "1"
	r.Items = []Group[AttributionTotals]{
		{Key: uuid, Label: uuid, Totals: lost},
		{Key: "legacy", Label: "legacy", Totals: legacyLost},
		{Key: "none", Label: "none", Totals: lost},
	}
	r.Summary = lost
	r.Summary.OrderCount, r.Summary.LostCount, r.Summary.StakePoints = "3", "3", "3"
	r.Summary.SettledStakePoints, r.Summary.FinalLostStakePoints = "3", "3"
	r.Summary.LegacyAttributionCount = "1"
	r.TotalGroups = "3"
	body, err := AttributionCSV(r)
	if err != nil {
		t.Fatal(err)
	}
	rows := attributionCSVRows(t, body)
	if len(rows) != 5 || rows[2][12] != uuid || rows[3][12] != "legacy" || rows[4][12] != "none" {
		t.Fatalf("agent grouping merged or rewrote saved provenance keys: %v", rows)
	}

	r.Query.GroupBy = "join_method"
	r.Query.JoinMethod = nil
	r.Query.GameID, r.Query.MemberID = nil, nil
	r.Items = []Group[AttributionTotals]{
		{Key: "agent_code", Label: "agent_code", Totals: lost},
		{Key: "domain", Label: "domain", Totals: lost},
		{Key: "legacy", Label: "legacy", Totals: legacyLost},
		{Key: "operator", Label: "operator", Totals: lost},
		{Key: "referral_code", Label: "referral_code", Totals: lost},
	}
	r.TotalGroups = "5"
	r.Summary.OrderCount, r.Summary.LostCount, r.Summary.StakePoints = "5", "5", "5"
	r.Summary.SettledStakePoints, r.Summary.FinalLostStakePoints = "5", "5"
	r.Summary.LegacyAttributionCount = "1"
	if _, err = AttributionCSV(r); err != nil {
		t.Fatalf("closed join-method categories rejected: %v", err)
	}
}

func TestAttributionCSVRejectsContradictorySourceAndDirectAgentGroups(t *testing.T) {
	r := attributionCSVFixture()
	r.Query.GroupBy = "agent"
	r.Query.AgentScope = "direct"
	r.Items[0].Key = "0199a000-0000-7000-8000-000000000099"
	if body, err := AttributionCSV(r); err != ErrInvalid || body != nil {
		t.Fatalf("accepted a group for a different direct agent: bytes=%d err=%v", len(body), err)
	}
	r = attributionCSVFixture()
	r.Query.GroupBy = "join_method"
	r.Query.JoinMethod = attributionStringPointer("legacy")
	r.Items[0].Key, r.Items[0].Label = "agent_code", "agent_code"
	if body, err := AttributionCSV(r); err != ErrInvalid || body != nil {
		t.Fatalf("accepted a group key that contradicts the saved source filter: bytes=%d err=%v", len(body), err)
	}
}

func TestAttributionCSVLegacyGroupRetainsLegacyProofWithoutAgentIdentity(t *testing.T) {
	r := attributionCSVFixture()
	r.Query.GroupBy = "agent"
	r.Query.AgentID = nil
	r.Query.AgentScope = "direct"
	r.Query.GroupBy = "agent"
	r.Query.GameID, r.Query.MemberID = nil, nil
	r.Query.JoinMethod = attributionStringPointer("legacy")
	r.Items = []Group[AttributionTotals]{{Key: "legacy", Label: "legacy", Totals: attributionTestTotals()}}
	r.Summary = attributionTestTotals()
	r.Summary.OrderCount = "1"
	r.Summary.StakePoints = "1"
	r.Summary.LostCount = "1"
	r.Summary.SettledStakePoints = "1"
	r.Summary.FinalLostStakePoints = "1"
	r.Summary.LegacyAttributionCount = "1"
	r.Items[0].Totals = r.Summary
	r.TotalGroups = "1"
	if _, err := AttributionCSV(r); err != nil {
		t.Fatalf("valid legacy-only evidence was rejected or rewritten: %v", err)
	}
}

func TestAttributionCSVRejectsSignedAndNoncanonicalMetrics(t *testing.T) {
	for _, edit := range []struct {
		name string
		fn   func(*AttributionReport)
	}{
		{"signed total", func(r *AttributionReport) { r.Summary.StakePoints = "-1" }},
		{"negative zero", func(r *AttributionReport) { r.Summary.FinalLostStakePoints = "-0" }},
		{"leading zero", func(r *AttributionReport) { r.Summary.OrderCount = "02" }},
		{"nondigit", func(r *AttributionReport) { r.Items[0].Totals.LegacyAttributionCount = "1.0" }},
	} {
		t.Run(edit.name, func(t *testing.T) {
			r := attributionCSVFixture()
			edit.fn(&r)
			if body, err := AttributionCSV(r); err != ErrInvalid || body != nil {
				t.Fatalf("accepted malformed unsigned metric: bytes=%d err=%v", len(body), err)
			}
		})
	}
}

func TestAttributionCSVRejectsContradictoryTotals(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*AttributionReport)
	}{
		{"nonzero values with zero orders", func(r *AttributionReport) {
			r.Summary, r.Items[0].Totals = attributionTestTotals(), attributionTestTotals()
			r.Summary.StakePoints, r.Items[0].Totals.StakePoints = "1", "1"
		}},
		{"status counts do not partition orders", func(r *AttributionReport) {
			r.Summary.PlacedCount, r.Items[0].Totals.PlacedCount = "0", "0"
		}},
		{"legacy count exceeds matching cohort", func(r *AttributionReport) {
			r.Summary.LegacyAttributionCount, r.Items[0].Totals.LegacyAttributionCount = "1", "1"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := attributionCSVFixture()
			test.edit(&r)
			if body, err := AttributionCSV(r); err != ErrInvalid || body != nil {
				t.Fatalf("accepted contradictory attribution totals: bytes=%d err=%v", len(body), err)
			}
		})
	}
}

func TestAttributionCSVUsesUnboundedExactDecimalSums(t *testing.T) {
	const maxUint64PlusOne = "18446744073709551616"
	r := attributionCSVFixture()
	totals := attributionTestTotals()
	totals.OrderCount = maxUint64PlusOne
	totals.LostCount = maxUint64PlusOne
	totals.StakePoints = "18446744073709551615"
	totals.SettledStakePoints = "18446744073709551615"
	totals.FinalLostStakePoints = "18446744073709551615"
	totals.LegacyAttributionCount = "18446744073709551616"
	r.Summary = totals
	r.Items[0].Totals = totals
	r.Query.AgentID = nil
	r.Query.AgentScope = "direct"
	r.Query.GroupBy = "agent"
	r.Query.GameID, r.Query.MemberID = nil, nil
	r.Query.JoinMethod = attributionStringPointer("legacy")
	r.Items[0].Key, r.Items[0].Label = "legacy", "legacy"
	if _, err := AttributionCSV(r); err != nil {
		t.Fatalf("rejected exact totals above uint64/int64 range: %v", err)
	}

	// Sum independently large groups using arbitrary precision, not machine ints.
	r.Items = make([]Group[AttributionTotals], 2)
	for i := range r.Items {
		v := attributionTestTotals()
		v.OrderCount = "9223372036854775808"
		v.LostCount = "9223372036854775808"
		v.StakePoints = "9223372036854775808"
		v.SettledStakePoints = "9223372036854775808"
		v.FinalLostStakePoints = "9223372036854775808"
		r.Items[i] = Group[AttributionTotals]{Key: fmt.Sprintf("0199a000-0000-7000-8000-%012d", i+100), Label: "agent", Totals: v}
	}
	r.Summary = attributionTestTotals()
	r.Summary.OrderCount = new(big.Int).Mul(big.NewInt(2), mustAttributionBig(t, "9223372036854775808")).String()
	r.Summary.StakePoints = r.Summary.OrderCount
	r.Summary.LostCount = r.Summary.OrderCount
	r.Summary.SettledStakePoints = r.Summary.OrderCount
	r.Summary.FinalLostStakePoints = r.Summary.OrderCount
	r.Query.AgentID = nil
	r.Query.AgentScope = "direct"
	r.Query.GroupBy = "agent"
	r.Query.GameID, r.Query.MemberID, r.Query.JoinMethod = nil, nil, nil
	r.TotalGroups = "2"
	if _, err := AttributionCSV(r); err != nil {
		t.Fatalf("rejected exact aggregation beyond int64: %v", err)
	}
}

func TestAttributionCSVHonorsGroupAndByteCapsWithoutPartialBody(t *testing.T) {
	r := attributionCSVFixture()
	r.Query.AgentID, r.Query.GameID, r.Query.MemberID, r.Query.JoinMethod = nil, nil, nil, nil
	r.Query.AgentScope = "direct"
	r.Query.GroupBy = "agent"
	zero := attributionTestTotals()
	const count = ExportGroupLimit
	r.Items = make([]Group[AttributionTotals], count)
	for i := range r.Items {
		key := fmt.Sprintf("00000000-0000-0000-0000-%012x", i+1)
		group := attributionTestTotals()
		group.OrderCount, group.LostCount, group.StakePoints = "1", "1", "1"
		group.SettledStakePoints, group.FinalLostStakePoints = "1", "1"
		r.Items[i] = Group[AttributionTotals]{Key: key, Label: key, Totals: group}
	}
	r.Summary = zero
	r.Summary.OrderCount, r.Summary.LostCount, r.Summary.StakePoints = fmt.Sprint(count), fmt.Sprint(count), fmt.Sprint(count)
	r.Summary.SettledStakePoints, r.Summary.FinalLostStakePoints = fmt.Sprint(count), fmt.Sprint(count)
	r.TotalGroups = fmt.Sprint(count)
	body, err := AttributionCSV(r)
	if err != nil || len(body) > CSVByteLimit {
		t.Fatalf("exactly %d groups should export completely: bytes=%d err=%v", count, len(body), err)
	}
	lastKey := "ffffffff-ffff-ffff-ffff-ffffffffffff"
	r.Items = append(r.Items, Group[AttributionTotals]{Key: lastKey, Label: lastKey, Totals: r.Items[0].Totals})
	r.TotalGroups = fmt.Sprint(len(r.Items))
	if body, err = AttributionCSV(r); err != ErrExportTooLarge || body != nil {
		t.Fatalf("group limit plus one must return no partial CSV: bytes=%d err=%v", len(body), err)
	}

	r = attributionCSVFixture()
	r.Query.GroupBy = "game"
	r.Query.GameID = &r.Items[0].Key
	r.Query.MemberID = nil
	r.Items[0].Label = strings.Repeat("x", CSVByteLimit)
	if body, err = AttributionCSV(r); err != ErrExportTooLarge || body != nil {
		t.Fatalf("byte limit overflow must return no partial CSV: bytes=%d err=%v", len(body), err)
	}
}

func mustAttributionBig(t *testing.T, value string) *big.Int {
	t.Helper()
	n, ok := new(big.Int).SetString(value, 10)
	if !ok {
		t.Fatalf("invalid test integer %q", value)
	}
	return n
}

func attributionStringPointer(value string) *string { return &value }
