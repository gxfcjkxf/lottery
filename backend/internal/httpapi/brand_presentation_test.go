package httpapi

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/brandskin"
	"testing"
)

const presentationPath = "/api/v1/admin/brand-presentation"

func grantPresentation(t *testing.T, f managementHTTP) {
	t.Helper()
	for _, key := range []string{"brand_presentation.view.brand", "brand_presentation.write.brand"} {
		if _, e := f.pool.Exec(context.Background(), `INSERT INTO role_permissions(role_id,permission_key)SELECT role_id,$2 FROM admin_account_roles WHERE account_id=$1 ON CONFLICT DO NOTHING`, f.root, key); e != nil {
			t.Fatal(e)
		}
	}
}
func TestBrandPresentationHTTPPublishesEffectiveContextAndCheckedReplay(t *testing.T) {
	f := managedFixture(t)
	mustStatus(t, f.call("GET", presentationPath, "", f.token, managedBrand, nil), 403)
	grantPresentation(t, f)
	initial := f.call("GET", presentationPath, "", f.token, managedBrand, nil)
	mustStatus(t, initial, 200)
	var before brandskin.Record
	managedData(t, initial, &before)
	title, color, locale := "运营显示名称", "#123456", "zh-CN"
	cfg := before.Config
	cfg.DisplayName = &title
	cfg.PrimaryColor = &color
	cfg.DefaultLocale = &locale
	body := brandskin.Input{Version: before.Version, Config: cfg, Reason: "publish exact audited presentation"}
	first := f.call("PUT", presentationPath, "presentation-publish-01", f.token, managedBrand, body)
	mustStatus(t, first, 200)
	var published brandskin.Record
	managedData(t, first, &published)
	if published.Version != before.Version+1 || published.AuditLogID == "" || published.Effective.DisplayName != title {
		t.Fatal(published)
	}
	replay := f.call("PUT", presentationPath, "presentation-publish-01", f.token, managedBrand, body)
	mustStatus(t, replay, 200)
	var repeated brandskin.Record
	managedData(t, replay, &repeated)
	if repeated.AuditLogID != published.AuditLogID || repeated.Version != published.Version {
		t.Fatal(repeated)
	}
	changed := body
	changed.Reason = "different intent"
	mustStatus(t, f.call("PUT", presentationPath, "presentation-publish-01", f.token, managedBrand, changed), 409)
	mustStatus(t, f.call("PUT", presentationPath, "presentation-stale-01", f.token, managedBrand, body), 409)
	public := f.call("GET", "/api/v1/context", "", "", managedBrand, nil)
	mustStatus(t, public, 200)
	var contextData struct {
		Brand struct {
			Name   string `json:"name"`
			Locale string `json:"default_locale"`
			Theme  struct {
				Primary string `json:"primary_color"`
			} `json:"theme"`
		} `json:"brand"`
		Locales []string `json:"available_locales"`
	}
	managedData(t, public, &contextData)
	if contextData.Brand.Name != title || contextData.Brand.Locale != locale || contextData.Brand.Theme.Primary != color || len(contextData.Locales) != 2 {
		t.Fatal(contextData)
	}
	other := f.call("GET", "/api/v1/b/harbor/context", "", "", managedBrand, nil)
	mustStatus(t, other, 200)
	managedData(t, other, &contextData)
	if contextData.Brand.Name != "Harbor" || contextData.Brand.Theme.Primary != "#274c75" {
		t.Fatal("other brand presentation changed", contextData)
	}
	mustStatus(t, f.call("GET", presentationPath, "", f.token, "0199a000-0000-7000-8000-000000000002", nil), 403)
	for _, query := range []string{"?limit=3&limit=4", "?unknown=x", "?offset=-1", "?limit=101", "?limit=%ZZ", "?limit=3;offset=0"} {
		mustStatus(t, f.call("GET", presentationPath+"/history"+query, "", f.token, managedBrand, nil), 400)
	}
	for _, raw := range []string{`{"version":1,"config":{},"reason":"test"}`, `{"version":1,"version":1,"config":{},"reason":"test"}`} {
		mustStatus(t, f.rawCall("PUT", presentationPath, "presentation-invalid-01", f.token, managedBrand, raw), 400)
	}
	if _, e := f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1) AND permission_key='brand_presentation.write.brand'`, f.root); e != nil {
		t.Fatal(e)
	}
	mustStatus(t, f.call("PUT", presentationPath, "presentation-publish-01", f.token, managedBrand, body), 403)
	var ledger, orders int
	if e := f.pool.QueryRow(context.Background(), `SELECT(SELECT count(*) FROM point_ledger_entries),(SELECT count(*) FROM bet_orders)`).Scan(&ledger, &orders); e != nil || ledger != 0 || orders != 0 {
		t.Fatal(ledger, orders, e)
	}
}
