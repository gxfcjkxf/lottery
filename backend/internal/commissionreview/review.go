// Package commissionreview inspects legacy commission histories offline.
// Reports are observations, never approval tokens or financial repair plans.
package commissionreview

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/reporting"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalid    = errors.New("invalid commission history review")
	ErrCheckpoint = errors.New("commission history review requires explicit 0074 offline preparation")
	ErrNotFound   = errors.New("commission history review cycle not found")
	ErrTooLarge   = errors.New("commission history review exceeds complete report capacity")
	canonicalUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

const checkpoint74 = "0074_commission_analysis_evidence.up.sql"
const paymentLimit = 1000

type Payment struct {
	ID               string `json:"id"`
	RunID            string `json:"run_id"`
	EvidenceEpoch    string `json:"evidence_epoch"`
	Version          string `json:"version"`
	State            string `json:"state"`
	PayoutMode       string `json:"payout_mode"`
	TotalPoints      string `json:"total_points"`
	PaidPoints       string `json:"paid_points"`
	PaidCount        string `json:"paid_count"`
	TargetCount      string `json:"target_count"`
	HasMoneyEvidence bool   `json:"has_money_evidence"`
}

type Report struct {
	FormatVersion          int          `json:"format_version"`
	ReviewOnly             bool         `json:"review_only"`
	ExecutionAuthorized    bool         `json:"execution_authorized"`
	MigrationCheckpoint    string       `json:"migration_checkpoint"`
	BrandID                string       `json:"brand_id"`
	CycleID                string       `json:"cycle_id"`
	SnapshotAt             time.Time    `json:"snapshot_at"`
	CycleState             string       `json:"cycle_state"`
	CurrentRunID           *string      `json:"current_run_id"`
	EvidenceEpoch          string       `json:"evidence_epoch"`
	PaymentCount           string       `json:"payment_count"`
	StaleMoneyPaymentCount string       `json:"stale_money_payment_count"`
	Payments               []Payment    `json:"payments"`
	Analysis               FullAnalysis `json:"analysis"`
}

// The offline full-filter report is not a paginated HTTP DTO. In particular,
// it may contain more than 100 beneficiaries and exposes no misleading limit.
type FullAnalysis struct {
	Coverage           reporting.CommissionAnalysisCoverage                  `json:"coverage"`
	Summary            reporting.CommissionAnalysisTotals                    `json:"summary"`
	Items              []reporting.Group[reporting.CommissionAnalysisTotals] `json:"beneficiaries"`
	TotalBeneficiaries string                                                `json:"total_beneficiaries"`
}

