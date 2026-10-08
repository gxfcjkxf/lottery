-- Append-only report observations. These records do not close a wallet,
-- authorize payments, rewrite old reports, or delete retained financial data.
CREATE TABLE report_archives (
 id uuid PRIMARY KEY,
 brand_id uuid NOT NULL REFERENCES brands(id),
 kind text NOT NULL CHECK(kind IN('daily','monthly')),
 period_key text NOT NULL,
 timezone text NOT NULL,
 from_at timestamptz NOT NULL,
 to_at timestamptz NOT NULL CHECK(to_at>from_at),
 revision bigint NOT NULL CHECK(revision BETWEEN 1 AND 9007199254740991),
 previous_id uuid,
 snapshot_at timestamptz NOT NULL,
 created_by uuid NOT NULL REFERENCES admin_accounts(id),
 reason text NOT NULL CHECK(octet_length(reason) BETWEEN 1 AND 500 AND btrim(reason)=reason AND reason !~ E'[\\r\\n]'),
 request_id text NOT NULL CHECK(length(request_id) BETWEEN 1 AND 80),
 payload jsonb NOT NULL CHECK(jsonb_typeof(payload)='object'),
 payload_sha256 text NOT NULL CHECK(payload_sha256 ~ '^[0-9a-f]{64}$'),
 audit_log_id uuid NOT NULL REFERENCES audit_logs(id),
 created_at timestamptz NOT NULL,
 UNIQUE(brand_id,id),
 UNIQUE(brand_id,kind,period_key,revision),
 FOREIGN KEY(brand_id,previous_id) REFERENCES report_archives(brand_id,id),
 CHECK((revision=1)=(previous_id IS NULL)),
 CHECK(created_at=snapshot_at AND snapshot_at>=to_at)
);
CREATE INDEX report_archive_history ON report_archives(brand_id,created_at DESC,id DESC);

CREATE FUNCTION report_archive_period_boundary(civil timestamp,z text) RETURNS timestamptz LANGUAGE sql STABLE AS $$
 WITH offsets AS (
  SELECT DISTINCT (probe AT TIME ZONE z)-(probe AT TIME ZONE 'UTC') AS offset_value
  FROM generate_series((civil AT TIME ZONE 'UTC')-interval '48 hours',(civil AT TIME ZONE 'UTC')+interval '48 hours',interval '30 minutes') probe
 ), candidates AS (
  SELECT DISTINCT (civil-offset_value) AT TIME ZONE 'UTC' AS candidate FROM offsets
  WHERE (((civil-offset_value) AT TIME ZONE 'UTC') AT TIME ZONE z)::date=civil::date
 ) SELECT min(candidate) FROM candidates
$$;

