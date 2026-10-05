package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Environment     string
	HTTPAddr        string
	DatabaseURL     string
	DatabaseReadURL string
	DBMaxConns      int32
}

func Load() (Config, error) {
	c := Config{Environment: value("APP_ENV", "development"), HTTPAddr: value("HTTP_ADDR", "127.0.0.1:8080"), DatabaseURL: os.Getenv("DATABASE_URL"), DatabaseReadURL: os.Getenv("DATABASE_READ_URL"), DBMaxConns: 20}
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
func value(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
