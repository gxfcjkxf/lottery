package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/finance"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

const pointsBrandB = "0199a000-0000-7000-8000-000000000002"

type pointsHTTPFixture struct {
	managementHTTP
	memberID  string
	userToken string
}

func pointsFixture(t *testing.T) pointsHTTPFixture {
	t.Helper()
	f := managedFixture(t)
	ctx := context.Background()
	var roleID string
	if err := f.pool.QueryRow(ctx, `SELECT r.id::text FROM roles r JOIN admin_account_roles ar ON ar.role_id=r.id WHERE ar.account_id=$1`, f.root).Scan(&roleID); err != nil {
		t.Fatal(err)
	}
	for _, permission := range []string{"wallet.view.brand", "wallet.freeze.brand", "wallet.adjust.brand", "recharge.view.brand", "recharge.write.brand"} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, permission); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2) ON CONFLICT DO NOTHING`, roleID, permission); err != nil {
			t.Fatal(err)
		}
	}
	created := f.call("POST", "/api/v1/admin/users", "points-user-create-001", f.token, managedBrand,
		map[string]string{"username": "points_member", "password": "points-member-password-2026", "display_name": "Points member", "reason": "create finance fixture"})
	if created.Code != 201 {
		t.Fatal(created.Code, created.Body.String())
	}
	var member struct {
		MemberID string `json:"member_id"`
	}
	managedData(t, created, &member)
	login := f.call("POST", "/api/v1/auth/login", "points-user-login-001", "", managedBrand,
		map[string]string{"identifier": "points_member", "password": "points-member-password-2026", "privacy_policy_version": "dev-1", "service_terms_version": "dev-1"})
	if login.Code != 200 {
		t.Fatal(login.Code, login.Body.String())
	}
	var auth identity.Authentication
	managedData(t, login, &auth)
	return pointsHTTPFixture{managementHTTP: f, memberID: member.MemberID, userToken: auth.AccessToken}
}

func pointsCall(f managementHTTP, method, path, key, token, brand, origin string, body any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	r := httptest.NewRequest(method, "http://localhost"+path, bytes.NewReader(raw))
	r.RemoteAddr = "192.0.2.73:23456"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", key)
	r.Header.Set("X-Brand-ID", brand)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	f.http.ServeHTTP(w, r)
	return w
}

func pointRecharge(t *testing.T, f pointsHTTPFixture, pointsAmount, reason, key string) finance.Recharge {
	t.Helper()
	r := f.call("POST", "/api/v1/admin/recharges", key, f.token, managedBrand, map[string]string{
		"member_id": f.memberID, "points": pointsAmount, "proof_reference": "manual-proof-" + key,
		"remark": "offline deposit", "reason": reason,
	})
	if r.Code != 201 {
		t.Fatalf("create recharge: status=%d body=%s", r.Code, r.Body.String())
	}
	var recharge finance.Recharge
	managedData(t, r, &recharge)
	return recharge
}

func pointWallet(t *testing.T, f pointsHTTPFixture) points.Wallet {
	t.Helper()
	r := f.call("GET", "/api/v1/wallet", "", f.userToken, managedBrand, nil)
	if r.Code != 200 {
		t.Fatalf("get user wallet: status=%d body=%s", r.Code, r.Body.String())
	}
	var wallet points.Wallet
	managedData(t, r, &wallet)
	return wallet
}

func TestPointsHTTPRechargeWalletFreezeAdjustAndIsolation(t *testing.T) {
	f := pointsFixture(t)
	ctx := context.Background()
	wallet := pointWallet(t, f)
	if wallet.AccountID == "" || wallet.MemberID != f.memberID || wallet.BrandID != managedBrand || wallet.DisplayPoints != 0 {
		t.Fatalf("initial wallet mismatch: %+v", wallet)
	}
	var bucketCount int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM point_buckets WHERE brand_id=$1 AND account_id=$2`, managedBrand, wallet.AccountID).Scan(&bucketCount); err != nil || bucketCount != 12 {
		t.Fatalf("wallet has %d buckets err=%v, want all 12", bucketCount, err)
	}

	recharge := pointRecharge(t, f, "250", "verified offline payment", "points-recharge-create-001")
	if recharge.State != "pending" || recharge.LedgerEntryID != "" || recharge.Points != "250" {
		t.Fatalf("unexpected pending recharge: %+v", recharge)
	}
	wallet = pointWallet(t, f)
	if wallet.DisplayPoints != 0 {
		t.Fatalf("pending recharge credited wallet: %+v", wallet)
	}
	var count int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE reference_type='recharge' AND reference_id=$1`, recharge.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("pending recharge ledger count=%d err=%v", count, err)
	}

	confirmPath := "/api/v1/admin/recharges/" + recharge.ID + "/confirm"
	confirmBody := map[string]any{"version": recharge.Version, "reason": "bank statement matched"}
	confirmedResp := f.call("POST", confirmPath, "points-recharge-confirm-001", f.token, managedBrand, confirmBody)
	if confirmedResp.Code != 200 {
		t.Fatal(confirmedResp.Code, confirmedResp.Body.String())
	}
	var confirmed finance.Recharge
	managedData(t, confirmedResp, &confirmed)
	if confirmed.State != "confirmed" || confirmed.Version != 2 || confirmed.LedgerEntryID == "" {
		t.Fatalf("unexpected confirmation: %+v", confirmed)
	}
	replay := f.call("POST", confirmPath, "points-recharge-confirm-001", f.token, managedBrand, confirmBody)
	var replayed finance.Recharge
	managedData(t, replay, &replayed)
	if replay.Code != 200 || replayed.LedgerEntryID != confirmed.LedgerEntryID {
		t.Fatalf("same-key confirm replay mismatch: %d %+v", replay.Code, replayed)
	}
	if r := f.call("POST", confirmPath, "points-recharge-confirm-002", f.token, managedBrand, confirmBody); r.Code != 409 {
		t.Fatalf("new-key duplicate confirm status=%d body=%s, want 409", r.Code, r.Body.String())
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE reference_type='recharge' AND reference_id=$1`, recharge.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("duplicate confirmation wrote %d ledger entries err=%v", count, err)
	}
	wallet = pointWallet(t, f)
	if wallet.BySource[0][0] != 250 || wallet.RechargePoints != 250 || wallet.AvailablePoints != 250 || wallet.DisplayPoints != 250 {
		t.Fatalf("recharge did not credit exact available bucket: %+v", wallet)
	}
	ledger := f.call("GET", "/api/v1/admin/wallets/"+f.memberID+"/ledger", "", f.token, managedBrand, nil)
	var ledgerResult struct {
		Items []points.Entry `json:"items"`
	}
	if ledger.Code != 200 {
		t.Fatal(ledger.Code, ledger.Body.String())
	}
	managedData(t, ledger, &ledgerResult)
	if len(ledgerResult.Items) != 1 || ledgerResult.Items[0].Before[0][0] != 0 || ledgerResult.Items[0].Delta[0][0] != 250 || ledgerResult.Items[0].After[0][0] != 250 {
		t.Fatalf("recharge ledger snapshots incorrect: %+v", ledgerResult.Items)
	}

	badUnfreeze := f.call("POST", "/api/v1/admin/wallets/"+f.memberID+"/unfreeze", "points-unfreeze-recharge-entry", f.token, managedBrand,
		map[string]string{"entry_id": confirmed.LedgerEntryID, "reason": "wrong source entry"})
	if badUnfreeze.Code != 400 {
		t.Fatalf("recharge ledger entry accepted as freeze reversal: status=%d body=%s", badUnfreeze.Code, badUnfreeze.Body.String())
	}
	freeze := f.call("POST", "/api/v1/admin/wallets/"+f.memberID+"/freeze", "points-freeze-001", f.token, managedBrand, map[string]string{"points": "70", "reason": "manual hold"})
	if freeze.Code != 200 {
		t.Fatal(freeze.Code, freeze.Body.String())
	}
	var frozen points.Entry
	managedData(t, freeze, &frozen)
	wallet = pointWallet(t, f)
	if wallet.BySource[0][0] != 180 || wallet.BySource[0][1] != 70 || wallet.AvailablePoints != 180 || wallet.ManualFrozenPoints != 70 || wallet.FrozenPoints != 70 || wallet.DisplayPoints != 250 || wallet.RechargePoints != 180 {
		t.Fatalf("freeze did not preserve source and summary buckets: %+v", wallet)
	}
	unfreezePath := "/api/v1/admin/wallets/" + f.memberID + "/unfreeze"
	unfreeze := f.call("POST", unfreezePath, "points-unfreeze-001", f.token, managedBrand, map[string]string{"entry_id": frozen.ID, "reason": "hold cleared"})
	if unfreeze.Code != 200 {
		t.Fatal(unfreeze.Code, unfreeze.Body.String())
	}
	var reversed points.Entry
	managedData(t, unfreeze, &reversed)
	if reversed.ReversalOf != frozen.ID || reversed.Delta[0][0] != 70 || reversed.Delta[0][1] != -70 {
		t.Fatalf("unfreeze did not reverse exact original allocation: %+v", reversed)
	}
	if retry := f.call("POST", unfreezePath, "points-unfreeze-002", f.token, managedBrand, map[string]string{"entry_id": frozen.ID, "reason": "duplicate clear"}); retry.Code != 409 {
		t.Fatalf("duplicate unfreeze status=%d body=%s, want 409", retry.Code, retry.Body.String())
	}
	wallet = pointWallet(t, f)
	if wallet.BySource[0][0] != 250 || wallet.BySource[0][1] != 0 || wallet.DisplayPoints != 250 {
		t.Fatalf("unfreeze did not restore original wallet: %+v", wallet)
	}

	adjustPath := "/api/v1/admin/wallets/" + f.memberID + "/adjust"
	if r := f.call("POST", adjustPath, "points-gift-plus", f.token, managedBrand, map[string]string{"source": "gift", "delta": "30", "reason": "gift adjustment"}); r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	if r := f.call("POST", adjustPath, "points-gift-floor-fail", f.token, managedBrand, map[string]string{"source": "gift", "delta": "-31", "reason": "do not underflow"}); r.Code != 409 {
		t.Fatalf("negative adjustment underflow status=%d body=%s", r.Code, r.Body.String())
	}
	if r := f.call("POST", adjustPath, "points-gift-zero", f.token, managedBrand, map[string]string{"source": "gift", "delta": "-30", "reason": "return gift"}); r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	wallet = pointWallet(t, f)
	if wallet.GiftPoints != 0 || wallet.BySource[2][0] != 0 || wallet.DisplayPoints != 250 {
		t.Fatalf("gift adjustments violated floor or source totals: %+v", wallet)
	}

	if r := f.call("GET", "/api/v1/b/harbor/wallet", "", f.userToken, managedBrand, nil); r.Code != 401 {
		t.Fatalf("user token crossed brand on wallet read: %d %s", r.Code, r.Body.String())
	}
	if r := f.call("GET", "/api/v1/admin/wallets/"+f.memberID, "", f.token, pointsBrandB, nil); r.Code != 403 {
		t.Fatalf("brand A admin read wallet in brand B: %d %s", r.Code, r.Body.String())
	}
	if r := f.call("POST", "/api/v1/admin/recharges", "cross-brand-recharge", f.token, pointsBrandB, map[string]string{"member_id": f.memberID, "points": "10", "proof_reference": "cross", "reason": "wrong brand"}); r.Code != 403 {
		t.Fatalf("brand A admin wrote recharge in brand B: %d %s", r.Code, r.Body.String())
	}
	csrf := pointsCall(f.managementHTTP, "POST", "/api/v1/admin/wallets/"+f.memberID+"/freeze", "points-csrf", f.token, managedBrand, "http://evil.example", map[string]string{"points": "1", "reason": "bad origin"})
	if csrf.Code != 403 {
		t.Fatalf("cross-origin points write status=%d body=%s, want 403", csrf.Code, csrf.Body.String())
	}
}

