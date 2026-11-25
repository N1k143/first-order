// Operator-run bounded discovery. Collection never publishes companies automatically.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"ingeneriya/backend/internal/discovery"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	limit := flag.Int("limit", 100, "Maximum companies, 1–100, balanced across categories")
	apply := flag.Bool("apply", false, "Store review candidates in DATABASE_URL; never publish")
	input := flag.String("input", "", "Reuse a previously saved Overpass snapshot")
	snapshotPath := flag.String("snapshot", "", "Save successful raw snapshot for offline replay")
	output := flag.String("output", "", "Write collected evidence JSON (mode 0600)")
	flag.Parse()
	if *limit < 1 || *limit > 100 {
		return fmt.Errorf("limit must be between 1 and 100")
	}
	if *apply && os.Getenv("DATABASE_URL") == "" {
		return fmt.Errorf("DATABASE_URL is required with --apply")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	var data discovery.Snapshot
	var err error
	if *input != "" {
		f, e := os.Open(*input)
		if e != nil {
			return fmt.Errorf("cannot open snapshot")
		}
		defer f.Close()
		data, err = discovery.Decode(f)
	} else {
		data, err = discovery.Fetch(ctx, os.Getenv("OVERPASS_URL"))
	}
	if err != nil {
		return err
	}
	if *snapshotPath != "" {
		raw, e := json.Marshal(data)
		if e != nil {
			return e
		}
		if err = writePrivate(*snapshotPath, raw); err != nil {
			return err
		}
	}
	candidates := discovery.Select(data, *limit)
	if len(candidates) == 0 {
		return fmt.Errorf("no companies found; nothing stored")
	}
	checker := discovery.NewChecker()
	defer checker.Client.CloseIdleConnections()
	wikidata := discovery.NewWikidata()
	defer wikidata.Client.CloseIdleConnections()
	counts := map[string]int{}
	categories := map[string]int{}
	for i := range candidates {
		if err = ctx.Err(); err != nil {
			return fmt.Errorf("collection canceled; nothing stored")
		}
		discovery.Enrich(ctx, &candidates[i], wikidata)
		candidates[i].Check = checker.Discover(ctx, candidates[i])
		counts[candidates[i].Check.Status]++
		categories[candidates[i].Category]++
		fmt.Fprintf(os.Stderr, "checked %d/%d\n", i+1, len(candidates))
		select {
		case <-ctx.Done():
			return fmt.Errorf("collection canceled; nothing stored")
		case <-time.After(300 * time.Millisecond):
		}
	}
	searched := 0
	for _, candidate := range candidates {
		if candidate.Check.SearchPerformed {
			searched++
		}
	}
	report := map[string]any{"city": "Астана", "source": "osm_astana", "attribution": discovery.Attribution, "collected_at": time.Now().UTC(), "total": len(candidates), "website_statuses": counts, "categories": categories, "search_performed": searched > 0, "searched_count": searched, "candidates": candidates}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if *output != "" {
		if err = writePrivate(*output, raw); err != nil {
			return err
		}
	}
	if *apply {
		pool, e := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
		if e != nil {
			return fmt.Errorf("database connection failed")
		}
		defer pool.Close()
		tx, e := pool.Begin(ctx)
		if e != nil {
			return fmt.Errorf("database unavailable")
		}
		defer tx.Rollback(context.Background())
		for _, candidate := range candidates {
			payload, e := json.Marshal(candidate)
			if e != nil {
				return e
			}
			// Rejected/approved decisions and already published cards survive repeat collection.
			_, e = tx.Exec(ctx, `INSERT INTO importer.company_candidates(source,external_id,payload) VALUES($1,$2,$3) ON CONFLICT(source,external_id) DO UPDATE SET payload=EXCLUDED.payload,collected_at=now() WHERE importer.company_candidates.review_status='pending' AND importer.company_candidates.draft='{}'::jsonb`, candidate.Source, candidate.ExternalID, payload)
			if e != nil {
				return fmt.Errorf("cannot stage candidates; transaction rolled back")
			}
		}
		if tx.Commit(ctx) != nil {
			return fmt.Errorf("cannot commit candidates")
		}
	}
	fmt.Printf("{\"city\":\"Астана\",\"collected\":%d,\"found\":%d,\"needs_review\":%d,\"staged\":%t,\"published\":0,\"searched_count\":%d}\n", len(candidates), counts["found"], counts["needs_review"], *apply, searched)
	return nil
}
func writePrivate(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("output already exists or cannot be created")
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}
