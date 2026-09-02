// Command api is the BBank HTTP server.
//
// It wires config -> stores -> services -> handlers and owns process lifecycle:
// startup validation, connection pools, and graceful shutdown. It contains no
// business logic; that lives in internal/domain and internal/service.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bbank/internal/domain"
	bbhttp "bbank/internal/http"
	"bbank/internal/platform"
	"bbank/internal/service"
	"bbank/internal/store"

	"github.com/jackc/pgx/v5/pgxpool"
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

	// WI-22 removed the second, database/sql connection pool. It existed only
	// for internal/legacy, which is now deleted: the whole API runs on one pgx
	// pool, so there is one place where connection limits are configured and
	// one pool to watch under load.

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

	signer, err := platform.NewSigner(cfg.JWTPrivateKey, cfg.JWTIssuer, cfg.JWTAudience, cfg.AllowEphemeralKey)
	if err != nil {
		logger.Error("cannot initialise JWT signer", "error", err)
		os.Exit(1)
	}
	if cfg.JWTPrivateKey == "" {
		logger.Warn("using an EPHEMERAL JWT key — every restart invalidates all sessions; never do this in production")
	}

	// Runtime-mutable settings (WI-21). Seeded from config, then owned by
	// PATCH /api/v1/admin/flags so the deprecated shim can be switched off and
	// back on during an incident without waiting for a deploy.
	flags := platform.NewFlags(cfg.LegacyShim)
	logger.Info("legacy /api/go shim", "enabled", flags.LegacyShim())

	// WI-18: create the first admin as an INVITATION, never as a credential.
	// No-op unless BOOTSTRAP_ADMIN_EMAIL is set and no active admin exists, so
	// leaving the variable in a deployment re-opens nothing.
	bootstrapAdmin(context.Background(), logger, pool, signer, cfg)

	// WI-23: the daily no-show sweep. An in-process ticker, not a cron entry,
	// because it must not depend on anything outside the deployment — and it is
	// idempotent, so a second replica running it too is harmless.
	// WI-85 moves this to River alongside the rest of the async platform.
	sweepCtx, stopSweep := context.WithCancel(context.Background())
	defer stopSweep()
	go runNoShowSweep(sweepCtx, logger, pool)
	// The other two sweeps the schema documents but nothing was running:
	// idempotency_keys has a 24h TTL (§6.4) and sessions expire, and both tables
	// otherwise grow without bound — idempotency_keys while retaining stored
	// response bodies.
	go runRetentionSweeps(sweepCtx, logger, pool)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           bbhttp.NewRouter(bbhttp.Deps{Cfg: cfg, Pool: pool, Signer: signer, Flags: flags}),
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

// bootstrapAdmin turns an environment-supplied email into a one-time invitation
// for the first admin.
//
// It replaces the hardcoded `admin@admin.com / admin` credential (TD-02). The
// difference that matters: a literal is a permanent way in, whereas this
// creates an account with **no password**, an expiring single-use token, and
// only when the deployment has nobody who could otherwise do it.
//
// Failures here are logged, never fatal. A deployment that cannot bootstrap
// should still start and serve — refusing to boot would turn a convenience into
// an outage.
func bootstrapAdmin(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool, signer *platform.Signer, cfg platform.Config) {
	if cfg.BootstrapAdminEmail == "" {
		return
	}
	q := store.New(pool)
	users := service.NewUserService(pool, q, service.NewAuthService(q, signer))

	has, err := users.HasActiveAdmin(ctx)
	if err != nil {
		logger.Error("bootstrap: cannot check for an existing admin", "error", err)
		return
	}
	if has {
		logger.Info("bootstrap: an active admin already exists; skipping",
			"env", platform.BootstrapEnvVar)
		return
	}

	id, token, err := users.Invite(ctx, service.InviteParams{
		Email: cfg.BootstrapAdminEmail,
		Role:  string(domain.RoleAdmin),
	})
	if err != nil {
		logger.Error("bootstrap: could not create the first admin invitation",
			"email", cfg.BootstrapAdminEmail, "error", err)
		return
	}

	// The token is printed once, here, for whoever is performing the
	// deployment. It is not stored in recoverable form and cannot be reissued —
	// a lost one is replaced by inviting again, not by looking it up.
	logger.Warn("bootstrap: FIRST ADMIN INVITATION CREATED — use this token once, then it is gone",
		"user_id", id,
		"email", cfg.BootstrapAdminEmail,
		"accept_endpoint", "POST /api/v1/invites/accept",
		"invite_token", token,
		"expires_in_days", int(service.InviteTTL.Hours()/24),
	)
}

// noShowSweepInterval is how often past appointments are swept.
//
// Hourly rather than daily despite FR-13 calling it a daily sweep: the work is
// a single indexed UPDATE that matches nothing most of the time, and running it
// often means a stale `scheduled` row is visible for an hour at worst instead of
// a day. Idempotence is what makes the frequency a free choice.
const noShowSweepInterval = time.Hour

// runNoShowSweep marks past, un-attended appointments (FR-13).
//
// Safe to run twice, and safe to run in several replicas at once: the statement
// matches only rows still `scheduled` whose slot passed more than the grace
// period ago, so whichever replica gets there first leaves nothing for the
// others. That property is why this needs no leader election.
func runNoShowSweep(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool) {
	svc := service.NewAppointmentService(pool, store.New(pool))

	sweep := func() {
		runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		n, err := svc.SweepNoShows(runCtx)
		if err != nil {
			logger.Error("no-show sweep failed", "error", err)
			return
		}
		if n > 0 {
			// Only when it did something: an hourly "swept 0" line is noise that
			// trains people to ignore the log.
			logger.Info("no-show sweep", "marked", n)
		}
	}

	sweep() // once at boot, so a restart after downtime catches up immediately
	ticker := time.NewTicker(noShowSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweep()
		}
	}
}

// retentionSweepInterval paces the housekeeping sweeps. Hourly, for the same
// reason as the no-show sweep: the work is a bounded indexed DELETE, and running
// it often keeps each pass small.
const retentionSweepInterval = time.Hour

// runRetentionSweeps deletes expired idempotency keys and sessions.
//
// Both were documented and neither was scheduled: §6.4 says idempotency keys
// have a 24-hour TTL "swept nightly", and `DeleteExpiredSessions` existed with
// no caller. Left alone, `idempotency_keys` grows without bound *and* keeps the
// stored response bodies past the window in which they mean anything.
func runRetentionSweeps(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool) {
	q := store.New(pool)
	idem := service.NewIdempotencyService(q)

	sweep := func() {
		runCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()

		if n, err := idem.Sweep(runCtx); err != nil {
			logger.Error("idempotency sweep failed", "error", err)
		} else if n > 0 {
			logger.Info("idempotency sweep", "deleted", n)
		}

		if n, err := q.DeleteExpiredSessions(runCtx); err != nil {
			logger.Error("session sweep failed", "error", err)
		} else if n > 0 {
			logger.Info("session sweep", "deleted", n)
		}
	}

	sweep()
	ticker := time.NewTicker(retentionSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweep()
		}
	}
}
