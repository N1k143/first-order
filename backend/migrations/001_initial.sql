-- Initial application schema, verified against the deployed PostgreSQL catalog.

-- Supabase owns auth.users; install Supabase Auth before applying this migration.

BEGIN;

DO $$ BEGIN IF to_regclass('auth.users') IS NULL THEN RAISE EXCEPTION 'Supabase auth.users is required'; END IF; END $$;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE SCHEMA IF NOT EXISTS profiles;

CREATE SCHEMA IF NOT EXISTS catalog;

CREATE SCHEMA IF NOT EXISTS importer;

CREATE SCHEMA IF NOT EXISTS orders;

CREATE SCHEMA IF NOT EXISTS applications;

CREATE TABLE IF NOT EXISTS applications.applications (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  order_id uuid NOT NULL,
  developer_id uuid NOT NULL,
  message text NOT NULL,
  proposed_price numeric(12,2),
  proposed_deadline date,
  created_at timestamp with time zone DEFAULT now() NOT NULL,
  updated_at timestamp with time zone DEFAULT now() NOT NULL,
  status text DEFAULT 'submitted'::text NOT NULL,
  CONSTRAINT applications_order_id_developer_id_key UNIQUE (order_id, developer_id),
  CONSTRAINT applications_pkey PRIMARY KEY (id),
  CONSTRAINT applications_proposed_price_check CHECK (proposed_price IS NULL OR proposed_price >= 0::numeric),
  CONSTRAINT applications_status_check CHECK (status = ANY (ARRAY['submitted'::text, 'accepted'::text, 'rejected'::text, 'withdrawn'::text]))
);

