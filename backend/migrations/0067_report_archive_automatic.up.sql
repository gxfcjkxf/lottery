-- Operational report jobs. No posting clock, wallet, financial event or old
-- archive payload is modified. Existing brands begin with both gates closed.
CREATE TABLE brand_report_archive_policies (
 brand_id uuid PRIMARY KEY REFERENCES brands(id), version bigint NOT NULL DEFAULT 1 CHECK(version BETWEEN 1 AND 9007199254740991),
 daily_enabled boolean NOT NULL DEFAULT false, monthly_enabled boolean NOT NULL DEFAULT false,
 daily_start_period text, monthly_start_period text, timezone text NOT NULL,
 audit_log_id uuid REFERENCES audit_logs(id), updated_at timestamptz NOT NULL DEFAULT statement_timestamp(),
 CHECK(NOT daily_enabled OR daily_start_period IS NOT NULL), CHECK(NOT monthly_enabled OR monthly_start_period IS NOT NULL)
);
CREATE TABLE report_archive_policy_revisions (
 LIKE brand_report_archive_policies INCLUDING DEFAULTS INCLUDING CONSTRAINTS,
 PRIMARY KEY(brand_id,version), FOREIGN KEY(brand_id) REFERENCES brand_report_archive_policies(brand_id), FOREIGN KEY(audit_log_id) REFERENCES audit_logs(id)
);
CREATE FUNCTION guard_report_archive_policy() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE a audit_logs; expected jsonb;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'archive policy cannot be deleted'; END IF;
 IF NEW.timezone='Local' OR NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.timezone) THEN RAISE EXCEPTION 'invalid archive policy timezone'; END IF;
 IF NEW.daily_start_period IS NOT NULL THEN
  IF NEW.daily_start_period !~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' OR substring(NEW.daily_start_period,1,4)::integer NOT BETWEEN 1 AND 9998 THEN RAISE EXCEPTION 'invalid daily activation'; END IF;
  PERFORM make_date(substring(NEW.daily_start_period,1,4)::integer,substring(NEW.daily_start_period,6,2)::integer,substring(NEW.daily_start_period,9,2)::integer);
 END IF;
 IF NEW.monthly_start_period IS NOT NULL THEN
  IF NEW.monthly_start_period !~ '^[0-9]{4}-[0-9]{2}$' OR substring(NEW.monthly_start_period,1,4)::integer NOT BETWEEN 1 AND 9998 THEN RAISE EXCEPTION 'invalid monthly activation'; END IF;
  PERFORM make_date(substring(NEW.monthly_start_period,1,4)::integer,substring(NEW.monthly_start_period,6,2)::integer,1);
 END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR NEW.daily_enabled OR NEW.monthly_enabled OR NEW.daily_start_period IS NOT NULL OR NEW.monthly_start_period IS NOT NULL OR NEW.audit_log_id IS NOT NULL OR NOT EXISTS(SELECT 1 FROM brands WHERE id=NEW.brand_id AND timezone=NEW.timezone) THEN RAISE EXCEPTION 'archive policy initializes disabled only'; END IF;
  RETURN NEW;
 END IF;
 IF NEW.brand_id<>OLD.brand_id OR NEW.version<>OLD.version+1 OR NEW.updated_at IS DISTINCT FROM statement_timestamp() THEN RAISE EXCEPTION 'archive policy requires next version'; END IF;
 SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
 expected:=jsonb_build_object('brand_id',NEW.brand_id,'version',NEW.version,'daily_enabled',NEW.daily_enabled,'monthly_enabled',NEW.monthly_enabled,'daily_start_period',NEW.daily_start_period,'monthly_start_period',NEW.monthly_start_period,'timezone',NEW.timezone);
 IF a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type<>'admin' OR a.actor_id IS NULL OR a.action<>'report_archive.policy.update' OR a.resource_type<>'report_archive_policy' OR a.resource_id IS DISTINCT FROM NEW.brand_id OR a.reason IS NULL OR btrim(a.reason)='' OR
  (a.before_json-'updated_at') IS DISTINCT FROM (to_jsonb(OLD)-'updated_at') OR (a.before_json->>'updated_at')::timestamptz IS DISTINCT FROM OLD.updated_at OR a.after_json IS DISTINCT FROM expected THEN RAISE EXCEPTION 'archive policy requires exact audited intent'; END IF;
 IF NOT EXISTS(SELECT 1 FROM brands WHERE id=NEW.brand_id AND status<>'disabled' AND timezone=NEW.timezone) THEN RAISE EXCEPTION 'archive policy requires current enabled brand'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_report_archive_policy BEFORE INSERT OR UPDATE OR DELETE ON brand_report_archive_policies FOR EACH ROW EXECUTE FUNCTION guard_report_archive_policy();
