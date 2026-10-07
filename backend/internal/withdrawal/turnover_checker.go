package withdrawal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"strconv"
	"strings"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrTurnoverBusy     = errors.New("turnover period is being changed; retry after settlement")
	ErrTurnoverEvidence = errors.New("withdrawal turnover evidence is incomplete or invalid")
)

// TurnoverChecker reads only authoritative, final settlement facts. It must be
// called in the application transaction while the member's wallet is locked.
// Policy, identity, compliance and available-source checks remain OrderService
// responsibilities. This checker never moves points or changes cycle cutoffs.
type TurnoverChecker struct{}

const turnoverWalletRange = `o.brand_id=$1 AND o.brand_member_id=$2 AND o.account_id=$3
 AND l.brand_id=o.brand_id AND l.account_id=o.account_id AND l.member_id=o.brand_member_id
 AND l.entry_type='bet' AND l.reference_type='bet_order' AND l.reference_id=o.id
 AND l.version>$4 AND l.version<=$5`

func (TurnoverChecker) Check(ctx context.Context, tx pgx.Tx, in EligibilityInput) (EligibilityDecision, error) {
	var empty EligibilityDecision
	if tx == nil || !validIDs(in.BrandID, in.MemberID, in.Wallet.AccountID) ||
		!strings.EqualFold(in.Wallet.BrandID, in.BrandID) || !strings.EqualFold(in.Wallet.MemberID, in.MemberID) ||
		in.CycleFromVersion < 0 || in.CutoffVersion != in.Wallet.Version || in.CutoffVersion < in.CycleFromVersion ||
		in.CutoffAt.IsZero() || (in.CycleFromAt == nil) != (in.CycleFromVersion == 0) ||
		(in.CycleFromAt != nil && in.CycleFromAt.After(in.CutoffAt)) {
		return empty, ErrInvalid
	}
	base, err := turnoverBaseSnapshot(in.Wallet)
	if err != nil || base != in.TurnoverBase {
		return empty, ErrInvalid
	}
	args := []any{strings.ToLower(in.BrandID), strings.ToLower(in.MemberID), in.Wallet.AccountID, in.CycleFromVersion, in.CutoffVersion}
	// Settlement and correction acquire period -> wallet. We already own the
	// wallet, so never WAIT for a period: NOWAIT makes contention roll back with
	// no reservation rather than introducing an inverse-order deadlock. Locks
	// also prevent a new correction from starting between the read and commit.
	rows, err := tx.Query(ctx, `SELECT p.id::text FROM periods p WHERE p.brand_id=$1
 AND EXISTS(SELECT 1 FROM bet_orders o JOIN point_ledger_entries l ON l.id=o.debit_entry_id
 WHERE o.period_id=p.id AND `+turnoverWalletRange+` AND o.status IN ('won','lost'))
 ORDER BY p.id FOR SHARE OF p NOWAIT`, args...)
	if err != nil {
		return empty, turnoverReadError(err)
	}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return empty, err
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return empty, turnoverReadError(err)
	}
	// Keep missing witnesses visible with LEFT JOIN: an otherwise final stake
	// without its snapshot/settlement evidence must fail closed, not disappear
	// from a successful qualification. Partial/unsettled/cancelled generations
	// never contribute. Paid losses count too, including zero-prize settlements.
	rows, err = tx.Query(ctx, `SELECT o.id::text,o.game_id::text,o.period_id::text,o.rule_version_id::text,
 o.total_points,l.version,o.withdrawal_rule_snapshot,
 c.id::text,j.id::text,coalesce(j.state='completed' AND j.draw_result_id=p.draw_result_id
 AND t.state='paid' AND t.calculation_id=c.id
 AND c.job_id=j.id AND c.period_id=o.period_id AND c.order_id=o.id
 AND c.definition_hash=o.definition_hash AND c.draw_hash=d.result_hash
 AND c.won=(o.status='won') AND c.prize_points=o.prize_points
 AND t.payout_entry_id IS NOT DISTINCT FROM o.payout_entry_id,false)
 FROM bet_orders o JOIN point_ledger_entries l ON l.id=o.debit_entry_id
 JOIN periods p ON p.brand_id=o.brand_id AND p.id=o.period_id
 LEFT JOIN settlement_jobs j ON j.brand_id=o.brand_id AND j.id=p.current_settlement_job_id
 LEFT JOIN settlement_calculations c ON c.brand_id=o.brand_id AND c.id=o.settlement_calculation_id
 LEFT JOIN settlement_targets t ON t.brand_id=o.brand_id AND t.job_id=j.id AND t.order_id=o.id
 LEFT JOIN draw_results d ON d.brand_id=o.brand_id AND d.id=p.draw_result_id
 WHERE `+turnoverWalletRange+` AND o.status IN ('won','lost') AND o.refund_entry_id IS NULL
 AND p.status='settled'
 AND NOT EXISTS(SELECT 1 FROM draw_corrections dc WHERE dc.brand_id=o.brand_id AND dc.period_id=p.id AND dc.state<>'completed')
 ORDER BY l.version,o.id`, args...)
	if err != nil {
		return empty, err
	}
	defer rows.Close()
	credit := new(big.Rat)
	validPoints := new(big.Int)
	count := new(big.Int)
	digest := sha256.New()
	for rows.Next() {
		var order, game, period, rule string
		var amount, version int64
		var raw []byte
		var calculation, job *string
		var valid bool
		if err = rows.Scan(&order, &game, &period, &rule, &amount, &version, &raw, &calculation, &job, &valid); err != nil {
			return empty, err
		}
		var snapshot betTurnoverSnapshot
		if !valid || calculation == nil || job == nil || amount <= 0 || decodeBetTurnoverSnapshot(raw, &snapshot) != nil {
			return empty, ErrTurnoverEvidence
		}
		if err = addTurnoverCredit(credit, TurnoverContribution{ValidPoints: points.Amount(amount), Multiple: snapshot.Multiple}); err != nil {
			return empty, ErrTurnoverEvidence
		}
		validPoints.Add(validPoints, big.NewInt(amount))
		count.Add(count, big.NewInt(1))
		// Stable, delimited immutable IDs and exact strings bind the summary to
		// its original bet/rule/ledger and settlement generation. Neither current
		// policy values nor mutable wallet balances enter this digest.
		line, err := json.Marshal([]string{order, game, period, rule, strconv.FormatInt(amount, 10), strconv.FormatInt(version, 10), snapshot.Multiple, snapshot.Source, snapshot.BrandVersion, snapshot.GameVersion, snapshot.BrandRevisionID, snapshot.GameRevisionID, *calculation, *job})
		if err != nil {
			return empty, err
		}
		_, _ = digest.Write(line)
		_, _ = digest.Write([]byte{'\n'})
	}
	if err = rows.Err(); err != nil {
		return empty, err
	}
	result := turnoverCreditResult(base.Points, credit)
	evidence, err := json.Marshal(map[string]any{"turnover_qualification": map[string]any{
		"schema_version": 1, "algorithm": "bet_snapshot_credit_v1", "valid_order_count": count.String(),
		"valid_points": validPoints.String(), "credit_numerator": result.Numerator, "credit_denominator": result.Denominator,
		"cycle_from_version": strconv.FormatInt(in.CycleFromVersion, 10), "cutoff_version": strconv.FormatInt(in.CutoffVersion, 10),
		"order_snapshot_digest": hex.EncodeToString(digest.Sum(nil)),
	}})
	return EligibilityDecision{Allowed: result.MeetsThreshold, Evidence: evidence}, err
}

