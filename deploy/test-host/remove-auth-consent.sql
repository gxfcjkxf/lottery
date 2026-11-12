ALTER TABLE brand_members DROP CONSTRAINT member_consent_consistent;
ALTER TABLE brand_members
    DROP COLUMN privacy_policy_version,
    DROP COLUMN service_terms_version,
    DROP COLUMN accepted_at,
    DROP COLUMN terms_accepted;
ALTER TABLE brands ALTER COLUMN auth_config SET DEFAULT '{"captcha_enabled": false, "telegram_enabled": false}'::jsonb;
UPDATE brands SET auth_config = auth_config - 'privacy_policy_version' - 'service_terms_version';
UPDATE platform_auth_settings SET config = config - 'privacy_policy_version' - 'service_terms_version';
