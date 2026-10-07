package withdrawal

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

func TestValidateOrderInput(t *testing.T) {
	valid := OrderInput{
		Points: 17,
		SourceAllocation: []points.Allocation{
			{Source: "recharge", State: "available", Points: 10},
			{Source: "gift", State: "available", Points: 7},
		},
		ClientKey: "client_123",
	}
	if err := ValidateOrderInput(valid); err != nil {
		t.Fatalf("valid order input: %v", err)
	}
	fourSource := OrderInput{Points: 4, SourceAllocation: []points.Allocation{{Source: "recharge", State: "available", Points: 1}, {Source: "winning", State: "available", Points: 1}, {Source: "gift", State: "available", Points: 1}, {Source: "commission", State: "available", Points: 1}}, ClientKey: "client_123"}
	if err := ValidateOrderInput(fourSource); err != nil {
		t.Fatalf("canonical four-source order rejected: %v", err)
	}

	cases := map[string]OrderInput{
		"missing key":         {Points: 17, SourceAllocation: valid.SourceAllocation},
		"short key":           {Points: 17, SourceAllocation: valid.SourceAllocation, ClientKey: "1234567"},
		"long key":            {Points: 17, SourceAllocation: valid.SourceAllocation, ClientKey: strings.Repeat("a", 129)},
		"non-ascii key":       {Points: 17, SourceAllocation: valid.SourceAllocation, ClientKey: "clienté12"},
		"invalid key char":    {Points: 17, SourceAllocation: valid.SourceAllocation, ClientKey: "client 12"},
		"zero amount":         {Points: 0, SourceAllocation: valid.SourceAllocation, ClientKey: "client_123"},
		"negative amount":     {Points: -1, SourceAllocation: valid.SourceAllocation, ClientKey: "client_123"},
		"empty allocation":    {Points: 1, ClientKey: "client_123"},
		"too many":            {Points: 5, SourceAllocation: []points.Allocation{{Source: "recharge", State: "available", Points: 1}, {Source: "winning", State: "available", Points: 1}, {Source: "gift", State: "available", Points: 1}, {Source: "commission", State: "available", Points: 1}, {Source: "commission", State: "available", Points: 1}}, ClientKey: "client_123"},
		"wrong order":         {Points: 2, SourceAllocation: []points.Allocation{{Source: "winning", State: "available", Points: 1}, {Source: "recharge", State: "available", Points: 1}}, ClientKey: "client_123"},
		"duplicate source":    {Points: 2, SourceAllocation: []points.Allocation{{Source: "recharge", State: "available", Points: 1}, {Source: "recharge", State: "available", Points: 1}}, ClientKey: "client_123"},
		"unknown source":      {Points: 1, SourceAllocation: []points.Allocation{{Source: "bonus", State: "available", Points: 1}}, ClientKey: "client_123"},
		"wrong state":         {Points: 1, SourceAllocation: []points.Allocation{{Source: "gift", State: "withdrawal", Points: 1}}, ClientKey: "client_123"},
		"zero allocation":     {Points: 1, SourceAllocation: []points.Allocation{{Source: "gift", State: "available", Points: 0}}, ClientKey: "client_123"},
		"negative allocation": {Points: 1, SourceAllocation: []points.Allocation{{Source: "gift", State: "available", Points: -1}}, ClientKey: "client_123"},
		"sum mismatch":        {Points: 2, SourceAllocation: []points.Allocation{{Source: "gift", State: "available", Points: 1}}, ClientKey: "client_123"},
		"sum overflow":        {Points: points.Amount(math.MaxInt64), SourceAllocation: []points.Allocation{{Source: "recharge", State: "available", Points: points.Amount(math.MaxInt64 - 1)}, {Source: "winning", State: "available", Points: 2}}, ClientKey: "client_123"},
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidateOrderInput(input); !errors.Is(err, ErrInvalid) {
				t.Fatalf("ValidateOrderInput error=%v, want ErrInvalid", err)
			}
		})
	}
}

func TestValidateActionInput(t *testing.T) {
	valid := ActionInput{Version: maxSafeInteger, ClientKey: "client_123", Reason: "  reviewed  "}
	if err := ValidateActionInput(valid); err != nil {
		t.Fatalf("valid action input: %v", err)
	}
	for name, input := range map[string]ActionInput{
		"zero version":       {Version: 0, ClientKey: "client_123", Reason: "review"},
		"negative version":   {Version: -1, ClientKey: "client_123", Reason: "review"},
		"unsafe version":     {Version: maxSafeInteger + 1, ClientKey: "client_123", Reason: "review"},
		"invalid client key": {Version: 1, ClientKey: "bad key!", Reason: "review"},
		"blank reason":       {Version: 1, ClientKey: "client_123", Reason: " \t\n"},
		"reason too long":    {Version: 1, ClientKey: "client_123", Reason: strings.Repeat("x", 501)},
		"invalid utf8":       {Version: 1, ClientKey: "client_123", Reason: string([]byte{0xff})},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateActionInput(input); !errors.Is(err, ErrInvalid) {
				t.Fatalf("ValidateActionInput error=%v, want ErrInvalid", err)
			}
		})
	}
}

func TestTransitionTarget(t *testing.T) {
	for _, tc := range []struct{ state, action, target string }{
		{"reviewing", "approve", "processing"},
		{"reviewing", "reject", "rejected"},
		{"reviewing", "cancel", "cancelled"},
		{"processing", "cancel", "cancelled"},
		{"processing", "fail", "failed"},
		{"processing", "mark_paid", "paid"},
	} {
		got, err := TransitionTarget(tc.state, tc.action)
		if err != nil || got != tc.target {
			t.Errorf("TransitionTarget(%q, %q)=(%q, %v), want %q", tc.state, tc.action, got, err, tc.target)
		}
	}
	for _, tc := range []struct{ state, action string }{
		{"paid", "cancel"}, {"rejected", "approve"}, {"cancelled", "fail"}, {"failed", "mark_paid"}, {"unknown", "approve"}, {"reviewing", "mark_paid"}, {"processing", "reject"}, {"reviewing", "unknown"},
	} {
		if _, err := TransitionTarget(tc.state, tc.action); !errors.Is(err, ErrOrderState) {
			t.Errorf("TransitionTarget(%q, %q) error=%v, want ErrOrderState", tc.state, tc.action, err)
		}
	}
}

func TestOrderAmountsRejectJSONNumbers(t *testing.T) {
	var input OrderInput
	if err := json.Unmarshal([]byte(`{"points":1.5,"source_allocation":[],"client_key":"client_123"}`), &input); !errors.Is(err, points.ErrInvalid) {
		t.Fatalf("floating point amount unmarshal error=%v, want points.ErrInvalid", err)
	}
	if err := json.Unmarshal([]byte(`{"points":"9223372036854775808","source_allocation":[],"client_key":"client_123"}`), &input); !errors.Is(err, points.ErrOverflow) {
		t.Fatalf("overflow amount unmarshal error=%v, want points.ErrOverflow", err)
	}
}
