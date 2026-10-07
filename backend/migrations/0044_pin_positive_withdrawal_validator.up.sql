-- 0042 is already applied: repair function lookup settings without changing
-- its checksum, historical policy data, or the meaning of the positive check.
DO $$
DECLARE app_schema text:=current_schema();
BEGIN
 IF app_schema IS NULL THEN RAISE EXCEPTION 'application schema required'; END IF;
 EXECUTE format('ALTER FUNCTION %I.valid_positive_withdrawal_multiple(jsonb) SET search_path=pg_catalog,%I,pg_temp',app_schema,app_schema);
END $$;
