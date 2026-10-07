-- A descendant's effective commission mode must match its parent's. Roots may
-- override the brand default, and historical revisions remain untouched.
CREATE FUNCTION agent_effective_mode_for_path(
 p_brand uuid,p_path uuid[],p_override_id uuid,p_override_config jsonb,p_policy_mode text
) RETURNS text LANGUAGE sql STABLE AS $$
 SELECT coalesce((
  SELECT CASE WHEN a.id=p_override_id THEN p_override_config->>'mode' ELSE a.config->>'mode' END
  FROM unnest(p_path) WITH ORDINALITY AS part(id,depth)
  JOIN agent_nodes a ON a.brand_id=p_brand AND a.id=part.id
  WHERE CASE WHEN a.id=p_override_id THEN p_override_config->'mode' ELSE a.config->'mode' END<>'null'::jsonb
  ORDER BY part.depth DESC LIMIT 1
 ),p_policy_mode)
$$;

CREATE FUNCTION guard_agent_uniform_node_mode() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE policy_mode text; parent_mode text; parent_path uuid[]; node_mode text;
BEGIN
 SELECT config->>'mode' INTO policy_mode FROM brand_agent_policies
  WHERE brand_id=NEW.brand_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'agent policy missing'; END IF;

 IF NEW.parent_id IS NOT NULL THEN
  SELECT parent.path,agent_effective_mode_for_path(NEW.brand_id,parent.path,NULL,NULL,policy_mode)
   INTO parent_path,parent_mode FROM agent_nodes parent
   WHERE parent.brand_id=NEW.brand_id AND parent.id=NEW.parent_id;
  IF NOT FOUND THEN RAISE EXCEPTION 'agent parent missing'; END IF;
  IF TG_OP='INSERT' AND EXISTS(
   SELECT 1 FROM agent_nodes child
   JOIN agent_nodes ancestor ON ancestor.brand_id=child.brand_id AND ancestor.id=child.parent_id
   WHERE child.brand_id=NEW.brand_id AND child.id=ANY(parent_path)
    AND agent_effective_mode_for_path(child.brand_id,child.path,NULL,NULL,policy_mode)
       IS DISTINCT FROM agent_effective_mode_for_path(ancestor.brand_id,ancestor.path,NULL,NULL,policy_mode)
  ) THEN
   RAISE EXCEPTION 'agent parent has a mixed effective mode ancestor chain' USING ERRCODE='23514',CONSTRAINT='agent_ancestor_mode_chain_consistent';
  END IF;
  node_mode:=coalesce(NULLIF(NEW.config->>'mode',''),parent_mode);
  IF node_mode IS DISTINCT FROM parent_mode THEN
   RAISE EXCEPTION 'child effective mode must match parent' USING ERRCODE='23514',CONSTRAINT='agent_child_mode_matches_parent';
  END IF;
 ELSE
  node_mode:=coalesce(NULLIF(NEW.config->>'mode',''),policy_mode);
 END IF;

 IF TG_OP='UPDATE' THEN
  -- Evaluate the proposed config over this node's complete existing subtree.
  -- This fails closed on legacy mixed-mode paths rather than repairing them.
  IF EXISTS(
   SELECT 1 FROM agent_nodes child
   WHERE child.brand_id=NEW.brand_id
    AND (child.path @> ARRAY[NEW.id]::uuid[] OR child.id=ANY(NEW.path))
    AND child.parent_id IS NOT NULL
    AND agent_effective_mode_for_path(NEW.brand_id,child.path,NEW.id,NEW.config,policy_mode)
       IS DISTINCT FROM agent_effective_mode_for_path(
        NEW.brand_id,(SELECT parent.path FROM agent_nodes parent WHERE parent.brand_id=child.brand_id AND parent.id=child.parent_id),
        NEW.id,NEW.config,policy_mode)
  ) THEN
   RAISE EXCEPTION 'agent update would mismatch descendant mode' USING ERRCODE='23514',CONSTRAINT='agent_descendant_mode_matches_parent';
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_agent_uniform_node_mode
 BEFORE INSERT OR UPDATE ON agent_nodes
 FOR EACH ROW EXECUTE FUNCTION guard_agent_uniform_node_mode();

CREATE FUNCTION guard_agent_uniform_policy_mode() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.config->>'mode' IS DISTINCT FROM OLD.config->>'mode' AND EXISTS(
  SELECT 1 FROM agent_nodes child
  JOIN agent_nodes parent ON parent.brand_id=child.brand_id AND parent.id=child.parent_id
  WHERE child.brand_id=NEW.brand_id
   AND agent_effective_mode_for_path(child.brand_id,child.path,NULL,NULL,NEW.config->>'mode')
      IS DISTINCT FROM agent_effective_mode_for_path(parent.brand_id,parent.path,NULL,NULL,NEW.config->>'mode')
 ) THEN
  RAISE EXCEPTION 'brand mode update would mismatch agent hierarchy' USING ERRCODE='23514',CONSTRAINT='agent_brand_mode_preserves_hierarchy';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_agent_uniform_policy_mode
 BEFORE UPDATE OF config ON brand_agent_policies
 FOR EACH ROW EXECUTE FUNCTION guard_agent_uniform_policy_mode();

-- New functions are pinned the same way as migration 0032, including in
-- tenant schemas restored with an empty caller search_path.
DO $$
DECLARE app_schema text:=current_schema(); f record;
BEGIN
 IF app_schema IS NULL THEN RAISE EXCEPTION 'application schema required'; END IF;
 FOR f IN
  SELECT p.proname,pg_get_function_identity_arguments(p.oid) AS args
  FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
  WHERE n.nspname=app_schema AND p.proname IN
   ('agent_effective_mode_for_path','guard_agent_uniform_node_mode','guard_agent_uniform_policy_mode')
 LOOP
  EXECUTE format('ALTER FUNCTION %I.%I(%s) SET search_path = pg_catalog, %I, pg_temp',app_schema,f.proname,f.args,app_schema);
 END LOOP;
END $$;
