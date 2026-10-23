package betting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"strings"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rules"
	"github.com/jackc/pgx/v5"
)

func templateEngine(t *testing.T, name, code string, configure ...func(*rules.ConfiguredCatalog)) (*rules.ConfiguredEngine, rules.ConfiguredRequest) {
	t.Helper()
	catalog, err := rules.ConfiguredTemplateCatalog(name)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range configure {
		change(&catalog)
	}
	bindings := rules.ConfiguredBindings{Version: "explicit-test-bindings", Attributes: map[string]map[string][]int{}}
	for _, a := range catalog.Attributes {
		labels := map[string][]int{}
		for _, label := range a.Labels {
			labels[label] = []int{}
		}
		for n := 1; n <= 49; n++ {
			label := a.Labels[(n-1)%len(a.Labels)]
			if fixed, ok := a.Fixed[fmtNumber(n)]; ok {
				label = fixed
			}
			labels[label] = append(labels[label], n)
		}
		bindings.Attributes[a.ID] = labels
	}
	book := map[string]string{}
	raw, _ := json.Marshal(catalog)
	var node any
	json.Unmarshal(raw, &node)
	var collect func(any)
	collect = func(value any) {
		switch v := value.(type) {
		case []any:
			for _, x := range v {
				collect(x)
			}
		case map[string]any:
			for k, x := range v {
				if k == "key" || k == "price_key" {
					if key, ok := x.(string); ok {
						book[key] = "2.5"
					}
				} else {
					collect(x)
				}
			}
		}
	}
	collect(node)
	prices := rules.ConfiguredPrices{Version: "explicit-test-prices", Plates: map[string]map[string]string{}}
	for _, plate := range catalog.Plates {
		prices.Plates[plate] = book
	}
	engine, err := rules.NewConfiguredEngine(catalog, bindings, prices)
	if err != nil {
		t.Fatal(err)
	}
	request := rules.ConfiguredRequest{Plate: catalog.Plates[0], Points: 10, Multiplier: 1}
	for _, p := range catalog.Plays {
		for _, o := range catalog.Domains[p.Selection.Domain].Options {
			if o.SourceCode == code || p.ID == code && o.ID == code {
				request.PlayID = p.ID
				request.Options = []string{o.ID}
			}
		}
	}
	if request.PlayID == "" {
		t.Fatal("template option not found", name, code)
	}
	return engine, request
}
func templatePurchase(t *testing.T, name, code string, draw rules.Draw, prize, refund points.Amount, configure ...func(*rules.ConfiguredCatalog)) configuredFixture {
	t.Helper()
	engine, request := templateEngine(t, name, code, configure...)
	return configuredFixtureFrom(t, engine.Snapshot(), []platformlottery.ConfiguredCase{{Requests: []rules.ConfiguredRequest{request}, Draw: draw, Bet: 10, Prize: prize, Refund: refund}})
}
func fmtNumber(n int) string { return fmt.Sprintf("%02d", n) }

