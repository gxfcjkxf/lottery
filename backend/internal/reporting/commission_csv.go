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

var commissionCSVFields = []string{
	"record_type", "brand_id", "snapshot_at", "timezone", "from", "to", "group_by",
	"agent_id", "member_id", "cycle_id", "key", "label",
	"entry_count", "paid_entry_count", "paid_points", "adjustment_entry_count",
	"adjustment_credit_points", "adjustment_debit_points", "correction_entry_count",
	"correction_credit_points", "correction_debit_points", "net_points",
}

func commissionCSVValues(v CommissionTotals) []string {
	return []string{v.EntryCount, v.PaidEntryCount, v.PaidPoints, v.AdjustmentEntryCount,
		v.AdjustmentCreditPoints, v.AdjustmentDebitPoints, v.CorrectionEntryCount,
		v.CorrectionCreditPoints, v.CorrectionDebitPoints, v.NetPoints}
}

func validCommissionTotals(v CommissionTotals) bool {
	values := commissionCSVValues(v)
	if decimalFields(values, len(values)-1) != nil {
		return false
	}
	entries, _ := new(big.Int).SetString(v.EntryCount, 10)
	paidEntries, _ := new(big.Int).SetString(v.PaidEntryCount, 10)
	adjustments, _ := new(big.Int).SetString(v.AdjustmentEntryCount, 10)
	corrections, _ := new(big.Int).SetString(v.CorrectionEntryCount, 10)
	if entries.Cmp(new(big.Int).Add(new(big.Int).Add(paidEntries, adjustments), corrections)) != 0 {
		return false
	}
	paid, _ := new(big.Int).SetString(v.PaidPoints, 10)
	credit, _ := new(big.Int).SetString(v.AdjustmentCreditPoints, 10)
	debit, _ := new(big.Int).SetString(v.AdjustmentDebitPoints, 10)
	correctionCredit, _ := new(big.Int).SetString(v.CorrectionCreditPoints, 10)
	correctionDebit, _ := new(big.Int).SetString(v.CorrectionDebitPoints, 10)
	net, _ := new(big.Int).SetString(v.NetPoints, 10)
	return net.Cmp(new(big.Int).Sub(new(big.Int).Add(new(big.Int).Add(paid, credit), correctionCredit), new(big.Int).Add(debit, correctionDebit))) == 0
}

func validCommissionGroup(q CommissionQuery, item Group[CommissionTotals]) bool {
	if item.Key == "" || item.Label != item.Key {
		return false
	}
	if q.GroupBy == "day" {
		t, err := time.Parse("2006-01-02", item.Key)
		return err == nil && t.Format("2006-01-02") == item.Key
	}
	return uuid.MatchString(item.Key)
}

// CommissionCSV exports every filtered ledger-posting group in stable key
// order. Exact point totals remain decimal text and net_points may be negative.
func CommissionCSV(r CommissionReport) ([]byte, error) {
	q := r.Query
	if len(r.Items) > ExportGroupLimit {
		return nil, ErrExportTooLarge
	}
	if !uuid.MatchString(r.BrandID) || r.SnapshotAt.IsZero() || q.Validate() != nil || q.Offset != 0 ||
		r.Timezone == "" || r.Timezone == "Local" || r.TotalGroups != fmt.Sprint(len(r.Items)) {
		return nil, ErrInvalid
	}
	if _, err := time.LoadLocation(r.Timezone); err != nil {
		return nil, ErrInvalid
	}
	if !validCommissionTotals(r.Summary) {
		return nil, ErrInvalid
	}
	filters := []string{"", "", ""}
	for i, value := range []*string{q.AgentID, q.MemberID, q.CycleID} {
		if value != nil {
			filters[i] = *value
		}
	}
	meta := []string{r.BrandID, r.SnapshotAt.UTC().Format(time.RFC3339Nano), r.Timezone,
		q.From.UTC().Format(time.RFC3339Nano), q.To.UTC().Format(time.RFC3339Nano), q.GroupBy}
	for _, value := range append(append([]string{}, meta...), filters...) {
		if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return nil, ErrInvalid
		}
	}
	target := &cappedCSV{}
	if _, err := target.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return nil, err
	}
	w := csv.NewWriter(target)
	if err := w.Write(commissionCSVFields); err != nil {
		return nil, err
	}
	write := func(record, key, label string, totals CommissionTotals) error {
		clean := make([]string, 3)
		for i, raw := range []string{record, key, label} {
			v, err := csvText(raw)
			if err != nil {
				return err
			}
			clean[i] = v
		}
		values := commissionCSVValues(totals)
		if strings.HasPrefix(values[len(values)-1], "-") {
			values[len(values)-1] = "'" + values[len(values)-1]
		}
		row := []string{clean[0], meta[0], meta[1], meta[2], meta[3], meta[4], meta[5],
			filters[0], filters[1], filters[2], clean[1], clean[2]}
		return w.Write(append(row, values...))
	}
	if err := write("summary", "", "", r.Summary); err != nil {
		return nil, err
	}
	sums := make([]big.Int, len(commissionCSVValues(r.Summary)))
	last := ""
	for i, item := range r.Items {
		if !validCommissionGroup(q, item) || i > 0 && item.Key <= last || !validCommissionTotals(item.Totals) {
			return nil, ErrInvalid
		}
		last = item.Key
		for j, value := range commissionCSVValues(item.Totals) {
			n, ok := new(big.Int).SetString(value, 10)
			if !ok {
				return nil, ErrInvalid
			}
			sums[j].Add(&sums[j], n)
		}
		if err := write("group", item.Key, item.Label, item.Totals); err != nil {
			return nil, err
		}
	}
	for i, value := range commissionCSVValues(r.Summary) {
		if sums[i].String() != value {
			return nil, ErrInvalid
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