CREATE FUNCTION capture_report_archive_policy() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN INSERT INTO report_archive_policy_revisions SELECT NEW.*; RETURN NULL; END $$;
CREATE TRIGGER captured_report_archive_policy AFTER INSERT OR UPDATE ON brand_report_archive_policies FOR EACH ROW EXECUTE FUNCTION capture_report_archive_policy();
CREATE FUNCTION guard_report_archive_policy_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN IF TG_OP<>'INSERT' OR NOT EXISTS(SELECT 1 FROM brand_report_archive_policies p WHERE p.brand_id=NEW.brand_id AND to_jsonb(p)=to_jsonb(NEW)) THEN RAISE EXCEPTION 'archive policy revision is immutable'; END IF; RETURN NEW; END $$;
CREATE TRIGGER guarded_report_archive_policy_revision BEFORE INSERT OR UPDATE OR DELETE ON report_archive_policy_revisions FOR EACH ROW EXECUTE FUNCTION guard_report_archive_policy_revision();
INSERT INTO brand_report_archive_policies(brand_id,timezone) SELECT id,timezone FROM brands;
CREATE FUNCTION initialize_report_archive_policy() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN INSERT INTO brand_report_archive_policies(brand_id,timezone) VALUES(NEW.id,NEW.timezone); RETURN NULL; END $$;
CREATE TRIGGER initialized_report_archive_policy AFTER INSERT ON brands FOR EACH ROW EXECUTE FUNCTION initialize_report_archive_policy();

CREATE TABLE report_archive_automatic_cursors (
 brand_id uuid NOT NULL,policy_version bigint NOT NULL,kind text NOT NULL CHECK(kind IN('daily','monthly')),next_period_key text,due_at timestamptz,
 PRIMARY KEY(brand_id,policy_version,kind), FOREIGN KEY(brand_id,policy_version) REFERENCES report_archive_policy_revisions(brand_id,version)
);
CREATE TABLE report_archive_automatic_tasks (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL,policy_version bigint NOT NULL,kind text NOT NULL CHECK(kind IN('daily','monthly')),period_key text NOT NULL,timezone text NOT NULL,
 from_at timestamptz NOT NULL,to_at timestamptz NOT NULL CHECK(to_at>from_at),state text NOT NULL DEFAULT 'pending' CHECK(state IN('pending','completed','skipped','failed')),
 version bigint NOT NULL DEFAULT 1 CHECK(version BETWEEN 1 AND 9007199254740991),attempt_count bigint NOT NULL DEFAULT 0 CHECK(attempt_count BETWEEN 0 AND 9007199254740991),
 archive_id uuid,last_error_code text CHECK(last_error_code='ARCHIVE_FAILED'),creation_audit_log_id uuid NOT NULL REFERENCES audit_logs(id),last_audit_log_id uuid NOT NULL REFERENCES audit_logs(id),
 created_at timestamptz NOT NULL DEFAULT statement_timestamp(),updated_at timestamptz NOT NULL DEFAULT statement_timestamp(),
 UNIQUE(brand_id,id),UNIQUE(brand_id,kind,period_key),FOREIGN KEY(brand_id,policy_version) REFERENCES report_archive_policy_revisions(brand_id,version),FOREIGN KEY(brand_id,archive_id) REFERENCES report_archives(brand_id,id),
 CHECK((state IN('completed','skipped'))=(archive_id IS NOT NULL)),CHECK((state='failed')=(last_error_code IS NOT NULL))
);
CREATE INDEX report_archive_automatic_pending ON report_archive_automatic_tasks(created_at,id) WHERE state='pending';
CREATE FUNCTION report_archive_period_end_boundary(civil timestamp,z text) RETURNS timestamptz LANGUAGE plpgsql STABLE AS $$
DECLARE step integer; result timestamptz;
BEGIN
 FOR step IN 0..7 LOOP
  result:=report_archive_period_boundary(civil+step*interval '1 day',z);
  IF result IS NOT NULL THEN RETURN result; END IF;
 END LOOP;
 RETURN NULL;
