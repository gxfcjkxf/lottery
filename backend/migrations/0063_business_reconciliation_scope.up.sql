-- Existing jobs and receipts remain wallet-only. New full checks explicitly
-- opt in; they observe, never move points or repair business records.
ALTER TABLE point_reconciliation_jobs ADD COLUMN check_scope text NOT NULL DEFAULT 'wallet'
 CHECK(check_scope IN('wallet','wallet_and_business'));
ALTER TABLE point_reconciliation_results ADD COLUMN business_preview jsonb
 CHECK(business_preview IS NULL OR jsonb_typeof(business_preview)='object');
CREATE INDEX business_reconciliation_point_audit_idx ON audit_logs(brand_id,(after_json->>'ledger_entry_id'),action,resource_id)
 WHERE resource_type='point_account';

CREATE FUNCTION point_business_family(kind text) RETURNS text LANGUAGE sql IMMUTABLE AS $$
 SELECT CASE
  WHEN kind IN('adjust','adjustment','freeze','unfreeze','withdrawal_transfer','reversal') THEN 'manual'
  WHEN kind IN('withdrawal_reserve','withdrawal_release','withdrawal_paid') THEN 'withdrawal'
  WHEN kind IN('reward_grant','reward_reversal') THEN 'reward'
  WHEN kind='recharge' THEN 'recharge'
  WHEN kind IN('bet','refund','prize','prize_reversal','commission','commission_adjustment','commission_correction') THEN kind
  ELSE 'unknown' END
$$;

