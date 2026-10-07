-- Manual commission corrections are append-only differences, never rewrites
-- of a bet snapshot, earning, original target, approval or ledger posting.
ALTER TABLE commission_payment_targets ADD UNIQUE(brand_id,id);
CREATE TABLE commission_adjustment_heads (
 target_id uuid PRIMARY KEY,brand_id uuid NOT NULL,version bigint NOT NULL DEFAULT 1 CHECK(version BETWEEN 1 AND 9007199254740991),
 points bigint NOT NULL CHECK(points>=0),last_adjustment_id uuid,
 UNIQUE(brand_id,target_id),FOREIGN KEY(brand_id,target_id) REFERENCES commission_payment_targets(brand_id,id)
);
CREATE TABLE commission_adjustments (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,target_id uuid NOT NULL,payment_id uuid NOT NULL,
 version bigint NOT NULL CHECK(version BETWEEN 2 AND 9007199254740991),
 points_before bigint NOT NULL CHECK(points_before>=0),points_after bigint NOT NULL CHECK(points_after>=0),delta_points bigint NOT NULL CHECK(delta_points<>0),
 ledger_entry_id uuid REFERENCES point_ledger_entries(id),audit_log_id uuid NOT NULL REFERENCES audit_logs(id),
 created_by uuid NOT NULL REFERENCES admin_accounts(id),reason text NOT NULL CHECK(octet_length(reason) BETWEEN 1 AND 500 AND reason=trim(reason)),
 point_policy_version bigint NOT NULL CHECK(point_policy_version>0),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 creation_xid xid8 NOT NULL DEFAULT pg_current_xact_id(),
 UNIQUE(brand_id,id),UNIQUE(target_id,version),UNIQUE(ledger_entry_id),UNIQUE(brand_id,target_id,id),
 FOREIGN KEY(brand_id,target_id) REFERENCES commission_adjustment_heads(brand_id,target_id),
 FOREIGN KEY(brand_id,payment_id) REFERENCES commission_payments(brand_id,id),
 CHECK(points_after::numeric-points_before::numeric=delta_points::numeric)
);
ALTER TABLE commission_adjustment_heads ADD FOREIGN KEY(brand_id,target_id,last_adjustment_id) REFERENCES commission_adjustments(brand_id,target_id,id);
CREATE INDEX commission_adjustment_history ON commission_adjustments(brand_id,target_id,version DESC);

CREATE FUNCTION guard_commission_adjustment_head() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE t commission_payment_targets; a commission_adjustments;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'commission adjustment head cannot be deleted'; END IF;
 SELECT * INTO t FROM commission_payment_targets WHERE brand_id=NEW.brand_id AND id=NEW.target_id;
 IF t.state IS DISTINCT FROM 'paid' THEN RAISE EXCEPTION 'commission adjustment head requires completed target'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR NEW.points<>t.points OR NEW.last_adjustment_id IS NOT NULL THEN RAISE EXCEPTION 'commission head starts at original paid amount'; END IF;
 ELSE
  IF NEW.brand_id<>OLD.brand_id OR NEW.target_id<>OLD.target_id OR NEW.version<>OLD.version+1 OR NEW.points=OLD.points OR NEW.last_adjustment_id IS NULL THEN RAISE EXCEPTION 'commission adjustment version/identity mismatch'; END IF;
  SELECT * INTO a FROM commission_adjustments WHERE brand_id=NEW.brand_id AND target_id=NEW.target_id AND id=NEW.last_adjustment_id;
  IF a.id IS NULL OR a.version<>NEW.version OR a.points_before<>OLD.points OR a.points_after<>NEW.points OR a.ledger_entry_id IS NULL THEN RAISE EXCEPTION 'head requires completed immutable adjustment'; END IF;
 END IF; RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_adjustment_head BEFORE INSERT OR UPDATE OR DELETE ON commission_adjustment_heads FOR EACH ROW EXECUTE FUNCTION guard_commission_adjustment_head();
-- Metadata-only backfill: no historical wallet, payout or audit changes.
INSERT INTO commission_adjustment_heads(target_id,brand_id,points) SELECT id,brand_id,points FROM commission_payment_targets WHERE state='paid';
CREATE FUNCTION initialize_commission_adjustment_head() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.state='pending' AND NEW.state='paid' THEN INSERT INTO commission_adjustment_heads(target_id,brand_id,points) VALUES(NEW.id,NEW.brand_id,NEW.points); END IF;
 RETURN NULL;
END $$;
CREATE TRIGGER commission_adjustment_head_init AFTER UPDATE ON commission_payment_targets FOR EACH ROW EXECUTE FUNCTION initialize_commission_adjustment_head();

