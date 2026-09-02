package domain_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"bbank/internal/domain"
)

// seededPolicies builds a snapshot from the VALUES ACTUALLY SEEDED by migration
// 000012 (schema §12.1), verbatim.
//
// Verbatim matters. A test that invents its own thresholds proves the arithmetic
// and nothing about the system: it would keep passing if the seed shipped 5 for
// the donation interval. `TestSeededPolicyValuesMatchTheMigration` in
// policy_test.go pins these strings against the migration file itself, so this
// helper cannot drift away from production without a test naming the drift.
func seededPolicyFixture() map[domain.PolicyKey]string {
	return map[domain.PolicyKey]string{
		domain.KeyDonorAgeYears:                               `{"min":18,"max":65,"first_time_max":60}`,
		domain.KeyDonorMinWeightKg:                            `{"kg":50}`,
		domain.KeyDonorMinHemoglobin:                          `{"female":12.5,"male":13.0}`,
		domain.KeyDonorVitalsRange:                            `{"bp_systolic":{"min":90,"max":180},"bp_diastolic":{"min":50,"max":100},"pulse_bpm":{"min":50,"max":100},"temperature_c":{"max":37.5}}`,
		domain.KeyDonationsPerYearMax:                         `{"male":6,"female":4,"apheresis_platelet":24}`,
		domain.IntervalKey(domain.ProcedureWholeBlood):        `{"days":56}`,
		domain.IntervalKey(domain.ProcedureApheresisPlatelet): `{"days":7}`,
		domain.ShelfLifeKey(domain.ComponentWholeBlood):       `{"hours":840,"storage_c":[1,6],"note":"35 days, CPDA-1"}`,
		domain.ShelfLifeKey(domain.ComponentPackedRedCells):   `{"hours":1008,"storage_c":[1,6],"note":"42 days, SAGM/AS-1"}`,
		domain.ShelfLifeKey(domain.ComponentPlatelets):        `{"hours":120,"storage_c":[20,24],"note":"5 days; 7 with bacterial testing"}`,
		domain.ShelfLifeKey(domain.ComponentFreshFrozenPlasm): `{"hours":8760,"storage_c":[-80,-18],"note":"12 months"}`,
		domain.ShelfLifeKey(domain.ComponentCryoprecipitate):  `{"hours":8760,"storage_c":[-80,-18],"note":"12 months"}`,
		domain.KeyExpiryAlertHours:                            `{"hours":72}`,
		domain.KeyAllocationMinRemaining:                      `{"hours":4}`,
	}
}

func seededPolicies(t *testing.T, overrides ...map[domain.PolicyKey]string) *domain.Policies {
	t.Helper()
	values := seededPolicyFixture()
	for _, o := range overrides {
		for k, v := range o {
			if v == "" {
				delete(values, k)
				continue
			}
			values[k] = v
		}
	}

	raw := make(map[domain.PolicyKey]json.RawMessage, len(values))
	rows := make([]domain.PolicyRow, 0, len(values))
	for k, v := range values {
		raw[k] = json.RawMessage(v)
		rows = append(rows, domain.PolicyRow{Key: k, Region: "*", Value: json.RawMessage(v)})
	}
	return domain.NewPolicies(domain.PolicyVersion(rows), raw)
}

// day is a date at noon: eligibility is counted in whole days, and starting the
// fixtures at midday makes an accidental timezone shift visible as a failure
// rather than hiding inside the same date.
func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 12, 0, 0, 0, time.UTC)
}

// eligibleDonor is a donor who passes everything, so each test can break exactly
// one thing and know that is what it measured.
func eligibleDonor() domain.DonorFacts {
	return domain.DonorFacts{
		DateOfBirth:   day(1990, time.June, 15),
		Gender:        domain.GenderFemale,
		AccountActive: true,
		FirstTime:     false,
		Procedure:     domain.ProcedureWholeBlood,
	}
}

func evaluate(t *testing.T, f domain.DonorFacts, p *domain.Policies, on time.Time) domain.Decision {
	t.Helper()
	d, err := domain.EvaluateEligibility(f, p, on)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	return d
}

