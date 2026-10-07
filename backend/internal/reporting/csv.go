package reporting

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode"
	"unicode/utf8"
)

const CSVByteLimit = 4 * 1024 * 1024

var unsignedDecimal = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)
var signedDecimal = regexp.MustCompile(`^(0|-?[1-9][0-9]*)$`)
var commonCSVFields = []string{"record_type", "brand_id", "snapshot_at", "timezone", "from", "to", "group_by", "game_id", "member_id", "key", "label"}
var withdrawalCSVFields = []string{"record_type", "brand_id", "snapshot_at", "timezone", "from", "to", "group_by", "member_id", "key", "label", "order_count", "requested_points", "reviewing_count", "reviewing_points", "processing_count", "processing_points", "paid_count", "paid_points", "rejected_count", "rejected_points", "failed_count", "failed_points", "cancelled_count", "cancelled_points"}
var balanceCSVFields = []string{"account_count", "available_points", "frozen_points", "withdrawal_points", "total_points"}

type cappedCSV struct{ data bytes.Buffer }

func (w *cappedCSV) Write(p []byte) (int, error) {
	if len(p) > CSVByteLimit-w.data.Len() {
		return 0, ErrExportTooLarge
	}
	return w.data.Write(p)
}
func csvText(v string) (string, error) {
	if !utf8.ValidString(v) || strings.ContainsRune(v, 0) {
		return "", ErrInvalid
	}
	lead := strings.TrimLeftFunc(v, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) || r == '\uFEFF' })
	if lead != "" && strings.ContainsRune("=+-@", rune(lead[0])) {
		return "'" + v, nil
	}
	return v, nil
}
func decimalFields(values []string, signedIndex int) error {
	for i, v := range values {
		if i == signedIndex {
			if !signedDecimal.MatchString(v) {
				return ErrInvalid
			}
		} else if !unsignedDecimal.MatchString(v) {
			return ErrInvalid
		}
	}
	return nil
}
func bettingCSVValues(v BettingTotals) []string {
	return []string{v.OrderCount, v.StakePoints, v.PlacedCount, v.WonCount, v.LostCount, v.AbnormalCount, v.CancelledCount, v.RefundPoints, v.SettledStakePoints, v.UnfinalizedStakePoints, v.AbnormalStakePoints, v.CurrentPrizePoints, v.CorrectionOpenCount}
}
func ledgerCSVValues(v LedgerTotals) []string {
	return []string{v.EntryCount, v.NetPoints, v.RechargePoints, v.PrizeCreditPoints, v.PrizeReversalPoints, v.RefundPoints}
}
func balanceCSVValues(v Balances) []string {
	return []string{v.AccountCount, v.AvailablePoints, v.FrozenPoints, v.WithdrawalPoints, v.TotalPoints}
}
func metadataCSV[T any](r Report[T], kind string) ([]string, error) {
	if !uuid.MatchString(r.BrandID) || r.SnapshotAt.IsZero() || r.Query.Validate(kind) != nil || r.Query.Offset != 0 || r.Timezone == "" || r.TotalGroups != fmt.Sprint(len(r.Items)) {
		return nil, ErrInvalid
	}
	if len(r.Items) > ExportGroupLimit {
		return nil, ErrExportTooLarge
	}
	if r.Timezone == "Local" {
		return nil, ErrInvalid
	}
	if _, e := time.LoadLocation(r.Timezone); e != nil {
		return nil, ErrInvalid
	}
	game, member := "", ""
	if r.Query.GameID != nil {
		game = *r.Query.GameID
	}
	if r.Query.MemberID != nil {
		member = *r.Query.MemberID
	}
	meta := []string{"", r.BrandID, r.SnapshotAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), r.Timezone, r.Query.From.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), r.Query.To.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), r.Query.GroupBy, game, member, "", ""}
	for _, v := range meta {
		if !utf8.ValidString(v) || strings.ContainsRune(v, 0) {
			return nil, ErrInvalid
		}
	}
	return meta, nil
}
func encodeCSV[T any](r Report[T], kind string, fields []aggregate, values func(T) []string, balances *Balances) ([]byte, error) {
	meta, e := metadataCSV(r, kind)
	if e != nil {
		return nil, e
	}
	signed := -1
	if kind == "ledger" {
		signed = 1
	}
	if e = decimalFields(values(r.Summary), signed); e != nil {
		return nil, e
	}
	if balances != nil {
		if e = decimalFields(balanceCSVValues(*balances), -1); e != nil {
			return nil, e
		}
	}
	target := &cappedCSV{}
	if _, e = target.Write([]byte{0xEF, 0xBB, 0xBF}); e != nil {
		return nil, e
	}
	writer := csv.NewWriter(target)
	header := append([]string{}, commonCSVFields...)
	for _, f := range fields {
		header = append(header, f.name)
	}
	balanceCols := 0
	if balances != nil {
		header = append(header, balanceCSVFields...)
		balanceCols = len(balanceCSVFields)
	}
	if e = writer.Write(header); e != nil {
		return nil, e
	}
	summary := append([]string{}, meta...)
	summary[0] = "summary"
	summary = append(summary, values(r.Summary)...)
	summary = append(summary, make([]string, balanceCols)...)
	if e = writer.Write(summary); e != nil {
		return nil, e
	}
	if balances != nil {
		row := append([]string{}, meta...)
		row[0] = "balances"
		row = append(row, make([]string, len(fields))...)
		row = append(row, balanceCSVValues(*balances)...)
		if e = writer.Write(row); e != nil {
			return nil, e
		}
	}
	last := ""
	sums := make([]big.Int, len(fields))
	for i, item := range r.Items {
		if item.Key == "" || i > 0 && item.Key <= last {
			return nil, ErrInvalid
		}
		last = item.Key
		if e = decimalFields(values(item.Totals), signed); e != nil {
			return nil, e
		}
		for j, value := range values(item.Totals) {
			n, ok := new(big.Int).SetString(value, 10)
			if !ok {
				return nil, ErrInvalid
			}
			sums[j].Add(&sums[j], n)
		}
		key, e := csvText(item.Key)
		if e != nil {
			return nil, e
		}
		label, e := csvText(item.Label)
		if e != nil {
			return nil, e
		}
		row := append([]string{}, meta...)
		row[0], row[9], row[10] = "group", key, label
		row = append(row, values(item.Totals)...)
		row = append(row, make([]string, balanceCols)...)
		if e = writer.Write(row); e != nil {
			return nil, e
		}
	}
	for i, value := range values(r.Summary) {
		if sums[i].String() != value {
			return nil, ErrInvalid
		}
	}
	if balances != nil {
		a, _ := new(big.Int).SetString(balances.AvailablePoints, 10)
		f, _ := new(big.Int).SetString(balances.FrozenPoints, 10)
		v, _ := new(big.Int).SetString(balances.WithdrawalPoints, 10)
		a.Add(a, f).Add(a, v)
		if a.String() != balances.TotalPoints {
			return nil, ErrInvalid
		}
	}
	writer.Flush()
	if e = writer.Error(); e != nil {
		if errors.Is(e, ErrExportTooLarge) {
			return nil, ErrExportTooLarge
		}
		return nil, e
	}
	return target.data.Bytes(), nil
}

