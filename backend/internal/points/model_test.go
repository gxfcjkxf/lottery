package points

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

type legacyHashBalance [3][4]Amount

func (b legacyHashBalance) MarshalJSON() ([]byte, error) {
	return (legacyBalance{b[0], b[1], b[2]}).MarshalJSON()
}

// This is the original typed Change field order used before commission.
type legacyHashChange struct {
	BrandID, MemberID, EntryType, ReferenceType, ReferenceID, OperationKey, Reason, ActorType, ActorID, RequestID, IP string
	Delta                                                                                                             legacyHashBalance
	Allocation                                                                                                        []Allocation
	ReversalOf                                                                                                        string
}

func TestLegacyThreeSourceChangeHashGolden(t *testing.T) {
	var delta Balance
	delta[0][0], delta[1][2], delta[2][3] = 125, -7, 3
	c := Change{
		BrandID: "0199a000-0000-7000-8000-000000000001", MemberID: "0199a000-0000-7000-8000-000000000002",
		EntryType: "adjustment", ReferenceType: "legacy", ReferenceID: "0199a000-0000-7000-8000-000000000003",
		OperationKey: "legacy:key", Reason: "golden legacy hash", ActorType: "system", ActorID: "",
		RequestID: "request-is-excluded", IP: "192.0.2.1", Delta: delta,
		Allocation: []Allocation{{Source: "recharge", State: "available", Points: 125}, {Source: "winning", State: "system_frozen", Points: 7}, {Source: "gift", State: "withdrawal", Points: 3}},
		ReversalOf: "",
	}
	legacy := legacyHashChange{
		BrandID: c.BrandID, MemberID: c.MemberID, EntryType: c.EntryType, ReferenceType: c.ReferenceType,
		ReferenceID: c.ReferenceID, OperationKey: c.OperationKey, Reason: c.Reason, ActorType: c.ActorType,
		ActorID: c.ActorID, RequestID: "", IP: "",
		Delta: legacyHashBalance{delta[0], delta[1], delta[2]}, Allocation: c.Allocation, ReversalOf: c.ReversalOf,
	}
	legacyJSON, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	legacySum := sha256.Sum256(legacyJSON)
	want := hex.EncodeToString(legacySum[:])
	if want != "9a912d7129ee841a7fcf201e2017dc4275144159ea800b5f6df8622e6302eab3" {
		t.Fatalf("typed legacy golden hash changed: got=%s json=%s", want, legacyJSON)
	}
	got, err := changeHash(c)
	if err != nil || got != want {
		t.Fatalf("changeHash=%s err=%v want typed legacy hash %s", got, err, want)
	}
	c.Delta[3][0] = 1
	changedHash, err := changeHash(c)
	if err != nil || changedHash == want {
		t.Fatalf("commission delta reused legacy hash: hash=%s err=%v", changedHash, err)
	}
	c.Delta[3][0] = 0
	c.Allocation = append(c.Allocation, Allocation{Source: "commission", State: "available", Points: 1})
	allocationHash, err := changeHash(c)
	if err != nil || allocationHash == want {
		t.Fatalf("commission allocation reused legacy hash: hash=%s err=%v", allocationHash, err)
	}
}

