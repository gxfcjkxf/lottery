-- Versioned in-app copy only. No new events, payments or external providers.
CREATE FUNCTION notification_template_defaults() RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$
 SELECT '{"member.joined":{"en":{"title":"Welcome","body":"Your membership is ready. Welcome aboard."},"zh-CN":{"title":"欢迎","body":"您的会员账户已准备就绪，欢迎加入。"}},"recharge.confirmed":{"en":{"title":"Recharge confirmed","body":"Recharge confirmed: {points} points."},"zh-CN":{"title":"充值已确认","body":"充值已确认：{points} 积分。"}},"bet.order.placed":{"en":{"title":"Order submitted","body":"Order submitted: {points} points."},"zh-CN":{"title":"注单已提交","body":"注单已提交，涉及 {points} 积分。"}},"bet.order.cancelled":{"en":{"title":"Order cancelled","body":"Cancelled. {points} points were returned to your original balance."},"zh-CN":{"title":"注单已取消","body":"注单已取消，{points} 积分已原路退回。"}},"bet.order.judged_cancelled":{"en":{"title":"Order cancelled after review","body":"Cancelled after review. {points} points were returned to your original balance."},"zh-CN":{"title":"注单已判定取消","body":"注单经判定已取消，{points} 积分已原路退回。"}},"bet.order.abnormal":{"en":{"title":"Order needs review","body":"Your order needs manual review. Points involved: {points}."},"zh-CN":{"title":"注单待人工处理","body":"您的注单需要人工处理，涉及积分：{points}。"}},"bet.order.won":{"en":{"title":"Prize credit recorded","body":"Historical record: {points} points were credited as this order''s prize. This records the credit, not your current wallet balance or a guaranteed final outcome. Any correction will appear as a separate prize event; this record is retained."},"zh-CN":{"title":"派奖入账记录","body":"历史记录：此注单的 {points} 积分奖金已记入账本。此记录仅表示该笔入账，不代表当前钱包余额，也不保证最终结果。任何更正都会作为单独的奖金事件记录；此记录会保留。"}},"bet.order.prize_reversed":{"en":{"title":"Prize reversal recorded","body":"Historical record: the full original prize amount of {points} points for this order was reversed. This records the reversal, not your current wallet balance. Any later prize correction will appear as a separate event; this record is retained."},"zh-CN":{"title":"奖金冲正记录","body":"历史记录：此注单原奖金全额 {points} 积分已冲回。此记录仅表示该笔冲正，不代表当前钱包余额。之后如有奖金更正，会作为单独事件记录；此记录会保留。"}}}'::jsonb
$$;
CREATE FUNCTION valid_notification_template_content(k text,v jsonb) RETURNS boolean LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE lang text; c jsonb; title text; body text; rest text;
BEGIN
 IF NOT(notification_template_defaults() ? k) OR jsonb_typeof(v)<>'object'
 OR (SELECT count(*) FROM jsonb_object_keys(v))<>2 OR NOT(v ?& ARRAY['en','zh-CN']) THEN RETURN false; END IF;
 FOREACH lang IN ARRAY ARRAY['en','zh-CN'] LOOP
  c:=v->lang;
  IF jsonb_typeof(c)<>'object' OR (SELECT count(*) FROM jsonb_object_keys(c))<>2
  OR NOT(c ?& ARRAY['title','body']) OR jsonb_typeof(c->'title')<>'string' OR jsonb_typeof(c->'body')<>'string' THEN RETURN false; END IF;
  title:=c->>'title'; body:=c->>'body';
  IF title<>btrim(title) OR body<>btrim(body) OR octet_length(title) NOT BETWEEN 1 AND 120 OR octet_length(body) NOT BETWEEN 1 AND 1200
  OR title ~ '[[:cntrl:]<>]' OR replace(replace(body,chr(10),''),chr(9),'') ~ '[[:cntrl:]<>]'
  OR (title||body) ~* '(https?:|javascript:|data:|www[.])' THEN RETURN false; END IF;
  rest:=replace(replace(title||body,'{points}',''),'{resource_id}','');
  IF rest ~ '[{}]' OR (k='member.joined' AND position('{points}' IN title||body)>0)
  OR (k<>'member.joined' AND position('{points}' IN body)=0) THEN RETURN false; END IF;
 END LOOP;
 RETURN true;