func TestConfiguredTemplatesLiveFundingAndHistoricalQuery(t *testing.T) {
	for _, name := range rules.ConfiguredTemplateNames {
		t.Run(name, func(t *testing.T) {
			draw := rules.Draw{Digits: []int{5, 4, 7}}
			code := "sum_16"
			prize, refund := points.Amount(25), points.Amount(0)
			if name == "mark_six" {
				draw = rules.Draw{Regular: []int{1, 2, 3, 4, 5, 6}, Special: []int{7}}
				code = "number_07"
			}
			if name == "canada_baccarat" {
				draw = rules.Draw{Digits: []int{9, 0, 1}}
				code = "banker"
			}
			if name == "canada_niuniu" {
				draw = rules.Draw{Digits: []int{1, 2, 9}}
				code = "aa"
				prize = 0
				refund = 10
			}
			c := templatePurchase(t, name, code, draw, prize, refund)
			ctx := context.Background()
			fundBettingWallet(t, c.f, 100, 0, 900)
			first, err := configuredPlace(t, c, "template-purchase", c.input)
			if err != nil || first.Status != 201 {
				t.Fatal(first, err)
			}
			again, err := configuredPlace(t, c, "template-purchase", c.input)
			if err != nil || string(first.Data) != string(again.Data) {
				t.Fatal("idempotency", again, err)
			}
			wallet, err := (points.Store{DB: c.f.db}).Read(ctx, c.f.brand, c.f.member)
			if err != nil {
				t.Fatal(err)
			}
			funding := points.Amount(10)
			if name == "canada_niuniu" {
				funding = 200
				if wallet.SystemFrozenPoints != 200 {
					t.Fatal("risk not reserved", wallet)
				}
			}
			if wallet.AvailablePoints != 1000-funding {
				t.Fatal("funding", wallet)
			}
			bettingTx(t, c.f.db, func(tx pgx.Tx) error {
				_, err := c.s.SetLifecycle(ctx, tx, c.a, c.g.ID, platformlottery.LifecycleInput{ExpectedVersion: c.g.Version, Status: "offline"}, points.Metadata{})
				return err
			})
			time.Sleep(time.Until(c.p.DrawAt.Add(20 * time.Millisecond)))
			bettingTx(t, c.f.db, func(tx pgx.Tx) error {
				_, err := c.s.Publish(ctx, tx, c.a, c.p.ID, draw, "", points.Metadata{})
				return err
			})
			if n, err := c.f.service.ProcessConfiguredSettlements(ctx, 10); err != nil || n != 1 {
				t.Fatal("settlement", n, err)
			}
			if n, err := c.f.service.ProcessConfiguredSettlements(ctx, 10); err != nil || n != 0 {
				t.Fatal("repeat settlement", n, err)
			}
			wallet, err = (points.Store{DB: c.f.db}).Read(ctx, c.f.brand, c.f.member)
			if err != nil || wallet.AvailablePoints != 1000-funding+prize+refund || wallet.SystemFrozenPoints != 0 {
				t.Fatal("settled funding", wallet, err)
			}
			var consistent bool
			if err = c.f.db.QueryRow(ctx, `SELECT (point_business_preview($1,$2)->>'consistent')::boolean`, c.f.brand, wallet.AccountID).Scan(&consistent); err != nil || !consistent {
				t.Fatal("reconciliation", err)
			}
			bettingTx(t, c.f.db, func(tx pgx.Tx) error {
				_, err := c.s.SetLifecycle(ctx, tx, c.a, c.g.ID, platformlottery.LifecycleInput{ExpectedVersion: c.g.Version + 1, Status: "deleted"}, points.Metadata{})
				return err
			})
			orders, err := c.f.service.ConfiguredOrders(ctx, c.f.brand, c.f.member, "", 0)
			if err != nil || len(orders) != 1 || orders[0].Status != "settled" || orders[0].GameName != c.g.Name {
				t.Fatal("historical query", orders, err)
			}
			foreign, err := c.f.service.ConfiguredOrders(ctx, storeTestOtherBrand, c.f.member, "", 0)
			if err != nil || len(foreign) != 0 {
				t.Fatal("brand isolation", err)
			}
		})
	}
}

// An independent member/session in the same database exercises actual scope
// predicates rather than mocking settlement aggregates.
func configuredMember(t *testing.T, original configuredFixture, brand string) configuredFixture {
	t.Helper()
	c := original
	ctx := context.Background()
	c.f.brand = brand
	c.f.member = ids.New()
	c.f.user.User.ID = ids.New()
	c.f.user.Member.ID = c.f.member
	c.f.user.Member.BrandID = brand
	c.f.user.ID = ids.New()
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO global_users(id,username,password_hash) VALUES($1,$2,'test-only')`, c.f.user.User.ID, "scope_"+c.f.user.User.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO brand_members(id,brand_id,global_user_id,join_method,privacy_policy_version,service_terms_version) VALUES($1,$2,$3,'domain','test-1','test-1')`, c.f.member, brand, c.f.user.User.ID); err != nil {
			return err
		}
		account := ids.New()
		if _, err := tx.Exec(ctx, `INSERT INTO point_accounts(id,brand_id,brand_member_id) VALUES($1,$2,$3)`, account, brand, c.f.member); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO point_buckets(brand_id,account_id,source,state) SELECT $1,$2,s,t FROM unnest(ARRAY['recharge','winning','gift','commission'])s CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal'])t ON CONFLICT DO NOTHING`, brand, account); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO sessions(id,token_hash,user_id,member_id,brand_id,expires_at) VALUES($1,$2,$3,$4,$5,clock_timestamp()+interval '1 hour')`, c.f.user.ID, strings.ReplaceAll(c.f.user.ID, "-", "")+strings.Repeat("0", 32), c.f.user.User.ID, c.f.member, brand)
		return err
	})
	if brand != original.f.brand {
		b := original.b
		b.BrandIDs = []string{brand}
		b.Roles = append([]access.Role(nil), b.Roles...)
		for i := range b.Roles {
			b.Roles[i].BrandID = brand
		}
		bettingTx(t, c.f.db, func(tx pgx.Tx) error {
			_, err := c.s.Connect(ctx, tx, b, brand, c.g.ID, "", points.Metadata{})
			if err != nil {
				return err
			}
			var snap rules.ConfiguredSnapshot
			json.Unmarshal(c.v.Snapshot, &snap)
			_, err = c.s.WriteConfiguredSales(ctx, tx, b, brand, c.g.ID, platformlottery.ConfiguredSales{RuleID: c.v.ID, Enabled: true, Prices: snap.Prices, MinPoints: 1, MaxPoints: 100, MaxPeriodPoints: 10000, MaxUserPeriodPoints: 10000}, points.Metadata{})
			return err
		})
	}
	return c
}

