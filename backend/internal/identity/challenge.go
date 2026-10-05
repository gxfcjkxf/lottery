package identity

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"fmt"
	"github.com/gxfcjkxf/lottery/backend/internal/authcrypto"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
	"regexp"
	"strings"
	"time"
)

type Challenge struct {
	ID        string    `json:"id"`
	SVG       string    `json:"svg,omitempty"`
	Nonce     string    `json:"nonce,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *Store) Challenge(ctx context.Context, e *mutation.Engine, brand, kind string) (Challenge, error) {
	var result Challenge
	var proof string
	result.ID = ids.New()
	result.ExpiresAt = time.Now().UTC().Add(5 * time.Minute)
	if kind == "captcha" {
		alphabet := "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
		seed := make([]byte, 6)
		if _, err := rand.Read(seed); err != nil {
			return result, err
		}
		for i := range seed {
			seed[i] = alphabet[int(seed[i])%len(alphabet)]
		}
		proof = string(seed)
		result.SVG = fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="220" height="70" viewBox="0 0 220 70"><rect width="220" height="70" rx="8" fill="#eef1f4"/><path d="M5 20L215 52M20 65L180 8M0 40L220 25" stroke="#9ab5b0"/><text x="16" y="47" font-family="monospace" font-weight="bold" font-size="32" letter-spacing="5" fill="#243c47">%s</text></svg>`, proof)
	} else if kind == "telegram" {
		token, err := authcrypto.NewSessionToken()
		if err != nil {
			return result, err
		}
		proof = token
		result.Nonce = token
	} else {
		return result, fmt.Errorf("invalid challenge kind")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "DELETE FROM auth_challenges WHERE expires_at<now()"); err != nil {
		return result, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO auth_challenges(id,brand_id,kind,proof_hash,expires_at) VALUES($1,$2,$3,$4,$5)`, result.ID, brand, kind, e.Fingerprint(result.ID+":"+proof), result.ExpiresAt); err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}

// Consume commits independently so a failed password cannot reuse a challenge.
// Execute invokes it only on the first logical request, never during idempotent replay.
func (s *Store) Consume(ctx context.Context, e *mutation.Engine, brand, kind, id, answer string) (bool, error) {
	if !regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`).MatchString(id) || len(answer) > 128 {
		return false, nil
	}
	if kind == "captcha" {
		answer = strings.ToUpper(strings.TrimSpace(answer))
	}
	var proof string
	err := s.DB.QueryRow(ctx, `UPDATE auth_challenges SET used_at=now() WHERE id=$1 AND brand_id=$2 AND kind=$3 AND used_at IS NULL AND expires_at>now() RETURNING proof_hash`, id, brand, kind).Scan(&proof)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return subtle.ConstantTimeCompare([]byte(proof), []byte(e.Fingerprint(id+":"+answer))) == 1, nil
}