EXCEPTION WHEN OTHERS THEN RETURN false;
END $$;
CREATE TABLE notification_templates(
 brand_id uuid NOT NULL REFERENCES brands(id),template_key text NOT NULL,
 version bigint NOT NULL DEFAULT 1 CHECK(version BETWEEN 1 AND 9007199254740991),
 content jsonb NOT NULL CHECK(valid_notification_template_content(template_key,content) IS TRUE),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(brand_id,template_key)
);
CREATE TABLE notification_template_revisions(
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),brand_id uuid NOT NULL REFERENCES brands(id),template_key text NOT NULL,
 version bigint NOT NULL CHECK(version BETWEEN 1 AND 9007199254740991),
 content jsonb NOT NULL CHECK(valid_notification_template_content(template_key,content) IS TRUE),
 changed_by uuid REFERENCES admin_accounts(id),reason text NOT NULL CHECK(length(trim(reason))>0 AND octet_length(reason)<=500),
 audit_log_id uuid REFERENCES audit_logs(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(brand_id,template_key,version),
 CHECK((version=1)=(changed_by IS NULL AND audit_log_id IS NULL)),
 CHECK(version=1 OR (changed_by IS NOT NULL AND audit_log_id IS NOT NULL))
);
CREATE FUNCTION guard_notification_template() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'notification template cannot be deleted'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.version<>1 OR NEW.content IS DISTINCT FROM notification_template_defaults()->NEW.template_key THEN RAISE EXCEPTION 'initial template must match default'; END IF;
 ELSIF NEW.brand_id IS DISTINCT FROM OLD.brand_id OR NEW.template_key IS DISTINCT FROM OLD.template_key OR NEW.version<>OLD.version+1 THEN
  RAISE EXCEPTION 'notification template identity/version invalid';
 END IF;
 NEW.updated_at:=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER guarded_notification_template BEFORE INSERT OR UPDATE OR DELETE ON notification_templates FOR EACH ROW EXECUTE FUNCTION guard_notification_template();
CREATE FUNCTION guard_notification_template_revision() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p notification_templates;a audit_logs;previous jsonb;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'notification template revision immutable'; END IF;
 SELECT * INTO p FROM notification_templates WHERE brand_id=NEW.brand_id AND template_key=NEW.template_key;
 IF NOT FOUND OR p.version IS DISTINCT FROM NEW.version OR p.content IS DISTINCT FROM NEW.content THEN RAISE EXCEPTION 'template revision requires current configuration'; END IF;
 IF NEW.version=1 THEN
  IF NEW.content IS DISTINCT FROM notification_template_defaults()->NEW.template_key THEN RAISE EXCEPTION 'initial history requires default'; END IF;
 ELSE
  SELECT content INTO previous FROM notification_template_revisions WHERE brand_id=NEW.brand_id AND template_key=NEW.template_key AND version=NEW.version-1;
  SELECT * INTO a FROM audit_logs WHERE id=NEW.audit_log_id;
  IF NOT FOUND OR previous IS NULL OR a.brand_id IS DISTINCT FROM NEW.brand_id OR a.actor_type IS DISTINCT FROM 'admin' OR a.actor_id IS DISTINCT FROM NEW.changed_by
  OR a.action IS DISTINCT FROM 'notification.template.update' OR a.resource_type IS DISTINCT FROM 'notification_template' OR a.resource_id IS DISTINCT FROM NEW.brand_id
  OR a.reason IS DISTINCT FROM NEW.reason OR a.after_json->>'brand_id' IS DISTINCT FROM NEW.brand_id::text OR a.after_json->>'key' IS DISTINCT FROM NEW.template_key
  OR (a.after_json->>'version')::bigint IS DISTINCT FROM NEW.version OR a.after_json->'content' IS DISTINCT FROM NEW.content
  OR a.before_json->>'brand_id' IS DISTINCT FROM NEW.brand_id::text OR a.before_json->>'key' IS DISTINCT FROM NEW.template_key
  OR (a.before_json->>'version')::bigint IS DISTINCT FROM NEW.version-1 OR a.before_json->'content' IS DISTINCT FROM previous THEN RAISE EXCEPTION 'matching template audit required'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guarded_notification_template_revision BEFORE INSERT OR UPDATE OR DELETE ON notification_template_revisions FOR EACH ROW EXECUTE FUNCTION guard_notification_template_revision();
