package identity

import (
	"context"
	"crypto/subtle"
	"fmt"
	"github.com/gxfcjkxf/lottery/backend/internal/authcrypto"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
	"regexp"
	"strconv"
	"time"
)

type Challenge struct {
	ID        string    `json:"id"`
	Puzzle    *Puzzle   `json:"puzzle,omitempty"`
	Nonce     string    `json:"nonce,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *Store) Challenge(ctx context.Context, e *mutation.Engine, brand, kind string) (Challenge, error) {
	var result Challenge
	var proof string
	result.ID = ids.New()
	result.ExpiresAt = time.Now().UTC().Add(5 * time.Minute)
	if kind == "captcha" {
		var err error
		result.Puzzle, proof, err = newPuzzle()
		if err != nil {
			return result, err
		}
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
	if _, err = tx.Exec(ctx, `INSERT INTO auth_challenges(id,brand_id,kind,proof_hash,expires_at) VALUES($1,NULLIF($2,'')::uuid,$3,$4,$5)`, result.ID, brand, kind, e.Fingerprint(result.ID+":"+proof), result.ExpiresAt); err != nil {
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

	var proof string
	err := s.DB.QueryRow(ctx, `UPDATE auth_challenges SET used_at=now() WHERE id=$1 AND brand_id IS NOT DISTINCT FROM NULLIF($2,'')::uuid AND kind=$3 AND used_at IS NULL AND expires_at>now() RETURNING proof_hash`, id, brand, kind).Scan(&proof)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	if kind == "captcha" {
		x, err := strconv.Atoi(answer)
		if err != nil || x < 0 || x > 300 {
			return false, nil
		}
		valid := 0
		// Four source-image pixels of tolerance for mouse and touch alignment.
		for offset := -4; offset <= 4; offset++ {
			valid |= subtle.ConstantTimeCompare([]byte(proof), []byte(e.Fingerprint(id+":"+strconv.Itoa(x+offset))))
		}
		return valid == 1, nil
	}
	return subtle.ConstantTimeCompare([]byte(proof), []byte(e.Fingerprint(id+":"+answer))) == 1, nil
}
