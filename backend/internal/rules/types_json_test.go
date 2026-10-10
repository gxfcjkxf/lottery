package rules

import (
	"encoding/json"
	"testing"
)

func TestPoolZeroBoundsUseCurrentJSONOmission(t *testing.T) {
	for _, tc := range []struct {
		pool Pool
		json string
	}{
		{Pool{Max: 6}, `{"max":6,"allow_repeat":false}`},
		{Pool{}, `{"allow_repeat":false}`},
	} {
		raw, err := json.Marshal(tc.pool)
		if err != nil || string(raw) != tc.json {
			t.Fatalf("current pool JSON=%s, err=%v; want %s", raw, err, tc.json)
		}
		var decoded Pool
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
		values, err := poolValues(decoded)
		if err != nil || len(values) != tc.pool.Max+1 || values[0] != 0 {
			t.Fatalf("omitted bounds must preserve zero-origin pool: values=%v err=%v", values, err)
		}
	}
}
