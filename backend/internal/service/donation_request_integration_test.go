package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"bbank/internal/domain"
	"bbank/internal/service"
	"bbank/internal/store"
	"bbank/internal/testsupport"

	"github.com/jackc/pgx/v5/pgxpool"
)

// allow is the "any row is yours" permit, for tests that are not about scope.
func allow(int64, int64) bool { return true }

// The acceptance criterion for FR-09, asserted against the database rather than
// against a response body: approving leaves the request PRESENT as 'approved'
// with a linked appointment. The original `confirm` deleted the row, which is
// what destroyed the link back to who asked and when.
func TestApproveKeepsTheRequestAndLinksTheAppointment(t *testing.T) {
	pool := testsupport.Pool(t)
	q := store.New(pool)
	svc := service.NewDonationRequestService(pool, q)

	center := testsupport.CenterID(t, pool)
	donorID := testsupport.NewDonor(t, pool, "approve@example.test", "Approve Donor")
	staffID := testsupport.NewStaff(t, pool, "staff@example.test", center)
	reqID := testsupport.NewPendingRequest(t, pool, donorID, center)

	appt, err := svc.Approve(context.Background(), reqID, staffID, time.Now().AddDate(0, 0, 14), allow)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}

	row, err := q.GetDonationRequest(context.Background(), reqID)
	if err != nil {
		t.Fatalf("the request row is gone after approval: %v", err)
	}
	if string(row.Status) != "approved" {
		t.Errorf("status = %q, want approved", row.Status)
	}
	if !row.ReviewedAt.Valid {
		t.Error("reviewed_at was not stamped")
	}
	if appt.DonationRequestID == nil || *appt.DonationRequestID != reqID {
		t.Errorf("appointment is not linked back to request %d", reqID)
	}
	if n := testsupport.CountRows(t, pool,
		`SELECT count(*) FROM appointments WHERE donation_request_id = $1`, reqID); n != 1 {
		t.Errorf("got %d appointments for the request, want exactly 1", n)
	}
}

// The reason the row is locked with FOR UPDATE.
//
// Without the lock both goroutines read 'pending', both pass the transition
// check, and the second INSERT dies on the UNIQUE over
// appointments.donation_request_id — a 500 for what is really a 409. This runs
// real concurrent transactions against real Postgres, which is the only way the
// claim can be tested at all: no mock has row locks.
func TestConcurrentApprovalsCreateExactlyOneAppointment(t *testing.T) {
	pool := testsupport.Pool(t)
	q := store.New(pool)
	svc := service.NewDonationRequestService(pool, q)

	center := testsupport.CenterID(t, pool)
	donorID := testsupport.NewDonor(t, pool, "race@example.test", "Race Donor")
	staffID := testsupport.NewStaff(t, pool, "racestaff@example.test", center)
	reqID := testsupport.NewPendingRequest(t, pool, donorID, center)

	const attempts = 8
	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		succeeded int
		conflicts int
		other     []error
	)
	start := make(chan struct{})

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // release them together, to actually contend
			_, err := svc.Approve(context.Background(), reqID, staffID, time.Now().AddDate(0, 0, 14), allow)

			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				succeeded++
			case errors.Is(err, service.ErrConflict):
				conflicts++
			default:
				other = append(other, err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if len(other) > 0 {
		t.Fatalf("unexpected errors (a constraint violation here means the lock is not doing its job): %v", other)
	}
	if succeeded != 1 {
		t.Errorf("%d approvals succeeded, want exactly 1", succeeded)
	}
	if conflicts != attempts-1 {
		t.Errorf("%d conflicts, want %d — every loser must get a clean 409", conflicts, attempts-1)
	}
	if n := testsupport.CountRows(t, pool,
		`SELECT count(*) FROM appointments WHERE donation_request_id = $1`, reqID); n != 1 {
		t.Fatalf("got %d appointments, want exactly 1 — the donor was double-booked", n)
	}
}

