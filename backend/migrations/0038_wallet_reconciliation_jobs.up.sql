CREATE TABLE point_reconciliation_jobs (
 id uuid PRIMARY KEY,
 brand_id uuid NOT NULL REFERENCES brands(id),
 state text NOT NULL DEFAULT 'pending' CHECK(state IN('pending','running','completed','failed')),
 version bigint NOT NULL DEFAULT 1 CHECK(version BETWEEN 1 AND 9007199254740991),
 target_count integer NOT NULL CHECK(target_count BETWEEN 0 AND 100000),
 created_by uuid NOT NULL REFERENCES admin_accounts(id),
 reason text NOT NULL CHECK(octet_length(reason) BETWEEN 1 AND 500 AND trim(reason)=reason),
 created_at timestamptz NOT NULL,
 last_step_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 started_at timestamptz,
 completed_at timestamptz,
 creation_audit_log_id uuid NOT NULL REFERENCES audit_logs(id),
 creation_xid xid8 NOT NULL DEFAULT pg_current_xact_id(),
 last_failure_id uuid,
 UNIQUE(brand_id,id),
 CHECK((state='completed')=(completed_at IS NOT NULL)),
 CHECK(state NOT IN('running','completed') OR started_at IS NOT NULL),
 CHECK((state='failed')=(last_failure_id IS NOT NULL)),
 CHECK(started_at IS NULL OR started_at>=created_at),
 CHECK(completed_at IS NULL OR completed_at>=started_at)
);
CREATE UNIQUE INDEX one_active_point_reconciliation ON point_reconciliation_jobs(brand_id) WHERE state IN('pending','running');
CREATE INDEX point_reconciliation_job_history ON point_reconciliation_jobs(brand_id,created_at DESC,id DESC);
CREATE INDEX point_reconciliation_worker_jobs ON point_reconciliation_jobs(last_step_at,id) WHERE state IN('pending','running');
CREATE TABLE point_reconciliation_targets (
 id uuid PRIMARY KEY,
 brand_id uuid NOT NULL,
 job_id uuid NOT NULL,
 account_id uuid NOT NULL,
 member_id uuid NOT NULL,
 state text NOT NULL DEFAULT 'pending' CHECK(state IN('pending','checked','failed')),
 attempt_count integer NOT NULL DEFAULT 0 CHECK(attempt_count>=0),
 next_check_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(brand_id,job_id,id),
 UNIQUE(brand_id,job_id,account_id),
 FOREIGN KEY(brand_id,job_id) REFERENCES point_reconciliation_jobs(brand_id,id),
 FOREIGN KEY(brand_id,account_id,member_id) REFERENCES point_accounts(brand_id,id,brand_member_id)
);
CREATE INDEX point_reconciliation_due ON point_reconciliation_targets(job_id,next_check_at,id) WHERE state='pending';
CREATE INDEX point_reconciliation_target_states ON point_reconciliation_targets(job_id,state);
CREATE TABLE point_reconciliation_results (
 target_id uuid PRIMARY KEY,
 brand_id uuid NOT NULL,
 job_id uuid NOT NULL,
 outcome text NOT NULL CHECK(outcome IN('consistent','repairable','corrupt')),
 preview jsonb NOT NULL CHECK(jsonb_typeof(preview)='object'),
 checked_at timestamptz NOT NULL,
 audit_log_id uuid NOT NULL REFERENCES audit_logs(id),
 FOREIGN KEY(brand_id,job_id,target_id) REFERENCES point_reconciliation_targets(brand_id,job_id,id)
);
CREATE INDEX point_reconciliation_outcomes ON point_reconciliation_results(brand_id,job_id,outcome,target_id);
CREATE TABLE point_reconciliation_failures (
 id uuid PRIMARY KEY,
 brand_id uuid NOT NULL,
 job_id uuid NOT NULL,
 target_id uuid NOT NULL,
 attempt_count integer NOT NULL CHECK(attempt_count>0),
 error_code text NOT NULL CHECK(error_code='CHECK_FAILED'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 audit_log_id uuid NOT NULL REFERENCES audit_logs(id),
 UNIQUE(brand_id,job_id,id),
 UNIQUE(target_id,attempt_count),
 FOREIGN KEY(brand_id,job_id,target_id) REFERENCES point_reconciliation_targets(brand_id,job_id,id)
);
ALTER TABLE point_reconciliation_jobs ADD FOREIGN KEY(brand_id,id,last_failure_id) REFERENCES point_reconciliation_failures(brand_id,job_id,id);
CREATE TABLE point_reconciliation_retries (
 id uuid PRIMARY KEY,
 brand_id uuid NOT NULL,
 job_id uuid NOT NULL,
 version bigint NOT NULL CHECK(version>1),
 previous_failure_id uuid NOT NULL,
 created_by uuid NOT NULL REFERENCES admin_accounts(id),
 reason text NOT NULL CHECK(octet_length(reason) BETWEEN 1 AND 500 AND trim(reason)=reason),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 audit_log_id uuid NOT NULL REFERENCES audit_logs(id),
 UNIQUE(job_id,version),
 FOREIGN KEY(brand_id,job_id) REFERENCES point_reconciliation_jobs(brand_id,id),
 FOREIGN KEY(brand_id,job_id,previous_failure_id) REFERENCES point_reconciliation_failures(brand_id,job_id,id)
);

CREATE FUNCTION guard_point_reconciliation_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
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
 IF expected IS NULL THEN expected:='{"recharge":{"available":"0","manual_frozen":"0","system_frozen":"0","withdrawal":"0"},"winning":{"available":"0","manual_frozen":"0","system_frozen":"0","withdrawal":"0"},"gift":{"available":"0","manual_frozen":"0","system_frozen":"0","withdrawal":"0"}}'::jsonb; END IF;
 IF a.action IS DISTINCT FROM 'wallet.reconciliation.checked' OR a.after_json->>'outcome' IS DISTINCT FROM NEW.outcome OR a.after_json->'preview' IS DISTINCT FROM NEW.preview OR (a.after_json->>'checked_at')::timestamptz IS DISTINCT FROM NEW.checked_at OR
  NEW.preview->>'account_id' IS DISTINCT FROM t.account_id::text OR NEW.preview->>'member_id' IS DISTINCT FROM t.member_id::text OR NEW.preview->>'version' IS DISTINCT FROM p.version::text OR NEW.preview->>'ledger_version' IS DISTINCT FROM n::text OR NEW.preview->'actual' IS DISTINCT FROM COALESCE(actual,'{}'::jsonb) OR jsonb_typeof(NEW.preview->'issues') IS DISTINCT FROM 'array' OR jsonb_typeof(NEW.preview->'consistent') IS DISTINCT FROM 'boolean' OR jsonb_typeof(NEW.preview->'repairable') IS DISTINCT FROM 'boolean' OR COALESCE(NEW.preview->>'token','') !~ '^[0-9a-f]{64}$' OR
  NEW.outcome IS DISTINCT FROM (CASE WHEN NEW.preview->>'consistent'='true' THEN 'consistent' WHEN NEW.preview->>'repairable'='true' THEN 'repairable' ELSE 'corrupt' END) THEN
  RAISE EXCEPTION 'reconciliation result audit or observed wallet mismatch';
 END IF;
 IF NEW.outcome IN('consistent','repairable') AND NEW.preview->'expected' IS DISTINCT FROM expected THEN RAISE EXCEPTION 'intact ledger expected balance mismatch'; END IF;
 IF NEW.outcome='consistent' AND (NEW.preview->>'repairable'<>'false' OR jsonb_array_length(NEW.preview->'issues')<>0 OR NEW.preview->'actual' IS DISTINCT FROM NEW.preview->'expected' OR p.version<>n) THEN RAISE EXCEPTION 'false consistent reconciliation'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER immutable_point_reconciliation_result BEFORE INSERT OR UPDATE OR DELETE ON point_reconciliation_results FOR EACH ROW EXECUTE FUNCTION guard_point_reconciliation_evidence();
CREATE TRIGGER immutable_point_reconciliation_failure BEFORE INSERT OR UPDATE OR DELETE ON point_reconciliation_failures FOR EACH ROW EXECUTE FUNCTION guard_point_reconciliation_evidence();
CREATE TRIGGER immutable_point_reconciliation_retry BEFORE INSERT OR UPDATE OR DELETE ON point_reconciliation_retries FOR EACH ROW EXECUTE FUNCTION guard_point_reconciliation_evidence();

CREATE FUNCTION guard_point_reconciliation_job() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE a audit_logs;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'reconciliation jobs cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN
  SELECT * INTO a FROM audit_logs WHERE id=NEW.creation_audit_log_id;
  IF NEW.state<>'pending' OR NEW.version<>1 OR NEW.creation_xid IS DISTINCT FROM pg_current_xact_id() OR NEW.started_at IS NOT NULL OR NEW.completed_at IS NOT NULL OR NEW.last_failure_id IS NOT NULL OR a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type IS DISTINCT FROM 'admin' OR a.actor_id IS DISTINCT FROM NEW.created_by OR a.resource_type IS DISTINCT FROM 'wallet_reconciliation_job' OR a.resource_id IS DISTINCT FROM NEW.id OR a.action IS DISTINCT FROM 'wallet.reconciliation.create' OR a.reason IS DISTINCT FROM NEW.reason OR a.after_json->>'target_count' IS DISTINCT FROM NEW.target_count::text OR (a.after_json->>'created_at')::timestamptz IS DISTINCT FROM NEW.created_at THEN RAISE EXCEPTION 'reconciliation creation audit mismatch'; END IF;
 ELSE
  IF (NEW.id,NEW.brand_id,NEW.target_count,NEW.created_by,NEW.reason,NEW.created_at,NEW.creation_audit_log_id,NEW.creation_xid) IS DISTINCT FROM (OLD.id,OLD.brand_id,OLD.target_count,OLD.created_by,OLD.reason,OLD.created_at,OLD.creation_audit_log_id,OLD.creation_xid) OR NEW.version<>OLD.version+1 OR OLD.state='completed' OR OLD.started_at IS NOT NULL AND NEW.started_at IS DISTINCT FROM OLD.started_at THEN RAISE EXCEPTION 'invalid reconciliation job identity or version'; END IF;
  IF OLD.state='failed' THEN
   IF NEW.state<>'pending' OR NOT EXISTS(SELECT 1 FROM point_reconciliation_retries WHERE job_id=NEW.id AND version=NEW.version AND previous_failure_id=OLD.last_failure_id) THEN RAISE EXCEPTION 'failed job requires audited manual retry'; END IF;
  ELSIF NEW.state NOT IN('running','completed','failed') THEN RAISE EXCEPTION 'invalid reconciliation transition'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_point_reconciliation_job BEFORE INSERT OR UPDATE OR DELETE ON point_reconciliation_jobs FOR EACH ROW EXECUTE FUNCTION guard_point_reconciliation_job();
CREATE FUNCTION guard_point_reconciliation_target() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE j point_reconciliation_jobs;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'reconciliation targets cannot be deleted'; END IF;
 SELECT * INTO j FROM point_reconciliation_jobs WHERE id=NEW.job_id AND brand_id=NEW.brand_id;
 IF TG_OP='INSERT' THEN
  IF j.version<>1 OR j.state<>'pending' OR NEW.state<>'pending' OR NEW.attempt_count<>0 OR j.creation_xid IS DISTINCT FROM pg_current_xact_id() THEN RAISE EXCEPTION 'target must be captured in the creation transaction'; END IF;
 ELSE
  IF (NEW.id,NEW.brand_id,NEW.job_id,NEW.account_id,NEW.member_id) IS DISTINCT FROM (OLD.id,OLD.brand_id,OLD.job_id,OLD.account_id,OLD.member_id) OR OLD.state='checked' THEN RAISE EXCEPTION 'reconciliation target identity and results are immutable'; END IF;
  IF NEW.state='checked' THEN
   IF OLD.state<>'pending' OR NEW.attempt_count<>OLD.attempt_count+1 OR NOT EXISTS(SELECT 1 FROM point_reconciliation_results WHERE target_id=NEW.id) THEN RAISE EXCEPTION 'checked target requires observation'; END IF;
  ELSIF NEW.state='failed' THEN
   IF OLD.state<>'pending' OR NEW.attempt_count<>OLD.attempt_count+1 OR NOT EXISTS(SELECT 1 FROM point_reconciliation_failures WHERE target_id=NEW.id AND attempt_count=NEW.attempt_count) THEN RAISE EXCEPTION 'failed target requires attempt'; END IF;
  ELSIF NEW.state='pending' THEN
   IF NEW.attempt_count<>OLD.attempt_count OR OLD.state='failed' AND (j.state<>'failed' OR NOT EXISTS(SELECT 1 FROM point_reconciliation_retries WHERE job_id=j.id AND version=j.version+1)) THEN RAISE EXCEPTION 'target retry requires job retry evidence'; END IF;
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_point_reconciliation_target BEFORE INSERT OR UPDATE OR DELETE ON point_reconciliation_targets FOR EACH ROW EXECUTE FUNCTION guard_point_reconciliation_target();
CREATE FUNCTION validate_point_reconciliation_job() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE j point_reconciliation_jobs; n bigint;
BEGIN
 SELECT * INTO j FROM point_reconciliation_jobs WHERE id=NEW.id;
 IF j.version=1 OR j.state='completed' THEN
  SELECT count(*) INTO n FROM point_reconciliation_targets WHERE job_id=j.id;
  IF n<>j.target_count THEN RAISE EXCEPTION 'reconciliation target scope differs'; END IF;
 END IF;
 IF j.state='completed' AND EXISTS(SELECT 1 FROM point_reconciliation_targets WHERE job_id=j.id AND state<>'checked') OR j.state IN('pending','running') AND EXISTS(SELECT 1 FROM point_reconciliation_targets WHERE job_id=j.id AND state='failed') OR j.state='failed' AND NOT EXISTS(SELECT 1 FROM point_reconciliation_failures f JOIN point_reconciliation_targets t ON t.id=f.target_id WHERE f.id=j.last_failure_id AND t.state='failed' AND t.attempt_count=f.attempt_count) THEN RAISE EXCEPTION 'reconciliation job and targets disagree'; END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER complete_point_reconciliation_job AFTER INSERT OR UPDATE ON point_reconciliation_jobs DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_point_reconciliation_job();

-- Fixed function search paths preserve restore and isolated-schema behavior.
DO $$ DECLARE f record; s text:=current_schema(); BEGIN
 FOR f IN SELECT p.oid::regprocedure::text signature FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=s AND p.proname IN('guard_point_reconciliation_evidence','guard_point_reconciliation_job','guard_point_reconciliation_target','validate_point_reconciliation_job') LOOP
  EXECUTE format('ALTER FUNCTION %s SET search_path TO pg_catalog, %I, pg_temp',f.signature,s);
 END LOOP;
END $$;
INSERT INTO permissions(key) VALUES('wallet.reconcile.brand') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key) SELECT id,'wallet.reconcile.brand' FROM roles WHERE is_bootstrap AND brand_id IS NOT NULL ON CONFLICT DO NOTHING;
