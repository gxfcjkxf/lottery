package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/identity"
)

func TestMemberRechargeQueryRejectsNonCanonicalInput(t *testing.T) {
	for _, raw := range []string{
		"?", "?member_id=x", "?limit=", "?limit=+20", "?limit=01", "?limit=0",
		"?limit=101", "?limit=1&limit=2", "?offset=-1", "?offset=01",
		"?offset=1000001", "?state=", "?state=unknown", "?x=1",
	} {
		r := httptest.NewRequest("GET", "http://localhost/api/v1/recharges"+raw, nil)
		if _, ok := parseMemberRechargeQuery(r); ok {
			t.Errorf("accepted invalid query %q", raw)
		}
	}
	for raw, want := range map[string]memberRechargeQuery{
		"":                          {Limit: 20},
		"?limit=1&offset=0":         {Limit: 1},
		"?state=confirmed&limit=10": {Limit: 10, State: "confirmed"},
	} {
		r := httptest.NewRequest("GET", "http://localhost/api/v1/recharges"+raw, nil)
		got, ok := parseMemberRechargeQuery(r)
		if !ok || got != want {
			t.Errorf("query %q parsed as %+v, %v; want %+v", raw, got, ok, want)
		}
	}
}

func TestMemberRechargeHTTPUnavailableIs503(t *testing.T) {
	mux := newRouteMux()
	registerRechargeUserRoutes(mux, Dependencies{})
	r := httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/recharges", http.NoBody)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	mustStatus(t, w, 503)
}

func TestMemberRechargeHTTPPausedFrozenAndExpiredAfterAuditWait(t *testing.T) {
	f := pointsFixture(t)
	ctx := context.Background()
	order := pointRecharge(t, f, "22", "auth boundary fixture", "member-recharge-auth-boundary")
	if _, err := f.pool.Exec(ctx, `UPDATE brands SET status='paused' WHERE id=$1`, managedBrand); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE brand_members SET status='frozen' WHERE id=$1`, f.memberID); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, rechargeUserGet(f, "/api/v1/b/aurora/recharges/"+order.ID, f.userToken), 200)
	ledgerBefore, auditBefore := rechargeUserCounts(t, f)
	if _, err := f.pool.Exec(ctx, `CREATE FUNCTION delay_member_recharge_read_audit() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN IF NEW.action='finance.recharge.user.view' THEN PERFORM pg_sleep(1); END IF; RETURN NEW; END$$; CREATE TRIGGER delay_member_recharge_read_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION delay_member_recharge_read_audit()`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE sessions SET expires_at=clock_timestamp()+interval '500 milliseconds' WHERE member_id=$1 AND admin_id IS NULL`, f.memberID); err != nil {
		t.Fatal(err)
	}
	r := rechargeUserGet(f, "/api/v1/recharges", f.userToken)
	mustStatus(t, r, 401)
	if strings.Contains(r.Body.String(), order.ID) || strings.Contains(r.Body.String(), `"data"`) {
		t.Fatal("expired read released a DTO")
	}
	ledgerAfter, auditAfter := rechargeUserCounts(t, f)
	if ledgerAfter != ledgerBefore || auditAfter != auditBefore {
		t.Fatal("expired read retained audit or financial effects")
	}
}

