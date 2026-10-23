package betting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

// These fixtures represent purchases made before dedicated sales were retired.
// They insert explicit frozen terms and real ledger evidence through the normal
// database guards; no removed quoting, selling or admission code is retained.
func historicalPurchase(t *testing.T, kind string, sources ...platformlottery.AutomationPlan) (bettingFixture, platformlottery.Service, access.Account, platformlottery.Game, platformlottery.Period, string) {
	return historicalPurchaseWindow(t, kind, 2*time.Second, sources...)
}

func historicalPurchaseWindow(t *testing.T, kind string, window time.Duration, sources ...platformlottery.AutomationPlan) (bettingFixture, platformlottery.Service, access.Account, platformlottery.Game, platformlottery.Period, string) {
	t.Helper()
	ctx := context.Background()
	f := newBettingFixture(t, storeTestBrand)
	fundBettingWallet(t, f, 600, 0, 400)
	s := platformlottery.Service{DB: f.db}
	a := access.Account{ID: f.version.CreatedBy, Type: access.AccountAdmin, SuperAdmin: true, Roles: []access.Role{{Permissions: []access.Permission{{Resource: "platform_game", Action: "write", Scope: access.ScopePlatform}}}}}
	b := access.Account{ID: a.ID, Type: access.AccountAdmin, BrandIDs: []string{f.brand}, Roles: []access.Role{{BrandID: f.brand, Permissions: []access.Permission{{Resource: "platform_game", Action: "connect", Scope: access.ScopeBrand}}}}}
	model := rules.Model{Type: "DIGITS_0_9", Length: 3, AllowRepeat: true, Ordered: true}
	if kind == "mark_six" {
		model = rules.Model{Type: "X_PLUS_Y", RegularPool: rules.Pool{Min: 1, Max: 49}, SpecialPool: rules.Pool{Min: 1, Max: 49}, RegularCount: 6, SpecialCount: 1, Ordered: true}
	}
	var game platformlottery.Game
	var period platformlottery.Period
	var attributes platformlottery.MarkSixAttributes
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		var err error
		game, err = s.Create(ctx, tx, a, platformlottery.GameInput{Code: "historical", Name: "Historical game", Timezone: "UTC", Model: model}, points.Metadata{})
		if err != nil {
			return err
		}
		_, err = s.Connect(ctx, tx, b, f.brand, game.ID, "", points.Metadata{})
		if err != nil {
			return err
		}
		if kind == "mark_six" {
			waves, zodiac, elements := map[string]string{}, map[string]string{}, map[string]string{}
			for n := 1; n <= 49; n++ {
				key := fmt.Sprintf("%02d", n)
				waves[key], zodiac[key], elements[key] = "red", "horse", "water"
			}
			waves["49"] = "green"
			attributes, err = s.WriteMarkSixAttributes(ctx, tx, a, game.ID, platformlottery.MarkSixAttributesInput{Year: time.Now().Year(), EffectiveAt: time.Now().Add(-time.Hour), ExpiresAt: time.Now().AddDate(1, 0, 0), Waves: waves, Zodiac: zodiac, Elements: elements, Reason: "historical fixture"}, points.Metadata{})
			if err != nil {
				return err
			}
		}
		input := platformlottery.PeriodInput{PeriodNo: "historical-1", BetStartAt: time.Now().Add(-time.Hour), BetEndAt: time.Now().Add(window), DrawAt: time.Now().Add(window)}
		if len(sources) > 0 {
			plan := sources[0]
			version := int64(1)
			input.AutomationVersion = &version
			if plan.SourceType == "hk_marksix_6hch" {
				input.PeriodNo = fmt.Sprintf("%d001", time.Now().Year())
			}
			if _, err = tx.Exec(ctx, `INSERT INTO platform_automation_versions(game_id,version,enabled,source_type,anchor_draw_at,anchor_period_no,interval_seconds,close_before_seconds,source_url,credential_ref,created_by) VALUES($1,1,true,$2,$3,$4,$5,0,$6,'',$7)`, game.ID, plan.SourceType, plan.AnchorDrawAt, plan.AnchorPeriodNo, plan.IntervalSeconds, plan.SourceURL, a.ID); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO platform_automation_settings(game_id,version) VALUES($1,1)`, game.ID); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO brand_automation_settings(brand_id,game_id,version,rule_version,enabled,close_before_seconds) VALUES($1,$2,1,1,true,0)`, f.brand, game.ID); err != nil {
				return err
			}
		}
		period, err = s.CreatePeriod(ctx, tx, a, game.ID, input, points.Metadata{})
		if err == nil && len(sources) > 0 {
			_, err = tx.Exec(ctx, `INSERT INTO brand_automation_periods(brand_id,game_id,period_id,close_before_seconds) VALUES($1,$2,$3,0)`, f.brand, game.ID, period.ID)
		}

		return err
	})
	id := ids.New()
	bettingTx(t, f.db, func(tx pgx.Tx) error {
		stake, reserve, target, family := points.Amount(10), points.Amount(10), "", "canada_order"
		odds := map[string]string{"sum_14": "2"}
		var items any = []CanadaItem{{Code: "sum_14", Points: 10, Odds: "2", RiskPoints: 10}}
		if kind == "mark_six" {
			stake, reserve, family = 20, 20, "mark_six_order"
			odds = map[string]string{"number_07": "48.99", "big": "1.98"}
			items = []MarkSixItem{{Code: "number_07", Points: 10, Odds: "48.99"}, {Code: "big", Points: 10, Odds: "1.98"}}
		} else if kind == "canada_niuniu" {
			reserve, target = 200, "system_frozen"
			odds = map[string]string{"aa": "1.95"}
			items = []CanadaItem{{Code: "aa", Points: 10, Odds: "1.95", RiskPoints: 200}}
		} else if kind == "canada_baccarat" {
			odds = map[string]string{"banker": "2"}
			items = []CanadaItem{{Code: "banker", Points: 10, Odds: "2", RiskPoints: 10}}
		}
		book, _ := json.Marshal(odds)
		if kind == "mark_six" {
			if _, err := tx.Exec(ctx, `INSERT INTO platform_mark_six_versions(game_id,version,odds,odds_b,created_by) VALUES($1,1,$2,$2,$3)`, game.ID, book, a.ID); err != nil {
				return err
			}
		} else if _, err := tx.Exec(ctx, `INSERT INTO platform_canada_versions(game_id,version,odds,profile,created_by) VALUES($1,1,$2,$3,$4)`, game.ID, book, kind, a.ID); err != nil {
			return err
		}
		ps := points.Store{DB: f.db}
		wallet, err := ps.LockedSnapshot(ctx, tx, f.brand, f.member)
		if err != nil {
			return err
		}
		allocation, err := wallet.BySource.Allocate(reserve, "available")
		if err != nil {
			return err
		}
		delta, err := points.AllocationDelta(allocation, "available", target)
		if err != nil {
			return err
		}
		operation := "canada-bet:"
		if kind == "mark_six" {
			operation = "mark-six-bet:"
		}
		requestID := ids.New()
		entry, err := ps.Post(ctx, tx, points.Change{BrandID: f.brand, MemberID: f.member, EntryType: "bet", ReferenceType: family, ReferenceID: id, OperationKey: operation + id, Reason: "historical fixture", ActorType: "user", ActorID: f.user.User.ID, RequestID: requestID, Delta: delta, Allocation: allocation})
		if err != nil {
			return err
		}
		snapshot, _ := json.Marshal(map[string]any{"profile": kind, "version": 1, "rule_version": 1, "enabled": true, "odds": odds, "odds_b": odds, "min_points": "1", "max_points": "1000", "max_period_points": "10000", "max_user_period_points": "5000"})
		rawItems, _ := json.Marshal(items)
		rawAllocation, _ := json.Marshal(allocation)
		if kind == "mark_six" {
			attrs, _ := json.Marshal(map[string]any{"waves": attributes.Waves, "zodiac": attributes.Zodiac, "elements": attributes.Elements})
			_, err = tx.Exec(ctx, `INSERT INTO platform_mark_six_orders(id,brand_id,member_id,user_id,game_id,period_id,rule_version,sales_version,items,sales_snapshot,total_points,allocation,debit_entry_id,attribute_id,attribute_snapshot,plate) VALUES($1,$2,$3,$4,$5,$6,1,1,$7,$8,$9,$10,$11,$12,$13,'A')`, id, f.brand, f.member, f.user.User.ID, game.ID, period.ID, rawItems, snapshot, stake, rawAllocation, entry.ID, attributes.ID, attrs)
		} else {
			_, err = tx.Exec(ctx, `INSERT INTO platform_canada_orders(id,brand_id,member_id,user_id,game_id,period_id,rule_version,sales_version,items,sales_snapshot,total_points,allocation,debit_entry_id,profile,reserved_points) VALUES($1,$2,$3,$4,$5,$6,1,1,$7,$8,$9,$10,$11,$12,$13)`, id, f.brand, f.member, f.user.User.ID, game.ID, period.ID, rawItems, snapshot, stake, rawAllocation, entry.ID, kind, reserve)
		}
		if err != nil {
			return err
		}
		action := "canada.place"
		if kind == "mark_six" {
			action = "mark_six.place"
		}
		_, err = audit.Append(ctx, tx, audit.Record{BrandID: f.brand, ActorType: "user", ActorID: f.user.User.ID, Action: action, ResourceType: family, ResourceID: id, Reason: "historical fixture", RequestID: requestID, After: items})
		return err
	})
	return f, s, a, game, period, id
}

