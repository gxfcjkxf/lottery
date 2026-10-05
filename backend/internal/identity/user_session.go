package identity

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
)

// AuthenticateTx revalidates a user session inside the caller's transaction.
// It takes locks in the same order as login and member administration so the
// authorization decision cannot become stale while the transaction proceeds.
func (s *Store) AuthenticateTx(ctx context.Context, tx pgx.Tx, brand, token string) (Session, error) {
	var v Session
	if tx == nil || len(token) != 43 {
		return v, ErrSession
	}

	// Resolve the rows without locking first. Cast the stored UUID to text so a
	// malformed caller supplied brand never reaches PostgreSQL's UUID parser.
	var userID, memberID, brandID string
	err := tx.QueryRow(ctx, `SELECT user_id::text,member_id::text,brand_id::text
		FROM sessions
		WHERE token_hash=$1 AND admin_id IS NULL AND brand_id::text=lower($2)`, tokenHash(token), brand).
		Scan(&userID, &memberID, &brandID)
	if err == pgx.ErrNoRows {
		return v, ErrSession
	}
	if err != nil {
		return v, err
	}

	var lockedBrandID, brandStatus string
	err = tx.QueryRow(ctx, `SELECT id::text,status FROM brands WHERE id::text=$1 FOR SHARE`, brandID).
		Scan(&lockedBrandID, &brandStatus)
	if err == pgx.ErrNoRows {
		return v, ErrSession
	}
	if err != nil {
		return v, err
	}
	if lockedBrandID != brandID || lockedBrandID != lowerUUID(brand) || brandStatus == "disabled" {
		return v, ErrSession
	}

	err = tx.QueryRow(ctx, `SELECT id::text,coalesce(username,''),coalesce(phone,''),
		coalesce(telegram_user_id,''),status
		FROM global_users WHERE id::text=$1 FOR SHARE`, userID).
		Scan(&v.User.ID, &v.User.Username, &v.User.Phone, &v.User.TelegramID, &v.User.Status)
	if err == pgx.ErrNoRows {
		return v, ErrSession
	}
	if err != nil {
		return v, err
	}
	if v.User.ID != userID || v.User.Status != "active" {
		return v, ErrSession
	}

	var memberUserID string
	var termsAccepted bool
	err = tx.QueryRow(ctx, `SELECT id::text,brand_id::text,global_user_id::text,status,
		display_name,joined_at,terms_accepted
		FROM brand_members WHERE id::text=$1 FOR SHARE`, memberID).
		Scan(&v.Member.ID, &v.Member.BrandID, &memberUserID, &v.Member.Status,
			&v.Member.DisplayName, &v.Member.JoinedAt, &termsAccepted)
	if err == pgx.ErrNoRows {
		return v, ErrSession
	}
	if err != nil {
		return v, err
	}
	if v.Member.ID != memberID || v.Member.BrandID != brandID || memberUserID != userID ||
		(v.Member.Status != "normal" && v.Member.Status != "frozen") || !termsAccepted {
		return v, ErrSession
	}

	// Lock and reread the session last. In particular, evaluate expiry against
	// the wall clock after any waits for the brand, user, or member locks.
	var sessionUserID, sessionMemberID, sessionBrandID string
	var usable bool
	err = tx.QueryRow(ctx, `SELECT id::text,user_id::text,member_id::text,brand_id::text,
		revoked_at IS NULL AND expires_at>clock_timestamp()
		FROM sessions
		WHERE token_hash=$1 AND admin_id IS NULL
		FOR SHARE`, tokenHash(token)).
		Scan(&v.ID, &sessionUserID, &sessionMemberID, &sessionBrandID, &usable)
	if err == pgx.ErrNoRows {
		return Session{}, ErrSession
	}
	if err != nil {
		return Session{}, err
	}
	if !usable || sessionUserID != userID || sessionMemberID != memberID || sessionBrandID != brandID {
		return Session{}, ErrSession
	}
	return v, nil
}

func lowerUUID(value string) string {
	// UUID text stored by PostgreSQL is canonical lowercase; lowercasing here
	// keeps valid uppercase UUID input equivalent without casting user input.
	return strings.ToLower(value)
}
