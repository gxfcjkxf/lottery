package config

import "testing"

func TestMissingDatabase(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("missing database must fail")
	}
}
func TestBounds(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	for _, v := range []string{"0", "501", "abc"} {
		t.Setenv("DB_MAX_CONNS", v)
		if _, err := Load(); err == nil {
			t.Fatalf("accepted %q", v)
		}
	}
	t.Setenv("DB_MAX_CONNS", "20")
	c, err := Load()
	if err != nil || c.DBMaxConns != 20 {
		t.Fatalf("config: %v", err)
	}
}
func TestTrustedProxyConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	for _, v := range []string{"invalid", "0.0.0.0/0", "::/0"} {
		t.Setenv("TRUSTED_PROXY_CIDRS", v)
		if _, err := Load(); err == nil {
			t.Fatal("unsafe proxy config accepted", v)
		}
	}
	t.Setenv("TRUSTED_PROXY_CIDRS", "192.0.2.0/24, 2001:db8::/32")
	c, err := Load()
	if err != nil || len(c.TrustedProxies) != 2 {
		t.Fatal(err)
	}
}
