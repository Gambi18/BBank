package service_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"bbank/internal/service"
	"bbank/internal/store"
	"bbank/internal/testsupport"

	"github.com/jackc/pgx/v5/pgxpool"
)

func centerSetup(t *testing.T) (*service.CenterService, *service.DonationRequestService, *pgxpool.Pool) {
	t.Helper()
	pool := testsupport.Pool(t)
	q := store.New(pool)
	centers := service.NewCenterService(q)
	elig := service.NewEligibilityService(q, service.NewPolicyService(q))
	return centers, service.NewDonationRequestService(pool, q, elig, centers), pool
}

// setCapacity narrows or widens the seeded centre for one test.
func setCapacity(t *testing.T, pool *pgxpool.Pool, centerID int64, seats int) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE donation_centers SET capacity_per_slot = $1 WHERE id = $2`, seats, centerID); err != nil {
		t.Fatalf("set capacity: %v", err)
	}
}

// **WI-24's acceptance criterion, verbatim**: "Two simultaneous approvals into a
// one-seat slot produce exactly one appointment."
//
// Eight rather than two, for the reason the admin-demotion race test had to
// learn: two goroutines rarely overlap, and a concurrency test that never enters
// the race window passes whatever the code does. Eight distinct donors, eight
// distinct requests, one seat.
func TestWI24_ConcurrentApprovalsCannotOverbookASlot(t *testing.T) {
	_, requests, pool := centerSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)
	setCapacity(t, pool, center, 1)

	const n = 8
	when := time.Now().AddDate(0, 0, 14)

	ids := make([]int32, 0, n)
	for i := 0; i < n; i++ {
		donorID := newAdultDonor(t, pool, fmt.Sprintf("overbook%d@example.test", i))
		row, err := requests.Create(ctx, service.CreateRequestParams{
			DonorID: donorID, CenterID: &center, PreferredDate: &when,
		})
		if err != nil {
			t.Fatalf("create request %d: %v", i, err)
		}
		ids = append(ids, row.ID)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	booked, refused, other := 0, 0, 0
	start := make(chan struct{})

	for _, id := range ids {
		wg.Add(1)
		go func(id int32) {
			defer wg.Done()
			<-start
			_, err := requests.Approve(ctx, id, 1, when, allow)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				booked++
			case errors.Is(err, service.ErrConflict):
				refused++
			default:
				other++
				t.Errorf("approval failed unexpectedly: %v", err)
			}
		}(id)
	}
	close(start)
	wg.Wait()

	live := testsupport.CountRows(t, pool,
		`SELECT count(*) FROM appointments WHERE center_id = $1 AND status <> 'cancelled'`, center)
	if live != 1 {
		t.Fatalf("%d appointments in a one-seat slot (%d booked, %d refused, %d errored) — "+
			"the slot was over-booked", live, booked, refused, other)
	}
	if booked != 1 {
		t.Errorf("%d approvals reported success for one seat", booked)
	}
	// The other seven must be told the slot is full, not handed a 500.
	if other != 0 {
		t.Errorf("%d approvals failed with something other than a conflict", other)
	}
}

// Capacity is a real number, not a boolean: a four-seat slot takes four.
func TestWI24_ASlotFillsToCapacityAndNoFurther(t *testing.T) {
	_, requests, pool := centerSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)
	const seats = 3
	setCapacity(t, pool, center, seats)

	when := time.Now().AddDate(0, 0, 14)
	for i := 0; i < seats+2; i++ {
		donorID := newAdultDonor(t, pool, fmt.Sprintf("fill%d@example.test", i))
		row, err := requests.Create(ctx, service.CreateRequestParams{
			DonorID: donorID, CenterID: &center, PreferredDate: &when,
		})
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}

		_, err = requests.Approve(ctx, row.ID, 1, when, allow)
		if i < seats {
			if err != nil {
				t.Fatalf("approval %d of %d seats failed: %v", i+1, seats, err)
			}
			continue
		}
		if !errors.Is(err, service.ErrConflict) {
			t.Errorf("approval %d into a %d-seat slot = %v, want a conflict", i+1, seats, err)
		}
	}

	live := testsupport.CountRows(t, pool,
		`SELECT count(*) FROM appointments WHERE center_id = $1 AND status <> 'cancelled'`, center)
	if live != seats {
		t.Errorf("%d appointments in a %d-seat slot", live, seats)
	}
}

// Cancelling gives the seat back. Nothing else does: a no-show, a completion and
// a deferral all describe a slot that WAS used, and freeing those would let a
// past slot be re-booked.
func TestWI24_OnlyCancellingFreesASeat(t *testing.T) {
	_, requests, pool := centerSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)
	setCapacity(t, pool, center, 1)

	when := time.Now().AddDate(0, 0, 14)
	book := func(email string) (int32, error) {
		donorID := newAdultDonor(t, pool, email)
		row, err := requests.Create(ctx, service.CreateRequestParams{
			DonorID: donorID, CenterID: &center, PreferredDate: &when,
		})
		if err != nil {
			t.Fatalf("create for %s: %v", email, err)
		}
		appt, err := requests.Approve(ctx, row.ID, 1, when, allow)
		return appt.ID, err
	}

	first, err := book("seatholder@example.test")
	if err != nil {
		t.Fatalf("the first booking failed: %v", err)
	}
	if _, err := book("waiting@example.test"); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("a second booking into a one-seat slot = %v, want a conflict", err)
	}

	// A no-show does NOT free it.
	if _, err := pool.Exec(ctx, `UPDATE appointments SET status = 'no_show' WHERE id = $1`, first); err != nil {
		t.Fatalf("mark no-show: %v", err)
	}
	if _, err := book("afternoshow@example.test"); !errors.Is(err, service.ErrConflict) {
		t.Errorf("a no-show freed the seat (%v) — that slot is in the past and cannot be rebooked", err)
	}

	// Cancelling does.
	if _, err := pool.Exec(ctx, `UPDATE appointments SET status = 'cancelled' WHERE id = $1`, first); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := book("aftercancel@example.test"); err != nil {
		t.Errorf("cancelling did not free the seat: %v", err)
	}
}

// FR-14: "deactivating a center stops new bookings and preserves history".
func TestWI24_DeactivatingACentreStopsBookingsAndKeepsHistory(t *testing.T) {
	centers, requests, pool := centerSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)

	// One appointment booked while the centre is open.
	existing := newAdultDonor(t, pool, "before@example.test")
	when := time.Now().AddDate(0, 0, 14)
	row, err := requests.Create(ctx, service.CreateRequestParams{
		DonorID: existing, CenterID: &center, PreferredDate: &when,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	appt, err := requests.Approve(ctx, row.ID, 1, when, allow)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}

	closed := false
	if _, err := centers.Update(ctx, center, service.CenterInput{IsActive: &closed}); err != nil {
		t.Fatalf("deactivate: %v", err)
	}

	// New bookings stop — at BOTH gates, because a request and an approval are
	// two different ways to arrive at an appointment.
	newDonor := newAdultDonor(t, pool, "after@example.test")
	if _, err := requests.Create(ctx, service.CreateRequestParams{
		DonorID: newDonor, CenterID: &center, PreferredDate: &when,
	}); !errors.Is(err, service.ErrConflict) {
		t.Errorf("a request was raised at a closed centre: %v", err)
	}

	// History is untouched.
	var status string
	if err := pool.QueryRow(ctx, `SELECT status::text FROM appointments WHERE id = $1`, appt.ID).Scan(&status); err != nil {
		t.Fatalf("the existing appointment vanished: %v", err)
	}
	if status != "scheduled" {
		t.Errorf("the existing appointment is %q after the centre closed, want scheduled", status)
	}

	// And it can still be finished: closing a centre must not strand the people
	// already booked into it.
	if _, err := pool.Exec(ctx, `UPDATE appointments SET status = 'completed' WHERE id = $1`, appt.ID); err != nil {
		t.Errorf("an appointment at a closed centre cannot be completed: %v", err)
	}

	// Reopening works.
	open := true
	if _, err := centers.Update(ctx, center, service.CenterInput{IsActive: &open}); err != nil {
		t.Fatalf("reactivate: %v", err)
	}
	if _, err := requests.Create(ctx, service.CreateRequestParams{
		DonorID: newDonor, CenterID: &center, PreferredDate: &when,
	}); err != nil {
		t.Errorf("bookings did not resume after reopening: %v", err)
	}
}

// The trigger is the backstop: no caller, including raw SQL, may seat somebody
// beyond capacity. The unique index stops two donors sharing a seat; this stops
// one donor inventing seat 99.
func TestWI24_TheDatabaseRefusesASeatBeyondCapacity(t *testing.T) {
	_, _, pool := centerSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)
	setCapacity(t, pool, center, 2)

	donorID := newAdultDonor(t, pool, "seat99@example.test")
	_, err := pool.Exec(ctx, `
		INSERT INTO appointments (donor_id, center_id, scheduled_at, status, slot_seat)
		VALUES ($1, $2, now() + INTERVAL '14 days', 'scheduled', 99)`, donorID, center)
	if err == nil {
		t.Fatal("raw SQL seated a donor at seat 99 in a two-seat slot")
	}

	// And a closed centre refuses an insert at the database level too, so a
	// future code path that forgets the service check still cannot book one.
	if _, err := pool.Exec(ctx, `UPDATE donation_centers SET is_active = FALSE WHERE id = $1`, center); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO appointments (donor_id, center_id, scheduled_at, status, slot_seat)
		VALUES ($1, $2, now() + INTERVAL '14 days', 'scheduled', 1)`, donorID, center); err == nil {
		t.Error("raw SQL booked an appointment at a deactivated centre")
	}
}

