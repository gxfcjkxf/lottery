package commission

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestPolicyDefaultsAndExactJSONShape(t *testing.T) {
	got := DefaultPolicyConfig()
	if got.Enabled || got.Calendar != nil || got.PayoutMode != PayoutManual {
		t.Fatalf("DefaultPolicyConfig() = %+v", got)
	}
	raw, err := json.Marshal(got)
	if err != nil || string(raw) != `{"enabled":false,"calendar":null,"payout_mode":"manual"}` {
		t.Fatalf("default JSON = %s, err=%v", raw, err)
	}
	weekday := 2
	config := PolicyConfig{Enabled: true, Calendar: &Calendar{Timezone: "Asia/Manila", Cycle: "weekly", BoundaryTime: "18:30:00", Weekday: &weekday}, PayoutMode: PayoutAutomatic}
	raw, err = json.Marshal(config)
	if err != nil || string(raw) != `{"enabled":true,"calendar":{"timezone":"Asia/Manila","cycle":"weekly","boundary_time":"18:30:00","weekday":2,"month_day":null,"short_month":""},"payout_mode":"automatic"}` {
		t.Fatalf("config JSON = %s, err=%v", raw, err)
	}
	var decoded PolicyConfig
	if err = json.Unmarshal(raw, &decoded); err != nil || !reflect.DeepEqual(decoded, config) {
		t.Fatalf("round trip = %+v, err=%v", decoded, err)
	}
}

func TestCommissionPolicyStrictJSONAndValidation(t *testing.T) {
	valid := `{"version":1,"config":{"enabled":false,"calendar":null,"payout_mode":"manual"},"reason":"initialize"}`
	var input PolicyInput
	if err := json.Unmarshal([]byte(valid), &input); err != nil || input.Validate() != nil {
		t.Fatalf("valid input rejected: %+v, %v", input, err)
	}
	badInputs := []string{
		`{}`, `null`, valid + ` {}`,
		`{"version":1,"version":1,"config":{"enabled":false,"calendar":null,"payout_mode":"manual"},"reason":"x"}`,
		`{"version":1,"config":{"enabled":false,"calendar":null,"payout_mode":"manual","extra":true},"reason":"x"}`,
		`{"version":1,"config":{"enabled":false,"calendar":null,"payout_mode":"manual"},"reason":null}`,
		`{"version":0,"config":{"enabled":false,"calendar":null,"payout_mode":"manual"},"reason":"x"}`,
		`{"version":1,"config":{"enabled":false,"calendar":null,"payout_mode":"automatic"},"reason":"  "}`,
		`{"version":1,"config":{"enabled":false,"calendar":null,"payout_mode":"auto"},"reason":"x"}`,
		`{"version":1,"config":{"enabled":true,"calendar":null,"payout_mode":"manual"},"reason":"x"}`,
		`{"version":1,"config":{"enabled":false,"calendar":{"timezone":"UTC","cycle":"weekly","boundary_time":"00:00:00","weekday":1,"month_day":null,"short_month":"","extra":0},"payout_mode":"manual"},"reason":"x"}`,
		`{"version":1,"config":{"enabled":false,"calendar":{"timezone":"UTC","cycle":"weekly","boundary_time":"00:00:00","weekday":1,"weekday":2,"month_day":null,"short_month":""},"payout_mode":"manual"},"reason":"x"}`,
		`{"version":1,"config":{"enabled":false,"calendar":{"timezone":"Local","cycle":"weekly","boundary_time":"00:00:00","weekday":1,"month_day":null,"short_month":""},"payout_mode":"manual"},"reason":"x"}`,
	}
	for _, raw := range badInputs {
		t.Run(raw, func(t *testing.T) {
			var got PolicyInput
			if err := json.Unmarshal([]byte(raw), &got); err == nil {
				t.Fatalf("invalid policy input accepted: %s", raw)
			}
		})
	}
}

func TestPolicyCalendarCycleShapeAndErrors(t *testing.T) {
	weekday := 1
	weekly := Calendar{Timezone: "UTC", Cycle: "weekly", BoundaryTime: "00:00:00", Weekday: &weekday}
	if weekly.Validate() != nil {
		t.Fatal("valid weekly calendar rejected")
	}
	day := 31
	monthly := Calendar{Timezone: "UTC", Cycle: "monthly", BoundaryTime: "00:00:00", MonthDay: &day, ShortMonth: "last_day"}
	if monthly.Validate() != nil {
		t.Fatal("valid monthly calendar rejected")
	}
	if err := (PolicyConfig{Enabled: true, Calendar: &weekly, PayoutMode: PayoutManual}).Validate(); err != nil {
		t.Fatal("enabled valid calendar rejected", err)
	}
	invalid := []PolicyConfig{
		{Enabled: true, PayoutMode: PayoutManual},
		{PayoutMode: "unsupported"},
		{Calendar: &Calendar{Timezone: "UTC", Cycle: "weekly", BoundaryTime: "00:00:00", MonthDay: &day, ShortMonth: "last_day"}, PayoutMode: PayoutManual},
	}
	for _, config := range invalid {
		if !errors.Is(config.Validate(), ErrInvalid) {
			t.Fatalf("invalid config accepted: %+v", config)
		}
	}
	wrongCycle := Calendar{Timezone: "UTC", Cycle: "weekly", BoundaryTime: "00:00:00", MonthDay: &day, ShortMonth: "last_day"}
	if !errors.Is(wrongCycle.Validate(), ErrInvalid) {
		t.Fatal("invalid weekly/monthly field mix accepted")
	}
}