func TestBaselineDonorIsEligible(t *testing.T) {
	d := evaluate(t, eligibleDonor(), seededPolicies(t), day(2026, time.September, 2))
	if !d.Eligible {
		t.Fatalf("the baseline donor is ineligible: %+v", d.Failures)
	}
	if d.Reason() != "eligible" {
		t.Errorf("Reason() = %q, want eligible", d.Reason())
	}
	if d.PolicyVersion == "" {
		t.Error("the decision carries no policy version")
	}
}

// TRD §13.3: age boundaries 17/18/65/66, and the first-time cap at 60.
//
// One case per boundary rather than a loop over a range, so a failure names the
// exact birthday that broke.
func TestAgeBoundaries(t *testing.T) {
	on := day(2026, time.September, 2)
	p := seededPolicies(t)

	cases := []struct {
		name      string
		dob       time.Time
		firstTime bool
		want      domain.Criterion // "" means eligible
	}{
		// The day BEFORE an eighteenth birthday, and the birthday itself. A
		// donor refused on the morning of their eighteenth is the defect
		// AgeYears was fixed for; this pins it from the eligibility side too.
		{"17, the day before turning 18", day(2008, time.September, 3), false, domain.CriterionUnderAge},
		{"18 exactly, on the birthday", day(2008, time.September, 2), false, ""},
		{"18 and a day", day(2008, time.September, 1), false, ""},

		{"65, the last eligible year", day(1961, time.September, 2), false, ""},
		{"65 and 364 days", day(1960, time.September, 3), false, ""},
		{"66, one day over", day(1960, time.September, 2), false, domain.CriterionOverAge},

		// The first-time narrowing: 60 is accepted, 61 is not — and the
		// criterion must be the first-time one, not `over_age`, because the
		// donor is inside the general band.
		{"first-time at 60", day(1966, time.September, 2), true, ""},
		{"first-time at 61", day(1965, time.September, 2), true, domain.CriterionFirstTimeOverAge},
		{"returning donor at 61", day(1965, time.September, 2), false, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := eligibleDonor()
			f.DateOfBirth, f.FirstTime = c.dob, c.firstTime
			d := evaluate(t, f, p, on)

			if c.want == "" {
				if !d.Eligible {
					t.Fatalf("want eligible, got %v (age %d)", d.Failures, domain.AgeYears(c.dob, on))
				}
				return
			}
			if !d.Has(c.want) {
				t.Fatalf("want %s, got %+v (age %d)", c.want, d.Failures, domain.AgeYears(c.dob, on))
			}
		})
	}
}

// TRD §13.3: the 56-day interval at 55, 56 and 57 days.
//
// 56 is the boundary and must be ELIGIBLE: the policy says "at least 56 days
// between donations", so the 56th day is the first day that satisfies it. Off by
// one here either bleeds a donor a day early or turns them away for nothing.
func TestWholeBloodIntervalBoundary(t *testing.T) {
	p := seededPolicies(t)
	last := day(2026, time.July, 1)

	for _, c := range []struct {
		days int
		want bool
	}{
		{54, false},
		{55, false},
		{56, true},
		{57, true},
	} {
		f := eligibleDonor()
		f.LastDonationAt = &last
		on := last.AddDate(0, 0, c.days)

		d := evaluate(t, f, p, on)
		if d.Eligible != c.want {
			t.Errorf("%d days after the last donation: eligible = %v, want %v (%+v)",
				c.days, d.Eligible, c.want, d.Failures)
		}
		if !c.want {
			if !d.Has(domain.CriterionIntervalNotElapsed) {
				t.Errorf("%d days: want interval_not_elapsed, got %+v", c.days, d.Failures)
			}
			// The donor must be told WHEN, not just no.
			if d.NextEligibleOn == nil {
				t.Errorf("%d days: no next-eligible date given", c.days)
			} else if got, want := d.NextEligibleOn.Format("2006-01-02"), last.AddDate(0, 0, 56).Format("2006-01-02"); got != want {
				t.Errorf("%d days: next eligible %s, want %s", c.days, got, want)
			}
		}
	}
}