func TestParseAmountCanonicalAndInt64Bounds(t *testing.T) {
	cases := []struct {
		input string
		want  Amount
		err   error
	}{
		{input: "0", want: 0},
		{input: "1", want: 1},
		{input: "-1", want: -1},
		{input: "9223372036854775807", want: Amount(math.MaxInt64)},
		{input: "-9223372036854775808", want: Amount(math.MinInt64)},
		{input: "+1", err: ErrInvalid},
		{input: "01", err: ErrInvalid},
		{input: "-01", err: ErrInvalid},
		{input: "-0", err: ErrInvalid},
		{input: "", err: ErrInvalid},
		{input: " 1", err: ErrInvalid},
		{input: "1.0", err: ErrInvalid},
		{input: "9223372036854775808", err: ErrOverflow},
		{input: "-9223372036854775809", err: ErrOverflow},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, err := ParseAmount(tc.input)
			if !errors.Is(err, tc.err) {
				t.Fatalf("ParseAmount(%q) error=%v, want %v", tc.input, err, tc.err)
			}
			if err == nil && got != tc.want {
				t.Fatalf("ParseAmount(%q)=%d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

func TestAmountJSONIsDecimalStringOnly(t *testing.T) {
	for _, input := range []string{`0`, `12`, `-7`, `1.0`, `1e2`, `null`, `true`, `{}`} {
		var amount Amount
		if err := json.Unmarshal([]byte(input), &amount); !errors.Is(err, ErrInvalid) {
			t.Errorf("Unmarshal(%s) error=%v, want ErrInvalid", input, err)
		}
	}
	var amount Amount
	if err := json.Unmarshal([]byte(`"9007199254740993"`), &amount); err != nil || amount != 9007199254740993 {
		t.Fatalf("large string amount=%d err=%v", amount, err)
	}
	got, err := json.Marshal(Amount(math.MinInt64))
	if err != nil || string(got) != `"-9223372036854775808"` {
		t.Fatalf("Marshal(MinInt64)=%s err=%v", got, err)
	}
}

func TestBalanceJSONRoundTripAndStrictLegacyOrSixteenBuckets(t *testing.T) {
	var balance Balance
	for source := range balance {
		for state := range balance[source] {
			balance[source][state] = Amount(source*10 + state)
		}
	}
	encoded, err := json.Marshal(balance)
	if err != nil {
		t.Fatal(err)
	}
	positions := []int{strings.Index(string(encoded), `"recharge":`), strings.Index(string(encoded), `"winning":`), strings.Index(string(encoded), `"gift":`), strings.Index(string(encoded), `"commission":`)}
	if !(positions[0] >= 0 && positions[0] < positions[1] && positions[1] < positions[2] && positions[2] < positions[3]) {
		t.Fatalf("balance JSON source order=%v JSON=%s", positions, encoded)
	}
	var decoded Balance
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, balance) {
		t.Fatalf("round trip differs: got=%v want=%v", decoded, balance)
	}

	valid := `{"recharge":{"available":"0","manual_frozen":"0","system_frozen":"0","withdrawal":"0"},"winning":{"available":"0","manual_frozen":"0","system_frozen":"0","withdrawal":"0"},"gift":{"available":"0","manual_frozen":"0","system_frozen":"0","withdrawal":"0"}}`
	legacy := valid
	full := strings.TrimSuffix(valid, "}") + `,"commission":{"available":"0","manual_frozen":"0","system_frozen":"0","withdrawal":"0"}}`
	var legacyBalance Balance
	if err := json.Unmarshal([]byte(legacy), &legacyBalance); err != nil {
		t.Fatalf("exact legacy snapshot rejected: %v", err)
	}
	if legacyBalance[3] != ([4]Amount{}) {
		t.Fatalf("legacy commission values=%v", legacyBalance[3])
	}
	canonical, err := json.Marshal(legacyBalance)
	if err != nil || !strings.Contains(string(canonical), `"commission":`) {
		t.Fatalf("legacy snapshot did not normalize to four sources: %s err=%v", canonical, err)
	}
	invalid := []struct {
		name string
		json string
	}{
		{name: "missing source", json: strings.Replace(valid, `,"gift":{"available":"0","manual_frozen":"0","system_frozen":"0","withdrawal":"0"}`, "", 1)},
		{name: "missing state", json: strings.Replace(valid, `,"withdrawal":"0"`, "", 1)},
		{name: "unknown source", json: strings.Replace(valid, `"gift":`, `"other":`, 1)},
		{name: "unknown state", json: strings.Replace(valid, `"withdrawal":"0"`, `"unknown":"0"`, 1)},
		{name: "number instead of string", json: strings.Replace(valid, `"available":"0"`, `"available":0`, 1)},
		{name: "duplicate source", json: strings.TrimSuffix(valid, "}") + `,"gift":{"available":"0","manual_frozen":"0","system_frozen":"0","withdrawal":"0"}}`},
		{name: "duplicate state", json: strings.Replace(valid, `"available":"0"`, `"available":"0","available":"0"`, 1)},
		{name: "incomplete four-source shape", json: strings.Replace(full, `,"gift":{"available":"0","manual_frozen":"0","system_frozen":"0","withdrawal":"0"}`, "", 1)},
		{name: "trailing value", json: valid + ` true`},
		{name: "null root", json: `null`},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			var got Balance
			err := json.Unmarshal([]byte(tc.json), &got)
			if tc.name == "trailing value" && err == nil {
				t.Fatal("trailing JSON value was accepted")
			}
			if tc.name != "trailing value" && !errors.Is(err, ErrInvalid) {
				t.Fatalf("Unmarshal error=%v, want ErrInvalid", err)
			}
		})
	}
	var signed Balance
	if err := json.Unmarshal([]byte(strings.Replace(valid, `"available":"0"`, `"available":"-9"`, 1)), &signed); err != nil || signed[0][0] != -9 {
		t.Fatalf("signed delta snapshot rejected: balance=%v err=%v", signed, err)
	}
}

func TestBalanceValidateAndApplyCheckedArithmetic(t *testing.T) {
	var balance Balance
	if err := balance.Validate(); err != nil {
		t.Fatal(err)
	}
	negative := balance
	negative[1][2] = -1
	if err := negative.Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("negative balance validation error=%v", err)
	}
	tooLarge := balance
	tooLarge[0][0] = Amount(math.MaxInt64)
	tooLarge[2][3] = 1
	if err := tooLarge.Validate(); !errors.Is(err, ErrOverflow) {
		t.Fatalf("aggregate overflow validation error=%v", err)
	}

	base := balance
	base[0][0] = 100
	base[0][1] = 20
	delta := balance
	delta[0][0] = -30
	delta[1][3] = 7
	got, err := base.Apply(delta)
	if err != nil || got[0][0] != 70 || got[0][1] != 20 || got[1][3] != 7 || base[0][0] != 100 {
		t.Fatalf("Apply got=%v err=%v base=%v", got, err, base)
	}
	underflow := balance
	underflow[0][0] = -101
	if _, err := base.Apply(underflow); !errors.Is(err, ErrInsufficient) {
		t.Fatalf("underflow error=%v", err)
	}
	max := balance
	max[0][0] = Amount(math.MaxInt64)
	plusOne := balance
	plusOne[0][0] = 1
	if _, err := max.Apply(plusOne); !errors.Is(err, ErrOverflow) {
		t.Fatalf("int64 overflow error=%v", err)
	}
	aggregate := balance
	aggregate[0][0] = Amount(math.MaxInt64)
	aggregate[2][3] = 1
	if _, err := balance.Apply(aggregate); !errors.Is(err, ErrOverflow) {
		t.Fatalf("aggregate overflow error=%v", err)
	}
}

