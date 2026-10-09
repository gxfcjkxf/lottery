-- Future published results only. Never manufacture audiences for old draws.
CREATE TABLE draw_notification_publications (
 draw_result_id uuid PRIMARY KEY REFERENCES draw_results(id),
 brand_id uuid NOT NULL REFERENCES brands(id), game_id uuid NOT NULL REFERENCES games(id),
 period_id uuid NOT NULL REFERENCES periods(id),
 event_type text NOT NULL CHECK(event_type IN('draw.result.published','draw.result.corrected')),
 period_no text NOT NULL, result jsonb NOT NULL, drawn_at timestamptz NOT NULL,
 previous_draw_id uuid REFERENCES draw_results(id), audit_log_id uuid NOT NULL REFERENCES audit_logs(id),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 creation_xid xid8 NOT NULL DEFAULT pg_current_xact_id(),
 UNIQUE(brand_id,draw_result_id),
 CHECK((event_type='draw.result.published')=(previous_draw_id IS NULL))
);
CREATE TABLE draw_notification_recipients (
 id uuid PRIMARY KEY, brand_id uuid NOT NULL, draw_result_id uuid NOT NULL,
 member_id uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 creation_xid xid8 NOT NULL DEFAULT pg_current_xact_id(),
 UNIQUE(brand_id,draw_result_id,member_id),
 FOREIGN KEY(brand_id,draw_result_id) REFERENCES draw_notification_publications(brand_id,draw_result_id),
 FOREIGN KEY(brand_id,member_id) REFERENCES brand_members(brand_id,id)
);

-- Rows visible to this transaction whose inserting/updating XID is still in
-- progress must be our own uncommitted rows. xmin can be a SAVEPOINT subxid;
-- pg_current_xact_id() is the top-level xid8. Lift the row XID into the current
-- epoch (or the next one if a subtransaction crossed the 32-bit wrap boundary).
-- Old committed rows and future/out-of-range IDs fail closed.
CREATE FUNCTION draw_notice_current_row(row_xid xid) RETURNS boolean LANGUAGE plpgsql VOLATILE AS $$
DECLARE top_xid numeric:=pg_current_xact_id()::text::numeric; full_xid numeric;
BEGIN
 full_xid:=top_xid-mod(top_xid,4294967296)+row_xid::text::numeric;
 IF full_xid<top_xid THEN full_xid:=full_xid+4294967296; END IF;
 RETURN pg_xact_status(full_xid::text::xid8)='in progress';
EXCEPTION WHEN OTHERS THEN RETURN false;
END $$;
CREATE FUNCTION guard_draw_notification_publication() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE valid boolean;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'published draw notification evidence immutable'; END IF;
 IF NEW.creation_xid IS DISTINCT FROM pg_current_xact_id() THEN RAISE EXCEPTION 'current publication transaction required'; END IF;
 SELECT EXISTS(
  SELECT 1 FROM draw_results d JOIN periods p ON p.brand_id=d.brand_id AND p.game_id=d.game_id AND p.id=d.period_id
  JOIN audit_logs a ON a.brand_id=d.brand_id AND a.id=NEW.audit_log_id
  WHERE d.id=NEW.draw_result_id AND d.brand_id=NEW.brand_id AND d.game_id=NEW.game_id AND d.period_id=NEW.period_id
   AND p.draw_result_id=d.id AND draw_notice_current_row(p.xmin) AND draw_notice_current_row(a.xmin)
   AND NEW.period_no=p.period_no AND NEW.result=d.result AND NEW.drawn_at=d.drawn_at
   AND NEW.previous_draw_id IS NOT DISTINCT FROM d.corrected_from_id
   AND NEW.event_type=CASE WHEN d.corrected_from_id IS NULL THEN 'draw.result.published' ELSE 'draw.result.corrected' END
   AND (
    (a.action='draw.lock' AND a.resource_type='draw_result' AND a.resource_id=d.id
     AND a.after_json->>'id'=d.id::text AND a.after_json->>'period_id'=p.id::text)
    OR
    (a.resource_type='draw_correction' AND a.after_json->>'draw_result_id'=d.id::text
     AND EXISTS(SELECT 1 FROM draw_corrections c WHERE c.brand_id=d.brand_id AND c.id=a.resource_id
      AND c.period_id=p.id AND c.draw_result_id=d.id AND c.previous_draw_result_id=d.corrected_from_id
      AND ((a.action='draw.correct' AND c.state='completed') OR (a.action='draw.correction.publish' AND c.state='resettling'))))
   )
 ) INTO valid;
 IF valid IS DISTINCT FROM true THEN RAISE EXCEPTION 'matching current published result and audit required'; END IF;
 NEW.created_at:=clock_timestamp(); RETURN NEW;
