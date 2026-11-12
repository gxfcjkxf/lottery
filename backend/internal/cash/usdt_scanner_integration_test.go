package cash

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Runs unchanged production query, receipt parsing, matching and ledger code.
// Only the external TRON responses are simulated; PostgreSQL is real.
func TestUSDTScannerPrincipalFeeAndReplay(t *testing.T) {
	f := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var database string
	if e := f.db.QueryRow(ctx, `SELECT current_database()`).Scan(&database); e != nil {
		t.Fatal(e)
	}
	if !strings.HasPrefix(database, "lottery_test_") {
		t.Fatal("requires an explicitly named independent test database")
	}
	o := usdtOrder(t, f, "100")
	transfer := transferFor(t, f, o)
	var confirmed atomic.Bool
	var historyCalls atomic.Int64
	barrier := make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/accounts/" + o.Destination.Number + "/transactions/trc20":
			if r.URL.Query().Get("only_confirmed") != "true" || r.URL.Query().Get("only_to") != "true" || r.URL.Query().Get("contract_address") != USDTContract {
				t.Error("missing confirmed USDT filters")
				http.Error(w, "invalid query", 400)
				return
			}
			if confirmed.Load() {
				if historyCalls.Add(1) == 8 {
					once.Do(func() { close(barrier) })
				}
				select {
				case <-barrier:
				case <-ctx.Done():
					http.Error(w, "test barrier timeout", 500)
					return
				}
			}
			// Duplicate provider records must still represent one transfer.
			fmt.Fprintf(w, `{"success":true,"data":[{"transaction_id":"%s"},{"transaction_id":"%s"}]}`, transfer.Transaction, transfer.Transaction)
		case "/walletsolidity/gettransactioninfobyid":
			var body map[string]string
			if e := json.NewDecoder(r.Body).Decode(&body); e != nil || body["value"] != transfer.Transaction {
				t.Error("incorrect receipt query")
				http.Error(w, "invalid receipt", 400)
				return
			}
			if !confirmed.Load() {
				fmt.Fprint(w, `{}`)
				return
			}
			fmt.Fprintf(w, `{"id":"%s","blockTimeStamp":%d,"receipt":{"result":"SUCCESS"},"log":[{"address":"%s","topics":["%s","%s","%s"],"data":"%064x"}]}`, transfer.Transaction, transfer.Time.UnixMilli(), usdtHex, transferTopic, strings.Repeat("0", 24)+usdtHex, strings.Repeat("0", 24)+"74472e7d35395a6b5add427eecb7f4b62ad2b071", transfer.Amount)
		default:
			http.Error(w, "unexpected query", 404)
		}
	}))
	defer server.Close()
	scanner := USDTScanner{DB: f.db, Source: &TronSource{BaseURL: server.URL, Client: server.Client()}, Cash: f.s}
	n, e := scanner.Tick(ctx)
	if e != nil || n != 0 {
		t.Fatal("unconfirmed transfer credited", n, e)
	}
	var before int
	if e = f.db.QueryRow(ctx, `SELECT count(*) FROM cash_entries WHERE order_id=$1`, o.ID).Scan(&before); e != nil || before != 0 {
		t.Fatal(before, e)
	}
	confirmed.Store(true)
	type outcome struct {
		n int
		e error
	}
	results := make(chan outcome, 8)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); n, e := scanner.Tick(ctx); results <- outcome{n, e} }()
	}
	wg.Wait()
	close(results)
	credited := 0
	for r := range results {
		if r.e != nil {
			t.Fatal(r.e)
		}
		credited += r.n
	}
	if credited != 1 || historyCalls.Load() != 8 {
		t.Fatal("concurrent scanner credit count", credited, historyCalls.Load())
	}
	for range 3 {
		n, e = scanner.Tick(ctx)
		if e != nil || n != 0 {
			t.Fatal("repeat scan credited", n, e)
		}
	}
	transfers, e := scanner.Source.Transfers(ctx, o.Destination.Number, transfer.Time.Add(-time.Minute))
	if e != nil || len(transfers) != 1 {
		t.Fatal("receipt reread", transfers, e)
	}
	for range 3 {
		ok, e := transact(t, f, func(tx pgx.Tx) (bool, error) { return f.s.CreditUSDT(ctx, tx, transfers[0]) })
		if e != nil || ok {
			t.Fatal("replayed receipt credited", ok, e)
		}
	}
	var principal, requested, fee, balance, withdrawable, reserved, version, received, delta int64
	var state, reference, sender string
	var automatic bool
	var entries, receipts int
	if e = f.db.QueryRow(ctx, `SELECT amount,requested_amount,fee,state,reference,sender,automatic FROM cash_orders WHERE id=$1`, o.ID).Scan(&principal, &requested, &fee, &state, &reference, &sender, &automatic); e != nil {
		t.Fatal(e)
	}
	if e = f.db.QueryRow(ctx, `SELECT deposited,withdrawable,reserved,version FROM cash_balances WHERE brand_id=$1 AND member_id=$2 AND currency='USDT'`, f.brand, f.member).Scan(&balance, &withdrawable, &reserved, &version); e != nil {
		t.Fatal(e)
	}
	if e = f.db.QueryRow(ctx, `SELECT count(*),coalesce(sum((delta_snapshot->>0)::bigint),0) FROM cash_entries WHERE order_id=$1 AND operation='confirm'`, o.ID).Scan(&entries, &delta); e != nil {
		t.Fatal(e)
	}
	if e = f.db.QueryRow(ctx, `SELECT count(*),coalesce(max(received),0) FROM cash_usdt_receipts WHERE order_id=$1`, o.ID).Scan(&receipts, &received); e != nil {
		t.Fatal(e)
	}
	if principal != 100000000 || requested != principal || fee != 1 || balance != principal || delta != principal || received != principal+fee || entries != 1 || receipts != 1 || version != 1 || withdrawable != 0 || reserved != 0 || state != "confirmed" || reference != transfer.Transaction || sender != USDTContract || !automatic {
		t.Fatalf("incorrect ledger: principal=%d fee=%d balance=%d delta=%d received=%d entries=%d receipts=%d version=%d state=%s", principal, fee, balance, delta, received, entries, receipts, version, state)
	}
	report, _ := json.Marshal(map[string]any{"database": database, "brand_id": f.brand, "order_id": o.ID, "chain_source": "simulated confirmed TRON receipt", "principal": Format(principal), "fee": Format(fee), "received": Format(received), "balance": Format(balance), "ledger_delta": Format(delta), "credit_entries": entries, "chain_receipts": receipts, "balance_version": version, "concurrent_scanners": 8, "new_credits": credited, "repeat_scans": 3, "receipt_replays": 3, "state": state})
	t.Log("USDT_VERIFICATION " + string(report))
}
