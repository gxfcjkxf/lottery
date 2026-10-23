package identity

import "context"

func (s *Store) SessionReplaced(ctx context.Context, token, brand string, admin bool) (bool, error) {
	if len(token) != 43 {
		return false, nil
	}
	var replaced bool
	err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sessions WHERE token_hash=$1
        AND revoked_at IS NOT NULL AND replaced_by_login
        AND (($3 AND admin_id IS NOT NULL) OR (NOT $3 AND admin_id IS NULL AND brand_id::text=$2)))`, tokenHash(token), brand, admin).Scan(&replaced)
	return replaced, err
}