END $$;
CREATE TRIGGER guarded_draw_notification_publication BEFORE INSERT OR UPDATE OR DELETE ON draw_notification_publications
 FOR EACH ROW EXECUTE FUNCTION guard_draw_notification_publication();

CREATE FUNCTION guard_draw_notification_recipient() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'published draw audience immutable'; END IF;
 IF NEW.creation_xid IS DISTINCT FROM pg_current_xact_id() OR NOT EXISTS(
  SELECT 1 FROM draw_notification_publications p JOIN bet_orders o ON o.brand_id=p.brand_id AND o.period_id=p.period_id
  WHERE p.brand_id=NEW.brand_id AND p.draw_result_id=NEW.draw_result_id AND p.creation_xid=pg_current_xact_id()
   AND o.brand_member_id=NEW.member_id
 ) THEN RAISE EXCEPTION 'recipient must belong to this current publication betting audience'; END IF;
 NEW.created_at:=clock_timestamp(); RETURN NEW;
END $$;
CREATE TRIGGER guarded_draw_notification_recipient BEFORE INSERT OR UPDATE OR DELETE ON draw_notification_recipients
 FOR EACH ROW EXECUTE FUNCTION guard_draw_notification_recipient();

CREATE FUNCTION valid_draw_notification_event(b uuid,k text,aggregate uuid,payload jsonb) RETURNS boolean
LANGUAGE plpgsql STABLE AS $$
BEGIN
 IF b IS NULL OR aggregate IS NULL OR k NOT IN('draw.result.published','draw.result.corrected')
  OR jsonb_typeof(payload) IS DISTINCT FROM 'object' OR (SELECT count(*) FROM jsonb_object_keys(payload))<>4
  OR NOT(payload ?& ARRAY['member_id','resource_id','publication_id','recipient_id']) THEN RETURN false; END IF;
 RETURN EXISTS(SELECT 1 FROM draw_notification_recipients r
  JOIN draw_notification_publications p ON p.brand_id=r.brand_id AND p.draw_result_id=r.draw_result_id
  WHERE r.brand_id=b AND r.id=aggregate AND p.event_type=k
   AND payload=jsonb_build_object('member_id',r.member_id::text,'resource_id',p.draw_result_id::text,
    'publication_id',p.draw_result_id::text,'recipient_id',r.id::text));
EXCEPTION WHEN OTHERS THEN RETURN false;
END $$;
CREATE UNIQUE INDEX draw_notification_event_once ON outbox_events(brand_id,event_type,aggregate_id)
 WHERE event_type IN('draw.result.published','draw.result.corrected');