func turnoverReadError(err error) error {
	var pgerr *pgconn.PgError
	if errors.As(err, &pgerr) && pgerr.Code == "55P03" {
		return ErrTurnoverBusy
	}
	return err
}

type betTurnoverSnapshot struct {
	SchemaVersion   int    `json:"schema_version"`
	Multiple        string `json:"multiple"`
	Source          string `json:"source"`
	BrandVersion    string `json:"brand_version"`
	GameVersion     string `json:"game_version"`
	BrandRevisionID string `json:"brand_revision_id"`
	GameRevisionID  string `json:"game_revision_id"`
}

func decodeBetTurnoverSnapshot(raw []byte, out *betTurnoverSnapshot) error {
	fields, err := fields(raw, "schema_version", "multiple", "source", "brand_version", "game_version", "brand_revision_id", "game_revision_id")
	if err != nil || len(fields) != 7 || json.Unmarshal(raw, out) != nil || out.SchemaVersion != 1 ||
		!ValidMultiple(out.Multiple) || out.Source != "brand" && out.Source != "game" ||
		!validIDs(out.BrandRevisionID, out.GameRevisionID) {
		return ErrTurnoverEvidence
	}
	for _, version := range []string{out.BrandVersion, out.GameVersion} {
		value, err := strconv.ParseInt(version, 10, 64)
		if err != nil || value < 1 || strconv.FormatInt(value, 10) != version {
			return ErrTurnoverEvidence
		}
	}
	return nil
}