// Text fields are formula-neutralized; exact numeric cells are not converted to
// floats or disguised as formulas. Spreadsheet applications must import them as
// TEXT to avoid automatic 15-digit rounding; CSV itself retains every digit.
func BettingCSV(r Report[BettingTotals]) ([]byte, error) {
	return encodeCSV(r, "betting", betAggregates, bettingCSVValues, nil)
}
func LedgerCSV(r LedgerReport) ([]byte, error) {
	return encodeCSV(r.Report, "ledger", ledgerAggregates, ledgerCSVValues, &r.Balances)
}

func withdrawalCSVValues(v WithdrawalTotals) []string {
	return []string{v.OrderCount, v.RequestedPoints, v.ReviewingCount, v.ReviewingPoints, v.ProcessingCount, v.ProcessingPoints,
		v.PaidCount, v.PaidPoints, v.RejectedCount, v.RejectedPoints, v.FailedCount, v.FailedPoints, v.CancelledCount, v.CancelledPoints}
}

func validWithdrawalTotals(v WithdrawalTotals) bool {
	values := withdrawalCSVValues(v)
	if decimalFields(values, -1) != nil {
		return false
	}
	counts, points := new(big.Int), new(big.Int)
	for _, state := range []string{"reviewing", "processing", "paid", "rejected", "failed", "cancelled"} {
		count, _ := new(big.Int).SetString(withdrawalValue(v, state+"_count"), 10)
		amount, _ := new(big.Int).SetString(withdrawalValue(v, state+"_points"), 10)
		counts.Add(counts, count)
		points.Add(points, amount)
	}
	return counts.String() == v.OrderCount && points.String() == v.RequestedPoints
}

