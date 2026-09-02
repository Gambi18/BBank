package domain

import (
	"fmt"
	"sort"
	"time"
)

// Criterion names one eligibility rule.
//
// The codes that overlap with the `donor_eligibility` view (schema §8.3) use
// the view's spelling exactly — `account_not_active`, `permanently_deferred`,
// `temporarily_deferred`, `under_age`, `over_age`, `interval_not_elapsed`. Two
// spellings for one rule is how a UI ends up with a branch that never runs, and
// the view is the older of the two so it wins.
type Criterion string

const (
	CriterionAccountNotActive    Criterion = "account_not_active"
	CriterionPermanentDeferral   Criterion = "permanently_deferred"
	CriterionTemporaryDeferral   Criterion = "temporarily_deferred"
	CriterionUnderAge            Criterion = "under_age"
	CriterionOverAge             Criterion = "over_age"
	CriterionFirstTimeOverAge    Criterion = "first_time_over_age"
	CriterionIntervalNotElapsed  Criterion = "interval_not_elapsed"
	CriterionAnnualCapReached    Criterion = "annual_cap_reached"
	CriterionUnderWeight         Criterion = "under_weight"
	CriterionLowHemoglobin       Criterion = "low_hemoglobin"
	CriterionBloodPressure       Criterion = "blood_pressure_out_of_range"
	CriterionPulse               Criterion = "pulse_out_of_range"
	CriterionTemperature         Criterion = "temperature_out_of_range"
	CriterionIncompleteScreening Criterion = "screening_incomplete"
)

// Failure is one reason a donor cannot donate, in language a donor can read.
//
// `FR-19` requires "a plain-language explanation, not an error code", and
// `FR-17` requires each failing criterion to be "named individually, not as a
// single 'ineligible'". So a failure carries both: the code for the UI to
// branch on and the sentence for a person to read. `ClearsOn` is set whenever
// waiting is the answer, which is what turns a refusal into an appointment the
// donor can actually make.
type Failure struct {
	Criterion Criterion
	Message   string
	ClearsOn  *time.Time
}

// Decision is the outcome of an eligibility evaluation.
//
// `PolicyVersion` is not decoration. A decision recorded without the version of
// the numbers that produced it cannot be explained once those numbers change,
// and `FR-68` requires that changing a policy "never rewrites decisions already
// made under the previous version" — which is only checkable if each decision
// says which version it used.
type Decision struct {
	Eligible       bool
	PolicyVersion  string
	NextEligibleOn *time.Time
	Failures       []Failure
}

// Reason is the single headline code, for callers that can show only one — the
// list view, the view's `reason` column. It is the FIRST failure in severity
// order, never a summary: the full list is always in `Failures`.
func (d Decision) Reason() Criterion {
	if len(d.Failures) == 0 {
		return "eligible"
	}
	return d.Failures[0].Criterion
}

// Has reports whether a specific criterion failed.
func (d Decision) Has(c Criterion) bool {
	for _, f := range d.Failures {
		if f.Criterion == c {
			return true
		}
	}
	return false
}

// Vitals are the measured values a screening records. Nil on a booking
// decision, where nobody has measured anything yet.
type Vitals struct {
	HemoglobinGdL float64
	WeightKg      float64
	BPSystolic    float64
	BPDiastolic   float64
	PulseBPM      float64
	TemperatureC  float64
}

// DonorFacts is everything an eligibility decision depends on.
//
// Every field is something the system OBSERVED. There is deliberately no
// "last donation" a donor can type: `D4` in the schema review is the defect
// where eligibility — a safety decision — was computed from a free-text date
// the donor could edit, so a second donation the next day was a form field
// away. `LastDonationAt` here comes from real `donations` rows joined to a
// completed appointment, and nothing else may populate it.
type DonorFacts struct {
	DateOfBirth   time.Time
	Gender        Gender
	AccountActive bool

	// FirstTime narrows the age band to `first_time_max`. A donor with no
	// completed donation is a first-time donor.
	FirstTime bool

	Procedure Procedure

	// LastDonationAt is the most recent completed donation OF THE SAME
	// PROCEDURE. Nil when there is none.
	LastDonationAt   *time.Time
	DonationsLast12M int

	PermanentlyDeferred bool
	// DeferredUntil is `deferrals.ends_on`, and it is EXCLUSIVE: the donor is
	// blocked up to but not including this date, and is eligible ON it.
	//
	// Exclusive because that is what the database already means by it. The
	// query keeps a deferral only while `ends_on > CURRENT_DATE`, so a deferral
	// ending today is not active today — the same half-open reading the schema
	// uses for every `daterange`, and the reading `deferrals_window
	// CHECK (ends_on > starts_on)` implies.
	//
	// This was inclusive here at first, which put the Go and the SQL a day
	// apart: the row vanished from the query on its end date while the domain
	// would still have blocked on it, and the donor was told to come back a day
	// later than the system would itself have allowed. The `donor_eligibility`
	// view had the same split internally — it filtered exclusively and then
	// reported `deferred_until + 1` — and migration 000018 corrects it.
	DeferredUntil *time.Time

	// Vitals is nil for a booking decision and set for a screening decision.
	// The distinction matters: booking must not refuse a donor for a
	// haemoglobin nobody has measured yet.
	Vitals *Vitals
}

