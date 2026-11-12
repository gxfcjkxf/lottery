package identity

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/attribution"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/authcrypto"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
)

type OperatorInput struct {
	AgentCode    string `json:"agent_code,omitempty"`
	ReferralCode string `json:"referral_code,omitempty"`
	Username     string `json:"username"`
	Phone        string `json:"phone"`
	Password     string `json:"password"`
	DisplayName  string `json:"display_name"`
	Notes        string `json:"notes"`
	Reason       string `json:"reason"`
}

type AuthSettingsRecord struct {
	InheritPlatform  bool   `json:"inherit_platform"`
	Version          int64  `json:"version"`
	CaptchaEnabled   bool   `json:"captcha_enabled"`
	TelegramEnabled  bool   `json:"telegram_enabled"`
	TelegramClientID string `json:"telegram_client_id"`
}

type AuthSettingsInput struct {
	InheritPlatform  bool   `json:"inherit_platform"`
	Version          int64  `json:"version"`
	CaptchaEnabled   bool   `json:"captcha_enabled"`
	TelegramEnabled  bool   `json:"telegram_enabled"`
	TelegramClientID string `json:"telegram_client_id"`
	Reason           string `json:"reason"`
}

type operatorCreated struct {
	UserID     string `json:"user_id"`
	MemberID   string `json:"member_id"`
	BrandID    string `json:"brand_id"`
	AuditLogID string `json:"audit_log_id"`
}

type authSettingsUpdated struct {
	InheritPlatform  bool   `json:"inherit_platform"`
	Version          int64  `json:"version"`
	CaptchaEnabled   bool   `json:"captcha_enabled"`
	TelegramEnabled  bool   `json:"telegram_enabled"`
	TelegramClientID string `json:"telegram_client_id"`
	AuditLogID       string `json:"audit_log_id"`
}

var telegramClientIDPattern = regexp.MustCompile(`^[1-9][0-9]*$`)

func validTrimmedText(value string, maxCharacters int, allowEmpty bool) (string, bool) {
	value = strings.TrimSpace(value)
	return value, utf8.ValidString(value) && utf8.RuneCountInString(value) <= maxCharacters && (allowEmpty || value != "")
}

func validTelegramClientID(value string, enabled bool) bool {
	if value == "" {
		return !enabled
	}
	if !telegramClientIDPattern.MatchString(value) {
		return false
	}
	n, err := strconv.ParseUint(value, 10, 53)
	return err == nil && n > 0 && n <= 9007199254740991
}

