BEGIN;
CREATE SCHEMA IF NOT EXISTS extensions;
CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA extensions;

CREATE FUNCTION catalog.search_normalize(text) RETURNS text
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE AS $$
 SELECT pg_catalog.btrim(pg_catalog.regexp_replace(pg_catalog.replace(pg_catalog.lower($1),'ё','е'),'[^[:alnum:]]+',' ','g'))
$$;

-- Supabase may already own pg_trgm in a different schema. Keep its location
-- and bind helpers/indexes to the actual extension rather than search_path.
DO $migration$
DECLARE ns text;
BEGIN
 SELECT n.nspname INTO STRICT ns FROM pg_extension e JOIN pg_namespace n ON n.oid=e.extnamespace WHERE e.extname='pg_trgm';
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='firstorder_backend') THEN
  EXECUTE format('GRANT USAGE ON SCHEMA %I TO firstorder_backend',ns);
 END IF;
 EXECUTE format('CREATE FUNCTION catalog.search_word_similar(text,text) RETURNS boolean LANGUAGE sql STABLE STRICT PARALLEL SAFE AS %L',
  format('SELECT $1 OPERATOR(%I.%%>) $2',ns));
 EXECUTE format('CREATE FUNCTION catalog.search_similarity(text,text) RETURNS real LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE AS %L',
  format('SELECT %I.word_similarity($1,$2)',ns));
 EXECUTE format('CREATE INDEX companies_search_trgm ON catalog.companies USING gin
  (catalog.search_normalize(coalesce(name,'''')||'' ''||coalesce(address,'''')||'' ''||coalesce(city,'''')||'' ''||coalesce(phone,'''')||'' ''||coalesce(email,'''')||'' ''||regexp_replace(coalesce(phone,''''),''[^0-9]'','''',''g'')) %I.gin_trgm_ops)',ns);
 EXECUTE format('CREATE INDEX categories_search_trgm ON catalog.categories USING gin (catalog.search_normalize(name) %I.gin_trgm_ops)',ns);
END $migration$;
CREATE INDEX companies_city_normalized ON catalog.companies(catalog.search_normalize(city));
COMMIT;
