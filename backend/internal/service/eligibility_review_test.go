package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"bbank/internal/domain"
	"bbank/internal/service"
	"bbank/internal/testsupport"
)

// Each test here guards a defect found by the WI-25/WI-26 code review. They are
// named for the defect rather than the requirement, because the requirement was
// already covered and passed while the defect was present — which is exactly why
// they are worth keeping.

// The gate must evaluate the procedure that gets STORED.
//
// `procedure` was accepted from the body, used by the gate, and then dropped:
// the insert omitted the column so every row was `whole_blood`. A donor who gave
// whole blood eight days ago could post `{"procedure":"apheresis_platelet"}`,
// pass the 7-day platelet interval, and have a WHOLE-BLOOD request stored — the
// gate guarding a donation nobody was going to make.
func TestTheGatedProcedureIsTheStoredProcedure(t *testing.T) {
	_, requests, pool := eligSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)

	donorID := newAdultDonor(t, pool, "storedproc@example.test")
	testsupport.NewCompletedDonation(t, pool, donorID, center, "whole_blood", time.Now().AddDate(0, 0, -8))

	soon := time.Now().AddDate(0, 0, 1)
	row, err := requests.Create(ctx, service.CreateRequestParams{
		DonorID: donorID, CenterID: &center, PreferredDate: &soon,
		Procedure: domain.ProcedureApheresisPlatelet,
	})
	if err != nil {
		t.Fatalf("a platelet booking 8 days after whole blood was refused: %v", err)
	}

	var stored string
	if err := pool.QueryRow(ctx,
		`SELECT procedure::text FROM donation_requests WHERE id = $1`, row.ID).Scan(&stored); err != nil {
		t.Fatalf("read the stored procedure: %v", err)
	}
	if stored != string(domain.ProcedureApheresisPlatelet) {
		t.Fatalf("the gate checked %s and the row stored %s — the 7-day platelet interval was used to "+
			"admit a whole-blood donation 8 days after the last one",
			domain.ProcedureApheresisPlatelet, stored)
	}
}

// The gate must decide for the date the row will carry.
//
// With no `preferred_date` the insert defaults to `CURRENT_DATE + 7`, but the
// gate defaulted to today — so a donor whose interval elapsed in three days was
// refused for a request that would have been dated a week out and was perfectly
// valid.
func TestAnOmittedDateIsGatedAtTheDateItWillBeStoredAs(t *testing.T) {
	_, requests, pool := eligSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)

	donorID := newAdultDonor(t, pool, "defaultdate@example.test")
	// 53 days ago: ineligible today, eligible on day 56 — three days from now,
	// and comfortably inside the seven-day default.
	testsupport.NewCompletedDonation(t, pool, donorID, center, "whole_blood", time.Now().AddDate(0, 0, -53))

	row, err := requests.Create(ctx, service.CreateRequestParams{DonorID: donorID, CenterID: &center})
	if err != nil {
		t.Fatalf("a booking with no date was refused, though its stored date is seven days out: %v", err)
	}

	var stored time.Time
	if err := pool.QueryRow(ctx,
		`SELECT preferred_date FROM donation_requests WHERE id = $1`, row.ID).Scan(&stored); err != nil {
		t.Fatalf("read the stored date: %v", err)
	}
	if want := time.Now().AddDate(0, 0, 7).Format("2006-01-02"); stored.Format("2006-01-02") != want {
		t.Errorf("stored preferred_date = %s, want %s — the gate and the insert must use one date",
			stored.Format("2006-01-02"), want)
	}
}