CREATE FUNCTION point_business_preview(b uuid,account uuid) RETURNS jsonb LANGUAGE sql STABLE AS $$
 WITH scope AS MATERIALIZED (
  SELECT id,brand_member_id,version FROM point_accounts WHERE brand_id=b AND id=account
 ), ledgers AS MATERIALIZED (
  SELECT l.*,point_business_family(l.entry_type) AS family FROM point_ledger_entries l
  JOIN scope s ON s.id=l.account_id WHERE l.brand_id=b
 ), witnesses AS MATERIALIZED (
  SELECT * FROM point_lottery_business_witnesses(b,account)
  UNION ALL SELECT * FROM point_finance_business_witnesses(b,account)
  UNION ALL
  SELECT 'manual',l.id,l.id,
   (SELECT count(*)=1 FROM audit_logs a
    WHERE a.brand_id=b AND a.actor_type=l.actor_type AND a.actor_id IS NOT DISTINCT FROM l.actor_id
     AND a.action='points.'||l.entry_type AND a.resource_type='point_account' AND a.resource_id=account
     AND a.reason=l.reason AND a.request_id=l.request_id
     AND a.after_json->>'ledger_entry_id'=l.id::text
     AND a.before_json=l.before_snapshot AND a.after_json=jsonb_build_object('ledger_entry_id',l.id,'version',l.version,'balance',l.after_snapshot)),
   l.entry_type FROM ledgers l WHERE l.family='manual'
 ), problems AS MATERIALIZED (
  SELECT l.family,'UNSUPPORTED_LEDGER_TYPE'::text AS code,l.entry_type,l.id AS ledger_id,
   'ledger'::text AS resource_type,l.id AS resource_id FROM ledgers l WHERE l.family='unknown'
  UNION ALL
  SELECT l.family,'INVALID_BUSINESS_BINDING',l.entry_type,l.id,'ledger',l.id FROM ledgers l
   WHERE l.family<>'manual' AND (SELECT count(*) FROM audit_logs a
    WHERE a.brand_id=b AND a.actor_type=l.actor_type AND a.actor_id IS NOT DISTINCT FROM l.actor_id
     AND a.action='points.'||l.entry_type AND a.resource_type='point_account' AND a.resource_id=account
     AND a.reason=l.reason AND a.request_id=l.request_id AND a.after_json->>'ledger_entry_id'=l.id::text
     AND a.before_json=l.before_snapshot AND a.after_json=jsonb_build_object('ledger_entry_id',l.id,'version',l.version,'balance',l.after_snapshot))<>1
  UNION ALL
  SELECT l.family,'MISSING_BUSINESS_RECORD',l.entry_type,l.id,'ledger',l.id FROM ledgers l
   WHERE l.family NOT IN('unknown','manual') AND NOT EXISTS(SELECT 1 FROM witnesses w WHERE w.ledger_id=l.id)
  UNION ALL
  SELECT w.family,'MISSING_LEDGER_ENTRY',w.expected_entry_type,w.ledger_id,w.family,w.source_id FROM witnesses w
   WHERE w.ledger_id IS NULL OR NOT EXISTS(SELECT 1 FROM ledgers l WHERE l.id=w.ledger_id)
  UNION ALL
  SELECT w.family,CASE WHEN w.family='manual' THEN 'INVALID_MANUAL_AUDIT' ELSE 'INVALID_BUSINESS_BINDING' END,
   w.expected_entry_type,w.ledger_id,w.family,w.source_id FROM witnesses w
   WHERE w.ledger_id IS NOT NULL AND w.source_valid IS DISTINCT FROM true AND EXISTS(SELECT 1 FROM ledgers l WHERE l.id=w.ledger_id)
  UNION ALL
  SELECT l.family,'DUPLICATE_BUSINESS_BINDING',l.entry_type,l.id,'ledger',l.id FROM ledgers l
   WHERE (SELECT count(*) FROM witnesses w WHERE w.ledger_id=l.id)>1
 ), families AS (
  SELECT unnest(ARRAY['bet','commission','commission_adjustment','commission_correction','manual','prize','prize_reversal','recharge','refund','reward','unknown','withdrawal']) AS family
 ), coverage AS (
  SELECT f.family,(SELECT count(*)::text FROM ledgers l WHERE l.family=f.family) AS ledger_entry_count,
   (SELECT count(*)::text FROM witnesses w WHERE w.family=f.family) AS business_reference_count,
   (SELECT count(*)::text FROM problems p WHERE p.family=f.family) AS issue_count FROM families f
 ), limited AS (
  SELECT code,entry_type,ledger_id AS ledger_entry_id,resource_type,resource_id FROM problems
  ORDER BY code COLLATE "C",resource_type COLLATE "C",coalesce(resource_id::text,''),coalesce(ledger_id::text,''),entry_type COLLATE "C" LIMIT 100
 ), material AS (
  SELECT jsonb_build_object('brand_id',b,'account_id',account,'account_version',(SELECT version FROM scope),
   'ledgers',coalesce((SELECT jsonb_agg(to_jsonb(l) ORDER BY version,id) FROM ledgers l),'[]'::jsonb),
   'witnesses',coalesce((SELECT jsonb_agg(to_jsonb(w) ORDER BY family COLLATE "C",source_id,ledger_id,expected_entry_type COLLATE "C") FROM witnesses w),'[]'::jsonb),
   'problems',coalesce((SELECT jsonb_agg(to_jsonb(p) ORDER BY code COLLATE "C",resource_type COLLATE "C",resource_id,ledger_id,entry_type COLLATE "C") FROM problems p),'[]'::jsonb)) AS data
 )
 SELECT jsonb_build_object('account_id',s.id,'member_id',s.brand_member_id,'account_version',s.version,
  'ledger_entry_count',(SELECT count(*)::text FROM ledgers),'business_reference_count',(SELECT count(*)::text FROM witnesses),
  'issue_count',(SELECT count(*)::text FROM problems),'issues_truncated',(SELECT count(*)>100 FROM problems),
  'consistent',NOT EXISTS(SELECT 1 FROM problems),'fingerprint',encode(sha256(convert_to(material.data::text,'UTF8')),'hex'),
  'issues',coalesce((SELECT jsonb_agg(to_jsonb(x) ORDER BY code COLLATE "C",resource_type COLLATE "C",coalesce(resource_id::text,''),coalesce(ledger_entry_id::text,''),entry_type COLLATE "C") FROM limited x),'[]'::jsonb),
  'coverage',(SELECT jsonb_agg(to_jsonb(c) ORDER BY family COLLATE "C") FROM coverage c))
 FROM scope s CROSS JOIN material
$$;

CREATE FUNCTION guard_point_reconciliation_scope() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE a audit_logs;
BEGIN
 IF TG_OP='UPDATE' AND NEW.check_scope IS DISTINCT FROM OLD.check_scope THEN
  RAISE EXCEPTION 'reconciliation scope is immutable';
 END IF;
 IF TG_OP='INSERT' THEN
  SELECT * INTO a FROM audit_logs WHERE id=NEW.creation_audit_log_id;
  IF coalesce(a.after_json->>'check_scope','wallet') IS DISTINCT FROM NEW.check_scope THEN
   RAISE EXCEPTION 'reconciliation scope requires explicit matching creation audit';
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_point_reconciliation_scope BEFORE INSERT OR UPDATE ON point_reconciliation_jobs
 FOR EACH ROW EXECUTE FUNCTION guard_point_reconciliation_scope();

