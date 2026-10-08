package reporting

import (
	"encoding/csv"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"
)

var rewardCSVFields = []string{
	"record_type", "brand_id", "snapshot_at", "timezone", "from", "to", "group_by", "member_id", "order_id", "key", "label",
	"entry_count", "grant_entry_count", "grant_points", "reversal_entry_count", "reversal_points", "net_points",
}

var rewardOrdersCSVFields = []string{
	"record_type", "brand_id", "snapshot_at", "timezone", "from", "to", "group_by", "member_id", "order_id", "key", "label",
	"order_count", "original_points", "granted_count", "granted_points", "pending_count", "pending_points", "revoked_count", "revoked_points",
}

func rewardCSVValues(v RewardTotals) []string {
	return []string{v.EntryCount, v.GrantEntryCount, v.GrantPoints, v.ReversalEntryCount, v.ReversalPoints, v.NetPoints}
}

func rewardOrdersCSVValues(v RewardOrderTotals) []string {
	return []string{v.OrderCount, v.OriginalPoints, v.GrantedCount, v.GrantedPoints, v.PendingCount, v.PendingPoints, v.RevokedCount, v.RevokedPoints}
}

func validRewardTotals(v RewardTotals) bool {
	values := rewardCSVValues(v)
	if decimalFields(values, len(values)-1) != nil {
		return false
	}
	entries, _ := new(big.Int).SetString(v.EntryCount, 10)
	grants, _ := new(big.Int).SetString(v.GrantEntryCount, 10)
	reversals, _ := new(big.Int).SetString(v.ReversalEntryCount, 10)
	grantPoints, _ := new(big.Int).SetString(v.GrantPoints, 10)
	reversalPoints, _ := new(big.Int).SetString(v.ReversalPoints, 10)
	net, _ := new(big.Int).SetString(v.NetPoints, 10)
	if entries.Cmp(new(big.Int).Add(new(big.Int).Set(grants), reversals)) != 0 ||
		(grants.Sign() == 0) != (grantPoints.Sign() == 0) || (reversals.Sign() == 0) != (reversalPoints.Sign() == 0) ||
		grants.Cmp(grantPoints) > 0 || reversals.Cmp(reversalPoints) > 0 {
		return false
	}
	return net.Cmp(new(big.Int).Sub(grantPoints, reversalPoints)) == 0
}

func validRewardOrderTotals(v RewardOrderTotals) bool {
	values := rewardOrdersCSVValues(v)
	if decimalFields(values, -1) != nil {
		return false
	}
	count, _ := new(big.Int).SetString(v.OrderCount, 10)
	original, _ := new(big.Int).SetString(v.OriginalPoints, 10)
	counts := []*big.Int{}
	points := []*big.Int{}
	for _, pair := range [][2]string{{v.GrantedCount, v.GrantedPoints}, {v.PendingCount, v.PendingPoints}, {v.RevokedCount, v.RevokedPoints}} {
		c, _ := new(big.Int).SetString(pair[0], 10)
		p, _ := new(big.Int).SetString(pair[1], 10)
		counts, points = append(counts, c), append(points, p)
		if (c.Sign() == 0) != (p.Sign() == 0) || c.Cmp(p) > 0 {
			return false
		}
	}
	stateCount := new(big.Int).Add(new(big.Int).Add(counts[0], counts[1]), counts[2])
	statePoints := new(big.Int).Add(new(big.Int).Add(points[0], points[1]), points[2])
	return stateCount.Cmp(count) == 0 && statePoints.Cmp(original) == 0 && (count.Sign() == 0) == (original.Sign() == 0) && count.Cmp(original) <= 0
}

func validRewardGroup(q RewardQuery, key, label, timezone string, states bool) bool {
	if key == "" || label != key {
		return false
	}
	if q.GroupBy == "day" {
		t, err := time.Parse("2006-01-02", key)
		loc, locErr := time.LoadLocation(timezone)
		if err != nil || locErr != nil || t.Format("2006-01-02") != key {
			return false
		}
		first := q.From.In(loc).Format("2006-01-02")
		last := q.To.Add(-time.Nanosecond).In(loc).Format("2006-01-02")
		return key >= first && key <= last
	}
	if states && q.GroupBy == "state" {
		return key == "granted" || key == "revocation_pending" || key == "revoked"
	}
	if !uuid.MatchString(key) || key != strings.ToLower(key) {
		return false
	}
	if q.GroupBy == "member" && q.MemberID != nil && key != strings.ToLower(*q.MemberID) ||
		q.GroupBy == "order" && q.OrderID != nil && key != strings.ToLower(*q.OrderID) {
		return false
	}
	return true
}

func validRewardGroupTotals(totals RewardTotals) bool {
	return validRewardTotals(totals) && totals.EntryCount != "0"
}

func validRewardOrderGroup(q RewardQuery, key string, totals RewardOrderTotals) bool {
	if !validRewardOrderTotals(totals) || totals.OrderCount == "0" {
		return false
	}
	if q.GroupBy != "state" {
		return true
	}
	switch key {
	case "granted":
		return totals.GrantedCount != "0" && totals.PendingCount == "0" && totals.RevokedCount == "0"
	case "revocation_pending":
		return totals.PendingCount != "0" && totals.GrantedCount == "0" && totals.RevokedCount == "0"
	case "revoked":
		return totals.RevokedCount != "0" && totals.GrantedCount == "0" && totals.PendingCount == "0"
	default:
		return false
	}
}

