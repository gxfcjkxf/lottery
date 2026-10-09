package commission

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

func allocationFixture(t *testing.T) allocationHistoryRow {
	t.Helper()
	s := snapshotFixture()
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return allocationHistoryRow{Allocation: Allocation{
		BrandID: s.BrandID, CycleID: ids.New(), RunID: ids.New(), CalculationID: ids.New(), OrderID: ids.New(),
		AgentID: s.Path[0].ID, MemberID: s.Path[0].MemberID, BettorMemberID: s.MemberID,
		BasePoints: 3, ExactAmount: ExactAmount{"3", "20"}, CreatedAt: s.CapturedAt,
	}, Snapshot: raw, PlacedAt: s.CapturedAt, Reason: "eligible", SnapshotMatches: true}
}

func TestAllocationProjectionUsesSavedDifferentialAndKeepsExactFraction(t *testing.T) {
	r := allocationFixture(t)
	got, err := projectAllocation(r)
	if err != nil || got.Mode != "loss" || got.AgentRatio != "0.1" || got.DownstreamRatio != "0.05" || got.DifferenceRatio != "0.05" || got.ExactAmount != (ExactAmount{"3", "20"}) {
		t.Fatalf("saved differential projection = %+v, %v", got, err)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"rule_snapshot", "Snapshot", "agent_path", "account_id", "financial_policy", "revision_id", "points_before", "paid_points"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("public projection leaks %s", private)
		}
	}
	assertJSONKeys(t, encoded, []string{"brand_id", "cycle_id", "run_id", "calculation_id", "order_id", "agent_id", "member_id", "bettor_member_id", "base_points", "mode", "agent_ratio", "downstream_ratio", "difference_ratio", "exact_amount", "created_at"})
}

func TestAllocationProjectionFailsClosedOnBrokenSavedEvidence(t *testing.T) {
	for name, mutate := range map[string]func(*allocationHistoryRow){
		"missing snapshot":          func(r *allocationHistoryRow) { r.Snapshot = nil },
		"changed original snapshot": func(r *allocationHistoryRow) { r.SnapshotMatches = false },
		"unknown agent":             func(r *allocationHistoryRow) { r.AgentID = ids.New() },
		"wrong beneficiary":         func(r *allocationHistoryRow) { r.MemberID = ids.New() },
		"wrong bettor":              func(r *allocationHistoryRow) { r.BettorMemberID = ids.New() },
		"wrong exact amount":        func(r *allocationHistoryRow) { r.ExactAmount = ExactAmount{"1", "1"} },
		"excluded calculation":      func(r *allocationHistoryRow) { r.Reason = "cancelled" },
		"negative base":             func(r *allocationHistoryRow) { r.BasePoints = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			r := allocationFixture(t)
			mutate(&r)
			if _, err := projectAllocation(r); !errors.Is(err, ErrPolicyEvidence) {
				t.Fatal("damaged evidence accepted", err)
			}
		})
	}
}

func TestAllocationProjectionZeroAndLargeExactAmounts(t *testing.T) {
	r := allocationFixture(t)
	r.BasePoints = 0
	r.ExactAmount = ExactAmount{"0", "1"}
	if got, err := projectAllocation(r); err != nil || got.ExactAmount != r.ExactAmount {
		t.Fatal(got, err)
	}
	var s BetSnapshot
	if err := json.Unmarshal(r.Snapshot, &s); err != nil {
		t.Fatal(err)
	}
	s.Agency.Config.RatioCap = "1"
	s.Path[0].Config.Ratio = "0.999999"
	s.Path[1].Config.Ratio = "0"
	r.Snapshot, _ = json.Marshal(s)
	r.BasePoints = points.Amount(9223372036854775807)
	exacts, err := DifferentialSameBasis(r.BasePoints, []string{"0.999999", "0"})
	if err != nil {
		t.Fatal(err)
	}
	r.ExactAmount = exacts[0]
	if got, err := projectAllocation(r); err != nil || got.ExactAmount != exacts[0] || len(got.ExactAmount.Numerator) <= 19 {
		t.Fatal("arbitrary precision lost", got, err)
	}
}

func TestAllocationProjectionDoesNotInventIdentityInequality(t *testing.T) {
	r := allocationFixture(t)
	var s BetSnapshot
	if err := json.Unmarshal(r.Snapshot, &s); err != nil {
		t.Fatal(err)
	}
	s.MemberID = s.Path[0].MemberID
	r.BettorMemberID = s.MemberID
	r.Snapshot, _ = json.Marshal(s)
	got, err := projectAllocation(r)
	if err != nil || got.MemberID != got.BettorMemberID {
		t.Fatal("readonly projection added a new betting admission rule", got, err)
	}
}

func TestAllocationAndRunEarningsRejectInvalidScopesBeforeDatabase(t *testing.T) {
	s := Service{}
	ctx := context.Background()
	q := AllocationQuery{Limit: 20}
	if _, err := s.AllocationsTx(ctx, nil, historyBrandID, historyCycleID, historyRunID, q); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := s.EarningsForRunTx(ctx, nil, historyBrandID, historyCycleID, historyRunID, 20, 0); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	bad := "not-a-uuid"
	q.AgentID = &bad
	if _, err := s.AllocationsTx(ctx, historyValidationTx{}, historyBrandID, historyCycleID, historyRunID, q); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := s.EarningsForRunTx(ctx, historyValidationTx{}, historyBrandID, historyCycleID, "", 20, 0); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}
