-- Real business admission rejections, separate from explicit administrative
-- simulations. Historical policy is retained even if changed before recording.
CREATE TABLE compliance_gate_rejections (
 id uuid PRIMARY KEY,brand_id uuid NOT NULL REFERENCES brands(id),policy_version bigint NOT NULL,
 config jsonb NOT NULL CHECK(valid_compliance_config(config) IS TRUE),
 operation text NOT NULL CHECK(operation IN('registration','betting')),
 action text NOT NULL CHECK(action IN('register','join','operator_join','bet_preview','bet_place')),
 decision text NOT NULL CHECK(decision='review'),checks jsonb NOT NULL CHECK(checks=compliance_stub_checks(config)),
 adapter_mode text NOT NULL CHECK(adapter_mode='stub'),actor_type text NOT NULL CHECK(actor_type IN('anonymous','user','admin')),
 actor_id uuid,member_id uuid,request_id text NOT NULL CHECK(length(request_id) BETWEEN 1 AND 80),
 audit_log_id uuid NOT NULL UNIQUE REFERENCES audit_logs(id),created_at timestamptz NOT NULL,
 FOREIGN KEY(brand_id,policy_version) REFERENCES compliance_policy_revisions(brand_id,version),
 FOREIGN KEY(brand_id,member_id) REFERENCES brand_members(brand_id,id),
 CHECK((operation='registration' AND action IN('register','join','operator_join')) OR (operation='betting' AND action IN('bet_preview','bet_place'))),
 CHECK((actor_type='anonymous' AND action='register' AND actor_id IS NULL AND member_id IS NULL)
 OR (actor_type='admin' AND action='operator_join' AND actor_id IS NOT NULL AND member_id IS NULL)
 OR (actor_type='user' AND actor_id IS NOT NULL AND ((action='join') OR (action IN('bet_preview','bet_place') AND member_id IS NOT NULL)))),
 CHECK(config->'age_enabled'='true'::jsonb OR config->'region_enabled'='true'::jsonb OR config->'identity_enabled'='true'::jsonb)
);
CREATE INDEX compliance_gate_rejections_page ON compliance_gate_rejections(brand_id,created_at DESC,id DESC);
CREATE INDEX compliance_gate_rejections_operation_page ON compliance_gate_rejections(brand_id,operation,created_at DESC,id DESC);
CREATE FUNCTION guard_compliance_gate_rejection() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p compliance_policy_revisions;a audit_logs;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'compliance admission rejection immutable'; END IF;
 SELECT * INTO p FROM compliance_policy_revisions WHERE brand_id=NEW.brand_id AND version=NEW.policy_version;
 SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
 IF NOT FOUND OR p.brand_id IS NULL OR p.config IS DISTINCT FROM NEW.config
 OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type IS DISTINCT FROM (CASE WHEN NEW.actor_type='anonymous' THEN 'system' ELSE NEW.actor_type END)
 OR a.actor_id IS DISTINCT FROM NEW.actor_id OR a.action IS DISTINCT FROM 'compliance.gate.reject'
 OR a.resource_type IS DISTINCT FROM 'compliance_gate' OR a.resource_id IS DISTINCT FROM NEW.id
 OR a.reason IS DISTINCT FROM 'COMPLIANCE_REVIEW_REQUIRED' OR a.request_id IS DISTINCT FROM NEW.request_id
 OR a.after_json->>'id' IS DISTINCT FROM NEW.id::text OR a.after_json->>'brand_id' IS DISTINCT FROM NEW.brand_id::text
 OR (a.after_json->>'policy_version')::bigint IS DISTINCT FROM NEW.policy_version OR a.after_json->'config' IS DISTINCT FROM NEW.config
 OR a.after_json->>'operation' IS DISTINCT FROM NEW.operation OR a.after_json->>'action' IS DISTINCT FROM NEW.action
 OR a.after_json->>'decision' IS DISTINCT FROM NEW.decision OR a.after_json->'checks' IS DISTINCT FROM NEW.checks
 OR a.after_json->>'adapter_mode' IS DISTINCT FROM NEW.adapter_mode OR a.after_json->>'actor_type' IS DISTINCT FROM NEW.actor_type
 OR a.after_json->'actor_id' IS DISTINCT FROM coalesce(to_jsonb(NEW.actor_id::text),'null'::jsonb)
 OR a.after_json->'member_id' IS DISTINCT FROM coalesce(to_jsonb(NEW.member_id::text),'null'::jsonb)
 OR (a.after_json->>'created_at')::timestamptz IS DISTINCT FROM NEW.created_at THEN RAISE EXCEPTION 'matching compliance gate audit required'; END IF;
 IF NEW.actor_type='admin' AND NOT EXISTS(SELECT 1 FROM admin_accounts WHERE id=NEW.actor_id)
 OR NEW.actor_type='user' AND NOT EXISTS(SELECT 1 FROM global_users WHERE id=NEW.actor_id)
 OR NEW.member_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM brand_members WHERE brand_id=NEW.brand_id AND id=NEW.member_id AND global_user_id=NEW.actor_id)
 THEN RAISE EXCEPTION 'compliance rejection actor/brand mismatch'; END IF;
 RETURN NEW;
END $$;
DO $$ BEGIN EXECUTE format('ALTER FUNCTION %I.guard_compliance_gate_rejection() SET search_path=pg_catalog,%I,pg_temp',current_schema(),current_schema()); END $$;
CREATE TRIGGER guarded_compliance_gate_rejection BEFORE INSERT OR UPDATE OR DELETE ON compliance_gate_rejections FOR EACH ROW EXECUTE FUNCTION guard_compliance_gate_rejection();