func TestHistoricalDedicatedOrdersSettleAfterSalesRetirement(t *testing.T) {
	for _, scenario := range []struct {
		name string
		draw rules.Draw
		want points.Amount
	}{
		{"mark_six", rules.Draw{Regular: []int{1, 2, 3, 4, 5, 6}, Special: []int{7}}, 1470},
		{"mark_six_push", rules.Draw{Regular: []int{1, 2, 3, 4, 5, 6}, Special: []int{49}}, 990},
		{"canada_996", rules.Draw{Digits: []int{5, 4, 5}}, 1010},
		{"canada_baccarat", rules.Draw{Digits: []int{5, 7, 5}}, 1000},
		{"canada_niuniu", rules.Draw{Digits: []int{1, 2, 9}}, 810},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			kind := scenario.name
			if kind == "mark_six_push" {
				kind = "mark_six"
			}
			f, s, actor, game, period, id := historicalPurchase(t, kind)
			ctx := context.Background()
			bettingTx(t, f.db, func(tx pgx.Tx) error {
				_, err := s.SetLifecycle(ctx, tx, actor, game.ID, platformlottery.LifecycleInput{ExpectedVersion: game.Version, Status: "offline"}, points.Metadata{})
				return err
			})
			time.Sleep(time.Until(period.DrawAt.Add(30 * time.Millisecond)))
			bettingTx(t, f.db, func(tx pgx.Tx) error {
				_, err := s.Publish(ctx, tx, actor, period.ID, scenario.draw, "", points.Metadata{})
				return err
			})
			settle := f.service.ProcessCanadaSettlements
			if kind == "mark_six" {
				settle = f.service.ProcessMarkSixSettlements
			}
			var wg sync.WaitGroup
			for range 2 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if _, err := settle(ctx, 10); err != nil {
						t.Error(err)
					}
				}()
			}
			wg.Wait()
			if n, err := settle(ctx, 10); err != nil || n != 0 {
				t.Fatal("repeated settlement", n, err)
			}
			wallet, err := (points.Store{DB: f.db}).Read(ctx, f.brand, f.member)
			if err != nil || wallet.AvailablePoints != scenario.want || wallet.SystemFrozenPoints != 0 {
				t.Fatal("historical funds", wallet, err)
			}
			var consistent bool
			if err = f.db.QueryRow(ctx, `SELECT (point_business_preview($1,$2)->>'consistent')::boolean`, f.brand, wallet.AccountID).Scan(&consistent); err != nil || !consistent {
				t.Fatal("ledger reconciliation", err)
			}
			bettingTx(t, f.db, func(tx pgx.Tx) error {
				_, err := s.SetLifecycle(ctx, tx, actor, game.ID, platformlottery.LifecycleInput{ExpectedVersion: game.Version + 1, Status: "deleted"}, points.Metadata{})
				return err
			})
			if kind == "mark_six" {
				orders, err := f.service.MarkSixOrders(ctx, f.brand, f.member, "", 0)
				if err != nil || len(orders) != 1 || orders[0].ID != id || orders[0].Items[0].Odds != "48.99" {
					t.Fatal("history changed", orders, err)
				}
				foreign, err := f.service.MarkSixOrders(ctx, storeTestOtherBrand, "", game.ID, 0)
				if err != nil || len(foreign) != 0 {
					t.Fatal("history leaked", err)
				}
			} else {
				orders, err := f.service.CanadaOrders(ctx, f.brand, f.member, "", 0)
				if err != nil || len(orders) != 1 || orders[0].ID != id || orders[0].Profile != kind {
					t.Fatal("history changed", orders, err)
				}
				foreign, err := f.service.CanadaOrders(ctx, storeTestOtherBrand, "", game.ID, 0)
				if err != nil || len(foreign) != 0 {
					t.Fatal("history leaked", err)
				}
			}
		})
	}
}

