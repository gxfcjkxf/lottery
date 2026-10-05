package rulebook

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

func TestPersistManualDrawForEachNumberModel(t *testing.T) {
	cases := []struct {
		name        string
		model       rules.Model
		input, want rules.Draw
	}{
		{"ordinary_special", rules.Model{Type: "X_PLUS_Y", RegularPool: rules.Pool{Min: 1, Max: 49}, SpecialPool: rules.Pool{Min: 1, Max: 10}, RegularCount: 2, SpecialCount: 1}, rules.Draw{Regular: []int{12, 1}, Special: []int{7}}, rules.Draw{Regular: []int{1, 12}, Special: []int{7}, Digits: []int{}}},
		{"select_m", rules.Model{Type: "M_SELECT_N", PoolSize: 5, TotalCount: 3, RegularCount: 2, SpecialCount: 1, RegularPool: rules.Pool{Min: 1, Max: 5}, SpecialPool: rules.Pool{Min: 1, Max: 5}}, rules.Draw{Regular: []int{2, 1}, Special: []int{5}}, rules.Draw{Regular: []int{1, 2}, Special: []int{5}, Digits: []int{}}},
		{"digits", rules.Model{Type: "DIGITS_0_9", Length: 3, Ordered: true, AllowRepeat: true}, rules.Draw{Digits: []int{3, 1, 0}}, rules.Draw{Regular: []int{}, Special: []int{}, Digits: []int{3, 1, 0}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, creator, _, _, _, _ := fixture(t)
			a := drawActor(creator.ID)
			ctx := context.Background()
			var g Game
			transact(t, s.DB, func(tx pgx.Tx) error {
				var e error
				g, e = s.CreateGame(ctx, tx, brand, a, tc.name, tc.name, tc.model, "UTC", "model persistence regression", points.Metadata{})
				return e
			})
			now := databaseNow(t, s)
			p := waitingDrawPeriod(t, s, g, "model-001", 1, now.Add(-2*time.Hour), "waiting_draw")
			var out DrawResult
			transact(t, s.DB, func(tx pgx.Tx) error {
				var e error
				out, e = s.ManualDraw(ctx, tx, brand, a, p.ID, p.Version, p.PeriodNo, tc.input, now.Add(-time.Minute), "verified test model", points.Metadata{})
				return e
			})
			if !reflect.DeepEqual(out.Result, tc.want) {
				t.Fatalf("canonical result=%+v want=%+v", out.Result, tc.want)
			}
			history, e := s.Draw(ctx, brand, p.ID, 50, 0)
			if e != nil || history.Current == nil || history.Current.ID != out.ID || !reflect.DeepEqual(history.Current.Result, tc.want) || history.Current.ResultHash != out.ResultHash {
				t.Fatalf("persisted model=%+v err=%v", history, e)
			}
		})
	}
}
