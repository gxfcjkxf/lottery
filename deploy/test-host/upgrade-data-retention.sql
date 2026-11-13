
-- 0074_monthly_data_retention
-- Monthly retention: operational details and immutable economic receipts are
-- separate stores. Fact readers deliberately read both; public detail readers
-- read operational tables only.
CREATE TABLE data_retention_policy (
 id boolean PRIMARY KEY DEFAULT true CHECK(id), version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 enabled boolean NOT NULL DEFAULT false, execution_day integer NOT NULL DEFAULT 10 CHECK(execution_day BETWEEN 1 AND 28),
 execution_time text NOT NULL DEFAULT '02:00' CHECK(execution_time ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'),
 next_execute_at timestamptz, next_cutoff_at timestamptz, next_timezone text,
 updated_at timestamptz NOT NULL DEFAULT now(), CHECK((next_execute_at IS NULL)=(next_cutoff_at IS NULL)),
 CHECK(enabled=(next_execute_at IS NOT NULL))
);
INSERT INTO data_retention_policy(id) VALUES(true);
CREATE TABLE data_retention_tasks (
 id uuid PRIMARY KEY,brand_id uuid REFERENCES brands(id),policy_version bigint NOT NULL,
 timezone text NOT NULL,execute_at timestamptz NOT NULL,cutoff_at timestamptz NOT NULL,
 state text NOT NULL DEFAULT 'pending' CHECK(state IN('pending','completed','deferred','failed')),
 counts jsonb NOT NULL DEFAULT '{}',error_code text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),finished_at timestamptz,
 UNIQUE NULLS NOT DISTINCT(brand_id,execute_at),CHECK(cutoff_at<execute_at)
);
CREATE INDEX data_retention_pending ON data_retention_tasks(execute_at,id) WHERE state='pending';
CREATE TABLE data_retention_receipts (
 family text NOT NULL CHECK(family IN('configured_orders','point_ledger_entries','cash_orders','cash_entries','audit_logs','recharge_orders','withdrawal_orders','notifications')),
 id uuid NOT NULL,brand_id uuid,task_id uuid NOT NULL REFERENCES data_retention_tasks(id),
 payload jsonb NOT NULL,original_sha256 text NOT NULL,creation_xid xid8 NOT NULL DEFAULT pg_current_xact_id(),
 PRIMARY KEY(family,id)
) WITH (toast_tuple_target=128);
CREATE INDEX data_retention_receipt_brand ON data_retention_receipts(brand_id,family,id);
CREATE TABLE data_retention_daily (
 brand_id uuid REFERENCES brands(id),timezone text NOT NULL,day date NOT NULL,
 family text NOT NULL,dimensions jsonb NOT NULL,totals jsonb NOT NULL,
 PRIMARY KEY(brand_id,timezone,day,family,dimensions)
);
CREATE FUNCTION retention_add_totals(a jsonb,b jsonb) RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$
 SELECT jsonb_object_agg(key,trim_scale(value)::text) FROM (
 SELECT key,sum(value::numeric) value FROM (SELECT * FROM jsonb_each_text(a) UNION ALL SELECT * FROM jsonb_each_text(b)) v GROUP BY key) s
$$;
-- A compact global key registry keeps FK scope and business uniqueness after
-- operational details are removed. This is not a second copy of the payload.
DO $keys$ DECLARE name text; cols text[]; c record; col text; definitions text; BEGIN
 FOREACH name IN ARRAY ARRAY['configured_orders','point_ledger_entries','cash_orders','cash_entries','audit_logs','recharge_orders','withdrawal_orders'] LOOP
  SELECT array_agg(DISTINCT a.attname::text) INTO cols FROM pg_constraint k
  CROSS JOIN LATERAL unnest(k.conkey) n JOIN pg_attribute a ON a.attrelid=k.conrelid AND a.attnum=n
  WHERE k.conrelid=name::regclass AND k.contype IN('p','u');
  IF name='cash_orders' THEN cols:=cols||ARRAY['member_id','kind','currency','state','reference','event_index']; END IF;
  SELECT array_agg(DISTINCT x) INTO cols FROM unnest(cols) x;
  SELECT string_agg(format('%I',x),',') INTO definitions FROM unnest(cols) x;
  EXECUTE format('CREATE TABLE retention_keys_%I AS SELECT %s FROM %I WITH NO DATA',name,definitions,name);
  FOR c IN SELECT k.conname,pg_get_constraintdef(k.oid) def FROM pg_constraint k WHERE k.conrelid=name::regclass AND k.contype IN('p','u') LOOP
   EXECUTE format('ALTER TABLE retention_keys_%I ADD CONSTRAINT %I %s',name,'retention_'||c.conname,c.def);
  END LOOP;
  EXECUTE format('INSERT INTO retention_keys_%I SELECT %s FROM %I',name,definitions,name);
 END LOOP;