CREATE FUNCTION guard_commission_adjustment() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE h commission_adjustment_heads; t commission_payment_targets; p commission_payments; log audit_logs; policy brand_point_policies;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'commission adjustments immutable'; END IF;
 IF TG_OP='UPDATE' THEN
  IF OLD.ledger_entry_id IS NOT NULL OR NEW.ledger_entry_id IS NULL OR OLD.creation_xid<>pg_current_xact_id() OR
   (to_jsonb(NEW)-'ledger_entry_id') IS DISTINCT FROM (to_jsonb(OLD)-'ledger_entry_id') THEN RAISE EXCEPTION 'commission adjustment only binds its ledger once in its creation transaction'; END IF;
  RETURN NEW;
 END IF;
 SELECT * INTO t FROM commission_payment_targets WHERE brand_id=NEW.brand_id AND id=NEW.target_id;
 SELECT * INTO p FROM commission_payments WHERE brand_id=NEW.brand_id AND id=NEW.payment_id;
 PERFORM 1 FROM commission_cycles WHERE brand_id=NEW.brand_id AND id=p.cycle_id FOR UPDATE NOWAIT;
 IF NOT FOUND OR p.state IS DISTINCT FROM 'paid' OR t.state IS DISTINCT FROM 'paid' OR t.payment_id IS DISTINCT FROM NEW.payment_id OR
  NOT commission_payment_evidence_current(NEW.brand_id,p.cycle_id,p.run_id,p.evidence_epoch) THEN RAISE EXCEPTION 'manual adjustment requires complete current evidence, not a blocked correction'; END IF;
 PERFORM 1 FROM brands WHERE id=NEW.brand_id AND status<>'disabled' FOR SHARE NOWAIT;
 IF NOT FOUND THEN RAISE EXCEPTION 'brand does not admit manual commission adjustment'; END IF;
 PERFORM 1 FROM brand_commission_payment_policies WHERE brand_id=NEW.brand_id AND enabled FOR SHARE NOWAIT;
 IF NOT FOUND THEN RAISE EXCEPTION 'commission financial writes disabled'; END IF;
 SELECT * INTO h FROM commission_adjustment_heads WHERE brand_id=NEW.brand_id AND target_id=NEW.target_id FOR UPDATE NOWAIT;
 IF h.target_id IS NULL OR NEW.version<>h.version+1 OR NEW.points_before<>h.points OR NEW.ledger_entry_id IS NOT NULL THEN RAISE EXCEPTION 'manual adjustment needs current target version'; END IF;
 SELECT * INTO policy FROM brand_point_policies WHERE brand_id=NEW.brand_id FOR SHARE NOWAIT;
 IF policy.version IS DISTINCT FROM NEW.point_policy_version OR (policy.max_adjustment_points IS NOT NULL AND abs(NEW.delta_points::numeric)>policy.max_adjustment_points) THEN RAISE EXCEPTION 'commission adjustment exceeds captured point policy'; END IF;
 SELECT * INTO log FROM audit_logs WHERE id=NEW.audit_log_id;
 IF log.id IS NULL OR log.brand_id IS DISTINCT FROM NEW.brand_id OR log.actor_type<>'admin' OR log.actor_id IS DISTINCT FROM NEW.created_by OR
  log.action<>'commission.adjustment.create' OR log.resource_type<>'commission_adjustment' OR log.resource_id IS DISTINCT FROM NEW.id OR log.reason<>NEW.reason OR
  log.before_json->>'version' IS DISTINCT FROM h.version::text OR log.before_json->>'points' IS DISTINCT FROM NEW.points_before::text OR
  log.after_json->>'version' IS DISTINCT FROM NEW.version::text OR log.after_json->>'points' IS DISTINCT FROM NEW.points_after::text OR
  log.after_json->>'delta_points' IS DISTINCT FROM NEW.delta_points::text OR log.after_json->>'target_id' IS DISTINCT FROM NEW.target_id::text OR
  log.after_json->>'payment_id' IS DISTINCT FROM NEW.payment_id::text THEN RAISE EXCEPTION 'manual commission adjustment needs matching single-operator approval audit'; END IF;
 NEW.creation_xid:=pg_current_xact_id();RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_adjustment BEFORE INSERT OR UPDATE OR DELETE ON commission_adjustments FOR EACH ROW EXECUTE FUNCTION guard_commission_adjustment();

