ALTER TABLE admin_accounts ADD COLUMN is_super_admin boolean NOT NULL DEFAULT false;
ALTER TABLE audit_logs ADD COLUMN ip_address text;
ALTER TABLE brands ADD COLUMN auth_config jsonb NOT NULL DEFAULT '{"captcha_enabled":false,"telegram_enabled":false,"privacy_policy_version":"dev-1","service_terms_version":"dev-1"}'::jsonb;
CREATE TABLE auth_rate_limits (
 bucket_hash text PRIMARY KEY,
 attempt_count integer NOT NULL CHECK(attempt_count>0),
 window_start timestamptz NOT NULL,
 expires_at timestamptz NOT NULL
);
CREATE INDEX auth_rate_limit_expiry ON auth_rate_limits(expires_at);
