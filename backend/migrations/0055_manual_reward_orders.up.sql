-- Manual rewards are business orders, not generic wallet adjustments. No
-- scheduler or automatic promotion/revocation is introduced by this migration.
CREATE TABLE reward_orders (
 id uuid PRIMARY KEY, brand_id uuid NOT NULL, member_id uuid NOT NULL,
 points bigint NOT NULL CHECK(points>0), state text NOT NULL CHECK(state IN('granted','revocation_pending','revoked')),
 version bigint NOT NULL DEFAULT 1 CHECK(version BETWEEN 1 AND 9007199254740991),
 grant_ledger_entry_id uuid UNIQUE REFERENCES point_ledger_entries(id),
 revoke_ledger_entry_id uuid UNIQUE REFERENCES point_ledger_entries(id),
 creation_audit_log_id uuid NOT NULL UNIQUE REFERENCES audit_logs(id), last_audit_log_id uuid NOT NULL REFERENCES audit_logs(id),
 created_by uuid NOT NULL REFERENCES admin_accounts(id), reason text NOT NULL CHECK(length(reason)>0 AND octet_length(reason)<=500),
 point_policy_version bigint NOT NULL CHECK(point_policy_version>0), last_error_code text,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_at timestamptz NOT NULL DEFAULT clock_timestamp(), revoked_at timestamptz,
 creation_xid xid8 NOT NULL DEFAULT pg_current_xact_id(), UNIQUE(brand_id,id),
 FOREIGN KEY(brand_id,member_id) REFERENCES brand_members(brand_id,id),
 CHECK((state='revoked')=(revoke_ledger_entry_id IS NOT NULL)), CHECK((state='revoked')=(revoked_at IS NOT NULL)),
 CHECK((state='revocation_pending')=(last_error_code IS NOT NULL)),
 CHECK(last_error_code IS NULL OR last_error_code='REWARD_AVAILABLE_INSUFFICIENT'),
 CHECK(state<>'granted' OR version=1)
);
CREATE INDEX reward_orders_page ON reward_orders(brand_id,created_at DESC,id DESC);
CREATE TABLE reward_order_actions (
 id uuid PRIMARY KEY, brand_id uuid NOT NULL, order_id uuid NOT NULL,
 version bigint NOT NULL CHECK(version BETWEEN 1 AND 9007199254740991),
 operation text NOT NULL CHECK(operation IN('grant','revoke','retry')), state_before text,
 state_after text NOT NULL CHECK(state_after IN('granted','revocation_pending','revoked')),
 actor_id uuid NOT NULL REFERENCES admin_accounts(id), reason text NOT NULL CHECK(length(reason)>0 AND octet_length(reason)<=500),
 audit_log_id uuid NOT NULL UNIQUE REFERENCES audit_logs(id), ledger_entry_id uuid UNIQUE REFERENCES point_ledger_entries(id),
 creation_xid xid8 NOT NULL DEFAULT pg_current_xact_id(), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(brand_id,order_id,version), FOREIGN KEY(brand_id,order_id) REFERENCES reward_orders(brand_id,id),
 CHECK((operation='grant' AND version=1 AND state_before IS NULL AND state_after='granted') OR
  (operation='revoke' AND version>1 AND state_before='granted' AND state_after IN('revocation_pending','revoked')) OR
  (operation='retry' AND version>1 AND state_before='revocation_pending' AND state_after IN('revocation_pending','revoked')))
);