END $keys$;
CREATE UNIQUE INDEX retention_cash_active_withdrawal ON retention_keys_cash_orders(brand_id,member_id) WHERE kind='withdrawal' AND state IN('reviewing','processing');
CREATE UNIQUE INDEX retention_cash_deposit_reference ON retention_keys_cash_orders(brand_id,currency,reference) WHERE kind='deposit' AND currency<>'USDT' AND state='confirmed' AND reference<>'';
CREATE UNIQUE INDEX retention_cash_usdt_reference ON retention_keys_cash_orders(reference,event_index) WHERE kind='deposit' AND currency='USDT' AND state='confirmed' AND reference<>'';
CREATE UNIQUE INDEX retention_cash_payout_reference ON retention_keys_cash_orders(brand_id,currency,reference) WHERE kind='withdrawal' AND state='paid' AND reference<>'';
CREATE FUNCTION retention_capture_key() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE columns text; values_sql text; updates text; target text:='retention_keys_'||TG_TABLE_NAME;
BEGIN
 SELECT string_agg(format('%I',attname),',' ORDER BY attnum),
 string_agg(format('(jsonb_populate_record(NULL::%I,$1)).%I',target,attname),',' ORDER BY attnum),
 string_agg(format('%I=EXCLUDED.%I',attname,attname),',' ORDER BY attnum)
 INTO columns,values_sql,updates FROM pg_attribute WHERE attrelid=target::regclass AND attnum>0 AND NOT attisdropped;
 IF TG_OP='INSERT' THEN
  EXECUTE format('INSERT INTO %I(%s) SELECT %s',target,columns,values_sql) USING to_jsonb(NEW);
 ELSE
  EXECUTE format('UPDATE %I k SET (%s)=(SELECT %s) WHERE k.id=$2',target,columns,values_sql) USING to_jsonb(NEW),NEW.id;
 END IF;
 RETURN NEW;
END $$;
DO $keys$ DECLARE name text;c record; BEGIN
 FOREACH name IN ARRAY ARRAY['configured_orders','point_ledger_entries','cash_orders','cash_entries','audit_logs','recharge_orders','withdrawal_orders'] LOOP
  EXECUTE format('CREATE TRIGGER aaa_retention_key_capture AFTER INSERT OR UPDATE ON %I FOR EACH ROW EXECUTE FUNCTION retention_capture_key()',name);
 END LOOP;
 FOR c IN SELECT k.conrelid::regclass source,k.conname,pg_get_constraintdef(k.oid) def,t.relname target
 FROM pg_constraint k JOIN pg_class t ON t.oid=k.confrelid JOIN pg_namespace n ON n.oid=t.relnamespace
 WHERE k.contype='f' AND n.nspname=current_schema() AND t.relname=ANY(ARRAY['configured_orders','point_ledger_entries','cash_orders','cash_entries','audit_logs','recharge_orders','withdrawal_orders']) LOOP
  EXECUTE format('ALTER TABLE %s DROP CONSTRAINT %I',c.source,c.conname);
  EXECUTE format('ALTER TABLE %s ADD CONSTRAINT %I %s',c.source,c.conname,
   replace(replace(c.def,'REFERENCES '||c.target||'(','REFERENCES retention_keys_'||c.target||'('),' ON DELETE CASCADE',''));
 END LOOP;
