package identity

import (
	"context"
	"encoding/hex"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/authcrypto"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
	"strings"
	"time"
)

type AgentSession struct {
	ID        string `json:"-"`
	AccountID string `json:"agent_account_id"`
	BrandID   string `json:"brand_id"`
	Username  string `json:"username"`
}
type AgentLoginInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}
type AgentPasswordInput struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (s *Store) AgentChangePassword(ctx context.Context, tx pgx.Tx, session AgentSession, in AgentPasswordInput, meta Metadata) (mutation.Result, error) {
	var stored string
	if err := tx.QueryRow(ctx, `SELECT password_hash FROM agent_accounts WHERE brand_id=$1 AND id=$2 FOR UPDATE`, session.BrandID, session.AccountID).Scan(&stored); err != nil {
		return mutation.Result{}, err
	}
	valid, err := s.verify(ctx, in.CurrentPassword, stored)
	if errors.Is(err, ErrBusy) {
		return mutation.Fail(429, "AUTH_BUSY", "登录繁忙，请稍后重试"), nil
	}
	if err != nil || !valid {
		return mutation.Fail(400, "AUTH_INVALID_CREDENTIALS", "当前密码不正确"), nil
	}
	hash, err := s.PasswordHash(ctx, in.NewPassword)
	if errors.Is(err, ErrBusy) {
		return mutation.Fail(429, "AUTH_BUSY", "登录繁忙，请稍后重试"), nil
	}
	if errors.Is(err, authcrypto.ErrInvalidPassword) {
		return mutation.Fail(400, "PASSWORD_INVALID", "密码须为 6-30 字节"), nil
	}
	if err != nil {
		return mutation.Result{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE agent_accounts SET password_hash=$3 WHERE brand_id=$1 AND id=$2`, session.BrandID, session.AccountID, hash); err != nil {
		return mutation.Result{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE agent_sessions SET revoked_at=clock_timestamp() WHERE brand_id=$1 AND agent_account_id=$2 AND revoked_at IS NULL`, session.BrandID, session.AccountID); err != nil {
		return mutation.Result{}, err
	}
	id, err := audit.Append(ctx, tx, audit.Record{BrandID: session.BrandID, ActorType: "agent", ActorID: session.AccountID, Action: "agent.password_change", ResourceType: "agent_account", ResourceID: session.AccountID, RequestID: meta.RequestID, IP: meta.IP, Before: map[string]string{"password": "redacted"}, After: map[string]string{"password": "redacted"}})
	if err != nil {
		return mutation.Result{}, err
	}
	return mutation.OK(200, map[string]string{"audit_log_id": id}), nil
}

type AgentAuthentication struct {
	AgentSession
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func (s *Store) AgentLogin(ctx context.Context, tx pgx.Tx, brand string, in AgentLoginInput) (mutation.Result, error) {
	var v AgentSession
	var hash string
	err := tx.QueryRow(ctx, `SELECT id::text,brand_id::text,username,password_hash FROM agent_accounts WHERE brand_id=$1 AND username=$2 FOR UPDATE`, brand, strings.ToLower(strings.TrimSpace(in.Username))).Scan(&v.AccountID, &v.BrandID, &v.Username, &hash)
	found := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return mutation.Result{}, err
	}
	if !found {
		hash = s.dummyHash
	}
	valid, err := s.verify(ctx, in.Password, hash)
	if err == ErrBusy {
		return mutation.Fail(429, "AUTH_BUSY", "登录繁忙，请稍后重试"), nil
	}
	if !found || !valid || err != nil {
		return mutation.Fail(401, "AUTH_INVALID_CREDENTIALS", "账号或密码不正确"), nil
	}
	var active bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_nodes n JOIN brands b ON b.id=n.brand_id WHERE n.brand_id=$1 AND n.member_id=$2 AND b.status<>'disabled' AND NOT EXISTS(SELECT 1 FROM agent_nodes a WHERE a.brand_id=n.brand_id AND a.id=ANY(n.path) AND a.config->>'status'<>'active'))`, brand, v.AccountID).Scan(&active); err != nil {
		return mutation.Result{}, err
	}
	if !active {
		return mutation.Fail(403, "AGENT_DISABLED", "代理账号已停用"), nil
	}
	token, err := authcrypto.NewSessionToken()
	if err != nil {
		return mutation.Result{}, err
	}
	digest := authcrypto.DigestSessionToken(token)
	v.ID = ids.New()
	expires := time.Now().Add(s.TTL)
	if _, err = tx.Exec(ctx, `UPDATE agent_sessions SET revoked_at=clock_timestamp() WHERE brand_id=$1 AND agent_account_id=$2 AND revoked_at IS NULL`, brand, v.AccountID); err != nil {
		return mutation.Result{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO agent_sessions(id,brand_id,agent_account_id,token_hash,expires_at) VALUES($1,$2,$3,decode($4,'hex'),$5)`, v.ID, brand, v.AccountID, hex.EncodeToString(digest[:]), expires); err != nil {
		return mutation.Result{}, err
	}
	return mutation.OK(200, AgentAuthentication{v, token, expires}), nil
}
func (s *Store) AgentAuthenticateTx(ctx context.Context, tx pgx.Tx, brand, token string) (AgentSession, error) {
	var v AgentSession
	if len(token) != 43 {
		return v, ErrSession
	}
	digest := authcrypto.DigestSessionToken(token)
	err := tx.QueryRow(ctx, `SELECT s.id::text,a.id::text,a.brand_id::text,a.username FROM agent_sessions s JOIN agent_accounts a ON a.id=s.agent_account_id AND a.brand_id=s.brand_id JOIN agent_nodes n ON n.member_id=a.id AND n.brand_id=a.brand_id JOIN brands b ON b.id=a.brand_id WHERE s.brand_id=$1 AND s.token_hash=decode($2,'hex') AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND b.status<>'disabled' AND NOT EXISTS(SELECT 1 FROM agent_nodes p WHERE p.brand_id=n.brand_id AND p.id=ANY(n.path) AND p.config->>'status'<>'active') FOR UPDATE OF s`, brand, hex.EncodeToString(digest[:])).Scan(&v.ID, &v.AccountID, &v.BrandID, &v.Username)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrSession
	}
	return v, err
}
