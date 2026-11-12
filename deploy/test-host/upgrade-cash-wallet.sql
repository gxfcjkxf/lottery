-- Multi-currency balances: recharge funds cannot be withdrawn before point conversion.
CREATE TABLE cash_configs (scope text PRIMARY KEY,version bigint NOT NULL CHECK(version>0),config jsonb NOT NULL);
CREATE TABLE cash_destinations (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL REFERENCES brands(id),member_id uuid,
 currency text NOT NULL CHECK(currency IN('CNY','USD','THB','USDT')),data jsonb NOT NULL,
 enabled boolean NOT NULL,version bigint NOT NULL CHECK(version>0),
 assigned_count bigint NOT NULL DEFAULT 0 CHECK(assigned_count>=0),assigned_amount bigint NOT NULL DEFAULT 0 CHECK(assigned_amount>=0),
 FOREIGN KEY(brand_id,member_id) REFERENCES brand_members(brand_id,id),UNIQUE(brand_id,id)
);
CREATE INDEX cash_destinations_scope ON cash_destinations(brand_id,member_id,currency,id);
CREATE TABLE cash_balances (
 brand_id uuid NOT NULL,member_id uuid NOT NULL,currency text NOT NULL CHECK(currency IN('CNY','USD','THB','USDT')),
 deposited bigint NOT NULL DEFAULT 0 CHECK(deposited>=0),withdrawable bigint NOT NULL DEFAULT 0 CHECK(withdrawable>=0),reserved bigint NOT NULL DEFAULT 0 CHECK(reserved>=0),
 PRIMARY KEY(brand_id,member_id,currency),FOREIGN KEY(brand_id,member_id) REFERENCES brand_members(brand_id,id),
 CHECK(currency='USDT' OR (deposited%10000=0 AND withdrawable%10000=0 AND reserved%10000=0))
);
CREATE TABLE cash_orders (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,member_id uuid NOT NULL,
 kind text NOT NULL CHECK(kind IN('deposit','to_points','from_points','withdrawal')),
 currency text NOT NULL CHECK(currency IN('CNY','USD','THB','USDT')),amount bigint NOT NULL CHECK(amount>0),
 points numeric NOT NULL DEFAULT 0 CHECK(points>=0 AND points=trunc(points,2)),fee bigint NOT NULL DEFAULT 0 CHECK(fee>=0 AND fee<amount),rate text NOT NULL DEFAULT '',
 state text NOT NULL CHECK(state IN('pending','submitted','confirmed','cancelled','expired','completed','reviewing','processing','paid','rejected','failed')),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),destination_id uuid,destination jsonb NOT NULL DEFAULT '{}',
 proof text NOT NULL DEFAULT '',reference text NOT NULL DEFAULT '',point_entry_id uuid, evidence jsonb NOT NULL DEFAULT '{}',
 created_at timestamptz NOT NULL DEFAULT now(),expires_at timestamptz,
 FOREIGN KEY(brand_id,member_id) REFERENCES brand_members(brand_id,id),FOREIGN KEY(brand_id,destination_id) REFERENCES cash_destinations(brand_id,id),UNIQUE(brand_id,id),
 CHECK(currency='USDT' OR (amount%10000=0 AND fee%10000=0))
);
CREATE INDEX cash_orders_member ON cash_orders(brand_id,member_id,created_at DESC,id DESC);
CREATE UNIQUE INDEX cash_active_withdrawal ON cash_orders(brand_id,member_id) WHERE kind='withdrawal' AND state IN('reviewing','processing');
CREATE UNIQUE INDEX cash_deposit_reference ON cash_orders(brand_id,currency,reference) WHERE kind='deposit' AND state='confirmed' AND reference<>'';
CREATE TABLE cash_entries (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,member_id uuid NOT NULL,currency text NOT NULL,
 order_id uuid NOT NULL,operation text NOT NULL,before_snapshot jsonb NOT NULL,delta_snapshot jsonb NOT NULL,after_snapshot jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),FOREIGN KEY(brand_id,member_id,currency) REFERENCES cash_balances(brand_id,member_id,currency),FOREIGN KEY(brand_id,order_id) REFERENCES cash_orders(brand_id,id),UNIQUE(order_id,operation)
);
CREATE TRIGGER cash_entries_immutable BEFORE UPDATE OR DELETE ON cash_entries FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
CREATE TABLE cash_turnover_cycles (
 brand_id uuid NOT NULL,member_id uuid NOT NULL,cutoff_at timestamptz NOT NULL,cutoff_version bigint NOT NULL CHECK(cutoff_version>0),order_id uuid NOT NULL,
 PRIMARY KEY(brand_id,member_id),FOREIGN KEY(brand_id,member_id) REFERENCES brand_members(brand_id,id),FOREIGN KEY(brand_id,order_id) REFERENCES cash_orders(brand_id,id)
);
CREATE TABLE cash_quota_entries (
 order_id uuid PRIMARY KEY,brand_id uuid NOT NULL,member_id uuid NOT NULL,points numeric NOT NULL CHECK(points>0),before_available numeric NOT NULL,after_available numeric NOT NULL CHECK(after_available>=0 AND after_available=before_available-points),
 created_at timestamptz NOT NULL DEFAULT now(),FOREIGN KEY(brand_id,order_id) REFERENCES cash_orders(brand_id,id),FOREIGN KEY(brand_id,member_id) REFERENCES brand_members(brand_id,id)
);
CREATE TRIGGER cash_quota_entries_immutable BEFORE UPDATE OR DELETE ON cash_quota_entries FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
ALTER TABLE cash_destinations ADD COLUMN rotation_epoch bigint NOT NULL DEFAULT 1;
ALTER TABLE cash_orders ADD COLUMN rotation_epoch bigint;
CREATE TABLE cash_rotation (brand_id uuid NOT NULL REFERENCES brands(id),currency text NOT NULL,destination_id uuid NOT NULL,PRIMARY KEY(brand_id,currency),FOREIGN KEY(brand_id,destination_id) REFERENCES cash_destinations(brand_id,id));
ALTER TABLE cash_orders ADD COLUMN requested_amount bigint NOT NULL CHECK(requested_amount>0);
CREATE FUNCTION guard_cash_order() RETURNS trigger LANGUAGE plpgsql SET search_path FROM CURRENT AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'cash orders are immutable'; END IF;
 IF (NEW.id,NEW.brand_id,NEW.member_id,NEW.kind,NEW.currency,NEW.requested_amount,NEW.points,NEW.fee,NEW.rate,NEW.destination_id,NEW.destination,NEW.created_at,NEW.expires_at) IS DISTINCT FROM (OLD.id,OLD.brand_id,OLD.member_id,OLD.kind,OLD.currency,OLD.requested_amount,OLD.points,OLD.fee,OLD.rate,OLD.destination_id,OLD.destination,OLD.created_at,OLD.expires_at) THEN RAISE EXCEPTION 'cash economic terms are immutable'; END IF;
 IF NEW.amount<>OLD.amount AND NOT(OLD.kind='deposit' AND OLD.state IN('pending','submitted','expired') AND NEW.state='confirmed') THEN RAISE EXCEPTION 'only verified receipt can change deposit amount'; END IF;
 IF OLD.state IN('confirmed','cancelled','paid','rejected','failed') OR OLD.state='completed' AND OLD.point_entry_id IS NOT NULL THEN
   IF NEW IS DISTINCT FROM OLD THEN RAISE EXCEPTION 'completed cash orders are immutable'; END IF;
 END IF;
 IF NEW.state<>OLD.state AND NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'cash transition requires next version'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER cash_order_guard BEFORE UPDATE OR DELETE ON cash_orders FOR EACH ROW EXECUTE FUNCTION guard_cash_order();
ALTER TABLE cash_balances ADD COLUMN version bigint NOT NULL DEFAULT 0 CHECK(version>=0);
ALTER TABLE cash_entries ADD COLUMN version bigint NOT NULL CHECK(version>0);
ALTER TABLE cash_entries ADD CONSTRAINT cash_entry_version UNIQUE(brand_id,member_id,currency,version);
CREATE UNIQUE INDEX cash_payout_reference ON cash_orders(brand_id,currency,reference) WHERE kind='withdrawal' AND state='paid' AND reference<>'';
CREATE UNIQUE INDEX cash_usdt_deposit_transaction ON cash_orders(reference) WHERE kind='deposit' AND currency='USDT' AND state='confirmed' AND reference<>'';
