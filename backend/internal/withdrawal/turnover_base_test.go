package withdrawal

import (
	"encoding/json"
	"errors"
	"math"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

func TestTurnoverBaseUsesOnlyAvailableRechargeAndGift(t *testing.T) {
	wallet := points.Wallet{Version: 19, RechargePoints: 1, GiftPoints: 1,
		BySource: points.Balance{{100, 200, 300, 400}, {500, 600, 700, 800}, {50, 60, 70, 80}, {900, 1000, 1100, 1200}}}
	base, err := turnoverBaseSnapshot(wallet)
	want := TurnoverBaseSnapshot{RechargeAvailable: 100, GiftAvailable: 50, Points: 150, WalletVersion: 19}
	if err != nil || base != want {
		t.Fatalf("base=%+v, error=%v, want %+v", base, err, want)
	}
	// Misleading summary fields do not replace the source/state buckets.
	wallet.BySource[0][0] = 1
	if base != want {
		t.Fatal("basis snapshot changed with the wallet value")
	}
}

func TestTurnoverBaseBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name    string
		balance points.Balance
		want    points.Amount
		err     error
	}{
		{"zero", points.Balance{}, 0, nil},
		{"winning only", points.Balance{{}, {100}}, 0, nil},
		{"frozen only", points.Balance{{0, 10, 20, 30}, {}, {0, 40, 50, 60}}, 0, nil},
		{"max", points.Balance{{points.Amount(math.MaxInt64 - 1)}, {}, {1}}, points.Amount(math.MaxInt64), nil},
		{"large exact", points.Balance{{9007199254740993}, {}, {7}}, 9007199254741000, nil},
		{"overflow", points.Balance{{points.Amount(math.MaxInt64)}, {}, {1}}, 0, points.ErrOverflow},
		{"negative basis", points.Balance{{-1}}, 0, points.ErrInvalid},
		{"negative excluded bucket", points.Balance{{1}, {0, -1}}, 0, points.ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base, err := turnoverBaseSnapshot(points.Wallet{BySource: tc.balance})
			if !errors.Is(err, tc.err) || base.Points != tc.want {
				t.Fatalf("base=%+v error=%v, want %d/%v", base, err, tc.want, tc.err)
			}
		})
	}
	if _, err := turnoverBaseSnapshot(points.Wallet{Version: -1}); !errors.Is(err, points.ErrInvalid) {
		t.Fatalf("negative wallet version: %v", err)
	}
}

func TestTurnoverBaseJSONPreservesLargeIntegers(t *testing.T) {
	base := TurnoverBaseSnapshot{RechargeAvailable: 9007199254740993, GiftAvailable: 7, Points: 9007199254741000, WalletVersion: 9007199254740993}
	raw, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]string
	if err = json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["points"] != "9007199254741000" || fields["recharge_available"] != "9007199254740993" || fields["wallet_version"] != "9007199254740993" {
		t.Fatalf("rounded snapshot: %s", raw)
	}
}