// The partial unique index `donation_requests_one_open_per_donor`. The service
// checks first for a readable error, but the index is what actually holds, so
// the test drives the service concurrently rather than sequentially.
func TestOnlyOneOpenRequestPerDonor(t *testing.T) {
	pool := testsupport.Pool(t)
	q := store.New(pool)
	svc := service.NewDonationRequestService(pool, q)

	center := testsupport.CenterID(t, pool)
	donorID := testsupport.NewDonor(t, pool, "double@example.test", "Double Booker")

	const attempts = 6
	var wg sync.WaitGroup
	var mu sync.Mutex
	created, conflicts := 0, 0
	other := []error{}
	start := make(chan struct{})

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := svc.Create(context.Background(), service.CreateRequestParams{DonorID: donorID, CenterID: &center})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				created++
			case errors.Is(err, service.ErrConflict):
				conflicts++
			default:
				other = append(other, err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if len(other) > 0 {
		t.Fatalf("a duplicate request surfaced as something other than a conflict: %v", other)
	}
	if created != 1 {
		t.Errorf("%d requests created, want 1", created)
	}
	if n := testsupport.CountRows(t, pool,
		`SELECT count(*) FROM donation_requests WHERE donor_id = $1 AND status = 'pending'`, donorID); n != 1 {
		t.Errorf("%d pending requests for one donor, want 1", n)
	}
}

// Decided states are terminal. Re-opening would let an appointment exist
// against a row that later reads 'rejected'.
func TestDecidedRequestsCannotBeDecidedAgain(t *testing.T) {
	pool := testsupport.Pool(t)
	svc := service.NewDonationRequestService(pool, store.New(pool))
	ctx := context.Background()

	center := testsupport.CenterID(t, pool)
	staffID := testsupport.NewStaff(t, pool, "terminal@example.test", center)

	t.Run("approved cannot be rejected", func(t *testing.T) {
		donorID := testsupport.NewDonor(t, pool, "t1@example.test", "T One")
		reqID := testsupport.NewPendingRequest(t, pool, donorID, center)
		if _, err := svc.Approve(ctx, reqID, staffID, time.Now().AddDate(0, 0, 7), allow); err != nil {
			t.Fatalf("setup approve: %v", err)
		}
		err := svc.Reject(ctx, reqID, staffID, domain.ReasonCenterClosed, "", allow)
		if !errors.Is(err, service.ErrConflict) {
			t.Fatalf("rejecting an approved request = %v, want ErrConflict", err)
		}
	})

	t.Run("rejected cannot be approved", func(t *testing.T) {
		donorID := testsupport.NewDonor(t, pool, "t2@example.test", "T Two")
		reqID := testsupport.NewPendingRequest(t, pool, donorID, center)
		if err := svc.Reject(ctx, reqID, staffID, domain.ReasonCenterAtCapacity, "", allow); err != nil {
			t.Fatalf("setup reject: %v", err)
		}
		_, err := svc.Approve(ctx, reqID, staffID, time.Now().AddDate(0, 0, 7), allow)
		if !errors.Is(err, service.ErrConflict) {
			t.Fatalf("approving a rejected request = %v, want ErrConflict", err)
		}
		if n := testsupport.CountRows(t, pool,
			`SELECT count(*) FROM appointments WHERE donation_request_id = $1`, reqID); n != 0 {
			t.Fatalf("a rejected request produced %d appointments", n)
		}
	})

	t.Run("cancelled cannot be approved", func(t *testing.T) {
		donorID := testsupport.NewDonor(t, pool, "t3@example.test", "T Three")
		reqID := testsupport.NewPendingRequest(t, pool, donorID, center)
		if err := svc.Cancel(ctx, reqID, donorID, allow); err != nil {
			t.Fatalf("setup cancel: %v", err)
		}
		if _, err := svc.Approve(ctx, reqID, staffID, time.Now().AddDate(0, 0, 7), allow); !errors.Is(err, service.ErrConflict) {
			t.Fatalf("approving a cancelled request = %v, want ErrConflict", err)
		}
	})
}

// FR-09 end to end: the coded reason reaches the column, and the schema's CHECK
// (status <> 'rejected' OR rejection_reason IS NOT NULL) is never reached
// because the domain refuses first.
func TestRejectionStoresACodedReason(t *testing.T) {
	pool := testsupport.Pool(t)
	svc := service.NewDonationRequestService(pool, store.New(pool))
	ctx := context.Background()

	center := testsupport.CenterID(t, pool)
	staffID := testsupport.NewStaff(t, pool, "rej@example.test", center)
	donorID := testsupport.NewDonor(t, pool, "rejd@example.test", "Rejected Booking")
	reqID := testsupport.NewPendingRequest(t, pool, donorID, center)

	if err := svc.Reject(ctx, reqID, staffID, "invented_reason", "x", allow); !errors.Is(err, service.ErrInvalid) {
		t.Fatalf("an unlisted reason = %v, want ErrInvalid", err)
	}
	if err := svc.Reject(ctx, reqID, staffID, domain.ReasonOther, "  ", allow); !errors.Is(err, service.ErrInvalid) {
		t.Fatalf("'other' with a blank note = %v, want ErrInvalid", err)
	}
	// Neither refusal may have written anything.
	if n := testsupport.CountRows(t, pool,
		`SELECT count(*) FROM donation_requests WHERE id = $1 AND status = 'pending'`, reqID); n != 1 {
		t.Fatal("a refused rejection still changed the row")
	}

	if err := svc.Reject(ctx, reqID, staffID, domain.ReasonCenterClosed, "closed for stocktake", allow); err != nil {
		t.Fatalf("valid rejection: %v", err)
	}
	var status, reason, note string
	if err := pool.QueryRow(ctx,
		`SELECT status, rejection_reason, coalesce(notes,'') FROM donation_requests WHERE id = $1`, reqID).
		Scan(&status, &reason, &note); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if status != "rejected" || reason != string(domain.ReasonCenterClosed) || note != "closed for stocktake" {
		t.Errorf("stored (%q, %q, %q)", status, reason, note)
	}
}

// Ownership is evaluated on the locked row inside the transaction, and a
// violation is ErrNotFound — never a 403, which would confirm the row exists.
func TestApproveOutsideScopeIsNotFound(t *testing.T) {
	pool := testsupport.Pool(t)
	svc := service.NewDonationRequestService(pool, store.New(pool))

	center := testsupport.CenterID(t, pool)
	other := testsupport.SecondCenterID(t, pool)
	donorID := testsupport.NewDonor(t, pool, "scope@example.test", "Scoped Donor")
	staffID := testsupport.NewStaff(t, pool, "scopestaff@example.test", center)
	reqID := testsupport.NewPendingRequest(t, pool, donorID, other)

	deny := func(int64, int64) bool { return false }
	_, err := svc.Approve(context.Background(), reqID, staffID, time.Now().AddDate(0, 0, 7), deny)
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("approving out of scope = %v, want ErrNotFound (403 would confirm it exists)", err)
	}
	if n := testsupport.CountRows(t, pool,
		`SELECT count(*) FROM appointments WHERE donation_request_id = $1`, reqID); n != 0 {
		t.Fatal("an out-of-scope approval still created an appointment")
	}
}