END $$;
CREATE FUNCTION report_archive_cursor_due(k text,key text,z text) RETURNS timestamptz LANGUAGE plpgsql STABLE AS $$
DECLARE d timestamp; e timestamp; f timestamptz; t timestamptz;
BEGIN
 IF key IS NULL THEN RETURN NULL; END IF;
 IF k='daily' AND key ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' THEN d:=make_timestamp(substring(key,1,4)::integer,substring(key,6,2)::integer,substring(key,9,2)::integer,0,0,0);e:=d+interval '1 day';
 ELSIF k='monthly' AND key ~ '^[0-9]{4}-[0-9]{2}$' THEN d:=make_timestamp(substring(key,1,4)::integer,substring(key,6,2)::integer,1,0,0,0);e:=d+interval '1 month'; ELSE RAISE EXCEPTION 'invalid archive cursor key'; END IF;
 IF extract(year FROM d) NOT BETWEEN 1 AND 9998 THEN RAISE EXCEPTION 'invalid archive cursor year'; END IF;
 f:=CASE WHEN k='monthly' THEN report_archive_period_end_boundary(d,z) ELSE report_archive_period_boundary(d,z) END;t:=report_archive_period_end_boundary(e,z);
 IF f IS NULL OR t IS NULL THEN RETURN statement_timestamp(); END IF;
 RETURN t;
END $$;
CREATE FUNCTION guard_report_archive_cursor() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p report_archive_policy_revisions; key text; advanced text; d timestamp; ending timestamp; f timestamptz; t timestamptz; steps integer:=0;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'archive discovery cursor cannot be deleted'; END IF;
 SELECT * INTO p FROM report_archive_policy_revisions WHERE brand_id=NEW.brand_id AND version=NEW.policy_version;
 IF p.brand_id IS NULL THEN RAISE EXCEPTION 'archive cursor lacks policy snapshot'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.next_period_key IS DISTINCT FROM (CASE WHEN NEW.kind='daily' THEN p.daily_start_period ELSE p.monthly_start_period END) OR (NEW.kind='daily' AND NOT p.daily_enabled) OR (NEW.kind='monthly' AND NOT p.monthly_enabled) THEN RAISE EXCEPTION 'archive cursor starts at saved activation only'; END IF;
 ELSE
  IF NEW.brand_id<>OLD.brand_id OR NEW.policy_version<>OLD.policy_version OR NEW.kind<>OLD.kind OR OLD.next_period_key IS NULL OR NEW.next_period_key IS NOT DISTINCT FROM OLD.next_period_key THEN RAISE EXCEPTION 'invalid archive cursor change'; END IF;
  key:=OLD.next_period_key;
  WHILE key IS DISTINCT FROM NEW.next_period_key LOOP
   steps:=steps+1;IF steps>107 OR key IS NULL THEN RAISE EXCEPTION 'archive cursor advance exceeds bounded evidence'; END IF;
   IF NEW.kind='daily' THEN d:=make_timestamp(substring(key,1,4)::integer,substring(key,6,2)::integer,substring(key,9,2)::integer,0,0,0);ending:=d+interval '1 day';advanced:=to_char(ending,'YYYY-MM-DD');
   ELSE d:=make_timestamp(substring(key,1,4)::integer,substring(key,6,2)::integer,1,0,0,0);ending:=d+interval '1 month';advanced:=to_char(ending,'YYYY-MM'); END IF;
   f:=CASE WHEN NEW.kind='monthly' THEN report_archive_period_end_boundary(d,p.timezone) ELSE report_archive_period_boundary(d,p.timezone) END;t:=report_archive_period_end_boundary(ending,p.timezone);
   IF f IS NOT NULL AND t IS NOT NULL AND (t>clock_timestamp() OR NOT EXISTS(SELECT 1 FROM report_archive_automatic_tasks j WHERE j.brand_id=NEW.brand_id AND j.kind=NEW.kind AND j.period_key=key)) THEN RAISE EXCEPTION 'archive cursor cannot skip an undiscovered elapsed window'; END IF;
   IF extract(year FROM ending)>9998 THEN advanced:=NULL; END IF;
   key:=advanced;
  END LOOP;
 END IF;
 NEW.due_at:=report_archive_cursor_due(NEW.kind,NEW.next_period_key,p.timezone);
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_report_archive_cursor BEFORE INSERT OR UPDATE OR DELETE ON report_archive_automatic_cursors FOR EACH ROW EXECUTE FUNCTION guard_report_archive_cursor();
ALTER TABLE report_archives ALTER COLUMN created_by DROP NOT NULL;
ALTER TABLE report_archives ADD COLUMN automatic_task_id uuid,ADD COLUMN automatic_policy_version bigint;
ALTER TABLE report_archives ADD CONSTRAINT report_archive_automatic_identity CHECK((created_by IS NULL)=(automatic_task_id IS NOT NULL) AND (automatic_task_id IS NULL)=(automatic_policy_version IS NULL));
ALTER TABLE report_archives ADD CONSTRAINT report_archive_automatic_task_fk FOREIGN KEY(brand_id,automatic_task_id) REFERENCES report_archive_automatic_tasks(brand_id,id);
CREATE UNIQUE INDEX report_archive_one_automatic_task ON report_archives(automatic_task_id) WHERE automatic_task_id IS NOT NULL;