// Two appointments a few minutes apart must land in the SAME slot, or capacity
// means nothing: a thirty-minute slot with four seats would accept an unbounded
// number of donors simply by varying the minute.
func TestWI24_RequestedTimesAreSnappedToTheSlotGrid(t *testing.T) {
	centers, _, pool := centerSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)

	if _, err := pool.Exec(ctx, `
		UPDATE donation_centers
		   SET slot_minutes = 30, timezone = 'Africa/Douala',
		       opening_hours = '{"mon":[["08:00","12:00"]],"tue":[["08:00","12:00"]],"wed":[["08:00","12:00"]],
		                         "thu":[["08:00","12:00"]],"fri":[["08:00","12:00"]],"sat":[["08:00","12:00"]],
		                         "sun":[["08:00","12:00"]]}'::jsonb
		 WHERE id = $1`, center); err != nil {
		t.Fatalf("configure hours: %v", err)
	}

	loc, err := time.LoadLocation("Africa/Douala")
	if err != nil {
		t.Skipf("no tzdata: %v", err)
	}
	day := time.Now().In(loc).AddDate(0, 0, 14)

	at := func(h, m int) time.Time {
		y, mo, d := day.Date()
		return time.Date(y, mo, d, h, m, 0, 0, loc)
	}

	for _, c := range []struct{ h, m, wantH, wantM int }{
		{8, 0, 8, 0},
		{8, 1, 8, 0},
		{8, 29, 8, 0},
		{8, 30, 8, 30},
		{8, 59, 8, 30},
		{11, 29, 11, 0},
		{11, 30, 11, 30}, // the last slot that finishes by 12:00
	} {
		got, err := centers.SlotFor(ctx, center, at(c.h, c.m))
		if err != nil {
			t.Errorf("%02d:%02d: %v", c.h, c.m, err)
			continue
		}
		if got.In(loc).Hour() != c.wantH || got.In(loc).Minute() != c.wantM {
			t.Errorf("%02d:%02d snapped to %s, want %02d:%02d",
				c.h, c.m, got.In(loc).Format("15:04"), c.wantH, c.wantM)
		}
	}

	// Outside opening hours is refused, not quietly relocated: booking at a time
	// the centre is shut is a mistake, and moving it hides the mistake.
	if _, err := centers.SlotFor(ctx, center, at(7, 30)); !errors.Is(err, service.ErrConflict) {
		t.Errorf("07:30 (before opening) = %v, want a conflict", err)
	}
	if _, err := centers.SlotFor(ctx, center, at(13, 0)); !errors.Is(err, service.ErrConflict) {
		t.Errorf("13:00 (after closing) = %v, want a conflict", err)
	}
	// 11:45 snaps DOWN to 11:30, which finishes exactly at closing. Snapping down
	// is the point: the donor asked for late morning and gets the slot their
	// request falls in, rather than being moved to a later appointment.
	if got, err := centers.SlotFor(ctx, center, at(11, 45)); err != nil {
		t.Errorf("11:45: %v", err)
	} else if got.In(loc).Format("15:04") != "11:30" {
		t.Errorf("11:45 snapped to %s, want 11:30", got.In(loc).Format("15:04"))
	}

	// A RAGGED interval is where the past-closing guard earns its place: with
	// 30-minute slots and a day ending at 12:10, a request at 12:05 falls inside
	// opening hours but the slot it would start runs twenty minutes past the
	// close. There is no such slot, and saying so beats booking a donor into a
	// building that is locking up.
	if _, err := pool.Exec(ctx, `
		UPDATE donation_centers SET opening_hours = jsonb_set(opening_hours, $1, '[["08:00","12:10"]]'::jsonb)
		 WHERE id = $2`, "{"+strings.ToLower(day.Format("Mon"))+"}", center); err != nil {
		t.Fatalf("configure a ragged day: %v", err)
	}
	if _, err := centers.SlotFor(ctx, center, at(12, 5)); !errors.Is(err, service.ErrConflict) {
		t.Errorf("12:05 on a day closing at 12:10 = %v, want a conflict — no 30-minute slot fits", err)
	}
	// And the 11:30 slot is still bookable, because it finishes at 12:00.
	if _, err := centers.SlotFor(ctx, center, at(11, 30)); err != nil {
		t.Errorf("11:30 on a day closing at 12:10 was refused: %v", err)
	}
}

