ALTER TABLE brands ADD COLUMN contact_config jsonb NOT NULL DEFAULT '{"items":[],"default_id":null}'::jsonb;
ALTER TABLE brands ADD COLUMN contact_version bigint NOT NULL DEFAULT 1 CHECK (contact_version > 0);
