BEGIN;
-- Existing installations may already have this nullable profile column.
ALTER TABLE profiles.profiles ADD COLUMN IF NOT EXISTS avatar_url text;
COMMIT;
