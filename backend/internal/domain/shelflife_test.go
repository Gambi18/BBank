package domain_test

import (
	"errors"
	"testing"
	"time"

	"bbank/internal/domain"
)

// Each component from foundation §3.3, one case per component so a failure
// names the bag that would have been mis-dated.
func TestShelfLifePerComponent(t *testing.T) {
	p := seededPolicies(t)

	cases := []struct {
		component domain.Component
		hours     float64
		days      float64
		storage   domain.StorageRange
	}{
		{domain.ComponentWholeBlood, 840, 35, domain.StorageRange{MinC: 1, MaxC: 6}},
		{domain.ComponentPackedRedCells, 1008, 42, domain.StorageRange{MinC: 1, MaxC: 6}},
		{domain.ComponentPlatelets, 120, 5, domain.StorageRange{MinC: 20, MaxC: 24}},
		{domain.ComponentFreshFrozenPlasm, 8760, 365, domain.StorageRange{MinC: -80, MaxC: -18}},
		{domain.ComponentCryoprecipitate, 8760, 365, domain.StorageRange{MinC: -80, MaxC: -18}},
	}

	for _, c := range cases {
		t.Run(string(c.component), func(t *testing.T) {
			sl, err := p.ShelfLife(c.component)
			if err != nil {
				t.Fatalf("shelf life: %v", err)
			}
			if got := sl.Duration.Hours(); got != c.hours {
				t.Errorf("duration = %v hours, want %v", got, c.hours)
			}
			if got := sl.Duration.Hours() / 24; got != c.days {
				t.Errorf("duration = %v days, want %v", got, c.days)
			}
			if sl.Storage != c.storage {
				t.Errorf("storage = %+v, want %+v", sl.Storage, c.storage)
			}
		})
	}
}

// Platelets are the reason shelf life is stored in HOURS: 5 days and the
// 7-day bacterial-testing variant differ by 48 hours, and a day-granular column
// could not express a change between them without a second unit column.
func TestPlateletBacterialTestingVariant(t *testing.T) {
	tested := seededPolicies(t, map[domain.PolicyKey]string{
		domain.ShelfLifeKey(domain.ComponentPlatelets): `{"hours":168,"storage_c":[20,24],"note":"7 days with bacterial testing"}`,
	})
	sl, err := tested.ShelfLife(domain.ComponentPlatelets)
	if err != nil {
		t.Fatalf("shelf life: %v", err)
	}
	if got := sl.Duration.Hours() / 24; got != 7 {
		t.Errorf("tested platelets last %v days, want 7", got)
	}

	// And the change must be a policy edit, not a code change: the untested
	// figure is still 5 days from the untouched snapshot.
	base, err := seededPolicies(t).ShelfLife(domain.ComponentPlatelets)
	if err != nil {
		t.Fatalf("shelf life: %v", err)
	}
	if got := base.Duration.Hours() / 24; got != 5 {
		t.Errorf("untested platelets last %v days, want 5", got)
	}
}

// TRD §13.3 calls a DST-shifted expiry "a real and embarrassing bug".
//
// A component's life is a physical fact: 35 days of refrigeration is 840 hours
// whatever the calendar does. Adding 35 CALENDAR days across a spring change
// gives 839 hours — an hour of shelf life invented — and across an autumn change
// gives 841, an hour of expired product still available for issue.
func TestExpiryIsUnaffectedByDaylightSaving(t *testing.T) {
	london, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Skipf("no tzdata for Europe/London: %v", err)
	}

	sl, err := seededPolicies(t).ShelfLife(domain.ComponentWholeBlood)
	if err != nil {
		t.Fatalf("shelf life: %v", err)
	}

	cases := []struct {
		name      string
		collected time.Time
	}{
		// BST starts on 2026-03-29: the clocks go forward an hour.
		{"across the spring change", time.Date(2026, time.March, 20, 10, 0, 0, 0, london)},
		// BST ends on 2026-10-25: the clocks go back an hour.
		{"across the autumn change", time.Date(2026, time.October, 15, 10, 0, 0, 0, london)},
		{"no change in the window", time.Date(2026, time.June, 1, 10, 0, 0, 0, london)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			expires := sl.ExpiresAt(c.collected)
			if got := expires.Sub(c.collected); got != sl.Duration {
				t.Errorf("elapsed = %v, want exactly %v — the expiry moved with the clocks", got, sl.Duration)
			}

			// The calendar arithmetic this function deliberately does NOT use,
			// shown failing, so the test explains itself if it ever regresses.
			calendar := c.collected.AddDate(0, 0, 35)
			if drift := calendar.Sub(expires); drift != 0 {
				t.Logf("AddDate(0,0,35) would have drifted by %v here — this is the bug being prevented", drift)
			}
		})
	}
}

