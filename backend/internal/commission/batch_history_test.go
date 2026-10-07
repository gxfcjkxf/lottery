package commission

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

const (
	historyBrandID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	historyCycleID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	historyRunID   = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
)

func TestBatchHistoryRejectsNilTransactionsAndInvalidScopes(t *testing.T) {
	ctx := context.Background()
	s := Service{}
	if _, err := s.RunsTx(ctx, nil, historyBrandID, historyCycleID, 20, 0); !errors.Is(err, ErrInvalid) {
		t.Fatalf("RunsTx with nil transaction = %v, want ErrInvalid", err)
	}
	if _, err := s.CalculationsTx(ctx, nil, historyBrandID, historyCycleID, historyRunID, 20, 0); !errors.Is(err, ErrInvalid) {
		t.Fatalf("CalculationsTx with nil transaction = %v, want ErrInvalid", err)
	}
	var validationTx pgx.Tx = historyValidationTx{}
	for _, tc := range []struct {
		brand, cycle  string
		limit, offset int
	}{
		{"not-a-uuid", historyCycleID, 20, 0},
		{historyBrandID, "not-a-uuid", 20, 0},
		{historyBrandID, historyCycleID, 0, 0},
		{historyBrandID, historyCycleID, 20, 1_000_001},
	} {
		if _, err := s.RunsTx(ctx, validationTx, tc.brand, tc.cycle, tc.limit, tc.offset); !errors.Is(err, ErrInvalid) {
			t.Errorf("RunsTx invalid scope/page = %v, want ErrInvalid", err)
		}
	}
	if _, err := s.CalculationsTx(ctx, validationTx, "not-a-uuid", historyCycleID, historyRunID, 20, 0); !errors.Is(err, ErrInvalid) {
		t.Fatalf("CalculationsTx invalid brand = %v, want ErrInvalid", err)
	}
	if _, err := s.CalculationsTx(ctx, validationTx, historyBrandID, historyCycleID, "", 20, 0); !errors.Is(err, ErrInvalid) {
		t.Fatalf("CalculationsTx without explicit run UUID = %v, want ErrInvalid", err)
	}
}

type historyValidationTx struct{ pgx.Tx }

func TestBatchHistorySerializersExposeOnlyApprovedFields(t *testing.T) {
	at := time.Date(2026, 10, 7, 4, 5, 6, 0, time.UTC)
	run, err := json.Marshal(RunHistoryItem{
		ID: historyRunID, BrandID: historyBrandID, CycleID: historyCycleID,
		Generation: "2", EvidenceEpoch: "17", State: "abandoned",
		CalculatedCount: "9007199254740993", EarningCount: "0", TotalPoints: "0", CreatedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertJSONKeys(t, run, []string{
		"id", "brand_id", "cycle_id", "generation", "evidence_epoch", "state",
		"calculated_count", "earning_count", "total_points", "created_at",
	})
	if !jsonContains(run, `"calculated_count":"9007199254740993"`) {
		t.Fatalf("large run count was not preserved as a string: %s", run)
	}

	calc, err := json.Marshal(CalculationHistoryItem{
		ID: historyCycleID, BrandID: historyBrandID, CycleID: historyCycleID, RunID: historyRunID,
		OrderID: historyCycleID, MemberID: historyBrandID, Reason: "eligible", Status: "won",
		StakePoints: points.Amount(9007199254740993), PrizePoints: 0, BasePoints: 44,
		AuditLogID: historyRunID, CreatedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertJSONKeys(t, calc, []string{
		"id", "brand_id", "cycle_id", "run_id", "order_id", "member_id", "reason", "status",
		"stake_points", "prize_points", "base_points", "job_id", "calculation_id", "generation",
		"audit_log_id", "created_at",
	})
	for _, forbidden := range []string{"rule_snapshot", "agent_path", "account_id", "credential", "current_run", "payout"} {
		if jsonContains(calc, forbidden) || jsonContains(run, forbidden) {
			t.Errorf("history DTO leaked forbidden field %q", forbidden)
		}
	}
	if !jsonContains(calc, `"stake_points":"9007199254740993"`) || !jsonContains(calc, `"generation":null`) {
		t.Fatalf("calculation JSON lost exact/null representation: %s", calc)
	}
}

func assertJSONKeys(t *testing.T, data []byte, want []string) {
	t.Helper()
	var got map[string]json.RawMessage
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("invalid serialized DTO: %v", err)
	}
	wantSet := make(map[string]struct{}, len(want))
	for _, key := range want {
		wantSet[key] = struct{}{}
	}
	if !reflect.DeepEqual(keySet(got), wantSet) {
		t.Fatalf("serialized keys = %v, want %v", keySet(got), wantSet)
	}
}

func keySet(values map[string]json.RawMessage) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for key := range values {
		out[key] = struct{}{}
	}
	return out
}

func jsonContains(data []byte, value string) bool {
	return strings.Contains(string(data), value)
}
