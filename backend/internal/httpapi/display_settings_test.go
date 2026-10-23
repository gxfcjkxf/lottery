package httpapi

import (
	"context"
	"testing"
)

func TestPublicDisplaySettingsUsesPlatformPolicyOnly(t *testing.T) {
	f := managedFixture(t)
	if _, err := f.pool.Exec(context.Background(), `UPDATE platform_settings SET timezone='Asia/Singapore',date_format='DD/MM/YYYY',time_format='hh:mm:ss A' WHERE id=true`); err != nil {
		t.Fatal(err)
	}
	response := f.call("GET", "/api/v1/display-settings", "", "", "", nil)
	mustStatus(t, response, 200)
	var data map[string]string
	managedData(t, response, &data)
	if len(data) != 3 || data["timezone"] != "Asia/Singapore" || data["date_format"] != "DD/MM/YYYY" || data["time_format"] != "hh:mm:ss A" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(data, response.Header())
	}
}

func TestPublicPlatformPresentationExposesOnlyBranding(t *testing.T) {
	f := managedFixture(t)
	if _, err := f.pool.Exec(context.Background(), `UPDATE platform_settings SET display_name='智彩网',logo_text='Z',logo_url='/logo.png' WHERE id=true`); err != nil {
		t.Fatal(err)
	}
	response := f.call("GET", "/api/v1/platform/presentation", "", "", "", nil)
	mustStatus(t, response, 200)
	var data map[string]string
	managedData(t, response, &data)
	if len(data) != 3 || data["display_name"] != "智彩网" || data["logo_text"] != "Z" || data["logo_url"] != "/logo.png" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(data, response.Header())
	}
}
