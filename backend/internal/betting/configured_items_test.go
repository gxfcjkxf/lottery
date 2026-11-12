package betting

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

func itemFixture(t *testing.T, combinations bool) configuredFixture {
	t.Helper()
	raw, err := os.ReadFile("../rules/config/examples/lucky_group_preview.json")
	if err != nil {
		t.Fatal(err)
	}
	var snap rules.ConfiguredSnapshot
	if err = json.Unmarshal(raw, &snap); err != nil {
		t.Fatal(err)
	}
	domain := snap.Catalog.Domains["lucky"]
	for _, id := range []string{"other", "third"} {
		option := domain.Options[0]
		option.ID = id
		option.Mapping = &rules.ConfiguredMapping{Kind: "numbers", Numbers: []int{8}}
		domain.Options = append(domain.Options, option)
	}
	snap.Catalog.Domains["lucky"] = domain
	snap.Catalog.Plays[0].Selection.Max = 3
	options := []string{"lucky", "other"}
	if combinations {
		snap.Catalog.Plays[0].Selection.Mode = "combinations"
		snap.Catalog.Plays[0].Selection.Min = 3
		snap.Catalog.Plays[0].Selection.PerLine = 2
		snap.Catalog.Plays[0].Tiers[0].Price = rules.ConfiguredPrice{Kind: "fixed", Key: "lucky_price"}
		options = append(options, "third")
	}
	raw, err = json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	c := configuredFixtureFrom(t, raw, []platformlottery.ConfiguredCase{{Requests: []rules.ConfiguredRequest{{PlayID: "operator_new_play", Options: options, Plate: "A", Points: 1000, Multiplier: 1}}}})
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		var err error
		c.p, err = c.s.CreatePeriod(context.Background(), tx, c.a, c.g.ID, platformlottery.PeriodInput{PeriodNo: "item-batch", BetStartAt: time.Now().Add(-time.Hour), BetEndAt: time.Now().Add(5 * time.Second), DrawAt: time.Now().Add(5 * time.Second)}, points.Metadata{})
		c.input.PeriodID = c.p.ID
		return err
	})
	return c
}

func placeItems(t *testing.T, c configuredFixture, key string) (mutation.Result, error) {
	t.Helper()
	m, err := mutation.New(c.f.db, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(c.input)
	return m.ExecuteChecked(context.Background(), c.f.brand, c.f.member, "configured.place", key, m.Fingerprint(string(raw)), func(ctx context.Context, tx pgx.Tx) error {
		return eligibility(ctx, tx, c.f.brand, c.f.user, false)
	}, func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
		out, err := c.f.service.PlaceConfigured(ctx, tx, c.f.brand, c.f.user, c.input, points.Metadata{RequestID: "item-test"})
		if err != nil {
			return mutation.Result{}, err
		}
		return mutation.OK(201, out), nil
	})
}