func TestBalanceTotalsAndIndexLookup(t *testing.T) {
	var balance Balance
	balance[0][0], balance[0][1], balance[0][2], balance[0][3] = 100, 20, 30, 5
	balance[1][0], balance[1][1], balance[1][2], balance[1][3] = 50, 7, 8, 10
	balance[2][0], balance[2][1], balance[2][2], balance[2][3] = 40, 9, 6, 4
	balance[3][0], balance[3][1], balance[3][2], balance[3][3] = 11, 2, 3, 4

	available, err := balance.StateTotal(0)
	if err != nil || available != 201 {
		t.Fatalf("available total=%d err=%v", available, err)
	}
	manualFrozen, err := balance.StateTotal(1)
	if err != nil || manualFrozen != 38 {
		t.Fatalf("manual frozen total=%d err=%v", manualFrozen, err)
	}
	systemFrozen, err := balance.StateTotal(2)
	if err != nil || systemFrozen != 47 {
		t.Fatalf("system frozen total=%d err=%v", systemFrozen, err)
	}
	withdrawal, err := balance.StateTotal(3)
	if err != nil || withdrawal != 23 {
		t.Fatalf("withdrawal total=%d err=%v", withdrawal, err)
	}
	displayTotal := available + manualFrozen + systemFrozen
	if displayTotal != 286 || displayTotal+withdrawal != 309 {
		t.Fatalf("display total should combine available and both frozen buckets, excluding withdrawal: display=%d withdrawal=%d", displayTotal, withdrawal)
	}
	rechargeTotal, err := balance.SourceTotal(0)
	if err != nil || rechargeTotal != 155 {
		t.Fatalf("recharge total=%d err=%v", rechargeTotal, err)
	}
	if _, err := balance.StateTotal(-1); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid state index error=%v", err)
	}
	commissionTotal, err := balance.SourceTotal(3)
	if err != nil || commissionTotal != 20 {
		t.Fatalf("commission total=%d err=%v", commissionTotal, err)
	}
	if _, err := balance.SourceTotal(4); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid source index error=%v", err)
	}
	if i, err := SourceIndex("winning"); err != nil || i != 1 {
		t.Fatalf("SourceIndex winning=%d err=%v", i, err)
	}
	if i, err := StateIndex("withdrawal"); err != nil || i != 3 {
		t.Fatalf("StateIndex withdrawal=%d err=%v", i, err)
	}
	if _, err := SourceIndex("bonus"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown source error=%v", err)
	}
	if _, err := StateIndex("expired"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown state error=%v", err)
	}

	overflow := Balance{}
	overflow[0][0], overflow[1][0] = Amount(math.MaxInt64), 1
	if _, err := overflow.StateTotal(0); !errors.Is(err, ErrOverflow) {
		t.Fatalf("state total overflow error=%v", err)
	}
	if _, err := overflow.SourceTotal(0); err != nil {
		t.Fatalf("single source total should fit: %v", err)
	}
}

