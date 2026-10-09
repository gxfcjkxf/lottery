package reporting

import (
	"encoding/csv"
	"fmt"
	"math/big"
	"strings"
	"time"
)

var attributionCSVFields = []string{
	"record_type", "brand_id", "snapshot_at", "timezone", "from", "to", "group_by",
	"game_id", "member_id", "agent_id", "agent_scope", "join_method", "key", "label",
}

func attributionCSVValues(v AttributionTotals) []string {
	return append(bettingCSVValues(v.BettingTotals), v.FinalLostStakePoints)
}

func attributionGroupKey(key, group string) bool {
	switch group {
	case "day":
		d, err := time.Parse("2006-01-02", key)
		return err == nil && d.Year() > 0 && d.Format("2006-01-02") == key
	case "game", "member":
		return uuid.MatchString(key)
	case "agent":
		return key == "none" || uuid.MatchString(key)
	case "join_method":
		return key == "domain" || key == "operator" || key == "agent_code" || key == "referral_code"
	}
	return false
}

func attributionTotalsMatch(t AttributionTotals, q AttributionQuery, key string) bool {
	parse := func(v string) *big.Int { n, _ := new(big.Int).SetString(v, 10); return n }
	orders := parse(t.OrderCount)
	count := new(big.Int)
	for _, v := range []string{t.PlacedCount, t.WonCount, t.LostCount, t.AbnormalCount, t.CancelledCount} {
		count.Add(count, parse(v))
	}
	if count.Cmp(orders) != 0 {
		return false
	}
	if orders.Sign() == 0 {
		for _, v := range attributionCSVValues(t) {
			if v != "0" {
				return false
			}
		}
	}
	if key == "" {
		return true
	}
	switch q.GroupBy {
	case "game":
		return q.GameID == nil || strings.EqualFold(key, *q.GameID)
	case "member":
		return q.MemberID == nil || strings.EqualFold(key, *q.MemberID)
	case "join_method":
		return q.JoinMethod == nil || key == *q.JoinMethod
	case "agent":
		return q.AgentID == nil || q.AgentScope == "downline" || strings.EqualFold(key, *q.AgentID)
	}
	return true
}

// AttributionCSV exports one full observation. No stitched pages, overlapping
// ancestry rows, balances, calculated commission or partial/truncated file.
func AttributionCSV(r AttributionReport) ([]byte, error) {
	if !uuid.MatchString(r.BrandID) || r.SnapshotAt.IsZero() || r.SnapshotAt.Year() < 1 || r.SnapshotAt.Year() > 9999 || r.Query.Validate() != nil || r.Query.Offset != 0 || r.TotalGroups != fmt.Sprint(len(r.Items)) || r.Timezone == "" || r.Timezone == "Local" {
		return nil, ErrInvalid
	}
	if len(r.Items) > ExportGroupLimit {
		return nil, ErrExportTooLarge
	}
	if _, err := time.LoadLocation(r.Timezone); err != nil {
		return nil, ErrInvalid
	}
	optional := func(v *string) string {
		if v == nil {
			return ""
		}
		return *v
	}
	meta := []string{
		"", r.BrandID, r.SnapshotAt.UTC().Format(time.RFC3339Nano), r.Timezone,
		r.Query.From.UTC().Format(time.RFC3339Nano), r.Query.To.UTC().Format(time.RFC3339Nano), r.Query.GroupBy,
		optional(r.Query.GameID), optional(r.Query.MemberID), optional(r.Query.AgentID), r.Query.AgentScope, optional(r.Query.JoinMethod), "", "",
	}
	values := attributionCSVValues(r.Summary)
	if err := decimalFields(values, -1); err != nil {
		return nil, err
	}
	if !attributionTotalsMatch(r.Summary, r.Query, "") {
		return nil, ErrInvalid
	}
	target := &cappedCSV{}
	if _, err := target.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return nil, err
	}
	w := csv.NewWriter(target)
	w.UseCRLF = true
	header := append([]string{}, attributionCSVFields...)
	for _, field := range attributionAggregates {
		header = append(header, field.name)
	}
	if err := w.Write(header); err != nil {
		return nil, err
	}
	summary := append(append([]string{}, meta...), values...)
	summary[0] = "summary"
	if err := w.Write(summary); err != nil {
		return nil, err
	}
	sums := make([]big.Int, len(values))
	last := ""
	for i, item := range r.Items {
		if !attributionGroupKey(item.Key, r.Query.GroupBy) || i > 0 && item.Key <= last {
			return nil, ErrInvalid
		}
		last = item.Key
		groupValues := attributionCSVValues(item.Totals)
		if err := decimalFields(groupValues, -1); err != nil {
			return nil, err
		}
		if !attributionTotalsMatch(item.Totals, r.Query, item.Key) {
			return nil, ErrInvalid
		}
		for j, value := range groupValues {
			n, ok := new(big.Int).SetString(value, 10)
			if !ok {
				return nil, ErrInvalid
			}
			sums[j].Add(&sums[j], n)
		}
		row := append(append([]string{}, meta...), groupValues...)
		row[0] = "group"
		row[12] = item.Key
		label, err := csvText(item.Label)
		if err != nil {
			return nil, err
		}
		row[13] = label
		if err = w.Write(row); err != nil {
			return nil, err
		}
	}
	for i, value := range values {
		if sums[i].String() != value {
			return nil, ErrInvalid
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return append([]byte(nil), target.data.Bytes()...), nil
}
