package rewards

import (
	"encoding/json"
	"testing"
)

func TestRewardInputRejectsUnpairedUnicodeEscapesWithoutRepairingReason(t *testing.T) {
	if err := json.Unmarshal([]byte("{\"version\":1,\"reason\":\""+string([]byte{0xff})+"\"}"), &ActionInput{}); err == nil {
		t.Fatal("invalid UTF-8 reason repaired instead of rejected")
	}
	for _, raw := range []string{`"\ud800"`, `"\udc00"`, `"\ud800x"`, `"\ud800\ud800"`, `"x\udfff"`} {
		var in ActionInput
		if err := json.Unmarshal([]byte(`{"version":1,"reason":`+raw+`}`), &in); err == nil {
			t.Fatalf("invalid Unicode reason repaired instead of rejected: %s", raw)
		}
	}
	for _, raw := range []string{`"\ud83d\ude00"`, `"\\ud800"`, `"\ufffd"`} {
		var in ActionInput
		if err := json.Unmarshal([]byte(`{"version":1,"reason":`+raw+`}`), &in); err != nil {
			t.Fatalf("legitimate Unicode or literal reason rejected: %s: %v", raw, err)
		}
	}
}
