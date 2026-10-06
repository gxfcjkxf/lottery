package identity

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/compliance"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/points"
	"github.com/gxfcjkxf/lottery/backend/internal/telegramauth"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func complianceConfig(t *testing.T, p *pgxpool.Pool, admin, brand string, enabled bool) {
	t.Helper()
	ctx := context.Background()
	s := compliance.Service{DB: p}
	v, e := s.Read(ctx, brand)
	if e != nil {
		t.Fatal(e)
	}
	tx, e := p.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	a := access.Account{ID: admin, Type: access.AccountAdmin, BrandIDs: []string{brand}, Roles: []access.Role{{BrandID: brand, Permissions: []access.Permission{{Resource: "compliance_policy", Action: "write", Scope: access.ScopeBrand}}}}}
	cfg := compliance.DefaultConfig()
	cfg.IdentityEnabled = enabled
	_, e = s.Update(ctx, tx, brand, a, compliance.Input{Version: v.Version, Config: cfg, Reason: "Configure identity admission fixture"}, points.Metadata{ActorType: "admin", ActorID: admin, RequestID: "identity-fixture-policy"})
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
}
func TestComplianceRegistrationJoinPendingAndOperatorDoNotBypass(t *testing.T) {
	p := testdb.New(t)
	ctx := context.Background()
	s, e := New(p)
	if e != nil {
		t.Fatal(e)
	}
	engine, e := mutation.New(p, make([]byte, 32))
	if e != nil {
		t.Fatal(e)
	}
	admin := operatorTestAdmin(t, ctx, p)
	meta := Metadata{RequestID: "identity-compliance", Domain: "localhost"}
	run := func(brand, key string, fn func(context.Context, pgx.Tx) (mutation.Result, error)) mutation.Result {
		t.Helper()
		r, e := engine.Execute(ctx, brand, "identity-compliance", "flow", key, engine.Fingerprint(key), fn)
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	complianceConfig(t, p, admin, operatorTestBrand, true)
	register := func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
		return s.Register(ctx, tx, operatorTestBrand, RegisterInput{Username: "compliance_first", Password: "identity-test-password-2026", Privacy: "dev-1", Terms: "dev-1"}, meta)
	}
	for range 2 {
		r := run(operatorTestBrand, "compliance-register-01", register)
		if r.Status != 409 || r.Error == nil || r.Error.Code != "COMPLIANCE_REVIEW_REQUIRED" {
			t.Fatal(r)
		}
	}
	var users, members, gates int
	if e = p.QueryRow(ctx, `SELECT (SELECT count(*) FROM global_users),(SELECT count(*) FROM brand_members),(SELECT count(*) FROM compliance_gate_rejections)`).Scan(&users, &members, &gates); e != nil || users != 0 || members != 0 || gates != 1 {
		t.Fatal("registration rejection changed identity or duplicated", users, members, gates, e)
	}
	operator := func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
		return s.OperatorCreate(ctx, tx, operatorTestBrand, admin, OperatorInput{Username: "compliance_operator", Password: "operator-password-2026", Reason: "Operator cannot bypass compliance"}, meta)
	}
	if r := run(operatorTestBrand, "compliance-operator-01", operator); r.Status != 409 {
		t.Fatal(r)
	}
	if e = p.QueryRow(ctx, `SELECT count(*) FROM global_users`).Scan(&users); e != nil || users != 0 {
		t.Fatal("operator left global identity after rollback", users, e)
	}
	complianceConfig(t, p, admin, operatorTestBrand, false)
	r := run(operatorTestBrand, "compliance-register-02", register)
	if r.Status != 201 {
		t.Fatal(r)
	}
	var auth Authentication
	if e = json.Unmarshal(r.Data, &auth); e != nil {
		t.Fatal(e)
	}
	complianceConfig(t, p, admin, operatorTestBrand, true)
	login := func(brand string, terms bool) func(context.Context, pgx.Tx) (mutation.Result, error) {
		return func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			in := LoginInput{Identifier: "compliance_first", Password: "identity-test-password-2026"}
			if terms {
				in.Privacy, in.Terms = "dev-1", "dev-1"
			}
			return s.Login(ctx, tx, brand, in, meta)
		}
	}
	if r = run(operatorTestBrand, "compliance-existing-login", login(operatorTestBrand, false)); r.Status != 200 {
		t.Fatal("existing login blocked", r)
	}
	complianceConfig(t, p, admin, operatorTestOtherBrand, true)
	if r = run(operatorTestOtherBrand, "compliance-newbrand-join", login(operatorTestOtherBrand, true)); r.Status != 409 || r.Error.Code != "COMPLIANCE_REVIEW_REQUIRED" {
		t.Fatal(r)
	}
	if e = p.QueryRow(ctx, `SELECT count(*) FROM brand_members WHERE brand_id=$1`, operatorTestOtherBrand).Scan(&members); e != nil || members != 0 {
		t.Fatal("joined brand under enabled stub", members, e)
	}
	complianceConfig(t, p, admin, operatorTestBrand, false)
	if r = run(operatorTestBrand, "compliance-pending-create", func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
		return s.OperatorCreate(ctx, tx, operatorTestBrand, admin, OperatorInput{Username: "compliance_pending", Password: "pending-password-2026", Reason: "Create before enabled check"}, meta)
	}); r.Status != 201 {
		t.Fatal(r)
	}
	complianceConfig(t, p, admin, operatorTestBrand, true)
	r = run(operatorTestBrand, "compliance-pending-login", func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
		return s.Login(ctx, tx, operatorTestBrand, LoginInput{Identifier: "compliance_pending", Password: "pending-password-2026", Privacy: "dev-1", Terms: "dev-1"}, meta)
	})
	if r.Status != 409 || r.Error.Code != "COMPLIANCE_REVIEW_REQUIRED" {
		t.Fatal(r)
	}
	var accepted bool
	if e = p.QueryRow(ctx, `SELECT terms_accepted FROM brand_members WHERE global_user_id=(SELECT id FROM global_users WHERE username='compliance_pending')`).Scan(&accepted); e != nil || accepted {
		t.Fatal("pending consent bypassed", accepted, e)
	}
	if _, e = s.Authenticate(ctx, operatorTestBrand, auth.AccessToken); e != nil {
		t.Fatal("old session reads blocked", e)
	}
}
func TestComplianceTelegramNewIdentityAndCrossBrandJoin(t *testing.T) {
	p := testdb.New(t)
	ctx := context.Background()
	s, e := New(p)
	if e != nil {
		t.Fatal(e)
	}
	engine, e := mutation.New(p, make([]byte, 32))
	if e != nil {
		t.Fatal(e)
	}
	admin := operatorTestAdmin(t, ctx, p)
	meta := Metadata{RequestID: "telegram-compliance", Domain: "localhost"}
	if _, e = p.Exec(ctx, `UPDATE brands SET auth_config=auth_config||'{"telegram_enabled":true,"telegram_client_id":"12345"}'::jsonb`); e != nil {
		t.Fatal(e)
	}
	run := func(brand, key string) mutation.Result {
		t.Helper()
		r, e := engine.Execute(ctx, brand, "telegram-compliance", "tg", key, engine.Fingerprint(key), func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			return s.Telegram(ctx, tx, brand, telegramauth.Claims{ID: "987777666", Subject: "verified-signature-test", Name: "Not identity verification"}, TelegramInput{Privacy: "dev-1", Terms: "dev-1"}, "", meta)
		})
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	complianceConfig(t, p, admin, operatorTestBrand, true)
	if r := run(operatorTestBrand, "compliance-telegram-01"); r.Status != 409 || r.Error.Code != "COMPLIANCE_REVIEW_REQUIRED" {
		t.Fatal(r)
	}
	var count int
	if e = p.QueryRow(ctx, `SELECT count(*) FROM global_users WHERE telegram_user_id='987777666'`).Scan(&count); e != nil || count != 0 {
		t.Fatal("new telegram identity persisted", count, e)
	}
	complianceConfig(t, p, admin, operatorTestBrand, false)
	if r := run(operatorTestBrand, "compliance-telegram-02"); r.Status != 200 {
		t.Fatal(r)
	}
	complianceConfig(t, p, admin, operatorTestBrand, true)
	if r := run(operatorTestBrand, "compliance-telegram-existing"); r.Status != 200 {
		t.Fatal("existing telegram login blocked", r)
	}
	complianceConfig(t, p, admin, operatorTestOtherBrand, true)
	if r := run(operatorTestOtherBrand, "compliance-telegram-join"); r.Status != 409 || r.Error.Code != "COMPLIANCE_REVIEW_REQUIRED" {
		t.Fatal(r)
	}
}

