package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/agency"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"strconv"
)

func TestFixedManagementEntries(t *testing.T) {
	f := pointsFixture(t)
	agentGrantRoot(t, f.managementHTTP)
	ctx := context.Background()
	for _, host := range []string{"admin.fixed.test", "agent.fixed.test"} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO platform_domains(domain,enabled) VALUES($1,true)`, host); err != nil {
			t.Fatal(err)
		}
	}
	sequence := 0
	call := func(host, method, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
		sequence++
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "http://"+host+path, bytes.NewReader(data))
		r.RemoteAddr = "192.0.2.100:12345"
		r.Header.Set("Origin", "http://"+host)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", "fixed-entry-"+strconv.Itoa(sequence))
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		f.http.ServeHTTP(w, r)
		return w
	}
	mustStatus(t, call("admin.fixed.test", "GET", "/api/v1/admin/auth/config", nil, nil), 200)
	mustStatus(t, call("agent.fixed.test", "GET", "/api/v1/agent/auth/config", nil, nil), 200)
	mustStatus(t, call("agent.fixed.test", "GET", "/api/v1/agent/me", nil, nil), 401)
	mustStatus(t, call("unknown.fixed.test", "GET", "/api/v1/agent/auth/config", nil, nil), 404)
	mustStatus(t, call("agent.fixed.test", "GET", "/api/v1/context", nil, nil), 404)
	// A central CAPTCHA must not be consumable by a brand login.
	challenge := call("admin.fixed.test", "GET", "/api/v1/admin/auth/challenge", nil, nil)
	mustStatus(t, challenge, 200)
	var c identity.Challenge
	managedData(t, challenge, &c)
	var proof string
	if err := f.pool.QueryRow(ctx, `SELECT proof_hash FROM auth_challenges WHERE id=$1 AND brand_id IS NULL`, c.ID).Scan(&proof); err != nil {
		t.Fatal(err)
	}
	engine, _ := mutation.New(f.pool, make([]byte, 32))
	answer := ""
	for x := 0; x <= 300; x++ {
		if engine.Fingerprint(c.ID+":"+strconv.Itoa(x)) == proof {
			answer = strconv.Itoa(x)
			break
		}
	}
	store, _ := identity.New(f.pool)
	if ok, err := store.Consume(ctx, engine, managedBrand, "captcha", c.ID, answer); ok || err != nil {
		t.Fatal("central captcha crossed brand", ok, err)
	}
	if ok, err := store.Consume(ctx, engine, "", "captcha", c.ID, answer); !ok || err != nil {
		t.Fatal("central captcha failed", ok, err)
	}
	// Brand administrators log in without a host brand.
	login := call("admin.fixed.test", "POST", "/api/v1/admin/auth/login", map[string]string{"identifier": "managed_root", "password": "root-test-password-2026"}, nil)
	mustStatus(t, login, 200)
	var admin identity.AdminAuthentication
	managedData(t, login, &admin)
	f.token = admin.AccessToken
	// Globally unique agent credentials select their actual brand.
	created := f.call("POST", "/api/v1/admin/agents", "fixed-agent-create", f.token, managedBrand, agency.CreateInput{PolicyVersion: 1, Username: "fixed_unique_agent", Password: "agent123", Config: agency.NodeConfig{Mode: new("loss"), Ratio: "0", Status: "active"}})
	mustStatus(t, created, 201)
	login = call("agent.fixed.test", "POST", "/api/v1/agent/login", identity.AgentLoginInput{Username: "fixed_unique_agent", Password: "agent123"}, nil)
	mustStatus(t, login, 200)
	cookies := login.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("missing central agent cookie")
	}
	self := call("agent.fixed.test", "GET", "/api/v1/agent/me", nil, cookies[0])
	mustStatus(t, self, 200)
	var own struct {
		Account identity.AgentSession `json:"account"`
	}
	managedData(t, self, &own)
	if own.Account.BrandID != managedBrand {
		t.Fatal("wrong brand", own)
	}
	mustStatus(t, call("agent.fixed.test", "GET", "/api/v1/b/aurora/context", nil, cookies[0]), 200)
	// Session brand cannot be overridden with a path or header.
	mustStatus(t, call("agent.fixed.test", "GET", "/api/v1/b/harbor/agent/me", nil, cookies[0]), 401)
	// Cookie-authenticated writes revalidate the central session correctly.
	mustStatus(t, call("agent.fixed.test", "POST", "/api/v1/agent/password", identity.AgentPasswordInput{CurrentPassword: "agent123", NewPassword: "changed456"}, cookies[0]), 200)
	mustStatus(t, call("agent.fixed.test", "GET", "/api/v1/agent/me", nil, cookies[0]), 401)
	// Uniqueness is enforced by PostgreSQL across brands, including concurrent creates.
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO agent_accounts(id,brand_id,username,password_hash) SELECT gen_random_uuid(),id,'fixed_unique_agent','unused' FROM brands WHERE code='harbor'`)
	var duplicate *pgconn.PgError
	if !errors.As(err, &duplicate) || duplicate.Code != "23505" || duplicate.ConstraintName != "agent_accounts_username_key" {
		t.Fatal("missing global username uniqueness", err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	op := securityOperator(t, f.managementHTTP)
	grantPlatformPermission(t, op, "auth_config.view.platform", "auth_config.write.platform")
	var defaults identity.AuthSettingsRecord
	read := op.call("GET", "/api/v1/platform/auth-settings", "", op.token, "", nil)
	mustStatus(t, read, 200)
	managedData(t, read, &defaults)
	mustStatus(t, op.call("PATCH", "/api/v1/platform/auth-settings", "central-captcha-enable", op.token, "", identity.AuthSettingsInput{Version: defaults.Version, CaptchaEnabled: true}), 200)
	for _, surface := range []string{"admin", "agent"} {
		config := call(surface+".fixed.test", "GET", "/api/v1/"+surface+"/auth/config", nil, nil)
		mustStatus(t, config, 200)
		var value struct {
			Enabled bool `json:"captcha_enabled"`
		}
		managedData(t, config, &value)
		if !value.Enabled {
			t.Fatal("central config ignored platform captcha")
		}
	}
	mustStatus(t, call("admin.fixed.test", "POST", "/api/v1/admin/auth/login", map[string]string{"identifier": "missing_account", "password": "invalid123"}, nil), 400)
	mustStatus(t, call("agent.fixed.test", "POST", "/api/v1/agent/login", identity.AgentLoginInput{Username: "missing_agent", Password: "invalid123"}, nil), 400)

}
