package commission

import (
	"math"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

func evaluationFact(snapshot BetSnapshot, status string, stake, prize points.Amount) OrderFact {
	basis, _ := Bases(status, stake, prize)
	return OrderFact{BrandID: snapshot.BrandID, MemberID: snapshot.MemberID, PlacedAt: snapshot.CapturedAt, Status: status, PrizePoints: prize, Basis: basis}
}

func setEvaluationMode(snapshot *BetSnapshot, mode string) {
	snapshot.Agency.Config.Mode = mode
	for i := range snapshot.Path {
		snapshot.Path[i].EffectiveMode = mode
		if snapshot.Path[i].Config.Mode != nil {
			v := mode
			snapshot.Path[i].Config.Mode = &v
		}
	}
}

func addEvaluationLevel(snapshot *BetSnapshot, ratio string, status string) {
	parent := snapshot.Path[len(snapshot.Path)-1].ID
	snapshot.Path = append(snapshot.Path, AgentRule{
		ID: ids.New(), MemberID: ids.New(), ParentID: &parent, Depth: len(snapshot.Path) + 1,
		RevisionID: ids.New(), Version: "1", Config: agency.NodeConfig{Ratio: ratio, Status: status},
		EffectiveMode: snapshot.Path[0].EffectiveMode,
	})
}

func TestEvaluateOrderTwoAndThreeLevelLossAndTurnover(t *testing.T) {
	for _, tc := range []struct {
		name  string
		mode  string
		depth int
		want  []ExactAmount
	}{
		{"two level loss", "loss", 2, []ExactAmount{{"5", "1"}, {"5", "1"}}},
		{"three level loss", "loss", 3, []ExactAmount{{"5", "1"}, {"3", "1"}, {"2", "1"}}},
		{"two level turnover", "turnover", 2, []ExactAmount{{"5", "1"}, {"5", "1"}}},
		{"three level turnover", "turnover", 3, []ExactAmount{{"5", "1"}, {"3", "1"}, {"2", "1"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := snapshotFixture()
			setEvaluationMode(&snapshot, tc.mode)
			if tc.depth == 3 {
				addEvaluationLevel(&snapshot, "0.02", "active")
			}
			status := "lost"
			if tc.mode == "turnover" {
				status = "won"
			}
			fact := evaluationFact(snapshot, status, 100, 0)
			got, err := EvaluateOrder(snapshot, fact)
			if err != nil || len(got) != tc.depth {
				t.Fatalf("EvaluateOrder() = %#v, %v", got, err)
			}
			for i, allocation := range got {
				if allocation.AgentID != snapshot.Path[i].ID || allocation.MemberID != snapshot.Path[i].MemberID || allocation.Exact != tc.want[i] {
					t.Fatalf("allocation[%d] = %#v, want node=%#v exact=%#v", i, allocation, snapshot.Path[i], tc.want[i])
				}
			}
		})
	}
}

func TestEvaluateOrderWonPrizeDoesNotChangeTurnoverAndLossIsZero(t *testing.T) {
	snapshot := snapshotFixture()
	setEvaluationMode(&snapshot, "turnover")
	for _, prize := range []points.Amount{0, 10, 25, 40} { // zero, below, tie, and above stake
		fact := evaluationFact(snapshot, "won", 25, prize)
		got, err := EvaluateOrder(snapshot, fact)
		if err != nil || len(got) != 2 || got[0].Exact != (ExactAmount{"5", "4"}) || got[1].Exact != (ExactAmount{"5", "4"}) {
			t.Fatalf("prize=%d allocations=%#v error=%v", prize, got, err)
		}
	}
	setEvaluationMode(&snapshot, "loss")
	zero, err := EvaluateOrder(snapshot, evaluationFact(snapshot, "won", 25, 0))
	if err != nil {
		t.Fatal(err)
	}
	for _, allocation := range zero {
		if allocation.Exact != (ExactAmount{"0", "1"}) {
			t.Fatalf("loss-mode win must preserve canonical zero node allocation: %#v", allocation)
		}
	}
}

func TestEvaluateOrderDisabledFinancialPolicyAndDisabledNodes(t *testing.T) {
	snapshot := snapshotFixture()
	fact := evaluationFact(snapshot, "lost", 100, 0)
	first, err := EvaluateOrder(snapshot, fact)
	if err != nil || len(first) != 2 || first[0].AgentID != snapshot.Path[0].ID {
		t.Fatalf("disabled node should retain saved earnings: %#v, %v", first, err)
	}
	second, err := EvaluateOrder(snapshot, fact)
	if err != nil || len(second) != len(first) {
		t.Fatalf("repeated evaluation = %#v, %v", second, err)
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("repeated evaluation changed allocation: %#v != %#v", first, second)
		}
	}

	snapshot.Financial.Config.Enabled = false
	got, err := EvaluateOrder(snapshot, fact)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("disabled financial policy = %#v, %v; want nonnil empty", got, err)
	}
}

func TestEvaluateOrderEmptyPathAndExactLargestBasis(t *testing.T) {
	snapshot := snapshotFixture()
	snapshot.Path = []AgentRule{}
	got, err := EvaluateOrder(snapshot, evaluationFact(snapshot, "lost", 1, 0))
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty path = %#v, %v; want nonnil empty", got, err)
	}

	snapshot = snapshotFixture()
	snapshot.Path = snapshot.Path[:1]
	snapshot.Path[0].Config.Ratio = "1"
	snapshot.Agency.Config.RatioCap = "1"
	got, err = EvaluateOrder(snapshot, evaluationFact(snapshot, "lost", points.Amount(math.MaxInt64), 0))
	want := ExactAmount{"9223372036854775807", "1"}
	if err != nil || len(got) != 1 || got[0].Exact != want {
		t.Fatalf("largest exact basis = %#v, %v; want %#v", got, err, want)
	}
}

func TestEvaluateOrderRejectsUnboundSnapshotAndUnverifiedBasis(t *testing.T) {
	snapshot := snapshotFixture()
	fact := evaluationFact(snapshot, "lost", 50, 0)
	cases := []struct {
		name   string
		mutate func(*BetSnapshot, *OrderFact)
	}{
		{"brand mismatch", func(_ *BetSnapshot, f *OrderFact) { f.BrandID = ids.New() }},
		{"member mismatch", func(_ *BetSnapshot, f *OrderFact) { f.MemberID = ids.New() }},
		{"placed time mismatch", func(_ *BetSnapshot, f *OrderFact) { f.PlacedAt = f.PlacedAt.Add(time.Nanosecond) }},
		{"snapshot ratio invalid", func(s *BetSnapshot, _ *OrderFact) { s.Path[0].Config.Ratio = "0.10" }},
		{"snapshot mode invalid", func(s *BetSnapshot, _ *OrderFact) { s.Path[1].EffectiveMode = "turnover" }},
		{"basis mismatch", func(_ *BetSnapshot, f *OrderFact) { f.Basis.LossPoints-- }},
		{"invalid status", func(_ *BetSnapshot, f *OrderFact) { f.Status = "pending" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := snapshot
			f := fact
			tc.mutate(&s, &f)
			if got, err := EvaluateOrder(s, f); err == nil || got != nil {
				t.Fatalf("invalid evidence returned %#v, %v", got, err)
			}
		})
	}
}
