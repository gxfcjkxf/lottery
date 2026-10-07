-- Internal financial workflow; no public eligibility or payout adapter is enabled.
CREATE TABLE withdrawal_orders (
 id uuid PRIMARY KEY, brand_id uuid NOT NULL, member_id uuid NOT NULL, account_id uuid NOT NULL,
 points bigint NOT NULL CHECK(points>0), state text NOT NULL CHECK(state IN('reviewing','processing','paid','rejected','failed','cancelled')),
 version bigint NOT NULL CHECK(version BETWEEN 1 AND 9007199254740991),
 source_allocation jsonb NOT NULL CHECK(jsonb_typeof(source_allocation)='array'),
 policy_snapshot jsonb NOT NULL CHECK(jsonb_typeof(policy_snapshot)='object'),
 eligibility_evidence jsonb NOT NULL CHECK(jsonb_typeof(eligibility_evidence)='object' AND octet_length(eligibility_evidence::text)<=16384),
 cycle_from_at timestamptz, cycle_from_version bigint NOT NULL CHECK(cycle_from_version>=0),
 reserve_version bigint NOT NULL CHECK(reserve_version>cycle_from_version),
 reserve_entry_id uuid NOT NULL UNIQUE, release_entry_id uuid UNIQUE, paid_entry_id uuid UNIQUE,
 created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL,
 reviewed_at timestamptz, completed_at timestamptz,
 decision_reason text NOT NULL DEFAULT '' CHECK(octet_length(decision_reason)<=500),
 FOREIGN KEY(brand_id,account_id,member_id) REFERENCES point_accounts(brand_id,id,brand_member_id),
 FOREIGN KEY(brand_id,account_id,reserve_entry_id) REFERENCES point_ledger_entries(brand_id,account_id,id),
 FOREIGN KEY(brand_id,account_id,release_entry_id) REFERENCES point_ledger_entries(brand_id,account_id,id),
 FOREIGN KEY(brand_id,account_id,paid_entry_id) REFERENCES point_ledger_entries(brand_id,account_id,id),
 UNIQUE(brand_id,id),
 CHECK((cycle_from_at IS NULL)=(cycle_from_version=0)),
 CHECK(cycle_from_at IS NULL OR cycle_from_at<=created_at),
 CHECK(updated_at>=created_at AND (reviewed_at IS NULL OR reviewed_at>=created_at) AND (completed_at IS NULL OR completed_at>=created_at)),
 CHECK((state IN('paid','rejected','failed','cancelled'))=(completed_at IS NOT NULL)),
 CHECK((state='paid')=(paid_entry_id IS NOT NULL)),
 CHECK((state IN('rejected','failed','cancelled'))=(release_entry_id IS NOT NULL)),
 CHECK(state NOT IN('processing','paid','failed') OR reviewed_at IS NOT NULL),
 CHECK(state<>'reviewing' OR reviewed_at IS NULL)
);
CREATE UNIQUE INDEX one_active_withdrawal ON withdrawal_orders(brand_id,member_id) WHERE state IN('reviewing','processing');
CREATE INDEX withdrawal_member_history ON withdrawal_orders(brand_id,member_id,created_at DESC,id DESC);
CREATE INDEX withdrawal_brand_queue ON withdrawal_orders(brand_id,state,created_at,id);

