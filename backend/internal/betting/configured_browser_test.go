//go:build drawbrowser

package betting

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/platformlottery"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/jackc/pgx/v5"
)

// Opt-in runtime suite: scripts/test-draw-browser.mjs requires a real browser
// and database, instead of silently skipping browser execution in other suites.
func TestConfiguredBrowserDOMAutomaticDrawAndSettlement(t *testing.T) {
	c := newConfiguredFixture(t)
	ctx := context.Background()
	drawAt := time.Now().Add(10 * time.Second).UTC().Truncate(time.Second)
	var mu sync.Mutex
	payload := map[string]any{"issue": "previous", "time": time.Now().Add(-time.Minute).UTC().Format(time.RFC3339), "next": "browser-next", "next_time": drawAt.Format(time.RFC3339), "numbers": []int{1, 3, 7, 4, 5, 6, 2}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/data" {
			mu.Lock()
			defer mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(payload)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<main id="app"></main><script>fetch('/data').then(r=>r.json()).then(d=>{document.querySelector('#app').innerHTML='<b id="issue">'+d.issue+'</b><time id="time" datetime="'+d.time+'"></time><b id="next">'+d.next+'</b><time id="next-time" datetime="'+d.next_time+'"></time>'+d.numbers.map(n=>'<i class="ball">'+n+'</i>').join('');document.querySelector('#app').dataset.ready='yes'})</script>`))
	}))
	defer server.Close()
	mapping := &platformlottery.FeedMapping{Format: "browser", Browser: &platformlottery.FeedBrowser{WaitSelector: "[data-ready=yes]", TimeoutMS: 5000}, Schedule: "source", TimeFormat: "rfc3339", Period: platformlottery.FeedField{Path: "#issue"}, DrawTime: platformlottery.FeedField{Path: "#time", Attribute: "datetime"}, Regular: &platformlottery.FeedNumbers{FeedField: platformlottery.FeedField{Path: ".ball"}, Count: 6}, Special: &platformlottery.FeedNumbers{FeedField: platformlottery.FeedField{Path: ".ball"}, Offset: 6, Count: 1}, NextPeriod: &platformlottery.FeedField{Path: "#next"}, NextDrawTime: &platformlottery.FeedField{Path: "#next-time", Attribute: "datetime"}}
	var source platformlottery.DrawSource
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		var e error
		source, e = c.s.SaveDrawSource(ctx, tx, c.a, c.g.ID, platformlottery.DrawSourceInput{DrawSource: platformlottery.DrawSource{Name: "Dynamic DOM", Plan: platformlottery.AutomationPlan{SourceType: "mapped", Mapping: mapping, Enabled: true, SourceURL: server.URL}}}, points.Metadata{})
		return e
	})
	var active platformlottery.AutomationPlan
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		var e error
		active, e = c.s.ActivateDrawSource(ctx, tx, c.a, c.g.ID, source.ID, platformlottery.DrawSourceActivation{Revision: 1}, points.Metadata{})
		return e
	})
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		_, e := c.s.WriteBrandAutomation(ctx, tx, c.b, c.f.brand, c.g.ID, platformlottery.BrandAutomationInput{BrandAutomation: platformlottery.BrandAutomation{RuleVersion: active.Version, Enabled: true}}, points.Metadata{})
		return e
	})
	sources := platformlottery.AutomationSources{Origins: []string{server.URL}, Client: server.Client()}
	if n, e := c.s.CollectAutomaticDraws(ctx, sources); e != nil || n != 0 {
		t.Fatal("initial dynamic schedule", n, e)
	}
	var periodID string
	if e := c.f.db.QueryRow(ctx, `SELECT id::text FROM platform_game_periods WHERE game_id=$1 AND period_no='browser-next'`, c.g.ID).Scan(&periodID); e != nil {
		t.Fatal(e)
	}
	fundBettingWallet(t, c.f, 100)
	c.input.PeriodID = periodID
	first, e := configuredPlace(t, c, "dynamic-dom-order", c.input)
	if e != nil {
		t.Fatal(e)
	}
	repeat, e := configuredPlace(t, c, "dynamic-dom-order", c.input)
	if e != nil || string(first.Data) != string(repeat.Data) {
		t.Fatal("idempotent debit", e)
	}
	wallet, e := (points.Store{DB: c.f.db}).Read(ctx, c.f.brand, c.f.member)
	if e != nil || wallet.AvailablePoints != 90 {
		t.Fatal("one debit after replay", wallet, e)
	}
	// A saved but unselected browser change cannot alter this frozen reader.
	bettingTx(t, c.f.db, func(tx pgx.Tx) error {
		source.Plan.Mapping.Browser.WaitSelector = "#not-selected"
		_, e := c.s.SaveDrawSource(ctx, tx, c.a, c.g.ID, platformlottery.DrawSourceInput{DrawSource: source}, points.Metadata{})
		return e
	})
	time.Sleep(time.Until(drawAt.Add(30 * time.Millisecond)))
	mu.Lock()
	payload["issue"] = "browser-next"
	payload["time"] = drawAt.Format(time.RFC3339)
	payload["next"] = "browser-later"
	payload["next_time"] = drawAt.Add(time.Hour).Format(time.RFC3339)
	mu.Unlock()
	if n, e := c.s.CollectAutomaticDraws(ctx, sources); e != nil || n != 1 {
		t.Fatal("dynamic automatic result", n, e)
	}
	// Competing workers must settle the order exactly once.
	type settled struct {
		count int
		err   error
	}
	workers := make(chan settled, 2)
	for i := 0; i < 2; i++ {
		go func() { n, err := c.f.service.ProcessConfiguredSettlements(ctx, 10); workers <- settled{n, err} }()
	}
	total := 0
	for i := 0; i < 2; i++ {
		r := <-workers
		if r.err != nil {
			t.Fatal("concurrent settlement", r.err)
		}
		total += r.count
	}
	if total != 1 {
		t.Fatal("concurrent settlement count", total)
	}
	if n, e := c.s.CollectAutomaticDraws(ctx, sources); e != nil || n != 0 {
		t.Fatal("repeat collection", n, e)
	}
	if n, e := c.f.service.ProcessConfiguredSettlements(ctx, 10); e != nil || n != 0 {
		t.Fatal("repeat payout", n, e)
	}
	wallet, e = (points.Store{DB: c.f.db}).Read(ctx, c.f.brand, c.f.member)
	if e != nil || wallet.AvailablePoints != 115 {
		t.Fatal("wallet", wallet, e)
	}
	if replay, err := configuredPlace(t, c, "dynamic-dom-order", c.input); err != nil || string(replay.Data) != string(first.Data) {
		t.Fatal("purchase replay after settlement", err)
	}
	var debits, prizes, refunds, publications, settlements int
	if e = c.f.db.QueryRow(ctx, `SELECT count(*) FILTER(WHERE l.entry_type='bet'),count(*) FILTER(WHERE l.entry_type='prize'),count(*) FILTER(WHERE l.entry_type='refund') FROM point_ledger_entries l JOIN configured_orders o ON l.reference_type='configured_order' AND l.reference_id=o.id WHERE o.period_id=$1`, periodID).Scan(&debits, &prizes, &refunds); e != nil || debits != 1 || prizes != 1 || refunds != 0 {
		t.Fatal("exact ledger counts", debits, prizes, refunds, e)
	}
	if e = c.f.db.QueryRow(ctx, `SELECT count(*) FILTER(WHERE action='platform_game.automation.result'),count(*) FILTER(WHERE action='configured.settle') FROM audit_logs WHERE resource_id=$1 OR resource_id IN(SELECT id FROM configured_orders WHERE period_id=$1)`, periodID).Scan(&publications, &settlements); e != nil || publications != 1 || settlements != 1 {
		t.Fatal("exact publication and settlement counts", publications, settlements, e)
	}
	var invalid int
	if e = c.f.db.QueryRow(ctx, `SELECT count(*) FROM point_configured_business_witnesses($1,$2) WHERE NOT source_valid`, c.f.brand, wallet.AccountID).Scan(&invalid); e != nil || invalid != 0 {
		t.Fatal("financial witnesses", invalid, e)
	}
	orders, e := c.f.service.ConfiguredOrders(ctx, storeTestOtherBrand, c.f.member, c.g.ID, 0)
	if e != nil || len(orders) != 0 {
		t.Fatal("foreign orders", e)
	}
	t.Logf("available balance 100 -> 90 -> 115; debit=%d prize=%d refund=%d automatic publication=%d settlement=%d", debits, prizes, refunds, publications, settlements)
}
