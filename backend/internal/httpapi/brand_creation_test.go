package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/brandregistry"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/tenant"
	"github.com/gxfcjkxf/lottery/backend/internal/testdb"
)

const creationPath = "/api/v1/admin/brands"

func grantCreation(t *testing.T, f managementHTTP) string {
	t.Helper()
	ctx := context.Background()
	role := ids.New()
	if _, err := f.pool.Exec(ctx, `INSERT INTO roles(id,code,name,is_bootstrap) VALUES($1,$2,'Platform creation',true)`, role, "platform_"+role); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"brand.create.platform", "brand.view.platform"} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO permissions(key) VALUES($1) ON CONFLICT DO NOTHING`, key); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,$2)`, role, key); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO admin_account_roles(account_id,role_id) VALUES($1,$2)`, f.root, role); err != nil {
		t.Fatal(err)
	}
	return role
}

func creationInput() brandregistry.Input {
	return brandregistry.Input{Code: "http_brand", Name: "HTTP Brand", DefaultLocale: "en", Timezone: "UTC", Reason: "create independent brand"}
}

func TestBrandCreationHTTPGlobalCheckedReplayAndRevocation(t *testing.T) {
	f := managedFixture(t)
	ctx := context.Background()
	in := creationInput()
	mustStatus(t, f.call("POST", creationPath, "brand-create-0001", f.token, "", in), 403)
	role := grantCreation(t, f)
	first := f.call("POST", creationPath, "brand-create-0001", f.token, "", in)
	mustStatus(t, first, 201)
	var receipt brandregistry.Receipt
	managedData(t, first, &receipt)
	if receipt.Status != "paused" || receipt.Version != 1 || receipt.AuditLogID == "" {
		t.Fatal(receipt)
	}
	replay := f.call("POST", creationPath, "brand-create-0001", f.token, "", in)
	mustStatus(t, replay, 201)
	var same brandregistry.Receipt
	managedData(t, replay, &same)
	if same != receipt {
		t.Fatal("creation replay changed initial receipt")
	}
	// A creation receipt is historical even after an independently authorized
	// operator resumes the brand. It must not be rebuilt from current projection.
	if _, err := f.pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_key) VALUES($1,'brand_operation.write.platform')`, role); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("PATCH", "/api/v1/admin/brand-operation", "brand-created-resume-01", f.token, receipt.ID, map[string]any{"version": int64(1), "status": "active", "reason": "resume after separate configuration"}), 200)
	replay = f.call("POST", creationPath, "brand-create-0001", f.token, "", in)
	mustStatus(t, replay, 201)
	managedData(t, replay, &same)
	if same != receipt {
		t.Fatal("replay confused creation snapshot with active brand")
	}
	mustStatus(t, f.call("POST", creationPath, "brand-create-0001", f.token, managedBrand, in), 400)
	changed := in
	changed.Name = "Other"
	mustStatus(t, f.call("POST", creationPath, "brand-create-0001", f.token, "", changed), 409)
	duplicate := f.call("POST", creationPath, "brand-create-0002", f.token, "", in)
	mustStatus(t, duplicate, 409)
	if !strings.Contains(duplicate.Body.String(), "BRAND_CODE_CONFLICT") {
		t.Fatal("duplicate code error missing")
	}
	var records, scopes int
	var stored string
	if err := f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM brand_creation_records),(SELECT count(*) FROM admin_brand_scopes WHERE brand_id=$1),(SELECT response::text FROM platform_idempotency_requests WHERE operation='admin.brand.create' AND key='brand-create-0001')`, receipt.ID).Scan(&records, &scopes, &stored); err != nil || records != 1 || scopes != 0 || strings.Contains(stored, in.Code) {
		t.Fatal("bad creation side effects/cache", err, records, scopes)
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_key='brand.create.platform'`, role); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("POST", creationPath, "brand-create-0001", f.token, "", in), 403)
}