CREATE TABLE withdrawal_order_transitions (
 id uuid PRIMARY KEY, brand_id uuid NOT NULL, order_id uuid NOT NULL,
 version bigint NOT NULL CHECK(version BETWEEN 1 AND 9007199254740991),
 from_state text NOT NULL, to_state text NOT NULL, reason text NOT NULL CHECK(octet_length(reason) BETWEEN 1 AND 500),
 actor_type text NOT NULL CHECK(actor_type IN('user','admin','system')), actor_id uuid,
 audit_log_id uuid NOT NULL REFERENCES audit_logs(id), created_at timestamptz NOT NULL,
 FOREIGN KEY(brand_id,order_id) REFERENCES withdrawal_orders(brand_id,id),
 UNIQUE(order_id,version),
 CHECK(actor_type='system' OR actor_id IS NOT NULL)
);
CREATE TABLE withdrawal_turnover_cycles (
 brand_id uuid NOT NULL, member_id uuid NOT NULL, account_id uuid NOT NULL,
 cutoff_at timestamptz NOT NULL, cutoff_version bigint NOT NULL CHECK(cutoff_version>0),
 last_paid_order_id uuid NOT NULL,
 PRIMARY KEY(brand_id,member_id),
 FOREIGN KEY(brand_id,account_id,member_id) REFERENCES point_accounts(brand_id,id,brand_member_id),
 FOREIGN KEY(brand_id,last_paid_order_id) REFERENCES withdrawal_orders(brand_id,id)
);
CREATE TABLE withdrawal_operation_receipts (
 id uuid PRIMARY KEY, brand_id uuid NOT NULL, order_id uuid NOT NULL,
 actor_type text NOT NULL CHECK(actor_type IN('user','admin')), actor_id uuid NOT NULL,
 client_key text NOT NULL CHECK(client_key ~ '^[a-zA-Z0-9_.:-]{8,128}$'),
 request_hash text NOT NULL CHECK(request_hash ~ '^[0-9a-f]{64}$'),
 response jsonb NOT NULL CHECK(jsonb_typeof(response)='object'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(brand_id,actor_type,actor_id,client_key),
 FOREIGN KEY(brand_id,order_id) REFERENCES withdrawal_orders(brand_id,id)
);

CREATE FUNCTION guard_withdrawal_order_projection() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'withdrawal orders cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR NEW.state<>'reviewing' OR NEW.updated_at<>NEW.created_at THEN RAISE EXCEPTION 'withdrawal must start in review'; END IF;
 ELSE
  IF ROW(NEW.id,NEW.brand_id,NEW.member_id,NEW.account_id,NEW.points,NEW.source_allocation,NEW.policy_snapshot,NEW.eligibility_evidence,NEW.cycle_from_at,NEW.cycle_from_version,NEW.reserve_version,NEW.reserve_entry_id,NEW.created_at)
   IS DISTINCT FROM ROW(OLD.id,OLD.brand_id,OLD.member_id,OLD.account_id,OLD.points,OLD.source_allocation,OLD.policy_snapshot,OLD.eligibility_evidence,OLD.cycle_from_at,OLD.cycle_from_version,OLD.reserve_version,OLD.reserve_entry_id,OLD.created_at) THEN RAISE EXCEPTION 'withdrawal request facts are immutable'; END IF;
  IF NEW.version<>OLD.version+1 OR NEW.updated_at<OLD.updated_at OR NOT(
   OLD.state='reviewing' AND NEW.state IN('processing','rejected','cancelled') OR
   OLD.state='processing' AND NEW.state IN('paid','failed','cancelled')) THEN RAISE EXCEPTION 'invalid withdrawal transition'; END IF;
  IF OLD.reviewed_at IS NOT NULL AND NEW.reviewed_at IS DISTINCT FROM OLD.reviewed_at THEN RAISE EXCEPTION 'review time is immutable'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER withdrawal_projection BEFORE INSERT OR UPDATE OR DELETE ON withdrawal_orders FOR EACH ROW EXECUTE FUNCTION guard_withdrawal_order_projection();

CREATE FUNCTION guard_withdrawal_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE o withdrawal_orders; a audit_logs; prev withdrawal_order_transitions;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'withdrawal evidence is immutable'; END IF;
 SELECT * INTO o FROM withdrawal_orders WHERE brand_id=NEW.brand_id AND id=NEW.order_id;
 IF o.id IS NULL THEN RAISE EXCEPTION 'withdrawal evidence has no scoped order'; END IF;
 IF TG_TABLE_NAME='withdrawal_order_transitions' THEN
  SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
  IF a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.resource_type<>'withdrawal_order' OR a.resource_id IS DISTINCT FROM NEW.order_id OR a.actor_type<>NEW.actor_type OR a.actor_id IS DISTINCT FROM NEW.actor_id OR a.action IS DISTINCT FROM 'withdrawal.'||NEW.to_state OR a.reason IS DISTINCT FROM NEW.reason OR a.before_json->>'state' IS DISTINCT FROM NEW.from_state OR a.after_json->>'state' IS DISTINCT FROM NEW.to_state OR a.after_json->>'version' IS DISTINCT FROM NEW.version::text THEN RAISE EXCEPTION 'withdrawal transition has no scoped audit'; END IF;
  IF NEW.version=1 THEN
   IF NEW.from_state<>'' OR NEW.to_state<>'reviewing' OR NEW.actor_type<>'user' OR NEW.created_at<>o.created_at OR NEW.actor_id IS DISTINCT FROM (SELECT global_user_id FROM brand_members WHERE brand_id=o.brand_id AND id=o.member_id) THEN RAISE EXCEPTION 'invalid withdrawal creation evidence'; END IF;
  ELSE
   SELECT * INTO prev FROM withdrawal_order_transitions WHERE order_id=NEW.order_id AND version=NEW.version-1;
   IF prev.id IS NULL OR NEW.from_state<>prev.to_state OR NEW.created_at<prev.created_at OR NOT(
    NEW.from_state='reviewing' AND NEW.to_state IN('processing','rejected','cancelled') OR
    NEW.from_state='processing' AND NEW.to_state IN('paid','failed','cancelled')) THEN RAISE EXCEPTION 'invalid withdrawal transition evidence'; END IF;
   IF NEW.actor_type='user' OR NEW.actor_type='system' AND (NEW.to_state<>'processing' OR o.policy_snapshot->'config'->>'review_mode' IS DISTINCT FROM 'automatic') THEN RAISE EXCEPTION 'unauthorized withdrawal evidence actor'; END IF;
  END IF;
 ELSE
  IF NEW.response->>'id' IS DISTINCT FROM NEW.order_id::text OR NEW.response->>'brand_id' IS DISTINCT FROM NEW.brand_id::text OR NOT EXISTS(SELECT 1 FROM withdrawal_order_transitions WHERE order_id=NEW.order_id AND version=(NEW.response->>'version')::bigint AND to_state=NEW.response->>'state') THEN RAISE EXCEPTION 'invalid withdrawal original receipt'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER withdrawal_transition_evidence BEFORE INSERT OR UPDATE OR DELETE ON withdrawal_order_transitions FOR EACH ROW EXECUTE FUNCTION guard_withdrawal_evidence();
CREATE TRIGGER withdrawal_receipt_evidence BEFORE INSERT OR UPDATE OR DELETE ON withdrawal_operation_receipts FOR EACH ROW EXECUTE FUNCTION guard_withdrawal_evidence();

CREATE FUNCTION guard_withdrawal_cycle() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE o withdrawal_orders;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'withdrawal cycle cannot be deleted'; END IF;
 SELECT * INTO o FROM withdrawal_orders WHERE brand_id=NEW.brand_id AND id=NEW.last_paid_order_id;
 IF o.id IS NULL OR o.state<>'paid' OR o.member_id<>NEW.member_id OR o.account_id<>NEW.account_id OR o.created_at<>NEW.cutoff_at OR o.reserve_version<>NEW.cutoff_version THEN RAISE EXCEPTION 'withdrawal cycle requires actual paid order submission boundary'; END IF;
 IF TG_OP='UPDATE' AND (ROW(NEW.brand_id,NEW.member_id,NEW.account_id) IS DISTINCT FROM ROW(OLD.brand_id,OLD.member_id,OLD.account_id) OR NEW.cutoff_version<=OLD.cutoff_version OR NEW.cutoff_at<OLD.cutoff_at) THEN RAISE EXCEPTION 'withdrawal cycle cannot move backwards'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER withdrawal_cycle BEFORE INSERT OR UPDATE OR DELETE ON withdrawal_turnover_cycles FOR EACH ROW EXECUTE FUNCTION guard_withdrawal_cycle();

CREATE FUNCTION validate_withdrawal_funds() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE o withdrawal_orders; e point_ledger_entries; f point_ledger_entries; latest withdrawal_order_transitions; allocation jsonb; total numeric:=0; expected jsonb:='{}'; item jsonb; src text; n bigint; seen text[]:='{}'; h bigint;
BEGIN
 SELECT * INTO o FROM withdrawal_orders WHERE id=NEW.id;
 SELECT * INTO e FROM point_ledger_entries WHERE id=o.reserve_entry_id;
 IF e.brand_id IS DISTINCT FROM o.brand_id OR e.account_id IS DISTINCT FROM o.account_id OR e.member_id IS DISTINCT FROM o.member_id OR e.entry_type<>'withdrawal_reserve' OR e.reference_type<>'withdrawal' OR e.reference_id IS DISTINCT FROM o.id OR e.version<>o.reserve_version OR e.reversal_of IS NOT NULL OR e.source_allocation IS DISTINCT FROM o.source_allocation THEN RAISE EXCEPTION 'withdrawal reserve lacks exact ledger evidence'; END IF;
 IF jsonb_array_length(o.source_allocation) NOT BETWEEN 1 AND 3 THEN RAISE EXCEPTION 'invalid withdrawal allocation'; END IF;
 FOR item IN SELECT value FROM jsonb_array_elements(o.source_allocation) LOOP
  src:=item->>'source';
  IF jsonb_typeof(item)<>'object' OR (SELECT count(*) FROM jsonb_object_keys(item))<>3 OR NOT(item ?& ARRAY['source','state','points']) OR src NOT IN('recharge','winning','gift') OR src=ANY(seen) OR item->>'state'<>'available' OR jsonb_typeof(item->'points')<>'string' OR item->>'points' !~ '^[1-9][0-9]*$' THEN RAISE EXCEPTION 'invalid withdrawal source allocation'; END IF;
  n:=(item->>'points')::bigint; total:=total+n; seen:=array_append(seen,src);
 END LOOP;
 IF total<>o.points THEN RAISE EXCEPTION 'withdrawal allocation does not sum to amount'; END IF;
 FOREACH src IN ARRAY ARRAY['recharge','winning','gift'] LOOP
  SELECT coalesce(sum((value->>'points')::bigint),0) INTO n FROM jsonb_array_elements(o.source_allocation) WHERE value->>'source'=src;
  expected:=expected||jsonb_build_object(src,jsonb_build_object('available',(-n)::text,'manual_frozen','0','system_frozen','0','withdrawal',n::text));
 END LOOP;
 IF e.delta_snapshot IS DISTINCT FROM expected THEN RAISE EXCEPTION 'withdrawal reserve changed wrong buckets'; END IF;
 IF o.release_entry_id IS NOT NULL THEN
  SELECT * INTO f FROM point_ledger_entries WHERE id=o.release_entry_id;
  IF f.reversal_of IS DISTINCT FROM e.id OR f.entry_type<>'withdrawal_release' OR f.reference_type<>'withdrawal' OR f.reference_id IS DISTINCT FROM o.id OR f.member_id<>o.member_id OR f.source_allocation IS DISTINCT FROM o.source_allocation THEN RAISE EXCEPTION 'withdrawal release is not original-source full reversal'; END IF;
  expected:='{}';
  FOREACH src IN ARRAY ARRAY['recharge','winning','gift'] LOOP
   n:=(e.delta_snapshot->src->>'withdrawal')::bigint;
   expected:=expected||jsonb_build_object(src,jsonb_build_object('available',n::text,'manual_frozen','0','system_frozen','0','withdrawal',(-n)::text));
  END LOOP;
  IF f.delta_snapshot IS DISTINCT FROM expected THEN RAISE EXCEPTION 'withdrawal release changed wrong buckets'; END IF;
 END IF;
 IF o.paid_entry_id IS NOT NULL THEN
  SELECT * INTO f FROM point_ledger_entries WHERE id=o.paid_entry_id;
  expected:='{}';
  FOREACH src IN ARRAY ARRAY['recharge','winning','gift'] LOOP
   n:=(e.delta_snapshot->src->>'withdrawal')::bigint;
   expected:=expected||jsonb_build_object(src,jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal',(-n)::text));
  END LOOP;
  IF f.entry_type<>'withdrawal_paid' OR f.reference_type<>'withdrawal' OR f.reference_id IS DISTINCT FROM o.id OR f.member_id<>o.member_id OR f.reversal_of IS NOT NULL OR f.delta_snapshot IS DISTINCT FROM expected THEN RAISE EXCEPTION 'withdrawal paid evidence changed wrong buckets'; END IF;
  IF NOT EXISTS(SELECT 1 FROM withdrawal_turnover_cycles WHERE brand_id=o.brand_id AND member_id=o.member_id AND cutoff_version>=o.reserve_version) THEN RAISE EXCEPTION 'paid withdrawal has no successful cycle boundary'; END IF;
 END IF;
 SELECT * INTO latest FROM withdrawal_order_transitions WHERE order_id=o.id ORDER BY version DESC LIMIT 1;
 SELECT count(*) INTO h FROM withdrawal_order_transitions WHERE order_id=o.id;
 IF latest.id IS NULL OR latest.version<>o.version OR latest.to_state<>o.state OR latest.created_at<>o.updated_at OR h<>o.version THEN RAISE EXCEPTION 'withdrawal projection has no complete immutable history'; END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER withdrawal_fund_evidence AFTER INSERT OR UPDATE ON withdrawal_orders DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_withdrawal_funds();
INSERT INTO permissions(key) VALUES('withdrawal.view.brand'),('withdrawal.view.platform'),('withdrawal.approve.brand'),('withdrawal.reject.brand'),('withdrawal.cancel.brand'),('withdrawal.fail.brand'),('withdrawal.mark_paid.brand') ON CONFLICT DO NOTHING;

-- The new trigger functions must retain scoped lookups under pg_restore's empty
-- search_path, and must not resolve a caller's temporary-table impersonation.
DO $$
DECLARE app_schema text:=current_schema(); f text;
BEGIN
 IF app_schema IS NULL THEN RAISE EXCEPTION 'application schema required'; END IF;
 FOREACH f IN ARRAY ARRAY['guard_withdrawal_order_projection','guard_withdrawal_evidence','guard_withdrawal_cycle','validate_withdrawal_funds'] LOOP
  EXECUTE format('ALTER FUNCTION %I.%I() SET search_path = pg_catalog, %I, pg_temp',app_schema,f,app_schema);
 END LOOP;
END $$;
