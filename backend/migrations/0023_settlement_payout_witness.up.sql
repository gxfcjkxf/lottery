-- SQL NULL must fail closed: an incomplete JSON delta is not a zero delta.
-- This is additive so previously applied 0022 migration checksums stay intact.
CREATE FUNCTION validate_settlement_payout_witness() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE l point_ledger_entries; zero_bucket jsonb; expected jsonb; src text; bucket text;
BEGIN
 IF OLD.status<>'placed' OR NEW.status NOT IN ('won','lost') OR NEW.prize_points=0 THEN RETURN NEW; END IF;
 SELECT * INTO l FROM point_ledger_entries WHERE id=NEW.payout_entry_id;
 zero_bucket:=jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal','0');
 expected:=jsonb_build_object('recharge',zero_bucket,'gift',zero_bucket,'winning',zero_bucket||jsonb_build_object('available',NEW.prize_points::text));
 IF l.id IS NULL OR l.account_id IS DISTINCT FROM NEW.account_id OR l.delta_snapshot IS DISTINCT FROM expected OR l.operation_key IS DISTINCT FROM 'settlement-payout:'||NEW.settlement_calculation_id::text OR
 jsonb_typeof(l.before_snapshot) IS DISTINCT FROM 'object' OR jsonb_typeof(l.after_snapshot) IS DISTINCT FROM 'object' THEN RAISE EXCEPTION 'incomplete settlement payout witness'; END IF;
 IF (SELECT count(*) FROM jsonb_object_keys(l.before_snapshot))<>3 OR (SELECT count(*) FROM jsonb_object_keys(l.after_snapshot))<>3 THEN RAISE EXCEPTION 'invalid payout snapshot sources'; END IF;
 FOREACH src IN ARRAY ARRAY['recharge','winning','gift'] LOOP
  IF jsonb_typeof(l.before_snapshot->src) IS DISTINCT FROM 'object' OR jsonb_typeof(l.after_snapshot->src) IS DISTINCT FROM 'object' THEN RAISE EXCEPTION 'missing payout source snapshot'; END IF;
  IF (SELECT count(*) FROM jsonb_object_keys(l.before_snapshot->src))<>4 OR (SELECT count(*) FROM jsonb_object_keys(l.after_snapshot->src))<>4 THEN RAISE EXCEPTION 'invalid payout snapshot buckets'; END IF;
  FOREACH bucket IN ARRAY ARRAY['available','manual_frozen','system_frozen','withdrawal'] LOOP
   IF jsonb_typeof(l.before_snapshot->src->bucket) IS DISTINCT FROM 'string' OR jsonb_typeof(l.after_snapshot->src->bucket) IS DISTINCT FROM 'string' OR
   (l.before_snapshot->src->>bucket) !~ '^(0|[1-9][0-9]*)$' OR (l.after_snapshot->src->>bucket) !~ '^(0|[1-9][0-9]*)$' THEN RAISE EXCEPTION 'invalid payout integer snapshot'; END IF;
   IF (l.before_snapshot->src->>bucket)::numeric+(expected->src->>bucket)::numeric IS DISTINCT FROM (l.after_snapshot->src->>bucket)::numeric OR
   NOT EXISTS(SELECT 1 FROM point_buckets b WHERE b.brand_id=NEW.brand_id AND b.account_id=NEW.account_id AND b.source=src AND b.state=bucket AND b.points::numeric=(l.after_snapshot->src->>bucket)::numeric) THEN RAISE EXCEPTION 'payout snapshot must match applied buckets'; END IF;
  END LOOP;
 END LOOP;
 IF NOT EXISTS(SELECT 1 FROM point_accounts a WHERE a.brand_id=NEW.brand_id AND a.id=NEW.account_id AND a.version=l.version) OR
 NOT EXISTS(SELECT 1 FROM point_ledger_entries prev WHERE prev.brand_id=NEW.brand_id AND prev.account_id=NEW.account_id AND prev.version=l.version-1 AND prev.after_snapshot=l.before_snapshot) OR
 NOT EXISTS(SELECT 1 FROM audit_logs a WHERE a.brand_id=NEW.brand_id AND a.actor_type='system' AND a.action='points.prize' AND a.resource_type='point_account' AND a.resource_id=NEW.account_id AND a.before_json=l.before_snapshot AND a.after_json->>'ledger_entry_id'=l.id::text AND a.after_json->'balance'=l.after_snapshot) THEN RAISE EXCEPTION 'payout requires applied ledger tip and audit witness'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER settlement_payout_witness BEFORE UPDATE ON bet_orders FOR EACH ROW EXECUTE FUNCTION validate_settlement_payout_witness();
