-- Extend the existing default registry in place so brand initialization keeps
-- using the original function OID and receives the two new templates.
DO $$ DECLARE defaults jsonb; BEGIN
 defaults:=notification_template_defaults() || '{
  "draw.result.published":{"en":{"title":"Draw result published","body":"Historical result ID {resource_id}: the result published for this period. This is not a guarantee of a win or prize payment. View the actual result in the app; users cannot edit this notice."},"zh-CN":{"title":"开奖结果已公布","body":"历史开奖结果 ID：{resource_id}，表示本期已公布的结果，不代表中奖或派奖保证。请在应用中查看实际结果；用户不能编辑此通知。"}},
  "draw.result.corrected":{"en":{"title":"Draw result corrected","body":"Historical result ID {resource_id}: the corrected result for this period. This is not a guarantee of a win or prize payment. View the actual result in the app; users cannot edit this notice."},"zh-CN":{"title":"开奖结果已更正","body":"历史开奖结果 ID：{resource_id}，表示本期已更正的结果，不代表中奖或派奖保证。请在应用中查看实际结果；用户不能编辑此通知。"}}
 }'::jsonb;
 EXECUTE format('CREATE OR REPLACE FUNCTION %I.notification_template_defaults() RETURNS jsonb LANGUAGE sql IMMUTABLE AS %L',
  current_schema(),'SELECT '||quote_literal(defaults::text)||'::jsonb');
END $$;

-- Keep the deployed validator OID and all existing text/Unicode checks. The
-- legacy placeholder policy is unchanged; draw copies have their own rule.
CREATE OR REPLACE FUNCTION valid_notification_template_content(k text,v jsonb) RETURNS boolean LANGUAGE plpgsql IMMUTABLE AS $$
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
  IF rest ~ '[{}]' THEN RETURN false; END IF;
  IF k IN('draw.result.published','draw.result.corrected') THEN
   IF position('{points}' IN title||body)>0 OR position('{resource_id}' IN body)=0 THEN RETURN false; END IF;
  ELSIF (k='member.joined' AND position('{points}' IN title||body)>0)
  OR (k<>'member.joined' AND position('{points}' IN body)=0) THEN RETURN false;
  END IF;
 END LOOP;
 RETURN true;
EXCEPTION WHEN OTHERS THEN RETURN false;
END $$;

INSERT INTO notification_templates(brand_id,template_key,content)
 SELECT b.id,d.key,d.value FROM brands b CROSS JOIN LATERAL jsonb_each(notification_template_defaults()) d
 WHERE d.key IN('draw.result.published','draw.result.corrected');
INSERT INTO notification_template_revisions(brand_id,template_key,version,content,reason)
 SELECT brand_id,template_key,version,content,'Initial in-app notification template'
 FROM notification_templates WHERE template_key IN('draw.result.published','draw.result.corrected');

ALTER TABLE notifications DROP CONSTRAINT notifications_event_type_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_event_type_check CHECK(event_type IN
 ('member.joined','recharge.confirmed','bet.order.placed','bet.order.cancelled','bet.order.judged_cancelled','bet.order.abnormal','bet.order.won','bet.order.prize_reversed',
  'withdrawal.order.reviewing','withdrawal.order.processing','withdrawal.order.paid','withdrawal.order.rejected','withdrawal.order.failed','withdrawal.order.cancelled',
  'commission.paid','commission.adjusted','commission.corrected','reward.order.granted','reward.order.revocation_pending','reward.order.revoked',
  'draw.result.published','draw.result.corrected'));

-- CREATE OR REPLACE resets function configuration; restore the hardened
-- search_path used by the existing notification template functions.
DO $$ DECLARE app_schema text:=current_schema(); f record;
BEGIN
 FOR f IN SELECT p.oid::regprocedure signature FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
  WHERE n.nspname=app_schema AND p.proname IN('notification_template_defaults','valid_notification_template_content') LOOP
  EXECUTE format('ALTER FUNCTION %s SET search_path TO pg_catalog, %I, pg_temp',f.signature,app_schema);
 END LOOP;
END $$;
