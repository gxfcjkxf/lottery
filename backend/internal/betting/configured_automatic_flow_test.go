package betting

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

// Both DEV feed shapes run through saved mappings, actual reservation and collection,
// and the configured settlement worker. No result is published by the test itself.
func TestConfiguredAutomaticMappedFeedToWallet(t *testing.T) {
	for _, schedule := range []string{"interval", "source"} {
		t.Run(schedule, func(t *testing.T) {
			ctx := context.Background()
			var c configuredFixture
			expectedPrize := points.Amount(2500)
			if schedule == "interval" {
				catalog, err := rules.ConfiguredTemplateCatalog("canada_28")
				if err != nil {
					t.Fatal(err)
				}
				prices := map[string]string{}
				for _, play := range catalog.Plays {
					for _, outcome := range play.Decisions.Outcomes {
						if outcome.PriceKey != "" {
							prices[outcome.PriceKey] = "2"
						}
					}
				}
				engine, err := rules.NewConfiguredEngine(catalog, rules.ConfiguredBindings{Version: "automatic-flow", Attributes: map[string]map[string][]int{}}, rules.ConfiguredPrices{Version: "automatic-flow", Plates: map[string]map[string]string{"A": prices}})
				if err != nil {
					t.Fatal(err)
				}
				cases := []platformlottery.ConfiguredCase{{Requests: []rules.ConfiguredRequest{{PlayID: "sum_14", Options: []string{"sum_14"}, Plate: "A", Points: 1000, Multiplier: 1}}, Draw: rules.Draw{Digits: []int{5, 4, 5}}, Bet: 1000, Prize: 2000}}
				c = configuredFixtureFrom(t, engine.Snapshot(), cases)
				expectedPrize = 2000
			} else {
				c = newConfiguredFixture(t)
			}
			drawAt := time.Now().UTC().Add(9 * time.Second).Truncate(time.Second)
			var complete atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if schedule == "interval" {
					if r.URL.Query().Get("game_code") != c.g.Code || r.URL.Query().Get("period_no") != "920001" {
						http.Error(w, "unexpected query", 400)
						return
					}
					if !complete.Load() {
						w.WriteHeader(204)
						return
					}
					json.NewEncoder(w).Encode(map[string]any{"period_no": "920001", "drawn_at": drawAt.Format(time.RFC3339), "result": map[string]any{"digits": []int{5, 4, 5}}})
				} else {
					issue, next, at, nextAt, kind := "2026000", "920001", drawAt.Add(-time.Hour), drawAt, 1
					if complete.Load() {
						issue, next, at, nextAt, kind = "920001", "920002", drawAt, drawAt.Add(time.Hour), 8
					}
					json.NewEncoder(w).Encode(map[string]any{"errorCode": 0, "result": map[string]any{"businessCode": 0, "data": map[string]any{"preDrawIssue": issue, "drawIssue": next, "preDrawTime": at.In(time.FixedZone("HK", 8*3600)).Format("2006-01-02 15:04:05"), "drawTime": nextAt.In(time.FixedZone("HK", 8*3600)).Format("2006-01-02 15:04:05"), "preDrawCode": "1,3,7,4,5,6,2", "type": kind}}})
				}
			}))
			defer server.Close()
			field := func(path string) platformlottery.FeedField { return platformlottery.FeedField{Path: path} }
			m := &platformlottery.FeedMapping{Format: "json", Schedule: schedule, TimeFormat: "rfc3339", Period: field("/period_no"), DrawTime: field("/drawn_at"), Digits: &platformlottery.FeedNumbers{FeedField: field("/result/digits"), Count: 3}, PeriodQuery: "period_no", GameQuery: "game_code"}
			interval, opening, number := 10, 10, "920001"
			plan := platformlottery.AutomationPlan{SourceType: "mapped", Mapping: m, Enabled: true, SourceURL: server.URL, AnchorDrawAt: &drawAt, AnchorPeriodNo: &number, IntervalSeconds: &interval, OpenBeforeSeconds: &opening, CloseBeforeSeconds: 1}
			if schedule == "source" {
				m.TimeFormat, m.TimeLayout, m.Timezone = "custom", "2006-01-02 15:04:05", "Asia/Hong_Kong"
				m.Period, m.DrawTime = field("/result/data/preDrawIssue"), field("/result/data/preDrawTime")
				m.Digits = nil
				m.Regular = &platformlottery.FeedNumbers{FeedField: field("/result/data/preDrawCode"), Count: 6, Separator: ","}
				m.Special = &platformlottery.FeedNumbers{FeedField: field("/result/data/preDrawCode"), Offset: 6, Count: 1, Separator: ","}
				m.NextPeriod, m.NextDrawTime = new(platformlottery.FeedField), new(platformlottery.FeedField)
				*m.NextPeriod, *m.NextDrawTime = field("/result/data/drawIssue"), field("/result/data/drawTime")
				m.Complete = &platformlottery.FeedCondition{FeedField: field("/result/data/type"), Equals: "8"}
				m.PeriodQuery, m.GameQuery = "", ""
				plan.AnchorDrawAt, plan.AnchorPeriodNo, plan.IntervalSeconds = nil, nil, nil
			}
			var source platformlottery.DrawSource
			bettingTx(t, c.f.db, func(tx pgx.Tx) error {
				var err error
				source, err = c.s.SaveDrawSource(ctx, tx, c.a, c.g.ID, platformlottery.DrawSourceInput{DrawSource: platformlottery.DrawSource{Name: "Mapped flow", Plan: plan}}, points.Metadata{})
				return err
			})
			bettingTx(t, c.f.db, func(tx pgx.Tx) error {
				_, err := c.s.ActivateDrawSource(ctx, tx, c.a, c.g.ID, source.ID, platformlottery.DrawSourceActivation{Revision: source.Revision}, points.Metadata{})
				return err
			})
			sources := platformlottery.AutomationSources{Origins: []string{server.URL}, Client: server.Client()}
			if schedule == "interval" {
				if n, err := c.s.GenerateAutomaticPeriods(ctx); err != nil || n != 2 {
					t.Fatal("automatic reservation", n, err)
				}
				if n, err := c.s.GenerateAutomaticPeriods(ctx); err != nil || n != 0 {
					t.Fatal("reservation replay", n, err)
				}
			} else if n, err := c.s.CollectAutomaticDraws(ctx, sources); err != nil || n != 0 {
				t.Fatal("announced reservation", n, err)
			}
			var periodID string
			if err := c.f.db.QueryRow(ctx, `SELECT id::text FROM platform_game_periods WHERE game_id=$1 AND period_no='920001' AND automation_version=1`, c.g.ID).Scan(&periodID); err != nil {
				t.Fatal(err)
			}
			fundBettingWallet(t, c.f, 10000)
			c.input.PeriodID = periodID
			first, err := configuredPlace(t, c, "automatic-flow-bet", c.input)
			if err != nil {
				t.Fatal(err)
			}
			replay, err := configuredPlace(t, c, "automatic-flow-bet", c.input)
			if err != nil || string(first.Data) != string(replay.Data) {
				t.Fatal("debit replay", err)
			}
			time.Sleep(time.Until(drawAt.Add(50 * time.Millisecond)))
			if n, err := c.s.CollectAutomaticDraws(ctx, sources); err != nil || n != 0 {
				t.Fatal("incomplete feed", n, err)
			}
			if n, err := c.f.service.ProcessConfiguredSettlements(ctx, 10); err != nil || n != 0 {
				t.Fatal("no result must not settle", n, err)
			}
			wallet, err := (points.Store{DB: c.f.db}).Read(ctx, c.f.brand, c.f.member)
			if err != nil || wallet.AvailablePoints != 9000 {
				t.Fatal("funds after incomplete feed", wallet, err)
			}
			time.Sleep(5100 * time.Millisecond)
			complete.Store(true)
			if n, err := c.s.CollectAutomaticDraws(ctx, sources); err != nil || n != 1 {
				t.Fatal("completed collection", n, err)
			}
			done := make(chan int, 2)
			failures := make(chan error, 2)
			for i := 0; i < 2; i++ {
				go func() { n, err := c.f.service.ProcessConfiguredSettlements(ctx, 10); done <- n; failures <- err }()
			}
			total := <-done + <-done
			for i := 0; i < 2; i++ {
				if err := <-failures; err != nil {
					t.Fatal(err)
				}
			}
			if total != 1 {
				t.Fatal("settled count", total)
			}
			if n, err := c.s.CollectAutomaticDraws(ctx, sources); err != nil || n != 0 {
				t.Fatal("collection replay", n, err)
			}
			if n, err := c.f.service.ProcessConfiguredSettlements(ctx, 10); err != nil || n != 0 {
				t.Fatal("settlement replay", n, err)
			}
			wallet, err = (points.Store{DB: c.f.db}).Read(ctx, c.f.brand, c.f.member)
			if err != nil || wallet.AvailablePoints != 9000+expectedPrize {
				t.Fatal("settled wallet", wallet, err)
			}
			var entries int
			if err := c.f.db.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries l JOIN configured_orders o ON l.reference_type='configured_order' AND l.reference_id=o.id WHERE o.period_id=$1`, periodID).Scan(&entries); err != nil || entries != 2 {
				t.Fatal("one bet and one payout", entries, err)
			}
		})
	}
}