func TestMemberRechargeHTTPProjectionIsolationAuditAndReadOnly(t *testing.T) {
	f := pointsFixture(t)
	ctx := context.Background()
	pending := pointRecharge(t, f, "150", "recharge private reason", "member-recharge-projection-pending")
	confirmed := pointRecharge(t, f, "225", "another private reason", "member-recharge-projection-confirmed")
	mustStatus(t, f.call("POST", "/api/v1/admin/recharges/"+confirmed.ID+"/confirm", "member-recharge-confirm", f.token, managedBrand, map[string]any{"version": confirmed.Version, "reason": "internal admin confirmation reason"}), 200)

	created := f.call("POST", "/api/v1/admin/users", "member-recharge-other-user", f.token, managedBrand, map[string]string{"username": "recharge_other", "password": "recharge-other-password-2026", "reason": "fixture"})
	mustStatus(t, created, 201)
	var other struct {
		MemberID string `json:"member_id"`
	}
	managedData(t, created, &other)
	login := f.call("POST", "/api/v1/auth/login", "member-recharge-other-login", "", managedBrand, map[string]string{"identifier": "recharge_other", "password": "recharge-other-password-2026", "privacy_policy_version": "dev-1", "service_terms_version": "dev-1"})
	mustStatus(t, login, 200)
	var otherAuth identity.Authentication
	managedData(t, login, &otherAuth)

	ledgerBefore, auditBefore := rechargeUserCounts(t, f)
	defaultList := rechargeUserGet(f, "/api/v1/recharges", f.userToken)
	mustStatus(t, defaultList, 200)
	var defaultPage map[string]json.RawMessage
	managedData(t, defaultList, &defaultPage)
	if string(defaultPage["state"]) != "null" || string(defaultPage["total_count"]) != `"2"` {
		t.Fatalf("default page must expose null state and exact count: %s", defaultList.Body.String())
	}
	list := rechargeUserGet(f, "/api/v1/recharges?state=confirmed&limit=1&offset=0", f.userToken)
	mustStatus(t, list, 200)
	var page struct {
		BrandID    string            `json:"brand_id"`
		MemberID   string            `json:"member_id"`
		SnapshotAt string            `json:"snapshot_at"`
		State      *string           `json:"state"`
		Items      []json.RawMessage `json:"items"`
		Limit      int               `json:"limit"`
		Offset     int               `json:"offset"`
		TotalCount string            `json:"total_count"`
	}
	managedData(t, list, &page)
	if page.BrandID != managedBrand || page.MemberID != f.memberID || page.State == nil || *page.State != "confirmed" || page.Limit != 1 || page.Offset != 0 || page.TotalCount != "1" || len(page.Items) != 1 || page.SnapshotAt == "" {
		t.Fatalf("unexpected member recharge page: %+v", page)
	}
	var projected map[string]json.RawMessage
	if err := json.Unmarshal(page.Items[0], &projected); err != nil {
		t.Fatal(err)
	}
	if _, ok := projected["confirmed_at"]; !ok {
		t.Fatal("projection omitted explicit confirmed_at")
	}
	if _, ok := projected["ledger_entry_id"]; !ok {
		t.Fatal("projection omitted explicit ledger_entry_id")
	}
	detail := rechargeUserGet(f, "/api/v1/recharges/"+confirmed.ID, f.userToken)
	mustStatus(t, detail, 200)
	pendingDetail := rechargeUserGet(f, "/api/v1/recharges/"+pending.ID, f.userToken)
	mustStatus(t, pendingDetail, 200)
	for _, response := range []*httptest.ResponseRecorder{defaultList, list, detail, pendingDetail} {
		body := response.Body.String()
		for _, secret := range []string{"account_id", "proof_reference", "manual-proof-", "remark", "created_by", "confirmed_by", "audit_log_id", "internal admin confirmation reason", "another private reason"} {
			if strings.Contains(body, secret) {
				t.Fatalf("member response leaked %q: %s", secret, body)
			}
		}
	}
	var confirmedDTO, pendingDTO map[string]json.RawMessage
	managedData(t, detail, &confirmedDTO)
	managedData(t, pendingDetail, &pendingDTO)
	if string(confirmedDTO["version"]) != `"2"` || string(confirmedDTO["confirmed_at"]) == "null" || string(confirmedDTO["ledger_entry_id"]) == "null" {
		t.Fatalf("confirmed projection has wrong version or missing confirmation fields: %s", detail.Body.String())
	}
	if string(pendingDTO["confirmed_at"]) != "null" || string(pendingDTO["ledger_entry_id"]) != "null" {
		t.Fatalf("pending projection must include explicit null confirmation fields: %s", pendingDetail.Body.String())
	}
	var ownAuditCount int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE action='finance.recharge.user.detail' AND resource_id=$1`, pending.ID).Scan(&ownAuditCount); err != nil || ownAuditCount != 1 {
		t.Fatalf("own detail audit resource count=%d err=%v", ownAuditCount, err)
	}
	assertMemberRechargeAudit(t, f)
	for _, path := range []string{
		"/api/v1/recharges?", "/api/v1/recharges?member_id=" + f.memberID,
		"/api/v1/recharges?limit=+20", "/api/v1/recharges?limit=01",
		"/api/v1/recharges?limit=20&limit=20", "/api/v1/recharges?state=",
		"/api/v1/recharges?state=unexpected", "/api/v1/recharges?extra=1",
		"/api/v1/recharges/" + confirmed.ID + "?x=1",
	} {
		mustStatus(t, rechargeUserGet(f, path, f.userToken), 400)
	}
	bodyRequest := rechargeUserRawCall(f, "GET", "/api/v1/recharges", f.userToken, `{"member_id":"`+other.MemberID+`"}`)
	mustStatus(t, bodyRequest, 400)
	bodyDetailRequest := rechargeUserRawCall(f, "GET", "/api/v1/recharges/"+confirmed.ID, f.userToken, `null`)
	mustStatus(t, bodyDetailRequest, 400)

	foreign := rechargeUserGet(f, "/api/v1/recharges/"+confirmed.ID, otherAuth.AccessToken)
	mustStatus(t, foreign, 404)
	foreignAuditResourceID, foreignAuditAfter := rechargeUserLastAudit(t, f, "finance.recharge.user.detail")
	if foreignAuditResourceID != "" || !strings.Contains(foreignAuditAfter, `"outcome": "not_found"`) || strings.Contains(foreignAuditAfter, confirmed.ID) {
		t.Fatalf("foreign detail audit exposed resource: resource=%q after=%s", foreignAuditResourceID, foreignAuditAfter)
	}
	mustStatus(t, rechargeUserGet(f, "/api/v1/b/harbor/recharges", f.userToken), 401)

	if _, err := f.pool.Exec(ctx, `UPDATE sessions SET revoked_at=clock_timestamp() WHERE token_hash=(SELECT token_hash FROM sessions WHERE member_id=$1 AND admin_id IS NULL LIMIT 1)`, f.memberID); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, rechargeUserGet(f, "/api/v1/recharges", f.userToken), 401)
	ledgerAfter, auditAfter := rechargeUserCounts(t, f)
	if ledgerAfter != ledgerBefore || auditAfter <= auditBefore {
		t.Fatalf("read path changed financial ledger or missed read audit: ledger %d->%d audits %d->%d", ledgerBefore, ledgerAfter, auditBefore, auditAfter)
	}
	var pendingState string
	if err := f.pool.QueryRow(ctx, `SELECT state FROM recharge_orders WHERE id=$1`, pending.ID).Scan(&pendingState); err != nil || pendingState != "pending" {
		t.Fatalf("read path changed pending recharge state: %q %v", pendingState, err)
	}
}

func TestMemberRechargeHTTPAuditFailureWithholdsData(t *testing.T) {
	f := pointsFixture(t)
	order := pointRecharge(t, f, "80", "audit failure fixture", "member-recharge-audit-failure-order")
	if _, err := f.pool.Exec(context.Background(), `CREATE FUNCTION reject_member_recharge_read_audit() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN IF NEW.action IN ('finance.recharge.user.view','finance.recharge.user.detail') THEN RAISE EXCEPTION 'injected audit failure'; END IF; RETURN NEW; END$$; CREATE TRIGGER reject_member_recharge_read_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_member_recharge_read_audit()`); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/recharges", "/api/v1/recharges/" + order.ID} {
		r := rechargeUserGet(f, path, f.userToken)
		mustStatus(t, r, 503)
		if strings.Contains(r.Body.String(), `"data"`) || strings.Contains(r.Body.String(), order.ID) {
			t.Fatalf("audit failure leaked read response: %s", r.Body.String())
		}
	}
}

func rechargeUserRawCall(f pointsHTTPFixture, method, path, token, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	f.http.ServeHTTP(w, r)
	return w
}

func rechargeUserGet(f pointsHTTPFixture, path, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, "http://localhost"+path, http.NoBody)
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("X-Brand-ID", managedBrand)
	w := httptest.NewRecorder()
	f.http.ServeHTTP(w, r)
	return w
}

func rechargeUserCounts(t *testing.T, f pointsHTTPFixture) (int, int) {
	t.Helper()
	var ledger, audits int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM point_ledger_entries`).Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE action LIKE 'finance.recharge.user.%'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	return ledger, audits
}