CREATE TABLE IF NOT EXISTS catalog.categories (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  name text NOT NULL,
  CONSTRAINT categories_name_key UNIQUE (name),
  CONSTRAINT categories_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS catalog.companies (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  name text NOT NULL,
  category_id uuid,
  city text DEFAULT 'Астана'::text NOT NULL,
  address text,
  phone text,
  email text,
  website_url text,
  website_status text DEFAULT 'needs_review'::text NOT NULL,
  website_checked_at timestamp with time zone,
  source_url text,
  visibility text DEFAULT 'published'::text NOT NULL,
  created_at timestamp with time zone DEFAULT now() NOT NULL,
  updated_at timestamp with time zone DEFAULT now() NOT NULL,
  CONSTRAINT companies_pkey PRIMARY KEY (id),
  CONSTRAINT companies_visibility_check CHECK (visibility = ANY (ARRAY['published'::text, 'hidden'::text])),
  CONSTRAINT companies_website_status_check CHECK (website_status = ANY (ARRAY['not_found'::text, 'found'::text, 'needs_review'::text]))
);

CREATE TABLE IF NOT EXISTS catalog.company_contacts (
  developer_id uuid NOT NULL,
  company_id uuid NOT NULL,
  status text DEFAULT 'planned'::text NOT NULL,
  notes text,
  contacted_at timestamp with time zone,
  updated_at timestamp with time zone DEFAULT now() NOT NULL,
  CONSTRAINT company_contacts_pkey PRIMARY KEY (developer_id, company_id),
  CONSTRAINT company_contacts_status_check CHECK (status = ANY (ARRAY['planned'::text, 'contacted'::text, 'replied'::text, 'declined'::text]))
);

CREATE TABLE IF NOT EXISTS catalog.correction_requests (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  company_id uuid NOT NULL,
  requested_by uuid NOT NULL,
  message text NOT NULL,
  status text DEFAULT 'pending'::text NOT NULL,
  resolved_by uuid,
  resolution_note text,
  created_at timestamp with time zone DEFAULT now() NOT NULL,
  resolved_at timestamp with time zone,
  CONSTRAINT correction_requests_pkey PRIMARY KEY (id),
  CONSTRAINT correction_requests_status_check CHECK (status = ANY (ARRAY['pending'::text, 'approved'::text, 'rejected'::text]))
);

CREATE TABLE IF NOT EXISTS catalog.favorites (
  developer_id uuid NOT NULL,
  company_id uuid NOT NULL,
  created_at timestamp with time zone DEFAULT now() NOT NULL,
  CONSTRAINT favorites_pkey PRIMARY KEY (developer_id, company_id)
);

CREATE TABLE IF NOT EXISTS catalog.moderation_actions (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  moderator_id uuid NOT NULL,
  company_id uuid,
  order_id uuid,
  action text NOT NULL,
  reason text,
  created_at timestamp with time zone DEFAULT now() NOT NULL,
  CONSTRAINT moderation_actions_action_check CHECK (action = ANY (ARRAY['publish'::text, 'hide'::text, 'correct'::text, 'approve'::text, 'reject'::text])),
  CONSTRAINT moderation_actions_pkey PRIMARY KEY (id),
  CONSTRAINT moderation_actions_target_check CHECK (company_id IS NOT NULL OR order_id IS NOT NULL)
);

CREATE TABLE IF NOT EXISTS importer.import_runs (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  source text NOT NULL,
  status text NOT NULL,
  started_at timestamp with time zone DEFAULT now() NOT NULL,
  processed_count integer DEFAULT 0 NOT NULL,
  error_message text,
  finished_at timestamp with time zone,
  CONSTRAINT import_runs_pkey PRIMARY KEY (id),
  CONSTRAINT import_runs_processed_count_check CHECK (processed_count >= 0),
  CONSTRAINT import_runs_status_check CHECK (status = ANY (ARRAY['pending'::text, 'running'::text, 'completed'::text, 'failed'::text]))
);

CREATE TABLE IF NOT EXISTS importer.source_records (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  source text NOT NULL,
  external_id text NOT NULL,
  company_id uuid,
  last_run_id uuid NOT NULL,
  raw_data jsonb NOT NULL,
  last_seen_at timestamp with time zone DEFAULT now() NOT NULL,
  CONSTRAINT source_records_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS orders.orders (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  company_id uuid NOT NULL,
  created_by uuid NOT NULL,
  title text NOT NULL,
  description text NOT NULL,
  budget_min numeric,
  budget_max numeric,
  deadline date,
  status text DEFAULT 'draft'::text NOT NULL,
  visibility boolean DEFAULT true NOT NULL,
  selected_application_id uuid,
  created_at timestamp with time zone DEFAULT now() NOT NULL,
  updated_at timestamp with time zone DEFAULT now() NOT NULL,
  CONSTRAINT orders_budget_check CHECK ((budget_min IS NULL OR budget_min >= 0::numeric) AND (budget_max IS NULL OR budget_max >= 0::numeric) AND (budget_min IS NULL OR budget_max IS NULL OR budget_max >= budget_min)),
  CONSTRAINT orders_pkey PRIMARY KEY (id),
  CONSTRAINT orders_status_check CHECK (status = ANY (ARRAY['draft'::text, 'open'::text, 'contractor_selected'::text, 'completed'::text, 'closed'::text, 'hidden'::text]))
);

CREATE TABLE IF NOT EXISTS profiles.company_members (
  company_id uuid NOT NULL,
  user_id uuid NOT NULL,
  member_role text DEFAULT 'representative'::text NOT NULL,
  created_at timestamp with time zone DEFAULT now() NOT NULL,
  CONSTRAINT company_members_pkey PRIMARY KEY (company_id, user_id),
  CONSTRAINT company_members_role_check CHECK (member_role = ANY (ARRAY['owner'::text, 'representative'::text]))
);

CREATE TABLE IF NOT EXISTS profiles.portfolio_items (
  id uuid DEFAULT gen_random_uuid() NOT NULL,
  developer_id uuid NOT NULL,
  title text NOT NULL,
  description text,
  project_url text,
  created_at timestamp with time zone DEFAULT now() NOT NULL,
  updated_at timestamp with time zone DEFAULT now() NOT NULL,
  CONSTRAINT portfolio_items_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS profiles.profiles (
  user_id uuid NOT NULL,
  name text NOT NULL,
  bio text,
  skills text,
  github_url text,
  is_moderator boolean,
  created_at timestamp with time zone DEFAULT now() NOT NULL,
  updated_at timestamp with time zone DEFAULT now() NOT NULL,
  role text DEFAULT 'developer'::text NOT NULL,
  CONSTRAINT profiles_pkey PRIMARY KEY (user_id),
  CONSTRAINT profiles_role_check CHECK (role = ANY (ARRAY['developer'::text, 'moderator'::text]))
);

CREATE UNIQUE INDEX IF NOT EXISTS applications_id_order_id_key ON applications.applications USING btree (id, order_id);

CREATE INDEX IF NOT EXISTS idx_applications_developer_id ON applications.applications USING btree (developer_id);

CREATE INDEX IF NOT EXISTS idx_companies_category ON catalog.companies USING btree (category_id);

CREATE INDEX IF NOT EXISTS idx_companies_city ON catalog.companies USING btree (city);

CREATE INDEX IF NOT EXISTS idx_companies_name ON catalog.companies USING btree (name);

CREATE INDEX IF NOT EXISTS correction_requests_company_id_idx ON catalog.correction_requests USING btree (company_id);

CREATE INDEX IF NOT EXISTS source_records_company_id_idx ON importer.source_records USING btree (company_id);

CREATE UNIQUE INDEX IF NOT EXISTS source_records_source_external_id_key ON importer.source_records USING btree (source, external_id);

CREATE INDEX IF NOT EXISTS orders_company_id_idx ON orders.orders USING btree (company_id);

CREATE INDEX IF NOT EXISTS orders_created_by_idx ON orders.orders USING btree (created_by);

CREATE INDEX IF NOT EXISTS orders_status_visible_idx ON orders.orders USING btree (status, visibility);

CREATE INDEX IF NOT EXISTS company_members_user_id_idx ON profiles.company_members USING btree (user_id);

CREATE INDEX IF NOT EXISTS portfolio_items_developer_id_idx ON profiles.portfolio_items USING btree (developer_id);

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='applications.applications'::regclass AND conname='applications_developer_id_fkey') THEN ALTER TABLE applications.applications ADD CONSTRAINT applications_developer_id_fkey FOREIGN KEY (developer_id) REFERENCES profiles.profiles(user_id) ON DELETE CASCADE; END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='applications.applications'::regclass AND conname='applications_order_id_fkey') THEN ALTER TABLE applications.applications ADD CONSTRAINT applications_order_id_fkey FOREIGN KEY (order_id) REFERENCES orders.orders(id) ON DELETE CASCADE; END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='catalog.companies'::regclass AND conname='companies_category_id_fkey') THEN ALTER TABLE catalog.companies ADD CONSTRAINT companies_category_id_fkey FOREIGN KEY (category_id) REFERENCES catalog.categories(id) ON DELETE SET NULL; END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='catalog.company_contacts'::regclass AND conname='company_contacts_company_id_fkey') THEN ALTER TABLE catalog.company_contacts ADD CONSTRAINT company_contacts_company_id_fkey FOREIGN KEY (company_id) REFERENCES catalog.companies(id) ON DELETE CASCADE; END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='catalog.company_contacts'::regclass AND conname='company_contacts_developer_id_fkey') THEN ALTER TABLE catalog.company_contacts ADD CONSTRAINT company_contacts_developer_id_fkey FOREIGN KEY (developer_id) REFERENCES profiles.profiles(user_id) ON DELETE CASCADE; END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='catalog.correction_requests'::regclass AND conname='correction_requests_company_id_fkey') THEN ALTER TABLE catalog.correction_requests ADD CONSTRAINT correction_requests_company_id_fkey FOREIGN KEY (company_id) REFERENCES catalog.companies(id) ON DELETE CASCADE; END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='catalog.correction_requests'::regclass AND conname='correction_requests_requested_by_fkey') THEN ALTER TABLE catalog.correction_requests ADD CONSTRAINT correction_requests_requested_by_fkey FOREIGN KEY (requested_by) REFERENCES profiles.profiles(user_id) ON DELETE CASCADE; END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='catalog.correction_requests'::regclass AND conname='correction_requests_resolved_by_fkey') THEN ALTER TABLE catalog.correction_requests ADD CONSTRAINT correction_requests_resolved_by_fkey FOREIGN KEY (resolved_by) REFERENCES profiles.profiles(user_id) ON DELETE SET NULL; END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='catalog.favorites'::regclass AND conname='favorites_company_id_fkey') THEN ALTER TABLE catalog.favorites ADD CONSTRAINT favorites_company_id_fkey FOREIGN KEY (company_id) REFERENCES catalog.companies(id) ON DELETE CASCADE; END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='catalog.favorites'::regclass AND conname='favorites_developer_id_fkey') THEN ALTER TABLE catalog.favorites ADD CONSTRAINT favorites_developer_id_fkey FOREIGN KEY (developer_id) REFERENCES profiles.profiles(user_id) ON DELETE CASCADE; END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='catalog.moderation_actions'::regclass AND conname='moderation_actions_company_id_fkey') THEN ALTER TABLE catalog.moderation_actions ADD CONSTRAINT moderation_actions_company_id_fkey FOREIGN KEY (company_id) REFERENCES catalog.companies(id) ON DELETE SET NULL; END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='catalog.moderation_actions'::regclass AND conname='moderation_actions_moderator_id_fkey') THEN ALTER TABLE catalog.moderation_actions ADD CONSTRAINT moderation_actions_moderator_id_fkey FOREIGN KEY (moderator_id) REFERENCES profiles.profiles(user_id) ON DELETE RESTRICT; END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='catalog.moderation_actions'::regclass AND conname='moderation_actions_order_id_fkey') THEN ALTER TABLE catalog.moderation_actions ADD CONSTRAINT moderation_actions_order_id_fkey FOREIGN KEY (order_id) REFERENCES orders.orders(id) ON DELETE SET NULL; END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='importer.source_records'::regclass AND conname='source_records_company_id_fkey') THEN ALTER TABLE importer.source_records ADD CONSTRAINT source_records_company_id_fkey FOREIGN KEY (company_id) REFERENCES catalog.companies(id) ON DELETE SET NULL; END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='importer.source_records'::regclass AND conname='source_records_last_run_id_fkey') THEN ALTER TABLE importer.source_records ADD CONSTRAINT source_records_last_run_id_fkey FOREIGN KEY (last_run_id) REFERENCES importer.import_runs(id) ON DELETE RESTRICT; END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='orders.orders'::regclass AND conname='orders_company_id_fkey') THEN ALTER TABLE orders.orders ADD CONSTRAINT orders_company_id_fkey FOREIGN KEY (company_id) REFERENCES catalog.companies(id) ON DELETE RESTRICT; END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='orders.orders'::regclass AND conname='orders_created_by_fkey') THEN ALTER TABLE orders.orders ADD CONSTRAINT orders_created_by_fkey FOREIGN KEY (created_by) REFERENCES profiles.profiles(user_id) ON DELETE RESTRICT; END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='orders.orders'::regclass AND conname='orders_selected_application_fkey') THEN ALTER TABLE orders.orders ADD CONSTRAINT orders_selected_application_fkey FOREIGN KEY (selected_application_id, id) REFERENCES applications.applications(id, order_id) ON DELETE RESTRICT; END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='profiles.company_members'::regclass AND conname='company_members_company_id_fkey') THEN ALTER TABLE profiles.company_members ADD CONSTRAINT company_members_company_id_fkey FOREIGN KEY (company_id) REFERENCES catalog.companies(id) ON DELETE CASCADE; END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='profiles.company_members'::regclass AND conname='company_members_user_id_fkey') THEN ALTER TABLE profiles.company_members ADD CONSTRAINT company_members_user_id_fkey FOREIGN KEY (user_id) REFERENCES profiles.profiles(user_id) ON DELETE CASCADE; END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='profiles.portfolio_items'::regclass AND conname='portfolio_items_developer_id_fkey') THEN ALTER TABLE profiles.portfolio_items ADD CONSTRAINT portfolio_items_developer_id_fkey FOREIGN KEY (developer_id) REFERENCES profiles.profiles(user_id) ON DELETE CASCADE; END IF; END $$;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='profiles.profiles'::regclass AND conname='profiles_user_id_fkey') THEN ALTER TABLE profiles.profiles ADD CONSTRAINT profiles_user_id_fkey FOREIGN KEY (user_id) REFERENCES auth.users(id) ON DELETE CASCADE; END IF; END $$;

