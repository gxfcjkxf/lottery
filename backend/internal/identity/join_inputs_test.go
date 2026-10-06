package identity

import (
	"encoding/json"
	"testing"
)

func TestAuthSelectorsRejectDuplicateNullAndUnknownFields(t *testing.T) {
	for _, raw := range []string{`{"agent_code":"ABCDEF012345ABCDEF012345","agent_code":"1234567890ABCDEF12345678"}`, `{"referral_code":null}`, `{"brand_id":"other"}`, `null`, `{"password":null}`} {
		for _, out := range []any{&RegisterInput{}, &LoginInput{}, &TelegramInput{}, &OperatorInput{}} {
			if e := json.Unmarshal([]byte(raw), out); e == nil {
				t.Fatalf("accepted %s for %T", raw, out)
			}
		}
	}
	for _, out := range []any{&RegisterInput{}, &LoginInput{}, &TelegramInput{}, &OperatorInput{}} {
		if e := json.Unmarshal([]byte(`{"agent_code":"ABCDEF012345ABCDEF012345"}`), out); e != nil {
			t.Fatal(out, e)
		}
	}
}