CREATE FUNCTION guard_reward_order() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE log audit_logs; a reward_order_actions;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'reward order history immutable'; END IF;
 PERFORM 1 FROM brands WHERE id=NEW.brand_id AND status<>'disabled' FOR SHARE NOWAIT;
 IF NOT FOUND THEN RAISE EXCEPTION 'reward brand does not admit writes'; END IF;
 IF TG_OP='INSERT' THEN
  PERFORM 1 FROM admin_accounts WHERE id=NEW.created_by AND status='active' AND NOT is_super_admin FOR SHARE NOWAIT;
  IF NOT FOUND THEN RAISE EXCEPTION 'reward grant requires active ordinary operator'; END IF;
  SELECT * INTO log FROM audit_logs WHERE id=NEW.creation_audit_log_id;
  IF NEW.version<>1 OR NEW.state<>'granted' OR NEW.grant_ledger_entry_id IS NOT NULL OR NEW.revoke_ledger_entry_id IS NOT NULL OR
   NEW.last_audit_log_id<>NEW.creation_audit_log_id OR log.brand_id IS DISTINCT FROM NEW.brand_id OR log.actor_type IS DISTINCT FROM 'admin' OR
   log.actor_id IS DISTINCT FROM NEW.created_by OR log.action IS DISTINCT FROM 'reward.order.grant' OR log.resource_type IS DISTINCT FROM 'reward_order' OR
   log.resource_id IS DISTINCT FROM NEW.id OR log.reason IS DISTINCT FROM NEW.reason OR log.after_json->>'version' IS DISTINCT FROM '1' OR
   log.after_json->>'state' IS DISTINCT FROM 'granted' OR log.after_json->>'member_id' IS DISTINCT FROM NEW.member_id::text OR
   log.after_json->>'points' IS DISTINCT FROM NEW.points::text THEN RAISE EXCEPTION 'reward grant requires matching manual approval audit'; END IF;
  NEW.creation_xid:=pg_current_xact_id();
 ELSE
  IF OLD.grant_ledger_entry_id IS NULL AND NEW.grant_ledger_entry_id IS NOT NULL AND OLD.creation_xid=pg_current_xact_id() AND
   (to_jsonb(NEW)-'grant_ledger_entry_id') IS NOT DISTINCT FROM (to_jsonb(OLD)-'grant_ledger_entry_id') THEN RETURN NEW; END IF;
  IF (to_jsonb(NEW)-ARRAY['state','version','revoke_ledger_entry_id','last_audit_log_id','last_error_code','revoked_at','updated_at']) IS DISTINCT FROM
   (to_jsonb(OLD)-ARRAY['state','version','revoke_ledger_entry_id','last_audit_log_id','last_error_code','revoked_at','updated_at']) OR
   OLD.grant_ledger_entry_id IS NULL OR NEW.version<>OLD.version+1 OR OLD.state='revoked' OR NEW.state NOT IN('revocation_pending','revoked') THEN
   RAISE EXCEPTION 'reward state, version or immutable identity conflict'; END IF;
  SELECT * INTO a FROM reward_order_actions WHERE brand_id=NEW.brand_id AND order_id=NEW.id AND version=NEW.version;
  IF a.id IS NULL OR a.state_before IS DISTINCT FROM OLD.state OR a.state_after IS DISTINCT FROM NEW.state OR
   a.audit_log_id IS DISTINCT FROM NEW.last_audit_log_id OR (NEW.state='revoked' AND a.ledger_entry_id IS DISTINCT FROM NEW.revoke_ledger_entry_id) OR
   (NEW.state='revocation_pending' AND a.ledger_entry_id IS NOT NULL) THEN RAISE EXCEPTION 'reward state requires explicit immutable action'; END IF;
  NEW.updated_at:=clock_timestamp();
  NEW.revoked_at:=CASE WHEN NEW.state='revoked' THEN NEW.updated_at ELSE NULL END;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_reward_order BEFORE INSERT OR UPDATE OR DELETE ON reward_orders FOR EACH ROW EXECUTE FUNCTION guard_reward_order();

