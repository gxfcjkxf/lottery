package identity

import (
	"context"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/telegramauth"
	"github.com/jackc/pgx/v5"
)

type TelegramInput struct {
	IDToken     string `json:"id_token"`
	ChallengeID string `json:"challenge_id"`
	Nonce       string `json:"nonce"`
	Privacy     string `json:"privacy_policy_version"`
	Terms       string `json:"service_terms_version"`
	Bind        bool   `json:"bind,omitempty"`
}

func (s *Store) Telegram(ctx context.Context, tx pgx.Tx, brand string, claims telegramauth.Claims, in TelegramInput, bindUser string, meta Metadata) (mutation.Result, error) {
	cfg, err := settings(ctx, tx, brand)
	if err != nil {
		return mutation.Result{}, err
	}
	if !cfg.TelegramEnabled {
		return mutation.Fail(403, "TELEGRAM_DISABLED", "当前品牌未启用 Telegram 登录"), nil
	}
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "telegram:"+claims.ID); err != nil {
		return mutation.Result{}, err
	}
	var u User
	err = tx.QueryRow(ctx, `SELECT id::text,coalesce(username,''),coalesce(phone,''),telegram_user_id,status FROM global_users WHERE telegram_user_id=$1 FOR UPDATE`, claims.ID).Scan(&u.ID, &u.Username, &u.Phone, &u.TelegramID, &u.Status)
	if err != nil && err != pgx.ErrNoRows {
		return mutation.Result{}, err
	}
	if bindUser != "" {
		if err == nil && u.ID != bindUser {
			return mutation.Fail(409, "TELEGRAM_ALREADY_BOUND", "Telegram 已属于其他账号"), nil
		}
		var old string
		if err = tx.QueryRow(ctx, `SELECT id::text,coalesce(username,''),coalesce(phone,''),coalesce(telegram_user_id,''),status FROM global_users WHERE id=$1 FOR UPDATE`, bindUser).Scan(&u.ID, &u.Username, &u.Phone, &old, &u.Status); err != nil {
			return mutation.Result{}, err
		}
		if old != "" && old != claims.ID {
			return mutation.Fail(409, "TELEGRAM_BINDING_IMMUTABLE", "Telegram 不允许解绑或换绑"), nil
		}
		if _, err = tx.Exec(ctx, "UPDATE global_users SET telegram_user_id=$2,updated_at=now() WHERE id=$1", u.ID, claims.ID); unique(err) {
			return mutation.Fail(409, "TELEGRAM_ALREADY_BOUND", "Telegram 已属于其他账号"), nil
		} else if err != nil {
			return mutation.Result{}, err
		}
		u.TelegramID = claims.ID
		if _, err = audit.Append(ctx, tx, audit.Record{BrandID: brand, ActorType: "user", ActorID: u.ID, Action: "user.telegram_bind", ResourceType: "user", ResourceID: u.ID, RequestID: meta.RequestID, IP: meta.IP, Before: map[string]bool{"telegram_bound": old != ""}, After: map[string]bool{"telegram_bound": true}}); err != nil {
			return mutation.Result{}, err
		}
	} else if err == pgx.ErrNoRows {
		if !accept(cfg, in.Privacy, in.Terms) {
			return mutation.Fail(409, "TERMS_REQUIRED", "请先接受当前品牌条款"), nil
		}
		u = User{ID: ids.New(), TelegramID: claims.ID, Status: "active"}
		if _, err = tx.Exec(ctx, "INSERT INTO global_users(id,telegram_user_id) VALUES($1,$2)", u.ID, claims.ID); err != nil {
			return mutation.Result{}, err
		}
	}
	if u.Status != "active" {
		return mutation.Fail(403, "MEMBER_DISABLED", "账号不可用"), nil
	}
	var m Member
	err = tx.QueryRow(ctx, `SELECT id::text,brand_id::text,status,display_name,joined_at FROM brand_members WHERE brand_id=$1 AND global_user_id=$2 FOR UPDATE`, brand, u.ID).Scan(&m.ID, &m.BrandID, &m.Status, &m.DisplayName, &m.JoinedAt)
	if err == pgx.ErrNoRows {
		if !accept(cfg, in.Privacy, in.Terms) {
			return mutation.Fail(409, "BRAND_JOIN_REQUIRED", "请接受当前品牌条款后加入"), nil
		}
		m, err = s.join(ctx, tx, brand, u.ID, in.Privacy, in.Terms, meta)
		if err == nil {
			runes := []rune(claims.Name)
			if len(runes) > 80 {
				runes = runes[:80]
			}
			m.DisplayName = string(runes)
			_, err = tx.Exec(ctx, "UPDATE brand_members SET display_name=$2 WHERE id=$1", m.ID, m.DisplayName)
		}
	}
	if err != nil {
		return mutation.Result{}, err
	}
	if m.Status != "normal" && m.Status != "frozen" {
		return mutation.Fail(403, "MEMBER_DISABLED", "当前品牌账号不可用"), nil
	}
	return s.issue(ctx, tx, View{u, m}, meta, "user.telegram_login")
}
