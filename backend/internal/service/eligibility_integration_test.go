package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"bbank/internal/domain"
	"bbank/internal/service"
	"bbank/internal/store"
	"bbank/internal/testsupport"

	"github.com/jackc/pgx/v5/pgxpool"
)

func eligSetup(t *testing.T) (*service.EligibilityService, *service.DonationRequestService, *pgxpool.Pool) {
	t.Helper()
	pool := testsupport.Pool(t)
	q := store.New(pool)
	elig := service.NewEligibilityService(q, service.NewPolicyService(q))
	return elig, service.NewDonationRequestService(pool, q, elig), pool
}

// newAdultDonor inserts a donor who passes every rule, so a test can break
// exactly one thing.
func newAdultDonor(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(), `
		WITH u AS (
			INSERT INTO users (email, password_hash, role, status)
			VALUES ($1, '$2b$12$abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ012', 'donor', 'active')
			RETURNING id
		)
		INSERT INTO donor_profiles (user_id, full_name, date_of_birth, gender, contact_phone)
		SELECT id, 'Eligible Donor', CURRENT_DATE - INTERVAL '30 years', 'female', '+237600000000' FROM u
		RETURNING user_id`, email).Scan(&id)
	if err != nil {
		t.Fatalf("insert donor: %v", err)
	}
	return id
}

// deferUntil records a temporary deferral ending on the given day.
func deferUntil(t *testing.T, pool *pgxpool.Pool, donorID int64, endsOn time.Time) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO deferrals (donor_id, type, reason, starts_on, ends_on)
		VALUES ($1, 'temporary', 'low haemoglobin at last screening', CURRENT_DATE - 1, $2)`,
		donorID, endsOn); err != nil {
		t.Fatalf("insert deferral: %v", err)
	}
}

func deferPermanently(t *testing.T, pool *pgxpool.Pool, donorID int64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO deferrals (donor_id, type, reason, starts_on)
		VALUES ($1, 'permanent', 'a permanent clinical contraindication', CURRENT_DATE - 1)`,
		donorID); err != nil {
		t.Fatalf("insert permanent deferral: %v", err)
	}
}

// FR-19: an active temporary deferral blocks booking, SERVER-SIDE.
//
// Through the service, not the handler: the acceptance criterion is "the block
// cannot be bypassed by calling the API directly", and a check that lives in one
// handler is bypassed by the next caller of the same method.
func TestFR19_ATemporaryDeferralBlocksBooking(t *testing.T) {
	_, requests, pool := eligSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)

	donorID := newAdultDonor(t, pool, "deferred@example.test")
	endsOn := time.Now().AddDate(0, 0, 20)
	deferUntil(t, pool, donorID, endsOn)

	preferred := time.Now().AddDate(0, 0, 7) // inside the deferral
	_, err := requests.Create(ctx, service.CreateRequestParams{
		DonorID: donorID, CenterID: &center, PreferredDate: &preferred,
	})
	if err == nil {
		t.Fatal("a deferred donor booked a donation")
	}

	e, ok := service.AsIneligible(err)
	if !ok {
		t.Fatalf("error = %v, want ErrIneligible carrying the decision", err)
	}
	if !e.Decision.Has(domain.CriterionTemporaryDeferral) {
		t.Errorf("failures = %+v, want temporarily_deferred", e.Decision.Failures)
	}
	// FR-08: blocked "with the date they become eligible".
	if e.Decision.NextEligibleOn == nil {
		t.Fatal("the refusal carries no next-eligible date")
	}
	// `ends_on` itself, not the day after: the boundary is exclusive, matching
	// the facts query's `ends_on > CURRENT_DATE` filter and the corrected view.
	if got, want := e.Decision.NextEligibleOn.Format("2006-01-02"), endsOn.Format("2006-01-02"); got != want {
		t.Errorf("next eligible = %s, want %s (ends_on itself — the first eligible day)", got, want)
	}
	// FR-19: "a plain-language explanation, not an error code".
	if msg := e.Decision.Failures[0].Message; msg == "" || msg == string(domain.CriterionTemporaryDeferral) {
		t.Errorf("message is not plain language: %q", msg)
	}

	// And nothing was written: a refused booking must leave no row behind.
	if n := testsupport.CountRows(t, pool, `SELECT count(*) FROM donation_requests WHERE donor_id = $1`, donorID); n != 0 {
		t.Errorf("%d donation request(s) were created despite the refusal", n)
	}
}

