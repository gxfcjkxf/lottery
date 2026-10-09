package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rewards"
)

const rewardOrdersHTTPPath = "/api/v1/admin/reward-orders"

func rewardFixture(t *testing.T) pointsHTTPFixture {
	t.Helper()
	return pointsFixture(t)
}

func grantRewardPermission(t *testing.T, f managementHTTP, permissions ...string) {
	t.Helper()
	all := append([]string{"reward.view.brand"}, permissions...)
	for _, permission := range all {
		if _, err := f.pool.Exec(context.Background(), `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, permission); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(context.Background(), `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,$2 FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root, permission); err != nil {
			t.Fatal(err)
		}
	}
}

func rewardRawCall(f managementHTTP, method, path, key, token, brand, actor, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(body))
	r.RemoteAddr = "192.0.2.55:12345"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", key)
	r.Header.Set("X-Brand-ID", brand)
	if actor != "" {
		r.Header.Set("X-Reward-Actor-ID", actor)
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.http.ServeHTTP(w, r)
	return w
}

func rewardErrorCode(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v body=%s", err, response.Body.String())
	}
	if body.Error == nil {
		t.Fatalf("response has no error: status=%d body=%s", response.Code, response.Body.String())
	}
	return body.Error.Code
}

func rewardCreate(t *testing.T, f pointsHTTPFixture, key, amount string) map[string]any {
	t.Helper()
	body := `{"member_id":"` + f.memberID + `","points":"` + amount + `","reason":"approved customer reward"}`
	response := rewardRawCall(f.managementHTTP, "POST", rewardOrdersHTTPPath, key, f.token, managedBrand, f.root, body)
	if response.Code != 201 {
		t.Fatalf("grant status=%d body=%s", response.Code, response.Body.String())
	}
	var result struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result.Data
}