CREATE FUNCTION guard_draw_notification_outbox() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN
  IF OLD.event_type IN('draw.result.published','draw.result.corrected') THEN RAISE EXCEPTION 'draw notification event immutable'; END IF;
  RETURN OLD;
 END IF;
 IF TG_OP='UPDATE' THEN
  IF OLD.event_type IN('draw.result.published','draw.result.corrected') OR NEW.event_type IN('draw.result.published','draw.result.corrected') THEN
   IF (to_jsonb(NEW)-'published_at') IS DISTINCT FROM (to_jsonb(OLD)-'published_at') THEN RAISE EXCEPTION 'draw notification event immutable except publication acknowledgement'; END IF;
   IF NOT valid_draw_notification_event(NEW.brand_id,NEW.event_type,NEW.aggregate_id,NEW.payload) THEN RAISE EXCEPTION 'draw notification evidence required'; END IF;
  END IF;
  RETURN NEW;
 END IF;
 IF NEW.event_type IN('draw.result.published','draw.result.corrected') AND
  (NOT valid_draw_notification_event(NEW.brand_id,NEW.event_type,NEW.aggregate_id,NEW.payload) OR NOT EXISTS(
   SELECT 1 FROM draw_notification_recipients r WHERE r.brand_id=NEW.brand_id AND r.id=NEW.aggregate_id AND r.creation_xid=pg_current_xact_id()
  )) THEN RAISE EXCEPTION 'new draw event requires the current immutable audience'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_draw_notification_outbox BEFORE INSERT OR UPDATE OR DELETE ON outbox_events
 FOR EACH ROW EXECUTE FUNCTION guard_draw_notification_outbox();

CREATE FUNCTION require_draw_notification_publication_commit() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF EXISTS(SELECT DISTINCT o.brand_member_id FROM bet_orders o WHERE o.brand_id=NEW.brand_id AND o.period_id=NEW.period_id
  EXCEPT SELECT r.member_id FROM draw_notification_recipients r WHERE r.brand_id=NEW.brand_id AND r.draw_result_id=NEW.draw_result_id)
  OR EXISTS(SELECT 1 FROM draw_notification_recipients r WHERE r.brand_id=NEW.brand_id AND r.draw_result_id=NEW.draw_result_id
   AND NOT EXISTS(SELECT 1 FROM outbox_events e WHERE e.brand_id=r.brand_id AND e.aggregate_id=r.id AND e.event_type=NEW.event_type
    AND valid_draw_notification_event(e.brand_id,e.event_type,e.aggregate_id,e.payload))) THEN
  RAISE EXCEPTION 'publication requires the complete unique audience and every matching event';
 END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER draw_notification_publication_commit AFTER INSERT ON draw_notification_publications
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_draw_notification_publication_commit();

CREATE FUNCTION guard_draw_notification_content() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.event_type IN('draw.result.published','draw.result.corrected') AND NOT EXISTS(
  SELECT 1 FROM outbox_events e JOIN draw_notification_recipients r ON r.brand_id=e.brand_id AND r.id=e.aggregate_id
  JOIN draw_notification_publications p ON p.brand_id=r.brand_id AND p.draw_result_id=r.draw_result_id
  WHERE e.id=NEW.event_id AND e.brand_id=NEW.brand_id AND e.event_type=NEW.event_type AND r.member_id=NEW.member_id
   AND valid_draw_notification_event(e.brand_id,e.event_type,e.aggregate_id,e.payload)
   AND NEW.payload#>>'{draw,drawn_at}' ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}([.][0-9]{1,6})?Z$'
   AND (NEW.payload#>>'{draw,drawn_at}')::timestamptz=p.drawn_at
   AND (NEW.payload#-'{draw,drawn_at}')=jsonb_build_object('resource_id',p.draw_result_id::text,'points',NULL,'draw',jsonb_build_object(
    'game_id',p.game_id::text,'period_id',p.period_id::text,'period_no',p.period_no,'result',p.result,
    'previous_draw_id',p.previous_draw_id::text))
 ) THEN RAISE EXCEPTION 'draw inbox requires the exact historical public facts'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_draw_notification_content BEFORE INSERT ON notifications FOR EACH ROW EXECUTE FUNCTION guard_draw_notification_content();

DO $$ DECLARE f record; BEGIN
 FOR f IN SELECT p.oid::regprocedure signature FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
  WHERE n.nspname=current_schema() AND p.proname IN('draw_notice_current_row','guard_draw_notification_publication','guard_draw_notification_recipient',
   'valid_draw_notification_event','guard_draw_notification_outbox','require_draw_notification_publication_commit','guard_draw_notification_content') LOOP
  EXECUTE format('ALTER FUNCTION %s SET search_path TO pg_catalog, %I, pg_temp',f.signature,current_schema());
 END LOOP;
END $$;