func rewardCSVMeta(brand string, snapshot time.Time, timezone string, q RewardQuery, kind string, total string, itemCount int) ([]string, error) {
	if !uuid.MatchString(brand) || snapshot.IsZero() || snapshot.Year() < 1 || snapshot.Year() > 9999 || q.Validate(kind) != nil || q.Offset != 0 || timezone == "" || timezone == "Local" || total != fmt.Sprint(itemCount) {
		return nil, ErrInvalid
	}
	if itemCount > ExportGroupLimit {
		return nil, ErrExportTooLarge
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return nil, ErrInvalid
	}
	member, order := "", ""
	if q.MemberID != nil {
		member = *q.MemberID
	}
	if q.OrderID != nil {
		order = *q.OrderID
	}
	meta := []string{"", brand, snapshot.UTC().Format(time.RFC3339Nano), timezone, q.From.UTC().Format(time.RFC3339Nano), q.To.UTC().Format(time.RFC3339Nano), q.GroupBy, member, order, "", ""}
	for _, value := range meta {
		if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return nil, ErrInvalid
		}
	}
	return meta, nil
}

func rewardCSVWrite(meta []string, headers []string, summary []string, rows []struct {
	key, label string
	values     []string
}) ([]byte, error) {
	target := &cappedCSV{}
	if _, err := target.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return nil, err
	}
	w := csv.NewWriter(target)
	if err := w.Write(headers); err != nil {
		return nil, err
	}
	summary = append([]string{}, summary...)
	if len(summary) > 0 && strings.HasPrefix(summary[len(summary)-1], "-") {
		summary[len(summary)-1] = "'" + summary[len(summary)-1]
	}
	write := func(record, key, label string, values []string) error {
		clean := make([]string, 3)
		for i, raw := range []string{record, key, label} {
			v, err := csvText(raw)
			if err != nil {
				return err
			}
			clean[i] = v
		}
		row := append([]string{}, meta...)
		row[0], row[9], row[10] = clean[0], clean[1], clean[2]
		return w.Write(append(row, values...))
	}
	if err := write("summary", "", "", summary); err != nil {
		return nil, err
	}
	for _, row := range rows {
		values := append([]string{}, row.values...)
		if len(values) > 0 && strings.HasPrefix(values[len(values)-1], "-") {
			values[len(values)-1] = "'" + values[len(values)-1]
		}
		if err := write("group", row.key, row.label, values); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		if errors.Is(err, ErrExportTooLarge) {
			return nil, ErrExportTooLarge
		}
		return nil, err
	}
	return target.data.Bytes(), nil
}

// RewardCSV emits a complete, stable export of immutable reward postings.
func RewardCSV(r RewardReport) ([]byte, error) {
	q := r.Query
	if len(r.Items) > ExportGroupLimit {
		return nil, ErrExportTooLarge
	}
	meta, err := rewardCSVMeta(r.BrandID, r.SnapshotAt, r.Timezone, q, "rewards", r.TotalGroups, len(r.Items))
	if err != nil {
		return nil, err
	}
	if !validRewardTotals(r.Summary) {
		return nil, ErrInvalid
	}
	sums := make([]big.Int, len(rewardCSVValues(r.Summary)))
	rows := make([]struct {
		key, label string
		values     []string
	}, 0, len(r.Items))
	last := ""
	for i, item := range r.Items {
		if !validRewardGroup(q, item.Key, item.Label, r.Timezone, false) || i > 0 && item.Key <= last || !validRewardGroupTotals(item.Totals) {
			return nil, ErrInvalid
		}
		last = item.Key
		values := rewardCSVValues(item.Totals)
		for j, value := range values {
			n, _ := new(big.Int).SetString(value, 10)
			sums[j].Add(&sums[j], n)
		}
		rows = append(rows, struct {
			key, label string
			values     []string
		}{item.Key, item.Label, values})
	}
	for i, value := range rewardCSVValues(r.Summary) {
		if sums[i].String() != value {
			return nil, ErrInvalid
		}
	}
	return rewardCSVWrite(meta, rewardCSVFields, rewardCSVValues(r.Summary), rows)
}

// RewardOrdersCSV emits every current-state order group in the created-time cohort.
func RewardOrdersCSV(r RewardOrderReport) ([]byte, error) {
	q := r.Query
	if len(r.Items) > ExportGroupLimit {
		return nil, ErrExportTooLarge
	}
	meta, err := rewardCSVMeta(r.BrandID, r.SnapshotAt, r.Timezone, q, "reward_orders", r.TotalGroups, len(r.Items))
	if err != nil {
		return nil, err
	}
	if !validRewardOrderTotals(r.Summary) {
		return nil, ErrInvalid
	}
	sums := make([]big.Int, len(rewardOrdersCSVValues(r.Summary)))
	rows := make([]struct {
		key, label string
		values     []string
	}, 0, len(r.Items))
	last := ""
	for i, item := range r.Items {
		if !validRewardGroup(q, item.Key, item.Label, r.Timezone, true) || i > 0 && item.Key <= last || !validRewardOrderGroup(q, item.Key, item.Totals) {
			return nil, ErrInvalid
		}
		last = item.Key
		values := rewardOrdersCSVValues(item.Totals)
		for j, value := range values {
			n, _ := new(big.Int).SetString(value, 10)
			sums[j].Add(&sums[j], n)
		}
		rows = append(rows, struct {
			key, label string
			values     []string
		}{item.Key, item.Label, values})
	}
	for i, value := range rewardOrdersCSVValues(r.Summary) {
		if sums[i].String() != value {
			return nil, ErrInvalid
		}
	}
	return rewardCSVWrite(meta, rewardOrdersCSVFields, rewardOrdersCSVValues(r.Summary), rows)
}