// A centre with no configured hours keeps working. An unset column is an
// administrative gap, not a decision to close, and every appointment already in
// the database sits at the 09:00 migration 000005 hardcoded.
func TestWI24_ACentreWithNoHoursFallsBackRatherThanRefusing(t *testing.T) {
	centers, _, pool := centerSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)

	var hours []byte
	if err := pool.QueryRow(ctx, `SELECT opening_hours FROM donation_centers WHERE id = $1`, center).Scan(&hours); err != nil {
		t.Fatalf("read hours: %v", err)
	}
	if string(hours) != "{}" {
		t.Skipf("the seeded centre has hours configured (%s); this test is about the unset case", hours)
	}

	slot, err := centers.SlotFor(ctx, center, time.Now().AddDate(0, 0, 14))
	if err != nil {
		t.Fatalf("a centre with no hours refused every booking: %v", err)
	}
	sched, _, err := centers.Scheduling(ctx, center)
	if err != nil {
		t.Fatalf("scheduling: %v", err)
	}
	if got := slot.In(sched.Location); got.Hour() != 9 || got.Minute() != 0 {
		t.Errorf("fallback slot is %s, want 09:00 local — the time every existing appointment sits at",
			got.Format("15:04"))
	}
}

func TestWI24_CentreCRUD(t *testing.T) {
	centers, _, _ := centerSetup(t)
	ctx := context.Background()

	created, err := centers.Create(ctx, service.CenterInput{
		Code: "north", Name: "North Centre", AddressLine: "1 Test Road",
		City: "Douala", Region: "Littoral",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Code != "NORTH" {
		t.Errorf("code = %q, want it upper-cased", created.Code)
	}
	if created.CapacityPerSlot != 4 || created.SlotMinutes != 30 {
		t.Errorf("defaults not applied: capacity %d, slot %d", created.CapacityPerSlot, created.SlotMinutes)
	}

	if _, err := centers.Create(ctx, service.CenterInput{
		Code: "NORTH", Name: "Duplicate", AddressLine: "2 Test Road", City: "Douala", Region: "Littoral",
	}); !errors.Is(err, service.ErrConflict) {
		t.Errorf("a duplicate code = %v, want a conflict", err)
	}

	// PATCH keeps what it is not sent — the same rule the donor profile learned.
	newName := "North Donation Centre"
	updated, err := centers.Update(ctx, created.ID, service.CenterInput{Name: newName})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != newName {
		t.Errorf("name = %q, want %q", updated.Name, newName)
	}
	if updated.AddressLine != created.AddressLine || updated.City != created.City {
		t.Errorf("updating the name cleared the address: %+v", updated)
	}

	for name, in := range map[string]service.CenterInput{
		"unknown timezone":    {Timezone: strPtr("Mars/Olympus")},
		"malformed hours":     {OpeningHours: []byte(`{"mon":[["17:00","09:00"]]}`)},
		"non-weekday key":     {OpeningHours: []byte(`{"funday":[["09:00","17:00"]]}`)},
		"overlapping windows": {OpeningHours: []byte(`{"mon":[["08:00","12:00"],["11:00","15:00"]]}`)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := centers.Update(ctx, created.ID, in); !errors.Is(err, service.ErrInvalid) {
				t.Errorf("%s = %v, want ErrInvalid", name, err)
			}
		})
	}

	if _, err := centers.Get(ctx, 999999); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("get missing centre = %v, want ErrNotFound", err)
	}

	rows, total, err := centers.List(ctx, service.ListCenterParams{Limit: 25})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total < 2 || len(rows) < 2 {
		t.Errorf("list returned %d of %d, want the seeded centres plus the new one", len(rows), total)
	}
}