func TestAllocateUsesSourcePriorityAndRequestedState(t *testing.T) {
	var balance Balance
	balance[0][0], balance[1][0], balance[2][0], balance[3][0] = 5, 7, 9, 11
	balance[0][1], balance[1][1], balance[2][1] = 100, 100, 100
	balance[0][3] = 3
	allocations, err := balance.Allocate(10, "available")
	if err != nil {
		t.Fatal(err)
	}
	want := []Allocation{{Source: "recharge", State: "available", Points: 5}, {Source: "winning", State: "available", Points: 5}}
	if !reflect.DeepEqual(allocations, want) {
		t.Fatalf("allocations=%+v want=%+v", allocations, want)
	}
	if balance[0][0] != 5 || balance[1][0] != 7 {
		t.Fatal("Allocate mutated input balance")
	}
	balance[0][0], balance[1][0], balance[2][0], balance[3][0] = 5, 7, 9, 11
	all, err := balance.Allocate(32, "available")
	wantAll := []Allocation{
		{Source: "recharge", State: "available", Points: 5},
		{Source: "winning", State: "available", Points: 7},
		{Source: "commission", State: "available", Points: 11},
		{Source: "gift", State: "available", Points: 9},
	}
	if err != nil || !reflect.DeepEqual(all, wantAll) {
		t.Fatalf("four-source debit allocation=%+v err=%v want=%+v", all, err, wantAll)
	}
	frozen, err := balance.Allocate(8, "manual_frozen")
	if err != nil || len(frozen) != 1 || frozen[0] != (Allocation{Source: "recharge", State: "manual_frozen", Points: 8}) {
		t.Fatalf("manual frozen allocation=%+v err=%v", frozen, err)
	}
	withdrawal, err := balance.Allocate(3, "withdrawal")
	if err != nil || len(withdrawal) != 1 || withdrawal[0].State != "withdrawal" {
		t.Fatalf("withdrawal allocation=%+v err=%v", withdrawal, err)
	}
	if _, err := balance.Allocate(0, "available"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("zero allocation error=%v", err)
	}
	if _, err := balance.Allocate(1, "expired"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown state allocation error=%v", err)
	}
	if _, err := balance.Allocate(33, "available"); !errors.Is(err, ErrInsufficient) {
		t.Fatalf("insufficient allocation error=%v", err)
	}
}