// Approval must re-check eligibility against the date STAFF choose.
//
// The gate at Create answers for the donor's preferred date. Approval takes its
// own date, and used to create the appointment with no check at all — so staff
// could schedule an appointment three weeks inside the interval by typing an
// earlier date.
func TestApprovalReGatesAgainstTheScheduledDate(t *testing.T) {
	_, requests, pool := eligSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)

	donorID := newAdultDonor(t, pool, "approvegate@example.test")
	testsupport.NewCompletedDonation(t, pool, donorID, center, "whole_blood", time.Now().AddDate(0, 0, -30))

	// Booked for day 60 after the last donation: past the 56-day interval, so
	// the booking itself is fine.
	preferred := time.Now().AddDate(0, 0, 30)
	row, err := requests.Create(ctx, service.CreateRequestParams{
		DonorID: donorID, CenterID: &center, PreferredDate: &preferred,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Staff approve it for tomorrow — 31 days after the donation, well inside
	// the interval.
	tooSoon := time.Now().AddDate(0, 0, 1)
	if _, err := requests.Approve(ctx, row.ID, 1, service.OnDate(tooSoon), allow); err == nil {
		t.Fatal("an appointment was scheduled 31 days after a donation, inside the 56-day interval")
	} else if e, ok := service.AsIneligible(err); !ok {
		t.Fatalf("approve returned %v, want ErrIneligible", err)
	} else if !e.Decision.Has(domain.CriterionIntervalNotElapsed) {
		t.Errorf("failures = %+v, want interval_not_elapsed", e.Decision.Failures)
	}

	// The honest date still works, and the request survives the refused attempt.
	if _, err := requests.Approve(ctx, row.ID, 1, service.OnDate(preferred), allow); err != nil {
		t.Fatalf("approving for the requested date failed: %v", err)
	}
}

// A deferral recorded between request and approval must be seen at approval.
func TestApprovalSeesADeferralRecordedAfterTheRequest(t *testing.T) {
	_, requests, pool := eligSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)

	donorID := newAdultDonor(t, pool, "laterdeferral@example.test")
	preferred := time.Now().AddDate(0, 0, 10)
	row, err := requests.Create(ctx, service.CreateRequestParams{
		DonorID: donorID, CenterID: &center, PreferredDate: &preferred,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// A screening the next day records a deferral covering the appointment.
	deferUntil(t, pool, donorID, time.Now().AddDate(0, 0, 30))

	if _, err := requests.Approve(ctx, row.ID, 1, service.OnDate(preferred), allow); err == nil {
		t.Fatal("a deferral recorded after the request was invisible at approval")
	} else if e, ok := service.AsIneligible(err); !ok {
		t.Fatalf("approve returned %v, want ErrIneligible", err)
	} else if !e.Decision.Has(domain.CriterionTemporaryDeferral) {
		t.Errorf("failures = %+v, want temporarily_deferred", e.Decision.Failures)
	}
}

// An override on a refusal that has nothing to override must still report the
// real failing criteria.
//
// It used to discard the decision and return a bare conflict saying only that
// the override was unnecessary — so the caller lost every criterion (FR-17) and
// the clearing date (FR-08), which is worse than not sending an override at all.
func TestAnOverrideDoesNotSwallowTheRealReasons(t *testing.T) {
	_, requests, pool := eligSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)

	donorID := newAdultDonor(t, pool, "swallow@example.test")
	testsupport.NewCompletedDonation(t, pool, donorID, center, "whole_blood", time.Now().AddDate(0, 0, -5))

	tomorrow := time.Now().AddDate(0, 0, 1)
	_, err := requests.Create(ctx, service.CreateRequestParams{
		DonorID: donorID, CenterID: &center, PreferredDate: &tomorrow,
		Override: &service.PermanentDeferralOverride{
			ActorID: 1, ActorRole: domain.RoleAdmin,
			Reason: "Attempting to book despite the interval, which is not overridable.",
		},
	})
	e, ok := service.AsIneligible(err)
	if !ok {
		t.Fatalf("error = %v, want ErrIneligible carrying the real criteria", err)
	}
	if !e.Decision.Has(domain.CriterionIntervalNotElapsed) {
		t.Errorf("failures = %+v, want interval_not_elapsed", e.Decision.Failures)
	}
	if e.Decision.NextEligibleOn == nil {
		t.Error("the refusal carries no next-eligible date; FR-08 needs one here")
	}
}

// A role with no right to override must be refused whether or not the donor
// happens to be eligible.
//
// The check ran after the eligibility decision, so for an eligible donor it
// never ran at all: anyone could send the field and get a 201 with no trace. The
// DTO says the field is "refused, not ignored" — this is what makes that true.
func TestANonAdminOverrideIsRefusedEvenForAnEligibleDonor(t *testing.T) {
	_, requests, pool := eligSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)

	donorID := newAdultDonor(t, pool, "eligibleoverride@example.test")
	preferred := time.Now().AddDate(0, 0, 7)

	_, err := requests.Create(ctx, service.CreateRequestParams{
		DonorID: donorID, CenterID: &center, PreferredDate: &preferred,
		Override: &service.PermanentDeferralOverride{
			ActorID: donorID, ActorRole: domain.RoleDonor,
			Reason: "I would like to donate anyway, please.",
		},
	})
	if err == nil {
		t.Fatal("a donor sent an override on their own booking and it was accepted silently")
	}
	if !errors.Is(err, service.ErrInvalid) {
		t.Errorf("error = %v, want ErrInvalid", err)
	}
	if n := testsupport.CountRows(t, pool, `SELECT count(*) FROM donation_requests WHERE donor_id = $1`, donorID); n != 0 {
		t.Errorf("%d request(s) created despite the refused override", n)
	}
}

