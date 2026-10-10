package compliance

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func age(value int) *int { return &value }

const validConfigJSON = `{"age_enabled":false,"minimum_age":null,"region_enabled":false,"allowed_countries":[],"identity_enabled":false,"account_risk_enabled":false,"betting_risk_enabled":false,"exclusion_enabled":false,"responsible_gambling_enabled":false}`

func TestDefaultAndValidConfig(t *testing.T) {
	c := DefaultConfig()
	if c.AgeEnabled || c.MinimumAge != nil || c.RegionEnabled || c.IdentityEnabled || c.AccountRiskEnabled || c.BettingRiskEnabled || c.ExclusionEnabled || c.ResponsibleGamblingEnabled || c.AllowedCountries == nil || len(c.AllowedCountries) != 0 {
		t.Fatalf("unexpected default config: %#v", c)
	}
	if !ValidConfig(c) {
		t.Fatal("default config must be valid")
	}
	for _, c := range []Config{
		{MinimumAge: age(18), AllowedCountries: []string{}},
		{AgeEnabled: true, MinimumAge: age(120), AllowedCountries: []string{}},
		{AllowedCountries: []string{"CA", "GB", "US"}},
		{RegionEnabled: true, AllowedCountries: []string{"SG"}},
		{IdentityEnabled: true, AllowedCountries: []string{}},
	} {
		if !ValidConfig(c) {
			t.Errorf("valid config rejected: %#v", c)
		}
	}
	for _, c := range []Config{
		{}, // nil would serialize as JSON null, not the required array.
		{AgeEnabled: true},
		{MinimumAge: age(17)},
		{MinimumAge: age(121)},
		{RegionEnabled: true},
		{AllowedCountries: make([]string, 251)},
		{AllowedCountries: []string{"US", "CA"}},
		{AllowedCountries: []string{"US", "US"}},
		{AllowedCountries: []string{"us"}},
		{AllowedCountries: []string{"U"}},
		{AllowedCountries: []string{"USA"}},
		{AllowedCountries: []string{"1A"}},
		{AllowedCountries: []string{"A_"}},
	} {
		if ValidConfig(c) {
			t.Errorf("invalid config accepted: %#v", c)
		}
	}
	maxCountries := make([]string, 250)
	for i := range maxCountries {
		maxCountries[i] = fmt.Sprintf("%c%c", 'A'+i/26, 'A'+i%26)
	}
	if !ValidConfig(Config{AllowedCountries: maxCountries}) {
		t.Fatal("250 sorted unique countries rejected")
	}
}

