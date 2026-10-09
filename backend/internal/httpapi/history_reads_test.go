package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/adminsys"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/database"
	"github.com/gxfcjkxf/lottery/backend/internal/identity"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/tenant"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func historyFixture(t *testing.T) (managementHTTP, Dependencies) {
	t.Helper()
	f := managedFixture(t)
	cfg := f.pool.Config().Copy()
	cfg.MaxConns = 1
	p, e := pgxpool.NewWithConfig(context.Background(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(p.Close)
	users, e := identity.New(p)
	if e != nil {
		t.Fatal(e)
	}
	engine, e := mutation.New(f.pool, make([]byte, 32))
	if e != nil {
		t.Fatal(e)
	}
	// A writable primary deliberately masquerading as a configured read node
	// must be rejected, not used just because a SELECT would succeed there.
	d := Dependencies{Brands: tenant.Store{DB: p}, Ready: p.Ping, Identity: users, Mutations: engine, Admins: adminsys.Store{DB: p}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), HistoryReads: database.NewHistoryRouter([]*pgxpool.Pool{f.pool})}
	f.http = New(d)
	return f, d
}
func grantHistoryReads(t *testing.T, f managementHTTP) {
	t.Helper()
	for _, resource := range []string{"notification_template", "compliance_policy", "brand_operation", "brand_domains", "brand_presentation"} {
		grantReportPermission(t, f, resource+".view.brand")
	}
}
func historyPaths() []string {
	return []string{
		"/api/v1/admin/audit",
		"/api/v1/admin/notification-templates/member.joined/history",
		"/api/v1/admin/compliance-policy/history",
		"/api/v1/admin/brand-operation/history",
		"/api/v1/admin/brand-presentation/history",
		"/api/v1/admin/brand-domains/history",
	}
}
func TestHistoryHTTPRejectsInvalidConfiguredReplicaWithoutFallback(t *testing.T) {
	f, _ := historyFixture(t)
	grantHistoryReads(t, f)
	for _, path := range historyPaths() {
		r := f.call("GET", path, "", f.token, managedBrand, nil)
		mustStatus(t, r, 503)
		if r.Header().Get("X-Read-Source") != "" || r.Header().Get("X-Read-Replica") != "" || strings.Contains(r.Body.String(), `"items"`) {
			t.Fatal(path, r.Header())
		}
	}
	for _, path := range []string{"/api/v1/admin/notification-templates", "/api/v1/admin/compliance-policy", "/api/v1/admin/brand-operation", "/api/v1/admin/brand-presentation", "/api/v1/admin/brand-domains", "/api/v1/admin/me"} {
		r := f.call("GET", path, "", f.token, managedBrand, nil)
		mustStatus(t, r, 200)
		if r.Header().Get("X-Read-Source") != "" {
			t.Fatal("current read entered history router", path)
		}
	}
}
func TestHistoryHTTPAuditFailureDoesNotReleasePayload(t *testing.T) {
	f, _ := historyFixture(t)
	grantHistoryReads(t, f)
	_, e := f.pool.Exec(context.Background(), `CREATE FUNCTION reject_history_audit() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN IF NEW.action IN ('audit.view','notification.template.history','compliance_policy.history.view','brand_operation.history','brand_presentation.history','brand_domains.history') THEN RAISE EXCEPTION 'injected audit failure'; END IF; RETURN NEW; END$$;CREATE TRIGGER reject_history_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_history_audit()`)
	if e != nil {
		t.Fatal(e)
	}
	for _, path := range historyPaths() {
		r := f.call("GET", path, "", f.token, managedBrand, nil)
		mustStatus(t, r, 503)
		if strings.Contains(r.Body.String(), `"items"`) || r.Header().Get("X-Read-Source") != "" {
			t.Fatal("unaudited history released", path, r.Body.String())
		}
	}
}
func TestHistoryHTTPPrimaryAuthorizationWinsOverReadNode(t *testing.T) {
	f, d := historyFixture(t)
	grantHistoryReads(t, f)
	initial, e := d.Admins.Account(context.Background(), f.root)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='audit.view.brand'`, f.root)
	if e != nil {
		t.Fatal(e)
	}
	r := httptest.NewRequest("GET", "http://localhost/api/v1/admin/audit", nil)
	r.Header.Set("Authorization", "Bearer "+f.token)
	w := httptest.NewRecorder()
	_, e = auditedHistoryRecord(w, r, d, initial, func(fresh access.Account) bool {
		return access.Authorize(fresh, "audit", "view", access.ScopeBrand, managedBrand)
	}, audit.Record{BrandID: managedBrand, Action: "audit.view", ResourceType: "audit"}, func(pgx.Tx) (any, error) { t.Fatal("revoked grant reached data query"); return nil, nil })
	if !historyFailure(w, r, e) || w.Code != http.StatusForbidden || w.Header().Get("X-Read-Source") != "" {
		t.Fatal(e, w.Code, w.Header())
	}
	mustStatus(t, f.call("GET", "/api/v1/admin/audit", "", f.token, managedBrand, nil), 403)
	_, e = f.pool.Exec(context.Background(), `UPDATE admin_accounts SET status='disabled' WHERE id=$1`, f.root)
	if e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("GET", "/api/v1/admin/notification-templates/member.joined/history", "", f.token, managedBrand, nil), 401)
}
func TestHistoryUnknownClientReadHintCannotEnableRouting(t *testing.T) {
	f, _ := historyFixture(t)
	grantHistoryReads(t, f)
	r := httptest.NewRequest("GET", "http://localhost/api/v1/admin/notification-templates", nil)
	r.Header.Set("Authorization", "Bearer "+f.token)
	r.Header.Set("X-Brand-ID", managedBrand)
	r.Header.Set("X-Read-Source", "replica")
	r.Header.Set("X-Read-Replica", "1")
	w := httptest.NewRecorder()
	f.http.ServeHTTP(w, r)
	mustStatus(t, w, 200)
	if w.Header().Get("X-Read-Source") != "" {
		t.Fatal("caller controlled routing")
	}
}