func TestRewardsHTTPGrantRevokeReplayAndAuditedReads(t *testing.T) {
	f := rewardFixture(t)
	grantRewardPermission(t, f.managementHTTP, "reward.grant.brand", "reward.revoke.brand")
	body := `{"member_id":"` + f.memberID + `","points":"25","reason":"approved customer reward"}`
	first := rewardRawCall(f.managementHTTP, "POST", rewardOrdersHTTPPath, "reward-grant-replay-001", f.token, managedBrand, f.root, body)
	if first.Code != 201 {
		t.Fatalf("grant status=%d body=%s", first.Code, first.Body.String())
	}
	var initial struct {
		Data struct {
			ID      string `json:"id"`
			State   string `json:"state"`
			Version int64  `json:"version"`
			Points  string `json:"points"`
		} `json:"data"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &initial); err != nil {
		t.Fatal(err)
	}
	if initial.Data.ID == "" || initial.Data.State != "granted" || initial.Data.Version != 1 || initial.Data.Points != "25" {
		t.Fatalf("unexpected grant response: %+v", initial.Data)
	}
	replay := rewardRawCall(f.managementHTTP, "POST", rewardOrdersHTTPPath, "reward-grant-replay-001", f.token, managedBrand, f.root, body)
	if replay.Code != 201 {
		t.Fatalf("grant replay status=%d body=%s", replay.Code, replay.Body.String())
	}
	var replayed struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(replay.Body.Bytes(), &replayed)
	if replayed.Data.ID != initial.Data.ID {
		t.Fatal("grant replay did not return the original order")
	}

	list := rewardRawCall(f.managementHTTP, "GET", rewardOrdersHTTPPath+"?limit=10&offset=0", "", f.token, managedBrand, "", "")
	if list.Code != 200 {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	detail := rewardRawCall(f.managementHTTP, "GET", rewardOrdersHTTPPath+"/"+initial.Data.ID, "", f.token, managedBrand, "", "")
	if detail.Code != 200 {
		t.Fatalf("detail status=%d body=%s", detail.Code, detail.Body.String())
	}
	actions := rewardRawCall(f.managementHTTP, "GET", rewardOrdersHTTPPath+"/"+initial.Data.ID+"/actions", "", f.token, managedBrand, "", "")
	var actionPage struct {
		Data rewards.ActionPage `json:"data"`
	}
	if actions.Code != 200 || json.Unmarshal(actions.Body.Bytes(), &actionPage) != nil || actionPage.Data.OrderID != initial.Data.ID || len(actionPage.Data.Items) != 1 || actionPage.Data.Items[0].Operation != "grant" {
		t.Fatalf("action history status=%d body=%s", actions.Code, actions.Body.String())
	}
	emptyActions := rewardRawCall(f.managementHTTP, "GET", rewardOrdersHTTPPath+"/"+initial.Data.ID+"/actions?offset=20", "", f.token, managedBrand, "", "")
	var emptyPage struct {
		Data rewards.ActionPage `json:"data"`
	}
	if emptyActions.Code != 200 || json.Unmarshal(emptyActions.Body.Bytes(), &emptyPage) != nil || emptyPage.Data.OrderID != initial.Data.ID || emptyPage.Data.Items == nil || len(emptyPage.Data.Items) != 0 {
		t.Fatalf("empty action page lost order context: status=%d body=%s", emptyActions.Code, emptyActions.Body.String())
	}

	revokeBody := `{"version":1,"reason":"customer request reversal"}`
	revoked := rewardRawCall(f.managementHTTP, "POST", rewardOrdersHTTPPath+"/"+initial.Data.ID+"/revoke", "reward-revoke-replay-01", f.token, managedBrand, f.root, revokeBody)
	if revoked.Code != 200 || !strings.Contains(revoked.Body.String(), `"state":"revoked"`) {
		t.Fatalf("revoke status=%d body=%s", revoked.Code, revoked.Body.String())
	}
	revokeReplay := rewardRawCall(f.managementHTTP, "POST", rewardOrdersHTTPPath+"/"+initial.Data.ID+"/revoke", "reward-revoke-replay-01", f.token, managedBrand, f.root, revokeBody)
	if revokeReplay.Code != 200 || !strings.Contains(revokeReplay.Body.String(), `"state":"revoked"`) {
		t.Fatalf("revoke replay status=%d body=%s", revokeReplay.Code, revokeReplay.Body.String())
	}
	var orders, actionsCount, audits int
	ctx := context.Background()
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM reward_orders WHERE brand_id=$1`, managedBrand).Scan(&orders); err != nil || orders != 1 {
		t.Fatalf("grant replay created %d orders err=%v", orders, err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM reward_order_actions WHERE order_id=$1`, initial.Data.ID).Scan(&actionsCount); err != nil || actionsCount != 2 {
		t.Fatalf("replay changed action history count=%d err=%v", actionsCount, err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE actor_id=$1 AND action LIKE 'reward.order.%'`, f.root).Scan(&audits); err != nil || audits < 4 {
		t.Fatalf("reward activity was not audited count=%d err=%v", audits, err)
	}
}

func TestRewardsHTTPActorBindingAndPermissionAreCheckedBeforeReceiptReplay(t *testing.T) {
	f := rewardFixture(t)
	grantRewardPermission(t, f.managementHTTP, "reward.grant.brand")
	body := `{"member_id":"` + f.memberID + `","points":"3","reason":"actor replay guard"}`
	key := "reward-actor-replay-001"
	for _, actor := range []string{"", ids.New(), strings.ToUpper(f.root)} {
		response := rewardRawCall(f.managementHTTP, "POST", rewardOrdersHTTPPath, key, f.token, managedBrand, actor, body)
		if response.Code != 401 || rewardErrorCode(t, response) != "AUTH_ACTOR_CONTEXT_CHANGED" {
			t.Fatalf("invalid actor status=%d body=%s", response.Code, response.Body.String())
		}
	}
	var receipts int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM idempotency_requests WHERE operation='admin.reward_order.grant' AND key=$1`, key).Scan(&receipts); err != nil || receipts != 0 {
		t.Fatalf("actor mismatch cached receipt count=%d err=%v", receipts, err)
	}
	first := rewardRawCall(f.managementHTTP, "POST", rewardOrdersHTTPPath, key, f.token, managedBrand, f.root, body)
	if first.Code != 201 {
		t.Fatalf("valid actor grant status=%d body=%s", first.Code, first.Body.String())
	}
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE permission_key='reward.grant.brand' AND role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1)`, f.root); err != nil {
		t.Fatal(err)
	}
	denied := rewardRawCall(f.managementHTTP, "POST", rewardOrdersHTTPPath, key, f.token, managedBrand, f.root, body)
	if denied.Code != 403 || rewardErrorCode(t, denied) != "PERMISSION_DENIED" {
		t.Fatalf("revoked permission replay status=%d body=%s", denied.Code, denied.Body.String())
	}
}

