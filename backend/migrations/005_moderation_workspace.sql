BEGIN;
ALTER TABLE importer.company_candidates
 ADD COLUMN IF NOT EXISTS draft jsonb NOT NULL DEFAULT '{}'::jsonb,
 ADD COLUMN IF NOT EXISTS revision bigint NOT NULL DEFAULT 1,
 ADD COLUMN IF NOT EXISTS review_note text;
CREATE OR REPLACE FUNCTION importer.advance_candidate_revision()
RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 NEW.revision := OLD.revision + 1;
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS candidate_revision ON importer.company_candidates;
CREATE TRIGGER candidate_revision BEFORE UPDATE ON importer.company_candidates
FOR EACH ROW EXECUTE FUNCTION importer.advance_candidate_revision();
CREATE TABLE IF NOT EXISTS importer.candidate_actions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 candidate_id uuid NOT NULL REFERENCES importer.company_candidates(id) ON DELETE CASCADE,
 moderator_id uuid REFERENCES profiles.profiles(user_id) ON DELETE SET NULL,
 action text NOT NULL CHECK(action IN('edit','approve','reject','link')),
 reason text,
 before_state jsonb NOT NULL,
 after_state jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS candidate_actions_candidate_idx ON importer.candidate_actions(candidate_id,created_at DESC,id);
CREATE INDEX IF NOT EXISTS candidate_actions_created_idx ON importer.candidate_actions(created_at DESC,id);
ALTER TABLE importer.candidate_actions ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON importer.candidate_actions FROM PUBLIC;
REVOKE ALL ON FUNCTION importer.advance_candidate_revision() FROM PUBLIC;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='anon') THEN REVOKE ALL ON importer.candidate_actions FROM anon; END IF;
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='authenticated') THEN REVOKE ALL ON importer.candidate_actions FROM authenticated; END IF;
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='firstorder_backend') THEN
  GRANT SELECT,INSERT ON importer.candidate_actions TO firstorder_backend;
  GRANT EXECUTE ON FUNCTION importer.advance_candidate_revision() TO firstorder_backend;
 END IF;
END $$;
COMMIT;