func TestConfigUnmarshalIsClosedAndStrict(t *testing.T) {
	valid := validConfigJSON
	var got Config
	if err := json.Unmarshal([]byte(valid), &got); err != nil || !ValidConfig(got) {
		t.Fatalf("valid config: %#v, %v", got, err)
	}
	bad := []string{
		`{}`, // missing all required fields
		`{"age_enabled":false,"minimum_age":null,"region_enabled":false,"allowed_countries":[],"account_risk_enabled":false,"betting_risk_enabled":false,"exclusion_enabled":false,"responsible_gambling_enabled":false}`, // missing field
		`{"age_enabled":false,"age_enabled":false,"minimum_age":null,"region_enabled":false,"allowed_countries":[],"identity_enabled":false,"account_risk_enabled":false,"betting_risk_enabled":false,"exclusion_enabled":false,"responsible_gambling_enabled":false}`,
		`{"age_enabled":false,"minimum_age":null,"region_enabled":false,"allowed_countries":[],"identity_enabled":false,"account_risk_enabled":false,"betting_risk_enabled":false,"exclusion_enabled":false,"responsible_gambling_enabled":false,"extra":true}`,
		`{"age_enabled":null,"minimum_age":null,"region_enabled":false,"allowed_countries":[],"identity_enabled":false,"account_risk_enabled":false,"betting_risk_enabled":false,"exclusion_enabled":false,"responsible_gambling_enabled":false}`,
		`{"age_enabled":"false","minimum_age":null,"region_enabled":false,"allowed_countries":[],"identity_enabled":false,"account_risk_enabled":false,"betting_risk_enabled":false,"exclusion_enabled":false,"responsible_gambling_enabled":false}`,
		`{"age_enabled":0,"minimum_age":null,"region_enabled":false,"allowed_countries":[],"identity_enabled":false,"account_risk_enabled":false,"betting_risk_enabled":false,"exclusion_enabled":false,"responsible_gambling_enabled":false}`,
		`{"age_enabled":false,"minimum_age":false,"region_enabled":false,"allowed_countries":[],"identity_enabled":false,"account_risk_enabled":false,"betting_risk_enabled":false,"exclusion_enabled":false,"responsible_gambling_enabled":false}`,
		`{"age_enabled":false,"minimum_age":"18","region_enabled":false,"allowed_countries":[],"identity_enabled":false,"account_risk_enabled":false,"betting_risk_enabled":false,"exclusion_enabled":false,"responsible_gambling_enabled":false}`,
		`{"age_enabled":false,"minimum_age":null,"region_enabled":false,"allowed_countries":null,"identity_enabled":false,"account_risk_enabled":false,"betting_risk_enabled":false,"exclusion_enabled":false,"responsible_gambling_enabled":false}`,
		`{"age_enabled":false,"minimum_age":null,"region_enabled":false,"allowed_countries":"US","identity_enabled":false,"account_risk_enabled":false,"betting_risk_enabled":false,"exclusion_enabled":false,"responsible_gambling_enabled":false}`,
		`{"age_enabled":true,"minimum_age":null,"region_enabled":false,"allowed_countries":[],"identity_enabled":false,"account_risk_enabled":false,"betting_risk_enabled":false,"exclusion_enabled":false,"responsible_gambling_enabled":false}`,
		`{"age_enabled":false,"minimum_age":17,"region_enabled":false,"allowed_countries":[],"identity_enabled":false,"account_risk_enabled":false,"betting_risk_enabled":false,"exclusion_enabled":false,"responsible_gambling_enabled":false}`,
		`{"age_enabled":false,"minimum_age":null,"region_enabled":true,"allowed_countries":[],"identity_enabled":false,"account_risk_enabled":false,"betting_risk_enabled":false,"exclusion_enabled":false,"responsible_gambling_enabled":false}`,
		`{"age_enabled":false,"minimum_age":null,"region_enabled":false,"allowed_countries":["US","CA"],"identity_enabled":false,"account_risk_enabled":false,"betting_risk_enabled":false,"exclusion_enabled":false,"responsible_gambling_enabled":false}`,
		`null`,
		`[]`,
	}
	for _, raw := range bad {
		err := json.Unmarshal([]byte(raw), &got)
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("Unmarshal(%s) error=%v, want ErrInvalid", raw, err)
		}
	}
	for _, flag := range []string{"account_risk_enabled", "betting_risk_enabled", "exclusion_enabled", "responsible_gambling_enabled"} {
		missing := strings.Replace(valid, `,"`+flag+`":false`, "", 1)
		if err := json.Unmarshal([]byte(missing), &got); !errors.Is(err, ErrInvalid) {
			t.Errorf("missing %s accepted: %v", flag, err)
		}
	}
	if err := json.Unmarshal([]byte(valid+` {}`), &got); err == nil {
		t.Fatal("trailing JSON value accepted")
	}
	invalidUTF8 := append([]byte(valid[:10]), append([]byte{0xff}, []byte(valid[10:])...)...)
	if err := json.Unmarshal(invalidUTF8, &got); !errors.Is(err, ErrInvalid) {
		t.Errorf("invalid UTF-8 error=%v, want ErrInvalid", err)
	}
}

func TestInputAndCheckInputClosedAndValidated(t *testing.T) {
	input := `{"version":1,"config":` + validConfigJSON + `,"reason":"admin policy update"}`
	var in Input
	if err := json.Unmarshal([]byte(input), &in); err != nil || in.Version != 1 {
		t.Fatalf("valid input: %#v, %v", in, err)
	}
	for _, raw := range []string{
		`{"version":0,"config":{"age_enabled":false,"minimum_age":null,"region_enabled":false,"allowed_countries":[],"identity_enabled":false,"account_risk_enabled":false,"betting_risk_enabled":false,"exclusion_enabled":false,"responsible_gambling_enabled":false},"reason":"valid"}`,
		`{"version":1,"config":{"age_enabled":false,"minimum_age":null,"region_enabled":false,"allowed_countries":[],"identity_enabled":false,"account_risk_enabled":false,"betting_risk_enabled":false,"exclusion_enabled":false,"responsible_gambling_enabled":false},"reason":"valid","x":0}`,
		`{"version":1,"version":2,"config":{"age_enabled":false,"minimum_age":null,"region_enabled":false,"allowed_countries":[],"identity_enabled":false,"account_risk_enabled":false,"betting_risk_enabled":false,"exclusion_enabled":false,"responsible_gambling_enabled":false},"reason":"valid"}`,
		`{"version":1,"config":null,"reason":"valid"}`,
		`{"version":1.5,"config":{"age_enabled":false,"minimum_age":null,"region_enabled":false,"allowed_countries":[],"identity_enabled":false,"account_risk_enabled":false,"betting_risk_enabled":false,"exclusion_enabled":false,"responsible_gambling_enabled":false},"reason":"valid"}`,
		`{"version":"1","config":{"age_enabled":false,"minimum_age":null,"region_enabled":false,"allowed_countries":[],"identity_enabled":false,"account_risk_enabled":false,"betting_risk_enabled":false,"exclusion_enabled":false,"responsible_gambling_enabled":false},"reason":"valid"}`,
	} {
		if err := json.Unmarshal([]byte(raw), &in); !errors.Is(err, ErrInvalid) {
			t.Errorf("Input.Unmarshal(%s) error=%v", raw, err)
		}
	}

	validCheck := `{"version":2,"operation":"withdrawal","reason":"manual review"}`
	var check CheckInput
	if err := json.Unmarshal([]byte(validCheck), &check); err != nil || check.Operation != "withdrawal" {
		t.Fatalf("valid check input: %#v, %v", check, err)
	}
	for _, raw := range []string{
		`{}`, `{"version":1,"operation":"registration","reason":"ok","extra":1}`,
		`{"version":1,"version":1,"operation":"betting","reason":"ok"}`,
		`{"version":1,"operation":"unknown","reason":"ok"}`,
		`{"version":-1,"operation":"betting","reason":"ok"}`,
		`{"version":1,"operation":null,"reason":"ok"}`,
		`{"version":1,"operation":"betting","reason":null}`,
	} {
		if err := json.Unmarshal([]byte(raw), &check); !errors.Is(err, ErrInvalid) {
			t.Errorf("CheckInput.Unmarshal(%s) error=%v", raw, err)
		}
	}
}

