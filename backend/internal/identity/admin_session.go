package identity

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// AdminAuthenticateTx shares the authorization transaction after the ACL gate.
// A role/session revocation cannot be bypassed using the preflight snapshot.
func (s *Store) AdminAuthenticateTx(ctx context.Context, tx pgx.Tx, token string) (string, error) {
	if tx == nil || len(token) != 43 {
		return "", ErrSession
	}
	var id string
	err := tx.QueryRow(ctx, `SELECT a.id::text FROM sessions s JOIN admin_accounts a ON a.id=s.admin_id
 WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND a.status='active'`, tokenHash(token)).Scan(&id)
	if err == pgx.ErrNoRows {
		return "", ErrSession
	}
	return id, err
}