// Booking is decided for the PREFERRED DATE, not for today.
//
// A donor deferred until the 20th who books for the 25th is making a perfectly
// correct booking, and refusing it would be refusing the commonest booking there
// is — the one a donor makes precisely because they cannot donate yet.
func TestFR19_BookingBeyondTheDeferralIsAllowed(t *testing.T) {
	_, requests, pool := eligSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)

	donorID := newAdultDonor(t, pool, "booksahead@example.test")
	deferUntil(t, pool, donorID, time.Now().AddDate(0, 0, 20))

	beyond := time.Now().AddDate(0, 0, 25)
	if _, err := requests.Create(ctx, service.CreateRequestParams{
		DonorID: donorID, CenterID: &center, PreferredDate: &beyond,
	}); err != nil {
		t.Fatalf("booking after the deferral ends was refused: %v", err)
	}
}

// FR-19: the interval window blocks too, and the donor is told when it opens.
func TestFR19_TheDonationIntervalBlocksBooking(t *testing.T) {
	_, requests, pool := eligSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)

	donorID := newAdultDonor(t, pool, "toosoon@example.test")
	collected := time.Now().AddDate(0, 0, -10)
	testsupport.NewCompletedDonation(t, pool, donorID, center, "whole_blood", collected)

	tomorrow := time.Now().AddDate(0, 0, 1)
	_, err := requests.Create(ctx, service.CreateRequestParams{
		DonorID: donorID, CenterID: &center, PreferredDate: &tomorrow,
	})
	e, ok := service.AsIneligible(err)
	if !ok {
		t.Fatalf("booking 10 days after a donation = %v, want ErrIneligible", err)
	}
	if !e.Decision.Has(domain.CriterionIntervalNotElapsed) {
		t.Errorf("failures = %+v, want interval_not_elapsed", e.Decision.Failures)
	}
	if e.Decision.NextEligibleOn == nil {
		t.Fatal("no next-eligible date on an interval refusal")
	}
	want := collected.AddDate(0, 0, 56).Format("2006-01-02")
	if got := e.Decision.NextEligibleOn.Format("2006-01-02"); got != want {
		t.Errorf("next eligible = %s, want %s (56 days after the donation)", got, want)
	}
}

// The interval is per PROCEDURE. A whole-blood donation last week must not push
// a platelet apheresis booking out by 56 days.
func TestFR19_TheIntervalIsPerProcedure(t *testing.T) {
	_, requests, pool := eligSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)

	donorID := newAdultDonor(t, pool, "platelets@example.test")
	testsupport.NewCompletedDonation(t, pool, donorID, center, "whole_blood", time.Now().AddDate(0, 0, -10))

	// 10 days after whole blood: the 7-day platelet interval has elapsed.
	soon := time.Now().AddDate(0, 0, 1)
	if _, err := requests.Create(ctx, service.CreateRequestParams{
		DonorID: donorID, CenterID: &center, PreferredDate: &soon,
		Procedure: domain.ProcedureApheresisPlatelet,
	}); err != nil {
		t.Fatalf("a platelet booking was refused because of a whole-blood donation: %v", err)
	}
}

// FR-19: "A permanent deferral cannot be bypassed by any role except `admin`,
// with audit."
func TestFR19_OnlyAnAdminMayOverrideAPermanentDeferral(t *testing.T) {
	_, requests, pool := eligSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)

	donorID := newAdultDonor(t, pool, "permanent@example.test")
	deferPermanently(t, pool, donorID)
	preferred := time.Now().AddDate(0, 0, 7)

	base := service.CreateRequestParams{DonorID: donorID, CenterID: &center, PreferredDate: &preferred}

	// No override: refused, and the donor is not told to come back later,
	// because a permanent deferral does not clear by waiting.
	_, err := requests.Create(ctx, base)
	e, ok := service.AsIneligible(err)
	if !ok {
		t.Fatalf("error = %v, want ErrIneligible", err)
	}
	if !e.Decision.Has(domain.CriterionPermanentDeferral) {
		t.Errorf("failures = %+v, want permanently_deferred", e.Decision.Failures)
	}
	if e.Decision.NextEligibleOn != nil {
		t.Errorf("a permanent deferral produced a next-eligible date of %v", e.Decision.NextEligibleOn)
	}

	// Every non-admin role is refused, one case per role rather than a loop over
	// one, so a failure names the role that got through.
	for _, role := range []domain.Role{
		domain.RoleDonor, domain.RoleStaff, domain.RoleLabTech,
		domain.RoleInventoryManager, domain.RoleHospitalUser,
	} {
		t.Run(string(role)+" cannot override", func(t *testing.T) {
			p := base
			p.Override = &service.PermanentDeferralOverride{
				ActorID: 1, ActorRole: role, Reason: "a considered clinical judgement",
			}
			_, err := requests.Create(ctx, p)
			if err == nil {
				t.Fatalf("%s overrode a permanent deferral", role)
			}
			if !errors.Is(err, service.ErrInvalid) {
				t.Errorf("%s got %v, want ErrInvalid naming the refusal", role, err)
			}
		})
	}

	// An admin with no usable reason is refused too: FR-19 requires the audit,
	// and an override nobody can explain afterwards is not audited.
	for _, reason := range []string{"", "   ", "ok", "fine"} {
		p := base
		p.Override = &service.PermanentDeferralOverride{
			ActorID: 1, ActorRole: domain.RoleAdmin, Reason: reason,
		}
		if _, err := requests.Create(ctx, p); !errors.Is(err, service.ErrInvalid) {
			t.Errorf("an admin overrode with reason %q and got %v, want ErrInvalid", reason, err)
		}
	}

	// An admin with a real reason may proceed.
	p := base
	p.Override = &service.PermanentDeferralOverride{
		ActorID: 1, ActorRole: domain.RoleAdmin,
		Reason: "Reviewed with the medical director; the 2019 deferral was recorded in error.",
	}
	if _, err := requests.Create(ctx, p); err != nil {
		t.Fatalf("an admin with a reason could not override: %v", err)
	}
}