END $keys$;
-- Typed fact functions preserve the original composite types used by balance
-- and settlement guards. There is no retry or fallback between the two stores.
DO $facts$ DECLARE name text; BEGIN
 FOREACH name IN ARRAY ARRAY['configured_orders','point_ledger_entries','cash_orders','cash_entries','audit_logs','recharge_orders','withdrawal_orders'] LOOP
  EXECUTE format('CREATE FUNCTION retention_%I() RETURNS SETOF %I LANGUAGE sql STABLE AS $body$ SELECT * FROM %I UNION ALL SELECT (jsonb_populate_record(NULL::%I,r.payload)).* FROM data_retention_receipts r WHERE r.family=%L $body$',name,name,name,name,name);
 END LOOP;
END $facts$;
-- Replace financial evidence reads in existing DB functions and views. Writes
-- continue to use operational tables. Limit the rewrite to FROM/JOIN clauses.
DO $facts$ DECLARE f record;name text;source text;changed text; BEGIN
 FOR f IN SELECT p.oid,pg_get_functiondef(p.oid) def FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
 WHERE n.nspname=current_schema() AND p.prokind='f' AND p.proname NOT LIKE 'retention_%' LOOP
  source:=f.def;changed:=source;
  FOREACH name IN ARRAY ARRAY['configured_orders','point_ledger_entries','cash_orders','cash_entries','audit_logs','recharge_orders','withdrawal_orders'] LOOP
   changed:=regexp_replace(changed,'\m(FROM|JOIN)\s+(\(*)'||name||'\M','\1 \2retention_'||name||'()','gi');
  END LOOP;
  IF changed<>source THEN EXECUTE changed; END IF;
 END LOOP;
 FOR f IN SELECT viewname,definition FROM pg_views WHERE schemaname=current_schema() LOOP
  changed:=f.definition;
  FOREACH name IN ARRAY ARRAY['configured_orders','point_ledger_entries','cash_orders','cash_entries','audit_logs','recharge_orders','withdrawal_orders'] LOOP
   changed:=regexp_replace(changed,'\m(FROM|JOIN)\s+(\(*)'||name||'\M','\1 \2retention_'||name||'()','gi');
  END LOOP;
  IF changed<>f.definition THEN EXECUTE format('CREATE OR REPLACE VIEW %I AS %s',f.viewname,changed); END IF;
 END LOOP;
END $facts$;
CREATE FUNCTION retention_delete_allowed(family text,old_row jsonb) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(SELECT 1 FROM data_retention_receipts r JOIN data_retention_tasks t ON t.id=r.task_id
 WHERE r.family=$1 AND r.id=($2->>'id')::uuid AND r.creation_xid=pg_current_xact_id()
 AND t.state='pending' AND r.brand_id IS NOT DISTINCT FROM t.brand_id
 AND r.original_sha256=encode(sha256(convert_to($2::text,'UTF8')),'hex'))
$$;
-- Existing immutable guards admit only an exact receipt captured in the same
-- task transaction. Normal UPDATE/DELETE remains forbidden.
DO $guards$ DECLARE f record;source text; BEGIN
 FOR f IN SELECT DISTINCT p.oid,pg_get_functiondef(p.oid) def FROM pg_trigger g JOIN pg_proc p ON p.oid=g.tgfoid
 JOIN pg_class t ON t.oid=g.tgrelid JOIN pg_namespace n ON n.oid=t.relnamespace
 WHERE n.nspname=current_schema() AND NOT g.tgisinternal AND (g.tgtype::int & 8)<>0
 AND t.relname=ANY(ARRAY['configured_orders','point_ledger_entries','cash_orders','cash_entries','audit_logs','recharge_orders','withdrawal_orders','notifications']) LOOP
  source:=regexp_replace(f.def,'\mBEGIN\M',E'BEGIN\n IF TG_OP=''DELETE'' AND retention_delete_allowed(TG_TABLE_NAME,to_jsonb(OLD)) THEN RETURN OLD; END IF;','i');
  EXECUTE source;
 END LOOP;
