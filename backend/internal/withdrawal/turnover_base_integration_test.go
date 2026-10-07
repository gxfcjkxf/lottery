package withdrawal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

type turnoverBaseCheckerFunc func(context.Context, pgx.Tx, EligibilityInput) (EligibilityDecision, error)

func (f turnoverBaseCheckerFunc) Check(ctx context.Context, tx pgx.Tx, input EligibilityInput) (EligibilityDecision, error) {
	return f(ctx, tx, input)
}

func TestWithdrawalTurnoverBaseSnapshotsAllAvailableBeforeReservation(t *testing.T) {
	f := newOrderIntegrationFixture(t, true, true)
	f.fund(t, []points.Allocation{
		{Source: "recharge", State: "available", Points: 100},
		{Source: "winning", State: "available", Points: 90},
		{Source: "gift", State: "available", Points: 80},
		{Source: "commission", State: "available", Points: 70},
	})
	ctx := context.Background()
	tx, err := f.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	allocation := []points.Allocation{{Source: "recharge", State: "available", Points: 40}, {Source: "gift", State: "available", Points: 30}}
	delta, err := points.AllocationDelta(allocation, "available", "manual_frozen")
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.points.Post(ctx, tx, points.Change{BrandID: orderTestBrand, MemberID: f.member, EntryType: "freeze", ReferenceType: "test_basis_freeze", ReferenceID: ids.New(), OperationKey: "test-basis-freeze", Reason: "exclude frozen source points from turnover basis", ActorType: "system", RequestID: ids.New(), Delta: delta, Allocation: allocation})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	var observed EligibilityInput
	checks := 0
	f.service.Eligibility = turnoverBaseCheckerFunc(func(_ context.Context, _ pgx.Tx, input EligibilityInput) (EligibilityDecision, error) {
		observed = input
		checks++
		// Even a checker must not replace the server's locked balance snapshot.
		return EligibilityDecision{Allowed: true, Evidence: json.RawMessage(`{"test_adapter":true,"turnover_base_snapshot":{"points":"1"}}`)}, nil
	})
	order := createOrder(t, f, 30, "turnover-base-small-request", []points.Allocation{{Source: "recharge", State: "available", Points: 20}, {Source: "gift", State: "available", Points: 10}})
	if observed.Wallet.BySource[0][0] != 60 || observed.Wallet.BySource[2][0] != 50 || observed.Wallet.BySource[3][0] != 70 {
		t.Fatalf("checker observed a deducted or incorrect balance: %+v", observed.Wallet.BySource)
	}
	if observed.TurnoverBase.Points != 110 || observed.TurnoverBase.WalletVersion != observed.Wallet.Version {
		t.Fatalf("checker basis is not the server-owned pre-reservation snapshot: %+v", observed.TurnoverBase)
	}
	var evidence struct {
		TestAdapter bool `json:"test_adapter"`
		Snapshot    struct {
			Recharge points.Amount `json:"recharge_available"`
			Gift     points.Amount `json:"gift_available"`
			Points   points.Amount `json:"points"`
			Version  int64         `json:"wallet_version,string"`
		} `json:"turnover_base_snapshot"`
	}
	if err = json.Unmarshal(order.EligibilityEvidence, &evidence); err != nil {
		t.Fatal(err)
	}
	if !evidence.TestAdapter || evidence.Snapshot.Recharge != 60 || evidence.Snapshot.Gift != 50 || evidence.Snapshot.Points != 110 || evidence.Snapshot.Version != observed.Wallet.Version {
		t.Fatalf("base must be all available recharge/gift before the 30-point reservation: %+v", evidence)
	}
	var saved json.RawMessage
	if err = f.db.QueryRow(ctx, `SELECT eligibility_evidence FROM withdrawal_orders WHERE id=$1`, order.ID).Scan(&saved); err != nil {
		t.Fatal(err)
	}
	var persisted map[string]json.RawMessage
	if err = json.Unmarshal(saved, &persisted); err != nil {
		t.Fatal(err)
	}
	var savedSnapshot struct {
		Points points.Amount `json:"points"`
	}
	if err = json.Unmarshal(persisted["turnover_base_snapshot"], &savedSnapshot); err != nil || savedSnapshot.Points != 110 {
		t.Fatalf("persistent base snapshot: %+v, error=%v", savedSnapshot, err)
	}
	// A later credit changes current balances, but original-key replay must not
	// recheck eligibility, recompute the basis or reserve again.
	f.fund(t, []points.Allocation{{Source: "gift", State: "available", Points: 7}})
	replayed := createOrder(t, f, 30, "turnover-base-small-request", order.SourceAllocation)
	var originalEvidence, replayedEvidence map[string]any
	if err = json.Unmarshal(order.EligibilityEvidence, &originalEvidence); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(replayed.EligibilityEvidence, &replayedEvidence); err != nil {
		t.Fatal(err)
	}
	if checks != 1 || replayed.ID != order.ID || !reflect.DeepEqual(replayedEvidence, originalEvidence) {
		t.Fatalf("replay altered original qualification: checks=%d original=%+v replay=%+v", checks, order, replayed)
	}
}

func TestWithdrawalTurnoverBaseEvidenceLimitRejectsBeforeReservation(t *testing.T) {
	f := newOrderIntegrationFixture(t, true, true)
	f.fund(t, oneRechargeAllocation(100))
	ctx := context.Background()
	for _, noteLength := range []int{16340, 16000} {
		name := "compact_limit"
		if noteLength == 16000 {
			name = "canonical_jsonb_limit"
		}
		t.Run(name, func(t *testing.T) {
			// The second case has a compact small representation but PostgreSQL
			// adds spaces for the many nested entries. Both must reject atomically.
			payload := map[string]any{"note": strings.Repeat("x", noteLength)}
			if noteLength == 16000 {
				delete(payload, "note")
				for i := 0; i < 1200; i++ {
					payload[fmt.Sprintf("k%07d", i)] = 1
				}
			}
			raw, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			if noteLength == 16000 && len(raw) > 16384 {
				t.Fatal("canonical-size fixture must fit the compact limit")
			}
			f.service.Eligibility = turnoverBaseCheckerFunc(func(context.Context, pgx.Tx, EligibilityInput) (EligibilityDecision, error) {
				return EligibilityDecision{Allowed: true, Evidence: raw}, nil
			})
			tx, err := f.db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			_, err = f.service.Create(ctx, tx, orderTestBrand, f.member, OrderInput{Points: 10, SourceAllocation: oneRechargeAllocation(10), ClientKey: "basis-limit-" + ids.New()}, f.actor)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("oversized basis evidence: %v", err)
			}
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			var count, entries int
			if err = f.db.QueryRow(ctx, `SELECT (SELECT count(*) FROM withdrawal_orders),(SELECT count(*) FROM point_ledger_entries WHERE entry_type='withdrawal_reserve')`).Scan(&count, &entries); err != nil {
				t.Fatal(err)
			}
			if count != 0 || entries != 0 {
				t.Fatalf("rejected evidence reserved funds: orders=%d entries=%d", count, entries)
			}
		})
	}
}