// EvaluateEligibility decides whether a donor may donate on the given day.
//
// It collects EVERY failing criterion rather than returning at the first, which
// is `FR-17`'s acceptance criterion and also the difference between "you cannot
// donate" and a list a donor can act on. Short-circuiting would also make the
// order of the checks clinically meaningful, which it is not.
//
// `on` is the day the decision is for — normally today, but a booking asks
// about a future date, and asking "will this donor be eligible on the 14th?" is
// the whole point of letting them book ahead.
//
// It returns an error only when the POLICY cannot be read. A donor who fails
// every rule is a valid decision; a system that cannot find its age band is
// not, and must not answer "ineligible" as though it had checked.
func EvaluateEligibility(f DonorFacts, p *Policies, on time.Time) (Decision, error) {
	d := Decision{PolicyVersion: p.Version()}
	on = startOfDay(on)

	if !f.AccountActive {
		d.add(Failure{
			Criterion: CriterionAccountNotActive,
			Message:   "This account is not active. Please contact the donation centre.",
		})
	}

	if f.PermanentlyDeferred {
		d.add(Failure{
			Criterion: CriterionPermanentDeferral,
			Message:   "A clinician has recorded a permanent deferral on this record. Please speak to the donation centre — they can explain what it means and whether it can be reviewed.",
		})
	}

	// A temporary deferral blocks up to but NOT INCLUDING its end date: the
	// donor is eligible on `ends_on` itself. See DonorFacts.DeferredUntil — the
	// same boundary the query and the view use, so the three cannot disagree
	// about which day a donor gets their life back.
	if f.DeferredUntil != nil {
		clears := startOfDay(*f.DeferredUntil)
		if on.Before(clears) {
			d.add(Failure{
				Criterion: CriterionTemporaryDeferral,
				Message: fmt.Sprintf("You are temporarily deferred. You can book again from %s.",
					formatDay(clears)),
				ClearsOn: &clears,
			})
		}
	}

	band, err := p.AgeBand()
	if err != nil {
		return Decision{}, err
	}
	age := AgeYears(f.DateOfBirth, on)
	switch {
	case age < band.Min:
		// Waiting works here, and saying so is kinder and more useful than a
		// bare refusal to a 17-year-old who wants to help.
		turns := f.DateOfBirth.AddDate(band.Min, 0, 0)
		clears := startOfDay(turns)
		d.add(Failure{
			Criterion: CriterionUnderAge,
			Message: fmt.Sprintf("Donors must be at least %d. You can donate from %s.",
				band.Min, formatDay(clears)),
			ClearsOn: &clears,
		})
	case age > band.Max:
		d.add(Failure{
			Criterion: CriterionOverAge,
			Message:   fmt.Sprintf("Regular donation is for donors up to %d. Thank you for everything you have given.", band.Max),
		})
	case f.FirstTime && age > band.FirstTimeMax:
		// A separate criterion, not "over_age": this donor is inside the
		// general band and blocked only by the first-time narrowing, which is a
		// different conversation to have with them.
		d.add(Failure{
			Criterion: CriterionFirstTimeOverAge,
			Message: fmt.Sprintf("A first donation is accepted up to age %d. Please speak to the centre — donors who have given before may still be eligible.",
				band.FirstTimeMax),
		})
	}

	interval, err := p.IntervalDays(f.Procedure)
	if err != nil {
		return Decision{}, err
	}
	if f.LastDonationAt != nil {
		next := startOfDay(*f.LastDonationAt).AddDate(0, 0, interval)
		if on.Before(next) {
			d.add(Failure{
				Criterion: CriterionIntervalNotElapsed,
				Message: fmt.Sprintf("There must be at least %d days between donations. Your next donation can be on %s.",
					interval, formatDay(next)),
				ClearsOn: &next,
			})
		}
	}

	cap, capped, err := p.AnnualCap(f.Gender, f.Procedure)
	if err != nil {
		return Decision{}, err
	}
	if capped && f.DonationsLast12M >= cap {
		// No ClearsOn. The cap is a ROLLING twelve months, and the date it
		// clears depends on when the oldest of those donations was — which is
		// not in DonorFacts. Naming a date this function cannot compute would
		// be worse than naming none, so it says what it knows.
		d.add(Failure{
			Criterion: CriterionAnnualCapReached,
			Message: fmt.Sprintf("You have made %d donations in the last twelve months, which is the maximum of %d. You can donate again twelve months after your earliest donation in that period.",
				f.DonationsLast12M, cap),
		})
	}

	if err := evaluateVitals(&d, f, p); err != nil {
		return Decision{}, err
	}

	sortFailures(d.Failures)
	d.Eligible = len(d.Failures) == 0
	d.NextEligibleOn = nextEligibleOn(d, on)
	return d, nil
}