END $guards$;
CREATE TRIGGER retention_receipt_immutable BEFORE UPDATE OR DELETE ON data_retention_receipts FOR EACH ROW EXECUTE FUNCTION reject_immutable_change();
DO $pin$ DECLARE f record; BEGIN
 FOR f IN SELECT p.oid::regprocedure signature FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
 WHERE n.nspname=current_schema() AND (p.proname LIKE 'retention_%') LOOP
 EXECUTE format('ALTER FUNCTION %s SET search_path=pg_catalog,%I,pg_temp',f.signature,current_schema()); END LOOP;
END $pin$;
ALTER TABLE data_retention_receipts ADD COLUMN discard boolean NOT NULL DEFAULT false;
CREATE FUNCTION retention_payload(family text,body jsonb) RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$
 SELECT CASE $1
 WHEN 'notifications' THEN '{}'::jsonb
 WHEN 'configured_orders' THEN $2||jsonb_build_object('snapshot',jsonb_build_object('catalog',jsonb_build_object('funding',$2->'snapshot'->'catalog'->'funding')),
  'quote',($2->'quote')-ARRAY['lines'],'sales_snapshot','{}'::jsonb)
 WHEN 'cash_orders' THEN $2||jsonb_build_object('destination',CASE WHEN $2->'destination' ? 'number' THEN jsonb_build_object('number',$2->'destination'->'number') ELSE '{}'::jsonb END,
  'proof','','evidence','{}'::jsonb,'sender','')
 WHEN 'audit_logs' THEN $2-'ip_address'
 ELSE $2 END
$$;
CREATE FUNCTION retention_guard_receipt() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE body jsonb;t data_retention_tasks;at_time timestamptz; BEGIN
 SELECT * INTO t FROM data_retention_tasks WHERE id=NEW.task_id FOR SHARE;
 IF t.id IS NULL OR t.state<>'pending' OR NEW.brand_id IS DISTINCT FROM t.brand_id OR NEW.creation_xid<>pg_current_xact_id() THEN RAISE EXCEPTION 'retention receipt requires current task'; END IF;
 EXECUTE format('SELECT to_jsonb(x) FROM %I x WHERE id=$1 FOR UPDATE',NEW.family) INTO body USING NEW.id;
 at_time:=coalesce((body->>'placed_at')::timestamptz,(body->>'created_at')::timestamptz);
 IF body IS NULL OR at_time IS NULL OR at_time>=t.cutoff_at OR
  (body->>'brand_id')::uuid IS DISTINCT FROM NEW.brand_id OR
  NEW.original_sha256<>encode(sha256(convert_to(body::text,'UTF8')),'hex') OR
  NEW.payload IS DISTINCT FROM retention_payload(NEW.family,body) OR
  (NEW.discard AND NOT(NEW.family='notifications' OR NEW.family='audit_logs' AND body->>'resource_type'='query')) THEN
  RAISE EXCEPTION 'retention receipt does not match eligible source';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER retention_receipt_capture BEFORE INSERT ON data_retention_receipts FOR EACH ROW EXECUTE FUNCTION retention_guard_receipt();
-- Only the temporary manifest for an ordinary query audit may be discarded.
CREATE FUNCTION retention_guard_receipt_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' AND OLD.discard AND OLD.creation_xid=pg_current_xact_id() AND ((OLD.family='audit_logs' AND NOT EXISTS(SELECT 1 FROM audit_logs WHERE id=OLD.id)) OR (OLD.family='notifications' AND NOT EXISTS(SELECT 1 FROM notifications WHERE id=OLD.id))) THEN RETURN OLD; END IF;
 RAISE EXCEPTION 'economic receipts are immutable';
