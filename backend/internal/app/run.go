package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"ingeneriya/backend/internal/api"
	"ingeneriya/backend/internal/config"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func Run() error {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required; see backend/.env.example")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pc, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("invalid DATABASE_URL")
	}
	pc.MaxConns = 12
	pc.MinConns = 1
	pc.ConnConfig.ConnectTimeout = 5 * time.Second
	pc.ConnConfig.RuntimeParams["statement_timeout"] = "20000"
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return fmt.Errorf("database pool: %w", err)
	}
	defer pool.Close()
	check, cancel := context.WithTimeout(ctx, 5*time.Second)
	err = pool.Ping(check)
	cancel()
	if err != nil {
		return fmt.Errorf("database not reachable (check DATABASE_URL)")
	}
	server := api.New(pool, cfg)
	workerDone := make(chan struct{})
	go func() { defer close(workerDone); server.RunImports(ctx) }()
	collectionDone := make(chan struct{})
	go func() { defer close(collectionDone); server.RunCollection(ctx) }()
	httpServer := &http.Server{Addr: cfg.Addr, Handler: server.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	errch := make(chan error, 1)
	go func() { slog.Info("api started", "addr", cfg.Addr); errch <- httpServer.ListenAndServe() }()
	select {
	case <-ctx.Done():
	case err = <-errch:
		stop()
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = httpServer.Shutdown(shutdown)
	stop()
	<-workerDone
	<-collectionDone
	return err
}
