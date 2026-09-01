package domain

import (
	"errors"
	"testing"
	"time"
)

// The legacy `donors` table stored blood type as free text — "O+", "o", " A "
// were all present (defect D7). ParseRhesus is the other half of untangling it.
func TestParseRhesus(t *testing.T) {
	positive := []string{"+", "positive", "POSITIVE", "Pos", "p", " + ", "POS"}
	for _, in := range positive {
		got, err := ParseRhesus(in)
		if err != nil || got != RhPositive {
			t.Errorf("ParseRhesus(%q) = (%q, %v), want positive", in, got, err)
		}
	}

	negative := []string{"-", "negative", "NEGATIVE", "Neg", "n", " - "}
	for _, in := range negative {
		got, err := ParseRhesus(in)
		if err != nil || got != RhNegative {
			t.Errorf("ParseRhesus(%q) = (%q, %v), want negative", in, got, err)
		}
	}

	// Anything else must fail rather than default. Guessing a rhesus is the kind
	// of "helpful" behaviour that puts the wrong blood in a patient.
	for _, in := range []string{"", "  ", "maybe", "0", "positve", "±"} {
		if _, err := ParseRhesus(in); !errors.Is(err, ErrInvalidRhesus) {
			t.Errorf("ParseRhesus(%q) did not fail", in)
		}
	}
}

func TestParseGender(t *testing.T) {
	for in, want := range map[string]Gender{
		"male": GenderMale, "MALE": GenderMale, " Female ": GenderFemale,
		"other": GenderOther, "undisclosed": GenderUndisclosed,
		// Not asked is the same fact about the record as declined to say.
		"": GenderUnstated, "   ": GenderUnstated,
	} {
		got, err := ParseGender(in)
		if err != nil || got != want {
			t.Errorf("ParseGender(%q) = (%q, %v), want %q", in, got, err, want)
		}
	}

	// The value that broke every signup once: it is not in the enum.
	for _, in := range []string{"unknown", "n/a", "prefer not to say", "m"} {
		if _, err := ParseGender(in); !errors.Is(err, ErrInvalidGender) {
			t.Errorf("ParseGender(%q) was accepted; the enum is male|female|other|undisclosed", in)
		}
	}
}

// AgeYears must agree with the donor_eligibility view's SQL, because the
// eligibility age band is computed in both places.
func TestAgeYears(t *testing.T) {
	on := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		dob  time.Time
		want int
	}{
		{"birthday already passed", time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), 26},
		{"birthday still to come", time.Date(2000, 12, 31, 0, 0, 0, 0, time.UTC), 25},
		{"birthday today", time.Date(2000, 6, 15, 0, 0, 0, 0, time.UTC), 26},
		{"day before birthday", time.Date(2000, 6, 16, 0, 0, 0, 0, time.UTC), 25},
		{"born today", on, 0},
	}
	for _, c := range cases {
		if got := AgeYears(c.dob, on); got != c.want {
			t.Errorf("%s: AgeYears = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestDonorValidate(t *testing.T) {
	valid := Donor{Email: "d@example.test", FullName: "A Donor"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("a valid donor was rejected: %v", err)
	}

	t.Run("name and email are required", func(t *testing.T) {
		d := valid
		d.FullName = "   "
		if err := d.Validate(); !errors.Is(err, ErrNameRequired) {
			t.Errorf("blank name = %v, want ErrNameRequired", err)
		}
		d = valid
		d.Email = ""
		if err := d.Validate(); !errors.Is(err, ErrEmailRequired) {
			t.Errorf("blank email = %v, want ErrEmailRequired", err)
		}
	})

	// The database enforces this too (donor_profiles_abo_paired). Duplicated on
	// purpose: a constraint violation is a 500, a domain error is a 422.
	t.Run("blood group and rhesus travel together", func(t *testing.T) {
		d := valid
		d.BloodGroup = GroupO
		if err := d.Validate(); !errors.Is(err, ErrGroupRhesusPaired) {
			t.Errorf("group without rhesus = %v, want ErrGroupRhesusPaired", err)
		}
		d = valid
		d.Rhesus = RhNegative
		if err := d.Validate(); !errors.Is(err, ErrGroupRhesusPaired) {
			t.Errorf("rhesus without group = %v, want ErrGroupRhesusPaired", err)
		}
		d = valid
		d.BloodGroup, d.Rhesus = GroupAB, RhPositive
		if err := d.Validate(); err != nil {
			t.Errorf("a complete blood type was rejected: %v", err)
		}
	})

	t.Run("an unknown blood group is refused", func(t *testing.T) {
		d := valid
		d.BloodGroup, d.Rhesus = BloodGroup("C"), RhPositive
		if err := d.Validate(); !errors.Is(err, ErrInvalidBloodGroup) {
			t.Errorf("group C = %v, want ErrInvalidBloodGroup", err)
		}
	})
}

// EnsureTransition's happy path, which the transition table test only exercised
// through its failures.
func TestEnsureTransitionAllowsLegalMoves(t *testing.T) {
	for _, to := range []RequestStatus{RequestApproved, RequestRejected, RequestCancelled, RequestExpired} {
		if err := EnsureTransition(RequestPending, to); err != nil {
			t.Errorf("pending -> %s = %v, want nil", to, err)
		}
	}
}