END $$;
DROP TRIGGER retention_receipt_immutable ON data_retention_receipts;
CREATE TRIGGER retention_receipt_immutable BEFORE UPDATE OR DELETE ON data_retention_receipts FOR EACH ROW EXECUTE FUNCTION retention_guard_receipt_change();
CREATE FUNCTION retention_guard_key() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE body jsonb;name text:=substr(TG_TABLE_NAME,length('retention_keys_')+1); BEGIN
 IF TG_OP='DELETE' THEN
  IF name='audit_logs' AND EXISTS(SELECT 1 FROM data_retention_receipts r WHERE r.family=name AND r.id=OLD.id AND r.discard AND r.creation_xid=pg_current_xact_id()) THEN RETURN OLD; END IF;
  RAISE EXCEPTION 'retained economic keys cannot be deleted';
 END IF;
 EXECUTE format('SELECT to_jsonb(x) FROM %I x WHERE id=$1',name) INTO body USING NEW.id;
 IF body IS NULL OR NOT body @> to_jsonb(NEW) THEN RAISE EXCEPTION 'retained key must match current source'; END IF;
 RETURN NEW;
END $$;
DO $keys$ DECLARE name text;BEGIN
 FOREACH name IN ARRAY ARRAY['configured_orders','point_ledger_entries','cash_orders','cash_entries','audit_logs','recharge_orders','withdrawal_orders'] LOOP
  EXECUTE format('CREATE TRIGGER retention_key_guard BEFORE INSERT OR UPDATE OR DELETE ON retention_keys_%I FOR EACH ROW EXECUTE FUNCTION retention_guard_key()',name);
 END LOOP;
END $keys$;
DO $pin$ DECLARE f record; BEGIN
 FOR f IN SELECT p.oid::regprocedure signature FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=current_schema() AND p.proname LIKE 'retention_%' LOOP EXECUTE format('ALTER FUNCTION %s SET search_path=pg_catalog,%I,pg_temp',f.signature,current_schema()); END LOOP;
END $pin$;
CREATE FUNCTION retention_json_hash(v jsonb) RETURNS text LANGUAGE sql IMMUTABLE AS $$ SELECT encode(sha256(convert_to(v::text,'UTF8')),'hex') $$;
ALTER TABLE data_retention_daily DROP CONSTRAINT data_retention_daily_pkey;
ALTER TABLE data_retention_daily ALTER COLUMN brand_id DROP NOT NULL;
ALTER TABLE data_retention_daily ADD COLUMN dimensions_hash text GENERATED ALWAYS AS(retention_json_hash(dimensions)) STORED;
ALTER TABLE data_retention_daily ADD CONSTRAINT data_retention_daily_pkey UNIQUE NULLS NOT DISTINCT(brand_id,timezone,day,family,dimensions_hash);
CREATE FUNCTION retention_report_timezone(b uuid,f timestamptz,t timestamptz) RETURNS text LANGUAGE plpgsql STABLE AS $$
DECLARE zones text[];zone text;bound timestamptz;BEGIN
 SELECT array_agg(DISTINCT timezone) INTO zones FROM data_retention_daily d WHERE d.brand_id=b AND
 (d.day::timestamp AT TIME ZONE d.timezone)<t AND ((d.day+1)::timestamp AT TIME ZONE d.timezone)>f;
 IF zones IS NULL THEN SELECT timezone INTO zone FROM brands WHERE id=b;RETURN zone; END IF;
 IF cardinality(zones)<>1 THEN RAISE EXCEPTION 'history requires one frozen timezone' USING ERRCODE='P7401';END IF;
 zone:=zones[1];
 FOREACH bound IN ARRAY ARRAY[f,t] LOOP
 IF (bound AT TIME ZONE zone)::time<>'00:00:00' THEN RAISE EXCEPTION 'history requires complete local days' USING ERRCODE='P7401';END IF;
 END LOOP;
 RETURN zone;
END $$;
CREATE VIEW retention_betting_report_facts AS
 SELECT b.brand_id,b.brand_member_id,b.game_id,b.placed_at,b.status,b.total_points,b.prize_points,b.refund_points,b.effective_points,b.attribution_snapshot,1::numeric weight
 FROM betting_financial_facts b JOIN configured_orders live ON live.id=b.id
 UNION ALL
 SELECT d.brand_id,(dimensions->>'member_id')::uuid,(dimensions->>'game_id')::uuid,d.day::timestamp AT TIME ZONE d.timezone,
 dimensions->>'status',(totals->>'stake_points')::numeric,(totals->>'prize_points')::numeric,(totals->>'refund_points')::numeric,
 (totals->>'effective_points')::numeric,dimensions->'attribution_snapshot',(totals->>'order_count')::numeric
 FROM data_retention_daily d WHERE family='configured_orders';
