package migrations

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestMigrationOuterTransaction(t *testing.T) {
	for _, sql := range []string{"BEGIN; SELECT 1; COMMIT;", "-- comment\nBEGIN; DO $$ BEGIN PERFORM 1; END $$; COMMIT;", "BEGIN; SELECT 'COMMIT;'; COMMIT;"} {
		if _, err := body(sql); err != nil {
			t.Fatal(err)
		}
	}
	for _, sql := range []string{"SELECT 1;", "BEGIN; SELECT 1; COMMIT; COMMIT;", "BEGIN; SELECT 1; END; COMMIT;", "BEGIN; SELECT 1; /* comment */ ROLLBACK; COMMIT;"} {
		if _, err := body(sql); err == nil {
			t.Fatalf("transaction escape accepted: %s", sql)
		}
	}
	files, _ := filepath.Glob("../../migrations/*.sql")
	if len(files) != 9 {
		t.Fatal("update expected migration coverage")
	}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = body(string(raw)); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
	}
}
func TestIntegrationMigrationAtomicityAndChecksums(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil || !strings.Contains(cfg.Database, "test") {
		t.Fatal("dedicated test database required")
	}
	conn, err := pgx.ConnectConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	dir := t.TempDir()
	var random [8]byte
	if _, err = rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	suffix := hex.EncodeToString(random[:])
	table := "migration_probe_" + suffix
	name := table + ".sql"
	constraint := "reject_" + suffix
	defer conn.Exec(context.Background(), `ALTER TABLE public.firstorder_schema_migrations DROP CONSTRAINT IF EXISTS `+constraint+`; DROP TABLE IF EXISTS public.`+table+`; DELETE FROM public.firstorder_schema_migrations WHERE name='`+name+`'`)
	file := filepath.Join(dir, name)
	write := func(sql string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(sql), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	absent := func() {
		t.Helper()
		var clean bool
		if err := conn.QueryRow(t.Context(), `SELECT to_regclass($1) IS NULL AND NOT EXISTS(SELECT 1 FROM public.firstorder_schema_migrations WHERE name=$2)`, "public."+table, name).Scan(&clean); err != nil || !clean {
			t.Fatal("partial migration or ledger committed")
		}
	}
	write("BEGIN; CREATE TABLE public." + table + "(id integer); SELECT missing_column; COMMIT;")
	if _, err = Apply(t.Context(), conn, dir); err == nil {
		t.Fatal("failed SQL migration succeeded")
	}
	absent()
	write("BEGIN; CREATE TABLE public." + table + "(id integer); COMMIT;")
	if _, err = conn.Exec(t.Context(), `ALTER TABLE public.firstorder_schema_migrations ADD CONSTRAINT `+constraint+` CHECK(name<>'`+name+`') NOT VALID`); err != nil {
		t.Fatal(err)
	}
	if _, err = Apply(t.Context(), conn, dir); err == nil {
		t.Fatal("failed ledger write succeeded")
	}
	absent()
	if _, err = conn.Exec(t.Context(), `ALTER TABLE public.firstorder_schema_migrations DROP CONSTRAINT `+constraint); err != nil {
		t.Fatal(err)
	}
	if _, err = Apply(t.Context(), conn, dir); err != nil {
		t.Fatal(err)
	}
	applied, err := Apply(t.Context(), conn, dir)
	if err != nil || len(applied) != 0 {
		t.Fatal("migration replay not idempotent")
	}
	write("BEGIN; CREATE TABLE public." + table + "(id integer,changed text); COMMIT;")
	if _, err = Apply(t.Context(), conn, dir); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatal("changed applied SQL accepted")
	}
}
