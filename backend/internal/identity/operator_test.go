package identity

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/telegramauth"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const operatorTestBrand = "0199a000-0000-7000-8000-000000000001"
const operatorTestOtherBrand = "0199a000-0000-7000-8000-000000000002"

func TestTelegramClientIDRequiresCanonicalPositiveDecimal(t *testing.T) {
	for _, tc := range []struct {
		value   string
		enabled bool
		valid   bool
	}{
		{value: "123", enabled: true, valid: true},
		{value: "9007199254740991", enabled: true, valid: true},
		{value: "000123", enabled: true, valid: false},
		{value: "000123", enabled: false, valid: false},
		{value: "0", enabled: false, valid: false},
		{value: "", enabled: false, valid: true},
		{value: "", enabled: true, valid: false},
		{value: "9007199254740992", enabled: false, valid: false},
	} {
		if got := validTelegramClientID(tc.value, tc.enabled); got != tc.valid {
			t.Errorf("validTelegramClientID(%q, enabled=%v)=%v, want %v", tc.value, tc.enabled, got, tc.valid)
		}
	}
}

func operatorTestAdmin(t *testing.T, ctx context.Context, p *pgxpool.Pool) string {
	t.Helper()
	id := ids.New()
	if _, err := p.Exec(ctx, "INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,$2,'not-a-real-hash')", id, "operator_"+strings.ReplaceAll(id, "-", "")); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestOperatorCreatePendingConsentFlowAndGlobalUniqueness(t *testing.T) {
	p := testdb.New(t)
	s, err := New(p)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := mutation.New(p, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	adminID := operatorTestAdmin(t, ctx, p)
	meta := Metadata{RequestID: "operator-create-test", IP: "127.0.0.1"}
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	created, err := s.OperatorCreate(ctx, tx, operatorTestBrand, adminID, OperatorInput{
		Username: "Pending_User", Phone: "+12025550124", Password: "operator-created-password",
		DisplayName: "  Example Person  ", Notes: "  created from test  ", Reason: "  support request  ",
	}, meta)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if created.Status != 201 {
		_ = tx.Rollback(ctx)
		t.Fatalf("operator create: %+v", created)
	}
	var response operatorCreated
	if err := json.Unmarshal(created.Data, &response); err != nil {
		t.Fatal(err)
	}
	if response.UserID == "" || response.MemberID == "" || response.BrandID != operatorTestBrand || response.TermsAccepted {
		t.Fatalf("unexpected create response: %+v", response)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var termsAccepted bool
	var acceptedAt *string
	var joinMethod, createdBy, displayName, notes, privacy, terms string
	if err = p.QueryRow(ctx, "SELECT terms_accepted,accepted_at::text,join_method,created_by::text,display_name,notes,privacy_policy_version,service_terms_version FROM brand_members WHERE id=$1", response.MemberID).Scan(&termsAccepted, &acceptedAt, &joinMethod, &createdBy, &displayName, &notes, &privacy, &terms); err != nil {
		t.Fatal(err)
	}
	if termsAccepted || acceptedAt != nil || joinMethod != "operator" || createdBy != adminID || displayName != "Example Person" || notes != "created from test" || privacy == "" || terms == "" {
		t.Fatalf("bad pending member: accepted=%v accepted_at=%v method=%q creator=%q name=%q notes=%q policy=%q/%q", termsAccepted, acceptedAt, joinMethod, createdBy, displayName, notes, privacy, terms)
	}
	var bucketCount, sessionCount int
	if err = p.QueryRow(ctx, "SELECT count(*) FROM point_buckets pb JOIN point_accounts pa ON pa.id=pb.account_id WHERE pa.brand_member_id=$1", response.MemberID).Scan(&bucketCount); err != nil || bucketCount != 12 {
		t.Fatalf("buckets=%d err=%v", bucketCount, err)
	}
	if err = p.QueryRow(ctx, "SELECT count(*) FROM sessions WHERE user_id=$1", response.UserID).Scan(&sessionCount); err != nil || sessionCount != 0 {
		t.Fatalf("operator create session count=%d err=%v", sessionCount, err)
	}
	if _, err = p.Exec(ctx, "UPDATE brands SET auth_config=auth_config||'{\"telegram_enabled\":true,\"telegram_client_id\":\"12345\"}'::jsonb WHERE id=$1", operatorTestBrand); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Exec(ctx, "UPDATE global_users SET telegram_user_id='987654321' WHERE id=$1", response.UserID); err != nil {
		t.Fatal(err)
	}
	login := func(key string, in LoginInput) mutation.Result {
		t.Helper()
		result, err := engine.Execute(ctx, operatorTestBrand, "operator-flow", "login", key, engine.Fingerprint(key), func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			return s.Login(ctx, tx, operatorTestBrand, in, meta)
		})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	noConsent := login("operator-login-pending", LoginInput{Identifier: "pending_user", Password: "operator-created-password"})
	if noConsent.Status != 409 || noConsent.Error == nil || noConsent.Error.Code != "BRAND_JOIN_REQUIRED" {
		t.Fatalf("pending password login: %+v", noConsent)
	}
	tg, err := engine.Execute(ctx, operatorTestBrand, "operator-flow", "telegram", "operator-tg-pending", engine.Fingerprint("operator-tg-pending"), func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
		return s.Telegram(ctx, tx, operatorTestBrand, telegramauth.Claims{ID: "987654321", Subject: "test"}, TelegramInput{}, "", meta)
	})
	if err != nil || tg.Status != 409 || tg.Error == nil || tg.Error.Code != "BRAND_JOIN_REQUIRED" {
		t.Fatalf("pending Telegram login: %+v err=%v", tg, err)
	}
	accepted := login("operator-login-accept", LoginInput{Identifier: "pending_user", Password: "operator-created-password", Privacy: privacy, Terms: terms})
	if accepted.Status != 200 {
		t.Fatalf("consent login: %+v", accepted)
	}
	var auth Authentication
	if err := json.Unmarshal(accepted.Data, &auth); err != nil {
		t.Fatal(err)
	}
	if auth.AccessToken == "" || auth.User.ID != response.UserID || auth.Member.ID != response.MemberID {
		t.Fatalf("unexpected authentication: %+v", auth)
	}
	if _, err = s.Authenticate(ctx, operatorTestBrand, auth.AccessToken); err != nil {
		t.Fatalf("accepted session did not authenticate: %v", err)
	}
	var acceptedAtPresent bool
	if err = p.QueryRow(ctx, "SELECT terms_accepted,accepted_at IS NOT NULL FROM brand_members WHERE id=$1", response.MemberID).Scan(&termsAccepted, &acceptedAtPresent); err != nil {
		t.Fatal(err)
	}
	if !termsAccepted || !acceptedAtPresent {
		t.Fatal("consent was not persisted")
	}
	var acceptAuditCount, createAuditCount int
	if err = p.QueryRow(ctx, "SELECT count(*) FILTER(WHERE action='member.terms_accept'),count(*) FILTER(WHERE action='user.operator_create') FROM audit_logs WHERE resource_id=$1", response.MemberID).Scan(&acceptAuditCount, &createAuditCount); err != nil || acceptAuditCount != 1 || createAuditCount != 1 {
		t.Fatalf("audit counts accept=%d create=%d err=%v", acceptAuditCount, createAuditCount, err)
	}
	var auditText string
	if err = p.QueryRow(ctx, "SELECT coalesce(before_json::text,'')||coalesce(after_json::text,'')||reason FROM audit_logs WHERE id=$1", response.AuditLogID).Scan(&auditText); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(auditText, "operator-created-password") || strings.Contains(auditText, "argon2id") || strings.Contains(auditText, "password_hash") {
		t.Fatalf("secret leaked into operator audit: %s", auditText)
	}
	dupTx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := s.OperatorCreate(ctx, dupTx, operatorTestOtherBrand, adminID, OperatorInput{Username: "pending_user", Phone: "+12025550125", Password: "another-secure-password", Reason: "duplicate global identity"}, meta)
	if err != nil {
		_ = dupTx.Rollback(ctx)
		t.Fatal(err)
	}
	if duplicate.Status != 409 || duplicate.Error == nil || duplicate.Error.Code != "IDENTITY_EXISTS" {
		t.Fatalf("duplicate identity: %+v", duplicate)
	}
	_ = dupTx.Rollback(ctx)
	var userCount, memberCount int
	if err = p.QueryRow(ctx, "SELECT count(*) FROM global_users WHERE username='pending_user'").Scan(&userCount); err != nil {
		t.Fatal(err)
	}
	if err = p.QueryRow(ctx, "SELECT count(*) FROM brand_members WHERE global_user_id=$1", response.UserID).Scan(&memberCount); err != nil {
		t.Fatal(err)
	}
	if userCount != 1 || memberCount != 1 {
		t.Fatalf("duplicate altered identity/membership: users=%d members=%d", userCount, memberCount)
	}
}

func TestOperatorCreateReturnsTransientPasswordHashBusyError(t *testing.T) {
	p := testdb.New(t)
	s, err := New(p)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	adminID := operatorTestAdmin(t, ctx, p)
	for i := 0; i < cap(s.slots); i++ {
		s.slots <- struct{}{}
	}
	defer func() {
		for i := 0; i < cap(s.slots); i++ {
			<-s.slots
		}
	}()
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.OperatorCreate(ctx, tx, operatorTestBrand, adminID, OperatorInput{
		Username: "busy_operator", Password: "operator-created-password", Reason: "temporary hash queue saturation",
	}, Metadata{RequestID: "operator-busy-test"})
	_ = tx.Rollback(ctx)
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("OperatorCreate error=%v, want ErrBusy", err)
	}
	if result.Status != 0 || result.Error != nil {
		t.Fatalf("transient error was converted into a final mutation result: %+v", result)
	}
}

func TestAuthSettingsCASValidationAndCrossStoreVisibility(t *testing.T) {
	p := testdb.New(t)
	s, err := New(p)
	if err != nil {
		t.Fatal(err)
	}
	otherStore, err := New(p)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	adminID := operatorTestAdmin(t, ctx, p)
	meta := Metadata{RequestID: "auth-settings-test", IP: "127.0.0.1"}
	before, err := s.ReadAuthSettings(ctx, operatorTestBrand)
	if err != nil {
		t.Fatal(err)
	}
	badTx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	invalid, err := s.UpdateAuthSettings(ctx, badTx, operatorTestBrand, adminID, AuthSettingsInput{Version: before.Version, TelegramEnabled: true, TelegramClientID: "12oops", Reason: "invalid client"}, meta)
	if err != nil || invalid.Status != 400 {
		t.Fatalf("invalid client id: %+v err=%v", invalid, err)
	}
	_ = badTx.Rollback(ctx)
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := s.UpdateAuthSettings(ctx, tx, operatorTestBrand, adminID, AuthSettingsInput{Version: before.Version, CaptchaEnabled: true, TelegramEnabled: true, TelegramClientID: "9007199254740991", Reason: "enable public login methods"}, meta)
	if err != nil || updated.Status != 200 {
		_ = tx.Rollback(ctx)
		t.Fatalf("valid update: %+v err=%v", updated, err)
	}
	var updateResponse authSettingsUpdated
	if err = json.Unmarshal(updated.Data, &updateResponse); err != nil {
		t.Fatal(err)
	}
	if updateResponse.Version != before.Version+1 || updateResponse.PrivacyPolicyVersion != before.PrivacyPolicyVersion || updateResponse.ServiceTermsVersion != before.ServiceTermsVersion || updateResponse.AuditLogID == "" {
		t.Fatalf("update response omitted or changed preserved record fields: before=%+v response=%+v", before, updateResponse)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := otherStore.ReadAuthSettings(ctx, operatorTestBrand)
	if err != nil {
		t.Fatal(err)
	}
	if after.Version != before.Version+1 || !after.CaptchaEnabled || !after.TelegramEnabled || after.TelegramClientID != "9007199254740991" || after.PrivacyPolicyVersion != before.PrivacyPolicyVersion || after.ServiceTermsVersion != before.ServiceTermsVersion {
		t.Fatalf("unexpected cross-Store config: before=%+v after=%+v", before, after)
	}
	staleTx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := s.UpdateAuthSettings(ctx, staleTx, operatorTestBrand, adminID, AuthSettingsInput{Version: before.Version, Reason: "stale write"}, meta)
	if err != nil || stale.Status != 409 || stale.Error == nil || stale.Error.Code != "CONFIG_VERSION_CONFLICT" {
		t.Fatalf("stale update: %+v err=%v", stale, err)
	}
	_ = staleTx.Rollback(ctx)
	var terms string
	if err = p.QueryRow(ctx, "SELECT (auth_config->>'privacy_policy_version')||'/'||(auth_config->>'service_terms_version') FROM brands WHERE id=$1", operatorTestBrand).Scan(&terms); err != nil {
		t.Fatal(err)
	}
	if terms != before.PrivacyPolicyVersion+"/"+before.ServiceTermsVersion {
		t.Fatalf("policy versions changed: %s", terms)
	}
	var action, beforeJSON, afterJSON string
	if err = p.QueryRow(ctx, "SELECT action,before_json::text,after_json::text FROM audit_logs WHERE action='auth_config.update' ORDER BY created_at DESC LIMIT 1").Scan(&action, &beforeJSON, &afterJSON); err != nil {
		t.Fatal(err)
	}
	if action != "auth_config.update" || !strings.Contains(beforeJSON, "telegram_client_id") || !strings.Contains(afterJSON, "telegram_client_id") || strings.Contains(afterJSON, "password") || strings.Contains(afterJSON, "secret") {
		t.Fatalf("unexpected config audit: %s %s %s", action, beforeJSON, afterJSON)
	}
}
