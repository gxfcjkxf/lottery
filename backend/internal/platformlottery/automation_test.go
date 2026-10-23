package platformlottery

import (
	"testing"
	"time"
)

func TestAutomaticSlotsKeepAnchorAndLeadingZeros(t *testing.T) {
	anchor := time.Date(2026, 10, 16, 2, 0, 0, 0, time.UTC)
	first := "000999"
	interval := 120
	p := AutomationPlan{SourceType: "mapped", Mapping: &FeedMapping{Format: "json", Schedule: "interval", TimeFormat: "rfc3339", Period: FeedField{Path: "/period_no"}, DrawTime: FeedField{Path: "/drawn_at"}, Regular: &FeedNumbers{FeedField: FeedField{Path: "/result/regular"}, Count: 6}, Special: &FeedNumbers{FeedField: FeedField{Path: "/result/special"}, Count: 1}}, AnchorDrawAt: &anchor, AnchorPeriodNo: &first, IntervalSeconds: &interval, CloseBeforeSeconds: 15}
	slot, e := automationSlot(p, anchor.Add(3*time.Minute), 0)
	if e != nil || slot.PeriodNo != "001001" || !slot.DrawAt.Equal(anchor.Add(4*time.Minute)) || !slot.BetEndAt.Equal(anchor.Add(225*time.Second)) || !slot.BetStartAt.Equal(anchor.Add(2*time.Minute)) {
		t.Fatalf("bad anchored slot %+v %v", slot, e)
	}
	next, e := automationSlot(p, anchor.Add(3*time.Minute), 1)
	if e != nil || next.PeriodNo != "001002" || !next.BetStartAt.Equal(slot.DrawAt) {
		t.Fatalf("cycle gap %+v %v", next, e)
	}
}
func TestAutomaticPlanRejectsInvalidSourceAndClose(t *testing.T) {
	anchor := time.Now()
	first := "20260001"
	interval := 120
	p := AutomationPlan{SourceType: "mapped", Mapping: &FeedMapping{Format: "json", Schedule: "interval", TimeFormat: "rfc3339", Period: FeedField{Path: "/period_no"}, DrawTime: FeedField{Path: "/drawn_at"}, Regular: &FeedNumbers{FeedField: FeedField{Path: "/result/regular"}, Count: 6}, Special: &FeedNumbers{FeedField: FeedField{Path: "/result/special"}, Count: 1}}, AnchorDrawAt: &anchor, AnchorPeriodNo: &first, IntervalSeconds: &interval, CloseBeforeSeconds: 15, SourceURL: "https://draw.vendor.com/result"}
	if ValidateAutomation(p) != nil {
		t.Fatal("valid plan rejected")
	}
	for _, mutate := range []func(*AutomationPlan){func(p *AutomationPlan) { p.CloseBeforeSeconds = 120 }, func(p *AutomationPlan) { p.SourceURL = "https://secret@draw.vendor.com" }, func(p *AutomationPlan) { v := "day-001"; p.AnchorPeriodNo = &v }, func(p *AutomationPlan) { p.CredentialRef = "AUTH_KEY" }} {
		invalid := p
		mutate(&invalid)
		if ValidateAutomation(invalid) == nil {
			t.Fatal("invalid plan accepted")
		}
	}
}

func TestNewAutomationRejectsRetiredProtocols(t *testing.T) {
	for _, source := range []string{"interval_json", "hk_marksix_6hch"} {
		if err := ValidateAutomation(AutomationPlan{SourceType: source, SourceURL: "https://draw.example/smallSix/findSmallSixInfo.do"}); err == nil {
			t.Fatal("retired source accepted", source)
		}
	}
}
