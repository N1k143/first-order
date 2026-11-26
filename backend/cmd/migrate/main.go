package main

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"ingeneriya/backend/internal/migrations"
	"log"
	"os"
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("cannot connect to migration database")
	}
	defer conn.Close(ctx)
	applied, err := migrations.Apply(ctx, conn, dir)
	if err != nil {
		return err
	}
	for _, name := range applied {
		fmt.Println("applied", name)
	}
	return nil
}
