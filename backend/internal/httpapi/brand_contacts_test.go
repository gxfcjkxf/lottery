package httpapi

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/brandskin"
)

func TestBrandContactsUploadPublishClearReplayAndIsolation(t *testing.T) {
	f := managedFixture(t)
	const path = "/api/v1/admin/brand-contacts"
	mustStatus(t, f.call("GET", path, "", f.token, managedBrand, nil), 403)
	grantPresentation(t, f)
	var before brandskin.ContactRecord
	initial := f.call("GET", path, "", f.token, managedBrand, nil)
	mustStatus(t, initial, 200)
	managedData(t, initial, &before)
	if before.Version != 1 || before.Config.Items == nil || len(before.Config.Items) != 0 || before.Config.DefaultID != nil {
		t.Fatal(before)
	}
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	upload := func(brand, media string, content []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://localhost"+path+"/image", bytes.NewReader(content))
		r.Header.Set("Authorization", "Bearer "+f.token)
		r.Header.Set("X-Brand-ID", brand)
		r.Header.Set("Content-Type", media)
		w := httptest.NewRecorder()
		f.http.ServeHTTP(w, r)
		return w
	}
	var asset struct {
		URL string `json:"url"`
	}
	imageResponse := upload(managedBrand, "image/png", data.Bytes())
	mustStatus(t, imageResponse, 200)
	managedData(t, imageResponse, &asset)
	mustStatus(t, upload("0199a000-0000-7000-8000-000000000002", "image/png", data.Bytes()), 403)
	mustStatus(t, upload(managedBrand, "image/svg+xml", []byte("<svg/>")), 400)
	id := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	cfg := brandskin.ContactConfig{Items: []brandskin.Contact{{ID: id, Kind: "wechat", Value: "support_wechat", ImageURL: &asset.URL}}, DefaultID: &id}
	body := brandskin.ContactInput{Version: 1, Config: cfg}
	first := f.call("PUT", path, "contact-publish-001", f.token, managedBrand, body)
	mustStatus(t, first, 200)
	var after, repeated brandskin.ContactRecord
	managedData(t, first, &after)
	replay := f.call("PUT", path, "contact-publish-001", f.token, managedBrand, body)
	mustStatus(t, replay, 200)
	managedData(t, replay, &repeated)
	if after.Version != 2 || after.AuditLogID == "" || repeated.AuditLogID != after.AuditLogID {
		t.Fatal(after, repeated)
	}
	mustStatus(t, f.call("PUT", path, "contact-stale-001", f.token, managedBrand, body), 409)
	var public brandskin.ContactConfig
	result := f.call("GET", "/api/v1/brand-contacts", "", "", managedBrand, nil)
	mustStatus(t, result, 200)
	managedData(t, result, &public)
	if len(public.Items) != 1 || public.Items[0].Value != "support_wechat" || *public.Items[0].ImageURL != asset.URL {
		t.Fatal(public)
	}
	other := f.call("GET", "/api/v1/b/harbor/brand-contacts", "", "", managedBrand, nil)
	mustStatus(t, other, 200)
	managedData(t, other, &public)
	if len(public.Items) != 0 || public.DefaultID != nil {
		t.Fatal("cross-brand leak", public)
	}
	mustStatus(t, f.call("GET", path, "", f.token, "0199a000-0000-7000-8000-000000000002", nil), 403)
	foreignID := strings.Repeat("a", 64)
	_, err := f.pool.Exec(context.Background(), `INSERT INTO brand_logo_assets(id,brand_id,media_type,content) VALUES($1,$2,'image/png',$3)`, foreignID, "0199a000-0000-7000-8000-000000000002", data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	foreignURL := "/api/v1/brand-assets/" + foreignID + ".png"
	cfg.Items[0].ImageURL = &foreignURL
	mustStatus(t, f.call("PUT", path, "contact-foreign-01", f.token, managedBrand, brandskin.ContactInput{Version: 2, Config: cfg}), 400)
	cfg.Items[0].ImageURL = nil
	cleared := f.call("PUT", path, "contact-clear-001", f.token, managedBrand, brandskin.ContactInput{Version: 2, Config: cfg})
	mustStatus(t, cleared, 200)
	managedData(t, cleared, &after)
	if after.Config.Items[0].ImageURL != nil || after.Config.Items[0].Value != "support_wechat" {
		t.Fatal("clear removed number", after)
	}
	cfg.Items[0].Value = ""
	cfg.Items[0].ImageURL = &asset.URL
	mustStatus(t, f.call("PUT", path, "contact-image-only", f.token, managedBrand, brandskin.ContactInput{Version: 3, Config: cfg}), 200)
	cfg.DefaultID = nil
	mustStatus(t, f.call("PUT", path, "contact-no-default", f.token, managedBrand, brandskin.ContactInput{Version: 4, Config: cfg}), 400)
	empty := brandskin.ContactConfig{Items: []brandskin.Contact{}, DefaultID: nil}
	mustStatus(t, f.call("PUT", path, "contact-remove-all", f.token, managedBrand, brandskin.ContactInput{Version: 4, Config: empty}), 200)
	if _, err = f.pool.Exec(context.Background(), `DELETE FROM role_permissions WHERE permission_key='brand_presentation.write.brand' AND role_id IN(SELECT role_id FROM admin_account_roles WHERE account_id=$1)`, f.root); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("PUT", path, "contact-publish-001", f.token, managedBrand, body), 403)
	grantPresentation(t, f)
	if _, err = f.pool.Exec(context.Background(), `UPDATE brands SET status='disabled' WHERE id=$1`, managedBrand); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, f.call("PUT", path, "contact-disabled-01", f.token, managedBrand, brandskin.ContactInput{Version: 5, Config: empty}), 409)
	mustStatus(t, upload(managedBrand, "image/png", data.Bytes()), 409)
}