func withdrawalValue(v WithdrawalTotals, field string) string {
	switch field {
	case "reviewing_count":
		return v.ReviewingCount
	case "reviewing_points":
		return v.ReviewingPoints
	case "processing_count":
		return v.ProcessingCount
	case "processing_points":
		return v.ProcessingPoints
	case "paid_count":
		return v.PaidCount
	case "paid_points":
		return v.PaidPoints
	case "rejected_count":
		return v.RejectedCount
	case "rejected_points":
		return v.RejectedPoints
	case "failed_count":
		return v.FailedCount
	case "failed_points":
		return v.FailedPoints
	case "cancelled_count":
		return v.CancelledCount
	case "cancelled_points":
		return v.CancelledPoints
	default:
		return ""
	}
}

// WithdrawalCSV exports application-time cohorts with the current state
// projection. In particular, paid_points is not a transfer-time measure.
func WithdrawalCSV(r WithdrawalReport) ([]byte, error) {
	q := r.Query
	if !uuid.MatchString(r.BrandID) || r.SnapshotAt.IsZero() || q.Validate("withdrawal") != nil || q.Offset != 0 || r.Timezone == "" || r.TotalGroups != fmt.Sprint(len(r.Items)) || len(r.Items) > ExportGroupLimit {
		return nil, ErrInvalid
	}
	if r.Timezone == "Local" {
		return nil, ErrInvalid
	}
	if _, err := time.LoadLocation(r.Timezone); err != nil {
		return nil, ErrInvalid
	}
	if !validWithdrawalTotals(r.Summary) {
		return nil, ErrInvalid
	}
	meta := []string{r.BrandID, r.SnapshotAt.UTC().Format(time.RFC3339Nano), r.Timezone,
		q.From.UTC().Format(time.RFC3339Nano), q.To.UTC().Format(time.RFC3339Nano), q.GroupBy}
	for _, value := range meta {
		if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return nil, ErrInvalid
		}
	}
	target := &cappedCSV{}
	if _, err := target.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return nil, err
	}
	w := csv.NewWriter(target)
	if err := w.Write(withdrawalCSVFields); err != nil {
		return nil, err
	}
	write := func(record, member, key, label string, values []string) error {
		clean := make([]string, 4)
		for i, raw := range []string{member, key, label, record} {
			v, err := csvText(raw)
			if err != nil {
				return err
			}
			clean[i] = v
		}
		row := []string{clean[3], meta[0], meta[1], meta[2], meta[3], meta[4], meta[5], clean[0], clean[1], clean[2]}
		return w.Write(append(row, values...))
	}
	if err := write("summary", qMember(q), "", "", withdrawalCSVValues(r.Summary)); err != nil {
		return nil, err
	}
	last := ""
	sums := make([]big.Int, len(withdrawalCSVValues(r.Summary)))
	for i, item := range r.Items {
		if item.Key == "" || i > 0 && item.Key <= last {
			return nil, ErrInvalid
		}
		last = item.Key
		values := withdrawalCSVValues(item.Totals)
		if !validWithdrawalTotals(item.Totals) {
			return nil, ErrInvalid
		}
		for j, value := range values {
			n, ok := new(big.Int).SetString(value, 10)
			if !ok {
				return nil, ErrInvalid
			}
			sums[j].Add(&sums[j], n)
		}
		if err := write("group", qMember(q), item.Key, item.Label, values); err != nil {
			return nil, err
		}
	}
	for i, value := range withdrawalCSVValues(r.Summary) {
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

func qMember(q Query) string {
	if q.MemberID == nil {
		return ""
	}
	return *q.MemberID
}
