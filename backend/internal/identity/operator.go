package identity

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/authcrypto"
	"github.com/gxfcjkxf/lottery/backend/internal/ids"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
)

type OperatorInput struct {
	Username    string `json:"username"`
	Phone       string `json:"phone"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Notes       string `json:"notes"`
	Reason      string `json:"reason"`
}

type AuthSettingsRecord struct {
	Version              int64  `json:"version"`
	CaptchaEnabled       bool   `json:"captcha_enabled"`
	TelegramEnabled      bool   `json:"telegram_enabled"`
	TelegramClientID     string `json:"telegram_client_id"`
	PrivacyPolicyVersion string `json:"privacy_policy_version"`
	ServiceTermsVersion  string `json:"service_terms_version"`
}

type AuthSettingsInput struct {
	Version          int64  `json:"version"`
	CaptchaEnabled   bool   `json:"captcha_enabled"`
	TelegramEnabled  bool   `json:"telegram_enabled"`
	TelegramClientID string `json:"telegram_client_id"`
	Reason           string `json:"reason"`
}

type operatorCreated struct {
	UserID        string `json:"user_id"`
	MemberID      string `json:"member_id"`
	BrandID       string `json:"brand_id"`
	TermsAccepted bool   `json:"terms_accepted"`
	AuditLogID    string `json:"audit_log_id"`
}

type authSettingsUpdated struct {
	Version              int64  `json:"version"`
	CaptchaEnabled       bool   `json:"captcha_enabled"`
	TelegramEnabled      bool   `json:"telegram_enabled"`
	TelegramClientID     string `json:"telegram_client_id"`
	PrivacyPolicyVersion string `json:"privacy_policy_version"`
	ServiceTermsVersion  string `json:"service_terms_version"`
	AuditLogID           string `json:"audit_log_id"`
}

var telegramClientIDPattern = regexp.MustCompile(`^[1-9][0-9]*$`)

func validTrimmedText(value string, maxBytes int, allowEmpty bool) (string, bool) {
	value = strings.TrimSpace(value)
	return value, utf8.ValidString(value) && len([]byte(value)) <= maxBytes && (allowEmpty || value != "")
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

// OperatorCreate provisions a new global identity and a pending-consent member.
// Existing global identities are intentionally never reused or modified.
func (s *Store) OperatorCreate(ctx context.Context, tx pgx.Tx, brand, operatorID string, in OperatorInput, meta Metadata) (mutation.Result, error) {
	reason, ok := validTrimmedText(in.Reason, 500, false)
	if !ok {
		return mutation.Fail(400, "OPERATOR_INPUT_INVALID", "操作原因须为 1 至 500 字节有效文本"), nil
	}
	in.Reason = reason
	in.DisplayName, ok = validTrimmedText(in.DisplayName, 120, true)
	if !ok {
		return mutation.Fail(400, "OPERATOR_INPUT_INVALID", "显示名称最多 120 字节且须为有效 UTF-8"), nil
	}
	in.Notes, ok = validTrimmedText(in.Notes, 2000, true)
	if !ok {
		return mutation.Fail(400, "OPERATOR_INPUT_INVALID", "备注最多 2000 字节且须为有效 UTF-8"), nil
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
	if len([]byte(in.Password)) < 10 || len([]byte(in.Password)) > 128 {
		return mutation.Fail(400, "PASSWORD_INVALID", "密码须为 10-128 字节"), nil
	}
	var status string
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT status,auth_config FROM brands WHERE id=$1 FOR SHARE`, brand).Scan(&status, &raw); err != nil {
		return mutation.Result{}, err
	}
	if status == "disabled" {
		return mutation.Fail(409, "BRAND_DISABLED", "已停用品牌不能新增用户"), nil
	}
	var cfg Settings
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return mutation.Result{}, err
	}
	hash, err := s.PasswordHash(ctx, in.Password)
	if errors.Is(err, ErrBusy) {
		return mutation.Result{}, err
	}
	if errors.Is(err, authcrypto.ErrInvalidPassword) {
		return mutation.Fail(400, "PASSWORD_INVALID", "密码须为 10-128 字节"), nil
	}
	if err != nil {
		return mutation.Result{}, err
	}
	userID := ids.New()
	_, err = tx.Exec(ctx, `INSERT INTO global_users(id,username,phone,password_hash,username_set_at,phone_set_at)
		VALUES($1,NULLIF($2,''),NULLIF($3,''),$4,CASE WHEN $2<>'' THEN now() END,CASE WHEN $3<>'' THEN now() END)`, userID, in.Username, in.Phone, hash)
	if unique(err) {
		return mutation.Fail(409, "IDENTITY_EXISTS", "全局账号已存在"), nil
	}
	if err != nil {
		return mutation.Result{}, err
	}
	member, err := s.createMember(ctx, tx, brand, userID, cfg.Privacy, cfg.Terms, meta, memberCreateOptions{
		joinMethod: "operator", createdBy: operatorID, displayName: in.DisplayName,
		notes: in.Notes, termsAccepted: false,
	})
	if err != nil {
		return mutation.Result{}, err
	}
	auditID, err := audit.Append(ctx, tx, audit.Record{
		BrandID: brand, ActorType: "admin", ActorID: operatorID, Action: "user.operator_create",
		ResourceType: "member", ResourceID: member.ID, Reason: in.Reason, RequestID: meta.RequestID, IP: meta.IP,
		After: map[string]any{"user_id": userID, "member_id": member.ID, "terms_accepted": false},
	})
	if err != nil {
		return mutation.Result{}, err
	}
	return mutation.OK(201, operatorCreated{UserID: userID, MemberID: member.ID, BrandID: brand, TermsAccepted: false, AuditLogID: auditID}), nil
}