func TestBrandlessPlatformCanLoginCreateFirstBrandAndRestoreSession(t *testing.T) {
	p := testdb.NewUnseeded(t)
	ctx := context.Background()
	users, err := identity.New(p)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := users.PasswordHash(ctx, "root-test-password-2026")
	if err != nil {
		t.Fatal(err)
	}
	root := ids.New()
	if _, err = p.Exec(ctx, `INSERT INTO admin_accounts(id,username,password_hash) VALUES($1,'managed_root',$2)`, root, hash); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Exec(ctx, `INSERT INTO platform_domains(domain) VALUES('localhost')`); err != nil {
		t.Fatal(err)
	}
	engine, err := mutation.New(p, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	f := managementHTTP{pool: p, root: root, http: New(Dependencies{Brands: tenant.Store{DB: p}, Ready: p.Ping, Identity: users, Mutations: engine, Admins: adminsys.Store{DB: p}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})}
	var brands, archives, guarded int
	if err = p.QueryRow(ctx, `SELECT (SELECT count(*) FROM brands),(SELECT count(*) FROM report_archives),(SELECT count(*) FROM pg_trigger WHERE tgrelid='report_archives'::regclass AND tgname='guarded_report_archive_truncate' AND tgenabled='O')`).Scan(&brands, &archives, &guarded); err != nil || brands != 0 || archives != 0 || guarded != 1 {
		t.Fatal("first-brand schema must be empty with archive guard intact", brands, archives, guarded, err)
	}
	grantCreation(t, f)
	mustStatus(t, f.call("GET", "/api/v1/context", "", "", "", nil), 404)
	login := f.call("POST", "/api/v1/admin/auth/login", "brandless-login-0001", "", "", map[string]string{"identifier": "managed_root", "password": "root-test-password-2026"})
	mustStatus(t, login, 200)
	var auth identity.AdminAuthentication
	managedData(t, login, &auth)
	mustStatus(t, f.call("GET", "/api/v1/admin/me", "", auth.AccessToken, "", nil), 200)
	created := f.call("POST", creationPath, "first-brand-create-01", auth.AccessToken, "", creationInput())
	mustStatus(t, created, 201)
	var out brandregistry.Receipt
	managedData(t, created, &out)
	mustStatus(t, f.call("GET", creationPath, "", auth.AccessToken, "", nil), 200)
	mustStatus(t, f.call("GET", "/api/v1/b/"+out.Code+"/context", "", "", "", nil), 200)
	// The bare platform host is not silently turned into a user's brand.
	mustStatus(t, f.call("GET", "/api/v1/context", "", "", "", nil), 404)
	logout := f.call("POST", "/api/v1/admin/auth/logout", "brandless-logout-01", auth.AccessToken, "", map[string]any{})
	mustStatus(t, logout, 200)
	mustStatus(t, f.call("GET", "/api/v1/admin/me", "", auth.AccessToken, "", nil), 401)
	mustStatus(t, f.call("POST", "/api/v1/admin/auth/logout", "brandless-logout-01", auth.AccessToken, "", map[string]any{}), 200)
}

func TestBrandCreationRejectsUnknownHostAndClosedInput(t *testing.T) {
	f := managedFixture(t)
	grantCreation(t, f)
	for _, body := range []map[string]any{
		{"code": "valid"},
		{"code": "valid", "name": "Valid", "default_locale": "en", "timezone": "UTC", "reason": "create", "brand_id": managedBrand},
		{"code": "valid", "name": "Valid", "default_locale": "en", "timezone": "UTC", "reason": "create", "status": "active"},
	} {
		mustStatus(t, f.call("POST", creationPath, "brand-invalid-body", f.token, "", body), 400)
	}
	raw, _ := json.Marshal(creationInput())
	req := httptest.NewRequest("POST", "http://unbound.example.test"+creationPath, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+f.token)
	req.Header.Set("Idempotency-Key", "brand-unknown-host")
	req.Header.Set("X-Forwarded-Host", "localhost")
	w := httptest.NewRecorder()
	f.http.ServeHTTP(w, req)
	mustStatus(t, w, 404)
	mustStatus(t, f.call("POST", creationPath+"?ignored=1", "brand-invalid-query", f.token, "", creationInput()), 400)
}
