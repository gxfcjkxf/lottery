package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/gxfcjkxf/lottery/backend/internal/config"
)

func TestGenerateMetricsTokenPrivateSeparateAndNeverOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.token")
	if err := generateMetricsToken(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("token must be private", err)
	}
	before, err := os.ReadFile(path)
	if err != nil || !regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`).Match(before) {
		t.Fatal("bad bearer token", err)
	}
	if err := generateMetricsToken(path); err == nil {
		t.Fatal("existing metrics secret overwritten")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("existing secret changed", err)
	}
	// Independent entropy, not another view of the auth encryption key.
	authPath := filepath.Join(filepath.Dir(path), "auth.key")
	if err := generateKey(authPath); err != nil {
		t.Fatal(err)
	}
	auth, err := os.ReadFile(authPath)
	if err != nil || bytes.Equal(auth, before) {
		t.Fatal("auth and metrics credentials reused", err)
	}
}

func TestGenerateAuthKeyPrivateAndNeverOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "auth.key")
	if err := generateKey(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("key must have private permissions: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	key, err := loadKey(config.Config{AuthKeyFile: path})
	if err != nil || len(key) != 32 {
		t.Fatalf("generated key is not usable: %v", err)
	}
	if err := generateKey(path); err == nil {
		t.Fatal("generating an existing key must fail, not replace encryption material")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("existing key changed")
	}
}

func TestLoadAuthKeyRejectsInvalidOrMissingMaterial(t *testing.T) {
	for _, cfg := range []config.Config{
		{}, {AuthKey: "not-base64"}, {AuthKey: "c2hvcnQ"},
		{AuthKeyFile: filepath.Join(t.TempDir(), "missing.key")},
	} {
		if _, err := loadKey(cfg); err == nil {
			t.Fatal("invalid or missing authentication key accepted")
		}
	}
}