// evaluateVitals applies the measured checks. A booking decision has no vitals
// and skips them entirely — refusing a booking for a haemoglobin nobody has
// taken would be refusing for a fact that does not exist.
func evaluateVitals(d *Decision, f DonorFacts, p *Policies) error {
	if f.Vitals == nil {
		return nil
	}
	v := *f.Vitals

	minWeight, err := p.MinWeightKg()
	if err != nil {
		return err
	}
	if v.WeightKg < minWeight {
		d.add(Failure{
			Criterion: CriterionUnderWeight,
			Message:   fmt.Sprintf("Donors must weigh at least %s kg.", trimFloat(minWeight)),
		})
	}

	minHb, err := p.MinHemoglobin(f.Gender)
	if err != nil {
		return err
	}
	if v.HemoglobinGdL < minHb {
		d.add(Failure{
			Criterion: CriterionLowHemoglobin,
			Message: fmt.Sprintf("Your haemoglobin today is %s g/dL and the minimum is %s. This protects you, not the donation — it is common, usually temporary, and worth mentioning to your doctor if it repeats.",
				trimFloat(v.HemoglobinGdL), trimFloat(minHb)),
		})
	}

	ranges, err := p.VitalsRange()
	if err != nil {
		return err
	}
	// Systolic and diastolic are one conversation, so they are one failure:
	// telling somebody their blood pressure is out of range twice is not twice
	// as informative.
	if !ranges.BPSystolic.Contains(v.BPSystolic) || !ranges.BPDiastolic.Contains(v.BPDiastolic) {
		d.add(Failure{
			Criterion: CriterionBloodPressure,
			Message: fmt.Sprintf("Your blood pressure today (%s/%s) is outside the range for donating.",
				trimFloat(v.BPSystolic), trimFloat(v.BPDiastolic)),
		})
	}
	if !ranges.Pulse.Contains(v.PulseBPM) {
		d.add(Failure{
			Criterion: CriterionPulse,
			Message:   fmt.Sprintf("Your pulse today (%s bpm) is outside the range for donating.", trimFloat(v.PulseBPM)),
		})
	}
	if !ranges.Temperature.Contains(v.TemperatureC) {
		d.add(Failure{
			Criterion: CriterionTemperature,
			Message:   fmt.Sprintf("Your temperature today (%s °C) is outside the range for donating.", trimFloat(v.TemperatureC)),
		})
	}
	return nil
}

func (d *Decision) add(f Failure) { d.Failures = append(d.Failures, f) }

// severity orders failures so the headline `Reason()` is the most consequential
// one. It is presentation order only — every failure is reported either way.
func severity(c Criterion) int {
	switch c {
	case CriterionAccountNotActive:
		return 0
	case CriterionPermanentDeferral:
		return 1
	case CriterionTemporaryDeferral:
		return 2
	case CriterionUnderAge:
		return 3
	case CriterionOverAge:
		return 4
	case CriterionFirstTimeOverAge:
		return 5
	case CriterionIntervalNotElapsed:
		return 6
	case CriterionAnnualCapReached:
		return 7
	case CriterionLowHemoglobin:
		return 8
	case CriterionUnderWeight:
		return 9
	case CriterionBloodPressure:
		return 10
	case CriterionPulse:
		return 11
	case CriterionTemperature:
		return 12
	default:
		return 99
	}
}

func sortFailures(fs []Failure) {
	sort.SliceStable(fs, func(i, j int) bool { return severity(fs[i].Criterion) < severity(fs[j].Criterion) })
}

// NextEligibleOn recomputes the clearing date for a decision whose failure list
// has been narrowed — an admin override removing a permanent deferral is the one
// case today. Exported for that caller: the field is only correct for the
// failures actually left.
func NextEligibleOn(d Decision, on time.Time) *time.Time { return nextEligibleOn(d, startOfDay(on)) }

// nextEligibleOn is the earliest day every time-clearing failure has passed.
//
// A failure with no `ClearsOn` — a permanent deferral, an age ceiling, an
// account that is not active — means there is no such day, and the answer is
// nil. Returning "today" for a permanently deferred donor would be a lie the
// booking form would act on.
func nextEligibleOn(d Decision, on time.Time) *time.Time {
	if len(d.Failures) == 0 {
		day := on
		return &day
	}
	latest := on
	for _, f := range d.Failures {
		if f.ClearsOn == nil {
			return nil
		}
		if f.ClearsOn.After(latest) {
			latest = *f.ClearsOn
		}
	}
	return &latest
}

// startOfDay drops the clock, keeping the location.
//
// Eligibility is counted in whole days — "56 days between donations", "deferred
// until the 14th" — so an evaluation must not depend on the time of day it
// happens to run at. Without this, a donation at 09:00 and one at 17:00 on the
// same date produce different answers 56 days later.
func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func formatDay(t time.Time) string { return t.Format("2 January 2006") }

// trimFloat prints a threshold the way a person writes it: 50 not 50.0, 12.5
// not 12.50.
func trimFloat(f float64) string {
	s := fmt.Sprintf("%.1f", f)
	if len(s) > 2 && s[len(s)-2:] == ".0" {
		return s[:len(s)-2]
	}
	return s
}
