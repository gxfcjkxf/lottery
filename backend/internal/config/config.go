package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	Environment      string
	HTTPAddr         string
	DatabaseURL      string
	DatabaseReadURLs []string
	DBMaxConns       int32
	AuthKeyFile      string
	AuthKey          string
	TrustedProxies   []*net.IPNet
}

func Load() (Config, error) {
	c := Config{Environment: value("APP_ENV", "development"), HTTPAddr: value("HTTP_ADDR", "127.0.0.1:8080"), DatabaseURL: os.Getenv("DATABASE_URL"), DBMaxConns: 20}
	if err := loadDatabaseReadURLs(&c); err != nil {
		return c, err
	}
	c.AuthKeyFile = os.Getenv("AUTH_KEY_FILE")
	c.AuthKey = os.Getenv("AUTH_KEY")
	for _, raw := range strings.Split(os.Getenv("TRUSTED_PROXY_CIDRS"), ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		_, network, err := net.ParseCIDR(raw)
		if err != nil {
			return c, fmt.Errorf("invalid TRUSTED_PROXY_CIDRS")
		}
		if network.String() == "0.0.0.0/0" || network.String() == "::/0" {
			return c, fmt.Errorf("TRUSTED_PROXY_CIDRS must not trust every address")
		}
		c.TrustedProxies = append(c.TrustedProxies, network)
	}
	if c.Environment != "development" && c.Environment != "test" && c.Environment != "production" {
		return c, fmt.Errorf("APP_ENV must be development, test or production")
	}
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL is required")
	}
	if raw := os.Getenv("DB_MAX_CONNS"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || n < 1 || n > 500 {
			return c, fmt.Errorf("DB_MAX_CONNS must be between 1 and 500")
		}
		c.DBMaxConns = int32(n)
	}
	return c, nil
}

func loadDatabaseReadURLs(c *Config) error {
	encoded := strings.TrimSpace(os.Getenv("DATABASE_READ_URLS"))
	if os.Getenv("DATABASE_READ_URL") != "" {
		return fmt.Errorf("DATABASE_READ_URL is unsupported; use DATABASE_READ_URLS")
	}
	if encoded == "" {
		return nil
	}
	var urls []string
	if err := json.Unmarshal([]byte(encoded), &urls); err != nil || urls == nil {
		return fmt.Errorf("DATABASE_READ_URLS must be a JSON array of PostgreSQL URLs")
	}
	if len(urls) > 8 {
		return fmt.Errorf("DATABASE_READ_URLS must contain at most 8 URLs")
	}
	seen := make(map[string]struct{}, len(urls))
	for i, raw := range urls {
		url := strings.TrimSpace(raw)
		if url == "" {
			return fmt.Errorf("DATABASE_READ_URLS entries must not be empty")
		}
		if _, ok := seen[url]; ok {
			return fmt.Errorf("DATABASE_READ_URLS must not contain duplicates")
		}
		if _, err := pgxpool.ParseConfig(url); err != nil {
			return fmt.Errorf("invalid DATABASE_READ_URLS entry %d", i+1)
		}
		seen[url] = struct{}{}
		urls[i] = url
	}
	c.DatabaseReadURLs = urls
	return nil
}

func value(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
