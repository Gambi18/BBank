// Command api is the BBank HTTP server.
//
// It wires config -> stores -> services -> handlers and owns process lifecycle:
// startup validation, connection pools, and graceful shutdown. It contains no
// business logic; that lives in internal/domain and internal/service.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	bbhttp "bbank/internal/http"
	"bbank/internal/platform"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/lib/pq" // legacy database/sql path; removed when internal/legacy is empty
)

func main() {
	cfg, err := platform.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)
	logger.Info("starting", "dsn", platform.SafeDSN(cfg.DatabaseURL))

	ctx := context.Background()

	// New path: pgx pool. Pool limits per TRD §11.3 (WI-04).
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		logger.Error("invalid DATABASE_URL", "error", err)
		os.Exit(1)
	}
	poolCfg.MaxConns = 25
	poolCfg.MinConns = 2
	poolCfg.MaxConnLifetime = 30 * time.Minute
	poolCfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		logger.Error("cannot create connection pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Legacy path: database/sql, for resources internal/legacy still serves.
	// Deleted along with that package once WI-22 finishes the migration.
	legacyDB, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		logger.Error("cannot open legacy database handle", "error", err)
		os.Exit(1)
	}
	defer legacyDB.Close()
	legacyDB.SetMaxOpenConns(10)
	legacyDB.SetMaxIdleConns(10)
	legacyDB.SetConnMaxLifetime(30 * time.Minute)
	legacyDB.SetConnMaxIdleTime(5 * time.Minute)

	// The DB may still be initialising in Docker; retry rather than exiting and
	// taking the container down with us.
	for attempt := 1; ; attempt++ {
		if err = pool.Ping(ctx); err == nil {
			logger.Info("database connection successful")
			break
		}
		if attempt >= 30 {
			logger.Error("database unreachable", "attempts", attempt, "error", err)
			os.Exit(1)
		}
		logger.Warn("waiting for database", "attempt", attempt, "max_attempts", 30, "error", err)
		time.Sleep(2 * time.Second)
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           bbhttp.NewRouter(bbhttp.Deps{Cfg: cfg, Pool: pool, LegacyD: legacyDB}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	shutdownDone := make(chan struct{})
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		sig := <-sigCh
		logger.Info("shutdown signal received, draining", "signal", sig.String(), "grace", cfg.ShutdownGrace.String())
		sctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownGrace)
		defer cancel()
		if err := srv.Shutdown(sctx); err != nil {
			logger.Error("graceful shutdown failed", "error", err)
		}
		close(shutdownDone)
	}()

	logger.Info("server listening", "port", cfg.Port, "allowed_origins", cfg.AllowedOrigins)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
	<-shutdownDone
	logger.Info("shutdown complete")
}