// The scope filter, exercised against real rows: a donor sees only their own,
// staff only their centre's, admin everything.
func TestListIsNarrowedByScope(t *testing.T) {
	pool := testsupport.Pool(t)
	svc := service.NewDonationRequestService(pool, store.New(pool))
	ctx := context.Background()

	main := testsupport.CenterID(t, pool)
	other := testsupport.SecondCenterID(t, pool)
	donorA := testsupport.NewDonor(t, pool, "a@example.test", "Donor A")
	donorB := testsupport.NewDonor(t, pool, "b@example.test", "Donor B")
	testsupport.NewPendingRequest(t, pool, donorA, main)
	testsupport.NewPendingRequest(t, pool, donorB, other)

	count := func(s service.Scope) int64 {
		_, total, err := svc.List(ctx, service.ListRequestParams{Scope: s, Limit: 25})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		return total
	}

	if got := count(service.Scope{}); got != 2 {
		t.Errorf("admin scope saw %d, want 2", got)
	}
	if got := count(service.Scope{OwnerID: &donorA}); got != 1 {
		t.Errorf("donor A saw %d of their own, want 1", got)
	}
	if got := count(service.Scope{CenterID: &main}); got != 1 {
		t.Errorf("centre-scoped staff saw %d, want 1", got)
	}
}

