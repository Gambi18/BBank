package testsupport

import (
	"context"
	"testing"

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