CREATE FUNCTION require_notification_template_history() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM notification_template_revisions WHERE brand_id=NEW.brand_id AND template_key=NEW.template_key AND version=NEW.version AND content=NEW.content) THEN RAISE EXCEPTION 'template requires immutable history'; END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER notification_template_history_required AFTER INSERT OR UPDATE ON notification_templates DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_notification_template_history();
CREATE FUNCTION initialize_brand_notification_templates() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO notification_templates(brand_id,template_key,content) SELECT NEW.id,key,value FROM jsonb_each(notification_template_defaults());
 INSERT INTO notification_template_revisions(brand_id,template_key,version,content,reason)
 SELECT NEW.id,key,1,value,'Initial in-app notification template' FROM jsonb_each(notification_template_defaults());
 RETURN NEW;
END $$;
INSERT INTO notification_templates(brand_id,template_key,content) SELECT b.id,d.key,d.value FROM brands b CROSS JOIN jsonb_each(notification_template_defaults()) d;
INSERT INTO notification_template_revisions(brand_id,template_key,version,content,reason)
 SELECT brand_id,template_key,version,content,'Initial in-app notification template' FROM notification_templates;
CREATE TRIGGER brand_notification_templates_initialized AFTER INSERT ON brands FOR EACH ROW EXECUTE FUNCTION initialize_brand_notification_templates();

-- Existing notifications remain untouched and keep their legacy v1 renderer.
ALTER TABLE notifications DROP CONSTRAINT notifications_template_version_check;
ALTER TABLE notifications ALTER COLUMN template_version TYPE bigint;
ALTER TABLE notifications ADD CONSTRAINT notifications_template_version_check CHECK(template_version BETWEEN 1 AND 9007199254740991);
ALTER TABLE notifications ADD COLUMN content jsonb;
ALTER TABLE notifications ADD CONSTRAINT notification_content_shape CHECK((content IS NULL AND template_version=1) OR (content IS NOT NULL AND valid_notification_template_content(template_key,content) IS TRUE));
ALTER TABLE notifications ADD CONSTRAINT notification_template_revision_fk FOREIGN KEY(brand_id,template_key,template_version) REFERENCES notification_template_revisions(brand_id,template_key,version);
CREATE FUNCTION guard_notification_template_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p notification_templates;
BEGIN
 SELECT * INTO p FROM notification_templates WHERE brand_id=NEW.brand_id AND template_key=NEW.template_key FOR SHARE;
 IF NOT FOUND OR NEW.content IS NULL OR NEW.template_version IS DISTINCT FROM p.version OR NEW.content IS DISTINCT FROM p.content THEN RAISE EXCEPTION 'current immutable template snapshot required'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER notification_template_snapshot_required BEFORE INSERT ON notifications FOR EACH ROW EXECUTE FUNCTION guard_notification_template_snapshot();
DO $$ DECLARE f text;BEGIN
 FOREACH f IN ARRAY ARRAY['notification_template_defaults()','valid_notification_template_content(text,jsonb)','guard_notification_template()','guard_notification_template_revision()','require_notification_template_history()','initialize_brand_notification_templates()','guard_notification_template_snapshot()'] LOOP
  EXECUTE format('ALTER FUNCTION %I.%s SET search_path=pg_catalog,%I,pg_temp',current_schema(),f,current_schema());
 END LOOP;
END $$;
INSERT INTO permissions(key) VALUES('notification_template.view.brand'),('notification_template.view.platform'),('notification_template.write.brand') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_key) SELECT r.id,p.key FROM roles r CROSS JOIN permissions p WHERE r.is_bootstrap AND
 ((r.brand_id IS NOT NULL AND p.key IN('notification_template.view.brand','notification_template.write.brand')) OR
 (r.brand_id IS NULL AND p.key='notification_template.view.platform' AND EXISTS(SELECT 1 FROM admin_account_roles ar JOIN admin_accounts a ON a.id=ar.account_id WHERE ar.role_id=r.id AND a.is_super_admin))) ON CONFLICT DO NOTHING;