func TestValidReasonAndOperation(t *testing.T) {
	for _, reason := range []string{"reason", "reason with internal spaces", strings.Repeat("x", 500), "理由"} {
		if !ValidReason(reason) {
			t.Errorf("valid reason rejected: %q", reason)
		}
	}
	for _, reason := range []string{"", " leading", "trailing ", "\tinside", "line\nbreak", "nul\x00byte", strings.Repeat("x", 501), "\u00a0trimmed\u00a0"} {
		if ValidReason(reason) {
			t.Errorf("invalid reason accepted: %q", reason)
		}
	}
	for _, op := range []string{"registration", "betting", "withdrawal"} {
		if !ValidOperation(op) {
			t.Errorf("valid operation rejected: %q", op)
		}
	}
	for _, op := range []string{"", "Registration", "deposit", "bet", "registration "} {
		if ValidOperation(op) {
			t.Errorf("invalid operation accepted: %q", op)
		}
	}
	for _, decision := range []string{"allow", "review", "deny", "freeze"} {
		if !ValidDecision(decision) {
			t.Errorf("valid decision rejected: %q", decision)
		}
	}
	if ValidDecision("approve") {
		t.Fatal("unknown decision accepted")
	}
}

func TestEvaluateAllFeatureCombinations(t *testing.T) {
	names := []string{"age", "region", "identity", "account_risk", "betting_risk", "exclusion", "responsible_gambling"}
	for mask := 0; mask < 1<<len(names); mask++ {
		c := Config{AgeEnabled: mask&1 != 0, MinimumAge: age(18), RegionEnabled: mask&2 != 0, AllowedCountries: []string{"US"}, IdentityEnabled: mask&4 != 0, AccountRiskEnabled: mask&8 != 0, BettingRiskEnabled: mask&16 != 0, ExclusionEnabled: mask&32 != 0, ResponsibleGamblingEnabled: mask&64 != 0}
		got, checks, err := Evaluate(c)
		if err != nil {
			t.Fatalf("mask %03b: %v", mask, err)
		}
		want := "allow"
		if mask != 0 {
			want = "review"
		}
		if got != want || len(checks) != len(names) {
			t.Fatalf("mask %03b: decision=%q checks=%#v", mask, got, checks)
		}
		for i, enabled := range []bool{c.AgeEnabled, c.RegionEnabled, c.IdentityEnabled, c.AccountRiskEnabled, c.BettingRiskEnabled, c.ExclusionEnabled, c.ResponsibleGamblingEnabled} {
			wantDecision, wantCode := "allow", "CHECK_DISABLED"
			if enabled {
				wantDecision, wantCode = "review", "ADAPTER_NOT_CONFIGURED"
			}
			if checks[i] != (Check{Check: names[i], Enabled: enabled, Decision: wantDecision, ReasonCode: wantCode}) {
				t.Errorf("mask %03b check %d: got %#v", mask, i, checks[i])
			}
		}
	}
	if _, checks, err := Evaluate(Config{AgeEnabled: true}); !errors.Is(err, ErrInvalid) || checks != nil {
		t.Fatalf("invalid config result checks=%#v err=%v", checks, err)
	}
}

func TestDefaultAndEvaluateResultsAreIsolated(t *testing.T) {
	first := DefaultConfig()
	first.AllowedCountries = append(first.AllowedCountries, "US")
	second := DefaultConfig()
	if len(second.AllowedCountries) != 0 {
		t.Fatal("DefaultConfig shared country slice")
	}
	_, checks, err := Evaluate(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	checks[0].Check = "mutated"
	_, again, err := Evaluate(DefaultConfig())
	if err != nil || again[0].Check != "age" {
		t.Fatalf("Evaluate shared result state: %#v, %v", again, err)
	}
}

func TestStubAdapterDoesNotMakeDecisionsFromUserData(t *testing.T) {
	for _, name := range []string{"age", "region", "identity", "account_risk", "betting_risk", "exclusion", "responsible_gambling"} {
		got, err := (StubAdapter{}).Check(nil, name, Config{})
		if err != nil || got != (Check{Check: name, Enabled: true, Decision: "review", ReasonCode: "ADAPTER_NOT_CONFIGURED"}) {
			t.Errorf("stub %q: %#v, %v", name, got, err)
		}
	}
	if _, err := (StubAdapter{}).Check(nil, "unknown", Config{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown stub check error=%v", err)
	}
}