func TestAllocationDeltaCreditDebitTransferAndValidation(t *testing.T) {
	allocations := []Allocation{{Source: "recharge", State: "available", Points: 10}, {Source: "winning", State: "available", Points: 4}}
	credit, err := AllocationDelta(allocations, "", "available")
	if err != nil || credit[0][0] != 10 || credit[1][0] != 4 {
		t.Fatalf("credit delta=%v err=%v", credit, err)
	}
	debit, err := AllocationDelta(allocations, "available", "")
	if err != nil || debit[0][0] != -10 || debit[1][0] != -4 {
		t.Fatalf("debit delta=%v err=%v", debit, err)
	}
	transfer, err := AllocationDelta(allocations, "available", "system_frozen")
	if err != nil || transfer[0][0] != -10 || transfer[0][2] != 10 || transfer[1][0] != -4 || transfer[1][2] != 4 {
		t.Fatalf("transfer delta=%v err=%v", transfer, err)
	}
	badCases := []struct {
		name string
		in   []Allocation
		from string
		to   string
	}{
		{name: "state mismatch", in: allocations, from: "manual_frozen", to: "available"},
		{name: "unknown source", in: []Allocation{{Source: "other", State: "available", Points: 1}}, from: "", to: "available"},
		{name: "nonpositive", in: []Allocation{{Source: "gift", State: "available", Points: 0}}, from: "", to: "available"},
		{name: "duplicate source state", in: []Allocation{{Source: "gift", State: "available", Points: 1}, {Source: "gift", State: "available", Points: 2}}, from: "", to: "available"},
		{name: "unknown destination", in: allocations[:1], from: "available", to: "expired"},
		{name: "both empty", in: allocations[:1], from: "", to: ""},
	}
	for _, tc := range badCases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := AllocationDelta(tc.in, tc.from, tc.to); !errors.Is(err, ErrInvalid) {
				t.Fatalf("AllocationDelta error=%v, want ErrInvalid", err)
			}
		})
	}
	overflowing := []Allocation{{Source: "recharge", State: "available", Points: Amount(math.MaxInt64)}, {Source: "winning", State: "available", Points: 1}}
	if _, err := AllocationDelta(overflowing, "", "available"); !errors.Is(err, ErrOverflow) {
		t.Fatalf("allocation sum overflow error=%v", err)
	}
}

func TestNegateChecksMinInt64(t *testing.T) {
	var value Balance
	value[0][0] = 5
	value[1][2] = -9
	got, err := Negate(value)
	if err != nil || got[0][0] != -5 || got[1][2] != 9 {
		t.Fatalf("Negate=%v err=%v", got, err)
	}
	value[2][3] = Amount(math.MinInt64)
	if _, err := Negate(value); !errors.Is(err, ErrOverflow) {
		t.Fatalf("Negate(MinInt64) error=%v", err)
	}
}

