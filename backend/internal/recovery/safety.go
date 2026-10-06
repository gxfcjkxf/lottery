//go:build !windows

package recovery

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

func drillPaths(root, bin string) error {
	for _, p := range []string{root, bin} {
		if !filepath.IsAbs(p) || filepath.Clean(p) != p || strings.ContainsAny(p, " \t\n\r'\"\\") {
			return fmt.Errorf("absolute unambiguous drill paths required")
		}
	}
	return nil
}

func drillBasePort(raw string) (int, error) {
	base := 55531
	if raw != "" {
		var err error
		base, err = strconv.Atoi(raw)
		if err != nil {
			return 0, fmt.Errorf("invalid drill base port")
		}
	}
	if base < 1024 || base > 65531 {
		return 0, fmt.Errorf("drill base port outside safe range")
	}
	for port := base; port < base+4; port++ {
		if port == 5432 || port == 55432 {
			return 0, fmt.Errorf("drill refuses standard and original development ports")
		}
	}
	return base, nil
}

func originalReadConfig(dsn string, base int) (*pgxpool.Config, error) {
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Hostname() != "127.0.0.1" || parsed.Port() == "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("explicit loopback baseline URL required")
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return nil, fmt.Errorf("invalid baseline URL options")
	}
	for key, values := range query {
		if key != "sslmode" || len(values) != 1 {
			return nil, fmt.Errorf("baseline URL permits only one sslmode option; no host/port overrides")
		}
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid baseline connection configuration")
	}
	safe := func(host string, port uint16) bool {
		return host == "127.0.0.1" && port > 0 && !(int(port) >= base && int(port) < base+4)
	}
	if !safe(cfg.ConnConfig.Host, cfg.ConnConfig.Port) {
		return nil, fmt.Errorf("baseline must not use an owned port or remote host")
	}
	for _, f := range cfg.ConnConfig.Fallbacks {
		if !safe(f.Host, f.Port) {
			return nil, fmt.Errorf("unsafe baseline fallback")
		}
	}
	cfg.MaxConns = 2
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	cfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	return cfg, nil
}

func drillCommandEnvironment(env []string) []string {
	out := make([]string, 0, len(env))
	for _, item := range env {
		key, _, _ := strings.Cut(item, "=")
		if strings.HasPrefix(key, "PG") || key == "AUTH_KEY" || key == "BOOTSTRAP_ADMIN_PASSWORD" || key == "DATABASE_URL" || key == "DATABASE_READ_URL" || key == "LOTTERY_RECOVERY_ORIGINAL_DATABASE_URL" {
			continue
		}
		out = append(out, item)
	}
	return append(out, "PGCONNECT_TIMEOUT=5")
}