ALTER TABLE applications.applications ENABLE ROW LEVEL SECURITY;

ALTER TABLE catalog.categories ENABLE ROW LEVEL SECURITY;

ALTER TABLE catalog.companies ENABLE ROW LEVEL SECURITY;

ALTER TABLE catalog.company_contacts ENABLE ROW LEVEL SECURITY;

ALTER TABLE catalog.correction_requests ENABLE ROW LEVEL SECURITY;

ALTER TABLE catalog.favorites ENABLE ROW LEVEL SECURITY;

ALTER TABLE catalog.moderation_actions ENABLE ROW LEVEL SECURITY;

ALTER TABLE importer.import_runs ENABLE ROW LEVEL SECURITY;

ALTER TABLE importer.source_records ENABLE ROW LEVEL SECURITY;

ALTER TABLE orders.orders ENABLE ROW LEVEL SECURITY;

ALTER TABLE profiles.company_members ENABLE ROW LEVEL SECURITY;

ALTER TABLE profiles.portfolio_items ENABLE ROW LEVEL SECURITY;

ALTER TABLE profiles.profiles ENABLE ROW LEVEL SECURITY;

CREATE OR REPLACE FUNCTION profiles.touch_updated_at() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN NEW.updated_at=now(); RETURN NEW; END $$;

