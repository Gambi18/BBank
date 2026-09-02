package testsupport

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Fixtures build the minimum real rows a test needs.
//
// They insert through SQL rather than through the services on purpose: a test
// of the approve path should not depend on the create path being correct, or a
// single bug fails both and neither failure says which.

// CenterID is the seeded 'MAIN' centre, present from migration 000003.
func CenterID(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`SELECT id FROM donation_centers WHERE code = 'MAIN'`).Scan(&id); err != nil {
		t.Fatalf("seeded MAIN centre missing: %v", err)
	}
	return id
}

// SecondCenterID returns a centre that is not MAIN, for cross-centre scope tests.
func SecondCenterID(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`SELECT id FROM donation_centers WHERE code <> 'MAIN' ORDER BY id LIMIT 1`).Scan(&id); err != nil {
		t.Fatalf("no second centre seeded: %v", err)
	}
	return id
}

// NewDonor inserts a user + donor_profiles pair and returns the user id.
func NewDonor(t *testing.T, pool *pgxpool.Pool, email, name string) int64 {
	t.Helper()
	ctx := context.Background()
	var id int64
	err := pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, role, status)
		VALUES ($1, '$2b$10$abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ012', 'donor', 'active')
		RETURNING id`, email).Scan(&id)
	if err != nil {
		t.Fatalf("insert donor user: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO donor_profiles (user_id, full_name, date_of_birth, gender, contact_phone)
		VALUES ($1, $2, DATE '1990-01-01', 'undisclosed', '+237600000000')`, id, name); err != nil {
		t.Fatalf("insert donor profile: %v", err)
	}
	return id
}

// NewStaff inserts a staff user homed at centerID. `staff` requires a centre
// (migration 000015's role/centre CHECK), which is why this takes one.
func NewStaff(t *testing.T, pool *pgxpool.Pool, email string, centerID int64) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (email, password_hash, role, status, center_id)
		VALUES ($1, '$2b$10$abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ012', 'staff', 'active', $2)
		RETURNING id`, email, centerID).Scan(&id)
	if err != nil {
		t.Fatalf("insert staff: %v", err)
	}
	return id
}

func NewAdmin(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (email, password_hash, role, status)
		VALUES ($1, '$2b$10$abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ012', 'admin', 'active')
		RETURNING id`, email).Scan(&id)
	if err != nil {
		t.Fatalf("insert admin: %v", err)
	}
	return id
}

// NewPendingRequest inserts a pending donation request and returns its id.
func NewPendingRequest(t *testing.T, pool *pgxpool.Pool, donorID, centerID int64) int32 {
	t.Helper()
	var id int32
	err := pool.QueryRow(context.Background(), `
		INSERT INTO donation_requests (donor_id, center_id, preferred_date, status)
		VALUES ($1, $2, CURRENT_DATE + 7, 'pending')
		RETURNING id`, donorID, centerID).Scan(&id)
	if err != nil {
		t.Fatalf("insert pending request: %v", err)
	}
	return id
}

// CountRows is a small readability helper for assertions.
func CountRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	return n
}

// IsolatePolicies makes a test's changes to the `policies` table local to it.
//
// `Truncate` deliberately does NOT empty the reference tables — a test that had
// to insert its own blood-compatibility matrix would be asserting against its
// own fixture. That is right, and it means a test which EDITS reference data
// leaves it edited for every test that follows, in a package that shares one
// database.
//
// The failure is not theoretical: a test here deleted the `donor_age_years` row
// to prove a missing policy stops the decision, and every subsequent test in the
// package then found no age band — including tests about donors, which have
// nothing to do with policy. The symptom appeared in a different file from the
// cause, which is the worst shape a test failure can have.
//
// Call this first in any test that writes to `policies`.
func IsolatePolicies(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	type row struct {
		key, region, description string
		value                    []byte
		effectiveFrom            pgtype.Date
		effectiveTo              pgtype.Date
	}
	rows, err := pool.Query(ctx, `
		SELECT key, region, COALESCE(description, ''), value, effective_from, effective_to
		FROM policies ORDER BY id`)
	if err != nil {
		t.Fatalf("snapshot policies: %v", err)
	}
	var saved []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.key, &r.region, &r.description, &r.value, &r.effectiveFrom, &r.effectiveTo); err != nil {
			rows.Close()
			t.Fatalf("scan policy: %v", err)
		}
		saved = append(saved, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("read policies: %v", err)
	}

	t.Cleanup(func() {
		ctx := context.Background()
		if _, err := pool.Exec(ctx, `DELETE FROM policies`); err != nil {
			t.Errorf("restore policies (delete): %v", err)
			return
		}
		for _, r := range saved {
			if _, err := pool.Exec(ctx, `
				INSERT INTO policies (key, value, region, description, effective_from, effective_to)
				VALUES ($1, $2, $3, $4, $5, $6)`,
				r.key, r.value, r.region, r.description, r.effectiveFrom, r.effectiveTo); err != nil {
				t.Errorf("restore policy %s: %v", r.key, err)
				return
			}
		}
	})
}

// NewCompletedDonation records a donation that actually happened: a completed
// appointment and the collection it produced.
//
// Both rows, always. `donor_eligibility` and the eligibility facts query each
// join `donations` to a **completed** appointment, because that join is what
// makes eligibility depend on donations the centre observed rather than on a
// date the donor typed — schema defect `D4`, the reason `donors.last_donation`
// no longer exists. A fixture that inserted only the donation would be invisible
// to both, and a test built on it would prove the opposite of what it claims.
//
// The phlebotomist is the donor's own user row. That is not clinically sensible
// and does not need to be: `donations.phlebotomist_id` is NOT NULL, no rule
// under test reads it, and inventing a second user would add a fixture nothing
// asserts on.
func NewCompletedDonation(t *testing.T, pool *pgxpool.Pool, donorID, centerID int64, procedure string, collectedAt time.Time) int64 {
	t.Helper()
	ctx := context.Background()

	var appointmentID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO appointments (donor_id, center_id, scheduled_at, procedure, status, completed_at)
		VALUES ($1, $2, $3, $4::donation_procedure, 'completed', $3)
		RETURNING id`, donorID, centerID, collectedAt, procedure).Scan(&appointmentID); err != nil {
		t.Fatalf("insert completed appointment: %v", err)
	}

	var donationID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO donations (appointment_id, donor_id, center_id, procedure, collected_at,
		                       volume_ml, bag_lot_number, phlebotomist_id)
		VALUES ($1, $2, $3, $4::donation_procedure, $5, 450, 'LOT-TEST-0001', $2)
		RETURNING id`, appointmentID, donorID, centerID, procedure, collectedAt).Scan(&donationID); err != nil {
		t.Fatalf("insert donation: %v", err)
	}
	return donationID
}
