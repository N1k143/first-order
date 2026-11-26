BEGIN;
CREATE TABLE profiles.notifications (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 user_id uuid NOT NULL REFERENCES profiles.profiles(user_id) ON DELETE CASCADE,
 actor_id uuid REFERENCES profiles.profiles(user_id) ON DELETE SET NULL,
 order_id uuid NOT NULL REFERENCES orders.orders(id) ON DELETE CASCADE,
 application_id uuid NOT NULL REFERENCES applications.applications(id) ON DELETE CASCADE,
 event text NOT NULL CHECK(event IN('application_submitted','application_withdrawn','application_accepted','application_rejected','order_completed','order_closed')),
 order_title text NOT NULL,
 company_name text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 read_at timestamptz,
 UNIQUE(user_id,event,application_id)
);
CREATE INDEX notifications_user_created ON profiles.notifications(user_id,created_at DESC,id DESC);
CREATE INDEX notifications_user_unread ON profiles.notifications(user_id) WHERE read_at IS NULL;
ALTER TABLE profiles.notifications ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON profiles.notifications FROM PUBLIC;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='anon') THEN REVOKE ALL ON profiles.notifications FROM anon; END IF;
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='authenticated') THEN REVOKE ALL ON profiles.notifications FROM authenticated; END IF;
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='firstorder_backend') THEN GRANT SELECT,INSERT,UPDATE,DELETE ON profiles.notifications TO firstorder_backend; END IF;
END $$;
COMMIT;
