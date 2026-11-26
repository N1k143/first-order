BEGIN;
CREATE TABLE importer.replenishment_policy (
 id boolean PRIMARY KEY DEFAULT true CHECK(id),
 enabled boolean NOT NULL DEFAULT true,
 minimum integer NOT NULL DEFAULT 20 CHECK(minimum BETWEEN 1 AND 10000),
 target integer NOT NULL DEFAULT 50 CHECK(target BETWEEN 2 AND 20000 AND target>minimum),
 batch_size integer NOT NULL DEFAULT 25 CHECK(batch_size BETWEEN 1 AND 100),
 pending_limit integer NOT NULL DEFAULT 100 CHECK(pending_limit BETWEEN 1 AND 1000),
 cooldown_hours integer NOT NULL DEFAULT 24 CHECK(cooldown_hours BETWEEN 1 AND 168),
 revision bigint NOT NULL DEFAULT 1,
 refill_active boolean NOT NULL DEFAULT false,
 next_check_at timestamptz NOT NULL DEFAULT now(),
 next_run_at timestamptz NOT NULL DEFAULT now(),
 last_checked_at timestamptz,
 last_reason text,
 last_job_id uuid REFERENCES importer.collection_jobs(id) ON DELETE SET NULL,
 updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO importer.replenishment_policy(id) VALUES(true);
CREATE TABLE importer.replenishment_actions (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 actor_id uuid REFERENCES profiles.profiles(user_id) ON DELETE SET NULL,
 before_policy jsonb NOT NULL,
 after_policy jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE importer.replenishment_policy ENABLE ROW LEVEL SECURITY;
ALTER TABLE importer.replenishment_actions ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON importer.replenishment_policy,importer.replenishment_actions FROM PUBLIC;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='anon') THEN REVOKE ALL ON importer.replenishment_policy,importer.replenishment_actions FROM anon; END IF;
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='authenticated') THEN REVOKE ALL ON importer.replenishment_policy,importer.replenishment_actions FROM authenticated; END IF;
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='firstorder_backend') THEN
  GRANT SELECT,UPDATE ON importer.replenishment_policy TO firstorder_backend;
  GRANT SELECT,INSERT ON importer.replenishment_actions TO firstorder_backend;
  GRANT USAGE ON SEQUENCE importer.replenishment_actions_id_seq TO firstorder_backend;
 END IF;
END $$;
COMMIT;
