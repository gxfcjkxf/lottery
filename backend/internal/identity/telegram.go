package identity

import (
	"context"
	"errors"
	"github.com/gxfcjkxf/lottery/backend/internal/attribution"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/gxfcjkxf/lottery/backend/internal/telegramauth"
	"github.com/jackc/pgx/v5"
	"strings"
)

type TelegramInput struct {
	AgentCode    string `json:"agent_code,omitempty"`
	ReferralCode string `json:"referral_code,omitempty"`
	IDToken      string `json:"id_token"`
	ChallengeID  string `json:"challenge_id"`
	Nonce        string `json:"nonce"`
	Bind         bool   `json:"bind,omitempty"`
}

func (s *Store) Telegram(ctx context.Context, tx pgx.Tx, brand string, claims telegramauth.Claims, in TelegramInput, bindUser string, meta Metadata) (mutation.Result, error) {
	kind, code, joinErr := attribution.Normalize(in.AgentCode, in.ReferralCode)
	if joinErr != nil {
		return mutation.Fail(400, "JOIN_CODE_INPUT_INVALID", "加入码格式不正确，且代理码与推荐码不能同时使用"), nil
	}
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
	err = tx.QueryRow(ctx, `SELECT id::text,coalesce(username,''),coalesce(phone,''),coalesce(wechat,''),coalesce(telegram,''),coalesce(whatsapp,''),telegram_user_id,status FROM global_users WHERE telegram_user_id=$1 FOR UPDATE`, claims.ID).Scan(&u.ID, &u.Username, &u.Phone, &u.Wechat, &u.Telegram, &u.Whatsapp, &u.TelegramID, &u.Status)
	if err != nil && err != pgx.ErrNoRows {
		return mutation.Result{}, err
	}
	if bindUser != "" {
		if err == nil && u.ID != bindUser {
			return mutation.Fail(409, "TELEGRAM_ALREADY_BOUND", "Telegram 已属于其他账号"), nil
		}
		var old string
		if err = tx.QueryRow(ctx, `SELECT id::text,coalesce(username,''),coalesce(phone,''),coalesce(wechat,''),coalesce(telegram,''),coalesce(whatsapp,''),coalesce(telegram_user_id,''),status FROM global_users WHERE id=$1 FOR UPDATE`, bindUser).Scan(&u.ID, &u.Username, &u.Phone, &u.Wechat, &u.Telegram, &u.Whatsapp, &old, &u.Status); err != nil {
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
	if err == nil && kind != "" {
		return mutation.Fail(409, "JOIN_ATTRIBUTION_FIXED", "当前品牌成员已存在，加入归属不能更改；请去掉编码再正常登录"), nil
	}
	if err == pgx.ErrNoRows {
		m, err = s.joinCode(ctx, tx, brand, u.ID, kind, code, meta)
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
		if errors.Is(attribution.DatabaseError(err), attribution.ErrUnavailable) {
			return mutation.Fail(400, "JOIN_CODE_UNAVAILABLE", "加入码不可用，请核对当前品牌、编码及有效期"), nil
		}
		return mutation.Result{}, err
	}
	if m.Status != "normal" && m.Status != "frozen" {
		return mutation.Fail(403, "MEMBER_DISABLED", "当前品牌账号不可用"), nil
	}
	if u.Username == "" || (claims.Username != "" && u.Telegram == "") {
		username := claims.ID
		contact := ""
		if claims.Username != "" {
			username = strings.ToLower(claims.Username)
			contact = "@" + claims.Username
		}
		const fillProfile = `UPDATE global_users SET
			username=coalesce(username,$2),
			username_set_at=CASE WHEN username IS NULL THEN now() ELSE username_set_at END,
			telegram=coalesce(NULLIF(telegram,''),NULLIF($3,'')),updated_at=now() WHERE id=$1`
		attempt, err := tx.Begin(ctx)
		if err != nil {
			return mutation.Result{}, err
		}
		_, err = attempt.Exec(ctx, fillProfile, u.ID, username, contact)
		if err != nil {
			if rollbackErr := attempt.Rollback(ctx); rollbackErr != nil {
				return mutation.Result{}, rollbackErr
			}
			if !unique(err) {
				return mutation.Result{}, err
			}
			if claims.Username == "" {
				return mutation.Fail(409, "IDENTITY_EXISTS", "账号已存在，请使用其他用户名或手机号"), nil
			}
			username = "tg_" + username
			_, err = tx.Exec(ctx, fillProfile, u.ID, username, contact)
		} else {
			err = attempt.Commit(ctx)
		}
		if unique(err) {
			return mutation.Fail(409, "IDENTITY_EXISTS", "账号已存在，请使用其他用户名或手机号"), nil
		}
		if err != nil {
			return mutation.Result{}, err
		}
		if u.Username == "" {
			u.Username = username
		}
		if u.Telegram == "" {
			u.Telegram = contact
		}
	}
	return s.issue(ctx, tx, View{u, m}, meta, "user.telegram_login")
}
