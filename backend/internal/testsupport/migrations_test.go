package testsupport_test

import (
	"context"
	"testing"

	"bbank/internal/testsupport"

	"github.com/golang-migrate/migrate/v4"
	"github.com/jackc/pgx/v5"
)

// Migrations must round-trip. A down migration that has never been run is not a
// rollback plan, it is a file.
//
// This is the same up -> down -> up the CI job runs, kept here as well so it
// fails during `go test` rather than only in a separate workflow step — the
// person who breaks a down migration is running tests, not reading CI.
func TestMigrationsRoundTrip(t *testing.T) {
	_, m := testsupport.FreshDatabase(t)

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("up: %v", err)
	}
	top, dirty, err := m.Version()
	if err != nil {
		t.Fatalf("version after up: %v", err)
	}
	if dirty {
		t.Fatal("schema is dirty after up")
	}

	if err := m.Down(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("down -all: %v", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("second up: %v", err)
	}
	again, dirty, err := m.Version()
	if err != nil {
		t.Fatalf("version after second up: %v", err)
	}
	if dirty {
		t.Fatal("schema is dirty after the second up")
	}
	if again != top {
		t.Fatalf("second up reached version %d, first reached %d", again, top)
	}
}

// The migration this project's data actually depends on, tested against the
// shape production was in — **including the damage the old `confirm` did**.
//
// `POST /requests/{id}/confirm` used to DELETE the request row after creating
// the appointment. `appointments.request_id` in the baseline schema has no
// foreign key, so those deletions left appointments pointing at requests that
// no longer exist. Any migration that assumed referential integrity would
// either fail or silently drop those appointments — and an appointment is a
// person with a date, so dropping one is not a rounding error.
//
// The fixture below reproduces exactly that: one intact request, and one
// appointment orphaned by a delete that already happened.
func TestLegacyRequestsMigrationPreservesOrphanedAppointments(t *testing.T) {
	dsn, m := testsupport.FreshDatabase(t)

	// Stand at the baseline: the schema main.go used to create at boot.
	if err := m.Migrate(0); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate to baseline: %v", err)
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	// Legacy rows, in the legacy shape.
	var donorID int
	if err := conn.QueryRow(ctx, `
		INSERT INTO donors (full_name, email, dob, gender, blood_group, rhesus, contact, address, password, last_donation)
		VALUES ('Legacy Donor', 'legacy@example.test', DATE '1988-03-02', 'female', 'O', '+',
		        '+237600111222', 'Douala', '$2b$10$abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ012', DATE '2026-01-10')
		RETURNING id`).Scan(&donorID); err != nil {
		t.Fatalf("insert legacy donor: %v", err)
	}

	// A request that still exists.
	var liveReqID int
	if err := conn.QueryRow(ctx, `
		INSERT INTO requests (donor_id, donor_name, last_donation)
		VALUES ($1, 'Legacy Donor', DATE '2026-01-10') RETURNING id`, donorID).Scan(&liveReqID); err != nil {
		t.Fatalf("insert legacy request: %v", err)
	}

	// An appointment whose request was deleted by the old confirm. 9999 is a
	// request id that has never existed — which is precisely the state the
	// DELETE left behind.
	const orphanRequestID = 9999
	var orphanApptID int
	if err := conn.QueryRow(ctx, `
		INSERT INTO appointments (request_id, donor_id, donor_name, appointment_date)
		VALUES ($1, $2, 'Legacy Donor', DATE '2026-02-20') RETURNING id`,
		orphanRequestID, donorID).Scan(&orphanApptID); err != nil {
		t.Fatalf("insert orphaned appointment: %v", err)
	}

	// A healthy appointment, linked to the request that survived.
	var goodApptID int
	if err := conn.QueryRow(ctx, `
		INSERT INTO appointments (request_id, donor_id, donor_name, appointment_date)
		VALUES ($1, $2, 'Legacy Donor', DATE '2026-03-05') RETURNING id`,
		liveReqID, donorID).Scan(&goodApptID); err != nil {
		t.Fatalf("insert healthy appointment: %v", err)
	}

	// Now run the whole migration path over that data.
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrating legacy data forward failed: %v", err)
	}

	// 1. The donor survived into the new identity model with the SAME id — the
	//    critical constraint of §11.3, because every FK in the old data uses it.
	var email string
	if err := conn.QueryRow(ctx,
		`SELECT email::text FROM users WHERE id = $1`, donorID).Scan(&email); err != nil {
		t.Fatalf("donor %d did not survive into users: %v", donorID, err)
	}
	if email != "legacy@example.test" {
		t.Errorf("users.email = %q", email)
	}
	if n := countConn(t, conn, `SELECT count(*) FROM donor_profiles WHERE user_id = $1`, donorID); n != 1 {
		t.Errorf("got %d donor_profiles rows for the migrated donor, want 1", n)
	}

	// 2. The surviving request became a donation_request, keeping its id.
	if n := countConn(t, conn, `SELECT count(*) FROM donation_requests WHERE id = $1`, liveReqID); n != 1 {
		t.Errorf("legacy request %d did not survive the rename", liveReqID)
	}

	// 3. **Both** appointments still exist. This is the assertion that matters:
	//    the orphan must not have been deleted to satisfy a new foreign key.
	if n := countConn(t, conn, `SELECT count(*) FROM appointments`); n != 2 {
		t.Fatalf("got %d appointments after migrating, want 2 — an appointment was dropped", n)
	}

	// 4. The orphan is retained with a NULL link rather than a dangling one, so
	//    the FK can exist without lying about provenance.
	var linked *int32
	if err := conn.QueryRow(ctx,
		`SELECT donation_request_id FROM appointments WHERE id = $1`, orphanApptID).Scan(&linked); err != nil {
		t.Fatalf("orphaned appointment %d is gone: %v", orphanApptID, err)
	}
	if linked != nil {
		t.Errorf("orphaned appointment kept a dangling link to request %d", *linked)
	}

	// 5. The healthy one kept its real link.
	if err := conn.QueryRow(ctx,
		`SELECT donation_request_id FROM appointments WHERE id = $1`, goodApptID).Scan(&linked); err != nil {
		t.Fatalf("healthy appointment %d is gone: %v", goodApptID, err)
	}
	if linked == nil || int(*linked) != liveReqID {
		t.Errorf("healthy appointment lost its link to request %d", liveReqID)
	}

	// 6. The loss was RECORDED, not silently absorbed. A quarantine table nobody
	//    writes to is the same as no quarantine table.
	if n := countConn(t, conn,
		`SELECT count(*) FROM migration_rejects WHERE source_table = 'appointments' AND source_id = $1`,
		orphanApptID); n != 1 {
		t.Errorf("the orphaned appointment was not recorded in migration_rejects")
	}
}

func countConn(t *testing.T, conn *pgx.Conn, query string, args ...any) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}