CREATE FUNCTION guard_reward_action() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE o reward_orders; log audit_logs; l point_ledger_entries;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'reward actions immutable'; END IF;
 IF TG_OP='UPDATE' THEN
  IF OLD.creation_xid<>pg_current_xact_id() OR OLD.ledger_entry_id IS NOT NULL OR NEW.ledger_entry_id IS NULL OR
   (to_jsonb(NEW)-'ledger_entry_id') IS DISTINCT FROM (to_jsonb(OLD)-'ledger_entry_id') THEN RAISE EXCEPTION 'reward action may only bind ledger in creation transaction'; END IF;
  SELECT * INTO l FROM point_ledger_entries WHERE id=NEW.ledger_entry_id;
  IF l.brand_id IS DISTINCT FROM NEW.brand_id OR l.reference_type IS DISTINCT FROM 'reward_order' OR l.reference_id IS DISTINCT FROM NEW.order_id OR
   l.actor_type IS DISTINCT FROM 'admin' OR l.actor_id IS DISTINCT FROM NEW.actor_id OR l.reason IS DISTINCT FROM NEW.reason OR
   l.entry_type IS DISTINCT FROM (CASE NEW.operation WHEN 'grant' THEN 'reward_grant' ELSE 'reward_reversal' END) THEN RAISE EXCEPTION 'reward action ledger mismatch'; END IF;
  RETURN NEW;
 END IF;
 SELECT * INTO o FROM reward_orders WHERE brand_id=NEW.brand_id AND id=NEW.order_id FOR UPDATE NOWAIT;
 PERFORM 1 FROM admin_accounts WHERE id=NEW.actor_id AND status='active' AND NOT is_super_admin FOR SHARE NOWAIT;
 IF NOT FOUND THEN RAISE EXCEPTION 'reward action requires active ordinary operator'; END IF;
 IF NEW.operation<>'grant' THEN
  PERFORM 1 FROM point_accounts WHERE brand_id=NEW.brand_id AND brand_member_id=o.member_id FOR UPDATE NOWAIT;
  IF NOT FOUND OR (NEW.state_after='revocation_pending' AND NOT EXISTS(
   SELECT 1 FROM point_buckets b JOIN point_accounts p ON p.brand_id=b.brand_id AND p.id=b.account_id
   WHERE p.brand_id=NEW.brand_id AND p.brand_member_id=o.member_id AND b.source='gift' AND b.state='available' AND b.points<o.points)) THEN
   RAISE EXCEPTION 'reward pending requires original gift available shortage'; END IF;
 END IF;
 SELECT * INTO log FROM audit_logs WHERE id=NEW.audit_log_id;
 IF o.id IS NULL OR NEW.ledger_entry_id IS NOT NULL OR
  (NEW.operation='grant' AND (o.creation_xid<>pg_current_xact_id() OR o.version<>1 OR o.state<>'granted' OR NEW.actor_id<>o.created_by OR NEW.audit_log_id<>o.creation_audit_log_id)) OR
  (NEW.operation<>'grant' AND (o.grant_ledger_entry_id IS NULL OR NEW.version<>o.version+1 OR NEW.state_before IS DISTINCT FROM o.state)) OR
  log.brand_id IS DISTINCT FROM NEW.brand_id OR log.actor_type IS DISTINCT FROM 'admin' OR log.actor_id IS DISTINCT FROM NEW.actor_id OR
  log.action IS DISTINCT FROM 'reward.order.'||NEW.operation OR log.resource_type IS DISTINCT FROM 'reward_order' OR log.resource_id IS DISTINCT FROM NEW.order_id OR
  log.reason IS DISTINCT FROM NEW.reason OR log.after_json->>'action_id' IS DISTINCT FROM NEW.id::text OR
  log.after_json->>'version' IS DISTINCT FROM NEW.version::text OR log.after_json->>'state' IS DISTINCT FROM NEW.state_after OR
  (NEW.operation<>'grant' AND (log.before_json->>'version' IS DISTINCT FROM o.version::text OR log.before_json->>'state' IS DISTINCT FROM o.state)) THEN
  RAISE EXCEPTION 'reward action requires matching explicit operator audit and version'; END IF;
 NEW.creation_xid:=pg_current_xact_id();RETURN NEW;
END $$;
CREATE TRIGGER guarded_reward_action BEFORE INSERT OR UPDATE OR DELETE ON reward_order_actions FOR EACH ROW EXECUTE FUNCTION guard_reward_action();