CREATE VIEW retention_ledger_report_facts AS
 SELECT l.brand_id,l.member_id,l.entry_type,l.created_at,1::numeric weight,n.net_points,
 greatest(n.net_points,0) positive_points,greatest(-n.net_points,0) negative_points
 FROM point_ledger_entries l CROSS JOIN LATERAL(SELECT sum(v.value::numeric) net_points FROM jsonb_each(l.delta_snapshot) s CROSS JOIN LATERAL jsonb_each_text(s.value) v) n
 UNION ALL
 SELECT d.brand_id,(dimensions->>'member_id')::uuid,dimensions->>'entry_type',d.day::timestamp AT TIME ZONE d.timezone,
 (totals->>'entry_count')::numeric,(totals->>'net_points')::numeric,(totals->>'positive_points')::numeric,(totals->>'negative_points')::numeric
 FROM data_retention_daily d WHERE family='point_ledger_entries';
DO $pin$ DECLARE f record;BEGIN
 FOR f IN SELECT p.oid::regprocedure signature FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=current_schema() AND p.proname LIKE 'retention_%' LOOP EXECUTE format('ALTER FUNCTION %s SET search_path=pg_catalog,%I,pg_temp',f.signature,current_schema()); END LOOP;
END $pin$;
ALTER TABLE idempotency_requests ADD COLUMN archived_at timestamptz;
ALTER TABLE platform_idempotency_requests ADD COLUMN archived_at timestamptz;
CREATE VIEW retention_withdrawal_report_facts AS
 SELECT brand_id,member_id,created_at,state,points,1::numeric weight FROM withdrawal_orders
 UNION ALL
 SELECT d.brand_id,(dimensions->>'member_id')::uuid,d.day::timestamp AT TIME ZONE d.timezone,dimensions->>'state',(totals->>'points')::numeric,(totals->>'order_count')::numeric
 FROM data_retention_daily d WHERE family='withdrawal_orders';
CREATE FUNCTION retention_add_snapshots(a jsonb,b jsonb) RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$
 SELECT jsonb_object_agg(src,states) FROM (
 SELECT s.key src,jsonb_object_agg(v.key,trim_scale(v.value::numeric+($2->s.key->>v.key)::numeric)::text) states
 FROM jsonb_each($1) s CROSS JOIN LATERAL jsonb_each_text(s.value) v GROUP BY s.key) totals
$$;
CREATE TABLE data_retention_account_days (
 brand_id uuid NOT NULL,account_id uuid NOT NULL,timezone text NOT NULL,day date NOT NULL,
 first_version bigint NOT NULL,last_version bigint NOT NULL,entry_count bigint NOT NULL,
 opening_snapshot jsonb NOT NULL,delta_snapshot jsonb NOT NULL,closing_snapshot jsonb NOT NULL,
 contiguous boolean GENERATED ALWAYS AS(entry_count=last_version-first_version+1) STORED,
 PRIMARY KEY(brand_id,account_id,timezone,day),FOREIGN KEY(brand_id,account_id) REFERENCES point_accounts(brand_id,id),
 CHECK(entry_count>0 AND first_version>0 AND last_version>=first_version),
 CHECK(NOT contiguous OR retention_add_snapshots(opening_snapshot,delta_snapshot)=closing_snapshot)
);
DO $pin$ DECLARE f record;BEGIN
 FOR f IN SELECT p.oid::regprocedure signature FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=current_schema() AND p.proname LIKE 'retention_%' LOOP EXECUTE format('ALTER FUNCTION %s SET search_path=pg_catalog,%I,pg_temp',f.signature,current_schema()); END LOOP;
END $pin$;