// Lowering capacity below what a slot already holds is allowed, and does not
// touch the appointments already there. Cancelling somebody's appointment
// because an administrator edited a number would be the system making a
// scheduling decision nobody asked it to make.
func TestWI24_LoweringCapacityKeepsExistingAppointments(t *testing.T) {
	centers, requests, pool := centerSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)
	setCapacity(t, pool, center, 3)

	when := time.Now().AddDate(0, 0, 14)
	for i := 0; i < 3; i++ {
		donorID := newAdultDonor(t, pool, fmt.Sprintf("shrink%d@example.test", i))
		row, err := requests.Create(ctx, service.CreateRequestParams{
			DonorID: donorID, CenterID: &center, PreferredDate: &when,
		})
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		if _, err := requests.Approve(ctx, row.ID, 1, when, allow); err != nil {
			t.Fatalf("approve %d: %v", i, err)
		}
	}

	one := int16(1)
	if _, err := centers.Update(ctx, center, service.CenterInput{CapacityPerSlot: &one}); err != nil {
		t.Fatalf("shrink the centre: %v", err)
	}

	live := testsupport.CountRows(t, pool,
		`SELECT count(*) FROM appointments WHERE center_id = $1 AND status <> 'cancelled'`, center)
	if live != 3 {
		t.Errorf("%d appointments survive a capacity cut, want 3 — an administrator's edit cancelled somebody", live)
	}

	// And the reduced figure governs the next booking.
	donorID := newAdultDonor(t, pool, "afterthecut@example.test")
	row, err := requests.Create(ctx, service.CreateRequestParams{
		DonorID: donorID, CenterID: &center, PreferredDate: &when,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := requests.Approve(ctx, row.ID, 1, when, allow); !errors.Is(err, service.ErrConflict) {
		t.Errorf("a booking after the cut = %v, want a conflict", err)
	}
}

func strPtr(s string) *string { return &s }

// A booking is for a day that has not happened.
//
// Nothing refused a past date before, and the eligibility gate would not: "were
// you eligible last Tuesday?" is a well-formed question it answers happily. The
// request would then sit in the queue for a day staff cannot schedule.
func TestWI24_APastDateIsRefused(t *testing.T) {
	_, requests, pool := centerSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)
	donorID := newAdultDonor(t, pool, "timetraveller@example.test")

	yesterday := time.Now().AddDate(0, 0, -1)
	if _, err := requests.Create(ctx, service.CreateRequestParams{
		DonorID: donorID, CenterID: &center, PreferredDate: &yesterday,
	}); !errors.Is(err, service.ErrInvalid) {
		t.Errorf("a booking for yesterday = %v, want ErrInvalid", err)
	}

	// Today is still fine — a walk-in booked the same morning is ordinary, and
	// the comparison is on the date rather than the instant so the hour of the
	// day cannot change the answer.
	today := time.Now()
	if _, err := requests.Create(ctx, service.CreateRequestParams{
		DonorID: donorID, CenterID: &center, PreferredDate: &today,
	}); err != nil {
		t.Errorf("a same-day booking was refused: %v", err)
	}
}
