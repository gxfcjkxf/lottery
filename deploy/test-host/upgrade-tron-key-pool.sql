
CREATE TABLE cash_tron_query_keys (
 id uuid PRIMARY KEY,name text NOT NULL CHECK(char_length(name) BETWEEN 1 AND 80),
 version bigint NOT NULL CHECK(version>0),key_cipher bytea NOT NULL,key_fingerprint text NOT NULL UNIQUE,key_hint text NOT NULL,
 enabled boolean NOT NULL,daily_limit bigint NOT NULL CHECK(daily_limit BETWEEN 1 AND 1000000000),
 usage_day date NOT NULL DEFAULT (now() AT TIME ZONE 'UTC')::date,used_count bigint NOT NULL DEFAULT 0 CHECK(used_count>=0),
 last_used_at timestamptz,last_checked_at timestamptz,blocked_until timestamptz,
 last_error text NOT NULL DEFAULT '' CHECK(last_error IN ('','credentials','restricted','limited','network','response')),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX cash_tron_query_keys_schedule ON cash_tron_query_keys(last_used_at,id) WHERE enabled;

CREATE TABLE cash_tron_wallets (
 brand_id uuid PRIMARY KEY REFERENCES brands(id),xpub text NOT NULL UNIQUE CHECK(xpub LIKE 'xpub%'),
 first_address text NOT NULL,version bigint NOT NULL CHECK(version>0),
 next_index bigint NOT NULL DEFAULT 0 CHECK(next_index BETWEEN 0 AND 2147483648)
);
CREATE TABLE cash_tron_derivations (
 brand_id uuid NOT NULL REFERENCES cash_tron_wallets(brand_id),
 address_id uuid NOT NULL UNIQUE REFERENCES cash_tron_addresses(id),
 derivation_index bigint NOT NULL CHECK(derivation_index BETWEEN 0 AND 2147483647),
 PRIMARY KEY(brand_id,derivation_index)
);
CREATE TRIGGER cash_tron_derivations_immutable BEFORE UPDATE OR DELETE ON cash_tron_derivations FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE FUNCTION guard_cash_tron_wallet() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'TRON receiving wallets are permanent'; END IF;
 IF OLD.next_index>0 AND (NEW.xpub,NEW.first_address) IS DISTINCT FROM (OLD.xpub,OLD.first_address) THEN RAISE EXCEPTION 'assigned TRON wallet is permanent'; END IF;
 IF NEW.next_index<OLD.next_index THEN RAISE EXCEPTION 'TRON derivation index cannot decrease'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER cash_tron_wallet_immutable BEFORE UPDATE OR DELETE ON cash_tron_wallets FOR EACH ROW EXECUTE FUNCTION guard_cash_tron_wallet();
