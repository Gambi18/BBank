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
)

func policySetup(t *testing.T) (*service.PolicyService, *store.Queries, context.Context) {
	t.Helper()
	pool := testsupport.Pool(t)
	q := store.New(pool)
	return service.NewPolicyService(q), q, context.Background()
}

// The seeded database must produce a snapshot the domain can actually decide
// with. The unit tests parse the migration text; this one reads it through
// Postgres, the view, sqlc and the resolver — the path production uses.
func TestResolverReadsTheSeededPolicySet(t *testing.T) {
	svc, _, ctx := policySetup(t)

	p, err := svc.Current(ctx)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if p.Version() == "" {
		t.Error("the snapshot has no version")
	}

	band, err := p.AgeBand()
	if err != nil {
		t.Fatalf("age band: %v", err)
	}
	if band.Min != 18 || band.Max != 65 || band.FirstTimeMax != 60 {
		t.Errorf("age band = %+v, want {18 65 60} from the seed", band)
	}

	days, err := p.IntervalDays(domain.ProcedureWholeBlood)
	if err != nil {
		t.Fatalf("interval: %v", err)
	}
	if days != 56 {
		t.Errorf("whole-blood interval = %d, want 56", days)
	}

	if _, err := p.AllShelfLives(); err != nil {
		t.Errorf("shelf lives: %v", err)
	}
}

