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
	request := rules.ConfiguredRequest{Plate: catalog.Plates[0], Points: 1000, Multiplier: 1}
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
	return configuredFixtureFrom(t, engine.Snapshot(), []platformlottery.ConfiguredCase{{Requests: []rules.ConfiguredRequest{request}, Draw: draw, Bet: 1000, Prize: prize, Refund: refund}})
}
func fmtNumber(n int) string { return fmt.Sprintf("%02d", n) }

func TestConfiguredTemplatesLiveFundingAndHistoricalQuery(t *testing.T) {
	for _, name := range rules.ConfiguredTemplateNames {
		t.Run(name, func(t *testing.T) {
			draw := rules.Draw{Digits: []int{5, 4, 7}}
			code := "sum_16"
			prize, refund := points.Amount(2500), points.Amount(0)
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
				refund = 1000
			}
			c := templatePurchase(t, name, code, draw, prize, refund)
			ctx := context.Background()
			fundBettingWallet(t, c.f, 10000, 0, 90000)
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
			funding := points.Amount(1000)
			if name == "canada_niuniu" {
				funding = 20000
				if wallet.SystemFrozenPoints != 20000 {
					t.Fatal("risk not reserved", wallet)
				}
			}
			if wallet.AvailablePoints != 100000-funding {
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
			if err != nil || wallet.AvailablePoints != 100000-funding+prize+refund || wallet.SystemFrozenPoints != 0 {
				t.Fatal("settled funding", wallet, err)
			}
			var consistent bool
			if err = c.f.db.QueryRow(ctx, `SELECT coalesce(bool_and(source_valid),false) FROM point_configured_business_witnesses($1,$2)`, c.f.brand, wallet.AccountID).Scan(&consistent); err != nil || !consistent {
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
			summaries, err := c.f.service.ConfiguredOrderSummaries(ctx, c.f.brand, c.f.member, "", 0)
			if err != nil || len(summaries) != len(orders) {
				t.Fatal("historical summaries", err)
			}
			for i := range orders {
				assertConfiguredSummary(t, orders[i], summaries[i])
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
		if _, err := tx.Exec(ctx, `INSERT INTO brand_members(id,brand_id,global_user_id,join_method) VALUES($1,$2,$3,'domain')`, c.f.member, brand, c.f.user.User.ID); err != nil {
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
			_, err = c.s.WriteConfiguredSales(ctx, tx, b, brand, c.g.ID, platformlottery.ConfiguredSales{RuleID: c.v.ID, Enabled: true, Prices: snap.Prices, MinPoints: 100, MaxPoints: 10000, MaxPeriodPoints: 1000000, MaxUserPeriodPoints: 1000000}, points.Metadata{})
			return err
		})
	}
	return c
}

func TestConfiguredSettlementCapsAndAggregatesAreMemberAndBrandScoped(t *testing.T) {
	for _, mode := range []string{"cap", "aggregate"} {
		t.Run(mode, func(t *testing.T) {
			name, code, draw := "canada_996", "sum_16", rules.Draw{Digits: []int{5, 4, 7}}
			configure := func(c *rules.ConfiguredCatalog) { cap := points.Amount(3000); c.PeriodPrizeCap = &cap }
			if mode == "aggregate" {
				name, code, draw = "canada_42", "big", rules.Draw{Digits: []int{5, 4, 5}}
				configure = func(c *rules.ConfiguredCatalog) {
					for i := range c.Plays {
						if alt := c.Plays[i].Decisions.Alternate; alt != nil {
							alt.AtLeast = 2000
						}
					}
				}
			}
			c := templatePurchase(t, name, code, draw, 2500, 0, configure)
			second := configuredMember(t, c, c.f.brand)
			foreign := configuredMember(t, c, storeTestOtherBrand)
			ctx := context.Background()
			for _, member := range []configuredFixture{c, second, foreign} {
				fundBettingWallet(t, member.f, 100000)
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
				want := points.Amount(101500)
				if i == 0 {
					want = 101000
					if mode == "aggregate" {
						want = 100000
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
	c := templatePurchase(t, "canada_niuniu", "aa", rules.Draw{Digits: []int{1, 2, 9}}, 0, 1000)
	fundBettingWallet(t, c.f, 30000)
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
	if err != nil || accepted != 1 || rejected != 1 || wallet.AvailablePoints != 10000 || wallet.SystemFrozenPoints != 20000 {
		t.Fatal("reserve overdraw", accepted, rejected, wallet, err)
	}
}

func TestConfiguredLatestImportedDrawUsesSavedRuleAttributes(t *testing.T) {
	engine, request := templateEngine(t, "mark_six", "number_01")
	draw := rules.Draw{Regular: []int{1, 2, 3, 4, 5, 6}, Special: []int{7}}
	c := configuredFixtureFrom(t, engine.Snapshot(), []platformlottery.ConfiguredCase{{Requests: []rules.ConfiguredRequest{request}, Draw: draw, Bet: 1000}})
	ctx := context.Background()
	drawAt := time.Now().Add(-24 * time.Hour).Truncate(time.Minute).Add(37 * time.Second)
	// The draw predates setup; the actual import/publication happens after saving
	// rules. No removed rule-approval audit event is created.
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		p, err := c.s.CreatePeriod(ctx, tx, c.a, c.g.ID, platformlottery.PeriodInput{PeriodNo: "2026300", BetStartAt: time.Now().Add(-48 * time.Hour), BetEndAt: time.Now().Add(-25 * time.Hour), DrawAt: drawAt}, points.Metadata{})
		if err != nil {
			return err
		}
		_, err = c.s.Publish(ctx, tx, c.a, p.ID, draw, "imported result", points.Metadata{})
		return err
	})
	latest, err := c.f.service.LatestConfiguredDraw(ctx, c.f.brand, c.g.ID)
	if err != nil || latest == nil {
		t.Fatal(latest, err)
	}
	if latest.DrawAt == nil || !latest.DrawAt.Equal(drawAt) || latest.DrawnAt == nil || latest.DrawAt.Equal(*latest.DrawnAt) {
		t.Fatalf("draw time must retain seconds and differ from publication: %#v", latest)
	}
	for _, number := range []string{"01", "07"} {
		a := latest.Attributes[number]
		if a.Wave == "" || a.Zodiac == "" || a.Element == "" {
			t.Fatal("saved rule attributes missing", number, a)
		}
	}
	// A later save must not relabel an already published result.
	var next rules.ConfiguredSnapshot
	if err := json.Unmarshal(engine.Snapshot(), &next); err != nil {
		t.Fatal(err)
	}
	for label, numbers := range next.Bindings.Attributes["zodiac"] {
		filtered := []int{}
		for _, n := range numbers {
			if n != 1 {
				filtered = append(filtered, n)
			}
		}
		next.Bindings.Attributes["zodiac"][label] = filtered
	}
	for label, numbers := range next.Bindings.Attributes["zodiac"] {
		if label != latest.Attributes["01"].Zodiac {
			next.Bindings.Attributes["zodiac"][label] = append(numbers, 1)
			break
		}
	}
	nextRaw, err := json.Marshal(next)
	if err != nil {
		t.Fatal(err)
	}
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		_, err := c.s.SaveConfiguredRules(ctx, tx, c.a, c.g.ID, platformlottery.ConfiguredRulesInput{ID: c.v.ID, Revision: c.v.Revision, Snapshot: nextRaw}, points.Metadata{})
		return err
	})
	archive, err := c.f.service.DrawArchive(ctx, c.f.brand, c.g.ID, "configured", time.Now().Format("2006"), 50, 0, 0)
	if err != nil || len(archive.Items) != 1 || archive.Items[0].Attributes["01"] != latest.Attributes["01"] {
		t.Fatal("latest and archive disagree", archive, err)
	}
}

func TestCanada28DailyDrawHistoryAcrossSurfaces(t *testing.T) {
	engine, request := templateEngine(t, "canada_28", "sum_14")
	c := configuredFixtureFrom(t, engine.Snapshot(), []platformlottery.ConfiguredCase{{Requests: []rules.ConfiguredRequest{request}, Draw: rules.Draw{Digits: []int{5, 4, 5}}, Bet: 1000, Prize: 2500}})
	ctx := context.Background()
	if _, err := c.f.db.Exec(ctx, `UPDATE platform_settings SET timezone='Asia/Singapore' WHERE id=true`); err != nil {
		t.Fatal(err)
	}
	zone, _ := time.LoadLocation("Asia/Singapore")
	now := time.Now().In(zone)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, zone)
	for i, at := range []time.Time{today.Add(-23 * time.Second), today, today.Add(time.Second)} {
		if _, err := c.f.db.Exec(ctx, `INSERT INTO platform_game_periods(id,game_id,period_no,bet_start_at,bet_end_at,draw_at,result,published_at,published_by) VALUES($1,$2,$3,$4,$5,$6,'{"regular":[],"special":[],"digits":[5,4,5]}',clock_timestamp(),$7)`, ids.New(), c.g.ID, fmt.Sprint(800000+i), at.Add(-time.Hour), at.Add(-time.Minute), at, c.a.ID); err != nil {
			t.Fatal(err)
		}
	}
	for _, choice := range []struct {
		date  string
		count string
	}{{today.Format("2006-01-02"), "2"}, {today.AddDate(0, 0, -1).Format("2006-01-02"), "1"}, {today.AddDate(0, 0, -2).Format("2006-01-02"), "0"}} {
		user, err := c.f.service.DrawArchive(ctx, c.f.brand, c.g.ID, "configured", choice.date, 1, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		management, err := c.s.DrawOverview(ctx, c.g.ID, "", choice.date, 1, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !user.Daily || !management.Daily || user.Selected != management.Date || user.Today != management.Today || user.Yesterday != management.Yesterday || user.TotalCount != choice.count || management.TotalCount != choice.count || user.HasMore != management.HasMore {
			t.Fatalf("date selection differs: user=%+v management=%+v", user, management)
		}
		if len(user.Items) > 0 && (user.Items[0].PeriodNo != management.Items[0].PeriodNo || !user.Items[0].DrawAt.Equal(*management.Items[0].DrawAt)) {
			t.Fatal("results differ")
		}
	}
	for _, date := range []string{"2026", "2026-02-30", today.AddDate(0, 0, 1).Format("2006-01-02")} {
		if _, err := c.f.service.DrawArchive(ctx, c.f.brand, c.g.ID, "configured", date, 50, 0, 0); err != ErrInvalid {
			t.Fatalf("user invalid date %s: %v", date, err)
		}
		if _, err := c.s.DrawOverview(ctx, c.g.ID, "", date, 50, 0, 0); err != platformlottery.ErrInvalid {
			t.Fatalf("management invalid date %s: %v", date, err)
		}
	}
	if _, err := c.s.DrawOverview(ctx, c.g.ID, "2026", "", 50, 0, 0); err != platformlottery.ErrInvalid {
		t.Fatal("Canada28 must reject annual selection", err)
	}
	page, err := c.f.service.DrawArchive(ctx, c.f.brand, c.g.ID, "configured", today.Format("2006-01-02"), 1, 1, 0)
	if err != nil || len(page.Items) != 1 || page.Items[0].PeriodNo != "800001" || page.HasMore {
		t.Fatalf("daily pagination %+v %v", page, err)
	}
}

func TestCanada28RecentDrawHistoryAcrossDays(t *testing.T) {
	engine, request := templateEngine(t, "canada_28", "sum_14")
	c := configuredFixtureFrom(t, engine.Snapshot(), []platformlottery.ConfiguredCase{{Requests: []rules.ConfiguredRequest{request}, Draw: rules.Draw{Digits: []int{5, 4, 5}}, Bet: 1000, Prize: 2500}})
	ctx := context.Background()
	zone, _ := time.LoadLocation("Asia/Singapore")
	now := time.Now().In(zone)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, zone)
	if _, err := c.f.db.Exec(ctx, `UPDATE platform_settings SET timezone='Asia/Singapore' WHERE id=true`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 105; i++ {
		at := today.Add(time.Duration(i-104) * 210 * time.Second)
		if _, err := c.f.db.Exec(ctx, `INSERT INTO platform_game_periods(id,game_id,period_no,bet_start_at,bet_end_at,draw_at,result,published_at,published_by) VALUES($1,$2,$3,$4,$5,$6,'{"regular":[],"special":[],"digits":[5,4,5]}',clock_timestamp(),$7)`, ids.New(), c.g.ID, fmt.Sprint(900000+i), at.Add(-time.Hour), at.Add(-time.Minute), at, c.a.ID); err != nil {
			t.Fatal(err)
		}
	}
	at := today.Add(-210 * time.Second)
	if _, err := c.f.db.Exec(ctx, `INSERT INTO platform_draw_history(game_id,period_no,draw_year,draw_date,result,source) VALUES($1,'900103',$2,$3,'{"regular":[],"special":[],"digits":[4,4,4]}','hkjc')`, c.g.ID, at.Year(), at.Format("2006-01-02")); err != nil {
		t.Fatal(err)
	}
	defaultUser, err := c.f.service.DrawArchive(ctx, c.f.brand, c.g.ID, "configured", "", 50, 0, 0)
	if err != nil || defaultUser.Recent != 50 || len(defaultUser.Items) != 50 || defaultUser.Selected != "" {
		t.Fatalf("user default must be latest 50: %+v %v", defaultUser, err)
	}
	defaultManagement, err := c.s.DrawOverview(ctx, c.g.ID, "", "", 50, 0, 0)
	if err != nil || defaultManagement.Recent != 50 || len(defaultManagement.Items) != 50 || defaultManagement.Date != "" {
		t.Fatalf("management default must be latest 50: %+v %v", defaultManagement, err)
	}

	for _, recent := range []int{50, 100} {
		for _, offset := range []int{0, 50, 100} {
			user, err := c.f.service.DrawArchive(ctx, c.f.brand, c.g.ID, "configured", "", 50, offset, recent)
			if err != nil {
				t.Fatal(err)
			}
			management, err := c.s.DrawOverview(ctx, c.g.ID, "", "", 50, offset, recent)
			if err != nil {
				t.Fatal(err)
			}
			expected := recent - offset
			if expected > 50 {
				expected = 50
			}
			if expected < 0 {
				expected = 0
			}
			if user.TotalCount != fmt.Sprint(recent) || management.TotalCount != user.TotalCount || user.Recent != recent || management.Recent != recent || len(user.Items) != expected || len(management.Items) != expected || user.HasMore != (offset+50 < recent) || management.HasMore != user.HasMore {
				t.Fatalf("recent=%d offset=%d user=%+v management=%+v", recent, offset, user, management)
			}
			for i, item := range user.Items {
				if item.PeriodNo != fmt.Sprint(900104-offset-i) || item.PeriodNo != management.Items[i].PeriodNo {
					t.Fatal("recent ordering or range", item, management.Items[i])
				}
			}
			if offset == 0 && user.Items[1].Result.Digits[0] != 4 {
				t.Fatal("duplicate imported evidence was not preferred")
			}
		}
	}
	if _, err := c.f.service.DrawArchive(ctx, c.f.brand, c.g.ID, "configured", today.Format("2006-01-02"), 50, 0, 50); err != ErrInvalid {
		t.Fatal("mixed date/recent accepted", err)
	}
	if _, err := c.s.DrawOverview(ctx, c.g.ID, "", today.Format("2006-01-02"), 50, 0, 50); err != platformlottery.ErrInvalid {
		t.Fatal("mixed management date/recent accepted", err)
	}
	for _, recent := range []int{-1, 25, 200} {
		if _, err := c.f.service.DrawArchive(ctx, c.f.brand, c.g.ID, "configured", "", 50, 0, recent); err != ErrInvalid {
			t.Fatal("invalid recent accepted", recent, err)
		}
		if _, err := c.s.DrawOverview(ctx, c.g.ID, "", "", 50, 0, recent); err != platformlottery.ErrInvalid {
			t.Fatal("invalid management recent accepted", recent, err)
		}
	}
	if _, err := c.f.db.Exec(ctx, `INSERT INTO platform_game_periods(id,game_id,period_no,bet_start_at,bet_end_at,draw_at,result,published_at,published_by) VALUES($1,$2,'900105',$3,$4,$5,'{"regular":[],"special":[],"digits":[1,2,3]}',clock_timestamp(),$6)`, ids.New(), c.g.ID, now.Add(-time.Hour), now.Add(-time.Minute), now.Add(-time.Second), c.a.ID); err != nil {
		t.Fatal(err)
	}
	updated, err := c.f.service.DrawArchive(ctx, c.f.brand, c.g.ID, "configured", "", 100, 0, 50)
	if err != nil || len(updated.Items) != 50 || updated.Items[0].PeriodNo != "900105" || updated.Items[49].PeriodNo != "900056" {
		t.Fatalf("recent window did not advance %+v %v", updated, err)
	}
}