// Appointments read through the same scope machinery, and `?donor_id=` is a
// separate field from the token-derived owner so it can only ever narrow.
func TestAppointmentListScopeAndFilter(t *testing.T) {
	pool := testsupport.Pool(t)
	q := store.New(pool)
	reqSvc := service.NewDonationRequestService(pool, q)
	apptSvc := service.NewAppointmentService(pool, q)
	ctx := context.Background()

	center := testsupport.CenterID(t, pool)
	staffID := testsupport.NewStaff(t, pool, "apptstaff@example.test", center)
	donorA := testsupport.NewDonor(t, pool, "apptA@example.test", "Appt A")
	donorB := testsupport.NewDonor(t, pool, "apptB@example.test", "Appt B")

	for _, d := range []int64{donorA, donorB} {
		reqID := testsupport.NewPendingRequest(t, pool, d, center)
		if _, err := reqSvc.Approve(ctx, reqID, staffID, time.Now().AddDate(0, 0, 10), allow); err != nil {
			t.Fatalf("approve for donor %d: %v", d, err)
		}
	}

	count := func(p service.ListAppointmentParams) int64 {
		p.Limit = 25
		_, total, err := apptSvc.List(ctx, p)
		if err != nil {
			t.Fatalf("list appointments: %v", err)
		}
		return total
	}

	if got := count(service.ListAppointmentParams{}); got != 2 {
		t.Errorf("admin scope saw %d appointments, want 2", got)
	}
	if got := count(service.ListAppointmentParams{Scope: service.Scope{OwnerID: &donorA}}); got != 1 {
		t.Errorf("donor A saw %d, want 1", got)
	}
	// The filter narrows within a wider scope...
	if got := count(service.ListAppointmentParams{DonorFilter: &donorB}); got != 1 {
		t.Errorf("admin filtering to donor B saw %d, want 1", got)
	}
	// ...and cannot widen a narrow one: donor A asking for donor B gets nothing,
	// because scope and filter are ANDed rather than one replacing the other.
	if got := count(service.ListAppointmentParams{
		Scope: service.Scope{OwnerID: &donorA}, DonorFilter: &donorB,
	}); got != 0 {
		t.Errorf("donor A filtering to donor B saw %d rows, want 0 — a filter must never widen a scope", got)
	}
}

func TestGetAppointmentAndRequestNotFound(t *testing.T) {
	pool := testsupport.Pool(t)
	q := store.New(pool)

	if _, err := service.NewAppointmentService(pool, q).Get(context.Background(), 999999); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("missing appointment = %v, want ErrNotFound", err)
	}
	if _, err := service.NewDonationRequestService(pool, q).Get(context.Background(), 999999); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("missing request = %v, want ErrNotFound", err)
	}
}

// FR-13: the sweep is safe to run twice, and creates NO deferral.
//
// Recording a missed appointment as a clinical deferral would put a mark on a
// donor's record that no clinician made — and `donor_eligibility` reads
// deferrals, so it would then block their next booking.
func TestNoShowSweepIsIdempotentAndCreatesNoDeferral(t *testing.T) {
	pool := testsupport.Pool(t)
	q := store.New(pool)
	svc := service.NewAppointmentService(pool, q)
	ctx := context.Background()

	center := testsupport.CenterID(t, pool)
	missed := testsupport.NewDonor(t, pool, "noshow@example.test", "No Show")
	upcoming := testsupport.NewDonor(t, pool, "upcoming@example.test", "Still Coming")
	recent := testsupport.NewDonor(t, pool, "recent@example.test", "Just Late")

	// Long past: should be swept.
	mkAppt(t, pool, missed, center, "-3 days", "scheduled")
	// In the future: must not be touched.
	mkAppt(t, pool, upcoming, center, "3 days", "scheduled")
	// Past, but inside the grace window: a donor stuck in traffic is a donation,
	// not an absence.
	mkAppt(t, pool, recent, center, "-1 hour", "scheduled")

	n, err := svc.SweepNoShows(ctx)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if n != 1 {
		t.Fatalf("swept %d rows, want 1", n)
	}

	// Idempotent: a second run finds nothing left to do.
	again, err := svc.SweepNoShows(ctx)
	if err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	if again != 0 {
		t.Errorf("the second sweep marked %d more rows; it is not idempotent", again)
	}

	if got := testsupport.CountRows(t, pool,
		`SELECT count(*) FROM appointments WHERE donor_id = $1 AND status = 'no_show'`, missed); got != 1 {
		t.Error("the missed appointment was not marked no_show")
	}
	if got := testsupport.CountRows(t, pool,
		`SELECT count(*) FROM appointments WHERE donor_id IN ($1,$2) AND status = 'scheduled'`, upcoming, recent); got != 2 {
		t.Error("the sweep touched a future or within-grace appointment")
	}
	// The assertion that matters clinically.
	if got := testsupport.CountRows(t, pool, `SELECT count(*) FROM deferrals`); got != 0 {
		t.Fatalf("the sweep created %d deferrals; missing an appointment is administrative, not clinical", got)
	}
}