-- Preserve the existing wallet proof, failure/retry checks and immutable
-- history; only full-scope results additionally require a recomputed witness.
CREATE OR REPLACE FUNCTION guard_point_reconciliation_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE t point_reconciliation_targets; j point_reconciliation_jobs; a audit_logs; p point_accounts; actual jsonb; expected jsonb; business jsonb; n bigint; wanted text;
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
 ) v;
 SELECT count(*) INTO n FROM point_ledger_entries WHERE brand_id=t.brand_id AND account_id=t.account_id;
 SELECT after_snapshot INTO expected FROM point_ledger_entries WHERE brand_id=t.brand_id AND account_id=t.account_id ORDER BY version DESC LIMIT 1;
 IF expected IS NULL THEN expected:=point_zero_snapshot(); ELSE expected:=normalize_point_snapshot(expected); END IF;
 IF j.check_scope='wallet_and_business' THEN
  business:=point_business_preview(t.brand_id,t.account_id);
  IF NEW.business_preview IS NULL OR business IS NULL OR NEW.business_preview IS DISTINCT FROM business OR
   a.after_json->'business_preview' IS DISTINCT FROM business THEN RAISE EXCEPTION 'full reconciliation requires exact current business witness'; END IF;
 ELSIF NEW.business_preview IS NOT NULL OR a.after_json ? 'business_preview' AND a.after_json->'business_preview'<>'null'::jsonb THEN
  RAISE EXCEPTION 'wallet-only observation cannot claim business coverage';
 END IF;
 wanted:=CASE WHEN business IS NOT NULL AND business->>'consistent'<>'true' THEN 'corrupt'
  WHEN NEW.preview->>'consistent'='true' THEN 'consistent' WHEN NEW.preview->>'repairable'='true' THEN 'repairable' ELSE 'corrupt' END;
 IF a.action IS DISTINCT FROM 'wallet.reconciliation.checked' OR a.after_json->>'outcome' IS DISTINCT FROM NEW.outcome OR a.after_json->'preview' IS DISTINCT FROM NEW.preview OR (a.after_json->>'checked_at')::timestamptz IS DISTINCT FROM NEW.checked_at OR
  NEW.preview->>'account_id' IS DISTINCT FROM t.account_id::text OR NEW.preview->>'member_id' IS DISTINCT FROM t.member_id::text OR NEW.preview->>'version' IS DISTINCT FROM p.version::text OR NEW.preview->>'ledger_version' IS DISTINCT FROM n::text OR NEW.preview->'actual' IS DISTINCT FROM COALESCE(actual,'{}'::jsonb) OR jsonb_typeof(NEW.preview->'issues') IS DISTINCT FROM 'array' OR jsonb_typeof(NEW.preview->'consistent') IS DISTINCT FROM 'boolean' OR jsonb_typeof(NEW.preview->'repairable') IS DISTINCT FROM 'boolean' OR COALESCE(NEW.preview->>'token','') !~ '^[0-9a-f]{64}$' OR NEW.outcome IS DISTINCT FROM wanted THEN
  RAISE EXCEPTION 'reconciliation result audit or observed wallet mismatch';
 END IF;
 -- Full business failures change the combined outcome, never weaken the
 -- independent wallet proof (including its expected balance and pass flags).
 IF (NEW.preview->>'consistent'='true' OR NEW.preview->>'repairable'='true') AND NEW.preview->'expected' IS DISTINCT FROM expected THEN RAISE EXCEPTION 'intact ledger expected balance mismatch'; END IF;
 IF NEW.preview->>'consistent'='true' AND (NEW.preview->>'repairable'<>'false' OR jsonb_array_length(NEW.preview->'issues')<>0 OR NEW.preview->'actual' IS DISTINCT FROM NEW.preview->'expected' OR p.version<>n) THEN RAISE EXCEPTION 'false consistent reconciliation'; END IF;
 RETURN NEW;
END $$;

DO $$ DECLARE s text:=current_schema(); f record; BEGIN
 FOR f IN SELECT p.oid::regprocedure signature FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=s AND p.proname IN(
  'point_business_family','point_business_preview','guard_point_reconciliation_scope','guard_point_reconciliation_evidence') LOOP
  EXECUTE format('ALTER FUNCTION %s SET search_path TO pg_catalog, %I, pg_temp',f.signature,s);
 END LOOP;
END $$;