// An override clears the permanent deferral and NOTHING else.
//
// Otherwise "override" quietly becomes "book anyone regardless", and the
// interval — which exists to stop a donor being bled too often — would be
// waivable by the same field.
func TestFR19_AnOverrideDoesNotWaiveEveryOtherRule(t *testing.T) {
	_, requests, pool := eligSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)

	donorID := newAdultDonor(t, pool, "both@example.test")
	deferPermanently(t, pool, donorID)
	testsupport.NewCompletedDonation(t, pool, donorID, center, "whole_blood", time.Now().AddDate(0, 0, -5))

	preferred := time.Now().AddDate(0, 0, 1)
	_, err := requests.Create(ctx, service.CreateRequestParams{
		DonorID: donorID, CenterID: &center, PreferredDate: &preferred,
		Override: &service.PermanentDeferralOverride{
			ActorID: 1, ActorRole: domain.RoleAdmin,
			Reason: "Reviewed with the medical director; the deferral was recorded in error.",
		},
	})
	e, ok := service.AsIneligible(err)
	if !ok {
		t.Fatalf("an override waived the donation interval as well: %v", err)
	}
	if e.Decision.Has(domain.CriterionPermanentDeferral) {
		t.Error("the permanent deferral was still reported after being overridden")
	}
	if !e.Decision.Has(domain.CriterionIntervalNotElapsed) {
		t.Errorf("failures = %+v, want the interval to still block", e.Decision.Failures)
	}
}

// An override offered where nothing needs overriding is refused rather than
// ignored, so "an override was used" and "an override was needed" stay the same
// fact in the audit trail.
func TestFR19_AnUnnecessaryOverrideIsRefused(t *testing.T) {
	_, requests, pool := eligSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)

	donorID := newAdultDonor(t, pool, "nodeferral@example.test")
	testsupport.NewCompletedDonation(t, pool, donorID, center, "whole_blood", time.Now().AddDate(0, 0, -5))

	preferred := time.Now().AddDate(0, 0, 1)
	_, err := requests.Create(ctx, service.CreateRequestParams{
		DonorID: donorID, CenterID: &center, PreferredDate: &preferred,
		Override: &service.PermanentDeferralOverride{
			ActorID: 1, ActorRole: domain.RoleAdmin, Reason: "there is nothing here to override",
		},
	})
	if !errors.Is(err, service.ErrConflict) {
		t.Errorf("an unnecessary override returned %v, want ErrConflict", err)
	}
}

// A suspended account cannot book, and the reason says so rather than blaming a
// clinical rule.
func TestFR19_AnInactiveAccountCannotBook(t *testing.T) {
	_, requests, pool := eligSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)

	donorID := newAdultDonor(t, pool, "suspended@example.test")
	if _, err := pool.Exec(ctx, `UPDATE users SET status = 'suspended' WHERE id = $1`, donorID); err != nil {
		t.Fatalf("suspend: %v", err)
	}

	preferred := time.Now().AddDate(0, 0, 7)
	_, err := requests.Create(ctx, service.CreateRequestParams{
		DonorID: donorID, CenterID: &center, PreferredDate: &preferred,
	})
	e, ok := service.AsIneligible(err)
	if !ok {
		t.Fatalf("a suspended donor's booking returned %v, want ErrIneligible", err)
	}
	if !e.Decision.Has(domain.CriterionAccountNotActive) {
		t.Errorf("failures = %+v, want account_not_active", e.Decision.Failures)
	}
}