// Apheresis platelets have their own, much shorter interval. The whole-blood
// figure must not be applied to them — 56 days would cost a platelet donor
// seven donations a year.
func TestApheresisPlateletIntervalIsItsOwn(t *testing.T) {
	p := seededPolicies(t)
	last := day(2026, time.August, 1)

	f := eligibleDonor()
	f.Procedure = domain.ProcedureApheresisPlatelet
	f.LastDonationAt = &last

	if d := evaluate(t, f, p, last.AddDate(0, 0, 6)); d.Eligible {
		t.Error("a platelet donor was eligible 6 days after donating; the interval is 7")
	}
	if d := evaluate(t, f, p, last.AddDate(0, 0, 7)); !d.Eligible {
		t.Errorf("a platelet donor was refused on day 7: %+v", d.Failures)
	}
	// And the whole-blood interval must not have been used by accident.
	if d := evaluate(t, f, p, last.AddDate(0, 0, 30)); !d.Eligible {
		t.Errorf("day 30 refused for a platelet donor — the whole-blood interval leaked: %+v", d.Failures)
	}
}

// TRD §13.3: annual caps, 6 male / 4 female, and the apheresis figure.
func TestAnnualCaps(t *testing.T) {
	p := seededPolicies(t)
	on := day(2026, time.September, 2)

	cases := []struct {
		name      string
		gender    domain.Gender
		procedure domain.Procedure
		count     int
		want      bool
	}{
		{"female, 3 of 4", domain.GenderFemale, domain.ProcedureWholeBlood, 3, true},
		{"female, 4 of 4", domain.GenderFemale, domain.ProcedureWholeBlood, 4, false},
		{"male, 5 of 6", domain.GenderMale, domain.ProcedureWholeBlood, 5, true},
		{"male, 6 of 6", domain.GenderMale, domain.ProcedureWholeBlood, 6, false},

		// An unstated gender takes the STRICTER cap. Declining to say must not
		// be the way to get two more donations a year.
		{"undisclosed, 3 of 4", domain.GenderUndisclosed, domain.ProcedureWholeBlood, 3, true},
		{"undisclosed, 4 — the female cap applies", domain.GenderUndisclosed, domain.ProcedureWholeBlood, 4, false},
		{"other, 4 — the female cap applies", domain.GenderOther, domain.ProcedureWholeBlood, 4, false},

		// Apheresis states its own cap, which does not vary by sex.
		{"female apheresis, 23 of 24", domain.GenderFemale, domain.ProcedureApheresisPlatelet, 23, true},
		{"female apheresis, 24 of 24", domain.GenderFemale, domain.ProcedureApheresisPlatelet, 24, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := eligibleDonor()
			f.Gender, f.Procedure, f.DonationsLast12M = c.gender, c.procedure, c.count
			d := evaluate(t, f, p, on)
			if d.Eligible != c.want {
				t.Fatalf("eligible = %v, want %v (%+v)", d.Eligible, c.want, d.Failures)
			}
			if !c.want && !d.Has(domain.CriterionAnnualCapReached) {
				t.Errorf("want annual_cap_reached, got %+v", d.Failures)
			}
		})
	}
}

// A procedure with no configured cap is uncapped, not capped at zero. Reading a
// missing key as 0 would block every plasma donation ever attempted.
func TestAProcedureWithNoCapIsUncapped(t *testing.T) {
	f := eligibleDonor()
	f.Procedure = domain.ProcedureApheresisPlasma
	f.DonationsLast12M = 40

	p := seededPolicies(t, map[domain.PolicyKey]string{
		domain.IntervalKey(domain.ProcedureApheresisPlasma): `{"days":14}`,
	})
	d := evaluate(t, f, p, day(2026, time.September, 2))
	if !d.Eligible {
		t.Fatalf("an uncapped procedure was blocked by the cap: %+v", d.Failures)
	}
}

