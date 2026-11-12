package cash

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTronSourceConfirmedPaginationAndReceipt(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("TRON-PRO-API-KEY") != "test-key" {
			t.Error("missing configured key")
		}
		if strings.HasPrefix(r.URL.Path, "/v1/accounts/") {
			if r.URL.Query().Get("only_confirmed") != "true" || r.URL.Query().Get("only_to") != "true" || r.URL.Query().Get("contract_address") != USDTContract {
				t.Error("missing transfer filters")
			}
			if r.URL.Query().Get("fingerprint") == "" {
				fmt.Fprintf(w, `{"success":true,"data":[{"transaction_id":"%s"}],"meta":{"fingerprint":"page2","links":{"next":"untrusted-link"}}}`, strings.Repeat("a", 64))
			} else {
				fmt.Fprint(w, `{"success":true,"data":[],"meta":{}}`)
			}
			return
		}
		if r.URL.Path != "/walletsolidity/gettransactioninfobyid" {
			t.Error("unconfirmed endpoint", r.URL.Path)
		}
		var in map[string]string
		if e := json.NewDecoder(r.Body).Decode(&in); e != nil {
			t.Error(e)
		}
		calls++
		fmt.Fprintf(w, `{"id":"%s","blockTimeStamp":1794355200000,"receipt":{"result":"SUCCESS"},"log":[{"address":"%s","topics":["%s","%s","%s"],"data":"%064x"}]}`, in["value"], usdtHex, transferTopic, strings.Repeat("0", 24)+usdtHex, strings.Repeat("0", 24)+usdtHex, 100000001)
	}))
	defer server.Close()
	source := TronSource{BaseURL: server.URL, Key: "test-key", Client: server.Client()}
	transfers, e := source.Transfers(context.Background(), USDTContract, time.Now().Add(-time.Hour))
	if e != nil || len(transfers) != 1 || calls != 1 || transfers[0].Amount != 100000001 {
		t.Fatal(transfers, calls, e)
	}
}
func TestTronSourceNeverCreditsFailedOrUnconfirmed(t *testing.T) {
	for _, receipt := range []string{`{}`, `{"Error":"quota"}`, fmt.Sprintf(`{"id":"%s","receipt":{"result":"REVERT"}}`, strings.Repeat("a", 64))} {
		t.Run(receipt, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/v1/") {
					fmt.Fprintf(w, `{"success":true,"data":[{"transaction_id":"%s"}]}`, strings.Repeat("a", 64))
					return
				}
				fmt.Fprint(w, receipt)
			}))
			defer server.Close()
			source := TronSource{BaseURL: server.URL, Client: server.Client()}
			transfers, _ := source.Transfers(context.Background(), USDTContract, time.Now())
			if len(transfers) != 0 {
				t.Fatal(transfers)
			}
		})
	}
}
