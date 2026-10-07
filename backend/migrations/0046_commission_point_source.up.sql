-- Append a fourth source without rewriting immutable economic history,
-- request hashes, business snapshots, audits, or encrypted receipts.
ALTER TABLE point_buckets DROP CONSTRAINT point_buckets_source_check;
ALTER TABLE point_buckets ADD CONSTRAINT point_buckets_source_check
 CHECK(source IN ('recharge','winning','gift','commission'));
INSERT INTO point_buckets(brand_id,account_id,source,state,points)
 SELECT brand_id,id,'commission',s,0 FROM point_accounts
 CROSS JOIN unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal']) s
 ON CONFLICT DO NOTHING;

CREATE FUNCTION initialize_commission_point_buckets() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO point_buckets(brand_id,account_id,source,state,points)
 SELECT NEW.brand_id,NEW.id,'commission',s,0
 FROM unnest(ARRAY['available','manual_frozen','system_frozen','withdrawal']) s;
 RETURN NULL;
END $$;
CREATE TRIGGER commission_point_buckets AFTER INSERT ON point_accounts
 FOR EACH ROW EXECUTE FUNCTION initialize_commission_point_buckets();

CREATE FUNCTION point_zero_snapshot() RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$
 SELECT jsonb_object_agg(s,jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal','0'))
 FROM unnest(ARRAY['recharge','winning','gift','commission']) s
$$;

-- Legacy is an exact, complete twelve-bucket object, never an arbitrary sparse
-- object. New writes require all sixteen buckets. Delta permits signed int64;
-- balance snapshots require nonnegative buckets and an int64 aggregate.
CREATE FUNCTION valid_point_snapshot(v jsonb,signed_amounts boolean,allow_legacy boolean)
 RETURNS boolean LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE src text; st text; n numeric; total numeric:=0; sources integer;
BEGIN
 IF v IS NULL OR jsonb_typeof(v) IS DISTINCT FROM 'object' THEN RETURN false; END IF;
 SELECT count(*) INTO sources FROM jsonb_object_keys(v);
 IF NOT(v ?& ARRAY['recharge','winning','gift']) OR
  NOT(sources=4 AND v ? 'commission' OR allow_legacy AND sources=3 AND NOT(v ? 'commission')) THEN RETURN false; END IF;
 FOR src IN SELECT jsonb_object_keys(v) LOOP
  IF src NOT IN('recharge','winning','gift','commission') OR jsonb_typeof(v->src) IS DISTINCT FROM 'object' OR
   (SELECT count(*) FROM jsonb_object_keys(v->src))<>4 OR
   NOT(v->src ?& ARRAY['available','manual_frozen','system_frozen','withdrawal']) THEN RETURN false; END IF;
  FOREACH st IN ARRAY ARRAY['available','manual_frozen','system_frozen','withdrawal'] LOOP
   IF jsonb_typeof(v->src->st) IS DISTINCT FROM 'string' OR
    v->src->>st !~ (CASE WHEN signed_amounts THEN '^(0|-?[1-9][0-9]*)$' ELSE '^(0|[1-9][0-9]*)$' END) THEN RETURN false; END IF;
   n:=(v->src->>st)::numeric;
   IF n < -9223372036854775808 OR n > 9223372036854775807 THEN RETURN false; END IF;
   total:=total+n;
  END LOOP;
 END LOOP;
 RETURN signed_amounts OR total<=9223372036854775807;
EXCEPTION WHEN OTHERS THEN RETURN false;
END $$;

CREATE FUNCTION normalize_point_snapshot(v jsonb) RETURNS jsonb LANGUAGE plpgsql IMMUTABLE AS $$
BEGIN
 IF valid_point_snapshot(v,true,true) IS NOT TRUE THEN RETURN NULL; END IF;
 IF v ? 'commission' THEN RETURN v; END IF;
 RETURN v||jsonb_build_object('commission',point_zero_snapshot()->'commission');
END $$;

CREATE FUNCTION guard_point_ledger_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE a point_accounts; prev jsonb; actual jsonb; original point_ledger_entries;
 src text; st text; item jsonb; seen text[]:='{}'; bucket_value numeric; amount numeric;
 negatives integer; positives integer; negative_state text; positive_state text;
 negative_amount numeric; positive_amount numeric; expected jsonb; allocated boolean;
BEGIN
 IF valid_point_snapshot(NEW.before_snapshot,false,false) IS NOT TRUE OR
  valid_point_snapshot(NEW.delta_snapshot,true,false) IS NOT TRUE OR
  valid_point_snapshot(NEW.after_snapshot,false,false) IS NOT TRUE THEN
  RAISE EXCEPTION 'new ledger requires complete sixteen-bucket snapshots' USING ERRCODE='23514',CONSTRAINT='ledger_four_source_snapshot';
 END IF;
 SELECT * INTO a FROM point_accounts WHERE brand_id=NEW.brand_id AND id=NEW.account_id AND brand_member_id=NEW.member_id FOR UPDATE;
 IF NOT FOUND OR a.version<>NEW.version-1 THEN RAISE EXCEPTION 'ledger must advance locked account by one version'; END IF;
 SELECT jsonb_object_agg(source,states) INTO actual FROM (
  SELECT source,jsonb_object_agg(state,points::text) states FROM point_buckets
  WHERE brand_id=NEW.brand_id AND account_id=NEW.account_id GROUP BY source
 ) buckets;
 IF actual IS DISTINCT FROM NEW.before_snapshot THEN RAISE EXCEPTION 'ledger before must match complete current wallet'; END IF;
 IF a.version=0 THEN prev:=point_zero_snapshot();
 ELSE SELECT normalize_point_snapshot(after_snapshot) INTO prev FROM point_ledger_entries
  WHERE brand_id=NEW.brand_id AND account_id=NEW.account_id AND version=a.version; END IF;
 IF prev IS DISTINCT FROM NEW.before_snapshot THEN RAISE EXCEPTION 'ledger before must match immutable predecessor'; END IF;
 IF NEW.before_snapshot=NEW.after_snapshot THEN RAISE EXCEPTION 'ledger cannot post empty change'; END IF;
 FOREACH src IN ARRAY ARRAY['recharge','winning','gift','commission'] LOOP
  FOREACH st IN ARRAY ARRAY['available','manual_frozen','system_frozen','withdrawal'] LOOP
   IF (NEW.before_snapshot->src->>st)::numeric+(NEW.delta_snapshot->src->>st)::numeric
     IS DISTINCT FROM (NEW.after_snapshot->src->>st)::numeric THEN RAISE EXCEPTION 'ledger snapshot arithmetic mismatch'; END IF;
  END LOOP;
 END LOOP;
 IF jsonb_typeof(NEW.source_allocation) IS DISTINCT FROM 'array' OR jsonb_array_length(NEW.source_allocation) NOT BETWEEN 1 AND 4 THEN RAISE EXCEPTION 'ledger requires source allocation'; END IF;
 FOR item IN SELECT value FROM jsonb_array_elements(NEW.source_allocation) LOOP
  src:=item->>'source'; st:=item->>'state';
  IF jsonb_typeof(item) IS DISTINCT FROM 'object' OR (SELECT count(*) FROM jsonb_object_keys(item))<>3 OR
   NOT(item ?& ARRAY['source','state','points']) OR src IS NULL OR src NOT IN('recharge','winning','gift','commission') OR src=ANY(seen) OR
   st IS NULL OR st NOT IN('available','manual_frozen','system_frozen','withdrawal') OR
   jsonb_typeof(item->'points') IS DISTINCT FROM 'string' OR item->>'points' !~ '^[1-9][0-9]*$' OR
   (item->>'points')::numeric>9223372036854775807 THEN RAISE EXCEPTION 'invalid ledger source allocation'; END IF;
  seen:=array_append(seen,src);
 END LOOP;
 IF NEW.reversal_of IS NOT NULL THEN
  SELECT * INTO original FROM point_ledger_entries WHERE brand_id=NEW.brand_id AND account_id=NEW.account_id AND id=NEW.reversal_of AND member_id=NEW.member_id;
  expected:=normalize_point_snapshot(original.delta_snapshot);
  IF original.id IS NULL OR original.reversal_of IS NOT NULL OR expected IS NULL OR NEW.source_allocation IS DISTINCT FROM original.source_allocation THEN RAISE EXCEPTION 'invalid original-source reversal'; END IF;
  FOREACH src IN ARRAY ARRAY['recharge','winning','gift','commission'] LOOP
   FOREACH st IN ARRAY ARRAY['available','manual_frozen','system_frozen','withdrawal'] LOOP
    expected:=jsonb_set(expected,ARRAY[src,st],to_jsonb((-(expected->src->>st)::numeric)::text));
   END LOOP;
  END LOOP;
  IF NEW.delta_snapshot IS DISTINCT FROM expected THEN RAISE EXCEPTION 'reversal must negate exact original source changes'; END IF;
 ELSE
  FOREACH src IN ARRAY ARRAY['recharge','winning','gift','commission'] LOOP
   negatives:=0; positives:=0; negative_state:=NULL; positive_state:=NULL;
   negative_amount:=0; positive_amount:=0;
   FOREACH st IN ARRAY ARRAY['available','manual_frozen','system_frozen','withdrawal'] LOOP
    bucket_value:=(NEW.delta_snapshot->src->>st)::numeric;
    IF bucket_value<0 THEN negatives:=negatives+1; negative_state:=st; negative_amount:=-bucket_value; END IF;
    IF bucket_value>0 THEN positives:=positives+1; positive_state:=st; positive_amount:=bucket_value; END IF;
   END LOOP;
   allocated:=src=ANY(seen);
   IF (negatives>0 OR positives>0) IS DISTINCT FROM allocated OR negatives>1 OR positives>1 OR
    negatives=1 AND positives=1 AND negative_amount<>positive_amount THEN RAISE EXCEPTION 'source allocation must explain exact bucket changes'; END IF;
   IF allocated THEN
    SELECT (x->>'points')::numeric,x->>'state' INTO amount,st FROM jsonb_array_elements(NEW.source_allocation) x WHERE x->>'source'=src;
    IF amount IS DISTINCT FROM (CASE WHEN negatives=1 THEN negative_amount ELSE positive_amount END) OR
     st IS DISTINCT FROM (CASE WHEN negatives=1 THEN negative_state ELSE positive_state END) THEN RAISE EXCEPTION 'source allocation amount/state mismatch'; END IF;
   END IF;
  END LOOP;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER ledger_four_source_snapshot BEFORE INSERT ON point_ledger_entries
 FOR EACH ROW EXECUTE FUNCTION guard_point_ledger_snapshot();

CREATE FUNCTION validate_point_ledger_projection() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE a point_accounts; tip point_ledger_entries; actual jsonb;
BEGIN
 SELECT * INTO a FROM point_accounts WHERE brand_id=NEW.brand_id AND id=NEW.account_id;
 SELECT * INTO tip FROM point_ledger_entries WHERE brand_id=NEW.brand_id AND account_id=NEW.account_id ORDER BY version DESC LIMIT 1;
 SELECT jsonb_object_agg(source,states) INTO actual FROM (
  SELECT source,jsonb_object_agg(state,points::text) states FROM point_buckets
  WHERE brand_id=NEW.brand_id AND account_id=NEW.account_id GROUP BY source
 ) buckets;
 IF tip.id IS NULL OR a.version IS DISTINCT FROM tip.version OR actual IS DISTINCT FROM tip.after_snapshot OR
  NOT EXISTS(SELECT 1 FROM audit_logs x WHERE x.brand_id=NEW.brand_id AND x.action='points.'||NEW.entry_type AND
   x.actor_type=NEW.actor_type AND x.actor_id IS NOT DISTINCT FROM NEW.actor_id AND x.resource_type='point_account' AND x.resource_id=NEW.account_id AND
   x.before_json=NEW.before_snapshot AND x.after_json->>'ledger_entry_id'=NEW.id::text AND x.after_json->'balance'=NEW.after_snapshot) THEN
  RAISE EXCEPTION 'ledger requires atomically applied wallet tip and matching audit';
 END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER ledger_four_source_projection AFTER INSERT ON point_ledger_entries
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_point_ledger_projection();

-- Replace only the source-dependent proof functions. Historical evidence is
-- normalized for comparison, never rewritten. Financial policy defaults stay
-- unchanged; operators must explicitly allow commission withdrawals.

CREATE OR REPLACE FUNCTION validate_settlement_payout_witness() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE l point_ledger_entries; zero_bucket jsonb; expected jsonb; src text; bucket text;
BEGIN
 IF OLD.status<>'placed' OR NEW.status NOT IN ('won','lost') OR NEW.prize_points=0 THEN RETURN NEW; END IF;
 SELECT * INTO l FROM point_ledger_entries WHERE id=NEW.payout_entry_id;
 zero_bucket:=jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal','0');
 expected:=jsonb_build_object('recharge',zero_bucket,'gift',zero_bucket,'commission',zero_bucket,'winning',zero_bucket||jsonb_build_object('available',NEW.prize_points::text));
 IF l.id IS NULL OR l.account_id IS DISTINCT FROM NEW.account_id OR l.delta_snapshot IS DISTINCT FROM expected OR l.operation_key IS DISTINCT FROM 'settlement-payout:'||NEW.settlement_calculation_id::text OR
 jsonb_typeof(l.before_snapshot) IS DISTINCT FROM 'object' OR jsonb_typeof(l.after_snapshot) IS DISTINCT FROM 'object' THEN RAISE EXCEPTION 'incomplete settlement payout witness'; END IF;
 IF (SELECT count(*) FROM jsonb_object_keys(l.before_snapshot))<>4 OR (SELECT count(*) FROM jsonb_object_keys(l.after_snapshot))<>4 THEN RAISE EXCEPTION 'invalid payout snapshot sources'; END IF;
 FOREACH src IN ARRAY ARRAY['recharge','winning','gift','commission'] LOOP
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
 NOT EXISTS(SELECT 1 FROM point_ledger_entries prev WHERE prev.brand_id=NEW.brand_id AND prev.account_id=NEW.account_id AND prev.version=l.version-1 AND normalize_point_snapshot(prev.after_snapshot)=l.before_snapshot) OR
 NOT EXISTS(SELECT 1 FROM audit_logs a WHERE a.brand_id=NEW.brand_id AND a.actor_type='system' AND a.action='points.prize' AND a.resource_type='point_account' AND a.resource_id=NEW.account_id AND a.before_json=l.before_snapshot AND a.after_json->>'ledger_entry_id'=l.id::text AND a.after_json->'balance'=l.after_snapshot) THEN RAISE EXCEPTION 'payout requires applied ledger tip and audit witness'; END IF;
 RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION correction_reversal_is_applied(t draw_correction_targets) RETURNS boolean LANGUAGE plpgsql AS $$
DECLARE l point_ledger_entries; original point_ledger_entries; zero_bucket jsonb; expected jsonb;
BEGIN
 IF t.old_prize_points=0 THEN RETURN t.old_payout_entry_id IS NULL AND t.reversal_entry_id IS NULL; END IF;
 SELECT * INTO original FROM point_ledger_entries WHERE id=t.old_payout_entry_id;
 SELECT * INTO l FROM point_ledger_entries WHERE id=t.reversal_entry_id;
 zero_bucket:=jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal','0');
 expected:=jsonb_build_object('recharge',zero_bucket,'gift',zero_bucket,'commission',zero_bucket,'winning',zero_bucket||jsonb_build_object('available',(-t.old_prize_points)::text));
 RETURN COALESCE(l.id IS NOT NULL AND original.id IS NOT NULL AND l.brand_id=t.brand_id AND l.account_id=original.account_id AND l.member_id=original.member_id AND l.entry_type='prize_reversal' AND l.reference_type='draw_correction' AND l.reference_id=t.correction_id AND l.reversal_of=t.old_payout_entry_id AND l.operation_key='draw-correction:'||t.correction_id::text||':'||t.order_id::text AND l.actor_type='system' AND l.source_allocation=original.source_allocation AND normalize_point_snapshot(l.delta_snapshot)=expected AND
  EXISTS(SELECT 1 FROM audit_logs a WHERE a.brand_id=t.brand_id AND a.action='points.prize_reversal' AND a.actor_type='system' AND a.resource_id=l.account_id AND a.after_json->>'ledger_entry_id'=l.id::text AND a.before_json=l.before_snapshot AND a.after_json->'balance'=l.after_snapshot),FALSE);
END $$;

CREATE OR REPLACE FUNCTION guard_settlement_bet_update() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE l point_ledger_entries; c settlement_calculations; j settlement_jobs;
BEGIN
 IF (to_jsonb(NEW)-ARRAY['status','version','refund_entry_id','cancelled_at','cancel_reason','settlement_calculation_id','payout_entry_id','prize_points','settled_at']) IS DISTINCT FROM
 (to_jsonb(OLD)-ARRAY['status','version','refund_entry_id','cancelled_at','cancel_reason','settlement_calculation_id','payout_entry_id','prize_points','settled_at']) OR NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'immutable bet identity or invalid version'; END IF;
 IF OLD.status IN ('won','lost') AND NEW.status='placed' THEN
  IF NEW.refund_entry_id IS DISTINCT FROM OLD.refund_entry_id OR NEW.cancelled_at IS DISTINCT FROM OLD.cancelled_at OR NEW.cancel_reason IS DISTINCT FROM OLD.cancel_reason OR NEW.settlement_calculation_id IS NOT NULL OR NEW.payout_entry_id IS NOT NULL OR NEW.prize_points<>0 OR NEW.settled_at IS NOT NULL OR NOT EXISTS(
   SELECT 1 FROM draw_correction_targets t JOIN draw_corrections dc ON dc.id=t.correction_id JOIN periods p ON p.id=dc.period_id
   WHERE t.brand_id=OLD.brand_id AND t.order_id=OLD.id AND t.old_order_version=OLD.version AND t.old_order_status=OLD.status AND t.old_calculation_id=OLD.settlement_calculation_id AND t.old_payout_entry_id IS NOT DISTINCT FROM OLD.payout_entry_id AND t.old_prize_points=OLD.prize_points AND t.state='reversed' AND t.reset_order_version=NEW.version AND dc.state='reversing' AND p.current_correction_id=dc.id AND p.draw_result_id=dc.previous_draw_result_id AND correction_reversal_is_applied(t) IS TRUE AND
   (t.old_prize_points=0 OR EXISTS(SELECT 1 FROM point_ledger_entries le JOIN point_accounts pa ON pa.id=le.account_id WHERE le.id=t.reversal_entry_id AND pa.version=le.version AND (SELECT count(*) FROM point_buckets pb WHERE pb.brand_id=le.brand_id AND pb.account_id=le.account_id AND pb.points::numeric=(normalize_point_snapshot(le.after_snapshot)->pb.source->>pb.state)::numeric)=16)))
   THEN RAISE EXCEPTION 'order reset requires applied full reversal'; END IF;
  RETURN NEW;
 END IF;
 IF OLD.status='placed' AND NEW.status IN ('won','lost') THEN
  SELECT * INTO c FROM settlement_calculations WHERE brand_id=NEW.brand_id AND order_id=NEW.id AND id=NEW.settlement_calculation_id;
  SELECT * INTO j FROM settlement_jobs WHERE id=c.job_id;
  IF c.id IS NULL OR c.order_version<>OLD.version OR c.definition_hash<>OLD.definition_hash OR NEW.prize_points<>c.prize_points OR NEW.status<>(CASE WHEN c.won THEN 'won' ELSE 'lost' END) OR j.state<>'paying' OR NOT EXISTS(SELECT 1 FROM periods WHERE id=j.period_id AND status='settling' AND draw_result_id=j.draw_result_id) OR NEW.refund_entry_id IS NOT NULL OR NEW.cancelled_at IS NOT NULL OR NEW.cancel_reason<>'' THEN RAISE EXCEPTION 'invalid payout calculation evidence'; END IF;
  IF c.prize_points>0 THEN
   SELECT * INTO l FROM point_ledger_entries WHERE id=NEW.payout_entry_id;
   IF l.id IS NULL OR l.brand_id<>NEW.brand_id OR l.member_id<>NEW.brand_member_id OR l.entry_type<>'prize' OR l.reference_type<>'settlement_calculation' OR l.reference_id IS DISTINCT FROM c.id OR l.actor_type<>'system' OR l.reversal_of IS NOT NULL OR l.source_allocation<>jsonb_build_array(jsonb_build_object('source','winning','state','available','points',c.prize_points::text)) OR
   (l.delta_snapshot->'winning'->>'available')::numeric<>c.prize_points OR EXISTS(SELECT 1 FROM jsonb_each(l.delta_snapshot) s WHERE (s.value->>'manual_frozen')::numeric<>0 OR (s.value->>'system_frozen')::numeric<>0 OR (s.value->>'withdrawal')::numeric<>0 OR (s.key<>'winning' AND (s.value->>'available')::numeric<>0)) THEN RAISE EXCEPTION 'invalid winning ledger evidence'; END IF;
  END IF;
  RETURN NEW;
 END IF;
 IF NEW.settlement_calculation_id IS DISTINCT FROM OLD.settlement_calculation_id OR NEW.payout_entry_id IS DISTINCT FROM OLD.payout_entry_id OR NEW.prize_points<>OLD.prize_points OR NEW.settled_at IS DISTINCT FROM OLD.settled_at THEN RAISE EXCEPTION 'settlement fields are immutable outside payout'; END IF;
 IF OLD.status='placed' AND NEW.status='abnormal' THEN
  IF NEW.refund_entry_id IS DISTINCT FROM OLD.refund_entry_id OR NEW.cancelled_at IS DISTINCT FROM OLD.cancelled_at OR NEW.cancel_reason IS DISTINCT FROM OLD.cancel_reason OR NOT EXISTS(SELECT 1 FROM bet_order_exceptions WHERE brand_id=NEW.brand_id AND order_id=NEW.id AND order_version=NEW.version) THEN RAISE EXCEPTION 'invalid exception classification'; END IF;
  RETURN NEW;
 END IF;
 IF NOT (OLD.status IN ('placed','abnormal') AND NEW.status IN ('bet_cancelled','judged_cancelled')) THEN RAISE EXCEPTION 'invalid bet transition'; END IF;
 IF NEW.status='judged_cancelled' AND NOT EXISTS(SELECT 1 FROM bet_order_judgments WHERE brand_id=NEW.brand_id AND order_id=NEW.id AND order_version=NEW.version AND reason=NEW.cancel_reason) AND NOT EXISTS(SELECT 1 FROM period_cancellation_targets t JOIN period_cancellations pc ON pc.id=t.cancellation_id JOIN periods phase ON phase.id=pc.period_id WHERE t.brand_id=NEW.brand_id AND t.order_id=NEW.id AND t.state='pending' AND pc.mode='judged_cancelled' AND pc.state='processing' AND phase.status=pc.mode AND phase.version=pc.period_version) THEN RAISE EXCEPTION 'judged refund needs individual or period judgment witness'; END IF;
 SELECT * INTO l FROM point_ledger_entries WHERE id=NEW.refund_entry_id;
 IF l.entry_type<>'refund' OR l.reference_type<>'bet_order' OR l.reference_id IS DISTINCT FROM OLD.id OR l.reversal_of IS DISTINCT FROM OLD.debit_entry_id OR l.source_allocation<>OLD.deduction_allocation OR NEW.cancel_reason='' THEN RAISE EXCEPTION 'invalid bet refund evidence'; END IF;
 RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION guard_point_reconciliation_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE t point_reconciliation_targets; j point_reconciliation_jobs; a audit_logs; p point_accounts; actual jsonb; expected jsonb; n bigint;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'wallet reconciliation evidence is immutable'; END IF;
 SELECT * INTO j FROM point_reconciliation_jobs WHERE id=NEW.job_id AND brand_id=NEW.brand_id;
 SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
 IF j.id IS NULL OR a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.resource_type IS DISTINCT FROM 'wallet_reconciliation_job' AND TG_TABLE_NAME='point_reconciliation_retries' THEN
  RAISE EXCEPTION 'reconciliation evidence lacks scoped job and audit';
 END IF;
 IF TG_TABLE_NAME='point_reconciliation_retries' THEN
  IF j.state<>'failed' OR j.last_failure_id IS DISTINCT FROM NEW.previous_failure_id OR NEW.version<>j.version+1 OR a.actor_type IS DISTINCT FROM 'admin' OR a.actor_id IS DISTINCT FROM NEW.created_by OR a.action IS DISTINCT FROM 'wallet.reconciliation.retry' OR a.resource_id IS DISTINCT FROM NEW.job_id OR a.reason IS DISTINCT FROM NEW.reason OR a.after_json->>'version' IS DISTINCT FROM NEW.version::text THEN
   RAISE EXCEPTION 'reconciliation retry lacks matching failed version and audit';
  END IF;
  RETURN NEW;
 END IF;
 SELECT * INTO t FROM point_reconciliation_targets WHERE id=NEW.target_id AND brand_id=NEW.brand_id AND job_id=NEW.job_id;
 IF t.id IS NULL OR t.state<>'pending' OR j.state NOT IN('pending','running') OR a.actor_type IS DISTINCT FROM 'system' OR a.resource_type IS DISTINCT FROM 'wallet_reconciliation_target' OR a.resource_id IS DISTINCT FROM NEW.target_id OR a.after_json->>'job_id' IS DISTINCT FROM NEW.job_id::text THEN
  RAISE EXCEPTION 'reconciliation observation lacks pending target and audit';
 END IF;
 IF TG_TABLE_NAME='point_reconciliation_failures' THEN
  IF NEW.attempt_count<>t.attempt_count+1 OR a.action IS DISTINCT FROM 'wallet.reconciliation.failed' OR a.after_json->>'error_code' IS DISTINCT FROM NEW.error_code THEN RAISE EXCEPTION 'failure audit mismatch'; END IF;
  RETURN NEW;
 END IF;
 SELECT * INTO p FROM point_accounts WHERE id=t.account_id AND brand_id=t.brand_id;
 SELECT jsonb_object_agg(source,states) INTO actual FROM (
  SELECT source,jsonb_object_agg(state,points::text) states FROM point_buckets WHERE brand_id=t.brand_id AND account_id=t.account_id GROUP BY source
 ) b;
 SELECT count(*) INTO n FROM point_ledger_entries WHERE brand_id=t.brand_id AND account_id=t.account_id;
 SELECT after_snapshot INTO expected FROM point_ledger_entries WHERE brand_id=t.brand_id AND account_id=t.account_id ORDER BY version DESC LIMIT 1;
 IF expected IS NULL THEN expected:=point_zero_snapshot(); ELSE expected:=normalize_point_snapshot(expected); END IF;
 IF a.action IS DISTINCT FROM 'wallet.reconciliation.checked' OR a.after_json->>'outcome' IS DISTINCT FROM NEW.outcome OR a.after_json->'preview' IS DISTINCT FROM NEW.preview OR (a.after_json->>'checked_at')::timestamptz IS DISTINCT FROM NEW.checked_at OR
  NEW.preview->>'account_id' IS DISTINCT FROM t.account_id::text OR NEW.preview->>'member_id' IS DISTINCT FROM t.member_id::text OR NEW.preview->>'version' IS DISTINCT FROM p.version::text OR NEW.preview->>'ledger_version' IS DISTINCT FROM n::text OR NEW.preview->'actual' IS DISTINCT FROM COALESCE(actual,'{}'::jsonb) OR jsonb_typeof(NEW.preview->'issues') IS DISTINCT FROM 'array' OR jsonb_typeof(NEW.preview->'consistent') IS DISTINCT FROM 'boolean' OR jsonb_typeof(NEW.preview->'repairable') IS DISTINCT FROM 'boolean' OR COALESCE(NEW.preview->>'token','') !~ '^[0-9a-f]{64}$' OR
  NEW.outcome IS DISTINCT FROM (CASE WHEN NEW.preview->>'consistent'='true' THEN 'consistent' WHEN NEW.preview->>'repairable'='true' THEN 'repairable' ELSE 'corrupt' END) THEN
  RAISE EXCEPTION 'reconciliation result audit or observed wallet mismatch';
 END IF;
 IF NEW.outcome IN('consistent','repairable') AND NEW.preview->'expected' IS DISTINCT FROM expected THEN RAISE EXCEPTION 'intact ledger expected balance mismatch'; END IF;
 IF NEW.outcome='consistent' AND (NEW.preview->>'repairable'<>'false' OR jsonb_array_length(NEW.preview->'issues')<>0 OR NEW.preview->'actual' IS DISTINCT FROM NEW.preview->'expected' OR p.version<>n) THEN RAISE EXCEPTION 'false consistent reconciliation'; END IF;
 RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION validate_withdrawal_funds() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE o withdrawal_orders; e point_ledger_entries; f point_ledger_entries; latest withdrawal_order_transitions; allocation jsonb; total numeric:=0; expected jsonb:='{}'; item jsonb; src text; n bigint; seen text[]:='{}'; h bigint;
BEGIN
 SELECT * INTO o FROM withdrawal_orders WHERE id=NEW.id;
 SELECT * INTO e FROM point_ledger_entries WHERE id=o.reserve_entry_id;
 IF e.brand_id IS DISTINCT FROM o.brand_id OR e.account_id IS DISTINCT FROM o.account_id OR e.member_id IS DISTINCT FROM o.member_id OR e.entry_type<>'withdrawal_reserve' OR e.reference_type<>'withdrawal' OR e.reference_id IS DISTINCT FROM o.id OR e.version<>o.reserve_version OR e.reversal_of IS NOT NULL OR e.source_allocation IS DISTINCT FROM o.source_allocation THEN RAISE EXCEPTION 'withdrawal reserve lacks exact ledger evidence'; END IF;
 IF jsonb_array_length(o.source_allocation) NOT BETWEEN 1 AND 4 THEN RAISE EXCEPTION 'invalid withdrawal allocation'; END IF;
 FOR item IN SELECT value FROM jsonb_array_elements(o.source_allocation) LOOP
  src:=item->>'source';
  IF jsonb_typeof(item)<>'object' OR (SELECT count(*) FROM jsonb_object_keys(item))<>3 OR NOT(item ?& ARRAY['source','state','points']) OR src NOT IN('recharge','winning','gift','commission') OR src=ANY(seen) OR item->>'state'<>'available' OR jsonb_typeof(item->'points')<>'string' OR item->>'points' !~ '^[1-9][0-9]*$' THEN RAISE EXCEPTION 'invalid withdrawal source allocation'; END IF;
  n:=(item->>'points')::bigint; total:=total+n; seen:=array_append(seen,src);
 END LOOP;
 IF total<>o.points THEN RAISE EXCEPTION 'withdrawal allocation does not sum to amount'; END IF;
 FOREACH src IN ARRAY ARRAY['recharge','winning','gift','commission'] LOOP
  SELECT coalesce(sum((value->>'points')::bigint),0) INTO n FROM jsonb_array_elements(o.source_allocation) WHERE value->>'source'=src;
  expected:=expected||jsonb_build_object(src,jsonb_build_object('available',(-n)::text,'manual_frozen','0','system_frozen','0','withdrawal',n::text));
 END LOOP;
 IF normalize_point_snapshot(e.delta_snapshot) IS DISTINCT FROM expected THEN RAISE EXCEPTION 'withdrawal reserve changed wrong buckets'; END IF;
 IF o.release_entry_id IS NOT NULL THEN
  SELECT * INTO f FROM point_ledger_entries WHERE id=o.release_entry_id;
  IF f.reversal_of IS DISTINCT FROM e.id OR f.entry_type<>'withdrawal_release' OR f.reference_type<>'withdrawal' OR f.reference_id IS DISTINCT FROM o.id OR f.member_id<>o.member_id OR f.source_allocation IS DISTINCT FROM o.source_allocation THEN RAISE EXCEPTION 'withdrawal release is not original-source full reversal'; END IF;
  expected:='{}';
  FOREACH src IN ARRAY ARRAY['recharge','winning','gift','commission'] LOOP
   n:=(normalize_point_snapshot(e.delta_snapshot)->src->>'withdrawal')::bigint;
   expected:=expected||jsonb_build_object(src,jsonb_build_object('available',n::text,'manual_frozen','0','system_frozen','0','withdrawal',(-n)::text));
  END LOOP;
  IF normalize_point_snapshot(f.delta_snapshot) IS DISTINCT FROM expected THEN RAISE EXCEPTION 'withdrawal release changed wrong buckets'; END IF;
 END IF;
 IF o.paid_entry_id IS NOT NULL THEN
  SELECT * INTO f FROM point_ledger_entries WHERE id=o.paid_entry_id;
  expected:='{}';
  FOREACH src IN ARRAY ARRAY['recharge','winning','gift','commission'] LOOP
   n:=(normalize_point_snapshot(e.delta_snapshot)->src->>'withdrawal')::bigint;
   expected:=expected||jsonb_build_object(src,jsonb_build_object('available','0','manual_frozen','0','system_frozen','0','withdrawal',(-n)::text));
  END LOOP;
  IF f.entry_type<>'withdrawal_paid' OR f.reference_type<>'withdrawal' OR f.reference_id IS DISTINCT FROM o.id OR f.member_id<>o.member_id OR f.reversal_of IS NOT NULL OR normalize_point_snapshot(f.delta_snapshot) IS DISTINCT FROM expected THEN RAISE EXCEPTION 'withdrawal paid evidence changed wrong buckets'; END IF;
  IF NOT EXISTS(SELECT 1 FROM withdrawal_turnover_cycles WHERE brand_id=o.brand_id AND member_id=o.member_id AND cutoff_version>=o.reserve_version) THEN RAISE EXCEPTION 'paid withdrawal has no successful cycle boundary'; END IF;
 END IF;
 SELECT * INTO latest FROM withdrawal_order_transitions WHERE order_id=o.id ORDER BY version DESC LIMIT 1;
 SELECT count(*) INTO h FROM withdrawal_order_transitions WHERE order_id=o.id;
 IF latest.id IS NULL OR latest.version<>o.version OR latest.to_state<>o.state OR latest.created_at<>o.updated_at OR h<>o.version THEN RAISE EXCEPTION 'withdrawal projection has no complete immutable history'; END IF;
 RETURN NULL;
END $$;

CREATE OR REPLACE FUNCTION valid_withdrawal_policy(v jsonb,game_scope boolean) RETURNS boolean LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE key_count integer; sources integer;
BEGIN
 IF jsonb_typeof(v)<>'object' THEN RETURN false; END IF;
 SELECT count(*) INTO key_count FROM jsonb_object_keys(v);
 IF game_scope THEN RETURN key_count=1 AND v ? 'turnover_multiple' AND (v->'turnover_multiple'='null'::jsonb OR valid_withdrawal_multiple(v->'turnover_multiple')); END IF;
 IF key_count<>6 OR NOT v ?& ARRAY['enabled','min_points','max_points','allowed_sources','review_mode','turnover_multiple'] OR jsonb_typeof(v->'enabled')<>'boolean' OR jsonb_typeof(v->'min_points')<>'string' OR NOT (v->>'min_points') ~ '^[1-9][0-9]{0,18}$' OR (v->>'min_points')::numeric>9223372036854775807 OR jsonb_typeof(v->'allowed_sources')<>'array' OR jsonb_typeof(v->'review_mode')<>'string' OR v->>'review_mode' NOT IN ('manual','automatic') OR NOT valid_withdrawal_multiple(v->'turnover_multiple') THEN RETURN false; END IF;
 IF v->'max_points'<>'null'::jsonb AND (jsonb_typeof(v->'max_points')<>'string' OR NOT (v->>'max_points') ~ '^[1-9][0-9]{0,18}$' OR (v->>'max_points')::numeric>9223372036854775807 OR (v->>'max_points')::numeric<(v->>'min_points')::numeric) THEN RETURN false; END IF;
 sources:=jsonb_array_length(v->'allowed_sources');
 IF sources<1 OR sources>4 OR EXISTS(SELECT 1 FROM jsonb_array_elements(v->'allowed_sources') s WHERE jsonb_typeof(s)<>'string' OR s#>>'{}' NOT IN ('recharge','winning','gift','commission')) OR (SELECT count(DISTINCT s) FROM jsonb_array_elements(v->'allowed_sources') s)<>sources THEN RETURN false; END IF;
 RETURN true;
EXCEPTION WHEN OTHERS THEN RETURN false;
END $$;

-- Preserve restore behavior and exclude caller temporary objects.
DO $$
DECLARE app_schema text:=current_schema(); f record;
BEGIN
 IF app_schema IS NULL THEN RAISE EXCEPTION 'application schema required'; END IF;
 FOR f IN SELECT p.proname,pg_get_function_identity_arguments(p.oid) AS args
  FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
  WHERE n.nspname=app_schema AND p.proname IN (
   'initialize_commission_point_buckets','point_zero_snapshot','valid_point_snapshot',
   'normalize_point_snapshot','guard_point_ledger_snapshot','validate_point_ledger_projection',
   'validate_settlement_payout_witness','correction_reversal_is_applied','guard_settlement_bet_update',
   'guard_point_reconciliation_evidence','validate_withdrawal_funds','valid_withdrawal_policy')
 LOOP
  EXECUTE format('ALTER FUNCTION %I.%I(%s) SET search_path = pg_catalog, %I, pg_temp',app_schema,f.proname,f.args,app_schema);
 END LOOP;
END $$;
