CREATE FUNCTION valid_bet_withdrawal_snapshot(v jsonb) RETURNS boolean
LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE
 key_count integer;
 field text;
 version_text text;
 id_text text;
BEGIN
 IF jsonb_typeof(v)<>'object' THEN RETURN false; END IF;
 SELECT count(*) INTO key_count FROM jsonb_object_keys(v);
 IF key_count<>7 OR NOT v ?& ARRAY[
  'schema_version','multiple','source','brand_version','game_version',
  'brand_revision_id','game_revision_id'
 ] THEN RETURN false; END IF;
 IF jsonb_typeof(v->'schema_version')<>'number' OR v->'schema_version'<>'1'::jsonb OR
    NOT valid_positive_withdrawal_multiple(v->'multiple') OR
    jsonb_typeof(v->'source')<>'string' OR v->>'source' NOT IN ('brand','game') THEN
  RETURN false;
 END IF;
 FOREACH field IN ARRAY ARRAY['brand_version','game_version'] LOOP
  IF jsonb_typeof(v->field)<>'string' THEN RETURN false; END IF;
  version_text:=v->>field;
  IF version_text !~ '^[1-9][0-9]*$' OR version_text::numeric>9223372036854775807 THEN
   RETURN false;
  END IF;
 END LOOP;
 FOREACH field IN ARRAY ARRAY['brand_revision_id','game_revision_id'] LOOP
  IF jsonb_typeof(v->field)<>'string' THEN RETURN false; END IF;
  id_text:=v->>field;
  IF id_text !~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' OR
     id_text::uuid::text<>id_text THEN
   RETURN false;
  END IF;
 END LOOP;
 RETURN true;
EXCEPTION WHEN OTHERS THEN RETURN false;
END $$;

ALTER TABLE bet_orders ADD COLUMN withdrawal_rule_snapshot jsonb;
ALTER TABLE bet_orders ADD CONSTRAINT bet_order_withdrawal_snapshot_valid
 CHECK(withdrawal_rule_snapshot IS NULL OR valid_bet_withdrawal_snapshot(withdrawal_rule_snapshot));

CREATE FUNCTION capture_bet_withdrawal_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
 brand_policy brand_withdrawal_policies;
 game_policy game_withdrawal_policies;
 effective_config jsonb;
 brand_revision uuid;
 game_revision uuid;
BEGIN
 SELECT * INTO brand_policy FROM brand_withdrawal_policies
  WHERE brand_id=NEW.brand_id FOR SHARE;
 IF NOT FOUND THEN RAISE EXCEPTION 'withdrawal snapshot policy unavailable'; END IF;
 SELECT * INTO game_policy FROM game_withdrawal_policies
  WHERE brand_id=NEW.brand_id AND game_id=NEW.game_id FOR SHARE;
 IF NOT FOUND THEN RAISE EXCEPTION 'withdrawal snapshot game policy unavailable'; END IF;

 IF game_policy.config->'turnover_multiple'<>'null'::jsonb THEN
  effective_config:=game_policy.config;
 ELSE
  effective_config:=brand_policy.config;
 END IF;
 IF NOT valid_positive_withdrawal_multiple(effective_config->'turnover_multiple') THEN
  RAISE EXCEPTION 'invalid effective withdrawal multiple';
 END IF;
 SELECT id INTO brand_revision FROM withdrawal_policy_revisions
  WHERE brand_id=NEW.brand_id AND game_id IS NULL AND version=brand_policy.version
    AND config=brand_policy.config;
 IF NOT FOUND THEN RAISE EXCEPTION 'matching brand withdrawal revision unavailable'; END IF;
 SELECT id INTO game_revision FROM withdrawal_policy_revisions
  WHERE brand_id=NEW.brand_id AND game_id=NEW.game_id AND version=game_policy.version
    AND config=game_policy.config;
 IF NOT FOUND THEN RAISE EXCEPTION 'matching game withdrawal revision unavailable'; END IF;

 IF game_policy.config->'turnover_multiple'<>'null'::jsonb THEN
  NEW.withdrawal_rule_snapshot:=jsonb_build_object(
   'schema_version',1,
   'multiple',game_policy.config->>'turnover_multiple',
   'source','game',
   'brand_version',brand_policy.version::text,
   'game_version',game_policy.version::text,
   'brand_revision_id',brand_revision::text,
   'game_revision_id',game_revision::text
  );
 ELSE
  NEW.withdrawal_rule_snapshot:=jsonb_build_object(
   'schema_version',1,
   'multiple',brand_policy.config->>'turnover_multiple',
   'source','brand',
   'brand_version',brand_policy.version::text,
   'game_version',game_policy.version::text,
   'brand_revision_id',brand_revision::text,
   'game_revision_id',game_revision::text
  );
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER capture_bet_withdrawal_snapshot
 BEFORE INSERT ON bet_orders FOR EACH ROW EXECUTE FUNCTION capture_bet_withdrawal_snapshot();

CREATE FUNCTION guard_bet_withdrawal_snapshot_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.withdrawal_rule_snapshot IS DISTINCT FROM OLD.withdrawal_rule_snapshot THEN
  RAISE EXCEPTION 'bet withdrawal snapshot is immutable';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER immutable_bet_withdrawal_snapshot
 BEFORE UPDATE ON bet_orders FOR EACH ROW EXECUTE FUNCTION guard_bet_withdrawal_snapshot_update();

-- Keep policy lookups schema-bound during restores and prevent temporary
-- objects from shadowing the policy tables or snapshot validator.
DO $$
DECLARE app_schema text:=current_schema();
BEGIN
 IF app_schema IS NULL THEN RAISE EXCEPTION 'application schema required'; END IF;
 EXECUTE format('ALTER FUNCTION %I.valid_bet_withdrawal_snapshot(jsonb) SET search_path = pg_catalog, %I, pg_temp',app_schema,app_schema);
 EXECUTE format('ALTER FUNCTION %I.capture_bet_withdrawal_snapshot() SET search_path = pg_catalog, %I, pg_temp',app_schema,app_schema);
 EXECUTE format('ALTER FUNCTION %I.guard_bet_withdrawal_snapshot_update() SET search_path = pg_catalog, %I, pg_temp',app_schema,app_schema);
END $$;
