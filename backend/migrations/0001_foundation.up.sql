CREATE TABLE brands (
 id uuid PRIMARY KEY,
 code text NOT NULL UNIQUE,
 name text NOT NULL,
 status text NOT NULL CHECK (status IN ('active','paused','disabled')),
 default_locale text NOT NULL DEFAULT 'en',
 timezone text NOT NULL DEFAULT 'Asia/Manila',
 theme jsonb NOT NULL DEFAULT '{}'::jsonb,
 config_version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE brand_domains (
 id uuid PRIMARY KEY,
 brand_id uuid NOT NULL REFERENCES brands(id),
 domain text NOT NULL UNIQUE CHECK (domain=lower(domain)),
 is_primary boolean NOT NULL DEFAULT false,
 enabled boolean NOT NULL DEFAULT true
);
CREATE UNIQUE INDEX brand_primary_domain ON brand_domains(brand_id) WHERE is_primary AND enabled;
CREATE TABLE platform_domains (
 domain text PRIMARY KEY CHECK(domain=lower(domain)),
 enabled boolean NOT NULL DEFAULT true
);
CREATE TABLE global_users (
 id uuid PRIMARY KEY,
 username text UNIQUE CHECK (username=lower(username)),
 phone text UNIQUE,
 password_hash text,
 telegram_user_id text UNIQUE,
 status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','disabled','deleted')),
 username_set_at timestamptz,
 phone_set_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(username IS NOT NULL OR phone IS NOT NULL OR telegram_user_id IS NOT NULL)
);
CREATE TABLE brand_members (
 id uuid PRIMARY KEY,
 brand_id uuid NOT NULL REFERENCES brands(id),
 global_user_id uuid NOT NULL REFERENCES global_users(id),
 status text NOT NULL DEFAULT 'normal' CHECK(status IN ('normal','frozen','disabled','expired','cancelled')),
 display_name text NOT NULL DEFAULT '',
 notes text NOT NULL DEFAULT '',
 profile_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
 join_method text NOT NULL CHECK(join_method IN ('domain','agent_code','referral_code','operator')),
 join_domain text,
 attribution_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
 privacy_policy_version text NOT NULL,
 service_terms_version text NOT NULL,
 accepted_at timestamptz NOT NULL DEFAULT now(),
 joined_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(brand_id,global_user_id),
 UNIQUE(brand_id,id)
);
CREATE INDEX members_brand_created ON brand_members(brand_id,joined_at);
CREATE TABLE admin_accounts (
 id uuid PRIMARY KEY,
 username text NOT NULL UNIQUE,
 password_hash text NOT NULL,
 status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','disabled')),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE roles (id uuid PRIMARY KEY, code text NOT NULL UNIQUE, name text NOT NULL);
CREATE TABLE permissions (key text PRIMARY KEY);
CREATE TABLE admin_account_roles (
 account_id uuid NOT NULL REFERENCES admin_accounts(id),
 role_id uuid NOT NULL REFERENCES roles(id),
 PRIMARY KEY(account_id,role_id)
);
CREATE TABLE role_permissions (
 role_id uuid NOT NULL REFERENCES roles(id),
 permission_key text NOT NULL REFERENCES permissions(key),
 PRIMARY KEY(role_id,permission_key)
);
CREATE TABLE admin_brand_scopes (
 account_id uuid NOT NULL REFERENCES admin_accounts(id),
 brand_id uuid NOT NULL REFERENCES brands(id),
 PRIMARY KEY(account_id,brand_id)
);
CREATE TABLE sessions (
 id uuid PRIMARY KEY,
 token_hash text NOT NULL UNIQUE,
 user_id uuid REFERENCES global_users(id),
 member_id uuid,
 admin_id uuid REFERENCES admin_accounts(id),
 brand_id uuid REFERENCES brands(id),
 expires_at timestamptz NOT NULL,
 revoked_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 CHECK((user_id IS NOT NULL AND member_id IS NOT NULL AND admin_id IS NULL AND brand_id IS NOT NULL)
    OR (admin_id IS NOT NULL AND user_id IS NULL AND member_id IS NULL AND brand_id IS NULL)),
 FOREIGN KEY(brand_id,member_id) REFERENCES brand_members(brand_id,id)
);
CREATE INDEX sessions_user_active ON sessions(brand_id,member_id) WHERE revoked_at IS NULL;
CREATE TABLE point_accounts (
 id uuid PRIMARY KEY,
 brand_id uuid NOT NULL,
 brand_member_id uuid NOT NULL,
 version bigint NOT NULL DEFAULT 0,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(brand_id,brand_member_id),
 UNIQUE(brand_id,id),
 FOREIGN KEY(brand_id,brand_member_id) REFERENCES brand_members(brand_id,id)
);
CREATE TABLE point_buckets (
 brand_id uuid NOT NULL,
 account_id uuid NOT NULL,
 source text NOT NULL CHECK(source IN ('recharge','winning','gift')),
 state text NOT NULL CHECK(state IN ('available','manual_frozen','system_frozen','withdrawal')),
 points bigint NOT NULL DEFAULT 0 CHECK(points>=0),
 PRIMARY KEY(brand_id,account_id,source,state),
 FOREIGN KEY(brand_id,account_id) REFERENCES point_accounts(brand_id,id)
);
CREATE TABLE audit_logs (
 id uuid PRIMARY KEY,
 brand_id uuid REFERENCES brands(id),
 actor_type text NOT NULL CHECK(actor_type IN ('user','admin','system')),
 actor_id uuid,
 action text NOT NULL,
 resource_type text NOT NULL,
 resource_id uuid,
 before_json jsonb,
 after_json jsonb,
 reason text NOT NULL DEFAULT '',
 request_id text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_brand_resource ON audit_logs(brand_id,resource_type,resource_id,created_at);
CREATE TABLE point_ledger_entries (
 id uuid PRIMARY KEY,
 brand_id uuid NOT NULL,
 account_id uuid NOT NULL,
 entry_type text NOT NULL,
 reference_type text NOT NULL,
 reference_id uuid,
 operation_key text NOT NULL,
 before_snapshot jsonb NOT NULL,
 delta_snapshot jsonb NOT NULL,
 after_snapshot jsonb NOT NULL,
 source_allocation jsonb NOT NULL DEFAULT '[]'::jsonb,
 reason text NOT NULL,
 actor_id uuid,
 request_id text NOT NULL,
 reversal_of uuid REFERENCES point_ledger_entries(id),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(brand_id,operation_key),
 FOREIGN KEY(brand_id,account_id) REFERENCES point_accounts(brand_id,id)
);
CREATE INDEX ledger_account_created ON point_ledger_entries(brand_id,account_id,created_at);
CREATE TABLE idempotency_requests (
 brand_id uuid NOT NULL REFERENCES brands(id),
 actor_id uuid NOT NULL,
 operation text NOT NULL,
 key text NOT NULL,
 request_hash text NOT NULL,
 status_code integer,
 response jsonb,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(brand_id,actor_id,operation,key)
);
CREATE TABLE outbox_events (
 id uuid PRIMARY KEY,
 brand_id uuid REFERENCES brands(id),
 event_type text NOT NULL,
 aggregate_id uuid NOT NULL,
 payload jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 published_at timestamptz,
 attempts integer NOT NULL DEFAULT 0
);
CREATE INDEX outbox_unpublished ON outbox_events(created_at) WHERE published_at IS NULL;
CREATE TABLE consumed_events (
 consumer text NOT NULL,
 event_id uuid NOT NULL,
 consumed_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(consumer,event_id)
);
CREATE FUNCTION reject_immutable_change() RETURNS trigger AS $$
BEGIN
 RAISE EXCEPTION 'append-only table cannot be updated or deleted';
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER audit_immutable BEFORE UPDATE OR DELETE ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TRIGGER ledger_immutable BEFORE UPDATE OR DELETE ON point_ledger_entries FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
