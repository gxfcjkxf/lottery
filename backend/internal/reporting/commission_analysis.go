package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrAnalysisIntegrity = errors.New("commission analysis source evidence is incomplete or invalid")

// Select entire saved cycles by their end, then observe all-time business
// postings for that cohort. A wallet balance is never an accrued-net source.
type CommissionAnalysisQuery CommissionQuery

func (q CommissionAnalysisQuery) Validate() error {
	if CommissionQuery(q).Validate() != nil || q.GroupBy != "cycle" && q.GroupBy != "agent" {
		return ErrInvalid
	}
	for _, id := range []*string{q.AgentID, q.MemberID, q.CycleID} {
		if id != nil && *id != strings.ToLower(*id) {
			return ErrInvalid
		}
	}
	return nil
}

type CommissionAnalysisCoverage struct {
	SelectedCycleCount string `json:"selected_cycle_count"`
	ReadyCycleCount    string `json:"ready_cycle_count"`
	UnreadyCycleCount  string `json:"unready_cycle_count"`
}

type CommissionAnalysisTotals struct {
	ObservedCalculatedPoints     string  `json:"observed_calculated_points"`
	CalculatedPoints             *string `json:"calculated_points"`
	PaidEntryCount               string  `json:"paid_entry_count"`
	PaidPoints                   string  `json:"paid_points"`
	AdjustmentEntryCount         string  `json:"adjustment_entry_count"`
	AdjustmentCreditPoints       string  `json:"adjustment_credit_points"`
	AdjustmentDebitPoints        string  `json:"adjustment_debit_points"`
	CorrectionEntryCount         string  `json:"correction_entry_count"`
	CorrectionCreditPoints       string  `json:"correction_credit_points"`
	CorrectionDebitPoints        string  `json:"correction_debit_points"`
	PostingEntryCount            string  `json:"posting_entry_count"`
	ActualNetPoints              string  `json:"actual_net_points"`
	ManualAdjustmentNetPoints    string  `json:"manual_adjustment_net_points"`
	EffectiveTargetPoints        *string `json:"effective_target_points"`
	CalculationMinusActualPoints *string `json:"calculation_minus_actual_points"`
	EffectiveMinusActualPoints   *string `json:"effective_minus_actual_points"`
	CalculationComplete          bool    `json:"calculation_complete"`
	EffectiveTargetComplete      bool    `json:"effective_target_complete"`
}

type CommissionAnalysisReport struct {
	BrandID     string                            `json:"brand_id"`
	SnapshotAt  time.Time                         `json:"snapshot_at"`
	Timezone    string                            `json:"timezone"`
	Query       CommissionAnalysisQuery           `json:"query"`
	Coverage    CommissionAnalysisCoverage        `json:"coverage"`
	Summary     CommissionAnalysisTotals          `json:"summary"`
	Items       []Group[CommissionAnalysisTotals] `json:"items"`
	TotalGroups string                            `json:"total_groups"`
}

func (s Service) CommissionAnalysis(ctx context.Context, brand string, q CommissionAnalysisQuery) (CommissionAnalysisReport, error) {
	if s.DB == nil {
		return CommissionAnalysisReport{}, ErrInvalid
	}
	return s.commissionAnalysis(ctx, s.DB, brand, q, q.Limit, q.Offset, false)
}
func (s Service) CommissionAnalysisRead(ctx context.Context, tx pgx.Tx, brand string, q CommissionAnalysisQuery) (CommissionAnalysisReport, error) {
	if tx == nil {
		return CommissionAnalysisReport{}, ErrInvalid
	}
	return s.commissionAnalysis(ctx, tx, brand, q, q.Limit, q.Offset, false)
}
func (s Service) CommissionAnalysisExport(ctx context.Context, tx pgx.Tx, brand string, q CommissionAnalysisQuery) (CommissionAnalysisReport, error) {
	if tx == nil || q.Offset != 0 {
		return CommissionAnalysisReport{}, ErrInvalid
	}
	return s.commissionAnalysis(ctx, tx, brand, q, ExportGroupLimit+1, 0, true)
}

func (s Service) commissionAnalysis(ctx context.Context, runner rowQuerier, brand string, q CommissionAnalysisQuery, limit, offset int, exporting bool) (CommissionAnalysisReport, error) {
	out := CommissionAnalysisReport{BrandID: brand, Query: q, Items: []Group[CommissionAnalysisTotals]{}}
	if runner == nil || !uuid.MatchString(brand) || brand != strings.ToLower(brand) || q.Validate() != nil {
		return out, ErrInvalid
	}
	q.From, q.To = q.From.UTC(), q.To.UTC()
	out.Query = q
	sql := commissionAnalysisSQL(q.GroupBy)
	var valid, integrity bool
	var coverage, summary, items []byte
	err := runner.QueryRow(ctx, sql, brand, databaseBound(q.From), databaseBound(q.To), q.AgentID, q.MemberID, q.CycleID, limit, offset).Scan(&out.SnapshotAt, &out.Timezone, &valid, &integrity, &coverage, &summary, &items, &out.TotalGroups)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !valid {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	if !integrity {
		return out, ErrAnalysisIntegrity
	}
	if exporting && groupCountExceeds(out.TotalGroups, ExportGroupLimit) {
		return out, ErrExportTooLarge
	}
	if err = json.Unmarshal(coverage, &out.Coverage); err == nil {
		err = json.Unmarshal(summary, &out.Summary)
	}
	if err == nil {
		err = json.Unmarshal(items, &out.Items)
	}
	if err != nil {
		return CommissionAnalysisReport{}, err
	}
	if !validCommissionAnalysisCoverage(out.Coverage) || !validCommissionAnalysisTotals(out.Summary) {
		return CommissionAnalysisReport{}, ErrAnalysisIntegrity
	}
	for _, g := range out.Items {
		if !validCommissionAnalysisTotals(g.Totals) {
			return CommissionAnalysisReport{}, ErrAnalysisIntegrity
		}
	}
	out.SnapshotAt = out.SnapshotAt.UTC()
	return out, nil
}