func TestConfiguredItemsIndependentFundingSettlementAndReplay(t *testing.T) {
	for _, combinations := range []bool{false, true} {
		name := "singles"
		if combinations {
			name = "combinations"
		}
		t.Run(name, func(t *testing.T) {
			c := itemFixture(t, combinations)
			ctx := context.Background()
			fundBettingWallet(t, c.f, 10000)
			first, err := placeItems(t, c, "batch-items")
			if err != nil {
				t.Fatal(err)
			}
			var receipt ConfiguredOrdersReceipt
			if err = json.Unmarshal(first.Data, &receipt); err != nil {
				t.Fatal(err)
			}
			count, optionCount := 2, 1
			if combinations {
				count, optionCount = 3, 2
			}
			if len(receipt.Items) != count {
				t.Fatalf("orders=%d", len(receipt.Items))
			}
			for _, order := range receipt.Items {
				if len(order.Quote.Lines) != 1 || len(order.Quote.Requests) != 1 || len(order.Quote.Requests[0].Options) != optionCount || order.Quote.BetPoints != 1000 {
					t.Fatalf("not an independent item: %+v", order.Quote)
				}
				foreign, e := c.f.service.ConfiguredOrders(ctx, storeTestOtherBrand, c.f.member, c.g.ID, 0)
				if e != nil || len(foreign) != 0 {
					t.Fatal("brand isolation", e)
				}
			}
			var wg sync.WaitGroup
			for range 4 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					r, e := placeItems(t, c, "batch-items")
					if e != nil || string(r.Data) != string(first.Data) {
						t.Error("batch replay", e)
					}
				}()
			}
			wg.Wait()
			wallet, err := (points.Store{DB: c.f.db}).Read(ctx, c.f.brand, c.f.member)
			if err != nil || wallet.AvailablePoints != 10000-points.Amount(count)*1000 {
				t.Fatal("batch funding", wallet, err)
			}
			var orders, debits int
			if err = c.f.db.QueryRow(ctx, `SELECT count(*),count(DISTINCT debit_entry_id) FROM configured_orders WHERE period_id=$1`, c.p.ID).Scan(&orders, &debits); err != nil || orders != count || debits != count {
				t.Fatal("funding evidence", orders, debits, err)
			}
			// A failed second batch must leave all existing item funding intact.
			c.input.Requests[0].Points = 5000
			if _, err = placeItems(t, c, "insufficient"); !errors.Is(err, points.ErrInsufficient) {
				t.Fatal("batch balance guard", err)
			}

			configuredPublishDraw(t, c, 7)
			n, err := c.f.service.ProcessConfiguredSettlements(ctx, 10)
			if err != nil || n != count {
				t.Fatal("independent settlement", n, err)
			}
			settled, err := c.f.service.ConfiguredOrders(ctx, c.f.brand, c.f.member, c.g.ID, 0)
			var prize points.Amount
			for _, o := range settled {
				if o.Status != "settled" || o.Result == nil || len(o.Result.Lines) != 1 {
					t.Fatal("item result", o)
				}
				prize += o.Result.PrizePoints
			}
			expectedPrize := points.Amount(2500)
			if combinations {
				expectedPrize = 5000
			}
			if err != nil || len(settled) != count || prize != expectedPrize {
				t.Fatal("settled items", prize, err)
			}
			before := walletBySource(t, c.f)
			if n, err = c.f.service.ProcessConfiguredSettlements(ctx, 10); err != nil || n != 0 || before != walletBySource(t, c.f) {
				t.Fatal("settlement replay", n, err)
			}
		})
	}
}

func TestConfiguredItemsRollbackAfterFirstDebit(t *testing.T) {
	c := itemFixture(t, false)
	ctx := context.Background()
	fundBettingWallet(t, c.f, 10000)
	// Reproduce a storage failure after the first item's debit and insert succeed.
	_, err := c.f.db.Exec(ctx, `CREATE FUNCTION reject_second_item() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF EXISTS(SELECT 1 FROM configured_orders WHERE period_id=NEW.period_id) THEN RAISE EXCEPTION 'injected item failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_second_item BEFORE INSERT ON configured_orders FOR EACH ROW EXECUTE FUNCTION reject_second_item()`)
	if err != nil {
		t.Fatal(err)
	}
	before := walletBySource(t, c.f)
	if _, err = placeItems(t, c, "rollback-items"); err == nil {
		t.Fatal("injected failure accepted")
	}
	var orders, debits int
	if err = c.f.db.QueryRow(ctx, `SELECT (SELECT count(*) FROM configured_orders),(SELECT count(*) FROM point_ledger_entries WHERE entry_type='bet')`).Scan(&orders, &debits); err != nil || orders != 0 || debits != 0 || before != walletBySource(t, c.f) {
		t.Fatal("partial batch persisted", orders, debits, err)
	}
	if _, err = c.f.db.Exec(ctx, `DROP TRIGGER reject_second_item ON configured_orders`); err != nil {
		t.Fatal(err)
	}
	if _, err = placeItems(t, c, "rollback-items"); err != nil {
		t.Fatal("retry after rollback", err)
	}
}

