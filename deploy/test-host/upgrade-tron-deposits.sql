
-- Imported, self-custodied TRON addresses. No private key is stored here.
CREATE TABLE cash_tron_settings (
 brand_id uuid PRIMARY KEY REFERENCES brands(id),enabled boolean NOT NULL DEFAULT false,version bigint NOT NULL CHECK(version>0)
);
CREATE TABLE cash_tron_addresses (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL REFERENCES brands(id),destination_id uuid NOT NULL UNIQUE,
 address text NOT NULL UNIQUE,member_id uuid,bound_at timestamptz,cursor_ms bigint NOT NULL DEFAULT 0 CHECK(cursor_ms>=0),
 next_scan_at timestamptz NOT NULL DEFAULT now(),created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(brand_id,destination_id) REFERENCES cash_destinations(brand_id,id),
 FOREIGN KEY(brand_id,member_id) REFERENCES wallet_owners(brand_id,id),
 CHECK((member_id IS NULL)=(bound_at IS NULL)),UNIQUE(brand_id,member_id)
);
CREATE INDEX cash_tron_scan ON cash_tron_addresses(next_scan_at,id) WHERE member_id IS NOT NULL;
CREATE FUNCTION guard_cash_tron_address() RETURNS trigger LANGUAGE plpgsql SET search_path FROM CURRENT AS $$
BEGIN
 IF TG_OP='DELETE' OR (NEW.id,NEW.brand_id,NEW.destination_id,NEW.address,NEW.created_at) IS DISTINCT FROM (OLD.id,OLD.brand_id,OLD.destination_id,OLD.address,OLD.created_at) THEN RAISE EXCEPTION 'imported TRON addresses are permanent'; END IF;
 IF OLD.member_id IS NOT NULL AND (NEW.member_id,NEW.bound_at) IS DISTINCT FROM (OLD.member_id,OLD.bound_at) THEN RAISE EXCEPTION 'TRON address assignment is permanent'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER cash_tron_address_immutable BEFORE UPDATE OR DELETE ON cash_tron_addresses FOR EACH ROW EXECUTE FUNCTION guard_cash_tron_address();
CREATE TABLE cash_tron_receipts (
 transaction_id text PRIMARY KEY CHECK(transaction_id ~ '^[a-f0-9]{64}$'),
 address_id uuid NOT NULL REFERENCES cash_tron_addresses(id),brand_id uuid NOT NULL,member_id uuid NOT NULL,
 credited_automatically boolean NOT NULL DEFAULT true,amount bigint NOT NULL CHECK(amount>0),block_timestamp bigint NOT NULL CHECK(block_timestamp>0),sender text NOT NULL,
 order_id uuid NOT NULL UNIQUE,created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(brand_id,member_id) REFERENCES wallet_owners(brand_id,id),FOREIGN KEY(brand_id,order_id) REFERENCES cash_orders(brand_id,id)
);
CREATE TRIGGER cash_tron_receipts_immutable BEFORE UPDATE OR DELETE ON cash_tron_receipts FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
