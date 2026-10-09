package identity

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/authcrypto"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
	"strings"
	"time"
)

type AdminAuthentication struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func (s *Store) AdminLogin(ctx context.Context, tx pgx.Tx, brand string, in LoginInput, meta Metadata, platform bool) (mutation.Result, error) {
	var id, hash, status string
	var super bool
	err := tx.QueryRow(ctx, `SELECT id::text,password_hash,status,is_super_admin FROM admin_accounts WHERE username=$1 FOR UPDATE`, strings.ToLower(strings.TrimSpace(in.Identifier))).Scan(&id, &hash, &status, &super)
	found := err == nil
	if err != nil && err != pgx.ErrNoRows {
		return mutation.Result{}, err
	}
	if hash == "" {
		hash = s.dummyHash
	}
	valid, err := s.verify(ctx, in.Password, hash)
	if err == ErrBusy {
		return mutation.Fail(429, "AUTH_BUSY", "登录繁忙，请稍后重试"), nil
	}
	if !found || !valid || err != nil || status != "active" {
		return mutation.Fail(401, "AUTH_INVALID_CREDENTIALS", "账号或密码不正确"), nil
	}
	if super != platform {
		return mutation.Fail(403, "ADMIN_ENTRY_MISMATCH", "账号类型与管理入口不匹配"), nil
	}
	token, err := authcrypto.NewSessionToken()
	if err != nil {
		return mutation.Result{}, err
	}
	expiry := time.Now().UTC().Add(12 * time.Hour)
	if _, err = tx.Exec(ctx, `INSERT INTO sessions(id,token_hash,admin_id,expires_at) VALUES($1,$2,$3,$4)`, ids.New(), tokenHash(token), id, expiry); err != nil {
		return mutation.Result{}, err
	}
	if _, err = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: id, Action: "admin.login", ResourceType: "admin", ResourceID: id, RequestID: meta.RequestID, IP: meta.IP}); err != nil {
		return mutation.Result{}, err
	}
	return mutation.OK(200, AdminAuthentication{token, "Bearer", expiry}), nil
}
func (s *Store) AdminAuthenticate(ctx context.Context, token string) (string, error) {
	if len(token) != 43 {
		return "", ErrSession
	}
	var id string
	err := s.DB.QueryRow(ctx, `SELECT a.id::text FROM sessions s JOIN admin_accounts a ON a.id=s.admin_id WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>now() AND a.status='active'`, tokenHash(token)).Scan(&id)
	if err == pgx.ErrNoRows {
		return "", ErrSession
	}
	return id, err
}
func (s *Store) AdminLogout(ctx context.Context, tx pgx.Tx, brand, token string, meta Metadata, platform bool) (mutation.Result, error) {
	var id, admin string
	var super bool
	err := tx.QueryRow(ctx, `SELECT s.id::text,s.admin_id::text,a.is_super_admin FROM sessions s JOIN admin_accounts a ON a.id=s.admin_id WHERE s.token_hash=$1 FOR UPDATE OF s`, tokenHash(token)).Scan(&id, &admin, &super)
	if err == pgx.ErrNoRows {
		return mutation.Fail(401, "AUTH_SESSION_REVOKED", "会话不可用"), nil
	}
	if err != nil {
		return mutation.Result{}, err
	}
	if super != platform {
		return mutation.Fail(403, "ADMIN_ENTRY_MISMATCH", "账号类型与管理入口不匹配"), nil
	}
	if _, err = tx.Exec(ctx, "UPDATE sessions SET revoked_at=coalesce(revoked_at,now()) WHERE id=$1", id); err != nil {
		return mutation.Result{}, err
	}
	auditID, err := audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "admin", ActorID: admin, Action: "admin.logout", ResourceType: "admin", ResourceID: admin, RequestID: meta.RequestID, IP: meta.IP})
	return mutation.OK(200, map[string]string{"audit_log_id": auditID}), err
}