// TRD §13.3: weight 49.9 / 50 / 50.1.
func TestWeightBoundary(t *testing.T) {
	p := seededPolicies(t)
	on := day(2026, time.September, 2)

	for _, c := range []struct {
		kg   float64
		want bool
	}{{49.9, false}, {50.0, true}, {50.1, true}} {
		f := eligibleDonor()
		f.Vitals = &domain.Vitals{
			WeightKg: c.kg, HemoglobinGdL: 13.5,
			BPSystolic: 120, BPDiastolic: 80, PulseBPM: 70, TemperatureC: 36.8,
		}
		d := evaluate(t, f, p, on)
		if d.Eligible != c.want {
			t.Errorf("%.1f kg: eligible = %v, want %v (%+v)", c.kg, d.Eligible, c.want, d.Failures)
		}
		if !c.want && !d.Has(domain.CriterionUnderWeight) {
			t.Errorf("%.1f kg: want under_weight, got %+v", c.kg, d.Failures)
		}
	}
}

// TRD §13.3: haemoglobin at the female and male thresholds ±0.1.
func TestHemoglobinBoundaries(t *testing.T) {
	p := seededPolicies(t)
	on := day(2026, time.September, 2)

	cases := []struct {
		gender domain.Gender
		hb     float64
		want   bool
	}{
		{domain.GenderFemale, 12.4, false},
		{domain.GenderFemale, 12.5, true},
		{domain.GenderFemale, 12.6, true},
		{domain.GenderMale, 12.9, false},
		{domain.GenderMale, 13.0, true},
		{domain.GenderMale, 13.1, true},

		// Unstated takes the HIGHER floor — the stricter test. The threshold
		// protects the donor from being bled while anaemic, so an unknown must
		// not be the easier path.
		{domain.GenderUndisclosed, 12.9, false},
		{domain.GenderUndisclosed, 13.0, true},
		{domain.GenderOther, 12.9, false},
	}

	for _, c := range cases {
		f := eligibleDonor()
		f.Gender = c.gender
		f.Vitals = &domain.Vitals{
			WeightKg: 70, HemoglobinGdL: c.hb,
			BPSystolic: 120, BPDiastolic: 80, PulseBPM: 70, TemperatureC: 36.8,
		}
		d := evaluate(t, f, p, on)
		if d.Eligible != c.want {
			t.Errorf("%s at %.1f g/dL: eligible = %v, want %v (%+v)", c.gender, c.hb, d.Eligible, c.want, d.Failures)
		}
	}
}

// TRD §13.3: BP and pulse bounds, and the temperature ceiling.
func TestVitalsBounds(t *testing.T) {
	p := seededPolicies(t)
	on := day(2026, time.September, 2)

	base := domain.Vitals{
		WeightKg: 70, HemoglobinGdL: 13.5,
		BPSystolic: 120, BPDiastolic: 80, PulseBPM: 70, TemperatureC: 36.8,
	}

	cases := []struct {
		name   string
		mutate func(*domain.Vitals)
		want   domain.Criterion
	}{
		{"systolic at the floor", func(v *domain.Vitals) { v.BPSystolic = 90 }, ""},
		{"systolic below the floor", func(v *domain.Vitals) { v.BPSystolic = 89 }, domain.CriterionBloodPressure},
		{"systolic at the ceiling", func(v *domain.Vitals) { v.BPSystolic = 180 }, ""},
		{"systolic above the ceiling", func(v *domain.Vitals) { v.BPSystolic = 181 }, domain.CriterionBloodPressure},
		{"diastolic at the floor", func(v *domain.Vitals) { v.BPDiastolic = 50 }, ""},
		{"diastolic below the floor", func(v *domain.Vitals) { v.BPDiastolic = 49 }, domain.CriterionBloodPressure},
		{"diastolic above the ceiling", func(v *domain.Vitals) { v.BPDiastolic = 101 }, domain.CriterionBloodPressure},
		{"pulse at the floor", func(v *domain.Vitals) { v.PulseBPM = 50 }, ""},
		{"pulse below the floor", func(v *domain.Vitals) { v.PulseBPM = 49 }, domain.CriterionPulse},
		{"pulse at the ceiling", func(v *domain.Vitals) { v.PulseBPM = 100 }, ""},
		{"pulse above the ceiling", func(v *domain.Vitals) { v.PulseBPM = 101 }, domain.CriterionPulse},
		{"temperature at the ceiling", func(v *domain.Vitals) { v.TemperatureC = 37.5 }, ""},
		{"temperature above the ceiling", func(v *domain.Vitals) { v.TemperatureC = 37.6 }, domain.CriterionTemperature},
		// The temperature policy states a max and no min, so a low reading is
		// not an eligibility failure here — hypothermia is a clinical judgement,
		// not a threshold this system owns.
		{"a low temperature is not bounded by policy", func(v *domain.Vitals) { v.TemperatureC = 35.0 }, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := base
			c.mutate(&v)
			f := eligibleDonor()
			f.Vitals = &v
			d := evaluate(t, f, p, on)

			if c.want == "" {
				if !d.Eligible {
					t.Fatalf("want eligible, got %+v", d.Failures)
				}
				return
			}
			if !d.Has(c.want) {
				t.Fatalf("want %s, got %+v", c.want, d.Failures)
			}
		})
	}
}

