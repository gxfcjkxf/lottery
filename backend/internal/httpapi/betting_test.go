package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/betting"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/rulebook"
	"github.com/gxfcjkxf/lottery/backend/internal/tenant"
)

func betHTTP(f managementHTTP) http.Handler {
	users, err := identity.New(f.pool)
	if err != nil {
		panic(err)
	}
	engine, err := mutation.New(f.pool, make([]byte, 32))
	if err != nil {
		panic(err)
	}
	d := Dependencies{Brands: tenant.Store{DB: f.pool}, Ready: f.pool.Ping, Identity: users, Mutations: engine,
		Admins: adminsys.Store{DB: f.pool}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	return New(d)
}

func betRequest(h http.Handler, method, path, token, origin, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://localhost"+path, bytes.NewBufferString(body))
	r.RemoteAddr = "192.0.2.73:23456"
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestBettingHTTPMissingDependenciesAndClosedRequestValidation(t *testing.T) {
	mux := http.NewServeMux()
	registerBetRoutes(mux, Dependencies{})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/bet-previews", nil))
	if w.Code != 503 {
		t.Fatalf("missing dependencies status=%d body=%s", w.Code, w.Body.String())
	}

	f := pointsFixture(t)
	h := betHTTP(f.managementHTTP)
	unknown := betRequest(h, "POST", "/api/v1/bet-previews", f.userToken, "", `{"member_id":"`+f.memberID+`"}`)
	if unknown.Code != 400 {
		t.Fatalf("client identity field status=%d body=%s", unknown.Code, unknown.Body.String())
	}
	csrf := betRequest(h, "POST", "/api/v1/bet-previews", f.userToken, "http://attacker.example", `{}`)
	if csrf.Code != 403 {
		t.Fatalf("cross-origin preview status=%d body=%s", csrf.Code, csrf.Body.String())
	}
	badSession := betRequest(h, "POST", "/api/v1/bet-previews", "invalid-session-token-value-00000000000000000000", "", `{}`)
	if badSession.Code != 401 {
		t.Fatalf("invalid session status=%d body=%s", badSession.Code, badSession.Body.String())
	}
	crossBrand := betRequest(h, "GET", "/api/v1/b/harbor/bet-orders", f.userToken, "", "")
	if crossBrand.Code != 401 {
		t.Fatalf("cross-brand session status=%d body=%s", crossBrand.Code, crossBrand.Body.String())
	}
	badID := betRequest(h, "GET", "/api/v1/bet-orders/not-a-uuid", f.userToken, "", "")
	if badID.Code != 400 {
		t.Fatalf("invalid order ID status=%d body=%s", badID.Code, badID.Body.String())
	}
	if e := json.Unmarshal(badID.Body.Bytes(), &struct {
		Success bool `json:"success"`
	}{}); e != nil {
		t.Fatal(e)
	}
}

func TestBettingHTTPUserOrderQueriesAreMemberScoped(t *testing.T) {
	f := pointsFixture(t)
	h := betHTTP(f.managementHTTP)
	list := betRequest(h, "GET", "/api/v1/bet-orders?limit=1", f.userToken, "", "")
	if list.Code != 200 {
		t.Fatalf("member order list status=%d body=%s", list.Code, list.Body.String())
	}
	var got struct {
		Data struct {
			Items []json.RawMessage `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Data.Items) != 0 {
		t.Fatalf("new member unexpectedly has orders: %s", got.Data.Items[0])
	}
	badPage := betRequest(h, "GET", "/api/v1/bet-orders?limit=101", f.userToken, "", "")
	if badPage.Code != 400 {
		t.Fatalf("invalid pagination status=%d body=%s", badPage.Code, badPage.Body.String())
	}
}

func TestBettingHTTPFreshSessionRequiredForWrites(t *testing.T) {
	f := pointsFixture(t)
	h := betHTTP(f.managementHTTP)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE sessions SET revoked_at=clock_timestamp() WHERE token_hash=(SELECT token_hash FROM sessions WHERE member_id=$1 LIMIT 1)`, f.memberID); err != nil {
		t.Fatal(err)
	}
	// Authentication happens before mutation execution, so a revoked token cannot
	// reserve an idempotency key or reach the transactional betting service.
	r := httptest.NewRequest("POST", "http://localhost/api/v1/bet-orders", bytes.NewBufferString(`{}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+f.userToken)
	r.Header.Set("Idempotency-Key", "bet-order-revoked-001")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("revoked user write status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestBettingHTTPPlaceReplayCancelRefundAndMemberIsolation(t *testing.T) {
	f := pointsFixture(t)
	h := betHTTP(f.managementHTTP)
	ctx := context.Background()

	// Provision a second member so order visibility and cancellation are checked
	// against a real, separately authenticated brand session.
	created := f.call("POST", "/api/v1/admin/users", "bet-user-second-create", f.token, managedBrand,
		map[string]string{"username": "bet_second_member", "password": "bet-second-password-2026", "display_name": "Second member", "reason": "bet isolation fixture"})
	mustStatus(t, created, 201)
	var secondMember struct {
		MemberID string `json:"member_id"`
	}
	managedData(t, created, &secondMember)
	login := f.call("POST", "/api/v1/auth/login", "bet-user-second-login", "", managedBrand,
		map[string]string{"identifier": "bet_second_member", "password": "bet-second-password-2026", "privacy_policy_version": "dev-1", "service_terms_version": "dev-1"})
	mustStatus(t, login, 200)
	var secondAuth identity.Authentication
	managedData(t, login, &secondAuth)

	// Create and approve a real rule through the administrative HTTP workflow.
	gameFixture := f
	for _, permission := range []string{"game.view.brand", "game.write.brand", "rule.view.brand", "rule.write.brand", "rule.validate.brand", "rule.submit.brand", "rule.review.brand"} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,$2 FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root, permission); err != nil {
			t.Fatal(err)
		}
	}
	definition := ruleInput()["definition"].(map[string]any)
	gameResponse := f.call("POST", "/api/v1/admin/games", "bet-game-create-001", f.token, managedBrand,
		map[string]any{"code": "bet_http_game", "name": "Bet HTTP game", "model": definition["model"], "timezone": "UTC", "reason": "bet fixture game"})
	mustStatus(t, gameResponse, 201)
	var game rulebook.Game
	managedData(t, gameResponse, &game)
	playResponse := gameFixture.call("POST", "/api/v1/admin/games/"+game.ID+"/plays", "bet-play-create-001", f.token, managedBrand,
		map[string]string{"code": "special", "name": "Special number", "reason": "bet fixture"})
	mustStatus(t, playResponse, 201)
	var play struct {
		ID string `json:"id"`
	}
	managedData(t, playResponse, &play)
	draft := gameFixture.call("POST", "/api/v1/admin/rule-versions", "bet-rule-draft-001", f.token, managedBrand,
		map[string]any{"play_id": play.ID, "definition": definition, "effect_mode": "immediate", "reason": "bet fixture rule"})
	mustStatus(t, draft, 201)
	var version rulebook.Version
	managedData(t, draft, &version)
	cases := []any{map[string]any{"name": "one matching special", "selection": ruleInput()["selection"], "draw": ruleInput()["draw"], "multiplier": "2", "expected_bet_points": "8", "expected_prize_points": "70", "expected_won": true}}
	validated := gameFixture.call("POST", "/api/v1/admin/rule-versions/"+version.ID+"/validate", "bet-rule-validate-001", f.token, managedBrand,
		map[string]any{"version": version.Version, "cases": cases, "reason": "bet fixture validation"})
	mustStatus(t, validated, 200)
	managedData(t, validated, &version)
	submitted := gameFixture.call("POST", "/api/v1/admin/rule-versions/"+version.ID+"/submit-review", "bet-rule-submit-001", f.token, managedBrand,
		map[string]any{"version": version.Version, "reason": "bet fixture review"})
	mustStatus(t, submitted, 200)
	managedData(t, submitted, &version)
	var reviewerRole string
	if err := f.pool.QueryRow(ctx, `SELECT role_id::text FROM admin_account_roles WHERE account_id=$1`, f.root).Scan(&reviewerRole); err != nil {
		t.Fatal(err)
	}
	reviewerID := ids.New()
	if _, err := f.pool.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) SELECT $1,'bet_reviewer',password_hash FROM admin_accounts WHERE id=$2`, reviewerID, f.root); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO admin_brand_scopes(account_id,brand_id) VALUES($1,$2)`, reviewerID, managedBrand); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, reviewerID, reviewerRole); err != nil {
		t.Fatal(err)
	}
	reviewerLogin := f.call("POST", "/api/v1/admin/auth/login", "bet-reviewer-login", "", managedBrand,
		map[string]string{"identifier": "bet_reviewer", "password": "root-test-password-2026"})
	mustStatus(t, reviewerLogin, 200)
	var reviewer identity.AdminAuthentication
	managedData(t, reviewerLogin, &reviewer)
	approved := gameFixture.call("POST", "/api/v1/admin/rule-versions/"+version.ID+"/approve", "bet-rule-approve-001", reviewer.AccessToken, managedBrand,
		map[string]any{"version": version.Version, "reason": "bet fixture approved", "warnings_acknowledged": true})
	mustStatus(t, approved, 200)
	managedData(t, approved, &version)
	if version.Status != "active" {
		t.Fatalf("rule is not active: %+v", version)
	}

	// Open a period and bind the approved version exactly as the betting service
	// expects an established period snapshot to look.
	periodID := ids.New()
	now := time.Now().UTC()
	if _, err := f.pool.Exec(ctx, `INSERT INTO periods(id,brand_id,game_id,period_no,sequence,bet_start_at,bet_end_at,draw_at,status,version) VALUES($1,$2,$3,'bet-http-001',1,$4,$5,$6,'pending',1)`, periodID, managedBrand, game.ID, now.Add(-time.Minute), now.Add(time.Hour), now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE periods SET status='betting',version=version+1,state_reason='open bet integration fixture' WHERE id=$1`, periodID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO period_rule_versions(brand_id,game_id,period_id,play_id,rule_version_id) VALUES($1,$2,$3,$4,$5)`, managedBrand, game.ID, periodID, play.ID, version.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE brand_bet_policies SET config=jsonb_set(config,'{user_cancel_allowed}','true'::jsonb) WHERE brand_id=$1`, managedBrand); err != nil {
		t.Fatal(err)
	}

	// Fund by posting through the production point ledger inside a transaction.
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	allocation := []points.Allocation{{Source: "gift", State: "available", Points: 500}}
	delta, err := points.AllocationDelta(allocation, "", "available")
	if err != nil {
		t.Fatal(err)
	}
	_, err = (points.Store{DB: f.pool}).Post(ctx, tx, points.Change{BrandID: managedBrand, MemberID: f.memberID, EntryType: "adjustment", ReferenceType: "bet_fixture", OperationKey: "bet-fixture-fund-001", Reason: "fund HTTP betting fixture", ActorType: "admin", ActorID: f.root, RequestID: "bet-fixture-fund", Delta: delta, Allocation: allocation})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	in := map[string]any{"period_id": periodID, "play_id": play.ID, "rule_version_id": version.ID,
		"selection": ruleInput()["selection"], "multiplier": "2", "policy_versions": map[string]int64{"brand": 1, "game": 1}}
	preview := betRequest(h, "POST", "/api/v1/bet-previews", f.userToken, "", mustJSON(t, in))
	mustStatus(t, preview, 200)
	var quote struct {
		BetPoints points.Amount `json:"bet_points"`
	}
	var previewEnvelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(preview.Body.Bytes(), &previewEnvelope); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(previewEnvelope.Data, &quote); err != nil {
		t.Fatal(err)
	}
	if quote.BetPoints != 8 {
		t.Fatalf("preview stake=%d, want 8", quote.BetPoints)
	}
	var accountContext struct {
		ActorContext string `json:"actor_context"`
	}
	decodeBetData(t, preview, &accountContext)
	if len(accountContext.ActorContext) != 64 {
		t.Fatal("missing signed confirmation context")
	}
	in["actor_context"] = accountContext.ActorContext
	// A different valid member cannot spend from a confirmation issued to this
	// user, even when cookies changed after the UI's last /me preflight.
	swapped := betRequestWithKey(h, "POST", "/api/v1/bet-orders", secondAuth.AccessToken, "", "bet-swapped-account", mustJSON(t, in))
	mustStatus(t, swapped, 403)

	body := mustJSON(t, in)
	place := betRequestWithKey(h, "POST", "/api/v1/bet-orders", f.userToken, "", "bet-order-place-001", body)
	mustStatus(t, place, 201)
	var order struct {
		ID     string        `json:"id"`
		Status string        `json:"status"`
		Total  points.Amount `json:"total_points"`
	}
	decodeBetData(t, place, &order)
	if order.ID == "" || order.Status != "placed" || order.Total != 8 {
		t.Fatalf("unexpected placed order: %+v", order)
	}
	repeat := betRequestWithKey(h, "POST", "/api/v1/bet-orders", f.userToken, "", "bet-order-place-001", body)
	mustStatus(t, repeat, 201)
	var repeated struct {
		ID string `json:"id"`
	}
	decodeBetData(t, repeat, &repeated)
	if repeated.ID != order.ID {
		t.Fatal("same-key placement returned a different order")
	}
	changedInput := map[string]any{"period_id": periodID, "play_id": play.ID, "rule_version_id": version.ID,
		"selection": map[string]any{"special": []int{7, 19, 31}}, "multiplier": "2", "policy_versions": map[string]int64{"brand": 1, "game": 1}, "actor_context": accountContext.ActorContext}
	changed := betRequestWithKey(h, "POST", "/api/v1/bet-orders", f.userToken, "", "bet-order-place-001", mustJSON(t, changedInput))
	mustStatus(t, changed, 409)

	if got := betRequest(h, "GET", "/api/v1/bet-orders/"+order.ID, secondAuth.AccessToken, "", ""); got.Code != 404 {
		t.Fatalf("other member order read=%d %s", got.Code, got.Body.String())
	}
	cancelBody := mustJSON(t, map[string]any{"version": 1, "reason": "user changed their mind"})
	otherCancel := betRequestWithKey(h, "POST", "/api/v1/bet-orders/"+order.ID+"/cancel", secondAuth.AccessToken, "", "bet-cancel-other-001", cancelBody)
	mustStatus(t, otherCancel, 404)
	cancel := betRequestWithKey(h, "POST", "/api/v1/bet-orders/"+order.ID+"/cancel", f.userToken, "", "bet-cancel-own-001", cancelBody)
	mustStatus(t, cancel, 200)
	var cancelled struct {
		Status      string `json:"status"`
		RefundEntry string `json:"refund_entry_id"`
	}
	decodeBetData(t, cancel, &cancelled)
	if cancelled.Status != "bet_cancelled" || cancelled.RefundEntry == "" {
		t.Fatalf("unexpected cancellation: %+v", cancelled)
	}
	cancelRepeat := betRequestWithKey(h, "POST", "/api/v1/bet-orders/"+order.ID+"/cancel", f.userToken, "", "bet-cancel-own-001", cancelBody)
	mustStatus(t, cancelRepeat, 200)
	var wallet struct {
		Display points.Amount `json:"display_points"`
		Gift    points.Amount `json:"gift_points"`
	}
	walletResult, err := (points.Store{DB: f.pool}).Read(ctx, managedBrand, f.memberID)
	if err != nil {
		t.Fatal(err)
	}
	wallet.Display, wallet.Gift = walletResult.DisplayPoints, walletResult.GiftPoints
	if wallet.Display != 500 || wallet.Gift != 500 {
		t.Fatalf("cancel failed to restore wallet: %+v", wallet)
	}
}

func TestBettingHTTPAdminPolicyPermissionsAndSuperReadonly(t *testing.T) {
	f := managedFixture(t)
	for _, permission := range []string{"bet_policy.view.brand", "bet_policy.write.brand"} {
		if _, err := f.pool.Exec(context.Background(), `INSERT INTO role_permissions(role_id,permission_key) SELECT role_id,$2 FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root, permission); err != nil {
			t.Fatal(err)
		}
	}
	read := f.call("GET", "/api/v1/admin/bet-policy", "", f.token, managedBrand, nil)
	mustStatus(t, read, 200)
	var policy betting.BrandPolicyRecord
	managedData(t, read, &policy)
	if policy.BrandID != managedBrand || policy.Version != 1 {
		t.Fatalf("unexpected initial policy: %+v", policy)
	}
	policy.Config.MinBetPoints = 2
	write := f.call("PUT", "/api/v1/admin/bet-policy", "bet-policy-write-001", f.token, managedBrand,
		map[string]any{"version": policy.Version, "config": policy.Config, "reason": "raise minimum stake"})
	mustStatus(t, write, 200)
	var updated betting.BrandPolicyRecord
	managedData(t, write, &updated)
	if updated.Version != 2 || updated.Config.MinBetPoints != 2 {
		t.Fatalf("policy update mismatch: %+v", updated)
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	denied := f.call("PUT", "/api/v1/admin/bet-policy", "bet-policy-super-denied", f.token, managedBrand,
		map[string]any{"version": updated.Version, "config": updated.Config, "reason": "must remain readonly"})
	mustStatus(t, denied, 403)
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
func decodeBetData(t *testing.T, response *httptest.ResponseRecorder, destination any) {
	t.Helper()
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(envelope.Data, destination); err != nil {
		t.Fatal(response.Code, response.Body.String(), err)
	}
}
func betRequestWithKey(h http.Handler, method, path, token, origin, key, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://localhost"+path, bytes.NewBufferString(body))
	r.RemoteAddr = "192.0.2.73:23456"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", key)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
