BEGIN;
CREATE OR REPLACE FUNCTION catalog.phone_key(value text) RETURNS text
LANGUAGE sql IMMUTABLE STRICT SET search_path=pg_catalog AS $$
 SELECT CASE WHEN length(n)=11 AND left(n,1)='8' THEN '7'||substring(n FROM 2)
             WHEN length(n)=11 AND left(n,1)='7' THEN n ELSE NULL END
 FROM (SELECT regexp_replace(value,'[^0-9]','','g') AS n) numbers;
$$;
REVOKE ALL ON FUNCTION catalog.phone_key(text) FROM PUBLIC;
DO $$ BEGIN IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='firstorder_backend') THEN GRANT EXECUTE ON FUNCTION catalog.phone_key(text) TO firstorder_backend; END IF; END $$;
-- An operator-run release is recorded as such, without impersonating an Auth user.
ALTER TABLE catalog.moderation_actions ALTER COLUMN moderator_id DROP NOT NULL;
ALTER TABLE catalog.moderation_actions ADD COLUMN IF NOT EXISTS actor_type text NOT NULL DEFAULT 'moderator';
ALTER TABLE importer.candidate_actions ADD COLUMN IF NOT EXISTS actor_type text NOT NULL DEFAULT 'moderator';
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='catalog.moderation_actions'::regclass AND conname='moderation_actions_actor_type_check') THEN
  ALTER TABLE catalog.moderation_actions ADD CONSTRAINT moderation_actions_actor_type_check CHECK((actor_type='moderator' AND moderator_id IS NOT NULL) OR (actor_type='operator' AND moderator_id IS NULL));
 END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='importer.candidate_actions'::regclass AND conname='candidate_actions_actor_type_check') THEN
  ALTER TABLE importer.candidate_actions ADD CONSTRAINT candidate_actions_actor_type_check CHECK(actor_type IN('moderator','operator'));
 END IF;
END $$;
COMMIT;