DROP TRIGGER IF EXISTS applications_applications_touch ON applications.applications;

CREATE TRIGGER applications_applications_touch BEFORE UPDATE ON applications.applications FOR EACH ROW EXECUTE FUNCTION profiles.touch_updated_at();

DROP TRIGGER IF EXISTS catalog_companies_touch ON catalog.companies;

CREATE TRIGGER catalog_companies_touch BEFORE UPDATE ON catalog.companies FOR EACH ROW EXECUTE FUNCTION profiles.touch_updated_at();

DROP TRIGGER IF EXISTS catalog_company_contacts_touch ON catalog.company_contacts;

CREATE TRIGGER catalog_company_contacts_touch BEFORE UPDATE ON catalog.company_contacts FOR EACH ROW EXECUTE FUNCTION profiles.touch_updated_at();

DROP TRIGGER IF EXISTS orders_orders_touch ON orders.orders;

CREATE TRIGGER orders_orders_touch BEFORE UPDATE ON orders.orders FOR EACH ROW EXECUTE FUNCTION profiles.touch_updated_at();

DROP TRIGGER IF EXISTS profiles_portfolio_items_touch ON profiles.portfolio_items;

CREATE TRIGGER profiles_portfolio_items_touch BEFORE UPDATE ON profiles.portfolio_items FOR EACH ROW EXECUTE FUNCTION profiles.touch_updated_at();

DROP TRIGGER IF EXISTS profiles_profiles_touch ON profiles.profiles;

CREATE TRIGGER profiles_profiles_touch BEFORE UPDATE ON profiles.profiles FOR EACH ROW EXECUTE FUNCTION profiles.touch_updated_at();

COMMIT;