func TestConfiguredCutoverStartsNextPeriodWithoutMixingHistoricalSales(t *testing.T) {
	for _, kind := range []string{"mark_six", "canada_996"} {
		t.Run(kind, func(t *testing.T) {
			f, s, a, game, period, _ := historicalPurchaseWindow(t, kind, 10*time.Second)
			ctx := context.Background()
			draw := rules.Draw{Digits: []int{5, 4, 5}}
			code := "sum_14"
			if kind == "mark_six" {
				draw = rules.Draw{Regular: []int{1, 2, 3, 4, 5, 6}, Special: []int{7}}
				code = "number_07"
			}
			engine, request := templateEngine(t, kind, code, func(c *rules.ConfiguredCatalog) { c.Model = game.Model })
			var version platformlottery.ConfiguredVersion
			bettingTx(t, f.db, func(tx pgx.Tx) error {
				var err error
				version, err = s.SaveConfiguredDraft(ctx, tx, a, game.ID, platformlottery.ConfiguredDraftInput{Snapshot: engine.Snapshot(), Cases: []platformlottery.ConfiguredCase{{Requests: []rules.ConfiguredRequest{request}, Draw: draw, Bet: 10, Prize: 25}}}, points.Metadata{})
				return err
			})
			for _, action := range []string{"validate", "submit", "approve", "publish"} {
				bettingTx(t, f.db, func(tx pgx.Tx) error {
					var err error
					version, err = s.ActConfiguredRule(ctx, tx, a, game.ID, version.ID, platformlottery.ConfiguredAction{Revision: version.Revision, Action: action}, points.Metadata{})
					return err
				})
			}
			b := access.Account{ID: a.ID, Type: access.AccountAdmin, BrandIDs: []string{f.brand}, Roles: []access.Role{{BrandID: f.brand, Permissions: []access.Permission{{Resource: "platform_game", Action: "sell", Scope: access.ScopeBrand}}}}}
			var snapshot rules.ConfiguredSnapshot
			json.Unmarshal(engine.Snapshot(), &snapshot)
			bettingTx(t, f.db, func(tx pgx.Tx) error {
				_, err := s.WriteConfiguredSales(ctx, tx, b, f.brand, game.ID, platformlottery.ConfiguredSales{RuleID: version.ID, Enabled: true, Prices: snapshot.Prices, MinPoints: 1, MaxPoints: 100, MaxPeriodPoints: 10000, MaxUserPeriodPoints: 10000}, points.Metadata{})
				return err
			})
			in := ConfiguredInput{PeriodID: period.ID, RuleID: version.ID, SalesVersion: 1, SnapshotHash: engine.SnapshotHash(), Requests: []rules.ConfiguredRequest{request}}
			before, err := (points.Store{DB: f.db}).Read(ctx, f.brand, f.member)
			if err != nil {
				t.Fatal(err)
			}
			tx, err := f.db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			_, err = f.service.PreviewConfigured(ctx, tx, f.brand, f.user, in, points.Metadata{})
			tx.Rollback(ctx)
			if !errors.Is(err, ErrLegacyPeriod) {
				t.Fatal("mixed-period preview", err)
			}
			c := configuredFixture{f: f, s: s, a: a, b: b, g: game, p: period, v: version, input: in}
			if _, err = configuredPlace(t, c, "cutover-old-period", in); !errors.Is(err, ErrLegacyPeriod) {
				t.Fatal("mixed-period purchase", err)
			}
			after, err := (points.Store{DB: f.db}).Read(ctx, f.brand, f.member)
			if err != nil || before.BySource != after.BySource {
				t.Fatal("cutover changed funds", after, err)
			}
			time.Sleep(time.Until(period.BetEndAt.Add(20 * time.Millisecond)))
			bettingTx(t, f.db, func(tx pgx.Tx) error {
				var err error
				c.p, err = s.CreatePeriod(ctx, tx, a, game.ID, platformlottery.PeriodInput{PeriodNo: "configured-next", BetStartAt: time.Now().Add(-time.Hour), BetEndAt: time.Now().Add(time.Hour), DrawAt: time.Now().Add(time.Hour)}, points.Metadata{})
				return err
			})
			in.PeriodID = c.p.ID
			receipt, err := configuredPlace(t, c, "cutover-next-period", in)
			if err != nil || receipt.Status != 201 {
				t.Fatal("next period sale", receipt, err)
			}
		})
	}
}