// WI-25's acceptance criterion, end to end: "Changing a `policies` row changes
// the next decision."
//
// Not "changes after a redeploy", and not "changes after a restart" — the whole
// argument for policy being data is that an administrator can correct a
// threshold and have it take effect. `Reload` is the path the policy console
// (WI-89) will use after an edit; the TTL is the path everything else takes.
func TestChangingAPolicyRowChangesTheNextDecision(t *testing.T) {
	svc, _, ctx := policySetup(t)
	pool := testsupport.Pool(t)
	testsupport.IsolatePolicies(t, pool)

	before, err := svc.Current(ctx)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got, _ := before.IntervalDays(domain.ProcedureWholeBlood); got != 56 {
		t.Fatalf("baseline interval = %d, want 56", got)
	}

	// The seed dates every row `effective_from = CURRENT_DATE`, and
	// `policies_window` requires `effective_to > effective_from` — so a row
	// created today cannot be closed today. Backdate it first, which is simply
	// what a deployment older than one day looks like.
	//
	// (That constraint is a real operational wrinkle for `WI-89`: an
	// administrator who adds a policy and then corrects it the same day has to
	// delete the row rather than supersede it. Recorded in PROJECT_STATUS.)
	if _, err := pool.Exec(ctx, `
		UPDATE policies SET effective_from = CURRENT_DATE - 30
		 WHERE key = 'donation_interval_days.whole_blood' AND region = '*'`); err != nil {
		t.Fatalf("backdate the current row: %v", err)
	}

	// Close the current row and open a new one, which is how an effective-dated
	// edit works: the old row is not overwritten, so a decision made yesterday
	// can still be explained.
	if _, err := pool.Exec(ctx, `
		UPDATE policies SET effective_to = CURRENT_DATE
		 WHERE key = 'donation_interval_days.whole_blood' AND region = '*'`); err != nil {
		t.Fatalf("close the current row: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO policies (key, value, region, effective_from, description)
		VALUES ('donation_interval_days.whole_blood', '{"days":30}', '*', CURRENT_DATE, 'shortened for this test')`); err != nil {
		t.Fatalf("insert the new row: %v", err)
	}

	after, err := svc.Reload(ctx)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got, _ := after.IntervalDays(domain.ProcedureWholeBlood); got != 30 {
		t.Fatalf("interval after the edit = %d, want 30", got)
	}
	if before.Version() == after.Version() {
		t.Error("the version stamp did not change when a threshold did — a decision made under each would be indistinguishable")
	}

	// And the decision itself moves, which is the point.
	last := time.Now().AddDate(0, 0, -40)
	f := domain.DonorFacts{
		DateOfBirth: time.Now().AddDate(-30, 0, 0), Gender: domain.GenderFemale,
		AccountActive: true, Procedure: domain.ProcedureWholeBlood, LastDonationAt: &last,
	}
	oldDecision, err := domain.EvaluateEligibility(f, before, time.Now())
	if err != nil {
		t.Fatalf("evaluate under the old policy: %v", err)
	}
	newDecision, err := domain.EvaluateEligibility(f, after, time.Now())
	if err != nil {
		t.Fatalf("evaluate under the new policy: %v", err)
	}
	if oldDecision.Eligible {
		t.Error("40 days after donating, the donor was eligible under a 56-day interval")
	}
	if !newDecision.Eligible {
		t.Errorf("40 days after donating, the donor is still ineligible under a 30-day interval: %+v", newDecision.Failures)
	}
}

// A snapshot is reused within the TTL and re-read after it. Without the cache
// every booking puts a query in front of it; without the expiry an
// administrator's correction never lands.
func TestTheSnapshotIsCachedAndThenRefreshed(t *testing.T) {
	svc, _, ctx := policySetup(t)
	pool := testsupport.Pool(t)
	testsupport.IsolatePolicies(t, pool)

	first, err := svc.Current(ctx)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE policies SET effective_from = CURRENT_DATE - 30
		 WHERE key = 'expiry_alert_hours' AND region = '*'`); err != nil {
		t.Fatalf("backdate the row: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE policies SET effective_to = CURRENT_DATE
		 WHERE key = 'expiry_alert_hours' AND region = '*'`); err != nil {
		t.Fatalf("close the row: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO policies (key, value, region, effective_from, description)
		VALUES ('expiry_alert_hours', '{"hours":48}', '*', CURRENT_DATE, 'changed for this test')`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Still inside the TTL: the change must NOT be visible yet, or the cache is
	// not a cache.
	cached, err := svc.Current(ctx)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if cached.Version() != first.Version() {
		t.Error("the snapshot was re-read inside the TTL")
	}

	// Reload is the escape hatch, and it must see it immediately.
	fresh, err := svc.Reload(ctx)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	window, err := fresh.ExpiryAlertWindow()
	if err != nil {
		t.Fatalf("expiry window: %v", err)
	}
	if window != 48*time.Hour {
		t.Errorf("expiry alert window = %v, want 48h after the edit", window)
	}
}

// A deleted threshold must stop decisions, not silently default them.
//
// This is the difference the whole design turns on. If the resolver invented a
// number here, deleting a clinical policy row would be invisible — the system
// would keep deciding, against numbers no one configured.
func TestADeletedPolicyStopsTheDecision(t *testing.T) {
	svc, _, ctx := policySetup(t)
	pool := testsupport.Pool(t)
	testsupport.IsolatePolicies(t, pool)

	if _, err := pool.Exec(ctx, `DELETE FROM policies WHERE key = 'donor_age_years'`); err != nil {
		t.Fatalf("delete: %v", err)
	}
	p, err := svc.Reload(ctx)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	if _, err := p.AgeBand(); !errors.Is(err, domain.ErrPolicyMissing) {
		t.Fatalf("AgeBand with no row = %v, want ErrPolicyMissing", err)
	}

	f := domain.DonorFacts{
		DateOfBirth: time.Now().AddDate(-30, 0, 0), Gender: domain.GenderFemale,
		AccountActive: true, Procedure: domain.ProcedureWholeBlood,
	}
	if _, err := domain.EvaluateEligibility(f, p, time.Now()); !errors.Is(err, domain.ErrPolicyMissing) {
		t.Errorf("the evaluation returned %v with no age policy; it must refuse to decide", err)
	}
}

// An effective-dated row that has not started yet, or has already ended, is not
// active — that is what `active_policies` means, and a decision must not pick
// one up early.
func TestAFuturePolicyIsNotYetActive(t *testing.T) {
	svc, _, ctx := policySetup(t)
	pool := testsupport.Pool(t)
	testsupport.IsolatePolicies(t, pool)

	if _, err := pool.Exec(ctx, `
		UPDATE policies SET effective_from = CURRENT_DATE - 30, effective_to = CURRENT_DATE + 30
		 WHERE key = 'donation_interval_days.whole_blood' AND region = '*'`); err != nil {
		t.Fatalf("bound the current row: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO policies (key, value, region, effective_from, description)
		VALUES ('donation_interval_days.whole_blood', '{"days":90}', '*', CURRENT_DATE + 30, 'takes effect next month')`); err != nil {
		t.Fatalf("insert the future row: %v", err)
	}

	p, err := svc.Reload(ctx)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	days, err := p.IntervalDays(domain.ProcedureWholeBlood)
	if err != nil {
		t.Fatalf("interval: %v", err)
	}
	if days != 56 {
		t.Errorf("interval = %d, want 56 — a policy dated a month out was applied today", days)
	}
}
