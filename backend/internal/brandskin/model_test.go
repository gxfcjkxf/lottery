package brandskin

import (
	"encoding/json"
	"testing"
)

func TestResolveOverridesAndInheritance(t *testing.T) {
	name, primary, locale := "Harbor Display", "#123456", "zh-CN"
	c := Config{DisplayName: &name, PrimaryColor: &primary, DefaultLocale: &locale}
	e, err := Resolve("Harbor", c)
	if err != nil {
		t.Fatal(err)
	}
	if e.DisplayName != name || e.PrimaryColor != primary || e.AccentColor != "#d9ef9b" || e.DefaultLocale != locale || len(e.AvailableLocales) != 2 {
		t.Fatal(e)
	}
	plain, err := Resolve("Harbor", Config{})
	if err != nil || plain.DisplayName != "Harbor" || plain.LogoText != "H" || plain.PrimaryColor != "#20594c" {
		t.Fatal(plain, err)
	}
}
func TestPresentationInputIsClosed(t *testing.T) {
	raw, err := json.Marshal(Input{Version: 1, Config: Config{}, Reason: "presentation test"})
	if err != nil {
		t.Fatal(err)
	}
	var in Input
	if err = json.Unmarshal(raw, &in); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{`{"version":1,"config":{},"reason":"test"}`, `{"version":1,"version":1,"config":{},"reason":"test"}`, `{"version":1,"config":null,"reason":"test"}`, `{"version":1,"config":{},"reason":"test","extra":1}`} {
		if json.Unmarshal([]byte(bad), &in) == nil {
			t.Fatal("accepted invalid input", bad)
		}
	}
}
func TestAssetURIRejectsExecutableAndAmbiguousInputs(t *testing.T) {
	for _, uri := range []string{"/icons/icon-192.png", "/brand-assets/brand-1.webp", "https://cdn.example.com/logo.png"} {
		if !ValidAssetURI(uri) {
			t.Fatal("valid URI rejected", uri)
		}
	}
	for _, uri := range []string{"javascript:alert(1)", "data:image/png;base64,aaa", "//evil.example/logo.png", "/api/v1/me", "/icons/../api/logo.png", "/icons/%2e%2e/logo.png", "https://localhost/logo.png", "https://127.0.0.1/logo.png", "https://user:password@example.com/logo.png", "https://example.com/logo.png?token=secret", "https://example.com/logo.svg", "/icons/logo.png#frag"} {
		if ValidAssetURI(uri) {
			t.Fatal("invalid URI accepted", uri)
		}
	}
}
