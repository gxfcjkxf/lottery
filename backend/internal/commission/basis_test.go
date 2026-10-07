package commission

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

func TestBases(t *testing.T) {
	tests := []struct {
		name    string
		status  string
		stake   points.Amount
		prize   points.Amount
		want    Basis
		wantErr bool
	}{
		{name: "lost", status: "lost", stake: 25, want: Basis{TurnoverPoints: 25, LossPoints: 25}},
		{name: "won", status: "won", stake: 25, prize: 30, want: Basis{TurnoverPoints: 25}},
		{name: "won tie", status: "won", stake: 25, prize: 25, want: Basis{TurnoverPoints: 25}},
		{name: "won below stake", status: "won", stake: 25, prize: 10, want: Basis{TurnoverPoints: 25}},
		{name: "won zero prize", status: "won", stake: 25, want: Basis{TurnoverPoints: 25}},
		{name: "zero stake", status: "lost", stake: 0, wantErr: true},
		{name: "negative stake", status: "won", stake: -1, wantErr: true},
		{name: "negative prize", status: "won", stake: 1, prize: -1, wantErr: true},
		{name: "lost with positive prize", status: "lost", stake: 1, prize: 1, wantErr: true},
		{name: "unknown status", status: "mystery", stake: 1, wantErr: true},
		{name: "pending", status: "pending", stake: 1, wantErr: true},
		{name: "cancelled", status: "cancelled", stake: 1, wantErr: true},
		{name: "abnormal", status: "abnormal", stake: 1, wantErr: true},
		{name: "invalid", status: "invalid", stake: 1, wantErr: true},
		{name: "empty status", stake: 1, wantErr: true},
		{name: "large won", status: "won", stake: points.Amount(1<<53 + 1), prize: points.Amount(math.MaxInt64), want: Basis{TurnoverPoints: points.Amount(1<<53 + 1)}},
		{name: "max stake lost", status: "lost", stake: points.Amount(math.MaxInt64), want: Basis{TurnoverPoints: points.Amount(math.MaxInt64), LossPoints: points.Amount(math.MaxInt64)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Bases(tt.status, tt.stake, tt.prize)
			if tt.wantErr {
				if err != ErrInvalid {
					t.Fatalf("Bases() error = %v, want %v", err, ErrInvalid)
				}
				return
			}
			if err != nil {
				t.Fatalf("Bases() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("Bases() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestBasisJSONPreservesExactAmounts(t *testing.T) {
	want := Basis{
		TurnoverPoints: points.Amount(math.MaxInt64),
		LossPoints:     points.Amount(1<<53 + 1),
	}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	const expected = `{"turnover_points":"9223372036854775807","loss_points":"9007199254740993"}`
	if string(data) != expected {
		t.Fatalf("json.Marshal() = %s, want %s", data, expected)
	}

	var got Basis
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got != want {
		t.Fatalf("JSON round trip = %#v, want %#v", got, want)
	}
}
