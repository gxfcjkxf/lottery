-- Preserve immutable historical zero values. NOT VALID deliberately avoids
-- rewriting existing policies while enforcing positive N on every new write.
CREATE OR REPLACE FUNCTION valid_positive_withdrawal_multiple(v jsonb)
RETURNS boolean LANGUAGE plpgsql IMMUTABLE AS $$
BEGIN
 RETURN coalesce(valid_withdrawal_multiple(v) AND (v#>>'{}')::numeric>0,false);
EXCEPTION WHEN OTHERS THEN RETURN false;
END $$;

ALTER TABLE brand_withdrawal_policies
 ADD CONSTRAINT brand_withdrawal_positive_multiple
 CHECK(valid_positive_withdrawal_multiple(config->'turnover_multiple')) NOT VALID;

ALTER TABLE game_withdrawal_policies
 ADD CONSTRAINT game_withdrawal_positive_multiple
 CHECK(config->'turnover_multiple'='null'::jsonb OR
       valid_positive_withdrawal_multiple(config->'turnover_multiple')) NOT VALID;

ALTER TABLE withdrawal_policy_revisions
 ADD CONSTRAINT withdrawal_revision_positive_multiple
 CHECK((game_id IS NOT NULL AND config->'turnover_multiple'='null'::jsonb) OR
       valid_positive_withdrawal_multiple(config->'turnover_multiple')) NOT VALID;
