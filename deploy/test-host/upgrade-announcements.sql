CREATE TABLE announcements (
 id uuid PRIMARY KEY,
 brand_id uuid REFERENCES brands(id) ON DELETE CASCADE,
 version bigint NOT NULL CHECK(version>0),
 generation bigint NOT NULL CHECK(generation>0),
 status text NOT NULL CHECK(status IN ('draft','published','withdrawn')),
 config jsonb NOT NULL,
 published_at timestamptz,
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX announcements_scope ON announcements(brand_id,status);
CREATE TABLE announcement_assets (
 id text PRIMARY KEY CHECK(id ~ '^[a-f0-9]{64}$'),
 brand_id uuid REFERENCES brands(id) ON DELETE CASCADE,
 media_type text NOT NULL CHECK(media_type IN ('image/png','image/jpeg','image/webp')),
 content bytea NOT NULL CHECK(octet_length(content)<=5242880)
);
CREATE TABLE announcement_receipts (
 brand_id uuid NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
 subject text NOT NULL,
 announcement_id uuid NOT NULL REFERENCES announcements(id) ON DELETE CASCADE,
 generation bigint NOT NULL,
 seen_at timestamptz,
 session_key text NOT NULL,
 muted_day text,
 PRIMARY KEY(brand_id,subject,announcement_id,generation)
);
INSERT INTO permissions(key) VALUES
 ('announcement.view.platform'),('announcement.write.platform'),('announcement.publish.platform'),
 ('announcement.view.brand'),('announcement.write.brand'),('announcement.publish.brand');