CREATE FUNCTION guard_reward_credit() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE o reward_orders; a reward_order_actions; expected jsonb; policy brand_point_policies; total_after numeric;
BEGIN
 IF NEW.entry_type NOT IN('reward_grant','reward_reversal') AND NEW.reference_type<>'reward_order' AND
  NOT EXISTS(SELECT 1 FROM point_ledger_entries WHERE id=NEW.reversal_of AND entry_type='reward_grant') THEN RETURN NEW; END IF;
 SELECT * INTO o FROM reward_orders WHERE brand_id=NEW.brand_id AND id=NEW.reference_id FOR UPDATE NOWAIT;
 SELECT * INTO a FROM reward_order_actions WHERE brand_id=NEW.brand_id AND order_id=o.id AND
  version=CASE NEW.entry_type WHEN 'reward_grant' THEN 1 ELSE o.version+1 END;
 PERFORM 1 FROM brands WHERE id=NEW.brand_id AND status<>'disabled' FOR SHARE NOWAIT;
 IF NOT FOUND OR o.id IS NULL OR a.id IS NULL OR a.creation_xid<>pg_current_xact_id() OR a.ledger_entry_id IS NOT NULL OR
  NEW.reference_type<>'reward_order' OR NEW.member_id IS DISTINCT FROM o.member_id OR NEW.actor_type IS DISTINCT FROM 'admin' OR
  NEW.actor_id IS DISTINCT FROM a.actor_id OR NEW.reason IS DISTINCT FROM a.reason THEN RAISE EXCEPTION 'reward posting requires current explicit business action'; END IF;
 IF NEW.entry_type='reward_grant' THEN
  IF o.state<>'granted' OR o.version<>1 OR o.creation_xid<>pg_current_xact_id() OR o.grant_ledger_entry_id IS NOT NULL OR a.operation<>'grant' OR
   NEW.operation_key IS DISTINCT FROM 'reward-grant:'||o.id::text OR NEW.reversal_of IS NOT NULL THEN RAISE EXCEPTION 'reward grant cannot repeat or change identity'; END IF;
  SELECT * INTO policy FROM brand_point_policies WHERE brand_id=NEW.brand_id FOR SHARE NOWAIT;
  SELECT sum(bucket.value::numeric) INTO total_after FROM jsonb_each(NEW.after_snapshot) src CROSS JOIN LATERAL jsonb_each_text(src.value) bucket;
  IF policy.version IS DISTINCT FROM o.point_policy_version OR policy.max_balance_points IS NOT NULL AND total_after>policy.max_balance_points THEN
   RAISE EXCEPTION 'reward grant current point policy mismatch'; END IF;
  expected:=jsonb_set(point_zero_snapshot(),'{gift,available}',to_jsonb(o.points::text));
 ELSIF NEW.entry_type='reward_reversal' THEN
  IF o.state NOT IN('granted','revocation_pending') OR o.grant_ledger_entry_id IS NULL OR o.revoke_ledger_entry_id IS NOT NULL OR
   a.operation NOT IN('revoke','retry') OR a.state_after<>'revoked' OR NEW.reversal_of IS DISTINCT FROM o.grant_ledger_entry_id OR
   NEW.operation_key IS DISTINCT FROM 'reward-revoke:'||o.id::text THEN RAISE EXCEPTION 'reward revocation must reverse original grant exactly once'; END IF;
  expected:=jsonb_set(point_zero_snapshot(),'{gift,available}',to_jsonb((-o.points)::text));
 ELSE RAISE EXCEPTION 'generic reversal cannot bypass reward revocation'; END IF;
 IF NEW.delta_snapshot IS DISTINCT FROM expected OR NEW.source_allocation IS DISTINCT FROM
  jsonb_build_array(jsonb_build_object('source','gift','state','available','points',o.points::text)) THEN RAISE EXCEPTION 'reward moves only exact gift available amount'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_reward_credit BEFORE INSERT ON point_ledger_entries FOR EACH ROW EXECUTE FUNCTION guard_reward_credit();

