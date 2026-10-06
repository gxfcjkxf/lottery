package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/attribution"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
)

func grantJoinCodeRoot(t *testing.T, f managementHTTP) string {
	t.Helper()
	ctx := context.Background()
	var role string
	if err := f.pool.QueryRow(ctx, `SELECT role_id::text FROM admin_account_roles WHERE account_id=$1 LIMIT 1`, f.root).Scan(&role); err != nil {
		t.Fatal(err)
	}
	for _, permission := range []string{"join_code.view.brand", "join_code.write.brand"} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, permission); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2) ON CONFLICT DO NOTHING`, role, permission); err != nil {
			t.Fatal(err)
		}
	}
	return role
}

func joinCodeCreateBody(owner string) map[string]any {
	return map[string]any{"kind": "referral", "owner_member_id": owner, "agent_id": nil, "starts_at": nil, "expires_at": nil, "reason": "join-code HTTP test"}
}

func TestJoinCodeAdminLifecyclePublicPrivacyAndNoFunds(t *testing.T) {
	f := pointsFixture(t)
	grantJoinCodeRoot(t, f.managementHTTP)
	ctx := context.Background()
	var accountsBefore, ledgerBefore int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM point_accounts`).Scan(&accountsBefore); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries`).Scan(&ledgerBefore); err != nil {
		t.Fatal(err)
	}
	body := joinCodeCreateBody(f.memberID)
	created := f.call("POST", "/api/v1/admin/join-codes", "join-create-replay-001", f.token, managedBrand, body)
	if created.Code != 201 {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var code attribution.Code
	managedData(t, created, &code)
	if len(code.Code) != 24 || code.Kind != "referral" || code.OwnerMemberID != f.memberID || code.Status != "active" || code.Version != 1 || code.AuditLogID == "" {
		t.Fatalf("unexpected created code: %+v", code)
	}
	replayed := f.call("POST", "/api/v1/admin/join-codes", "join-create-replay-001", f.token, managedBrand, body)
	var replay attribution.Code
	if replayed.Code != 201 {
		t.Fatalf("create replay status=%d body=%s", replayed.Code, replayed.Body.String())
	}
	managedData(t, replayed, &replay)
	if replay.ID != code.ID || replay.Code != code.Code {
		t.Fatal("create replay did not return the original code")
	}
	changed := joinCodeCreateBody(f.memberID)
	changed["reason"] = "different intent"
	if conflict := f.call("POST", "/api/v1/admin/join-codes", "join-create-replay-001", f.token, managedBrand, changed); conflict.Code != 409 {
		t.Fatalf("same-key changed request status=%d body=%s", conflict.Code, conflict.Body.String())
	}

	got := f.call("GET", "/api/v1/admin/join-codes/"+code.ID, "", f.token, managedBrand, nil)
	if got.Code != 200 {
		t.Fatalf("get status=%d body=%s", got.Code, got.Body.String())
	}
	list := f.call("GET", "/api/v1/admin/join-codes?kind=referral&owner_member_id="+f.memberID+"&limit=20", "", f.token, managedBrand, nil)
	if list.Code != 200 {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	var page attribution.Page
	managedData(t, list, &page)
	if len(page.Items) != 1 || page.Items[0].ID != code.ID || page.Kind == nil || *page.Kind != "referral" || page.OwnerMemberID == nil || *page.OwnerMemberID != f.memberID || page.TotalCount != "1" {
		t.Fatalf("unexpected list response: %+v", page)
	}
	for _, query := range []string{"?unknown=1", "?kind=referral&kind=agent", "?limit=0", "?owner_member_id=bad"} {
		if response := f.call("GET", "/api/v1/admin/join-codes"+query, "", f.token, managedBrand, nil); response.Code != 400 {
			t.Fatalf("invalid query %s status=%d body=%s", query, response.Code, response.Body.String())
		}
	}
	history := f.call("GET", "/api/v1/admin/join-codes/"+code.ID+"/history", "", f.token, managedBrand, nil)
	if history.Code != 200 {
		t.Fatalf("history status=%d body=%s", history.Code, history.Body.String())
	}
	var revisions attribution.History
	managedData(t, history, &revisions)
	if len(revisions.Items) != 1 || revisions.Items[0].Reason != "join-code HTTP test" || revisions.Items[0].ActorID != f.root {
		t.Fatalf("unexpected initial history: %+v", revisions)
	}
	update := map[string]any{"version": code.Version, "status": "disabled", "starts_at": nil, "expires_at": nil, "reason": "disable test code"}
	updated := f.call("PUT", "/api/v1/admin/join-codes/"+code.ID, "join-update-replay-001", f.token, managedBrand, update)
	if updated.Code != 200 {
		t.Fatalf("update status=%d body=%s", updated.Code, updated.Body.String())
	}
	var disabled attribution.Code
	managedData(t, updated, &disabled)
	if disabled.Status != "disabled" || disabled.Version != code.Version+1 || disabled.AuditLogID == "" {
		t.Fatalf("unexpected update response: %+v", disabled)
	}
	if response := f.call("PUT", "/api/v1/admin/join-codes/"+code.ID, "join-update-replay-001", f.token, managedBrand, update); response.Code != 200 {
		t.Fatalf("update replay status=%d body=%s", response.Code, response.Body.String())
	}

	publicCodes := pointsCall(f.managementHTTP, "GET", "/api/v1/me/join-codes", "", f.userToken, managedBrand, "", nil)
	if publicCodes.Code != 200 {
		t.Fatalf("self codes status=%d body=%s", publicCodes.Code, publicCodes.Body.String())
	}
	var self attribution.SelfPage
	managedData(t, publicCodes, &self)
	if self.MemberID != f.memberID || len(self.Items) != 1 || self.Items[0].ID != code.ID {
		t.Fatalf("unexpected self-code response: %+v", self)
	}
	var publicEnvelope map[string]any
	if err := json.Unmarshal(publicCodes.Body.Bytes(), &publicEnvelope); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(publicEnvelope["data"])
	for _, private := range []string{"reason", "actor_id", "audit_log_id", "agent_id"} {
		var data map[string]any
		if err := json.Unmarshal(encoded, &data); err != nil {
			t.Fatal(err)
		}
		if _, exists := data[private]; exists {
			t.Fatalf("public self-code response exposed %q: %s", private, encoded)
		}
	}
	attributionResponse := pointsCall(f.managementHTTP, "GET", "/api/v1/me/attribution", "", f.userToken, managedBrand, "", nil)
	if attributionResponse.Code != 200 {
		t.Fatalf("public attribution status=%d body=%s", attributionResponse.Code, attributionResponse.Body.String())
	}
	var own attribution.PublicAttribution
	managedData(t, attributionResponse, &own)
	if own.MemberID != f.memberID || own.JoinMethod != "operator" || own.CodeID != nil || own.SourceCode != nil {
		t.Fatalf("unexpected public attribution response: %+v", own)
	}

	foreignOwner := newNormalAgentMemberInBrand(t, f.managementHTTP, pointsBrandB, "foreign_join_owner")
	foreign := f.call("POST", "/api/v1/admin/join-codes", "join-foreign-owner-001", f.token, managedBrand, joinCodeCreateBody(foreignOwner))
	if foreign.Code != 404 {
		t.Fatalf("cross-brand owner status=%d body=%s", foreign.Code, foreign.Body.String())
	}
	var accountsAfter, ledgerAfter int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM point_accounts`).Scan(&accountsAfter); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM point_ledger_entries`).Scan(&ledgerAfter); err != nil {
		t.Fatal(err)
	}
	if accountsAfter != accountsBefore || ledgerAfter != ledgerBefore {
		t.Fatalf("join-code operations touched points: accounts %d->%d ledger %d->%d", accountsBefore, accountsAfter, ledgerBefore, ledgerAfter)
	}
}

func TestJoinCodeFreshPermissionAuditRollbackAndPlatformReadOnly(t *testing.T) {
	f := managedFixture(t)
	role := grantJoinCodeRoot(t, f)
	member := newNormalAgentMember(t, f, "join_permission_owner")
	body := joinCodeCreateBody(member)
	created := f.call("POST", "/api/v1/admin/join-codes", "join-revocation-replay-001", f.token, managedBrand, body)
	if created.Code != 201 {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='join_code.write.brand'`, role); err != nil {
		t.Fatal(err)
	}
	if revoked := f.call("POST", "/api/v1/admin/join-codes", "join-revocation-replay-001", f.token, managedBrand, body); revoked.Code != 403 {
		t.Fatalf("cached response after grant revocation status=%d body=%s", revoked.Code, revoked.Body.String())
	}
	if missing := f.call("POST", "/api/v1/admin/join-codes", "join-no-grant-001", f.token, managedBrand, joinCodeCreateBody(newNormalAgentMember(t, f, "join_no_grant"))); missing.Code != 403 {
		t.Fatalf("missing write grant status=%d body=%s", missing.Code, missing.Body.String())
	}
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,'join_code.write.brand')`, role); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `CREATE FUNCTION fail_join_code_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action LIKE 'join_code.%' THEN RAISE EXCEPTION 'test join-code audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_join_code_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION fail_join_code_audit()`); err != nil {
		t.Fatal(err)
	}
	rollbackBody := joinCodeCreateBody(newNormalAgentMember(t, f, "join_audit_failure"))
	if failed := f.call("POST", "/api/v1/admin/join-codes", "join-audit-failure-001", f.token, managedBrand, rollbackBody); failed.Code != 503 {
		t.Fatalf("audit failure status=%d body=%s", failed.Code, failed.Body.String())
	}
	var count int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM join_codes WHERE owner_member_id=$1`, rollbackBody["owner_member_id"]).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("code survived failed audit transaction")
	}

	if _, err := f.pool.Exec(context.Background(), `DROP TRIGGER fail_join_code_audit ON audit_logs; DROP FUNCTION fail_join_code_audit()`); err != nil {
		t.Fatal(err)
	}
	platformRole := ids.New()
	for _, permission := range []string{"join_code.view.platform", "join_code.write.brand"} {
		if _, err := f.pool.Exec(context.Background(), `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, permission); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO roles(id,brand_id,code,name) VALUES($1,NULL,$2,'Join code platform test')`, platformRole, "join_platform_"+platformRole[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,'join_code.view.platform'),($1,'join_code.write.brand')`, platformRole); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, f.root, platformRole); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE admin_accounts SET is_super_admin=true WHERE id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	if view := f.call("GET", "/api/v1/admin/join-codes", "", f.token, managedBrand, nil); view.Code != 200 {
		t.Fatalf("platform reader with explicit grant status=%d body=%s", view.Code, view.Body.String())
	}
	if write := f.call("POST", "/api/v1/admin/join-codes", "join-super-write-001", f.token, managedBrand, joinCodeCreateBody(member)); write.Code != 403 {
		t.Fatalf("super administrator write status=%d body=%s", write.Code, write.Body.String())
	}
}