// **The view and the domain must agree.**
//
// `donor_eligibility` reimplements the age band, the interval and the deferral
// rules in SQL; `domain.EvaluateEligibility` implements them in Go. Two
// implementations of one clinical rule is the shape of the four defects fixed in
// the 2026-09-02 sweep — each path guessed, and they disagreed. This test is the
// guard: if the SQL and the Go ever answer differently about the same donor on
// the same day, it fails and names the donor.
func TestTheViewAndTheDomainAgree(t *testing.T) {
	elig, _, pool := eligSetup(t)
	ctx := context.Background()
	center := testsupport.CenterID(t, pool)
	q := store.New(pool)

	type fixture struct {
		name  string
		build func(t *testing.T, donorID int64)
	}
	fixtures := []fixture{
		{"a plainly eligible donor", func(t *testing.T, id int64) {}},
		{"inside a temporary deferral", func(t *testing.T, id int64) {
			deferUntil(t, pool, id, time.Now().AddDate(0, 0, 14))
		}},
		{"permanently deferred", func(t *testing.T, id int64) {
			deferPermanently(t, pool, id)
		}},
		{"inside the donation interval", func(t *testing.T, id int64) {
			testsupport.NewCompletedDonation(t, pool, id, center, "whole_blood", time.Now().AddDate(0, 0, -10))
		}},
		{"exactly on the interval boundary", func(t *testing.T, id int64) {
			testsupport.NewCompletedDonation(t, pool, id, center, "whole_blood", time.Now().AddDate(0, 0, -56))
		}},
		{"one day short of the boundary", func(t *testing.T, id int64) {
			testsupport.NewCompletedDonation(t, pool, id, center, "whole_blood", time.Now().AddDate(0, 0, -55))
		}},
		{"a suspended account", func(t *testing.T, id int64) {
			if _, err := pool.Exec(ctx, `UPDATE users SET status = 'suspended' WHERE id = $1`, id); err != nil {
				t.Fatalf("suspend: %v", err)
			}
		}},
		{"a deferral that ended yesterday", func(t *testing.T, id int64) {
			if _, err := pool.Exec(ctx, `
				INSERT INTO deferrals (donor_id, type, reason, starts_on, ends_on)
				VALUES ($1, 'temporary', 'resolved', CURRENT_DATE - 30, CURRENT_DATE - 1)`, id); err != nil {
				t.Fatalf("insert deferral: %v", err)
			}
		}},
	}

	for i, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			donorID := newAdultDonor(t, pool, agreeEmail(i))
			f.build(t, donorID)

			view, err := q.GetDonorEligibilityView(ctx, donorID)
			if err != nil {
				t.Fatalf("read the view: %v", err)
			}
			decision, err := elig.Evaluate(ctx, donorID, domain.ProcedureWholeBlood, time.Now())
			if err != nil {
				t.Fatalf("evaluate: %v", err)
			}

			viewSays := anyBool(view.IsEligibleToday)
			if viewSays == nil {
				t.Fatalf("the view could not decide (reason %v) while the domain answered eligible=%v — "+
					"they must at least agree about whether an answer exists", view.Reason, decision.Eligible)
			}
			if *viewSays != decision.Eligible {
				t.Fatalf("the view says eligible=%v and the domain says eligible=%v — "+
					"two implementations of one clinical rule have diverged (view reason: %v, domain: %+v)",
					*viewSays, decision.Eligible, view.Reason, decision.Failures)
			}
			// And when both refuse, they must refuse for the same reason.
			if !decision.Eligible {
				if got, want := string(decision.Reason()), reasonString(view.Reason); got != want {
					t.Errorf("the view's reason is %q and the domain's is %q", want, got)
				}
			}
		})
	}
}

func agreeEmail(i int) string {
	return "agree" + string(rune('a'+i)) + "@example.test"
}

// anyBool reads the view's `is_eligible_today`, which is a CASE expression and
// so arrives untyped. nil means the view could not decide.
func anyBool(v any) *bool {
	switch b := v.(type) {
	case nil:
		return nil
	case bool:
		return &b
	case *bool:
		return b
	}
	return nil
}

func reasonString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case *string:
		if s == nil {
			return ""
		}
		return *s
	default:
		return ""
	}
}
