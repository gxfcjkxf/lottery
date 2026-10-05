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
