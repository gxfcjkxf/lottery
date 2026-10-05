// Package authcrypto contains password and session-token primitives.
package authcrypto

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	passwordMinBytes = 10
	passwordMaxBytes = 128

	argonVersion     = 19
	argonMemoryKiB   = 64 * 1024
	argonIterations  = 3
	argonParallelism = 2
	argonSaltBytes   = 16
	argonKeyBytes    = 32
	maxEncodedHash   = 512
	maxMemoryKiB     = argonMemoryKiB
	maxIterations    = argonIterations
	maxParallelism   = 4
	minSaltBytes     = 16
	maxSaltBytes     = 32
	minKeyBytes      = 16
	maxKeyBytes      = 64
)

var (
	ErrInvalidPassword = errors.New("password must be between 10 and 128 bytes")
	ErrInvalidHash     = errors.New("invalid password hash")
)

// HashPassword creates a versioned Argon2id password hash using bounded,
// intentionally expensive parameters and a fresh cryptographic salt.
func HashPassword(password string) (string, error) {
	if !validPassword(password) {
		return "", ErrInvalidPassword
	}
	salt := make([]byte, argonSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonIterations, argonMemoryKiB, argonParallelism, argonKeyBytes)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argonVersion, argonMemoryKiB, argonIterations, argonParallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword checks a password against a bounded Argon2id PHC string.
// Malformed or over-budget hashes return ErrInvalidHash without running Argon2.
func VerifyPassword(password, encodedHash string) (bool, error) {
	if !validPassword(password) {
		return false, ErrInvalidPassword
	}
	parsed, err := parseHash(encodedHash)
	if err != nil {
		return false, ErrInvalidHash
	}
	actual := argon2.IDKey([]byte(password), parsed.salt, parsed.iterations, parsed.memoryKiB, parsed.parallelism, uint32(len(parsed.key)))
	return subtle.ConstantTimeCompare(actual, parsed.key) == 1, nil
}

func validPassword(password string) bool {
	return len([]byte(password)) >= passwordMinBytes && len([]byte(password)) <= passwordMaxBytes
}

type parsedHash struct {
	memoryKiB   uint32
	iterations  uint32
	parallelism uint8
	salt        []byte
	key         []byte
}

func parseHash(encoded string) (parsedHash, error) {
	if len(encoded) > maxEncodedHash {
		return parsedHash{}, ErrInvalidHash
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" {
		return parsedHash{}, ErrInvalidHash
	}

	params := strings.Split(parts[3], ",")
	if len(params) != 3 {
		return parsedHash{}, ErrInvalidHash
	}
	memory, okM := parseUintParam(params[0], "m=")
	iterations, okT := parseUintParam(params[1], "t=")
	parallelism, okP := parseUintParam(params[2], "p=")
	if !okM || !okT || !okP || memory == 0 || memory > maxMemoryKiB ||
		iterations == 0 || iterations > maxIterations ||
		parallelism == 0 || parallelism > maxParallelism ||
		memory < 8*parallelism {
		return parsedHash{}, ErrInvalidHash
	}

	salt, errSalt := base64.RawStdEncoding.DecodeString(parts[4])
	key, errKey := base64.RawStdEncoding.DecodeString(parts[5])
	if errSalt != nil || errKey != nil || len(salt) < minSaltBytes || len(salt) > maxSaltBytes ||
		len(key) < minKeyBytes || len(key) > maxKeyBytes {
		return parsedHash{}, ErrInvalidHash
	}
	return parsedHash{
		memoryKiB: memory, iterations: iterations, parallelism: uint8(parallelism), salt: salt, key: key,
	}, nil
}

func parseUintParam(value, prefix string) (uint32, bool) {
	if !strings.HasPrefix(value, prefix) || len(value) == len(prefix) {
		return 0, false
	}
	n, err := strconv.ParseUint(strings.TrimPrefix(value, prefix), 10, 32)
	return uint32(n), err == nil
}