// A leap day inside the window must not add or lose a day either. 12 months of
// frozen plasma is 8760 hours — 365 days — and February 29th does not extend it.
func TestExpiryAcrossALeapDay(t *testing.T) {
	sl, err := seededPolicies(t).ShelfLife(domain.ComponentFreshFrozenPlasm)
	if err != nil {
		t.Fatalf("shelf life: %v", err)
	}

	// 2028 is a leap year; this window contains 2028-02-29.
	collected := time.Date(2027, time.December, 1, 8, 0, 0, 0, time.UTC)
	expires := sl.ExpiresAt(collected)

	if got := expires.Sub(collected); got != sl.Duration {
		t.Errorf("elapsed = %v, want exactly %v", got, sl.Duration)
	}
	// 365 × 24h from 2027-12-01 lands on 2028-11-30, not 2028-12-01, precisely
	// because the leap day is inside the window. That is correct: the policy
	// says 8760 hours, not "one calendar year".
	if got, want := expires.Format("2006-01-02"), "2028-11-30"; got != want {
		t.Errorf("expiry = %s, want %s", got, want)
	}
}

func TestRemainingAtGoesNegativeAfterExpiry(t *testing.T) {
	sl, err := seededPolicies(t).ShelfLife(domain.ComponentPlatelets)
	if err != nil {
		t.Fatalf("shelf life: %v", err)
	}
	collected := time.Date(2026, time.September, 1, 9, 0, 0, 0, time.UTC)

	if got := sl.RemainingAt(collected, collected.Add(24*time.Hour)); got != 96*time.Hour {
		t.Errorf("remaining after a day = %v, want 96h", got)
	}
	// Negative rather than clamped at zero: "expired an hour ago" and "expired
	// last month" are different operational situations.
	if got := sl.RemainingAt(collected, collected.Add(130*time.Hour)); got != -10*time.Hour {
		t.Errorf("remaining after expiry = %v, want -10h", got)
	}
}

// Every component must have a shelf life. A partial map is how a unit ends up
// with no expiry date, and a unit with no expiry never expires.
func TestAllShelfLivesRefusesAPartialSet(t *testing.T) {
	full, err := seededPolicies(t).AllShelfLives()
	if err != nil {
		t.Fatalf("the seeded set is incomplete: %v", err)
	}
	if len(full) != len(domain.Components()) {
		t.Fatalf("got %d shelf lives, want %d", len(full), len(domain.Components()))
	}

	missing := seededPolicies(t, map[domain.PolicyKey]string{
		domain.ShelfLifeKey(domain.ComponentPlatelets): "",
	})
	if _, err := missing.AllShelfLives(); !errors.Is(err, domain.ErrPolicyMissing) {
		t.Errorf("a set missing platelets returned %v, want ErrPolicyMissing", err)
	}
}

// A malformed row must not reach a bag label as a zero-length shelf life.
func TestMalformedShelfLifeIsRejected(t *testing.T) {
	cases := map[string]string{
		"no hours":          `{"storage_c":[1,6]}`,
		"zero hours":        `{"hours":0,"storage_c":[1,6]}`,
		"negative hours":    `{"hours":-24,"storage_c":[1,6]}`,
		"no storage range":  `{"hours":840}`,
		"one-ended storage": `{"hours":840,"storage_c":[4]}`,
		"reversed storage":  `{"hours":840,"storage_c":[6,1]}`,
		"hours as a string": `{"hours":"840","storage_c":[1,6]}`,
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			p := seededPolicies(t, map[domain.PolicyKey]string{
				domain.ShelfLifeKey(domain.ComponentWholeBlood): value,
			})
			if _, err := p.ShelfLife(domain.ComponentWholeBlood); err == nil {
				t.Fatal("a malformed shelf-life policy was accepted")
			} else if !errors.Is(err, domain.ErrPolicyMalformed) {
				t.Errorf("error = %v, want ErrPolicyMalformed", err)
			}
		})
	}
}
