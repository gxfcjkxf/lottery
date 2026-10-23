package httpapi

import (
	"bytes"
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/brandskin"
	"image"
	"image/png"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBrandLogoUploadPublishIsolationAndDefaults(t *testing.T) {
	f := managedFixture(t)
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	upload := func(brand, media string, body []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://localhost/api/v1/admin/brand-presentation/logo", bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+f.token)
		r.Header.Set("X-Brand-ID", brand)
		r.Header.Set("Content-Type", media)
		w := httptest.NewRecorder()
		f.http.ServeHTTP(w, r)
		return w
	}
	mustStatus(t, upload(managedBrand, "image/png", data.Bytes()), 403)
	grantPresentation(t, f)
	defaults := f.call("GET", "/api/v1/admin/brand-presentation/defaults", "", f.token, managedBrand, nil)
	mustStatus(t, defaults, 200)
	var base brandskin.Effective
	managedData(t, defaults, &base)
	if base.DisplayName != "Aurora" || base.LogoURL != nil {
		t.Fatal(base)
	}
	first := upload(managedBrand, "image/png", data.Bytes())
	mustStatus(t, first, 200)
	var receipt struct {
		URL string `json:"url"`
	}
	managedData(t, first, &receipt)
	replay := upload(managedBrand, "image/png", data.Bytes())
	mustStatus(t, replay, 200)
	if !strings.Contains(replay.Body.String(), receipt.URL) {
		t.Fatal("unstable upload")
	}
	for _, bad := range []struct {
		media string
		body  []byte
	}{{"image/svg+xml", []byte("<svg/>")}, {"image/jpeg", data.Bytes()}, {"image/png", data.Bytes()[:24]}, {"image/png", make([]byte, (1<<20)+1)}} {
		mustStatus(t, upload(managedBrand, bad.media, bad.body), 400)
	}
	mustStatus(t, upload("0199a000-0000-7000-8000-000000000002", "image/png", data.Bytes()), 403)
	var before brandskin.Record
	managedData(t, f.call("GET", presentationPath, "", f.token, managedBrand, nil), &before)
	if before.Config.LogoURL != nil {
		t.Fatal("upload published before Save")
	}
	cfg := before.Config
	cfg.LogoURL = &receipt.URL
	mustStatus(t, f.call("PUT", presentationPath, "logo-publish-001", f.token, managedBrand, brandskin.Input{Version: before.Version, Config: cfg}), 200)
	public := f.call("GET", receipt.URL, "", "", "", nil)
	mustStatus(t, public, 200)
	if public.Header().Get("Content-Type") != "image/png" || !bytes.Equal(public.Body.Bytes(), data.Bytes()) {
		t.Fatal("asset changed")
	}
	mustStatus(t, f.call("GET", strings.TrimSuffix(receipt.URL, "png")+"jpg", "", "", "", nil), 404)
	var count int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM brand_logo_assets WHERE brand_id=$1`, managedBrand).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE brands SET status='disabled' WHERE id=$1`, managedBrand); err != nil {
		t.Fatal(err)
	}
	mustStatus(t, upload(managedBrand, "image/png", data.Bytes()), 409)
}
