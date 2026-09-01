package service_test

import (
	"context"
	"testing"
	"time"

	"bbank/internal/domain"
	"bbank/internal/testsupport"
)

// domain.AgeYears and the database must agree, because both compute the age the
// eligibility band is checked against — Go in `internal/domain`, Postgres in the
// `donor_eligibility` view as `EXTRACT(YEAR FROM age(...))`. A disagreement is
// two components reaching different conclusions about the same person, and with
// a minimum donor age of 18 the disagreement lands exactly on somebody's
// eighteenth birthday.
//
// This test exists because they DID disagree: AgeYears compared `YearDay()`,
// which is off by one whenever a leap day falls between the birth date and the
// reference date. The unit test caught it; this asserts the property that
// actually matters, against the real implementation on the other side.
func TestAgeYearsAgreesWithPostgres(t *testing.T) {
	pool := testsupport.Pool(t)
	ctx := context.Background()

	// Deliberately loaded with the cases that break naive arithmetic: leap-year
	// births, February 29th, birthdays either side of the reference date, and
	// dates whose YearDay differs from the reference year's.
	dobs := []string{
		"2000-06-15", "2000-01-01", "2000-12-31", "2000-02-29",
		"1996-02-29", "2004-02-29", "2008-03-01", "1999-12-31",
		"2001-01-01", "1988-07-04", "2007-02-28", "2005-06-15",
	}
	asOf := []string{
		"2026-06-15", "2026-01-01", "2026-12-31", "2026-02-28",
		"2024-02-29", "2025-03-01", "2026-09-02",
	}

	for _, d := range dobs {
		for _, a := range asOf {
			dob, err := time.Parse("2006-01-02", d)
			if err != nil {
				t.Fatalf("bad fixture date %q: %v", d, err)
			}
			on, err := time.Parse("2006-01-02", a)
			if err != nil {
				t.Fatalf("bad fixture date %q: %v", a, err)
			}
			if on.Before(dob) {
				continue // a negative age is not a case either side claims to handle
			}

			var fromSQL int
			if err := pool.QueryRow(ctx,
				`SELECT EXTRACT(YEAR FROM age($1::date, $2::date))::int`, a, d).Scan(&fromSQL); err != nil {
				t.Fatalf("postgres age(%s, %s): %v", a, d, err)
			}

			if got := domain.AgeYears(dob, on); got != fromSQL {
				t.Errorf("dob %s as of %s: Go says %d, Postgres says %d", d, a, got, fromSQL)
			}
		}
	}
}
