package withdrawal

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"strconv"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

// QualificationView is a point-in-time, member-owned turnover summary, not an
// application authorization or a reservation. It excludes private rule IDs,
// raw snapshots and digests; Create always recalculates under its wallet lock.
type QualificationView struct {
	BrandID           string        `json:"brand_id"`
	MemberID          string        `json:"member_id"`
	AccountID         string        `json:"account_id"`
	BasePoints        points.Amount `json:"base_points"`
	ValidPoints       string        `json:"valid_points"`
	ValidOrderCount   string        `json:"valid_order_count"`
	CreditNumerator   string        `json:"credit_numerator"`
	CreditDenominator string        `json:"credit_denominator"`
	MeetsTurnover     bool          `json:"meets_turnover"`
	CycleFromAt       *time.Time    `json:"cycle_from_at"`
	CycleFromVersion  string        `json:"cycle_from_version"`
	CutoffAt          time.Time     `json:"cutoff_at"`
	CutoffVersion     string        `json:"cutoff_version"`
}

func (s OrderService) QualificationTx(ctx context.Context, tx pgx.Tx, brand, member string) (QualificationView, error) {
	var out QualificationView
	if tx == nil || !validIDs(brand, member) {
		return out, ErrInvalid
	}
	if s.Eligibility == nil {
		return out, ErrEligibilityNotConfigured
	}
	available, err := s.AvailabilityTx(ctx, tx, brand, member)
	if err != nil {
		return out, err
	}
	if !available.CanApply {
		return out, ErrIneligible
	}
	policy, err := scanBrand(tx.QueryRow(ctx, `SELECT `+brandFields+` FROM brand_withdrawal_policies WHERE brand_id=$1 FOR SHARE`, brand))
	if err != nil {
		return out, err
	}
	// Shared wallet locking is sufficient for this read. NOWAIT period locks in
	// the checker remain essential: settlement acquires period then wallet.
	wallet, err := s.Points.Snapshot(ctx, tx, brand, member)
	if err != nil {
		return out, err
	}
	base, err := turnoverBaseSnapshot(wallet)
	if err != nil {
		return out, err
	}
	var cycle *time.Time
	var cycleVersion int64
	err = tx.QueryRow(ctx, `SELECT cutoff_at,cutoff_version FROM withdrawal_turnover_cycles WHERE brand_id=$1 AND member_id=$2`, brand, member).Scan(&cycle, &cycleVersion)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	var cutoff time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&cutoff); err != nil {
		return out, err
	}
	cutoff = cutoff.UTC()
	check := EligibilityInput{BrandID: brand, MemberID: member, Policy: policy, Wallet: wallet, TurnoverBase: base, CycleFromAt: cycle, CycleFromVersion: cycleVersion, CutoffAt: cutoff, CutoffVersion: wallet.Version}
	// The same configured server checker is used for preview and Create. Do not
	// infer a preview from a test adapter's opaque "allowed" flag.
	decision, err := s.Eligibility.Check(ctx, tx, check)
	if err != nil {
		return out, err
	}
	var evidence struct {
		Qualification struct {
			SchemaVersion int    `json:"schema_version"`
			Algorithm     string `json:"algorithm"`
			Count         string `json:"valid_order_count"`
			Points        string `json:"valid_points"`
			Numerator     string `json:"credit_numerator"`
			Denominator   string `json:"credit_denominator"`
			From          string `json:"cycle_from_version"`
			Cutoff        string `json:"cutoff_version"`
		} `json:"turnover_qualification"`
	}
	if len(decision.Evidence) > 16384 || json.Unmarshal(decision.Evidence, &evidence) != nil {
		return out, ErrTurnoverEvidence
	}
	q := evidence.Qualification
	if q.SchemaVersion != 1 || q.Algorithm != "bet_snapshot_credit_v1" || q.From != strconv.FormatInt(cycleVersion, 10) || q.Cutoff != strconv.FormatInt(wallet.Version, 10) {
		return out, ErrTurnoverEvidence
	}
	for _, value := range []string{q.Count, q.Points, q.Numerator, q.Denominator} {
		if _, ok := canonicalTurnoverInteger(value); !ok {
			return out, ErrTurnoverEvidence
		}
	}
	numerator, _ := canonicalTurnoverInteger(q.Numerator)
	denominator, _ := canonicalTurnoverInteger(q.Denominator)
	if denominator.Sign() == 0 || decision.Allowed != (numerator.Cmp(new(big.Int).Mul(big.NewInt(int64(base.Points)), denominator)) >= 0) {
		return out, ErrTurnoverEvidence
	}
	out = QualificationView{BrandID: brand, MemberID: member, AccountID: wallet.AccountID, BasePoints: base.Points, ValidPoints: q.Points, ValidOrderCount: q.Count, CreditNumerator: q.Numerator, CreditDenominator: q.Denominator, MeetsTurnover: decision.Allowed, CycleFromAt: cycle, CycleFromVersion: q.From, CutoffAt: cutoff, CutoffVersion: q.Cutoff}
	return out, nil
}

func canonicalTurnoverInteger(value string) (*big.Int, bool) {
	if len(value) == 0 || len(value) > 16384 || len(value) > 1 && value[0] == '0' {
		return nil, false
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return nil, false
		}
	}
	n, ok := new(big.Int).SetString(value, 10)
	return n, ok
}
