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

// DeleteGlobalUser deletes the login identity across all brands. Financial and
// historical rows retain their foreign keys to the deleted identity.
func (s Store) DeleteGlobalUser(ctx context.Context, tx pgx.Tx, actor access.Account, userID, requestID, ip string) (DeletedUser, error) {
	var result DeletedUser
	if tx == nil || userID == "" {
		return result, ErrInvalid
	}
	if !actor.SuperAdmin || !access.Authorize(actor, "user", "delete", access.ScopePlatform, "") {
		return result, ErrDenied
	}
	var status string
	err := tx.QueryRow(ctx, `SELECT id::text,coalesce(username,phone,telegram_user_id),status FROM global_users WHERE id=$1 FOR UPDATE`, userID).Scan(&result.ID, &result.AccountName, &status)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && status == "deleted" {
		return result, ErrNotFound
	}
	if err != nil {
		return result, err
	}
	if _, err = tx.Exec(ctx, `UPDATE global_users SET status='deleted',password_hash=NULL,updated_at=now() WHERE id=$1`, userID); err != nil {
		return result, err
	}
	if _, err = tx.Exec(ctx, `UPDATE brand_members SET status='cancelled' WHERE global_user_id=$1 AND status<>'cancelled'`, userID); err != nil {
		return result, err
	}
	if _, err = tx.Exec(ctx, `UPDATE sessions SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, userID); err != nil {
		return result, err
	}
	result.AuditLogID, err = audit.Append(ctx, tx, audit.Record{ActorType: "admin", ActorID: actor.ID, Action: "user.delete.platform", ResourceType: "global_user", ResourceID: userID, RequestID: requestID, IP: ip, Before: map[string]string{"status": status}, After: map[string]string{"status": "deleted"}})
	return result, err
}
