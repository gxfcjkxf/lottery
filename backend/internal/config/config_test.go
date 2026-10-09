package config

import (
	"strings"
	"testing"
)

func cleanEnv(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", "development")
	t.Setenv("HTTP_ADDR", "127.0.0.1:8080")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("DATABASE_READ_URL", "")
	t.Setenv("DATABASE_READ_URLS", "")
	t.Setenv("DB_MAX_CONNS", "")
	t.Setenv("TRUSTED_PROXY_CIDRS", "")
}

func TestMissingDatabase(t *testing.T) {
	cleanEnv(t)
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("missing database must fail")
	}
}
func TestBounds(t *testing.T) {
	cleanEnv(t)
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
	cleanEnv(t)
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

func TestDatabaseReadURLs(t *testing.T) {
	cleanEnv(t)
	t.Setenv("DATABASE_READ_URLS", ` [" postgres://replica-a/test ", "postgres://replica-b/test"] `)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.DatabaseReadURLs) != 2 || c.DatabaseReadURLs[0] != "postgres://replica-a/test" || c.DatabaseReadURLs[1] != "postgres://replica-b/test" {
		t.Fatalf("read URLs = %#v", c.DatabaseReadURLs)
	}
}

func TestDatabaseReadURLIsUnsupported(t *testing.T) {
	cleanEnv(t)
	for _, value := range []string{"postgres://legacy/test", " "} {
		t.Setenv("DATABASE_READ_URL", value)
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "DATABASE_READ_URL is unsupported") {
			t.Fatalf("legacy read URL %q was not explicitly rejected: %v", value, err)
		}
	}
}

func TestInvalidDatabaseReadURLs(t *testing.T) {
	bad := []struct {
		name string
		read string
		list string
	}{
		{name: "legacy and list", read: "postgres://legacy/test", list: `["postgres://replica/test"]`},
		{name: "not JSON array", list: `"postgres://replica/test"`},
		{name: "null", list: `null`},
		{name: "empty entry", list: `[""]`},
		{name: "whitespace entry", list: `["  "]`},
		{name: "duplicate after trimming", list: `["postgres://replica/test", " postgres://replica/test "]`},
		{name: "too many", list: `["postgres://a/test","postgres://b/test","postgres://c/test","postgres://d/test","postgres://e/test","postgres://f/test","postgres://g/test","postgres://h/test","postgres://i/test"]`},
		{name: "invalid list URL", list: `["https://replica/test"]`},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			cleanEnv(t)
			t.Setenv("DATABASE_READ_URL", tc.read)
			t.Setenv("DATABASE_READ_URLS", tc.list)
			if _, err := Load(); err == nil {
				t.Fatal("invalid read replica configuration accepted")
			} else if strings.Contains(err.Error(), "replica") || strings.Contains(err.Error(), "postgres://") || strings.Contains(err.Error(), "https://") {
				t.Fatalf("error exposed a DSN: %v", err)
			}
		})
	}
}

func TestDatabaseReadURLLoadIsolation(t *testing.T) {
	cleanEnv(t)
	t.Setenv("DATABASE_READ_URLS", `["postgres://replica/test"]`)
	first, err := Load()
	if err != nil || len(first.DatabaseReadURLs) != 1 {
		t.Fatalf("first load = %+v, err = %v", first, err)
	}
	t.Setenv("DATABASE_READ_URLS", "")
	second, err := Load()
	if err != nil || len(second.DatabaseReadURLs) != 0 {
		t.Fatalf("second load = %+v, err = %v", second, err)
	}
}
