CREATE TABLE auth_challenges (
 id uuid PRIMARY KEY,
 brand_id uuid NOT NULL REFERENCES brands(id),
 kind text NOT NULL CHECK(kind IN ('captcha','telegram')),
 proof_hash text NOT NULL,
 expires_at timestamptz NOT NULL,
 used_at timestamptz
);
CREATE INDEX auth_challenges_expiry ON auth_challenges(expires_at);
