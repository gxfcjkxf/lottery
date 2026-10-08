package notification

import (
	"fmt"
	"testing"
)

const validRewardEventJSON = `{"member_id":"11111111-1111-4111-8111-111111111111","resource_id":"22222222-2222-4222-8222-222222222222","points":"7","action_id":"33333333-3333-4333-8333-333333333333","version":1,"audit_log_id":"44444444-4444-4444-8444-444444444444"}`

func TestParseRewardEventRequiresSixUniqueClosedKeys(t *testing.T) {
	if in, version, err := parseRewardEvent([]byte(validRewardEventJSON)); err != nil || version != 1 || in.Points != "7" {
		t.Fatalf("valid reward payload rejected: %+v version=%d err=%v", in, version, err)
	}
	for _, raw := range [][]byte{
		[]byte(`{"member_id":"11111111-1111-4111-8111-111111111111","member_id":"11111111-1111-4111-8111-111111111111","resource_id":"22222222-2222-4222-8222-222222222222","points":"7","action_id":"33333333-3333-4333-8333-333333333333","version":1,"audit_log_id":"44444444-4444-4444-8444-444444444444"}`),
		[]byte(`{"member_id":"11111111-1111-4111-8111-111111111111","resource_id":"22222222-2222-4222-8222-222222222222","points":"7","action_id":"33333333-3333-4333-8333-333333333333","version":1}`),
		[]byte(`{"member_id":"11111111-1111-4111-8111-111111111111","resource_id":"22222222-2222-4222-8222-222222222222","points":"7","action_id":"33333333-3333-4333-8333-333333333333","version":1,"audit_log_id":"44444444-4444-4444-8444-444444444444","private":"x"}`),
		[]byte(`{"member_id":null,"resource_id":"22222222-2222-4222-8222-222222222222","points":"7","action_id":"33333333-3333-4333-8333-333333333333","version":1,"audit_log_id":"44444444-4444-4444-8444-444444444444"}`),
		[]byte(`{"member_id":"11111111-1111-4111-8111-111111111111","resource_id":"22222222-2222-4222-8222-222222222222","points":"7","action_id":"33333333-3333-4333-8333-333333333333","version":1,"audit_log_id":null}`),
		append(append([]byte(nil), []byte(validRewardEventJSON)...), []byte(` {}`)...),
		[]byte(`[]`),
	} {
		if _, _, err := parseRewardEvent(raw); err == nil {
			t.Errorf("accepted malformed reward event JSON: %s", raw)
		}
	}
}

func TestParseRewardEventEnforcesUUIDAndPositiveInt64Points(t *testing.T) {
	for _, points := range []string{"1", "9223372036854775807"} {
		raw := []byte(replaceRewardField(validRewardEventJSON, `"points":"7"`, fmt.Sprintf(`"points":%q`, points)))
		if _, _, err := parseRewardEvent(raw); err != nil {
			t.Errorf("accepted-range points %q rejected: %v", points, err)
		}
	}
	for _, points := range []string{"0", "-1", "01", "+1", "9223372036854775808", "1.0", ""} {
		raw := []byte(replaceRewardField(validRewardEventJSON, `"points":"7"`, fmt.Sprintf(`"points":%q`, points)))
		if _, _, err := parseRewardEvent(raw); err == nil {
			t.Errorf("accepted invalid points %q", points)
		}
	}
	for _, field := range []struct{ from, to string }{
		{`"member_id":"11111111-1111-4111-8111-111111111111"`, `"member_id":"bad"`},
		{`"action_id":"33333333-3333-4333-8333-333333333333"`, `"action_id":"bad"`},
		{`"audit_log_id":"44444444-4444-4444-8444-444444444444"`, `"audit_log_id":"bad"`},
	} {
		if _, _, err := parseRewardEvent([]byte(replaceRewardField(validRewardEventJSON, field.from, field.to))); err == nil {
			t.Errorf("accepted invalid UUID field replacement %s", field.to)
		}
	}
}

func TestParseRewardEventVersionRequiresSafeJSONInteger(t *testing.T) {
	for _, version := range []string{"1", "9007199254740991"} {
		if _, parsed, err := parseRewardEvent([]byte(replaceRewardField(validRewardEventJSON, `"version":1`, `"version":`+version))); err != nil || parsed == 0 {
			t.Errorf("valid version %s rejected: parsed=%d err=%v", version, parsed, err)
		}
	}
	for _, version := range []string{"0", "-1", "9007199254740992", `"1"`, "1.0", "1e0", "null", "true"} {
		if _, _, err := parseRewardEvent([]byte(replaceRewardField(validRewardEventJSON, `"version":1`, `"version":`+version))); err == nil {
			t.Errorf("accepted invalid version %s", version)
		}
	}
}

func replaceRewardField(value, old, replacement string) string {
	for i := 0; i+len(old) <= len(value); i++ {
		if value[i:i+len(old)] == old {
			return value[:i] + replacement + value[i+len(old):]
		}
	}
	return value
}
