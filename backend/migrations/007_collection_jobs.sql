BEGIN;
CREATE TABLE importer.collection_jobs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 kind text NOT NULL CHECK(kind IN('collect','recheck')),
 source text NOT NULL DEFAULT 'osm_astana' CHECK(source='osm_astana'),
 city text NOT NULL DEFAULT 'Астана' CHECK(city='Астана'),
 requested_limit integer NOT NULL CHECK(requested_limit BETWEEN 1 AND 100),
 status text NOT NULL DEFAULT 'pending' CHECK(status IN('pending','running','completed','failed','canceled')),
 created_by uuid REFERENCES profiles.profiles(user_id) ON DELETE SET NULL,
 request_key text NOT NULL UNIQUE,
 attempt integer NOT NULL DEFAULT 0 CHECK(attempt BETWEEN 0 AND 3),
 processed integer NOT NULL DEFAULT 0,
 total integer NOT NULL DEFAULT 0,
 saved integer NOT NULL DEFAULT 0,
 skipped integer NOT NULL DEFAULT 0,
 found integer NOT NULL DEFAULT 0,
 needs_review integer NOT NULL DEFAULT 0,
 items jsonb,
 error_code text,
 cancel_requested boolean NOT NULL DEFAULT false,
 lease_token uuid,
 lease_until timestamptz,
 available_at timestamptz NOT NULL DEFAULT now(),
 created_at timestamptz NOT NULL DEFAULT now(),
 started_at timestamptz,
 finished_at timestamptz,
 updated_at timestamptz NOT NULL DEFAULT now()
);
-- Only one queued/running job; website checks and source fetches stay bounded.
CREATE UNIQUE INDEX collection_jobs_one_active ON importer.collection_jobs((true)) WHERE status IN('pending','running');
CREATE TABLE importer.collection_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 job_id uuid NOT NULL REFERENCES importer.collection_jobs(id) ON DELETE CASCADE,
 event text NOT NULL,
 actor_id uuid REFERENCES profiles.profiles(user_id) ON DELETE SET NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX collection_events_job ON importer.collection_events(job_id,id);
CREATE TABLE importer.collection_results (
 job_id uuid NOT NULL REFERENCES importer.collection_jobs(id) ON DELETE CASCADE,
 item_index integer NOT NULL,
 external_id text NOT NULL,
 name text NOT NULL,
 website_status text NOT NULL,
 reason text NOT NULL,
 saved boolean NOT NULL,
 checked_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(job_id,item_index)
);
CREATE TABLE importer.collection_sources (
 source text PRIMARY KEY CHECK(source='osm_astana'),
 next_fetch_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO importer.collection_sources(source) VALUES('osm_astana');
ALTER TABLE importer.collection_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE importer.collection_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE importer.collection_sources ENABLE ROW LEVEL SECURITY;
ALTER TABLE importer.collection_results ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON importer.collection_results FROM PUBLIC;
REVOKE ALL ON importer.collection_jobs,importer.collection_events,importer.collection_sources FROM PUBLIC;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='anon') THEN REVOKE ALL ON importer.collection_jobs,importer.collection_events,importer.collection_sources FROM anon; END IF;
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='anon') THEN REVOKE ALL ON importer.collection_results FROM anon; END IF;
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='authenticated') THEN REVOKE ALL ON importer.collection_jobs,importer.collection_events,importer.collection_sources FROM authenticated; END IF;
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='authenticated') THEN REVOKE ALL ON importer.collection_results FROM authenticated; END IF;
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='firstorder_backend') THEN
  GRANT SELECT,INSERT,UPDATE ON importer.collection_jobs,importer.collection_sources TO firstorder_backend;
  GRANT SELECT,INSERT ON importer.collection_events TO firstorder_backend;
  GRANT USAGE ON SEQUENCE importer.collection_events_id_seq TO firstorder_backend;
  GRANT SELECT,INSERT ON importer.collection_results TO firstorder_backend;
 END IF;
END $$;
COMMIT;