// Inspect selects an entire saved cycle, including stale original payments
// and all later payments. All migration checks, states and validated amounts
// share one primary REPEATABLE READ / READ ONLY snapshot. Unknown evidence is
// retained as null; damaged ledger/source evidence releases no partial report.
func Inspect(ctx context.Context, db *pgxpool.Pool, brandID, cycleID string) (Report, error) {
	if ctx == nil || db == nil || !canonicalUUID.MatchString(brandID) || !canonicalUUID.MatchString(cycleID) {
		return Report{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Report{}, err
	}
	defer tx.Rollback(ctx)
	out, err := InspectTx(ctx, tx, brandID, cycleID)
	if err != nil {
		return Report{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Report{}, err
	}
	return out, nil
}

// InspectTx performs the complete legacy review in the caller's transaction.
// It never begins, commits, or rolls back the transaction. The caller owns
// isolation and any row locks needed to serialize a subsequent decision.
func InspectTx(ctx context.Context, tx pgx.Tx, brandID, cycleID string) (Report, error) {
	if ctx == nil || tx == nil || !canonicalUUID.MatchString(brandID) || !canonicalUUID.MatchString(cycleID) {
		return Report{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	checkpoint, err := database.CheckCommissionReviewMigrationsTx(ctx, tx)
	if err != nil {
		return Report{}, err
	}
	if checkpoint != checkpoint74 {
		return Report{}, ErrCheckpoint
	}
	var schema string
	if err = tx.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		return Report{}, err
	}
	if _, err = tx.Exec(ctx, `SET LOCAL search_path TO pg_catalog, `+pgx.Identifier{schema}.Sanitize()+`, pg_temp`); err != nil {
		return Report{}, err
	}
	if _, err = tx.Exec(ctx, `SET LOCAL statement_timeout='15s'`); err != nil {
		return Report{}, err
	}
	if _, err = tx.Exec(ctx, `SET LOCAL TIME ZONE 'UTC'`); err != nil {
		return Report{}, err
	}
	out := Report{FormatVersion: 1, ReviewOnly: true, MigrationCheckpoint: checkpoint, BrandID: brandID, CycleID: cycleID, Payments: []Payment{}}
	var end time.Time
	err = tx.QueryRow(ctx, `SELECT state,current_run_id::text,evidence_epoch::text,window_to FROM commission_cycles WHERE brand_id=$1 AND id=$2`, brandID, cycleID).Scan(&out.CycleState, &out.CurrentRunID, &out.EvidenceEpoch, &end)
	if errors.Is(err, pgx.ErrNoRows) {
		return Report{}, ErrNotFound
	}
	if err != nil {
		return Report{}, err
	}
	rows, err := tx.Query(ctx, paymentSQL, brandID, cycleID, paymentLimit+1)
	if err != nil {
		return Report{}, err
	}
	stale := 0
	for rows.Next() {
		var p Payment
		if err = rows.Scan(&p.ID, &p.RunID, &p.EvidenceEpoch, &p.Version, &p.State, &p.PayoutMode, &p.TotalPoints, &p.PaidPoints, &p.PaidCount, &p.TargetCount, &p.HasMoneyEvidence); err != nil {
			rows.Close()
			return Report{}, err
		}
		out.Payments = append(out.Payments, p)
		if p.State == "stale" && p.HasMoneyEvidence {
			stale++
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return Report{}, err
	}
	if len(out.Payments) > paymentLimit {
		return Report{}, ErrTooLarge
	}
	out.PaymentCount, out.StaleMoneyPaymentCount = strconv.Itoa(len(out.Payments)), strconv.Itoa(stale)
	// A microsecond window includes exactly this saved window_to. The cycle
	// filter—not posting time—binds the cohort; export returns ALL beneficiaries.
	query := reporting.CommissionAnalysisQuery{From: end.UTC(), To: end.UTC().Add(time.Microsecond), GroupBy: "agent", Limit: 100, CycleID: &cycleID}
	analysis, err := (reporting.Service{}).CommissionAnalysisExport(ctx, tx, brandID, query)
	if err != nil {
		return Report{}, err
	}
	// Reuse full-filter CSV validation to prove exact group/summary arithmetic
	// before returning JSON; no file or audit is written by this offline read.
	if _, err = reporting.CommissionAnalysisCSV(analysis); err != nil {
		return Report{}, err
	}
	out.Analysis = FullAnalysis{Coverage: analysis.Coverage, Summary: analysis.Summary, Items: analysis.Items, TotalBeneficiaries: analysis.TotalGroups}
	out.SnapshotAt = analysis.SnapshotAt
	return out, nil
}

// This is the 0075 preflight's money-evidence detector expressed without its
// not-yet-installed function. Suspicious pointers/reverse bindings are not
// proof of payment: the independent complete analysis must also validate them.
const paymentSQL = `SELECT p.id::text,p.run_id::text,p.evidence_epoch::text,p.version::text,p.state,p.payout_mode,p.total_points::text,
(SELECT coalesce(sum(t.points),0)::text FROM commission_payment_targets t WHERE t.brand_id=p.brand_id AND t.payment_id=p.id AND t.state='paid'),
(SELECT count(*)::text FROM commission_payment_targets t WHERE t.brand_id=p.brand_id AND t.payment_id=p.id AND t.state='paid'),p.target_count::text,
 EXISTS(SELECT 1 FROM commission_payment_targets t WHERE t.brand_id=p.brand_id AND t.payment_id=p.id AND
 (t.ledger_entry_id IS NOT NULL OR EXISTS(SELECT 1 FROM point_ledger_entries l WHERE l.brand_id=p.brand_id AND l.reference_type='commission_payment_target' AND l.reference_id=t.id)))
 OR EXISTS(SELECT 1 FROM commission_adjustments a WHERE a.brand_id=p.brand_id AND a.payment_id=p.id AND
 (a.ledger_entry_id IS NOT NULL OR EXISTS(SELECT 1 FROM point_ledger_entries l WHERE l.brand_id=p.brand_id AND l.reference_type='commission_adjustment' AND l.reference_id=a.id)))
 OR EXISTS(SELECT 1 FROM commission_correction_plans cp JOIN commission_correction_executions x ON x.brand_id=cp.brand_id AND x.plan_id=cp.id
 JOIN commission_correction_execution_targets t ON t.brand_id=x.brand_id AND t.execution_id=x.id
 WHERE cp.brand_id=p.brand_id AND cp.payment_id=p.id AND t.state='applied' AND t.ledger_entry_id IS NOT NULL)
FROM commission_payments p WHERE p.brand_id=$1 AND p.cycle_id=$2 ORDER BY p.id LIMIT $3`