func TestPointsHTTPReadOnlyPermissionCannotFreeze(t *testing.T) {
	f := pointsFixture(t)
	role := f.call("POST", "/api/v1/admin/roles", "points-reader-role", f.token, managedBrand, map[string]any{"code": "wallet_reader", "name": "Wallet reader", "permissions": []string{"wallet.view.brand"}, "reason": "read wallet only"})
	if role.Code != 201 {
		t.Fatal(role.Code, role.Body.String())
	}
	var roleRecord adminsys.RoleRecord
	managedData(t, role, &roleRecord)
	password := "wallet-reader-password-2026"
	created := f.call("POST", "/api/v1/admin/accounts", "points-reader-account", f.token, managedBrand, map[string]any{"username": "wallet_reader", "password": password, "role_ids": []string{roleRecord.ID}, "reason": "create wallet reader"})
	if created.Code != 201 {
		t.Fatal(created.Code, created.Body.String())
	}
	login := f.call("POST", "/api/v1/admin/auth/login", "points-reader-login", "", managedBrand, map[string]string{"identifier": "wallet_reader", "password": password})
	if login.Code != 200 {
		t.Fatal(login.Code, login.Body.String())
	}
	var auth identity.AdminAuthentication
	managedData(t, login, &auth)
	if r := f.call("GET", "/api/v1/admin/wallets/"+f.memberID, "", auth.AccessToken, managedBrand, nil); r.Code != 200 {
		t.Fatalf("wallet.view could not read wallet: %d %s", r.Code, r.Body.String())
	}
	if r := f.call("POST", "/api/v1/admin/wallets/"+f.memberID+"/freeze", "reader-freeze-denied", auth.AccessToken, managedBrand, map[string]string{"points": "1", "reason": "should be denied"}); r.Code != 403 {
		t.Fatalf("wallet.view-only actor froze balance: %d %s", r.Code, r.Body.String())
	}
}