CREATE FUNCTION guard_commission_adjustment_credit() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE a commission_adjustments; t commission_payment_targets; p commission_payments; expected jsonb; policy brand_point_policies; total_after numeric;
BEGIN
 IF NEW.entry_type<>'commission_adjustment' AND NEW.reference_type<>'commission_adjustment' THEN RETURN NEW; END IF;
 SELECT * INTO a FROM commission_adjustments WHERE brand_id=NEW.brand_id AND id=NEW.reference_id FOR UPDATE NOWAIT;
 SELECT * INTO t FROM commission_payment_targets WHERE brand_id=NEW.brand_id AND id=a.target_id;
 SELECT * INTO p FROM commission_payments WHERE brand_id=NEW.brand_id AND id=a.payment_id FOR UPDATE NOWAIT;
 PERFORM 1 FROM commission_cycles WHERE brand_id=NEW.brand_id AND id=p.cycle_id FOR UPDATE NOWAIT;
 IF NOT FOUND OR a.id IS NULL OR a.creation_xid<>pg_current_xact_id() OR a.ledger_entry_id IS NOT NULL OR p.state IS DISTINCT FROM 'paid' OR
  NOT commission_payment_evidence_current(NEW.brand_id,p.cycle_id,p.run_id,p.evidence_epoch) THEN RAISE EXCEPTION 'adjustment posting requires its current approved business record'; END IF;
 PERFORM 1 FROM brand_commission_payment_policies WHERE brand_id=NEW.brand_id AND enabled FOR SHARE NOWAIT;
 IF NOT FOUND THEN RAISE EXCEPTION 'adjustment financial switch disabled'; END IF;
 SELECT * INTO policy FROM brand_point_policies WHERE brand_id=NEW.brand_id FOR SHARE NOWAIT;
 SELECT sum(bucket.value::text::numeric) INTO total_after FROM jsonb_each(NEW.after_snapshot) src CROSS JOIN LATERAL jsonb_each_text(src.value) bucket;
 IF policy.version IS DISTINCT FROM a.point_policy_version OR policy.max_balance_points IS NOT NULL AND total_after>policy.max_balance_points THEN RAISE EXCEPTION 'adjustment current balance policy mismatch'; END IF;
 expected:=jsonb_set(point_zero_snapshot(),'{commission,available}',to_jsonb(a.delta_points::text));
 IF NEW.entry_type<>'commission_adjustment' OR NEW.reference_type<>'commission_adjustment' OR NEW.member_id IS DISTINCT FROM t.member_id OR
  NEW.actor_type<>'admin' OR NEW.actor_id IS DISTINCT FROM a.created_by OR NEW.operation_key IS DISTINCT FROM 'commission-adjustment:'||a.id::text OR NEW.reversal_of IS NOT NULL OR
  NEW.delta_snapshot IS DISTINCT FROM expected OR NEW.source_allocation IS DISTINCT FROM jsonb_build_array(jsonb_build_object('source','commission','state','available','points',abs(a.delta_points)::text)) THEN RAISE EXCEPTION 'adjustment must post exact C-available difference for original beneficiary'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_commission_adjustment_credit BEFORE INSERT ON point_ledger_entries FOR EACH ROW EXECUTE FUNCTION guard_commission_adjustment_credit();
CREATE FUNCTION require_commission_adjustment_commit() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE a commission_adjustments;
BEGIN
 SELECT * INTO a FROM commission_adjustments WHERE id=NEW.id;
 IF a.ledger_entry_id IS NULL OR NOT EXISTS(SELECT 1 FROM commission_adjustment_heads h WHERE h.brand_id=a.brand_id AND h.target_id=a.target_id AND h.version>=a.version) OR
  NOT EXISTS(SELECT 1 FROM point_ledger_entries l JOIN commission_payment_targets t ON t.brand_id=a.brand_id AND t.id=a.target_id WHERE l.id=a.ledger_entry_id AND l.brand_id=a.brand_id AND l.member_id=t.member_id AND l.entry_type='commission_adjustment' AND l.reference_type='commission_adjustment' AND l.reference_id=a.id AND l.actor_type='admin' AND l.actor_id=a.created_by AND l.operation_key='commission-adjustment:'||a.id::text) THEN RAISE EXCEPTION 'orphan commission adjustment cannot commit'; END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER commission_adjustment_commit AFTER INSERT ON commission_adjustments DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_commission_adjustment_commit();
CREATE FUNCTION require_commission_adjustment_ledger_commit() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM commission_adjustments WHERE brand_id=NEW.brand_id AND id=NEW.reference_id AND ledger_entry_id=NEW.id) THEN RAISE EXCEPTION 'orphan commission adjustment ledger'; END IF;RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER commission_adjustment_ledger_commit AFTER INSERT ON point_ledger_entries DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
 WHEN (NEW.entry_type='commission_adjustment' OR NEW.reference_type='commission_adjustment') EXECUTE FUNCTION require_commission_adjustment_ledger_commit();
INSERT INTO permissions(key) VALUES('commission_adjustment.write.brand') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key) SELECT id,'commission_adjustment.write.brand' FROM roles WHERE is_bootstrap AND brand_id IS NOT NULL ON CONFLICT DO NOTHING;
DO $$
DECLARE app_schema text:=current_schema(); f record;
BEGIN
 FOR f IN SELECT p.oid::regprocedure signature FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=app_schema AND p.proname IN(
  'guard_commission_adjustment_head','initialize_commission_adjustment_head','guard_commission_adjustment','guard_commission_adjustment_credit',
  'require_commission_adjustment_commit','require_commission_adjustment_ledger_commit') LOOP
  EXECUTE format('ALTER FUNCTION %s SET search_path TO pg_catalog, %I, pg_temp',f.signature,app_schema);
 END LOOP;
END $$;
