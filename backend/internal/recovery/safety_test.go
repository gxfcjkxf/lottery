//go:build !windows

package recovery

import (
	"strings"
	"testing"
)

func TestDrillPortsRefuseExistingDevelopmentRanges(t *testing.T) {
	for _, raw := range []string{"0", "1023", "5432", "5429", "55429", "55432", "65532", "-1", "abc"} {
		t.Run(raw, func(t *testing.T) {
			if _, err := drillBasePort(raw); err == nil {
				t.Fatal("unsafe port accepted")
			}
		})
	}
	if n, err := drillBasePort(""); err != nil || n != 55531 {
		t.Fatal(n, err)
	}
}

func TestOriginalBaselineCannotRedirectToRemoteOrOwnedPort(t *testing.T) {
	for _, dsn := range []string{
		"", "postgres://example.com:55432/postgres", "postgres://127.0.0.1/postgres", "postgres://127.0.0.1:55531/postgres", "postgres://127.0.0.1:55534/postgres",
		"postgres://127.0.0.1:55432/postgres?host=192.0.2.1", "postgres://127.0.0.1:55432/postgres?port=55531", "postgres://127.0.0.1:55432/postgres?options=-c%20default_transaction_read_only=off",
		"postgres://127.0.0.1:55432/postgres?sslmode=disable&sslmode=require", "postgres://127.0.0.1:55432/postgres#fragment",
	} {
		t.Run(dsn, func(t *testing.T) {
			if _, err := originalReadConfig(dsn, 55531); err == nil {
				t.Fatal("unsafe baseline accepted")
			}
		})
	}
	cfg, err := originalReadConfig("postgres://lottery_test@127.0.0.1:55432/postgres?sslmode=disable", 55531)
	if err != nil || cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] != "on" {
		t.Fatal("baseline not strictly read-only", err)
	}
}

func TestDrillPathsAndChildEnvironmentRejectAmbiguity(t *testing.T) {
	for _, p := range []string{"relative/path", "/tmp/../other", "/tmp/has space", "/tmp/quote'", "/tmp/new\nline", "/tmp/"} {
		if err := drillPaths(p, "/tmp/bin"); err == nil {
			t.Fatal("ambiguous path accepted")
		}
	}
	if err := drillPaths("/tmp/owned", "/tmp/bin"); err != nil {
		t.Fatal(err)
	}
	env := drillCommandEnvironment([]string{"PATH=/safe", "PGHOSTADDR=192.0.2.1", "PGOPTIONS=unsafe", "PGPASSWORD=not-real-secret", "PGSERVICE=other", "AUTH_KEY=not-real-secret", "LOTTERY_RECOVERY_ORIGINAL_DATABASE_URL=not-real-secret", "DATABASE_URL=not-real-secret"})
	joined := strings.Join(env, ";")
	if strings.Contains(joined, "not-real-secret") || strings.Contains(joined, "192.0.2.1") || strings.Contains(joined, "PGOPTIONS") || !strings.Contains(joined, "PGCONNECT_TIMEOUT=5") || !strings.Contains(joined, "PATH=/safe") {
		t.Fatal("unsafe child environment")
	}
}
