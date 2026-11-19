package main

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	dir := "migrations"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil || len(files) == 0 {
		return fmt.Errorf("no SQL migrations in %s", dir)
	}
	sort.Strings(files)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("cannot connect to migration database")
	}
	defer conn.Close(ctx)
	if _, err = conn.Exec(ctx, "SELECT pg_advisory_lock(1730191919)"); err != nil {
		return err
	}
	if _, err = conn.Exec(ctx, "CREATE TABLE IF NOT EXISTS public.firstorder_schema_migrations(name text PRIMARY KEY,applied_at timestamptz NOT NULL DEFAULT now())"); err != nil {
		return err
	}
	for _, file := range files {
		name := filepath.Base(file)
		var done bool
		if err = conn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM public.firstorder_schema_migrations WHERE name=$1)", name).Scan(&done); err != nil {
			return err
		}
		if done {
			continue
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		if _, err = conn.Exec(ctx, string(raw)); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err = conn.Exec(ctx, "INSERT INTO public.firstorder_schema_migrations(name) VALUES($1)", name); err != nil {
			return err
		}
		fmt.Println("applied", name)
	}
	return nil
}
