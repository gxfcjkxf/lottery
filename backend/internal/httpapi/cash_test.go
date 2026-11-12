package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gxfcjkxf/lottery/backend/internal/agency"

	"github.com/gxfcjkxf/lottery/backend/internal/cash"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestCashHTTPScopedIdempotencyAndPermission(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	brand, role := ids.New(), ids.New()
	code := "cash-http-" + brand[:8]
	for _, q := range []struct {
		sql  string
		args []any
	}{{`INSERT INTO brands(id,code,name,status) VALUES($1,$2,'Cash HTTP isolated test','active')`, []any{brand, code}}, {`INSERT INTO brand_domains(id,brand_id,domain) VALUES($1,$2,$3)`, []any{ids.New(), brand, code + ".localhost"}}, {`INSERT INTO admin_brand_scopes(account_id,brand_id) VALUES($1,$2)`, []any{f.root, brand}}, {`INSERT INTO roles(id,brand_id,code,name) VALUES($1,$2,'cash_test','Cash HTTP test')`, []any{role, brand}}, {`INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, []any{f.root, role}}} {
		if _, e := f.pool.Exec(ctx, q.sql, q.args...); e != nil {
			t.Fatal(e)
		}
	}
	for _, permission := range []string{"user.create.brand", "brand.view.brand", "recharge.view.brand", "recharge.confirm.brand", "point_policy.view.brand", "point_policy.write.brand", "agent.write.brand"} {
		if _, e := f.pool.Exec(ctx, `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, permission); e != nil {
			t.Fatal(e)
		}
		if _, e := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2)`, role, permission); e != nil {
			t.Fatal(e)
		}
	}
	created := f.call("POST", "/api/v1/admin/users", "cash-create-member", f.token, brand, map[string]string{"username": "cash_http_member", "password": "cash-http-test-password"})
	mustStatus(t, created, 201)
	var member struct {
		MemberID string `json:"member_id"`
	}
	managedData(t, created, &member)
	cashContext := ""
	userCall := func(method, path, key, token string, body any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		if body == nil {
			raw = nil
		}
		r := httptest.NewRequest(method, "http://"+code+".localhost"+path, bytes.NewReader(raw))
		r.RemoteAddr = "192.0.2.91:12345"
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Cash-Actor-Context", cashContext)
		r.Header.Set("Origin", "http://"+code+".localhost")
		r.Header.Set("Idempotency-Key", key)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		f.http.ServeHTTP(w, r)
		return w
	}
	login := userCall("POST", "/api/v1/auth/login", "cash-login", "", map[string]string{"identifier": "cash_http_member", "password": "cash-http-test-password"})
	mustStatus(t, login, 200)
	var auth identity.Authentication
	managedData(t, login, &auth)
	initialState := userCall("GET", "/api/v1/cash", "", auth.AccessToken, nil)
	mustStatus(t, initialState, 200)
	var initial struct {
		ActorContext string `json:"actor_context"`
	}
	managedData(t, initialState, &initial)
	cashContext = initial.ActorContext
	tx, e := f.pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	cfg := cash.EmptyConfig()
	for i := range cfg.Rules {
		r := &cfg.Rules[i]
		r.Enabled = true
		r.Rate = "7"
		r.DepositMin = "1"
		r.DepositMax = "10000"
		r.WithdrawalMin = "1"
		r.WithdrawalMax = "10000"
	}
	meta := points.Metadata{ActorType: "system", RequestID: ids.New()}
	if _, e = cash.SaveConfig(ctx, tx, "platform", cfg, meta); e != nil {
		t.Fatal(e)
	}
	if _, e = cash.SaveConfig(ctx, tx, brand, cfg, meta); e != nil {
		t.Fatal(e)
	}
	if _, e = cash.SaveDestination(ctx, tx, brand, "", cash.Destination{Currency: "CNY", Channel: "bank", Holder: "Test", Institution: "Bank", Number: "123", Enabled: true}, meta); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	body := map[string]string{"currency": "CNY", "amount": "10"}
	savedContext := cashContext
	cashContext = "changed-account"
	mustStatus(t, userCall("POST", "/api/v1/cash/deposits", "cash-wrong-actor", auth.AccessToken, body), 403)
	cashContext = savedContext

	first := userCall("POST", "/api/v1/cash/deposits", "cash-original-deposit", auth.AccessToken, body)
	mustStatus(t, first, 200)
	var order cash.Order
	managedData(t, first, &order)
	replay := userCall("POST", "/api/v1/cash/deposits", "cash-original-deposit", auth.AccessToken, body)
	mustStatus(t, replay, 200)
	var repeated cash.Order
	managedData(t, replay, &repeated)
	if order.ID != repeated.ID {
		t.Fatal("replay allocated a new order")
	}
	changed := userCall("POST", "/api/v1/cash/deposits", "cash-original-deposit", auth.AccessToken, map[string]string{"currency": "CNY", "amount": "11"})
	mustStatus(t, changed, 409)
	duplicate := userCall("POST", "/api/v1/cash/deposits", "cash-second-deposit", auth.AccessToken, body)
	mustStatus(t, duplicate, 409)
	if !strings.Contains(duplicate.Body.String(), "CASH_DEPOSIT_PENDING") {
		t.Fatal(duplicate.Body.String())
	}
	confirm := map[string]any{"version": 1, "amount": "10", "reference": "cash-http-bank-reference"}
	path := "/api/v1/admin/cash/orders/" + order.ID + "/confirm"
	mustStatus(t, f.call("POST", path, "cash-confirm-original", f.token, brand, confirm), 200)
	mustStatus(t, f.call("POST", path, "cash-confirm-original", f.token, brand, confirm), 200)

	// Independent agents deposit into their own wallet in this owned test brand.
	createAgent := func(name string) agency.Node {
		out := f.call("POST", "/api/v1/admin/agents", "cash-create-"+name, f.token, brand, agency.CreateInput{PolicyVersion: 1, Username: name, Password: "agent123", Config: agency.NodeConfig{Mode: new("loss"), Ratio: "0", Status: "active"}})
		mustStatus(t, out, 201)
		var n agency.Node
		managedData(t, out, &n)
		return n
	}
	own, other := createAgent("cash_agent_own"), createAgent("cash_agent_other")
	loginAgent := func(name string) string {
		out := userCall("POST", "/api/v1/agent/login", "", "", identity.AgentLoginInput{Username: name, Password: "agent123"})
		mustStatus(t, out, 200)
		for _, c := range out.Result().Cookies() {
			if c.Name == agentCookieName(brand) {
				return c.Value
			}
		}
		t.Fatal("agent cookie missing")
		return ""
	}
	agentToken, otherToken := loginAgent("cash_agent_own"), loginAgent("cash_agent_other")
	mustStatus(t, userCall("GET", "/api/v1/agent/cash", "", auth.AccessToken, nil), 401)
	readAgent := userCall("GET", "/api/v1/agent/cash", "", agentToken, nil)
	mustStatus(t, readAgent, 200)
	var agentState struct {
		ActorContext string `json:"actor_context"`
		MemberID     string `json:"member_id"`
	}
	managedData(t, readAgent, &agentState)
	if agentState.MemberID != own.MemberID {
		t.Fatal("agent wallet owner differs")
	}
	cashContext = agentState.ActorContext
	mustStatus(t, userCall("POST", "/api/v1/agent/cash/deposits", "agent-wrong-context", otherToken, body), 403)
	createdAgent := userCall("POST", "/api/v1/agent/cash/deposits", "agent-cash-original", agentToken, body)
	mustStatus(t, createdAgent, 200)
	var agentOrder cash.Order
	managedData(t, createdAgent, &agentOrder)
	if agentOrder.MemberID != own.MemberID || agentOrder.MemberAccount != "cash_agent_own" {
		t.Fatal("agent order ownership", agentOrder)
	}
	replayAgent := userCall("POST", "/api/v1/agent/cash/deposits", "agent-cash-original", agentToken, body)
	mustStatus(t, replayAgent, 200)
	var agentRepeated cash.Order
	managedData(t, replayAgent, &agentRepeated)
	if agentRepeated.ID != agentOrder.ID {
		t.Fatal("agent replay created second order")
	}
	mustStatus(t, userCall("POST", "/api/v1/agent/cash/deposits", "agent-cash-original", agentToken, map[string]string{"currency": "CNY", "amount": "11"}), 409)
	readOther := userCall("GET", "/api/v1/agent/cash", "", otherToken, nil)
	mustStatus(t, readOther, 200)
	managedData(t, readOther, &initial)
	cashContext = initial.ActorContext
	mustStatus(t, userCall("POST", "/api/v1/agent/cash/orders/"+agentOrder.ID+"/cancel", "other-agent-cancel", otherToken, map[string]int{"version": 1}), 409)
	mustStatus(t, userCall("GET", "/api/v1/b/harbor/agent/cash", "", agentToken, nil), 404)
	mustStatus(t, userCall("GET", "/api/v1/agent/cash/orders?member_id="+own.MemberID, "", otherToken, nil), 400)
	cashContext = agentState.ActorContext
	mustStatus(t, userCall("POST", "/api/v1/agent/cash/orders/"+agentOrder.ID+"/confirm", "agent-self-confirm", agentToken, map[string]int{"version": 1}), 400)
	mustStatus(t, userCall("POST", "/api/v1/agent/cash/orders/"+agentOrder.ID+"/submit", "agent-submit", agentToken, map[string]any{"version": 1, "reference": "agent-transfer"}), 200)
	agentConfirm := map[string]any{"version": 2, "amount": "9.5", "reference": "agent-confirmed-reference"}
	agentPath := "/api/v1/admin/cash/orders/" + agentOrder.ID + "/confirm"
	mustStatus(t, f.call("POST", agentPath, "agent-confirm-other-brand", f.token, managedBrand, agentConfirm), 403)
	mustStatus(t, f.call("POST", agentPath, "agent-confirm-once", f.token, brand, agentConfirm), 200)
	mustStatus(t, f.call("POST", agentPath, "agent-confirm-once", f.token, brand, agentConfirm), 200)
	var agentBalance int64
	var agentCredits int
	if e = f.pool.QueryRow(ctx, `SELECT deposited,(SELECT count(*) FROM cash_entries WHERE order_id=$3 AND operation='confirm') FROM cash_balances WHERE brand_id=$1 AND member_id=$2 AND currency='CNY'`, brand, own.MemberID, agentOrder.ID).Scan(&agentBalance, &agentCredits); e != nil || agentBalance != 9500000 || agentCredits != 1 {
		t.Fatal("agent duplicate or incorrect credit", agentBalance, agentCredits, e)
	}
	var otherBalances int
	if e = f.pool.QueryRow(ctx, `SELECT count(*) FROM cash_balances WHERE brand_id=$1 AND member_id=$2`, brand, other.MemberID).Scan(&otherBalances); e != nil || otherBalances != 0 {
		t.Fatal("other agent balance changed", otherBalances, e)
	}
	// Agents exchange independently, withdraw to their own destination, and
	// transfer only to members in their line. Replays never move funds twice.
	if _, e = f.pool.Exec(ctx, `INSERT INTO brand_recharge_quotas(brand_id,available) VALUES($1,10000)`, brand); e != nil {
		t.Fatal(e)
	}
	exchange := map[string]string{"currency": "CNY", "direction": "to_points", "amount": "9", "rate": "7"}
	mustStatus(t, userCall("POST", "/api/v1/agent/cash/exchanges", "agent-exchange", agentToken, exchange), 200)
	mustStatus(t, userCall("POST", "/api/v1/agent/cash/exchanges", "agent-exchange", agentToken, exchange), 200)
	memberCreated := userCall("POST", "/api/v1/agent/members", "transfer-member", agentToken, identity.OperatorInput{Username: "agent_transfer_member", Password: "member123"})
	mustStatus(t, memberCreated, 201)
	var recipient struct {
		MemberID string `json:"member_id"`
	}
	managedData(t, memberCreated, &recipient)
	transfer := map[string]string{"member_id": recipient.MemberID, "amount": "20"}
	mustStatus(t, userCall("POST", "/api/v1/agent/transfers", "transfer-once", agentToken, transfer), 200)
	mustStatus(t, userCall("POST", "/api/v1/agent/transfers", "transfer-once", agentToken, transfer), 200)
	mustStatus(t, userCall("POST", "/api/v1/agent/transfers", "transfer-outside", otherToken, transfer), 400)
	mustStatus(t, userCall("POST", "/api/v1/agent/transfers", "transfer-foreign", agentToken, map[string]string{"member_id": newNormalAgentMemberInBrand(t, f, pointsBrandB, "transfer_foreign"), "amount": "1"}), 400)
	mustStatus(t, userCall("POST", "/api/v1/agent/transfers", "transfer-overspend", agentToken, map[string]string{"member_id": recipient.MemberID, "amount": "44"}), 409)
	mustStatus(t, userCall("POST", "/api/v1/agent/transfers", "transfer-negative", agentToken, map[string]string{"member_id": recipient.MemberID, "amount": "-1"}), 400)

	history := userCall("GET", "/api/v1/agent/transfers?limit=1&offset=0", "", agentToken, nil)
	mustStatus(t, history, 200)
	var records struct {
		Items []struct {
			MemberID  string `json:"member_id"`
			Account   string `json:"member_account"`
			Amount    string `json:"amount"`
			CreatedAt string `json:"created_at"`
		}
		Total string `json:"total_count"`
	}
	managedData(t, history, &records)
	if records.Total != "1" || len(records.Items) != 1 || records.Items[0].MemberID != recipient.MemberID || records.Items[0].Account != "agent_transfer_member" || records.Items[0].Amount != "20" || records.Items[0].CreatedAt == "" {
		t.Fatal("incorrect transfer history", history.Body.String())
	}
	otherHistory := userCall("GET", "/api/v1/agent/transfers?limit=1&offset=0", "", otherToken, nil)
	mustStatus(t, otherHistory, 200)
	managedData(t, otherHistory, &records)
	if records.Total != "0" || len(records.Items) != 0 {
		t.Fatal("agent transfer history leaked", otherHistory.Body.String())
	}
	offsetHistory := userCall("GET", "/api/v1/agent/transfers?limit=1&offset=1", "", agentToken, nil)
	mustStatus(t, offsetHistory, 200)
	managedData(t, offsetHistory, &records)
	if records.Total != "1" || len(records.Items) != 0 {
		t.Fatal("transfer pagination", offsetHistory.Body.String())
	}
	mustStatus(t, userCall("GET", "/api/v1/agent/transfers?member_id="+own.MemberID, "", otherToken, nil), 400)
	mustStatus(t, userCall("GET", "/api/v1/b/harbor/agent/transfers", "", agentToken, nil), 404)
	store := points.Store{DB: f.pool}
	received, e := store.Read(ctx, brand, recipient.MemberID)
	if e != nil || received.AvailablePoints.String() != "20" || received.RechargePoints.String() != "20" {
		t.Fatal(received, e)
	}
	mustStatus(t, userCall("POST", "/api/v1/agent/cash/exchanges", "agent-exchange-out", agentToken, map[string]string{"currency": "CNY", "direction": "from_points", "amount": "14", "rate": "7"}), 200)
	destResult := userCall("POST", "/api/v1/agent/cash/destinations", "agent-destination", agentToken, cash.Destination{Currency: "CNY", Channel: "bank", Holder: "Agent", Institution: "Bank", Number: "987654", Enabled: true})
	mustStatus(t, destResult, 200)
	var dest cash.Destination
	managedData(t, destResult, &dest)
	payout := map[string]any{"currency": "CNY", "amount": "1", "fee": "0", "destination_id": dest.ID, "destination_version": dest.Version}
	withdrawn := userCall("POST", "/api/v1/agent/cash/withdrawals", "agent-withdraw", agentToken, payout)
	mustStatus(t, withdrawn, 200)
	var agentWithdrawal cash.Order
	managedData(t, withdrawn, &agentWithdrawal)
	mustStatus(t, userCall("POST", "/api/v1/agent/cash/withdrawals", "agent-withdraw", agentToken, payout), 200)
	mustStatus(t, userCall("GET", "/api/v1/agent/cash/orders?kind=withdrawal", "", agentToken, nil), 200)
	mustStatus(t, userCall("POST", "/api/v1/agent/cash/orders/"+agentWithdrawal.ID+"/cancel", "agent-withdraw-cancel", agentToken, map[string]int{"version": 1}), 200)
	after, e := store.Read(ctx, brand, own.MemberID)
	if e != nil || after.AvailablePoints.String() != "29" {
		t.Fatal(after, e)
	}

	results := make(chan int, 2)
	var transfers sync.WaitGroup
	for _, key := range []string{"agent-transfer-concurrent-a", "agent-transfer-concurrent-b"} {
		transfers.Add(1)
		go func(key string) {
			defer transfers.Done()
			results <- userCall("POST", "/api/v1/agent/transfers", key, agentToken, transfer).Code
		}(key)
	}
	transfers.Wait()
	close(results)
	successes, rejections := 0, 0
	for status := range results {
		if status == 200 {
			successes++
		} else if status == 409 {
			rejections++
		} else {
			t.Fatalf("unexpected transfer status %d", status)
		}
	}
	if successes != 1 || rejections != 1 {
		t.Fatal("concurrent overspend", successes, rejections)
	}
	after, e = store.Read(ctx, brand, own.MemberID)
	if e != nil || after.AvailablePoints.String() != "9" {
		t.Fatal(after, e)
	}
	received, e = store.Read(ctx, brand, recipient.MemberID)
	if e != nil || received.AvailablePoints.String() != "40" {
		t.Fatal(received, e)
	}

	var previous *cash.AgentWalletRecord
	seen := map[string]bool{}
	for offset := 0; offset < 6; offset += 2 {
		response := userCall("GET", fmt.Sprintf("/api/v1/agent/wallet/records?limit=2&offset=%d", offset), "", agentToken, nil)
		mustStatus(t, response, 200)
		var page cash.AgentWalletRecordPage
		managedData(t, response, &page)
		if page.TotalCount != "6" || len(page.Items) != 2 {
			t.Fatal("combined pagination", response.Body.String())
		}
		for _, item := range page.Items {
			if seen[item.ID] {
				t.Fatal("duplicate combined record", item.ID)
			}
			seen[item.ID] = true
			if previous != nil && (item.CreatedAt.After(previous.CreatedAt) || item.CreatedAt.Equal(previous.CreatedAt) && item.ID > previous.ID) {
				t.Fatal("combined records not descending", previous, item)
			}
			if item.Kind == "transfer" {
				if item.Transfer == nil || item.Order != nil || item.Transfer.MemberID != recipient.MemberID {
					t.Fatal("wrong transfer record", item)
				}
			} else if item.Order == nil || item.Transfer != nil || item.Order.MemberID != own.MemberID {
				t.Fatal("wrong cash record", item)
			}
			copy := item
			previous = &copy
		}
	}
	empty := userCall("GET", "/api/v1/agent/wallet/records?limit=2&offset=6", "", agentToken, nil)
	mustStatus(t, empty, 200)
	var emptyPage cash.AgentWalletRecordPage
	managedData(t, empty, &emptyPage)
	if emptyPage.TotalCount != "6" || len(emptyPage.Items) != 0 {
		t.Fatal("combined empty page", empty.Body.String())
	}
	otherRecords := userCall("GET", "/api/v1/agent/wallet/records", "", otherToken, nil)
	mustStatus(t, otherRecords, 200)
	managedData(t, otherRecords, &emptyPage)
	if emptyPage.TotalCount != "0" {
		t.Fatal("combined records leaked", otherRecords.Body.String())
	}
	mustStatus(t, userCall("GET", "/api/v1/agent/wallet/records?member_id="+own.MemberID, "", otherToken, nil), 400)
	mustStatus(t, userCall("GET", "/api/v1/b/harbor/agent/wallet/records", "", agentToken, nil), 404)

	recipientLogin := userCall("POST", "/api/v1/auth/login", "recipient-transfer-login", "", map[string]string{"identifier": "agent_transfer_member", "password": "member123"})
	mustStatus(t, recipientLogin, 200)
	var recipientAuth identity.Authentication
	managedData(t, recipientLogin, &recipientAuth)
	receipts := userCall("GET", "/api/v1/wallet/ledger?limit=20&offset=0", "", recipientAuth.AccessToken, nil)
	mustStatus(t, receipts, 200)
	var memberLedger struct {
		Items []userLedgerEntry
		Total string `json:"total_count"`
	}
	managedData(t, receipts, &memberLedger)
	if memberLedger.Total != "2" || len(memberLedger.Items) != 2 {
		t.Fatal("member transfer receipts", receipts.Body.String())
	}
	for _, entry := range memberLedger.Items {
		receipt := entry.AgentTransfer
		if receipt == nil || receipt.AgentID != own.MemberID || receipt.AgentAccount == nil || *receipt.AgentAccount != "cash_agent_own" || receipt.Amount.String() != "20" || !receipt.CreatedAt.Equal(entry.CreatedAt) || entry.MemberID != recipient.MemberID {
			t.Fatal("incorrect incoming agent transfer", receipts.Body.String())
		}
	}
	unrelated := userCall("GET", "/api/v1/wallet/ledger?limit=20&offset=0", "", auth.AccessToken, nil)
	mustStatus(t, unrelated, 200)
	managedData(t, unrelated, &memberLedger)
	for _, entry := range memberLedger.Items {
		if entry.AgentTransfer != nil {
			t.Fatal("incoming transfer exposed to another member", unrelated.Body.String())
		}
	}
	mustStatus(t, userCall("GET", "/api/v1/b/harbor/wallet/ledger?limit=20&offset=0", "", recipientAuth.AccessToken, nil), 404)

	if _, e = f.pool.Exec(ctx, `UPDATE agent_sessions SET revoked_at=now() WHERE brand_id=$1 AND agent_account_id=$2`, brand, own.MemberID); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, userCall("POST", "/api/v1/agent/cash/deposits", "agent-cash-original", agentToken, body), 401)
	cashContext = savedContext
	// Revocation applies before even an otherwise successful cached replay.
	if _, e = f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='recharge.confirm.brand'`, role); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("POST", path, "cash-confirm-original", f.token, brand, confirm), 403)
	var credits int
	if e = f.pool.QueryRow(ctx, `SELECT count(*) FROM cash_entries WHERE order_id=$1 AND operation='confirm'`, order.ID).Scan(&credits); e != nil || credits != 1 {
		t.Fatal(credits, e)
	}
	mustStatus(t, f.call("POST", path, "cash-other-brand", f.token, managedBrand, confirm), 403)
	withdrawal := userCall("POST", "/api/v1/withdrawals", "old-point-withdrawal", auth.AccessToken, map[string]any{"points": "10"})
	mustStatus(t, withdrawal, 409)
	state := userCall("GET", "/api/v1/cash", "", auth.AccessToken, nil)
	mustStatus(t, state, 200)
	for _, kind := range []string{"deposit", "withdrawal", "to_points", "from_points"} {
		response := userCall("GET", "/api/v1/cash/orders?kind="+kind+"&limit=20&offset=0", "", auth.AccessToken, nil)
		mustStatus(t, response, 200)
		var listed struct {
			Items []cash.Order `json:"items"`
			Total string       `json:"total_count"`
		}
		managedData(t, response, &listed)
		for _, entry := range listed.Items {
			if entry.MemberID != member.MemberID || entry.BrandID != brand || entry.Kind != kind {
				t.Fatal("wallet list scope mismatch", entry)
			}
		}
	}

	mustStatus(t, userCall("GET", "/api/v1/cash/orders?member_id="+member.MemberID, "", auth.AccessToken, nil), 400)
	for _, endpoint := range []struct{ method, path string }{{"GET", "/api/v1/admin/cash/tron"}, {"PUT", "/api/v1/admin/cash/tron"}, {"POST", "/api/v1/admin/cash/tron/wallet"}} {
		mustStatus(t, f.call(endpoint.method, endpoint.path, "retired-wallet", f.token, brand, nil), 404)
	}
}