CREATE FUNCTION require_reward_order_commit() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE o reward_orders; a reward_order_actions;
BEGIN
 SELECT * INTO o FROM reward_orders WHERE id=NEW.id;
 SELECT * INTO a FROM reward_order_actions WHERE brand_id=o.brand_id AND order_id=o.id AND version=o.version;
 IF o.grant_ledger_entry_id IS NULL OR a.id IS NULL OR a.state_after<>o.state OR a.audit_log_id<>o.last_audit_log_id OR
  NOT EXISTS(SELECT 1 FROM reward_order_actions g JOIN point_ledger_entries l ON l.id=g.ledger_entry_id WHERE g.brand_id=o.brand_id AND g.order_id=o.id AND g.operation='grant' AND g.version=1 AND
   g.ledger_entry_id=o.grant_ledger_entry_id AND l.brand_id=o.brand_id AND l.member_id=o.member_id AND l.reference_id=o.id AND l.reference_type='reward_order' AND l.entry_type='reward_grant') OR
  (o.state='revoked' AND a.ledger_entry_id IS DISTINCT FROM o.revoke_ledger_entry_id) OR
  (o.state='revocation_pending' AND a.ledger_entry_id IS NOT NULL) THEN RAISE EXCEPTION 'orphan or incomplete reward order cannot commit'; END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER reward_order_commit AFTER INSERT OR UPDATE ON reward_orders DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_reward_order_commit();
CREATE FUNCTION require_reward_action_commit() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE a reward_order_actions; o reward_orders;
BEGIN
 SELECT * INTO a FROM reward_order_actions WHERE id=NEW.id;
 SELECT * INTO o FROM reward_orders WHERE brand_id=a.brand_id AND id=a.order_id;
 IF o.id IS NULL OR a.version>o.version OR
  (a.state_after='revocation_pending' AND a.ledger_entry_id IS NOT NULL) OR
  (a.state_after<>'revocation_pending' AND (a.ledger_entry_id IS NULL OR NOT EXISTS(
   SELECT 1 FROM point_ledger_entries l WHERE l.id=a.ledger_entry_id AND l.brand_id=a.brand_id AND l.member_id=o.member_id AND
   l.reference_type='reward_order' AND l.reference_id=o.id AND l.actor_type='admin' AND l.actor_id=a.actor_id AND
   l.entry_type=CASE a.operation WHEN 'grant' THEN 'reward_grant' ELSE 'reward_reversal' END))) THEN RAISE EXCEPTION 'orphan reward action cannot commit'; END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER reward_action_commit AFTER INSERT ON reward_order_actions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_reward_action_commit();
CREATE FUNCTION require_reward_ledger_commit() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.entry_type IN('reward_grant','reward_reversal') OR NEW.reference_type='reward_order' THEN
  IF NOT EXISTS(SELECT 1 FROM reward_order_actions WHERE brand_id=NEW.brand_id AND order_id=NEW.reference_id AND ledger_entry_id=NEW.id) THEN
   RAISE EXCEPTION 'orphan reward ledger cannot commit'; END IF;
 END IF; RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER reward_ledger_commit AFTER INSERT ON point_ledger_entries DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
 WHEN (NEW.entry_type IN('reward_grant','reward_reversal') OR NEW.reference_type='reward_order') EXECUTE FUNCTION require_reward_ledger_commit();
INSERT INTO permissions(key) VALUES('reward.view.brand'),('reward.view.platform'),('reward.grant.brand'),('reward.revoke.brand'),('reward.retry.brand') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key) SELECT r.id,p.key FROM roles r CROSS JOIN permissions p WHERE r.is_bootstrap AND
 ((r.brand_id IS NOT NULL AND p.key IN('reward.view.brand','reward.grant.brand','reward.revoke.brand','reward.retry.brand')) OR
 (r.brand_id IS NULL AND p.key='reward.view.platform')) ON CONFLICT DO NOTHING;
DO $$
DECLARE app_schema text:=current_schema(); f record;
BEGIN
 FOR f IN SELECT p.oid::regprocedure signature FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=app_schema AND p.proname IN(
  'guard_reward_order','guard_reward_action','guard_reward_credit','require_reward_order_commit','require_reward_action_commit','require_reward_ledger_commit') LOOP
  EXECUTE format('ALTER FUNCTION %s SET search_path TO pg_catalog, %I, pg_temp',f.signature,app_schema);
 END LOOP;
END $$;
