package withdrawal

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestBetTurnoverSnapshotRequiresExactClosedHistoricalEvidence(t *testing.T) {
	valid := `{"schema_version":1,"multiple":"2.5","source":"game","brand_version":"9007199254740993","game_version":"2","brand_revision_id":"0199a000-0000-7000-8000-000000000001","game_revision_id":"0199a000-0000-7000-8000-000000000002"}`
	var snapshot betTurnoverSnapshot
	if err := decodeBetTurnoverSnapshot([]byte(valid), &snapshot); err != nil || snapshot.BrandVersion != "9007199254740993" || snapshot.Multiple != "2.5" {
		t.Fatal(snapshot, err)
	}
	for _, bad := range []string{
		`null`, `{}`, strings.Replace(valid, `"multiple":"2.5"`, `"multiple":"0"`, 1),
		strings.Replace(valid, `"multiple":"2.5"`, `"multiple":"2.50"`, 1),
		strings.Replace(valid, `"source":"game"`, `"source":"current_policy"`, 1),
		strings.Replace(valid, `"brand_version":"9007199254740993"`, `"brand_version":9007199254740993`, 1),
		strings.Replace(valid, `"game_version":"2"`, `"game_version":"02"`, 1),
		strings.Replace(valid, `"game_version":"2"`, `"game_version":"9223372036854775808"`, 1),
		strings.Replace(valid, `"schema_version":1`, `"schema_version":2`, 1),
		strings.Replace(valid, `"schema_version":1`, `"schema_version":1,"schema_version":1`, 1),
		strings.Replace(valid, `"multiple":"2.5"`, `"multiple":"2.5","qualification_override":true`, 1),
		strings.Replace(valid, `"game_revision_id":"0199a000-0000-7000-8000-000000000002"`, `"game_revision_id":null`, 1),
	} {
		var value betTurnoverSnapshot
		if err := decodeBetTurnoverSnapshot([]byte(bad), &value); !errors.Is(err, ErrTurnoverEvidence) {
			t.Fatalf("invalid snapshot accepted: %s: %v", bad, err)
		}
	}
	// A legacy snapshot is not reconstructed from the policy request model.
	legacy, _ := json.Marshal(DefaultBrandConfig())
	if err := decodeBetTurnoverSnapshot(legacy, &snapshot); !errors.Is(err, ErrTurnoverEvidence) {
		t.Fatal("current policy substituted for bet-time evidence", err)
	}
}
