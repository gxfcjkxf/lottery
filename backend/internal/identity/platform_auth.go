package identity

import (
	"context"
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/audit"
	"github.com/gxfcjkxf/lottery/backend/internal/mutation"
	"github.com/jackc/pgx/v5"
)

func (s *Store) updatePlatformAuthSettings(ctx context.Context, tx pgx.Tx, operator string, in AuthSettingsInput, meta Metadata) (mutation.Result, error) {
	if in.InheritPlatform {
		return mutation.Fail(400, "AUTH_CONFIG_INVALID", "AUTH_CONFIG_INVALID"), nil
	}
	var version int64
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT version,config FROM platform_auth_settings WHERE id=true FOR UPDATE`).Scan(&version, &raw); err != nil {
		return mutation.Result{}, err
	}
	if version != in.Version {
		return mutation.Fail(409, "CONFIG_VERSION_CONFLICT", "CONFIG_VERSION_CONFLICT"), nil
	}
	var before Settings
	if err := json.Unmarshal(raw, &before); err != nil {
		return mutation.Result{}, err
	}
	after := AuthSettingsRecord{Version: version + 1, CaptchaEnabled: in.CaptchaEnabled, TelegramEnabled: in.TelegramEnabled, TelegramClientID: in.TelegramClientID}
	cfg := map[string]any{"captcha_enabled": in.CaptchaEnabled, "telegram_enabled": in.TelegramEnabled, "telegram_client_id": in.TelegramClientID}
	encoded, err := json.Marshal(cfg)
	if err != nil {
		return mutation.Result{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE platform_auth_settings SET version=version+1,config=$1 WHERE id=true`, encoded); err != nil {
		return mutation.Result{}, err
	}
	auditID, err := audit.Append(ctx, tx, audit.Record{ActorType: "admin", ActorID: operator, Action: "auth_config.platform.update", ResourceType: "platform_auth_settings", Reason: in.Reason, RequestID: meta.RequestID, IP: meta.IP, Before: before, After: after})
	if err != nil {
		return mutation.Result{}, err
	}
	return mutation.OK(200, authSettingsUpdated{Version: after.Version, CaptchaEnabled: after.CaptchaEnabled, TelegramEnabled: after.TelegramEnabled, TelegramClientID: after.TelegramClientID, AuditLogID: auditID}), nil
}
