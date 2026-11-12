-- Read-only dependency inventory. Run against the current migrated schema.
-- FK dependencies are only one part of the review: SQL/Go references and
-- immutable evidence checks must also be reviewed before any deletion.
BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;
WITH tables AS (
 SELECT c.oid,c.relname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
 WHERE n.nspname=current_schema() AND c.relkind IN ('r','p')
), inventory AS (
 SELECT t.relname AS name,
  coalesce((SELECT jsonb_agg(a.attname ORDER BY a.attnum) FROM pg_attribute a
   WHERE a.attrelid=t.oid AND a.attnum>0 AND NOT a.attisdropped),'[]'::jsonb) AS columns,
  coalesce((SELECT jsonb_agg(jsonb_build_object('table',c.conrelid::regclass::text,
   'constraint',c.conname,'definition',pg_get_constraintdef(c.oid)) ORDER BY c.conname)
   FROM pg_constraint c WHERE c.contype='f' AND c.confrelid=t.oid),'[]'::jsonb) AS referenced_by,
  coalesce((SELECT jsonb_agg(jsonb_build_object('table',c.confrelid::regclass::text,
   'constraint',c.conname,'definition',pg_get_constraintdef(c.oid)) ORDER BY c.conname)
   FROM pg_constraint c WHERE c.contype='f' AND c.conrelid=t.oid),'[]'::jsonb) AS references,
  coalesce((SELECT jsonb_agg(jsonb_build_object('name',g.tgname,'definition',pg_get_triggerdef(g.oid),
   'function',g.tgfoid::regprocedure::text) ORDER BY g.tgname)
   FROM pg_trigger g WHERE g.tgrelid=t.oid AND NOT g.tgisinternal),'[]'::jsonb) AS triggers
 FROM tables t
)
SELECT jsonb_build_object('tables',(SELECT jsonb_agg(to_jsonb(i) ORDER BY name) FROM inventory i),
 'views',(SELECT jsonb_agg(jsonb_build_object('name',viewname,'definition',definition) ORDER BY viewname)
  FROM pg_views WHERE schemaname=current_schema()))::text;
COMMIT;