// A procedure the deployment has no interval for is a 422 the caller can act on,
// not a 500.
//
// `donation_procedure` has four members and the seed configures an interval for
// two. `{"procedure":"double_red_cell"}` is a well-formed request this
// deployment cannot decide — it used to reach the client as an internal error.
func TestAnUnconfiguredProcedureIsAValidationFailureNotACrash(t *testing.T) {
	_, requests, pool := eligSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)

	donorID := newAdultDonor(t, pool, "unconfigured@example.test")
	preferred := time.Now().AddDate(0, 0, 7)

	for _, proc := range []domain.Procedure{domain.ProcedureDoubleRedCell, domain.ProcedureApheresisPlasma} {
		t.Run(string(proc), func(t *testing.T) {
			_, err := requests.Create(ctx, service.CreateRequestParams{
				DonorID: donorID, CenterID: &center, PreferredDate: &preferred, Procedure: proc,
			})
			if err == nil {
				t.Fatalf("%s was booked with no configured interval", proc)
			}
			if !errors.Is(err, service.ErrInvalid) {
				t.Errorf("%s returned %v, want ErrInvalid (422), not an internal error", proc, err)
			}
		})
	}
}

// Concurrent approvals must not deadlock the connection pool.
//
// The first approval gate called the pool-wide eligibility service from INSIDE
// the approving transaction, so every goroutine held a transaction and a
// `FOR UPDATE` lock while waiting for a second connection that another goroutine
// was holding. Eight of them hung until the suite timed out at ten minutes.
//
// `TestConcurrentApprovalsCreateExactlyOneAppointment` also covers this, but it
// is about the appointment count and would read as a flake if it hung; this one
// names the cause.
func TestTheApprovalGateDoesNotHoldTwoConnections(t *testing.T) {
	_, requests, pool := eligSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)

	// More concurrent approvals than a small pool has connections.
	const n = 12

	// Widen the slot to hold them all. Without this the test measures WI-24's
	// capacity constraint instead of the thing it is named for: the seeded
	// centre holds four, so eight approvals would be correctly refused and the
	// deadlock this guards against would go unexercised.
	//
	// It is a better test for the widening, not a weaker one — twelve concurrent
	// inserts into a twelve-seat slot must come out as twelve DISTINCT seats,
	// which is the seat allocator under real contention.
	if _, err := pool.Exec(ctx,
		`UPDATE donation_centers SET capacity_per_slot = $1 WHERE id = $2`, n, center); err != nil {
		t.Fatalf("widen the slot: %v", err)
	}
	type booking struct {
		id       int32
		donorID  int64
		schedule time.Time
	}
	bookings := make([]booking, 0, n)
	for i := 0; i < n; i++ {
		donorID := newAdultDonor(t, pool, poolEmail(i))
		when := time.Now().AddDate(0, 0, 7)
		row, err := requests.Create(ctx, service.CreateRequestParams{
			DonorID: donorID, CenterID: &center, PreferredDate: &when,
		})
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		bookings = append(bookings, booking{id: row.ID, donorID: donorID, schedule: when})
	}

	done := make(chan error, n)
	start := make(chan struct{})
	for _, b := range bookings {
		go func(b booking) {
			<-start
			_, err := requests.Approve(ctx, b.id, 1, service.OnDate(b.schedule), allow)
			done <- err
		}(b)
	}
	close(start)

	deadline := time.After(60 * time.Second)
	for i := 0; i < n; i++ {
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("approval %d failed: %v", i, err)
			}
		case <-deadline:
			t.Fatalf("only %d of %d approvals finished in 60s — the gate is holding a second "+
				"connection while inside its own transaction", i, n)
		}
	}

	// Twelve appointments, twelve distinct seats. A duplicate would mean the
	// unique index is not doing the work; a gap would mean the allocator skipped
	// a free seat under contention.
	seats := testsupport.CountRows(t, pool, `
		SELECT count(DISTINCT slot_seat) FROM appointments
		 WHERE center_id = $1 AND status <> 'cancelled'`, center)
	if seats != n {
		t.Errorf("%d distinct seats across %d appointments — the allocator collided or skipped", seats, n)
	}
}

func poolEmail(i int) string {
	return "poolapprove" + string(rune('a'+i)) + "@example.test"
}