// OperatorCreate provisions a new global identity and an immediately usable member.
// Administrative creation does not record personal acceptance of the terms.
// Existing global identities are intentionally never reused or modified.
func (s *Store) OperatorCreate(ctx context.Context, tx pgx.Tx, brand, operatorID string, in OperatorInput, meta Metadata) (mutation.Result, error) {
	return s.operatorCreate(ctx, tx, brand, operatorID, in, meta, "admin")
}
func (s *Store) AgentCreateMember(ctx context.Context, tx pgx.Tx, v AgentSession, in OperatorInput, meta Metadata) (mutation.Result, error) {
	var code string
	err := tx.QueryRow(ctx, `SELECT c.code FROM join_codes c JOIN agent_nodes n ON n.brand_id=c.brand_id AND n.id=c.agent_id WHERE n.brand_id=$1 AND n.member_id=$2 AND c.kind='agent' AND join_code_usable(c) ORDER BY c.created_at,c.id LIMIT 1`, v.BrandID, v.AccountID).Scan(&code)
	if err != nil {
		return mutation.Fail(409, "JOIN_CODE_UNAVAILABLE", "代理码不可用"), nil
	}
	// The caller cannot reassign a new member to a different agent.
	in.AgentCode = code
	in.ReferralCode = ""
	return s.operatorCreate(ctx, tx, v.BrandID, v.AccountID, in, meta, "agent")
}
func (s *Store) operatorCreate(ctx context.Context, tx pgx.Tx, brand, operatorID string, in OperatorInput, meta Metadata, actorType string) (mutation.Result, error) {
	kind, code, joinErr := attribution.Normalize(in.AgentCode, in.ReferralCode)
	if joinErr != nil {
		return mutation.Fail(400, "JOIN_CODE_INPUT_INVALID", "加入码格式不正确，且代理码与推荐码不能同时使用"), nil
	}
	reason, ok := validTrimmedText(in.Reason, 500, true)
	if !ok {
		return mutation.Fail(400, "OPERATOR_INPUT_INVALID", "操作原因最多 500 字符有效文本"), nil
	}
	in.Reason = reason
	in.DisplayName, ok = validTrimmedText(in.DisplayName, 120, true)
	if !ok {
		return mutation.Fail(400, "OPERATOR_INPUT_INVALID", "显示名称最多 120 字符且须为有效 UTF-8"), nil
	}
	in.Notes, ok = validTrimmedText(in.Notes, 2000, true)
	if !ok {
		return mutation.Fail(400, "OPERATOR_INPUT_INVALID", "备注最多 2000 字符且须为有效 UTF-8"), nil
	}
	if in.Username != "" {
		var err error
		in.Username, err = normalizeUsername(in.Username)
		if err != nil {
			return mutation.Fail(400, "OPERATOR_INPUT_INVALID", "用户名格式不正确"), nil
		}
	}
	if in.Phone != "" {
		var err error
		in.Phone, err = normalizePhone(in.Phone)
		if err != nil {
			return mutation.Fail(400, "OPERATOR_INPUT_INVALID", "手机号格式不正确"), nil
		}
	}
	if in.Username == "" && in.Phone == "" {
		return mutation.Fail(400, "OPERATOR_INPUT_INVALID", "需要用户名或手机号"), nil
	}
	if utf8.RuneCountInString(in.Password) < 6 || utf8.RuneCountInString(in.Password) > 30 {
		return mutation.Fail(400, "PASSWORD_INVALID", "密码须为 6-30 字符"), nil
	}
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM brands WHERE id=$1 FOR SHARE`, brand).Scan(&status); err != nil {
		return mutation.Result{}, err
	}
	if status == "disabled" {
		return mutation.Fail(409, "BRAND_DISABLED", "已停用品牌不能新增用户"), nil
	}
	hash, err := s.PasswordHash(ctx, in.Password)
	if errors.Is(err, ErrBusy) {
		return mutation.Result{}, err
	}
	if errors.Is(err, authcrypto.ErrInvalidPassword) {
		return mutation.Fail(400, "PASSWORD_INVALID", "密码须为 6-30 字符"), nil
	}
	if err != nil {
		return mutation.Result{}, err
	}
	userID := ids.New()
	_, err = tx.Exec(ctx, `INSERT INTO global_users(id,username,phone,password_hash,username_set_at,phone_set_at)
		VALUES($1,NULLIF($2,''),NULLIF($3,''),$4,CASE WHEN $2<>'' THEN now() END,CASE WHEN $3<>'' THEN now() END)`, userID, in.Username, in.Phone, hash)
	if unique(err) {
		return mutation.Fail(409, "IDENTITY_EXISTS", "账号已存在，请使用其他用户名或手机号"), nil
	}
	if err != nil {
		return mutation.Result{}, err
	}
	joinMethod, createdBy := "operator", operatorID
	if actorType == "agent" {
		joinMethod, createdBy = "agent_code", ""
	}
	member, err := s.createMember(ctx, tx, brand, userID, meta, memberCreateOptions{
		joinMethod: joinMethod, createdBy: createdBy, displayName: in.DisplayName,
		notes: in.Notes, codeKind: kind, code: code,
	})
	if err != nil {
		if errors.Is(attribution.DatabaseError(err), attribution.ErrUnavailable) {
			return mutation.Fail(400, "JOIN_CODE_UNAVAILABLE", "加入码不可用，请核对当前品牌、编码及有效期"), nil
		}
		return mutation.Result{}, err
	}
	auditID, err := audit.Append(ctx, tx, audit.Record{
		BrandID: brand, ActorType: actorType, ActorID: operatorID, Action: "user.operator_create",
		ResourceType: "member", ResourceID: member.ID, Reason: in.Reason, RequestID: meta.RequestID, IP: meta.IP,
		After: map[string]any{"user_id": userID, "member_id": member.ID},
	})
	if err != nil {
		return mutation.Result{}, err
	}
	return mutation.OK(201, operatorCreated{UserID: userID, MemberID: member.ID, BrandID: brand, AuditLogID: auditID}), nil
}

// UpdateAuthSettings changes only the three public login configuration fields.
func (s *Store) UpdateAuthSettings(ctx context.Context, tx pgx.Tx, brand, operatorID string, in AuthSettingsInput, meta Metadata) (mutation.Result, error) {
	reason, ok := validTrimmedText(in.Reason, 500, true)
	if !ok {
		return mutation.Fail(400, "AUTH_CONFIG_INVALID", "操作原因最多 500 字符有效文本"), nil
	}
	if in.Version <= 0 {
		return mutation.Fail(400, "AUTH_CONFIG_INVALID", "配置版本必须大于 0"), nil
	}
	in.TelegramClientID = strings.TrimSpace(in.TelegramClientID)
	if !in.InheritPlatform && (!utf8.ValidString(in.TelegramClientID) || !validTelegramClientID(in.TelegramClientID, in.TelegramEnabled)) {
		return mutation.Fail(400, "AUTH_CONFIG_INVALID", "Telegram 启用时需提供有效客户端 ID"), nil
	}
	if brand == "" {
		return s.updatePlatformAuthSettings(ctx, tx, operatorID, in, meta)
	}
	var current AuthSettingsRecord
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT config_version,auth_config,auth_inherit_platform FROM brands WHERE id=$1 FOR UPDATE`, brand).Scan(&current.Version, &raw, &current.InheritPlatform)
	if err != nil {
		return mutation.Result{}, err
	}
	if current.Version != in.Version {
		return mutation.Fail(409, "CONFIG_VERSION_CONFLICT", "认证配置已更新，请刷新后重试"), nil
	}
	var cfg Settings
	if err = json.Unmarshal(raw, &cfg); err != nil {
		return mutation.Result{}, err
	}
	current.CaptchaEnabled = cfg.CaptchaEnabled
	current.TelegramEnabled = cfg.TelegramEnabled
	current.TelegramClientID = cfg.TelegramClientID
	if in.InheritPlatform || current.InheritPlatform {
		var defaults []byte
		if err = tx.QueryRow(ctx, `SELECT config FROM platform_auth_settings WHERE id=true FOR SHARE`).Scan(&defaults); err != nil {
			return mutation.Result{}, err
		}
		var effective Settings
		if err = json.Unmarshal(defaults, &effective); err != nil {
			return mutation.Result{}, err
		}
		if current.InheritPlatform {
			current.CaptchaEnabled = effective.CaptchaEnabled
			current.TelegramEnabled = effective.TelegramEnabled
			current.TelegramClientID = effective.TelegramClientID
		}
		if in.InheritPlatform {
			in.CaptchaEnabled = effective.CaptchaEnabled
			in.TelegramEnabled = effective.TelegramEnabled
			in.TelegramClientID = effective.TelegramClientID
		}
	}
	after := AuthSettingsRecord{InheritPlatform: in.InheritPlatform,
		Version: current.Version + 1, CaptchaEnabled: in.CaptchaEnabled,
		TelegramEnabled: in.TelegramEnabled, TelegramClientID: in.TelegramClientID,
	}
	if _, err = tx.Exec(ctx, `UPDATE brands SET auth_inherit_platform=$6,auth_config=jsonb_set(jsonb_set(jsonb_set(auth_config,
		'{captcha_enabled}',to_jsonb($2::boolean),true),'{telegram_enabled}',to_jsonb($3::boolean),true),
		'{telegram_client_id}',to_jsonb($4::text),true),config_version=config_version+1,updated_at=now()
		WHERE id=$1 AND config_version=$5`, brand, after.CaptchaEnabled, after.TelegramEnabled, after.TelegramClientID, in.Version, in.InheritPlatform); err != nil {
		return mutation.Result{}, err
	}
	auditID, err := audit.Append(ctx, tx, audit.Record{
		BrandID: brand, ActorType: "admin", ActorID: operatorID, Action: "auth_config.update",
		ResourceType: "brand", ResourceID: brand, Reason: reason, RequestID: meta.RequestID, IP: meta.IP,
		Before: current, After: after,
	})
	if err != nil {
		return mutation.Result{}, err
	}
	return mutation.OK(200, authSettingsUpdated{InheritPlatform: after.InheritPlatform, Version: after.Version, CaptchaEnabled: after.CaptchaEnabled,
		TelegramEnabled: after.TelegramEnabled, TelegramClientID: after.TelegramClientID,
		AuditLogID: auditID}), nil
}
