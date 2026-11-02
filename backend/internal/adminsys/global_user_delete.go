package adminsys

import (
	"context"
	"errors"

	"github.com/gxfcjkxf/lottery/backend/internal/access"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/jackc/pgx/v5"
)

type DeletedUser struct {
	ID          string `json:"id"`
	AccountName string `json:"account_name"`
	AuditLogID  string `json:"audit_log_id"`
}

// DeleteGlobalUser removes access to the selected brand. The login identity is
// retired only after its last membership is cancelled; history remains intact.
func (s Store) DeleteGlobalUser(ctx context.Context, tx pgx.Tx, actor access.Account, brandID, userID, requestID, ip string) (DeletedUser, error) {
	var result DeletedUser
	if tx == nil || brandID == "" || userID == "" {
		return result, ErrInvalid
	}
	if !actor.SuperAdmin || !access.Authorize(actor, "user", "delete", access.ScopePlatform, "") {
		return result, ErrDenied
	}
	memberID, status, err := cancelUserBrand(ctx, tx, brandID, userID)
	if err != nil {
		return result, err
	}
	if err = tx.QueryRow(ctx, `SELECT id::text,coalesce(username,phone,telegram_user_id) FROM global_users WHERE id=$1`, userID).Scan(&result.ID, &result.AccountName); err != nil {
		return result, err
	}
	result.AuditLogID, err = audit.Append(ctx, tx, audit.Record{BrandID: brandID, ActorType: "admin", ActorID: actor.ID, Action: "user.delete.platform", ResourceType: "global_user", ResourceID: userID, RequestID: requestID, IP: ip, Before: map[string]string{"status": status}, After: map[string]string{"status": "cancelled", "member_id": memberID}})
	return result, err
}

// Lock the identity before the membership so simultaneous deletions in different
// brands cannot both leave an orphaned login identity.
func cancelUserBrand(ctx context.Context, tx pgx.Tx, brandID, userID string) (string, string, error) {
	var status, memberID string
	err := tx.QueryRow(ctx, `SELECT status FROM global_users WHERE id=$1 FOR UPDATE`, userID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && status == "deleted" {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", err
	}
	err = tx.QueryRow(ctx, `SELECT id::text,status FROM brand_members WHERE brand_id=$1 AND global_user_id=$2 FOR UPDATE`, brandID, userID).Scan(&memberID, &status)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && status == "cancelled" {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", err
	}
	if _, err = tx.Exec(ctx, `UPDATE brand_members SET status='cancelled' WHERE id=$1`, memberID); err != nil {
		return "", "", err
	}
	if _, err = tx.Exec(ctx, `UPDATE sessions SET revoked_at=now() WHERE brand_id=$1 AND user_id=$2 AND revoked_at IS NULL`, brandID, userID); err != nil {
		return "", "", err
	}
	// Suspended memberships still belong to the account and must be preserved.
	if _, err = tx.Exec(ctx, `UPDATE global_users SET status='deleted',password_hash=NULL,updated_at=now()
		WHERE id=$1 AND NOT EXISTS(SELECT 1 FROM brand_members WHERE global_user_id=$1 AND status<>'cancelled')`, userID); err != nil {
		return "", "", err
	}
	return memberID, status, nil
}