func TestRewardsHTTPClosedBodiesQueriesAndTargetValidation(t *testing.T) {
	f := rewardFixture(t)
	grantRewardPermission(t, f.managementHTTP, "reward.grant.brand", "reward.revoke.brand")
	for i, body := range []string{
		`{}`, `null`, `{"member_id":"` + f.memberID + `","points":"1"}`,
		`{"member_id":"` + f.memberID + `","points":"1","reason":"ok","extra":true}`,
		`{"member_id":"` + f.memberID + `","member_id":"` + f.memberID + `","points":"1","reason":"ok"}`,
		`{"member_id":"` + f.memberID + `","points":"1","reason":"ok"} {}`,
	} {
		response := rewardRawCall(f.managementHTTP, "POST", rewardOrdersHTTPPath, "reward-closed-body-001", f.token, managedBrand, f.root, body)
		if response.Code != 400 || rewardErrorCode(t, response) != "REWARD_INPUT_INVALID" {
			t.Errorf("body %d was not rejected strictly: status=%d body=%s", i, response.Code, response.Body.String())
		}
	}
	for _, query := range []string{"?limit=0", "?limit=20&limit=30", "?unknown=1", "?limit=+20", "?offset=001", "?limit=%ZZ", "?"} {
		response := rewardRawCall(f.managementHTTP, "GET", rewardOrdersHTTPPath+query, "", f.token, managedBrand, "", "")
		if response.Code != 400 {
			t.Errorf("invalid page query accepted query=%q status=%d body=%s", query, response.Code, response.Body.String())
		}
	}
	for _, path := range []string{rewardOrdersHTTPPath, rewardOrdersHTTPPath + "/" + ids.New(), rewardOrdersHTTPPath + "/" + ids.New() + "/actions"} {
		response := rewardRawCall(f.managementHTTP, "GET", path, "", f.token, managedBrand, "", `{"ignored":"body"}`)
		if response.Code != 400 || rewardErrorCode(t, response) != "REWARD_INPUT_INVALID" {
			t.Errorf("GET body was ignored for %s: status=%d body=%s", path, response.Code, response.Body.String())
		}
		request := httptest.NewRequest("GET", "http://localhost"+path, strings.NewReader(`{"chunked":true}`))
		request.ContentLength = -1
		request.TransferEncoding = []string{"chunked"}
		request.Header.Set("X-Brand-ID", managedBrand)
		request.Header.Set("Authorization", "Bearer "+f.token)
		out := httptest.NewRecorder()
		f.http.ServeHTTP(out, request)
		if out.Code != 400 || rewardErrorCode(t, out) != "REWARD_INPUT_INVALID" {
			t.Errorf("chunked GET body was ignored for %s: status=%d body=%s", path, out.Code, out.Body.String())
		}
	}
	for _, path := range []string{rewardOrdersHTTPPath + "/" + ids.New() + "?limit=1", rewardOrdersHTTPPath + "/bad"} {
		response := rewardRawCall(f.managementHTTP, "GET", path, "", f.token, managedBrand, "", "")
		if response.Code != 400 {
			t.Errorf("invalid detail request accepted path=%q status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	response := rewardRawCall(f.managementHTTP, "POST", rewardOrdersHTTPPath+"/"+ids.New()+"/revoke", "reward-target-before-receipt-01", f.token, managedBrand, ids.New(), `{"version":1,"reason":"actor mismatch must precede lookup"}`)
	if response.Code != 401 {
		t.Fatalf("actor mismatch reached target lookup: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRewardsHTTPNoPermissionInheritanceAndSuperAdminMutationDenied(t *testing.T) {
	f := rewardFixture(t)
	for _, key := range []string{"wallet.view.brand", "commission.view.brand", "recharge.view.brand", "wallet.adjust.brand", "commission_payment.approve.brand", "recharge.write.brand"} {
		if _, err := f.pool.Exec(context.Background(), `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, key); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(context.Background(), `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,$2 FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root, key); err != nil {
			t.Fatal(err)
		}
	}
	response := rewardRawCall(f.managementHTTP, "GET", rewardOrdersHTTPPath, "", f.token, managedBrand, "", "")
	if response.Code != 403 || rewardErrorCode(t, response) != "PERMISSION_DENIED" {
		t.Fatalf("inherited wallet/commission/recharge permission allowed rewards read: status=%d body=%s", response.Code, response.Body.String())
	}
	response = rewardRawCall(f.managementHTTP, "POST", rewardOrdersHTTPPath, "rewards-no-inherit-001", f.token, managedBrand, f.root,
		`{"member_id":"`+f.memberID+`","points":"1","reason":"must not inherit another namespace"}`)
	if response.Code != 403 || rewardErrorCode(t, response) != "PERMISSION_DENIED" {
		t.Fatalf("inherited wallet/commission/recharge permission allowed rewards grant: status=%d body=%s", response.Code, response.Body.String())
	}
	grantRewardPermission(t, f.managementHTTP, "reward.grant.brand")
	otherBrand := rewardRawCall(f.managementHTTP, "GET", rewardOrdersHTTPPath, "", f.token, pointsBrandB, "", "")
	if otherBrand.Code != 403 || rewardErrorCode(t, otherBrand) != "PERMISSION_DENIED" {
		t.Fatalf("brand permission crossed into another brand: status=%d body=%s", otherBrand.Code, otherBrand.Body.String())
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	read := rewardRawCall(f.managementHTTP, "GET", rewardOrdersHTTPPath, "", f.token, managedBrand, "", "")
	if read.Code != 403 {
		t.Fatalf("brand permission reached super account through brand entry: status=%d body=%s", read.Code, read.Body.String())
	}
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO permissions(key) VALUES('reward.view.platform') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	platformRole := ids.New()
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO roles(id,code,name) VALUES($1,$2,'Platform reward reader')`, platformRole, "reward_platform_"+platformRole[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,'reward.view.platform')`, platformRole); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, f.root, platformRole); err != nil {
		t.Fatal(err)
	}
	platformToken := platformAdminToken(t, f.managementHTTP)
	platformPath := strings.Replace(rewardOrdersHTTPPath, "/api/v1/admin", "/api/v1/platform", 1)
	read = rewardRawCall(f.managementHTTP, "GET", platformPath, "", platformToken, managedBrand, "", "")
	if read.Code != 200 {
		t.Fatalf("platform grant did not authorize reward read: status=%d body=%s", read.Code, read.Body.String())
	}
	write := rewardRawCall(f.managementHTTP, "POST", platformPath, "reward-superadmin-write-1", platformToken, managedBrand, f.root,
		`{"member_id":"`+f.memberID+`","points":"1","reason":"super administrator writes are denied"}`)
	if write.Code != 404 {
		t.Fatalf("super admin mutation status=%d body=%s", write.Code, write.Body.String())
	}
}

func TestRewardsHTTPPendingRevocationAndRetryAreNormalResponses(t *testing.T) {
	f := rewardFixture(t)
	grantRewardPermission(t, f.managementHTTP, "reward.grant.brand", "reward.revoke.brand", "reward.retry.brand")
	order := rewardCreate(t, f, "reward-pending-create-01", "20")
	orderID := order["id"].(string)
	store := points.Store{DB: f.pool}
	tx, err := f.pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var delta points.Balance
	delta[2][0] = -15
	if _, err = store.Post(context.Background(), tx, points.Change{BrandID: managedBrand, MemberID: f.memberID, EntryType: "test_gift_spend", ReferenceType: "test", OperationKey: "reward-test-spend:" + orderID,
		Reason: "consume some gift points before reversal", ActorType: "admin", ActorID: f.root, RequestID: "reward-test-gift-spend-01",
		Delta: delta, Allocation: []points.Allocation{{Source: "gift", State: "available", Points: 15}}}); err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	pending := rewardRawCall(f.managementHTTP, "POST", rewardOrdersHTTPPath+"/"+orderID+"/revoke", "reward-pending-revoke-01", f.token, managedBrand, f.root,
		`{"version":1,"reason":"revoke exceeds remaining gift balance"}`)
	if pending.Code != 200 || !strings.Contains(pending.Body.String(), `"state":"revocation_pending"`) {
		t.Fatalf("pending revoke should be a normal 200 response: status=%d body=%s", pending.Code, pending.Body.String())
	}
	var credit points.Balance
	credit[2][0] = 15
	tx, err = f.pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Post(context.Background(), tx, points.Change{BrandID: managedBrand, MemberID: f.memberID, EntryType: "test_gift_credit", ReferenceType: "test", OperationKey: "reward-test-credit:" + orderID,
		Reason: "restore gift points for reversal retry", ActorType: "admin", ActorID: f.root, RequestID: "reward-test-gift-credit-01",
		Delta: credit, Allocation: []points.Allocation{{Source: "gift", State: "available", Points: 15}}}); err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	retried := rewardRawCall(f.managementHTTP, "POST", rewardOrdersHTTPPath+"/"+orderID+"/retry-revocation", "reward-retry-revocation-01", f.token, managedBrand, f.root,
		`{"version":2,"reason":"retry after restoring gift balance"}`)
	if retried.Code != 200 || !strings.Contains(retried.Body.String(), `"state":"revoked"`) {
		t.Fatalf("retry should complete reversal: status=%d body=%s", retried.Code, retried.Body.String())
	}
	var actionCount int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM reward_order_actions WHERE order_id=$1`, orderID).Scan(&actionCount); err != nil || actionCount != 3 {
		t.Fatalf("pending/retry action count=%d err=%v", actionCount, err)
	}
	staleReceipt := rewardRawCall(f.managementHTTP, "POST", rewardOrdersHTTPPath+"/"+orderID+"/revoke", "reward-pending-revoke-01", f.token, managedBrand, f.root,
		`{"version":1,"reason":"revoke exceeds remaining gift balance"}`)
	if staleReceipt.Code != 200 || !strings.Contains(staleReceipt.Body.String(), `"state":"revocation_pending"`) {
		t.Fatalf("same-key replay should return original pending receipt: status=%d body=%s", staleReceipt.Code, staleReceipt.Body.String())
	}
	currentAttempt := rewardRawCall(f.managementHTTP, "POST", rewardOrdersHTTPPath+"/"+orderID+"/revoke", "reward-current-state-replay-01", f.token, managedBrand, f.root,
		`{"version":1,"reason":"revoke exceeds remaining gift balance"}`)
	if currentAttempt.Code != 409 || rewardErrorCode(t, currentAttempt) != "REWARD_VERSION_CONFLICT" {
		t.Fatalf("new-key request did not observe current order version: status=%d body=%s", currentAttempt.Code, currentAttempt.Body.String())
	}
	var liveState string
	if err := f.pool.QueryRow(context.Background(), `SELECT state FROM reward_orders WHERE id=$1`, orderID).Scan(&liveState); err != nil || liveState != "revoked" {
		t.Fatalf("different-key request changed live order state=%q err=%v", liveState, err)
	}
}

func TestRewardsHTTPBusyIsNotCachedAndAuditFailureWithholdsRead(t *testing.T) {
	f := rewardFixture(t)
	grantRewardPermission(t, f.managementHTTP, "reward.grant.brand")
	body := `{"member_id":"` + f.memberID + `","points":"2","reason":"busy must not be cached"}`
	tx, err := f.pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(context.Background(), `SELECT id FROM brands WHERE id=$1 FOR UPDATE`, managedBrand); err != nil {
		t.Fatal(err)
	}
	response := rewardRawCall(f.managementHTTP, "POST", rewardOrdersHTTPPath, "reward-busy-uncached-01", f.token, managedBrand, f.root, body)
	_ = tx.Rollback(context.Background())
	if response.Code != 503 || rewardErrorCode(t, response) != "REWARD_BUSY" {
		t.Fatalf("brand lock contention status=%d body=%s", response.Code, response.Body.String())
	}
	var receipts int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM idempotency_requests WHERE operation='admin.reward_order.grant' AND key='reward-busy-uncached-01'`).Scan(&receipts); err != nil || receipts != 0 {
		t.Fatalf("busy response cached receipt count=%d err=%v", receipts, err)
	}

	order := rewardCreate(t, f, "reward-read-audit-fault-create", "2")
	if _, err := f.pool.Exec(context.Background(), `CREATE FUNCTION reject_reward_read_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='reward.order.read' THEN RAISE EXCEPTION 'test reward audit unavailable'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_reward_read_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_reward_read_audit()`); err != nil {
		t.Fatal(err)
	}
	read := rewardRawCall(f.managementHTTP, "GET", rewardOrdersHTTPPath+"/"+order["id"].(string), "", f.token, managedBrand, "", "")
	if read.Code != 503 || strings.Contains(read.Body.String(), order["id"].(string)) {
		t.Fatalf("unaudited reward read leaked: status=%d body=%s", read.Code, read.Body.String())
	}
}

func TestRewardsHTTPPausedBrandWritesButDisabledBrandOnlyReads(t *testing.T) {
	f := rewardFixture(t)
	grantRewardPermission(t, f.managementHTTP, "reward.grant.brand")
	if _, err := f.pool.Exec(context.Background(), `UPDATE brands SET status='paused' WHERE id=$1`, managedBrand); err != nil {
		t.Fatal(err)
	}
	body := `{"member_id":"` + f.memberID + `","points":"2","reason":"paused brands permit reward operations"}`
	paused := rewardRawCall(f.managementHTTP, "POST", rewardOrdersHTTPPath, "reward-paused-brand-01", f.token, managedBrand, f.root, body)
	if paused.Code != 201 {
		t.Fatalf("paused-brand grant status=%d body=%s", paused.Code, paused.Body.String())
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE brands SET status='disabled' WHERE id=$1`, managedBrand); err != nil {
		t.Fatal(err)
	}
	read := rewardRawCall(f.managementHTTP, "GET", rewardOrdersHTTPPath, "", f.token, managedBrand, "", "")
	if read.Code != 200 {
		t.Fatalf("disabled-brand historical read status=%d body=%s", read.Code, read.Body.String())
	}
	disabledBody := `{"member_id":"` + f.memberID + `","points":"1","reason":"disabled brands reject reward mutations"}`
	write := rewardRawCall(f.managementHTTP, "POST", rewardOrdersHTTPPath, "reward-disabled-brand-01", f.token, managedBrand, f.root, disabledBody)
	if write.Code != 409 || rewardErrorCode(t, write) != "REWARD_STATE_CONFLICT" {
		t.Fatalf("disabled-brand grant status=%d body=%s", write.Code, write.Body.String())
	}
	var receipts int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM idempotency_requests WHERE operation='admin.reward_order.grant' AND key='reward-disabled-brand-01'`).Scan(&receipts); err != nil || receipts != 0 {
		t.Fatalf("disabled-brand state rejection cached receipt count=%d err=%v", receipts, err)
	}
}

func TestRewardsHTTPReadRechecksSessionAfterAuditWaitAndRollsBack(t *testing.T) {
	f := rewardFixture(t)
	grantRewardPermission(t, f.managementHTTP)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `CREATE FUNCTION delay_reward_list_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='reward.order.list' THEN PERFORM pg_sleep(1.2); END IF; RETURN NEW; END $$; CREATE TRIGGER delay_reward_list_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION delay_reward_list_audit()`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE sessions SET expires_at=clock_timestamp()+interval '500 milliseconds' WHERE admin_id=$1 AND revoked_at IS NULL`, f.root); err != nil {
		t.Fatal(err)
	}
	response := rewardRawCall(f.managementHTTP, "GET", rewardOrdersHTTPPath, "", f.token, managedBrand, "", "")
	if response.Code != 401 || rewardErrorCode(t, response) != "AUTH_SESSION_REVOKED" {
		t.Fatalf("session expiry during audit append status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), `"brand_id"`) || strings.Contains(response.Body.String(), `"items"`) {
		t.Fatalf("read returned DTO after session expiry: %s", response.Body.String())
	}
	var auditRows int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE actor_id=$1 AND action='reward.order.list'`, f.root).Scan(&auditRows); err != nil || auditRows != 0 {
		t.Fatalf("query audit row was not rolled back count=%d err=%v", auditRows, err)
	}
}

func TestRewardsHTTPWriteActorIsRecheckedAfterSessionRevocation(t *testing.T) {
	f := rewardFixture(t)
	grantRewardPermission(t, f.managementHTTP, "reward.grant.brand")
	body := `{"member_id":"` + f.memberID + `","points":"1","reason":"session must be fresh on replay"}`
	first := rewardRawCall(f.managementHTTP, "POST", rewardOrdersHTTPPath, "reward-session-replay-01", f.token, managedBrand, f.root, body)
	if first.Code != 201 {
		t.Fatalf("initial grant status=%d body=%s", first.Code, first.Body.String())
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET status='disabled' WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	replay := rewardRawCall(f.managementHTTP, "POST", rewardOrdersHTTPPath, "reward-session-replay-01", f.token, managedBrand, f.root, body)
	if replay.Code != 401 || rewardErrorCode(t, replay) != "AUTH_SESSION_REVOKED" {
		t.Fatalf("revoked admin replay status=%d body=%s", replay.Code, replay.Body.String())
	}
}
