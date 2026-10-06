-- pg_dump/pg_restore intentionally use an empty session search_path. Pin each
-- application function to its own schema so nested validators and trigger table
-- lookups remain valid on restore and cannot resolve through caller temp tables.
DO $$
DECLARE app_schema text:=current_schema(); f record;
BEGIN
 IF app_schema IS NULL THEN RAISE EXCEPTION 'application schema required'; END IF;
 FOR f IN
  SELECT p.proname,pg_get_function_identity_arguments(p.oid) AS args
  FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
  WHERE n.nspname=app_schema AND p.prokind='f'
  ORDER BY p.proname,p.oid
 LOOP
  EXECUTE format('ALTER FUNCTION %I.%I(%s) SET search_path = pg_catalog, %I, pg_temp',app_schema,f.proname,f.args,app_schema);
 END LOOP;
END $$;
