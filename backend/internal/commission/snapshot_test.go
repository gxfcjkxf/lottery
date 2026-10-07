package commission

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
)

func snapshotFixture() BetSnapshot {
	d := 1
	root, child := ids.New(), ids.New()
	return BetSnapshot{
		SchemaVersion: 1, BrandID: ids.New(), MemberID: ids.New(), CapturedAt: time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC),
		Financial: FinancialRule{ids.New(), "2", PolicyConfig{true, &Calendar{Timezone: "UTC", Cycle: "weekly", BoundaryTime: "00:00:00", Weekday: &d}, PayoutManual}},
		Agency:    AgencyRule{ids.New(), "2", agency.PolicyConfig{Enabled: true, MaxDepth: 5, RatioCap: "0.2", Mode: "loss", Cycle: "weekly"}},
		Path: []AgentRule{
			{ID: root, MemberID: ids.New(), Depth: 1, RevisionID: ids.New(), Version: "1", Config: agency.NodeConfig{Ratio: "0.1", Status: "disabled"}, EffectiveMode: "loss"},
			{ID: child, MemberID: ids.New(), ParentID: &root, Depth: 2, RevisionID: ids.New(), Version: "1", Config: agency.NodeConfig{Ratio: "0.05", Status: "active"}, EffectiveMode: "loss"},
		},
	}
}

func TestParseBetSnapshotStrictEvidenceAndDisabledEarnings(t *testing.T) {
	valid := snapshotFixture()
	raw, _ := json.Marshal(valid)
	got, err := ParseBetSnapshot(raw, valid.BrandID, valid.MemberID, valid.CapturedAt)
	if err != nil || len(got.Path) != 2 || got.Path[0].Config.Status != "disabled" {
		t.Fatal(got, err)
	}
	for _, bad := range [][]byte{nil, []byte("null"), []byte("{}"), append(append([]byte(nil), raw...), []byte(" {}")...), []byte(strings.Replace(string(raw), `"schema_version":1`, `"schema_version":1,"schema_version":1`, 1))} {
		if _, err = ParseBetSnapshot(bad, valid.BrandID, valid.MemberID, valid.CapturedAt); !errors.Is(err, ErrPolicyEvidence) {
			t.Fatal("malformed snapshot accepted", err)
		}
	}
	for name, mutate := range map[string]func(*BetSnapshot){
		"schema":               func(s *BetSnapshot) { s.SchemaVersion = 2 },
		"brand":                func(s *BetSnapshot) { s.BrandID = ids.New() },
		"member":               func(s *BetSnapshot) { s.MemberID = ids.New() },
		"timestamp":            func(s *BetSnapshot) { s.CapturedAt = s.CapturedAt.Add(time.Nanosecond) },
		"version":              func(s *BetSnapshot) { s.Path[0].Version = "01" },
		"overflow version":     func(s *BetSnapshot) { s.Financial.Version = "9223372036854775808" },
		"nil path":             func(s *BetSnapshot) { s.Path = nil },
		"ratio increase":       func(s *BetSnapshot) { s.Path[1].Config.Ratio = "0.2" },
		"ratio cap":            func(s *BetSnapshot) { s.Agency.Config.RatioCap = "0.05" },
		"depth":                func(s *BetSnapshot) { s.Path[1].Depth = 3 },
		"duplicate member":     func(s *BetSnapshot) { s.Path[1].MemberID = s.Path[0].MemberID },
		"duplicate agent":      func(s *BetSnapshot) { s.Path[1].ID = s.Path[0].ID },
		"mixed mode":           func(s *BetSnapshot) { v := "turnover"; s.Path[1].Config.Mode = &v; s.Path[1].EffectiveMode = v },
		"wrong effective mode": func(s *BetSnapshot) { s.Path[1].EffectiveMode = "turnover" },
		"missing parent":       func(s *BetSnapshot) { s.Path[1].ParentID = nil },
		"disabled agency":      func(s *BetSnapshot) { s.Agency.Config.Enabled = false },
		"different cycle":      func(s *BetSnapshot) { s.Agency.Config.Cycle = "monthly" },
	} {
		t.Run(name, func(t *testing.T) {
			s := snapshotFixture()
			s.BrandID = valid.BrandID
			s.MemberID = valid.MemberID
			mutate(&s)
			b, _ := json.Marshal(s)
			if _, e := ParseBetSnapshot(b, valid.BrandID, valid.MemberID, valid.CapturedAt); !errors.Is(e, ErrPolicyEvidence) {
				t.Fatal("invalid snapshot accepted", e)
			}
		})
	}
	// A root override remains legal when all descendants inherit it.
	v := "turnover"
	valid.Path[0].Config.Mode = &v
	valid.Path[0].EffectiveMode = v
	valid.Path[1].EffectiveMode = v
	raw, _ = json.Marshal(valid)
	if _, err = ParseBetSnapshot(raw, valid.BrandID, valid.MemberID, valid.CapturedAt); err != nil {
		t.Fatal("root override rejected", err)
	}
	valid.Path = []AgentRule{}
	raw, _ = json.Marshal(valid)
	if _, err = ParseBetSnapshot(raw, valid.BrandID, valid.MemberID, valid.CapturedAt); err != nil {
		t.Fatal("unattributed member rejected", err)
	}
}
