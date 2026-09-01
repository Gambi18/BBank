// Package testsupport spins up a real PostgreSQL 18 for integration tests
// (WI-29, TRD §13.2).
//
// Why a real database rather than a mock or sqlite:
//
//   - Half of what this system relies on IS the database. Partial unique
//     indexes, CHECK constraints, enum types, `FOR UPDATE` row locks, triggers,
//     `ON CONFLICT DO NOTHING` — none of them exist in a mock, so a mocked test
//     of the allocation race or the one-open-request rule proves nothing about
//     the property it claims to test.
//   - The migrations are applied with golang-migrate, the same tool the
//     `migrate` compose service uses, against the same files. So "the schema the
//     tests ran against" and "the schema production gets" are the same artefact
//     rather than two things that agree until they don't.
//
// Container reuse: one container per test binary, created on first use. Each
// test then gets a clean set of rows via Truncate rather than a new container —
// a container per test would be correct and unbearably slow.
package testsupport

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/lib/pq" // database/sql driver golang-migrate needs
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	image    = "postgres:18" // must match compose.yaml
	database = "bbank_test"
	user     = "test"
	password = "test"
)

var (
	once      sync.Once
	sharedDSN string
	setupErr  error
	container *tcpostgres.PostgresContainer
)

// Pool returns a connection pool to a migrated, empty database.
//
// Skips the test when Docker is unavailable or `-short` is set, so
// `go test -short ./...` stays runnable on a machine without Docker. CI runs it
// without `-short`, which is where the gate actually bites.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test skipped in -short mode")
	}

	once.Do(func() { sharedDSN, setupErr = start() })
	if setupErr != nil {
		// A missing Docker daemon is an environment gap, not a failing test; a
		// broken migration is a real failure. Distinguishing them keeps a
		// laptop without Docker from looking like a red build.
		if os.Getenv("CI") == "" {
			t.Skipf("cannot start postgres (is Docker running?): %v", setupErr)
		}
		t.Fatalf("cannot start postgres: %v", setupErr)
	}

	pool, err := pgxpool.New(context.Background(), sharedDSN)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	Truncate(t, pool)
	return pool
}

func start() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	c, err := tcpostgres.Run(ctx, image,
		tcpostgres.WithDatabase(database),
		tcpostgres.WithUsername(user),
		tcpostgres.WithPassword(password),
		testcontainers.WithWaitStrategy(
			// Postgres in an init container reports ready, restarts, and reports
			// ready again. Waiting for two occurrences is the documented way to
			// avoid connecting to the one that is about to go away.
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(2*time.Minute),
		),
	)
	if err != nil {
		return "", fmt.Errorf("start container: %w", err)
	}
	container = c

	dsn, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return "", fmt.Errorf("connection string: %w", err)
	}
	if err := Migrate(dsn); err != nil {
		return "", err
	}
	return dsn, nil
}

// Migrate applies every migration, using the same tool and the same files as
// the `migrate` compose service.
func Migrate(dsn string) error {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("open for migrate: %w", err)
	}
	defer db.Close()

	driver, err := migratepg.WithInstance(db, &migratepg.Config{})
	if err != nil {
		return fmt.Errorf("migrate driver: %w", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://"+MigrationsDir(), "postgres", driver)
	if err != nil {
		return fmt.Errorf("migrate init: %w", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

// MigrationsDir resolves backend/migrations from this source file's location,
// so tests work regardless of which package directory `go test` runs in.
func MigrationsDir() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations")
}

// Truncate empties every table that tests write to, leaving the reference and
// seed data (policies, test_types, abo_compatibility, donation_centers) intact.
//
// Seed data is deliberately kept: it is part of the schema's meaning, and a test
// that had to insert its own blood-compatibility matrix would be asserting
// against its own fixture rather than against the one the application ships.
func Truncate(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	// Order does not matter — one TRUNCATE ... CASCADE handles the FK graph.
	_, err := pool.Exec(context.Background(), `
		TRUNCATE TABLE
			idempotency_keys, sessions, notifications,
			unit_allocations, issuances, blood_requests,
			test_results, unit_status_events, blood_units,
			donations, screenings, deferrals,
			appointments, donation_requests, donor_profiles, users
		RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

// Terminate stops the shared container. Call from TestMain when a package needs
// deterministic teardown; otherwise Ryuk reaps it.
func Terminate() {
	if container != nil {
		_ = container.Terminate(context.Background())
	}
}

// FreshDatabase creates an empty, UNMIGRATED database on the shared container
// and returns its DSN plus a migrate handle positioned at version 0.
//
// This exists for tests about the migration path itself, which need to stand at
// a specific version, insert data in the shape that version had, and then step
// forward. Pool() cannot serve them: it hands back a fully migrated schema,
// which is the end state those tests are trying to arrive at.
func FreshDatabase(t *testing.T) (dsn string, m *migrate.Migrate) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test skipped in -short mode")
	}
	once.Do(func() { sharedDSN, setupErr = start() })
	if setupErr != nil {
		if os.Getenv("CI") == "" {
			t.Skipf("cannot start postgres (is Docker running?): %v", setupErr)
		}
		t.Fatalf("cannot start postgres: %v", setupErr)
	}

	name := fmt.Sprintf("mig_%d", time.Now().UnixNano())
	admin, err := sql.Open("postgres", sharedDSN)
	if err != nil {
		t.Fatalf("open admin: %v", err)
	}
	defer admin.Close()
	if _, err := admin.Exec("CREATE DATABASE " + name); err != nil {
		t.Fatalf("create database: %v", err)
	}

	u, err := url.Parse(sharedDSN)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	u.Path = "/" + name
	dsn = u.String()

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open fresh db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	driver, err := migratepg.WithInstance(db, &migratepg.Config{})
	if err != nil {
		t.Fatalf("migrate driver: %v", err)
	}
	m, err = migrate.NewWithDatabaseInstance("file://"+MigrationsDir(), "postgres", driver)
	if err != nil {
		t.Fatalf("migrate init: %v", err)
	}
	return dsn, m
}
