BEGIN;
CREATE TABLE IF NOT EXISTS importer.company_candidates (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 source text NOT NULL CHECK(source='osm_astana'),
 external_id text NOT NULL,
 payload jsonb NOT NULL,
 review_status text NOT NULL DEFAULT 'pending' CHECK(review_status IN('pending','approved','rejected')),
 company_id uuid REFERENCES catalog.companies(id) ON DELETE SET NULL,
 collected_at timestamptz NOT NULL DEFAULT now(),
 reviewed_at timestamptz,
 reviewed_by uuid REFERENCES profiles.profiles(user_id) ON DELETE SET NULL,
 UNIQUE(source,external_id)
);
CREATE INDEX IF NOT EXISTS company_candidates_review_idx ON importer.company_candidates(review_status,collected_at,id);
ALTER TABLE importer.company_candidates ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON importer.company_candidates FROM PUBLIC;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='anon') THEN REVOKE ALL ON importer.company_candidates FROM anon; END IF;
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='authenticated') THEN REVOKE ALL ON importer.company_candidates FROM authenticated; END IF;
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='firstorder_backend') THEN GRANT SELECT,INSERT,UPDATE,DELETE ON importer.company_candidates TO firstorder_backend; END IF;
END $$;
COMMIT;
