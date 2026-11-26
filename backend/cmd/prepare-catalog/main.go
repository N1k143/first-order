package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"ingeneriya/backend/internal/api"
	"os"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	apply := flag.Bool("apply", false, "Create qualifying hidden companies; default is read-only preview")
	publish := flag.Bool("publish", false, "Publish qualifying companies, requires --apply")
	report := flag.String("report", "", "Save private report JSON to a new file")
	flag.Parse()
	if *publish && !*apply {
		return fmt.Errorf("--publish requires --apply")
	}
	if os.Getenv("DATABASE_URL") == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return fmt.Errorf("database unavailable")
	}
	defer pool.Close()
	var output *os.File
	if *report != "" {
		output, err = os.OpenFile(*report, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return fmt.Errorf("report already exists or cannot be created")
		}
		defer output.Close()
	}
	result, err := api.New(pool, api.Config{}).PrepareCatalog(ctx, *apply, *publish)
	if err != nil {
		return fmt.Errorf("catalog preparation failed; transaction rolled back")
	}
	if output != nil {
		if err = json.NewEncoder(output).Encode(result); err != nil {
			return fmt.Errorf("catalog transaction completed but report writing failed")
		}
	}
	ready := 0
	for _, item := range result.Items {
		if item.Ready {
			ready++
		}
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"candidates": len(result.Items), "eligible": ready, "skipped": result.Skipped, "created": result.Created, "published": result.Published, "applied": *apply})
}
