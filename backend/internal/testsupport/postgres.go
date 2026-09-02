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

	// Before the truncate, never after: the truncate is what empties `policies`,
	// and the tests that follow are what disturb `donation_centers`.
	captureReferenceData(pool)
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
//
// **`policies` needs explicit restoring, and this is not a nicety.**
// `policies.created_by` references `users`, and `TRUNCATE users ... CASCADE`
// propagates to every table with a foreign key pointing at it — cascade truncation
// ignores `ON DELETE SET NULL`, which is what that column declares. So the
// statement below silently emptied the clinical policy table on every single
// test, and this comment claimed the opposite for as long as it existed.
//
// Nothing caught it because nothing read policy until `WI-25`: the seed
// cross-check tests parse the migration FILE, and the `donor_eligibility` view
// COALESCEs a hardcoded fallback for each threshold, so it kept answering with
// 56 days and 18 years from an empty table. The first code to actually require a
// policy row found none.
//
// `test_types`, `abo_compatibility` and `donation_centers` have no foreign key to
// `users` and genuinely do survive, which is why the claim looked true.
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
	restoreReferenceData(t, pool)
}

// The seeded reference rows, captured once from the freshly migrated database
// before any test can disturb them.
//
// Captured rather than hardcoded: a copy of the values here would be a second
// source of truth, drifting from the migrations in exactly the way
// `TestSeededPolicyValuesMatchTheMigration` exists to prevent.
//
// Two tables need this, for two different reasons.
//
//   - `policies` is EMPTIED by the truncate below, because `policies.created_by`
//     references `users` and `TRUNCATE ... CASCADE` propagates across foreign
//     keys regardless of the `ON DELETE SET NULL` that column declares.
//   - `donation_centers` survives the truncate, and is disturbed by tests
//     instead: `WI-24` gave centres a capacity, opening hours and an active
//     flag, and a test that closes a centre or narrows a slot leaves it that way
//     for everything that runs afterwards. That already happened — a
//     deactivation test made three unrelated tests fail with "that centre is not
//     currently taking bookings", in a different file from the cause.
//
// Restoring both here rather than asking each test to remember is the only
// version of this that stays true: the test that forgets is the one that breaks
// somebody else.
var (
	referenceOnce sync.Once
	policySeed    []policyRow
	centerSeed    []centerRow
	referenceErr  error
)

type policyRow struct {
	key, region, description string
	value                    []byte
	effectiveFrom            time.Time
	effectiveTo              *time.Time
}

type centerRow struct {
	code, name, addressLine, city, region, timezone string
	phone, email                                    *string
	capacityPerSlot, slotMinutes                    int16
	openingHours                                    []byte
	isActive                                        bool
}

func restoreReferenceData(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	if referenceErr != nil {
		t.Fatalf("capture the reference seed: %v", referenceErr)
	}
	// A nil seed with no error means the capture never ran — `Truncate` is
	// exported, so a caller who obtained a pool some other way can reach here
	// first. Failing loudly matters: returning quietly would leave the reference
	// tables in whatever state the last test left them, which is the silent
	// cross-test coupling this exists to prevent.
	if policySeed == nil || centerSeed == nil {
		t.Fatal("the reference seed was never captured — call testsupport.Pool(t) rather than Truncate directly")
	}

	// Both are rebuilt from scratch rather than repaired, so a test that ADDED a
	// row is undone as well as one that edited a row. Identity columns reassign
	// ids; nothing depends on a specific one, because the fixtures look centres
	// up by code.
	if _, err := pool.Exec(ctx, `DELETE FROM policies`); err != nil {
		t.Fatalf("clear policies: %v", err)
	}
	for _, r := range policySeed {
		if _, err := pool.Exec(ctx, `
			INSERT INTO policies (key, value, region, description, effective_from, effective_to)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			r.key, r.value, r.region, r.description, r.effectiveFrom, r.effectiveTo); err != nil {
			t.Fatalf("restore policy %s: %v", r.key, err)
		}
	}

	// Centres are repaired in place and matched by code, NOT deleted and
	// reinserted. `storage_locations.center_id` is `ON DELETE RESTRICT` and
	// `storage_locations` is itself seed data this does not truncate, so a
	// wholesale delete fails on the foreign key. Updating also keeps the ids
	// stable, which costs nothing and surprises nobody.
	codes := make([]string, 0, len(centerSeed))
	for _, c := range centerSeed {
		codes = append(codes, c.code)
		if _, err := pool.Exec(ctx, `
			UPDATE donation_centers
			   SET name = $2, address_line = $3, city = $4, region = $5, phone = $6, email = $7,
			       capacity_per_slot = $8, slot_minutes = $9, opening_hours = $10,
			       timezone = $11, is_active = $12
			 WHERE code = $1`,
			c.code, c.name, c.addressLine, c.city, c.region, c.phone, c.email,
			c.capacityPerSlot, c.slotMinutes, c.openingHours, c.timezone, c.isActive); err != nil {
			t.Fatalf("restore centre %s: %v", c.code, err)
		}
	}
	// A centre a test CREATED is removed, so the next test sees the seeded
	// estate and nothing else. These are safe to delete: nothing seeded points
	// at them, and everything a test attached has just been truncated.
	if _, err := pool.Exec(ctx,
		`DELETE FROM donation_centers WHERE code <> ALL($1::text[])`, codes); err != nil {
		t.Fatalf("remove test-created centres: %v", err)
	}
}

// captureReferenceData reads the seeded reference rows once, on the first Pool()
// of a test binary — the only moment they are guaranteed untouched, because the
// Truncate immediately after it is what disturbs them.
func captureReferenceData(pool *pgxpool.Pool) {
	referenceOnce.Do(func() {
		ctx := context.Background()
		policySeed, referenceErr = readPolicies(ctx, pool)
		if referenceErr != nil {
			return
		}
		centerSeed, referenceErr = readCenters(ctx, pool)
	})
}

func readCenters(ctx context.Context, pool *pgxpool.Pool) ([]centerRow, error) {
	rows, err := pool.Query(ctx, `
		SELECT code, name, address_line, city, region, phone, email,
		       capacity_per_slot, slot_minutes, opening_hours, timezone, is_active
		FROM donation_centers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []centerRow
	for rows.Next() {
		var c centerRow
		if err := rows.Scan(&c.code, &c.name, &c.addressLine, &c.city, &c.region, &c.phone, &c.email,
			&c.capacityPerSlot, &c.slotMinutes, &c.openingHours, &c.timezone, &c.isActive); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("the freshly migrated database has no donation centres — migration 000012 did not seed")
	}
	return out, nil
}

func readPolicies(ctx context.Context, pool *pgxpool.Pool) ([]policyRow, error) {
	rows, err := pool.Query(ctx, `
		SELECT key, region, COALESCE(description, ''), value, effective_from, effective_to
		FROM policies ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []policyRow
	for rows.Next() {
		var r policyRow
		if err := rows.Scan(&r.key, &r.region, &r.description, &r.value, &r.effectiveFrom, &r.effectiveTo); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("the freshly migrated database has no policy rows — migration 000012 did not seed")
	}
	return out, nil
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