-- A STABLE read-only guard shares the INSERT statement snapshot with capture;
-- concurrent financial commits cannot make the independent recomputation
-- observe a different wallet halfway through one archive observation.
CREATE FUNCTION guard_report_archive_version() RETURNS trigger LANGUAGE plpgsql STABLE AS $$
DECLARE prior report_archives; witness audit_logs; expected jsonb; original_zone text; boundary timestamp; next_boundary timestamp; first_at timestamptz; end_at timestamptz;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'report archives are immutable'; END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended('report-archive:'||NEW.brand_id::text||':'||NEW.kind||':'||NEW.period_key,0));
 SELECT * INTO prior FROM report_archives WHERE brand_id=NEW.brand_id AND kind=NEW.kind AND period_key=NEW.period_key ORDER BY revision DESC LIMIT 1;
 IF prior.id IS NOT NULL THEN
  IF prior.payload_sha256 IS DISTINCT FROM encode(sha256(convert_to(prior.payload::text,'UTF8')),'hex') THEN RAISE EXCEPTION 'archive predecessor integrity cannot be proven'; END IF;
  IF NEW.previous_id IS DISTINCT FROM prior.id OR NEW.revision<>prior.revision+1 OR NEW.timezone IS DISTINCT FROM prior.timezone OR NEW.from_at IS DISTINCT FROM prior.from_at OR NEW.to_at IS DISTINCT FROM prior.to_at THEN
   RAISE EXCEPTION 'archive revision must extend the original immutable calendar scope';
  END IF;
 ELSE
  SELECT timezone INTO original_zone FROM brands WHERE id=NEW.brand_id;
  IF NEW.previous_id IS NOT NULL OR NEW.revision<>1 OR NEW.timezone IS DISTINCT FROM original_zone THEN RAISE EXCEPTION 'first archive requires current brand timezone'; END IF;
 END IF;
 -- Revisions reuse the original absolute interval, even if a later timezone
 -- rules database would resolve the same civil key differently.
 IF prior.id IS NULL THEN
 IF NEW.kind='daily' AND NEW.period_key ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' THEN
  boundary:=make_timestamp(substring(NEW.period_key,1,4)::integer,substring(NEW.period_key,6,2)::integer,substring(NEW.period_key,9,2)::integer,0,0,0);
  next_boundary:=boundary+interval '1 day';
 ELSIF NEW.kind='monthly' AND NEW.period_key ~ '^[0-9]{4}-[0-9]{2}$' THEN
  boundary:=make_timestamp(substring(NEW.period_key,1,4)::integer,substring(NEW.period_key,6,2)::integer,1,0,0,0);
  next_boundary:=boundary+interval '1 month';
 ELSE RAISE EXCEPTION 'archive requires canonical daily or monthly period'; END IF;
 IF extract(year FROM boundary) NOT BETWEEN 1 AND 9998 OR NEW.timezone='Local' OR NOT EXISTS(SELECT 1 FROM pg_timezone_names WHERE name=NEW.timezone) THEN RAISE EXCEPTION 'archive calendar is invalid'; END IF;
 first_at:=report_archive_period_boundary(boundary,NEW.timezone); end_at:=report_archive_period_boundary(next_boundary,NEW.timezone);
 IF first_at IS NULL OR end_at IS NULL OR NEW.from_at IS DISTINCT FROM first_at OR NEW.to_at IS DISTINCT FROM end_at OR extract(year FROM first_at AT TIME ZONE 'UTC')<1 OR extract(year FROM end_at AT TIME ZONE 'UTC')>9999 THEN RAISE EXCEPTION 'archive boundaries do not match the saved calendar'; END IF;
 END IF;
 IF NEW.snapshot_at IS DISTINCT FROM statement_timestamp() OR NEW.created_at IS DISTINCT FROM NEW.snapshot_at OR NEW.to_at>NEW.snapshot_at THEN RAISE EXCEPTION 'archive requires an elapsed period and the actual capture timestamp'; END IF;
 SELECT * INTO witness FROM audit_logs WHERE id=NEW.audit_log_id;
 IF witness.id IS NULL OR witness.brand_id IS DISTINCT FROM NEW.brand_id OR witness.actor_type IS DISTINCT FROM 'admin' OR witness.actor_id IS DISTINCT FROM NEW.created_by OR
  witness.action IS DISTINCT FROM 'report_archive.create' OR witness.resource_type IS DISTINCT FROM 'report_archive' OR witness.resource_id IS DISTINCT FROM NEW.id OR
  witness.reason IS DISTINCT FROM NEW.reason OR witness.request_id IS DISTINCT FROM NEW.request_id OR
  witness.before_json IS DISTINCT FROM (CASE WHEN prior.id IS NULL THEN 'null'::jsonb ELSE jsonb_build_object('id',prior.id,'revision',prior.revision,'payload_sha256',prior.payload_sha256) END) OR
  (witness.after_json-ARRAY['from','to']) IS DISTINCT FROM jsonb_build_object('id',NEW.id,'brand_id',NEW.brand_id,'kind',NEW.kind,'period_key',NEW.period_key,'timezone',NEW.timezone,'revision',NEW.revision,'previous_id',NEW.previous_id) OR
  (witness.after_json->>'from')::timestamptz IS DISTINCT FROM NEW.from_at OR (witness.after_json->>'to')::timestamptz IS DISTINCT FROM NEW.to_at THEN
  RAISE EXCEPTION 'archive lacks matching creation audit';
 END IF;
 expected:=report_archive_capture(NEW.brand_id,NEW.from_at,NEW.to_at,NEW.timezone);
 IF expected IS NULL OR NEW.payload IS DISTINCT FROM expected OR NEW.payload_sha256 IS DISTINCT FROM encode(sha256(convert_to(expected::text,'UTF8')),'hex') THEN
  RAISE EXCEPTION 'archive requires exact source snapshot and canonical SHA-256';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_report_archive_version BEFORE INSERT OR UPDATE OR DELETE ON report_archives FOR EACH ROW EXECUTE FUNCTION guard_report_archive_version();
CREATE TRIGGER guarded_report_archive_truncate BEFORE TRUNCATE ON report_archives FOR EACH STATEMENT EXECUTE FUNCTION guard_report_archive_version();

DO $$ DECLARE s text:=current_schema(); BEGIN
 EXECUTE format('ALTER FUNCTION guard_report_archive_version() SET search_path TO pg_catalog, %I, pg_temp',s);
 EXECUTE format('ALTER FUNCTION report_archive_period_boundary(timestamp,text) SET search_path TO pg_catalog, %I, pg_temp',s);
END $$;