CREATE FUNCTION guard_report_archive_automatic_task() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p report_archive_policy_revisions; a audit_logs; target report_archives; boundary timestamp; ending timestamp; expected jsonb;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'automatic archive task cannot be deleted'; END IF;
 SELECT * INTO p FROM report_archive_policy_revisions WHERE brand_id=NEW.brand_id AND version=NEW.policy_version;
 IF p.brand_id IS NULL OR NEW.timezone IS DISTINCT FROM p.timezone OR (NEW.kind='daily' AND (NOT p.daily_enabled OR NEW.period_key<p.daily_start_period)) OR (NEW.kind='monthly' AND (NOT p.monthly_enabled OR NEW.period_key<p.monthly_start_period)) THEN RAISE EXCEPTION 'automatic task requires saved enabled policy'; END IF;
 IF TG_OP='INSERT' THEN
 IF NEW.kind='daily' AND NEW.period_key ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' THEN boundary:=make_timestamp(substring(NEW.period_key,1,4)::integer,substring(NEW.period_key,6,2)::integer,substring(NEW.period_key,9,2)::integer,0,0,0);ending:=boundary+interval '1 day';
 ELSIF NEW.kind='monthly' AND NEW.period_key ~ '^[0-9]{4}-[0-9]{2}$' THEN boundary:=make_timestamp(substring(NEW.period_key,1,4)::integer,substring(NEW.period_key,6,2)::integer,1,0,0,0);ending:=boundary+interval '1 month'; ELSE RAISE EXCEPTION 'automatic task requires canonical period'; END IF;
 IF extract(year FROM boundary) NOT BETWEEN 1 AND 9998 OR extract(year FROM NEW.from_at AT TIME ZONE 'UTC')<1 OR extract(year FROM NEW.to_at AT TIME ZONE 'UTC')>9999 OR NEW.from_at IS DISTINCT FROM (CASE WHEN NEW.kind='monthly' THEN report_archive_period_end_boundary(boundary,NEW.timezone) ELSE report_archive_period_boundary(boundary,NEW.timezone) END) OR NEW.to_at IS DISTINCT FROM report_archive_period_end_boundary(ending,NEW.timezone) OR NEW.to_at>clock_timestamp() THEN RAISE EXCEPTION 'automatic task calendar is not elapsed'; END IF;
 END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.state<>'pending' OR NEW.version<>1 OR NEW.attempt_count<>0 OR NEW.archive_id IS NOT NULL OR NEW.last_error_code IS NOT NULL OR NEW.last_audit_log_id<>NEW.creation_audit_log_id OR NEW.created_at IS DISTINCT FROM statement_timestamp() OR NEW.updated_at IS DISTINCT FROM NEW.created_at THEN RAISE EXCEPTION 'automatic task begins pending'; END IF;
  SELECT * INTO a FROM audit_logs WHERE id=NEW.creation_audit_log_id;
  expected:=jsonb_build_object('id',NEW.id,'kind',NEW.kind,'period_key',NEW.period_key,'timezone',NEW.timezone,'policy_version',NEW.policy_version);
  IF a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type<>'system' OR a.actor_id IS NOT NULL OR a.action<>'report_archive.task.enqueue' OR a.resource_type<>'report_archive_task' OR a.resource_id IS DISTINCT FROM NEW.id OR a.before_json<>'null'::jsonb OR a.after_json IS DISTINCT FROM expected OR
   NOT EXISTS(SELECT 1 FROM brand_report_archive_policies c JOIN brands b ON b.id=c.brand_id WHERE c.brand_id=NEW.brand_id AND c.version=NEW.policy_version AND b.status<>'disabled' AND ((NEW.kind='daily' AND c.daily_enabled) OR (NEW.kind='monthly' AND c.monthly_enabled))) THEN RAISE EXCEPTION 'automatic task requires audited current discovery'; END IF;
  RETURN NEW;
 END IF;
 IF (to_jsonb(NEW)-ARRAY['state','version','attempt_count','archive_id','last_error_code','last_audit_log_id','updated_at']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','version','attempt_count','archive_id','last_error_code','last_audit_log_id','updated_at']) OR NEW.version<>OLD.version+1 OR NEW.updated_at IS DISTINCT FROM statement_timestamp() THEN RAISE EXCEPTION 'automatic task scope and history immutable'; END IF;
 SELECT * INTO a FROM audit_logs WHERE id=NEW.last_audit_log_id;
 IF a.id IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.resource_type<>'report_archive_task' OR a.resource_id IS DISTINCT FROM NEW.id OR a.before_json IS DISTINCT FROM jsonb_build_object('state',OLD.state,'version',OLD.version,'attempt_count',OLD.attempt_count,'archive_id',OLD.archive_id,'last_error_code',OLD.last_error_code) OR a.after_json IS DISTINCT FROM jsonb_build_object('state',NEW.state,'version',NEW.version,'attempt_count',NEW.attempt_count,'archive_id',NEW.archive_id,'last_error_code',NEW.last_error_code) THEN RAISE EXCEPTION 'automatic task transition requires exact audit'; END IF;
 IF OLD.state='failed' AND NEW.state='pending' THEN
  IF a.action<>'report_archive.task.retry' OR a.actor_type<>'admin' OR a.actor_id IS NULL OR NEW.attempt_count<>OLD.attempt_count THEN RAISE EXCEPTION 'automatic task retry requires administrator'; END IF;
 ELSIF OLD.state='pending' AND NEW.state IN('completed','skipped','failed') THEN
  IF a.action<>'report_archive.task.finish' OR a.actor_type<>'system' OR a.actor_id IS NOT NULL OR NEW.attempt_count<>OLD.attempt_count+1 THEN RAISE EXCEPTION 'automatic task finish requires system attempt'; END IF;
 ELSE RAISE EXCEPTION 'invalid automatic task transition'; END IF;
 IF NEW.state IN('completed','skipped') THEN
  SELECT * INTO target FROM report_archives WHERE id=NEW.archive_id AND brand_id=NEW.brand_id AND kind=NEW.kind AND period_key=NEW.period_key;
  IF target.id IS NULL OR target.payload_sha256 IS DISTINCT FROM encode(sha256(convert_to(target.payload::text,'UTF8')),'hex') OR (NEW.state='completed' AND (target.automatic_task_id IS DISTINCT FROM NEW.id OR target.automatic_policy_version IS DISTINCT FROM NEW.policy_version)) OR (NEW.state='skipped' AND target.automatic_task_id IS NOT DISTINCT FROM NEW.id) THEN RAISE EXCEPTION 'automatic task outcome needs genuine retained archive'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_report_archive_automatic_task BEFORE INSERT OR UPDATE OR DELETE ON report_archive_automatic_tasks FOR EACH ROW EXECUTE FUNCTION guard_report_archive_automatic_task();

CREATE FUNCTION report_archive_automatic_valid(r report_archives) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(SELECT 1 FROM report_archive_automatic_tasks j JOIN report_archive_policy_revisions p ON p.brand_id=j.brand_id AND p.version=j.policy_version
 JOIN brand_report_archive_policies c ON c.brand_id=j.brand_id JOIN brands b ON b.id=j.brand_id JOIN audit_logs a ON a.id=r.audit_log_id
 WHERE j.id=r.automatic_task_id AND j.brand_id=r.brand_id AND j.state='pending' AND j.policy_version=r.automatic_policy_version AND j.kind=r.kind AND j.period_key=r.period_key AND j.timezone=r.timezone AND j.from_at=r.from_at AND j.to_at=r.to_at
 AND b.status<>'disabled' AND ((j.kind='daily' AND c.daily_enabled AND p.daily_enabled) OR (j.kind='monthly' AND c.monthly_enabled AND p.monthly_enabled))
 AND r.created_by IS NULL AND r.revision=1 AND r.previous_id IS NULL AND r.snapshot_at=statement_timestamp() AND r.created_at=r.snapshot_at AND r.to_at<=r.snapshot_at
 AND NOT EXISTS(SELECT 1 FROM report_archives old WHERE old.brand_id=r.brand_id AND old.kind=r.kind AND old.period_key=r.period_key)
 AND a.brand_id=r.brand_id AND a.actor_type='system' AND a.actor_id IS NULL AND a.action='report_archive.create.automatic' AND a.resource_type='report_archive' AND a.resource_id=r.id AND a.reason=r.reason AND a.request_id=r.request_id AND a.before_json='null'::jsonb
 AND a.after_json=jsonb_build_object('id',r.id,'task_id',j.id,'policy_version',j.policy_version)
 AND r.payload=report_archive_capture(r.brand_id,r.from_at,r.to_at,r.timezone) AND r.payload_sha256=encode(sha256(convert_to(r.payload::text,'UTF8')),'hex'))
$$;
-- Preserve the original guard OID and immutable/manual checks. Add system
-- provenance, schema-scoped locks, and corrected skipped-date ends.
DO $$ DECLARE definition text; anchor text:=' IF TG_OP<>''INSERT'' THEN RAISE EXCEPTION ''report archives are immutable''; END IF;'; calendar_anchor text:=' first_at:=report_archive_period_boundary(boundary,NEW.timezone); end_at:=report_archive_period_boundary(next_boundary,NEW.timezone);'; lock_anchor text:='hashtextextended(''report-archive:''||NEW.brand_id::text'; BEGIN
 definition:=pg_get_functiondef('guard_report_archive_version()'::regprocedure);
 IF position(anchor IN definition)=0 THEN RAISE EXCEPTION 'archive manual guard shape changed'; END IF;
 definition:=replace(definition,anchor,anchor||E'\n IF NEW.automatic_task_id IS NOT NULL THEN\n  PERFORM pg_advisory_xact_lock(hashtextextended(''report-archive:''||NEW.brand_id::text||'':''||NEW.kind||'':''||NEW.period_key,0));\n  IF NOT report_archive_automatic_valid(NEW) THEN RAISE EXCEPTION ''automatic archive lacks task and snapshot evidence''; END IF;\n  RETURN NEW;\n END IF;');
 IF position(calendar_anchor IN definition)=0 THEN RAISE EXCEPTION 'archive manual calendar shape changed'; END IF;
 definition:=replace(definition,calendar_anchor,' first_at:=CASE WHEN NEW.kind=''monthly'' THEN report_archive_period_end_boundary(boundary,NEW.timezone) ELSE report_archive_period_boundary(boundary,NEW.timezone) END; end_at:=report_archive_period_end_boundary(next_boundary,NEW.timezone);');
 IF position(lock_anchor IN definition)=0 THEN RAISE EXCEPTION 'archive manual lock shape changed'; END IF;
 definition:=replace(definition,lock_anchor,'hashtextextended(TG_TABLE_SCHEMA||'':report-archive:''||NEW.brand_id::text');
 EXECUTE definition;
END $$;
CREATE FUNCTION require_report_archive_automatic_link() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN IF NEW.automatic_task_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM report_archive_automatic_tasks j WHERE j.id=NEW.automatic_task_id AND j.brand_id=NEW.brand_id AND j.state='completed' AND j.archive_id=NEW.id) THEN RAISE EXCEPTION 'automatic archive needs atomic completed task'; END IF; RETURN NULL; END $$;
CREATE CONSTRAINT TRIGGER required_report_archive_automatic_link AFTER INSERT ON report_archives DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_report_archive_automatic_link();

INSERT INTO permissions(key) VALUES('report_archive_policy.write.brand'),('report_archive_task.retry.brand') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key) SELECT r.id,p.key FROM roles r CROSS JOIN permissions p WHERE r.is_bootstrap AND r.brand_id IS NOT NULL AND p.key IN('report_archive_policy.write.brand','report_archive_task.retry.brand') ON CONFLICT DO NOTHING;
DO $$ DECLARE s text:=current_schema(); f text; BEGIN
 FOREACH f IN ARRAY ARRAY['guard_report_archive_policy()','capture_report_archive_policy()','guard_report_archive_policy_revision()','initialize_report_archive_policy()','report_archive_period_end_boundary(timestamp,text)','report_archive_cursor_due(text,text,text)','guard_report_archive_cursor()','guard_report_archive_automatic_task()','report_archive_automatic_valid(report_archives)','require_report_archive_automatic_link()'] LOOP EXECUTE format('ALTER FUNCTION %s SET search_path TO pg_catalog, %I, pg_temp',f,s); END LOOP;
END $$;