// FR-11: cancelling frees the slot, and only before the appointment starts.
func TestCancelAndRescheduleBoundaries(t *testing.T) {
	pool := testsupport.Pool(t)
	svc := service.NewAppointmentService(pool, store.New(pool))
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)

	t.Run("cancel a future appointment", func(t *testing.T) {
		d := testsupport.NewDonor(t, pool, "cancelme@example.test", "Cancel Me")
		id := mkAppt(t, pool, d, center, "5 days", "scheduled")
		if err := svc.Cancel(ctx, id, "changed my mind", allow); err != nil {
			t.Fatalf("cancel: %v", err)
		}
		var status, reason string
		if err := pool.QueryRow(ctx,
			`SELECT status::text, coalesce(cancellation_reason,'') FROM appointments WHERE id = $1`, id).
			Scan(&status, &reason); err != nil {
			t.Fatalf("read back: %v", err)
		}
		if status != "cancelled" || reason != "changed my mind" {
			t.Errorf("got (%s, %q)", status, reason)
		}
	})

	t.Run("cancelling twice is a conflict", func(t *testing.T) {
		d := testsupport.NewDonor(t, pool, "twice@example.test", "Twice")
		id := mkAppt(t, pool, d, center, "5 days", "scheduled")
		if err := svc.Cancel(ctx, id, "", allow); err != nil {
			t.Fatalf("first cancel: %v", err)
		}
		if err := svc.Cancel(ctx, id, "", allow); !errors.Is(err, service.ErrConflict) {
			t.Errorf("second cancel = %v, want ErrConflict", err)
		}
	})

	t.Run("a past appointment cannot be cancelled", func(t *testing.T) {
		d := testsupport.NewDonor(t, pool, "gone@example.test", "Already Gone")
		id := mkAppt(t, pool, d, center, "-2 days", "scheduled")
		if err := svc.Cancel(ctx, id, "", allow); !errors.Is(err, service.ErrConflict) {
			t.Errorf("cancelling a past appointment = %v, want ErrConflict", err)
		}
	})

	t.Run("reschedule into the future", func(t *testing.T) {
		d := testsupport.NewDonor(t, pool, "moveme@example.test", "Move Me")
		id := mkAppt(t, pool, d, center, "5 days", "scheduled")
		to := time.Now().Add(10 * 24 * time.Hour).Truncate(time.Second)
		if err := svc.Reschedule(ctx, id, to, allow); err != nil {
			t.Fatalf("reschedule: %v", err)
		}
		var at time.Time
		if err := pool.QueryRow(ctx, `SELECT scheduled_at FROM appointments WHERE id = $1`, id).Scan(&at); err != nil {
			t.Fatalf("read back: %v", err)
		}
		if !at.Equal(to) {
			t.Errorf("scheduled_at = %v, want %v", at, to)
		}
	})

	t.Run("reschedule into the past is refused", func(t *testing.T) {
		d := testsupport.NewDonor(t, pool, "backwards@example.test", "Backwards")
		id := mkAppt(t, pool, d, center, "5 days", "scheduled")
		if err := svc.Reschedule(ctx, id, time.Now().Add(-time.Hour), allow); !errors.Is(err, service.ErrInvalid) {
			t.Errorf("rescheduling into the past = %v, want ErrInvalid", err)
		}
	})

	t.Run("a checked-in appointment cannot be moved", func(t *testing.T) {
		d := testsupport.NewDonor(t, pool, "inchair@example.test", "In The Chair")
		id := mkAppt(t, pool, d, center, "1 hour", "checked_in")
		if err := svc.Reschedule(ctx, id, time.Now().Add(72*time.Hour), allow); !errors.Is(err, service.ErrConflict) {
			t.Errorf("rescheduling a checked-in appointment = %v, want ErrConflict", err)
		}
		if err := svc.Cancel(ctx, id, "", allow); !errors.Is(err, service.ErrConflict) {
			t.Errorf("cancelling a checked-in appointment = %v, want ErrConflict", err)
		}
	})

	t.Run("out of scope is not found", func(t *testing.T) {
		d := testsupport.NewDonor(t, pool, "notyours@example.test", "Not Yours")
		id := mkAppt(t, pool, d, center, "5 days", "scheduled")
		deny := func(int64, int64) bool { return false }
		if err := svc.Cancel(ctx, id, "", deny); !errors.Is(err, service.ErrNotFound) {
			t.Errorf("cancelling out of scope = %v, want ErrNotFound", err)
		}
	})
}

// mkAppt inserts an appointment offset from now by a Postgres interval string.
func mkAppt(t *testing.T, pool *pgxpool.Pool, donorID, centerID int64, offset, status string) int32 {
	t.Helper()
	var id int32
	err := pool.QueryRow(context.Background(), `
		INSERT INTO appointments (donor_id, center_id, scheduled_at, status)
		VALUES ($1, $2, now() + $3::interval, $4::appointment_status) RETURNING id`,
		donorID, centerID, offset, status).Scan(&id)
	if err != nil {
		t.Fatalf("insert appointment: %v", err)
	}
	return id
}