// A booking decision has no vitals, and must not invent failures for values
// nobody has measured. This is what lets the same function serve booking
// (WI-26) and screening (WI-42).
func TestBookingWithoutVitalsChecksOnlyWhatIsKnown(t *testing.T) {
	f := eligibleDonor()
	f.Vitals = nil
	d := evaluate(t, f, seededPolicies(t), day(2026, time.September, 2))
	if !d.Eligible {
		t.Fatalf("a booking decision failed on unmeasured vitals: %+v", d.Failures)
	}
}

// A temporary deferral blocks up to but NOT INCLUDING `ends_on`: the donor is
// eligible on that date.
//
// The boundary is exclusive because that is what the database means by it — the
// facts query keeps a deferral only while `ends_on > CURRENT_DATE`. This was
// inclusive here at first, which put the Go a day behind the SQL: the row
// vanished from the query on its end date while the domain would still have
// blocked, and the donor was told to come back a day later than the system would
// itself have allowed. The `donor_eligibility` view had the same split
// internally and migration 000018 corrects it.
func TestTemporaryDeferralBoundary(t *testing.T) {
	p := seededPolicies(t)
	endsOn := day(2026, time.September, 10)

	for _, c := range []struct {
		on   time.Time
		want bool
	}{
		{day(2026, time.September, 8), false},
		{day(2026, time.September, 9), false}, // the last deferred day
		{day(2026, time.September, 10), true}, // ends_on itself: eligible
		{day(2026, time.September, 11), true},
	} {
		f := eligibleDonor()
		f.DeferredUntil = &endsOn
		d := evaluate(t, f, p, c.on)

		if d.Eligible != c.want {
			t.Errorf("on %s: eligible = %v, want %v (%+v)",
				c.on.Format("2006-01-02"), d.Eligible, c.want, d.Failures)
			continue
		}
		if c.want {
			continue
		}
		if !d.Has(domain.CriterionTemporaryDeferral) {
			t.Errorf("on %s: want temporarily_deferred, got %+v", c.on.Format("2006-01-02"), d.Failures)
		}
		// The clearing date is ends_on itself, not the day after.
		if d.NextEligibleOn == nil {
			t.Errorf("on %s: no next-eligible date", c.on.Format("2006-01-02"))
		} else if got := d.NextEligibleOn.Format("2006-01-02"); got != "2026-09-10" {
			t.Errorf("on %s: next eligible = %s, want 2026-09-10 (ends_on itself)",
				c.on.Format("2006-01-02"), got)
		}
	}
}

// A permanent deferral has no clearing date, and the decision must say so by
// leaving NextEligibleOn nil. Returning today's date would let a booking form
// tell a permanently deferred donor to come back tomorrow.
func TestPermanentDeferralHasNoNextEligibleDate(t *testing.T) {
	f := eligibleDonor()
	f.PermanentlyDeferred = true
	d := evaluate(t, f, seededPolicies(t), day(2026, time.September, 2))

	if d.Eligible {
		t.Fatal("a permanently deferred donor was eligible")
	}
	if !d.Has(domain.CriterionPermanentDeferral) {
		t.Fatalf("want permanently_deferred, got %+v", d.Failures)
	}
	if d.NextEligibleOn != nil {
		t.Errorf("a permanent deferral produced a next-eligible date of %v", d.NextEligibleOn)
	}
}

