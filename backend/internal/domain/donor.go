// Package domain holds pure business logic.
//
// DEPENDENCY RULE: this package imports nothing else from this project — no
// store, no http, no service. Only the standard library and small pure helpers.
// That is what makes it 100% unit-testable without a database, and it is
// enforced in CI (see .github/workflows/ci.yml, "architecture" job).
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// BloodGroup and Rhesus mirror the database enums. They are declared here, not
// imported from store, because the dependency rule runs this way round: domain
// defines the vocabulary and store maps onto it.
type BloodGroup string

const (
	GroupA  BloodGroup = "A"
	GroupB  BloodGroup = "B"
	GroupAB BloodGroup = "AB"
	GroupO  BloodGroup = "O"
)

type Rhesus string

const (
	RhPositive Rhesus = "positive"
	RhNegative Rhesus = "negative"
)

var (
	ErrInvalidBloodGroup = errors.New("blood group must be one of A, B, AB, O")
	ErrInvalidRhesus     = errors.New("rhesus must be positive or negative")
	ErrGroupRhesusPaired = errors.New("blood group and rhesus must be given together or not at all")
	ErrNameRequired      = errors.New("full name is required")
	ErrEmailRequired     = errors.New("email is required")
)

// ParseBloodGroup normalises the free-text forms found in the legacy data.
// The legacy `donors` table stored things like "O+", "o", and " A " (defect D7),
// so this is deliberately liberal in what it accepts and strict in what it returns.
//
// Note "O+" yields group O and rhesus positive: the legacy column packed both
// facts into one field.
func ParseBloodGroup(s string) (BloodGroup, Rhesus, error) {
	t := strings.ToUpper(strings.TrimSpace(s))
	if t == "" {
		return "", "", ErrInvalidBloodGroup
	}

	var rh Rhesus
	switch {
	case strings.HasSuffix(t, "+"):
		rh, t = RhPositive, strings.TrimSuffix(t, "+")
	case strings.HasSuffix(t, "-"):
		rh, t = RhNegative, strings.TrimSuffix(t, "-")
	}
	t = strings.TrimSpace(t)

	switch BloodGroup(t) {
	case GroupA, GroupB, GroupAB, GroupO:
		return BloodGroup(t), rh, nil
	}
	return "", "", ErrInvalidBloodGroup
}

// ParseRhesus normalises "+", "-", "positive", "POSITIVE", "Neg" and friends.
func ParseRhesus(s string) (Rhesus, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "+", "positive", "pos", "p":
		return RhPositive, nil
	case "-", "negative", "neg", "n":
		return RhNegative, nil
	}
	return "", ErrInvalidRhesus
}

// AgeYears returns completed years at `on`. Used for the eligibility age band;
// the database computes the same thing in donor_eligibility, and the two must agree.
func AgeYears(dob time.Time, on time.Time) int {
	years := on.Year() - dob.Year()
	if on.YearDay() < dob.YearDay() {
		years--
	}
	return years
}

// Donor is the domain view of a donor. It is deliberately not the database row
// type and not the wire type — those live in store and http/dto respectively.
type Donor struct {
	ID          int64
	Email       string
	FullName    string
	DateOfBirth time.Time
	Gender      string
	BloodGroup  BloodGroup
	Rhesus      Rhesus
	Phone       string
	Address     string
}

// Validate enforces the invariants that are true regardless of transport or
// storage. Shape checks (required, email format) belong on the DTO; this is for
// rules that are part of the domain itself.
func (d Donor) Validate() error {
	if strings.TrimSpace(d.FullName) == "" {
		return ErrNameRequired
	}
	if strings.TrimSpace(d.Email) == "" {
		return ErrEmailRequired
	}
	// The database enforces this too (donor_profiles_abo_paired). Duplicated on
	// purpose: a constraint violation is a 500, a domain error is a 400.
	if (d.BloodGroup == "") != (d.Rhesus == "") {
		return ErrGroupRhesusPaired
	}
	if d.BloodGroup != "" {
		switch d.BloodGroup {
		case GroupA, GroupB, GroupAB, GroupO:
		default:
			return fmt.Errorf("%w: got %q", ErrInvalidBloodGroup, d.BloodGroup)
		}
	}
	return nil
}