func TestComplianceEvidenceFailureRollsBackAllAndSameKeyCanRetry(t *testing.T) {
	p := testdb.New(t)
	ctx := context.Background()
	s, e := New(p)
	if e != nil {
		t.Fatal(e)
	}
	engine, e := mutation.New(p, make([]byte, 32))
	if e != nil {
		t.Fatal(e)
	}
	admin := operatorTestAdmin(t, ctx, p)
	complianceConfig(t, p, admin, operatorTestBrand, true)
	// The rejection insert fails in this test's newly owned schema, after the
	// audit append. Neither partial proof nor a negative cache may survive.
	if _, e = p.Exec(ctx, `CREATE FUNCTION reject_gate_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected proof storage failure'; END $$; CREATE TRIGGER gate_test_failure BEFORE INSERT ON compliance_gate_rejections FOR EACH ROW EXECUTE FUNCTION reject_gate_test()`); e != nil {
		t.Fatal(e)
	}
	call := func() (mutation.Result, error) {
		return engine.Execute(ctx, operatorTestBrand, "gate-storage-test", "registration", "gate-proof-retry-01", engine.Fingerprint("unchanged"), func(ctx context.Context, tx pgx.Tx) (mutation.Result, error) {
			return s.Register(ctx, tx, operatorTestBrand, RegisterInput{Username: "proof_failure_user", Password: "proof-test-password-2026", Privacy: "dev-1", Terms: "dev-1"}, Metadata{RequestID: "proof-storage-request"})
		})
	}
	if _, e = call(); e == nil {
		t.Fatal("failed proof storage returned accepted/cacheable result")
	}
	var rows int
	if e = p.QueryRow(ctx, `SELECT (SELECT count(*) FROM global_users)+(SELECT count(*) FROM compliance_gate_rejections)+(SELECT count(*) FROM audit_logs WHERE action='compliance.gate.reject')+(SELECT count(*) FROM idempotency_requests WHERE key='gate-proof-retry-01')`).Scan(&rows); e != nil || rows != 0 {
		t.Fatal("failed proof left partial data", rows, e)
	}
	if _, e = p.Exec(ctx, `DROP TRIGGER gate_test_failure ON compliance_gate_rejections`); e != nil {
		t.Fatal(e)
	}
	r, e := call()
	if e != nil || r.Status != 409 || r.Error.Code != "COMPLIANCE_REVIEW_REQUIRED" {
		t.Fatal(r, e)
	}
}
