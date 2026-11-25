// Queue a bounded operator run. The API worker performs the actual collection.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"ingeneriya/backend/internal/api"
	"os"
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
	kind := flag.String("kind", "collect", "collect new sources or recheck pending candidates")
	limit := flag.Int("limit", 10, "Maximum companies, 1–100")
	key := flag.String("request-key", "", "Required UUID; reuse it only for the same request")
	flag.Parse()
	if os.Getenv("DATABASE_URL") == "" {
		return fmt.Errorf("DATABASE_URL is required in the private environment")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return fmt.Errorf("database configuration invalid")
	}
	defer pool.Close()
	item, _, err := api.New(pool, api.Config{}).QueueCollection(ctx, *kind, *limit, *key, "")
	if err != nil {
		return fmt.Errorf("cannot queue collection: check parameters, active jobs and database availability")
	}
	return json.NewEncoder(os.Stdout).Encode(item)
}