// FR-17: "Each failing criterion is named individually, not as a single
// 'ineligible'." A donor who fails four rules must be told four things.
func TestEveryFailingCriterionIsNamed(t *testing.T) {
	deferred := day(2026, time.September, 30)
	last := day(2026, time.August, 20)

	f := domain.DonorFacts{
		DateOfBirth:      day(2012, time.January, 1), // under age
		Gender:           domain.GenderFemale,
		AccountActive:    false, // account not active
		Procedure:        domain.ProcedureWholeBlood,
		LastDonationAt:   &last,     // interval not elapsed
		DeferredUntil:    &deferred, // temporarily deferred
		DonationsLast12M: 9,         // annual cap
		Vitals: &domain.Vitals{
			WeightKg: 45, HemoglobinGdL: 10, // under weight, low haemoglobin
			BPSystolic: 200, BPDiastolic: 120, PulseBPM: 190, TemperatureC: 39,
		},
	}
	d := evaluate(t, f, seededPolicies(t), day(2026, time.September, 2))

	want := []domain.Criterion{
		domain.CriterionAccountNotActive,
		domain.CriterionTemporaryDeferral,
		domain.CriterionUnderAge,
		domain.CriterionIntervalNotElapsed,
		domain.CriterionAnnualCapReached,
		domain.CriterionUnderWeight,
		domain.CriterionLowHemoglobin,
		domain.CriterionBloodPressure,
		domain.CriterionPulse,
		domain.CriterionTemperature,
	}
	for _, c := range want {
		if !d.Has(c) {
			t.Errorf("criterion %s was not reported; got %+v", c, d.Failures)
		}
	}
	if len(d.Failures) != len(want) {
		t.Errorf("got %d failures, want %d: %+v", len(d.Failures), len(want), d.Failures)
	}

	// The headline is the most severe, and the account status outranks the rest.
	if d.Reason() != domain.CriterionAccountNotActive {
		t.Errorf("Reason() = %s, want account_not_active", d.Reason())
	}
}

// FR-19: "The donor sees a plain-language explanation, not an error code."
// Every failure must carry a sentence, and no sentence may be the code.
func TestEveryFailureCarriesAPlainLanguageMessage(t *testing.T) {
	deferred := day(2026, time.September, 30)
	last := day(2026, time.August, 20)
	f := domain.DonorFacts{
		DateOfBirth:      day(2012, time.January, 1),
		Gender:           domain.GenderFemale,
		AccountActive:    false,
		Procedure:        domain.ProcedureWholeBlood,
		LastDonationAt:   &last,
		DeferredUntil:    &deferred,
		DonationsLast12M: 9,
		Vitals: &domain.Vitals{
			WeightKg: 45, HemoglobinGdL: 10,
			BPSystolic: 200, BPDiastolic: 120, PulseBPM: 190, TemperatureC: 39,
		},
	}
	d := evaluate(t, f, seededPolicies(t), day(2026, time.September, 2))

	for _, fail := range d.Failures {
		if fail.Message == "" {
			t.Errorf("%s has no message", fail.Criterion)
			continue
		}
		if fail.Message == string(fail.Criterion) {
			t.Errorf("%s: the message IS the code", fail.Criterion)
		}
		// A code leaking into prose reads as a bug to the person it is shown to.
		if containsUnderscoreWord(fail.Message) {
			t.Errorf("%s: message contains a code-like token: %q", fail.Criterion, fail.Message)
		}
	}
}

func containsUnderscoreWord(s string) bool {
	for i := 1; i < len(s)-1; i++ {
		if s[i] == '_' && isLetter(s[i-1]) && isLetter(s[i+1]) {
			return true
		}
	}
	return false
}

