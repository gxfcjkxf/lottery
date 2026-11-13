CREATE OR REPLACE FUNCTION initial_brand_presentation(b brands) RETURNS jsonb
    LANGUAGE sql IMMUTABLE
    AS $_$
 SELECT jsonb_build_object(
 'display_name',NULL,'logo_text',NULL,'logo_url',NULL,'favicon_url',NULL,
 'default_locale',CASE WHEN b.default_locale IN('en','zh-CN') THEN b.default_locale WHEN b.default_locale='zh' THEN 'zh-CN' ELSE NULL END,'available_locales',NULL,'content',NULL)
$_$;
ALTER FUNCTION initial_brand_presentation(brands) SET search_path = pg_catalog, public, pg_temp;
ALTER TABLE brands DROP COLUMN theme;
ALTER TABLE brand_presentations DISABLE TRIGGER guarded_presentation_update;
ALTER TABLE brand_presentations DISABLE TRIGGER presentation_history_required;
ALTER TABLE brand_presentation_revisions DISABLE TRIGGER guarded_presentation_history;
UPDATE brand_presentations SET config = config - ARRAY['primary_color','accent_color','success_color','warning_color','danger_color','font_family','font_scale','radius','shadow']::text[];
UPDATE brand_presentation_revisions SET config = config - ARRAY['primary_color','accent_color','success_color','warning_color','danger_color','font_family','font_scale','radius','shadow']::text[], effective = effective - ARRAY['primary_color','accent_color','success_color','warning_color','danger_color','font_family','font_scale','radius','shadow']::text[];
ALTER TABLE brand_presentation_revisions ENABLE TRIGGER guarded_presentation_history;
ALTER TABLE brand_presentations ENABLE TRIGGER presentation_history_required;
ALTER TABLE brand_presentations ENABLE TRIGGER guarded_presentation_update;
ALTER TABLE brand_presentations ADD CONSTRAINT brand_presentations_config_keys_check CHECK (jsonb_typeof(config) = 'object' AND config ?& ARRAY['display_name','logo_text','logo_url','favicon_url','default_locale','available_locales','content']::text[] AND config - ARRAY['display_name','logo_text','logo_url','favicon_url','default_locale','available_locales','content']::text[] = '{}'::jsonb);
ALTER TABLE brand_presentation_revisions ADD CONSTRAINT brand_presentation_revisions_config_keys_check CHECK (jsonb_typeof(config) = 'object' AND config ?& ARRAY['display_name','logo_text','logo_url','favicon_url','default_locale','available_locales','content']::text[] AND config - ARRAY['display_name','logo_text','logo_url','favicon_url','default_locale','available_locales','content']::text[] = '{}'::jsonb);
ALTER TABLE brand_presentation_revisions ADD CONSTRAINT brand_presentation_revisions_effective_keys_check CHECK (jsonb_typeof(effective) = 'object' AND effective ?& ARRAY['display_name','logo_text','logo_url','favicon_url','default_locale','available_locales','content']::text[] AND effective - ARRAY['display_name','logo_text','logo_url','favicon_url','default_locale','available_locales','content']::text[] = '{}'::jsonb);
