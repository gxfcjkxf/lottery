package authcrypto

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
)

func TestHashAndVerifyPassword(t *testing.T) {
	password := "correct horse battery staple"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Fatalf("hash is not versioned Argon2id: %q", hash)
	}
	ok, err := VerifyPassword(password, hash)
	if err != nil || !ok {
		t.Fatalf("valid password rejected: ok=%v err=%v", ok, err)
	}
	ok, err = VerifyPassword("wrong horse battery staple", hash)
	if err != nil || ok {
		t.Fatalf("wrong password accepted: ok=%v err=%v", ok, err)
	}
	second, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if second == hash {
		t.Fatal("same password produced the same salted hash")
	}
}

func TestPasswordByteBounds(t *testing.T) {
	for _, password := range []string{"12345", strings.Repeat("x", 31)} {
		if _, err := HashPassword(password); err != ErrInvalidPassword {
			t.Errorf("HashPassword(%d bytes) err=%v, want ErrInvalidPassword", len(password), err)
		}
	}
	if _, err := HashPassword("123456"); err != nil {
		t.Fatalf("minimum length password rejected: %v", err)
	}
	if _, err := HashPassword(strings.Repeat("x", 30)); err != nil {
		t.Fatalf("maximum length password rejected: %v", err)
	}
	if ok, err := VerifyPassword("12345", "$invalid"); ok || err != ErrInvalidPassword {
		t.Fatalf("VerifyPassword did not enforce password bounds: ok=%v err=%v", ok, err)
	}
}

func TestPasswordBoundsApplyToDefaultAdminPassword(t *testing.T) {
	if _, err := HashPassword("short"); err != ErrInvalidPassword {
		t.Fatalf("ordinary HashPassword accepted too-short password: %v", err)
	}
	if _, err := VerifyPassword("short", "$invalid"); err != ErrInvalidPassword {
		t.Fatalf("ordinary VerifyPassword accepted too-short password: %v", err)
	}
	hash, err := HashPassword("admin123")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Fatalf("development hash is not Argon2id: %q", hash)
	}
	if ok, err := VerifyPassword("admin123", hash); err != nil || !ok {
		t.Fatalf("default admin password verification failed: ok=%v err=%v", ok, err)
	}
	if ok, err := VerifyPassword("admin123", "$invalid"); err != ErrInvalidHash || ok {
		t.Fatalf("malformed password hash accepted: ok=%v err=%v", ok, err)
	}
}

func TestVerifyRejectsMalformedAndOverBudgetHashes(t *testing.T) {
	validSalt := base64.RawStdEncoding.EncodeToString(make([]byte, argonSaltBytes))
	validKey := base64.RawStdEncoding.EncodeToString(make([]byte, argonKeyBytes))
	tests := []string{
		"",
		"$argon2i$v=19$m=65536,t=3,p=2$" + validSalt + "$" + validKey,
		"$argon2id$v=18$m=65536,t=3,p=2$" + validSalt + "$" + validKey,
		"$argon2id$v=19$m=4294967295,t=3,p=2$" + validSalt + "$" + validKey,
		"$argon2id$v=19$m=65536,t=4294967295,p=2$" + validSalt + "$" + validKey,
		"$argon2id$v=19$m=65536,t=3,p=255$" + validSalt + "$" + validKey,
		"$argon2id$v=19$m=0,t=3,p=2$" + validSalt + "$" + validKey,
		"$argon2id$v=19$m=65536,t=3,p=2$%%%$" + validKey,
		strings.Repeat("$", maxEncodedHash+1),
		fmt.Sprintf("$argon2id$v=19$m=%d,t=3,p=2$%s$%s", maxMemoryKiB+1, validSalt, validKey),
		fmt.Sprintf("$argon2id$v=19$m=65536,t=%d,p=2$%s$%s", maxIterations+1, validSalt, validKey),
		fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=%d$%s$%s", maxParallelism+1, validSalt, validKey),
	}
	for _, hash := range tests {
		t.Run(hash, func(t *testing.T) {
			if ok, err := VerifyPassword("correct horse battery staple", hash); ok || err != ErrInvalidHash {
				t.Fatalf("malformed or unbounded hash result: ok=%v err=%v", ok, err)
			}
		})
	}
}