func TestConfiguredSettlementCapsAndAggregatesAreMemberAndBrandScoped(t *testing.T) {
	for _, mode := range []string{"cap", "aggregate"} {
		t.Run(mode, func(t *testing.T) {
			name, code, draw := "canada_996", "sum_16", rules.Draw{Digits: []int{5, 4, 7}}
			configure := func(c *rules.ConfiguredCatalog) { cap := points.Amount(30); c.PeriodPrizeCap = &cap }
			if mode == "aggregate" {
				name, code, draw = "canada_42", "big", rules.Draw{Digits: []int{5, 4, 5}}
				configure = func(c *rules.ConfiguredCatalog) {
					for i := range c.Plays {
						if alt := c.Plays[i].Decisions.Alternate; alt != nil {
							alt.AtLeast = 20
						}
					}
				}
			}
			c := templatePurchase(t, name, code, draw, 25, 0, configure)
			second := configuredMember(t, c, c.f.brand)
			foreign := configuredMember(t, c, storeTestOtherBrand)
			ctx := context.Background()
			for _, member := range []configuredFixture{c, second, foreign} {
				fundBettingWallet(t, member.f, 1000)
			}
			// Account and brand setup can outlast the fixture's initial betting window.
			// Open the period after setup so this test measures settlement scoping.
			bettingTx(t, c.f.db, func(tx pgx.Tx) error {
				var err error
				now := bettingDatabaseNow(t, c.f.db)
				// Four real configured purchases must fit under race instrumentation.
				end := now.Add(15 * time.Second)
				c.p, err = c.s.CreatePeriod(ctx, tx, c.a, c.g.ID, platformlottery.PeriodInput{PeriodNo: "scoped-settlement", BetStartAt: now.Add(-time.Minute), BetEndAt: end, DrawAt: end}, points.Metadata{})
				return err
			})
			c.input.PeriodID, second.input.PeriodID, foreign.input.PeriodID = c.p.ID, c.p.ID, c.p.ID
			for i, member := range []configuredFixture{c, c, second, foreign} {
				receipt, err := configuredPlace(t, member, fmt.Sprintf("scoped-%d", i), member.input)
				if err != nil || receipt.Status != 201 {
					t.Fatalf("scoped purchase %d: status=%d err=%v", i, receipt.Status, err)
				}
			}
			time.Sleep(time.Until(c.p.DrawAt.Add(20 * time.Millisecond)))
			bettingTx(t, c.f.db, func(tx pgx.Tx) error {
				_, err := c.s.Publish(ctx, tx, c.a, c.p.ID, draw, "", points.Metadata{})
				return err
			})
			if n, err := c.f.service.ProcessConfiguredSettlements(ctx, 10); err != nil || n != 4 {
				t.Fatal(n, err)
			}
			for i, member := range []configuredFixture{c, second, foreign} {
				wallet, err := (points.Store{DB: c.f.db}).Read(ctx, member.f.brand, member.f.member)
				want := points.Amount(1015)
				if i == 0 {
					want = 1010
					if mode == "aggregate" {
						want = 1000
					}
				}
				if err != nil || wallet.AvailablePoints != want {
					t.Fatal(mode, i, wallet, err)
				}
			}
		})
	}
}

func TestConfiguredReserveConcurrentAdmissionsDoNotOverdraw(t *testing.T) {
	c := templatePurchase(t, "canada_niuniu", "aa", rules.Draw{Digits: []int{1, 2, 9}}, 0, 10)
	fundBettingWallet(t, c.f, 300)
	results := make(chan error, 2)
	for i := range 2 {
		go func() {
			r, err := configuredPlace(t, c, fmt.Sprintf("risk-concurrent-%d", i), c.input)
			if err == nil && r.Status != 201 {
				err = fmt.Errorf("unexpected status %d", r.Status)
			}
			results <- err
		}()
	}
	accepted, rejected := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			accepted++
		} else if errors.Is(err, points.ErrInsufficient) {
			rejected++
		} else {
			t.Fatal(err)
		}
	}
	wallet, err := (points.Store{DB: c.f.db}).Read(context.Background(), c.f.brand, c.f.member)
	if err != nil || accepted != 1 || rejected != 1 || wallet.AvailablePoints != 100 || wallet.SystemFrozenPoints != 200 {
		t.Fatal("reserve overdraw", accepted, rejected, wallet, err)
	}
}