func TestPointsHTTPConcurrentRechargeConfirmationsSerializeWithoutDoubleCredit(t *testing.T) {
	f := pointsFixture(t)
	ctx := context.Background()
	same := pointRecharge(t, f, "11", "same order", "points-concurrent-same-create")
	statuses := concurrentConfirms(f.managementHTTP, []finance.Recharge{same}, 8, "same-order")
	success, conflicts := 0, 0
	for _, status := range statuses {
		switch status {
		case 200:
			success++
		case 409:
			conflicts++
		default:
			t.Fatalf("same-order concurrent confirmation status=%d", status)
		}
	}
	if success != 1 || conflicts != 7 {
		t.Fatalf("same order confirmations: success=%d conflicts=%d", success, conflicts)
	}

	orders := make([]finance.Recharge, 8)
	for i := range orders {
		orders[i] = pointRecharge(t, f, "3", "parallel account posting", fmt.Sprintf("points-concurrent-create-%03d", i))
	}
	statuses = concurrentConfirms(f.managementHTTP, orders, 1, "different-orders")
	for i, status := range statuses {
		if status != 200 {
			t.Fatalf("distinct order %d confirmation status=%d", i, status)
		}
	}
	var count int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries WHERE member_id=$1 AND entry_type='recharge'`, f.memberID).Scan(&count); err != nil || count != 9 {
		t.Fatalf("concurrent recharge ledger count=%d err=%v, want 9", count, err)
	}
	wallet := pointWallet(t, pointsHTTPFixture{managementHTTP: f.managementHTTP, memberID: f.memberID, userToken: f.userToken})
	if wallet.RechargePoints != 35 || wallet.DisplayPoints != 35 {
		t.Fatalf("different-order confirmations did not all credit account: %+v", wallet)
	}
}

func concurrentConfirms(f managementHTTP, orders []finance.Recharge, repeats int, keyPrefix string) []int {
	total := len(orders) * repeats
	statuses := make([]int, total)
	type result struct{ index, status int }
	results := make(chan result, total)
	var wait sync.WaitGroup
	for orderIndex, order := range orders {
		for repeat := 0; repeat < repeats; repeat++ {
			index := orderIndex*repeats + repeat
			key := fmt.Sprintf("%s-%03d-%03d", keyPrefix, orderIndex, repeat)
			wait.Add(1)
			go func(index int, order finance.Recharge, key string) {
				defer wait.Done()
				path := "/api/v1/admin/recharges/" + order.ID + "/confirm"
				r := f.call("POST", path, key, f.token, managedBrand, map[string]any{"version": order.Version, "reason": "concurrent confirmation"})
				results <- result{index: index, status: r.Code}
			}(index, order, key)
		}
	}
	done := make(chan struct{})
	go func() { wait.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		panic("concurrent recharge confirmations deadlocked or exceeded 20 seconds")
	}
	close(results)
	for result := range results {
		statuses[result.index] = result.status
	}
	return statuses
}