// UpdateAuthSettings changes only the three public login configuration fields.
func (s *Store) UpdateAuthSettings(ctx context.Context, tx pgx.Tx, brand, operatorID string, in AuthSettingsInput, meta Metadata) (mutation.Result, error) {
	reason, ok := validTrimmedText(in.Reason, 500, false)
	if !ok {
		return mutation.Fail(400, "AUTH_CONFIG_INVALID", "操作原因须为 1 至 500 字节有效文本"), nil
	}
	if in.Version <= 0 {
		return mutation.Fail(400, "AUTH_CONFIG_INVALID", "配置版本必须大于 0"), nil
	}
	in.TelegramClientID = strings.TrimSpace(in.TelegramClientID)
	if !utf8.ValidString(in.TelegramClientID) || !validTelegramClientID(in.TelegramClientID, in.TelegramEnabled) {
		return mutation.Fail(400, "AUTH_CONFIG_INVALID", "Telegram 启用时需提供有效客户端 ID"), nil
	}
	var current AuthSettingsRecord
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT config_version,auth_config FROM brands WHERE id=$1 FOR UPDATE`, brand).Scan(&current.Version, &raw)
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
	current.PrivacyPolicyVersion = cfg.Privacy
	current.ServiceTermsVersion = cfg.Terms
	after := AuthSettingsRecord{
		Version: current.Version + 1, CaptchaEnabled: in.CaptchaEnabled,
		TelegramEnabled: in.TelegramEnabled, TelegramClientID: in.TelegramClientID,
		PrivacyPolicyVersion: current.PrivacyPolicyVersion, ServiceTermsVersion: current.ServiceTermsVersion,
	}
	if _, err = tx.Exec(ctx, `UPDATE brands SET auth_config=jsonb_set(jsonb_set(jsonb_set(auth_config,
		'{captcha_enabled}',to_jsonb($2::boolean),true),'{telegram_enabled}',to_jsonb($3::boolean),true),
		'{telegram_client_id}',to_jsonb($4::text),true),config_version=config_version+1,updated_at=now()
		WHERE id=$1 AND config_version=$5`, brand, after.CaptchaEnabled, after.TelegramEnabled, after.TelegramClientID, in.Version); err != nil {
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
	return mutation.OK(200, authSettingsUpdated{Version: after.Version, CaptchaEnabled: after.CaptchaEnabled,
		TelegramEnabled: after.TelegramEnabled, TelegramClientID: after.TelegramClientID,
		PrivacyPolicyVersion: after.PrivacyPolicyVersion, ServiceTermsVersion: after.ServiceTermsVersion,
		AuditLogID: auditID}), nil
}