func rechargeUserLastAudit(t *testing.T, f pointsHTTPFixture, action string) (string, string) {
	t.Helper()
	var resourceID, after string
	err := f.pool.QueryRow(context.Background(), `SELECT coalesce(resource_id::text,''),after_json::text FROM audit_logs WHERE action=$1 ORDER BY created_at DESC,id DESC LIMIT 1`, action).Scan(&resourceID, &after)
	if err != nil {
		t.Fatalf("read audit record: %v", err)
	}
	return resourceID, after
}

func assertMemberRechargeAudit(t *testing.T, f pointsHTTPFixture) {
	t.Helper()
	ctx := context.Background()
	var userID string
	if err := f.pool.QueryRow(ctx, `SELECT global_user_id::text FROM brand_members WHERE id=$1`, f.memberID).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	rows, err := f.pool.Query(ctx, `SELECT actor_type,actor_id::text,after_json FROM audit_logs WHERE action IN ('finance.recharge.user.view','finance.recharge.user.detail')`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var actorType, actorID string
		var raw []byte
		if err = rows.Scan(&actorType, &actorID, &raw); err != nil {
			t.Fatal(err)
		}
		var after map[string]json.RawMessage
		if err = json.Unmarshal(raw, &after); err != nil {
			t.Fatal(err)
		}
		if actorType != "user" || actorID == "" {
			t.Fatalf("unexpected user recharge audit actor: type=%q id=%q after=%s", actorType, actorID, raw)
		}
		if string(after["member_id"]) == `"`+f.memberID+`"` && actorID != userID {
			t.Fatalf("member read audit used wrong global user actor: got %s want %s", actorID, userID)
		}
		if len(after) != 3 || after["member_id"] == nil || after["query"] == nil || after["outcome"] == nil {
			t.Fatalf("unexpected recharge read audit metadata: %s", raw)
		}
		count++
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count < 3 {
		t.Fatalf("expected list/detail/foreign read audits, got %d", count)
	}
}