func TestJoinCodeAdminReadsAreAuditedAndForeignIDsAreHidden(t *testing.T) {
	f := managedFixture(t)
	grantJoinCodeRoot(t, f)
	member := newNormalAgentMember(t, f, "join_read_owner")
	created := f.call("POST", "/api/v1/admin/join-codes", "join-read-create-001", f.token, managedBrand, joinCodeCreateBody(member))
	if created.Code != 201 {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var code attribution.Code
	managedData(t, created, &code)
	for _, action := range []string{"join_code.list.view", "join_code.view", "join_code.history.view"} {
		var count int
		if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE actor_id=$1 AND action=$2`, f.root, action).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("audit action %s unexpectedly pre-existed", action)
		}
	}
	requests := []struct{ path, action string }{
		{"/api/v1/admin/join-codes", "join_code.list.view"},
		{"/api/v1/admin/join-codes/" + code.ID, "join_code.view"},
		{"/api/v1/admin/join-codes/" + code.ID + "/history", "join_code.history.view"},
	}
	for _, request := range requests {
		response := f.call("GET", request.path, "", f.token, managedBrand, nil)
		if response.Code != 200 {
			t.Fatalf("read %s status=%d body=%s", request.path, response.Code, response.Body.String())
		}
		var count int
		if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE actor_id=$1 AND action=$2`, f.root, request.action).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("read %s wrote %d %s audit records", request.path, count, request.action)
		}
	}
	foreignBrandID := pointsBrandB
	foreignOwner := newNormalAgentMemberInBrand(t, f, foreignBrandID, "join_foreign_code")
	foreignTx, err := f.pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	foreignActor := access.Account{ID: f.root, Type: access.AccountAdmin, BrandIDs: []string{foreignBrandID}, Roles: []access.Role{{BrandID: foreignBrandID, Permissions: []access.Permission{{Resource: "join_code", Action: "write", Scope: access.ScopeBrand}}}}}
	foreignCode, err := (attribution.Service{DB: f.pool}).Create(context.Background(), foreignTx, foreignBrandID, foreignActor, attribution.CreateInput{Kind: "referral", OwnerMemberID: foreignOwner, Reason: "foreign brand fixture"}, points.Metadata{ActorType: "admin", ActorID: f.root, RequestID: "join-foreign-fixture"})
	if err != nil {
		_ = foreignTx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err := foreignTx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if response := f.call("GET", "/api/v1/admin/join-codes/"+foreignCode.ID, "", f.token, managedBrand, nil); response.Code != 404 {
		t.Fatalf("cross-brand code read status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAuthHTTPJoinCodeRegistrationReplayAndIsolation(t *testing.T) {
	f := pointsFixture(t)
	grantJoinCodeRoot(t, f.managementHTTP)
	ctx := context.Background()

	created := f.call("POST", "/api/v1/admin/join-codes", "auth-join-code-create-001", f.token, managedBrand, joinCodeCreateBody(f.memberID))
	if created.Code != 201 {
		t.Fatalf("create referral code status=%d body=%s", created.Code, created.Body.String())
	}
	var code attribution.Code
	managedData(t, created, &code)
	privacyVersion, termsVersion := currentAuthTerms(t, f.managementHTTP, "/api/v1/context")

	registerBody := map[string]any{
		"username": "join_auth_success", "password": "join-code-auth-password-2026",
		"privacy_policy_version": privacyVersion, "service_terms_version": termsVersion,
		"referral_code": lowerJoinCode(code.Code),
	}
	registered := f.call("POST", "/api/v1/auth/register", "auth-join-code-register-001", "", managedBrand, registerBody)
	if registered.Code != 201 {
		t.Fatalf("code registration status=%d body=%s", registered.Code, registered.Body.String())
	}
	var auth identity.Authentication
	managedData(t, registered, &auth)
	if auth.User.Username != "join_auth_success" || auth.Member.BrandID != managedBrand || auth.AccessToken == "" {
		t.Fatalf("unexpected registration response: %+v", auth)
	}
	attributionResponse := pointsCall(f.managementHTTP, "GET", "/api/v1/me/attribution", "", auth.AccessToken, managedBrand, "", nil)
	if attributionResponse.Code != 200 {
		t.Fatalf("registered member attribution status=%d body=%s", attributionResponse.Code, attributionResponse.Body.String())
	}
	var attributed attribution.PublicAttribution
	managedData(t, attributionResponse, &attributed)
	if attributed.BrandID != managedBrand || attributed.MemberID != auth.Member.ID || attributed.JoinMethod != "referral_code" || attributed.CodeID == nil || *attributed.CodeID != code.ID || attributed.SourceCode == nil || *attributed.SourceCode != code.Code || attributed.Legacy {
		t.Fatalf("unexpected public attribution: %+v; code=%+v", attributed, code)
	}
	var publicEnvelope struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(attributionResponse.Body.Bytes(), &publicEnvelope); err != nil {
		t.Fatal(err)
	}
	serialized, _ := json.Marshal(publicEnvelope.Data)
	if len(publicEnvelope.Data) != 7 {
		t.Fatalf("public attribution DTO has unexpected fields: %s", serialized)
	}
	for _, privateField := range []string{"owner_member_id", "agent_id", "reason", "actor_id", "audit_log_id"} {
		if _, exists := publicEnvelope.Data[privateField]; exists {
			t.Fatalf("public attribution exposed %q: %s", privateField, serialized)
		}
	}

	// A code is brand-local. Resolve the second brand through the platform path
	// and prove its registration cannot consume this brand's code.
	var otherBrandCode string
	if err := f.pool.QueryRow(ctx, `SELECT code FROM brands WHERE id=$1`, pointsBrandB).Scan(&otherBrandCode); err != nil {
		t.Fatal(err)
	}
	foreignPrivacyVersion, foreignTermsVersion := currentAuthTerms(t, f.managementHTTP, "/api/v1/b/"+otherBrandCode+"/context")
	foreignBody := map[string]any{
		"username": "join_auth_crossbrand", "password": "join-code-auth-password-2026",
		"privacy_policy_version": foreignPrivacyVersion, "service_terms_version": foreignTermsVersion,
		"referral_code": code.Code,
	}
	foreign := f.call("POST", "/api/v1/b/"+otherBrandCode+"/auth/register", "auth-join-code-crossbrand-001", "", pointsBrandB, foreignBody)
	if foreign.Code != 400 || errorCode(foreign) != "JOIN_CODE_UNAVAILABLE" {
		t.Fatalf("cross-brand registration status=%d body=%s", foreign.Code, foreign.Body.String())
	}
	assertNoRegistrationResidue(t, f.managementHTTP, "join_auth_crossbrand")

	// Disable the source after one committed registration. A new registration
	// must fail atomically, while replay of the committed operation returns its
	// sealed original receipt without re-evaluating the now-disabled code.
	update := map[string]any{"version": code.Version, "status": "disabled", "starts_at": nil, "expires_at": nil, "reason": "disable source for auth integration"}
	disabled := f.call("PUT", "/api/v1/admin/join-codes/"+code.ID, "auth-join-code-disable-001", f.token, managedBrand, update)
	if disabled.Code != 200 {
		t.Fatalf("disable source status=%d body=%s", disabled.Code, disabled.Body.String())
	}
	failedBody := map[string]any{
		"username": "join_auth_disabled", "password": "join-code-auth-password-2026",
		"privacy_policy_version": privacyVersion, "service_terms_version": termsVersion,
		"referral_code": code.Code,
	}
	countsBefore := registrationResidueCounts(t, f.managementHTTP, "join_auth_disabled")
	failed := f.call("POST", "/api/v1/auth/register", "auth-join-code-disabled-001", "", managedBrand, failedBody)
	if failed.Code != 400 || errorCode(failed) != "JOIN_CODE_UNAVAILABLE" {
		t.Fatalf("disabled source registration status=%d body=%s", failed.Code, failed.Body.String())
	}
	assertRegistrationResidueCounts(t, f.managementHTTP, "join_auth_disabled", countsBefore)

	replay := f.call("POST", "/api/v1/auth/register", "auth-join-code-register-001", "", managedBrand, registerBody)
	if replay.Code != 201 {
		t.Fatalf("committed registration replay after disable status=%d body=%s", replay.Code, replay.Body.String())
	}
	var replayed identity.Authentication
	managedData(t, replay, &replayed)
	if replayed.User.ID != auth.User.ID || replayed.Member.ID != auth.Member.ID || replayed.AccessToken != auth.AccessToken {
		t.Fatal("registration replay after source disable did not return its original receipt")
	}

	loginWithCode := map[string]any{"identifier": "join_auth_success", "password": "join-code-auth-password-2026", "referral_code": lowerJoinCode(code.Code)}
	fixed := f.call("POST", "/api/v1/auth/login", "auth-join-code-fixed-001", "", managedBrand, loginWithCode)
	if fixed.Code != 409 || errorCode(fixed) != "JOIN_ATTRIBUTION_FIXED" {
		t.Fatalf("existing member with code status=%d body=%s", fixed.Code, fixed.Body.String())
	}
	loginWithoutCode := map[string]any{"identifier": "join_auth_success", "password": "join-code-auth-password-2026"}
	loggedIn := f.call("POST", "/api/v1/auth/login", "auth-join-code-normal-login-001", "", managedBrand, loginWithoutCode)
	if loggedIn.Code != 200 {
		t.Fatalf("existing member without code status=%d body=%s", loggedIn.Code, loggedIn.Body.String())
	}

	duplicateRaw := `{"username":"join_auth_duplicate","password":"join-code-auth-password-2026","privacy_policy_version":"dev-1","service_terms_version":"dev-1","referral_code":"` + code.Code + `","referral_code":"` + code.Code + `"}`
	duplicate := f.rawCall("POST", "/api/v1/auth/register", "auth-join-code-duplicate-001", "", managedBrand, duplicateRaw)
	if duplicate.Code != 400 {
		t.Fatalf("duplicate code selector status=%d body=%s", duplicate.Code, duplicate.Body.String())
	}
	assertNoRegistrationResidue(t, f.managementHTTP, "join_auth_duplicate")
}

func currentAuthTerms(t *testing.T, f managementHTTP, path string) (string, string) {
	t.Helper()
	response := f.call("GET", path, "", "", "", nil)
	if response.Code != 200 {
		t.Fatalf("read current auth terms at %s status=%d body=%s", path, response.Code, response.Body.String())
	}
	var envelope struct {
		Data struct {
			Terms struct {
				Privacy string `json:"privacy_policy_version"`
				Service string `json:"service_terms_version"`
			} `json:"terms"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Terms.Privacy == "" || envelope.Data.Terms.Service == "" {
		t.Fatalf("missing current auth terms at %s: %s", path, response.Body.String())
	}
	return envelope.Data.Terms.Privacy, envelope.Data.Terms.Service
}

func lowerJoinCode(code string) string {
	return strings.ToLower(code)
}

type registrationCounts struct {
	users, members, accounts, ledger int
}

func registrationResidueCounts(t *testing.T, f managementHTTP, username string) registrationCounts {
	t.Helper()
	var counts registrationCounts
	err := f.pool.QueryRow(context.Background(), `
		SELECT
		 (SELECT count(*) FROM global_users WHERE username=$1),
		 (SELECT count(*) FROM brand_members m JOIN global_users u ON u.id=m.global_user_id WHERE u.username=$1),
		 (SELECT count(*) FROM point_accounts a JOIN brand_members m ON m.id=a.brand_member_id JOIN global_users u ON u.id=m.global_user_id WHERE u.username=$1),
		 (SELECT count(*) FROM point_ledger_entries e JOIN point_accounts a ON a.id=e.account_id JOIN brand_members m ON m.id=a.brand_member_id JOIN global_users u ON u.id=m.global_user_id WHERE u.username=$1)`, username).
		Scan(&counts.users, &counts.members, &counts.accounts, &counts.ledger)
	if err != nil {
		t.Fatal(err)
	}
	return counts
}

func assertNoRegistrationResidue(t *testing.T, f managementHTTP, username string) {
	t.Helper()
	assertRegistrationResidueCounts(t, f, username, registrationCounts{})
}

func assertRegistrationResidueCounts(t *testing.T, f managementHTTP, username string, want registrationCounts) {
	t.Helper()
	if got := registrationResidueCounts(t, f, username); got != want {
		t.Fatalf("unexpected failed-registration residue for %s: got %+v, want %+v", username, got, want)
	}
}

func errorCode(r *httptest.ResponseRecorder) string {
	var envelope struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(r.Body.Bytes(), &envelope) != nil || envelope.Error == nil {
		return ""
	}
	return envelope.Error.Code
}