func TestFourSourceAllocationValidationBoundsAndCommission(t *testing.T) {
	allocations := []Allocation{
		{Source: "recharge", State: "available", Points: 1},
		{Source: "winning", State: "available", Points: 2},
		{Source: "gift", State: "available", Points: 3},
		{Source: "commission", State: "available", Points: 4},
	}
	delta, err := AllocationDelta(allocations, "", "available")
	if err != nil || delta[3][0] != 4 {
		t.Fatalf("commission allocation delta=%v err=%v", delta, err)
	}
	commissionMove := []Allocation{{Source: "commission", State: "manual_frozen", Points: 6}}
	move, err := AllocationDelta(commissionMove, "manual_frozen", "available")
	if err != nil || move[3][1] != -6 || move[3][0] != 6 {
		t.Fatalf("commission state transfer=%v err=%v", move, err)
	}
	c := Change{Delta: delta, Allocation: allocations}
	if !allocationMatches(c) {
		t.Fatalf("four-source allocation did not match delta: %+v", c)
	}
	c.Delta[3][0]++
	if allocationMatches(c) {
		t.Fatal("commission delta mismatch was accepted")
	}
	c.Delta = delta
	c.Allocation = allocations[:3]
	if allocationMatches(c) {
		t.Fatal("commission delta without commission allocation was accepted")
	}
	tooMany := append(append([]Allocation{}, allocations...), Allocation{Source: "commission", State: "available", Points: 1})
	if _, err := AllocationDelta(tooMany, "", "available"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate commission source error=%v", err)
	}
	var boundary Balance
	boundary[3][0] = Amount(math.MaxInt64)
	if err := boundary.Validate(); err != nil {
		t.Fatal(err)
	}
	boundary[0][0] = 1
	if !errors.Is(boundary.Validate(), ErrOverflow) {
		t.Fatal("four-source aggregate overflow was accepted")
	}
	overflowing := []Allocation{
		{Source: "recharge", State: "available", Points: Amount(math.MaxInt64)},
		{Source: "winning", State: "available", Points: 1},
	}
	if _, err := AllocationDelta(overflowing, "", "available"); !errors.Is(err, ErrOverflow) {
		t.Fatalf("four-source allocation sum overflow error=%v", err)
	}
}

func TestAllocateResolvesDebitPriorityToStableStorageIndices(t *testing.T) {
	var balance Balance
	balance[0][0], balance[1][0], balance[2][0], balance[3][0] = 2, 4, 3, 9
	allocations, err := balance.Allocate(18, "available")
	want := []Allocation{
		{Source: "recharge", State: "available", Points: 2},
		{Source: "winning", State: "available", Points: 4},
		{Source: "commission", State: "available", Points: 9},
		{Source: "gift", State: "available", Points: 3},
	}
	if err != nil || !reflect.DeepEqual(allocations, want) {
		t.Fatalf("allocation=%+v err=%v want=%+v", allocations, err, want)
	}
	delta, err := AllocationDelta(allocations, "available", "")
	if err != nil || delta[0][0] != -2 || delta[1][0] != -4 || delta[2][0] != -3 || delta[3][0] != -9 {
		t.Fatalf("debit delta=%v err=%v", delta, err)
	}
	if !allocationMatches(Change{Delta: delta, Allocation: allocations}) {
		t.Fatal("priority allocations did not match their original storage buckets")
	}
	swapped := append([]Allocation(nil), allocations...)
	swapped[2].Source, swapped[3].Source = swapped[3].Source, swapped[2].Source
	if allocationMatches(Change{Delta: delta, Allocation: swapped}) {
		t.Fatal("gift and commission allocations could claim each other's bucket changes")
	}
}

func TestWalletSummaryIncludesCommission(t *testing.T) {
	var balance Balance
	balance[3][0], balance[3][1], balance[3][2], balance[3][3] = 12, 3, 4, 5
	wallet, err := makeWallet("brand", "member", "account", 7, balance)
	if err != nil {
		t.Fatal(err)
	}
	if wallet.CommissionPoints != 12 || wallet.AvailablePoints != 12 || wallet.ManualFrozenPoints != 3 || wallet.SystemFrozenPoints != 4 || wallet.WithdrawalPoints != 5 || wallet.FrozenPoints != 7 || wallet.DisplayPoints != 19 {
		t.Fatalf("commission wallet totals are inconsistent: %+v", wallet)
	}
}
