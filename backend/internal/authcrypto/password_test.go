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
	for _, password := range []string{"123456789", strings.Repeat("x", 129)} {
		if _, err := HashPassword(password); err != ErrInvalidPassword {
			t.Errorf("HashPassword(%d bytes) err=%v, want ErrInvalidPassword", len(password), err)
		}
	}
	if _, err := HashPassword("1234567890"); err != nil {
		t.Fatalf("minimum length password rejected: %v", err)
	}
	if _, err := HashPassword(strings.Repeat("x", 128)); err != nil {
		t.Fatalf("maximum length password rejected: %v", err)
	}
	if ok, err := VerifyPassword("123456789", "$invalid"); ok || err != ErrInvalidPassword {
		t.Fatalf("VerifyPassword did not enforce password bounds: ok=%v err=%v", ok, err)
	}
}

func TestDevelopmentAdminPasswordUsesArgon2WithoutChangingNormalBounds(t *testing.T) {
	if _, err := HashPassword("admin123"); err != ErrInvalidPassword {
		t.Fatalf("ordinary HashPassword accepted short password: %v", err)
	}
	if _, err := VerifyPassword("admin123", "$invalid"); err != ErrInvalidPassword {
		t.Fatalf("ordinary VerifyPassword accepted short password: %v", err)
	}
	hash, err := HashDevelopmentAdminPassword()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Fatalf("development hash is not Argon2id: %q", hash)
	}
	if ok, err := VerifyDevelopmentAdminPassword(hash); err != nil || !ok {
		t.Fatalf("development password verification failed: ok=%v err=%v", ok, err)
	}
	if ok, err := VerifyDevelopmentAdminPassword("$invalid"); err != ErrInvalidHash || ok {
		t.Fatalf("malformed development hash accepted: ok=%v err=%v", ok, err)
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
