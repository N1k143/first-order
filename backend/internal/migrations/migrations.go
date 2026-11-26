package migrations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

var wrapper = regexp.MustCompile(`(?is)^BEGIN\s*;(.*)COMMIT\s*;\s*$`)

// body removes only the outer transaction. The runner owns COMMIT, including
// the ledger write, so interruption can never commit one without the other.
func body(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	for strings.HasPrefix(s, "--") {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			return "", fmt.Errorf("missing migration transaction")
		}
		s = strings.TrimSpace(s[i+1:])
	}
	match := wrapper.FindStringSubmatch(s)
	if match == nil {
		return "", fmt.Errorf("migration must use an outer BEGIN/COMMIT transaction")
	}
	if transactionControl(match[1]) {
		return "", fmt.Errorf("migration body may not control transactions")
	}
	return match[1], nil
}

var dollarTag = regexp.MustCompile(`^\$(?:[A-Za-z_][A-Za-z_0-9]*)?\$`)

// Ignore SQL strings, identifiers, comments and dollar-quoted function bodies.
// Top-level transaction control would break atomicity even inside tx.Exec.
func transactionControl(sql string) bool {
	first := true
	for i := 0; i < len(sql); {
		c := sql[i]
		if c == ' ' || c == '\n' || c == '\r' || c == '\t' {
			i++
			continue
		}
		if strings.HasPrefix(sql[i:], "--") {
			j := strings.IndexByte(sql[i:], '\n')
			if j < 0 {
				return false
			}
			i += j
			continue
		}
		if strings.HasPrefix(sql[i:], "/*") {
			depth := 1
			i += 2
			for i < len(sql) && depth > 0 {
				if strings.HasPrefix(sql[i:], "/*") {
					depth++
					i += 2
				} else if strings.HasPrefix(sql[i:], "*/") {
					depth--
					i += 2
				} else {
					i++
				}
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote := c
			i++
			for i < len(sql) {
				if sql[i] == quote {
					if i+1 < len(sql) && sql[i+1] == quote {
						i += 2
						continue
					}
					i++
					break
				}
				if sql[i] == '\\' && i+1 < len(sql) {
					i += 2
				} else {
					i++
				}
			}
			first = false
			continue
		}
		if c == '$' {
			tag := dollarTag.FindString(sql[i:])
			if tag != "" {
				i += len(tag)
				j := strings.Index(sql[i:], tag)
				if j < 0 {
					return false
				}
				i += j + len(tag)
				first = false
				continue
			}
		}
		if c == ';' {
			first = true
			i++
			continue
		}
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' {
			j := i + 1
			for j < len(sql) && (sql[j] >= 'A' && sql[j] <= 'Z' || sql[j] >= 'a' && sql[j] <= 'z' || sql[j] == '_') {
				j++
			}
			word := strings.ToUpper(sql[i:j])
			if first {
				switch word {
				case "BEGIN", "COMMIT", "END", "ROLLBACK", "ABORT", "START":
					return true
				}
			}
			first = false
			i = j
			continue
		}
		first = false
		i++
	}
	return false
}

func Apply(ctx context.Context, conn *pgx.Conn, dir string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil || len(files) == 0 {
		return nil, fmt.Errorf("no SQL migrations")
	}
	sort.Strings(files)
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock(1730191919)`); err != nil {
		return nil, err
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(1730191919)`)
	if _, err = conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS public.firstorder_schema_migrations(name text PRIMARY KEY,applied_at timestamptz NOT NULL DEFAULT now()); ALTER TABLE public.firstorder_schema_migrations ADD COLUMN IF NOT EXISTS checksum text`); err != nil {
		return nil, err
	}
	applied := []string{}
	for _, file := range files {
		name := filepath.Base(file)
		raw, e := os.ReadFile(file)
		if e != nil {
			return nil, fmt.Errorf("cannot read migration %s", name)
		}
		hash := sha256.Sum256(raw)
		checksum := hex.EncodeToString(hash[:])
		var stored *string
		err = conn.QueryRow(ctx, `SELECT checksum FROM public.firstorder_schema_migrations WHERE name=$1`, name).Scan(&stored)
		if err == nil {
			if stored != nil && *stored != checksum {
				return nil, fmt.Errorf("migration %s checksum mismatch; restore original SQL", name)
			}
			// Legacy installations have names only. Record an explicit baseline once;
			// it cannot establish which historical SQL actually ran before checksums.
			if stored == nil {
				if _, err = conn.Exec(ctx, `UPDATE public.firstorder_schema_migrations SET checksum=$2 WHERE name=$1 AND checksum IS NULL`, name, checksum); err != nil {
					return nil, err
				}
			}
			continue
		}
		if err != pgx.ErrNoRows {
			return nil, err
		}
		sql, e := body(string(raw))
		if e != nil {
			return nil, fmt.Errorf("migration %s: %w", name, e)
		}
		tx, e := conn.Begin(ctx)
		if e != nil {
			return nil, e
		}
		if _, e = tx.Exec(ctx, sql); e == nil {
			_, e = tx.Exec(ctx, `INSERT INTO public.firstorder_schema_migrations(name,checksum) VALUES($1,$2)`, name, checksum)
		}
		if e != nil {
			tx.Rollback(context.Background())
			return nil, fmt.Errorf("migration %s failed; transaction rolled back", name)
		}
		if e = tx.Commit(ctx); e != nil {
			return nil, fmt.Errorf("migration %s could not commit", name)
		}
		applied = append(applied, name)
	}
	return applied, nil
}