func isLetter(b byte) bool { return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') }

// WI-25's acceptance criterion: "Changing a `policies` row changes the next
// decision." The same donor, the same day, two different policy sets.
func TestChangingAPolicyChangesTheDecision(t *testing.T) {
	on := day(2026, time.September, 2)
	last := day(2026, time.July, 20) // 44 days earlier

	f := eligibleDonor()
	f.LastDonationAt = &last

	if d := evaluate(t, f, seededPolicies(t), on); d.Eligible {
		t.Fatal("44 days after a donation the donor was eligible under the 56-day policy")
	}

	relaxed := seededPolicies(t, map[domain.PolicyKey]string{
		domain.IntervalKey(domain.ProcedureWholeBlood): `{"days":30}`,
	})
	if d := evaluate(t, f, relaxed, on); !d.Eligible {
		t.Errorf("the donor is still ineligible after the interval was changed to 30 days: %+v", d.Failures)
	}

	// And the two decisions must be distinguishable afterwards, which is what
	// the version stamp is for (FR-68).
	a := evaluate(t, f, seededPolicies(t), on)
	b := evaluate(t, f, relaxed, on)
	if a.PolicyVersion == b.PolicyVersion {
		t.Error("two different policy sets produced the same version stamp")
	}
}

// A missing threshold must stop the decision, not default it. This is the
// difference between "we could not check" and "we checked and you passed".
func TestAMissingPolicyRefusesToDecide(t *testing.T) {
	for _, key := range []domain.PolicyKey{
		domain.KeyDonorAgeYears,
		domain.IntervalKey(domain.ProcedureWholeBlood),
		domain.KeyDonationsPerYearMax,
	} {
		p := seededPolicies(t, map[domain.PolicyKey]string{key: ""})
		_, err := domain.EvaluateEligibility(eligibleDonor(), p, day(2026, time.September, 2))
		if !errors.Is(err, domain.ErrPolicyMissing) {
			t.Errorf("without %s the evaluation returned %v, want ErrPolicyMissing", key, err)
		}
	}

	// The vitals thresholds are only needed when vitals are present, and their
	// absence must fail a SCREENING rather than being silently skipped.
	f := eligibleDonor()
	f.Vitals = &domain.Vitals{WeightKg: 70, HemoglobinGdL: 13.5, BPSystolic: 120, BPDiastolic: 80, PulseBPM: 70, TemperatureC: 36.8}
	for _, key := range []domain.PolicyKey{
		domain.KeyDonorMinWeightKg, domain.KeyDonorMinHemoglobin, domain.KeyDonorVitalsRange,
	} {
		p := seededPolicies(t, map[domain.PolicyKey]string{key: ""})
		if _, err := domain.EvaluateEligibility(f, p, day(2026, time.September, 2)); !errors.Is(err, domain.ErrPolicyMissing) {
			t.Errorf("without %s the screening returned %v, want ErrPolicyMissing", key, err)
		}
	}
}

// The evaluation must not depend on the time of day it runs at. Eligibility is
// counted in whole days, so 00:01 and 23:59 on the same date are the same day.
func TestTheTimeOfDayDoesNotChangeTheAnswer(t *testing.T) {
	p := seededPolicies(t)
	last := time.Date(2026, time.July, 1, 9, 0, 0, 0, time.UTC)

	f := eligibleDonor()
	f.LastDonationAt = &last

	// The 56th day, at both ends of the day.
	early := time.Date(2026, time.August, 26, 0, 1, 0, 0, time.UTC)
	late := time.Date(2026, time.August, 26, 23, 59, 0, 0, time.UTC)

	a, b := evaluate(t, f, p, early), evaluate(t, f, p, late)
	if a.Eligible != b.Eligible {
		t.Errorf("00:01 says %v and 23:59 says %v on the same date", a.Eligible, b.Eligible)
	}

	// And a donation recorded late in the evening must not extend the wait.
	evening := time.Date(2026, time.July, 1, 23, 30, 0, 0, time.UTC)
	f.LastDonationAt = &evening
	if c := evaluate(t, f, p, early); c.Eligible != a.Eligible {
		t.Errorf("a donation at 23:30 gives %v where one at 09:00 gives %v", c.Eligible, a.Eligible)
	}
}
