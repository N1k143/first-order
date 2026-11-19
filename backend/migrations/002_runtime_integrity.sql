BEGIN;
-- Direct browser/PostgREST access is intentionally closed. All application ACL
-- checks are made by the Go API. Auth continues to own its own schema and tables.
DO $$
DECLARE principal text;
BEGIN
 FOR principal IN SELECT rolname FROM pg_roles WHERE rolname IN ('anon','authenticated') LOOP
  EXECUTE format('REVOKE ALL ON ALL TABLES IN SCHEMA profiles,catalog,importer,orders,applications FROM %I',principal);
  EXECUTE format('REVOKE EXECUTE ON ALL FUNCTIONS IN SCHEMA profiles,catalog,importer,orders,applications FROM %I',principal);
 END LOOP;
END $$;
-- Defense in depth against selecting two accepted applications even outside the API.
CREATE UNIQUE INDEX IF NOT EXISTS applications_one_accepted_per_order
 ON applications.applications(order_id) WHERE status='accepted';
-- Internal, actor-explicit variant for a trusted backend connection. API code uses
-- the same order-first lock ordering for selection, submit, edit and withdrawal.
CREATE OR REPLACE FUNCTION orders.select_application(p_order_id uuid,p_application_id uuid,p_actor_id uuid)
RETURNS void LANGUAGE plpgsql SECURITY INVOKER SET search_path=pg_catalog AS $$
DECLARE company uuid; state text;
BEGIN
 SELECT company_id,status INTO company,state FROM orders.orders WHERE id=p_order_id FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'Order not found' USING ERRCODE='P0002'; END IF;
 IF state<>'open' THEN RAISE EXCEPTION 'Order must be open' USING ERRCODE='23514'; END IF;
 PERFORM 1 FROM profiles.company_members WHERE company_id=company AND user_id=p_actor_id FOR SHARE;
 IF NOT FOUND THEN RAISE EXCEPTION 'Membership required' USING ERRCODE='42501'; END IF;
 PERFORM 1 FROM applications.applications WHERE id=p_application_id AND order_id=p_order_id AND status='submitted' FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'Submitted application required' USING ERRCODE='23514'; END IF;
 UPDATE applications.applications SET status=CASE WHEN id=p_application_id THEN 'accepted' ELSE 'rejected' END,updated_at=now() WHERE order_id=p_order_id AND status='submitted';
 UPDATE orders.orders SET selected_application_id=p_application_id,status='contractor_selected',updated_at=now() WHERE id=p_order_id;
END $$;
REVOKE ALL ON FUNCTION orders.select_application(uuid,uuid,uuid) FROM PUBLIC;
-- A runtime login is provisioned by the operator, never from a checked-in password.
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='firstorder_backend') THEN
  CREATE ROLE firstorder_backend NOLOGIN BYPASSRLS;
 END IF;
END $$;
GRANT USAGE ON SCHEMA profiles,catalog,importer,orders,applications TO firstorder_backend;
GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA profiles,catalog,importer,orders,applications TO firstorder_backend;
GRANT SELECT(id,email) ON auth.users TO firstorder_backend;
GRANT USAGE ON SCHEMA auth TO firstorder_backend;
GRANT EXECUTE ON FUNCTION orders.select_application(uuid,uuid,uuid) TO firstorder_backend;
COMMIT;