func TestConfiguredItemsConcurrentBatchesCannotOverdraw(t *testing.T) {
	c := itemFixture(t, false)
	fundBettingWallet(t, c.f, 10000)
	c.input.Requests[0].Points = 3000
	results := make(chan error, 2)
	for _, key := range []string{"distinct-batch-one", "distinct-batch-two"} {
		go func(key string) {
			r, err := placeItems(t, c, key)
			if err == nil && r.Status != 201 {
				err = ErrState
			}
			results <- err
		}(key)
	}
	success := 0
	for range 2 {
		err := <-results
		if err == nil {
			success++
		} else if !errors.Is(err, points.ErrInsufficient) {
			t.Fatal(err)
		}
	}
	wallet, err := (points.Store{DB: c.f.db}).Read(context.Background(), c.f.brand, c.f.member)
	if err != nil || success != 1 || wallet.AvailablePoints != 4000 {
		t.Fatal("concurrent batch balance", success, wallet, err)
	}
}

func TestConfiguredItemsReserveFundingAndOriginalSourceRelease(t *testing.T) {
	draw := rules.Draw{Digits: []int{1, 2, 9}}
	c := templatePurchase(t, "canada_niuniu", "aa", draw, 0, 1000)
	second := c.input.Requests[0]
	second.Points = 500
	c.input.Requests[0].Points = 500
	second.PlayID = "bb"
	second.Options = []string{"bb"}
	c.input.Requests = append(c.input.Requests, second)
	fundBettingWallet(t, c.f, 10000, 0, 20000)
	ctx := context.Background()
	expected := walletBySource(t, c.f)
	expected[0][0] = 500
	expected[2][0] = 10500
	r, err := placeItems(t, c, "reserve-item-batch")
	if err != nil || r.Status != 201 {
		t.Fatal(r, err)
	}
	var receipt ConfiguredOrdersReceipt
	if err = json.Unmarshal(r.Data, &receipt); err != nil || len(receipt.Items) != 2 {
		t.Fatal(receipt, err)
	}
	wallet, err := (points.Store{DB: c.f.db}).Read(ctx, c.f.brand, c.f.member)
	if err != nil || wallet.SystemFrozenPoints != 20000 || wallet.AvailablePoints != 10000 {
		t.Fatal("item reserves", wallet, err)
	}
	time.Sleep(time.Until(c.p.DrawAt.Add(20 * time.Millisecond)))
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		_, err := c.s.Publish(ctx, tx, c.a, c.p.ID, draw, "", points.Metadata{})
		return err
	})
	if n, err := c.f.service.ProcessConfiguredSettlements(ctx, 10); err != nil || n != 2 {
		t.Fatal("item reserve settlement", n, err)
	}
	if after := walletBySource(t, c.f); after != expected {
		t.Fatal("reserve release failed to restore original sources", expected, after)
	}
	if n, err := c.f.service.ProcessConfiguredSettlements(ctx, 10); err != nil || n != 0 || walletBySource(t, c.f) != expected {
		t.Fatal("reserve settlement replay", n, err)
	}
}

func TestConfiguredItemsBatchLimitRejectsWithoutFunding(t *testing.T) {
	c := itemFixture(t, false)
	ctx := context.Background()
	fundBettingWallet(t, c.f, 10000)
	sale, err := platformlottery.ReadConfiguredSales(ctx, c.f.db, c.f.brand, c.g.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	sale.MaxPoints = 1000
	sale.MaxUserPeriodPoints = 1500
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		var err error
		sale, err = c.s.WriteConfiguredSales(ctx, tx, c.b, c.f.brand, c.g.ID, sale, points.Metadata{})
		return err
	})
	c.input.SalesVersion = sale.Version
	before := walletBySource(t, c.f)
	if _, err = placeItems(t, c, "batch-over-period-limit"); !errors.Is(err, ErrLimit) {
		t.Fatal("batch limit", err)
	}
	var orders int
	if err = c.f.db.QueryRow(ctx, `SELECT count(*) FROM configured_orders WHERE period_id=$1`, c.p.ID).Scan(&orders); err != nil || orders != 0 || before != walletBySource(t, c.f) {
		t.Fatal("limit rejection wrote funding", orders, err)
	}
}
