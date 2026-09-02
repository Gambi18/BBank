package domain_test

import (
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"bbank/internal/domain"
)

const policySeedFile = "../../migrations/000012_seed_reference_data.up.sql"

// seededPolicyRows parses the `policies` INSERT out of the seed migration.
//
// The same approach as `TestCodeMatrixMatchesSeedMigration`, for the same
// reason: parsing the .sql keeps this a fast unit test with no fixture, so the
// clinical constants are cross-checked on every commit rather than only when
// somebody runs the integration suite.
func seededPolicyRows(t *testing.T) map[domain.PolicyKey]json.RawMessage {
	t.Helper()
	b, err := os.ReadFile(policySeedFile)
	if err != nil {
		t.Fatalf("cannot read %s: %v", policySeedFile, err)
	}

	// (' donor_age_years ', '{"min":18,...}', '*', 'description')
	re := regexp.MustCompile(`\('([a-z_0-9.]+)',\s*'(\{[^']*\})',\s*'([^']*)',`)
	matches := re.FindAllStringSubmatch(string(b), -1)
	if len(matches) == 0 {
		t.Fatalf("parsed zero policy rows from %s — has its format changed?", policySeedFile)
	}

	out := make(map[domain.PolicyKey]json.RawMessage, len(matches))
	for _, m := range matches {
		if m[3] != "*" {
			continue // A region-specific seed row; the default set is what this pins.
		}
		out[domain.PolicyKey(m[1])] = json.RawMessage(m[2])
	}
	return out
}

// The fixture every eligibility test uses must be the values production
// actually seeds.
//
// Without this, `seededPolicyFixture` is a set of numbers a test author typed,
// and the whole suite would keep passing if migration 000012 shipped a 5-day
// donation interval. The point of testing against policy is lost if the policy
// under test is invented.
func TestSeededPolicyValuesMatchTheMigration(t *testing.T) {
	fromSeed := seededPolicyRows(t)
	fixture := seededPolicyFixture()

	for key, fixtureValue := range fixture {
		seedValue, ok := fromSeed[key]
		if !ok {
			t.Errorf("the test fixture has %s but the seed migration does not", key)
			continue
		}
		if !sameJSON(t, seedValue, json.RawMessage(fixtureValue)) {
			t.Errorf("%s: fixture has %s, the seed has %s", key, fixtureValue, seedValue)
		}
	}

	for key := range fromSeed {
		if _, ok := fixture[key]; !ok {
			t.Errorf("the seed migration has %s and the test fixture does not — a clinical constant is untested", key)
		}
	}
}

// Every key the domain can ask for must exist in the seed. A decision that
// cannot be made because its threshold was never seeded is an outage, and it
// should be caught here rather than by the first donor who tries to book.
func TestSeedCoversEveryKeyTheDomainNeeds(t *testing.T) {
	fromSeed := seededPolicyRows(t)

	required := []domain.PolicyKey{
		domain.KeyDonorAgeYears,
		domain.KeyDonorMinWeightKg,
		domain.KeyDonorMinHemoglobin,
		domain.KeyDonorVitalsRange,
		domain.KeyDonationsPerYearMax,
		domain.KeyExpiryAlertHours,
		domain.KeyAllocationMinRemaining,
		// Whole blood is the procedure every donor defaults to; a missing
		// interval for it would refuse every booking.
		domain.IntervalKey(domain.ProcedureWholeBlood),
	}
	for _, c := range domain.Components() {
		required = append(required, domain.ShelfLifeKey(c))
	}

	var missing []string
	for _, key := range required {
		if _, ok := fromSeed[key]; !ok {
			missing = append(missing, string(key))
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("the seed migration is missing %d key(s) the domain requires: %s",
			len(missing), strings.Join(missing, ", "))
	}
}

// The seeded set must actually satisfy a real evaluation, not merely parse.
// This is the difference between "the keys exist" and "a donor can book".
func TestTheSeededPolicySetCanDecide(t *testing.T) {
	raw := seededPolicyRows(t)
	rows := make([]domain.PolicyRow, 0, len(raw))
	for k, v := range raw {
		rows = append(rows, domain.PolicyRow{Key: k, Region: "*", Value: v})
	}
	p := domain.NewPolicies(domain.PolicyVersion(rows), raw)

	f := eligibleDonor()
	f.Vitals = &domain.Vitals{
		WeightKg: 70, HemoglobinGdL: 13.5,
		BPSystolic: 120, BPDiastolic: 80, PulseBPM: 70, TemperatureC: 36.8,
	}
	d, err := domain.EvaluateEligibility(f, p, day(2026, time.September, 2))
	if err != nil {
		t.Fatalf("the seeded policy set cannot decide: %v", err)
	}
	if !d.Eligible {
		t.Errorf("a healthy donor is ineligible under the seeded policy: %+v", d.Failures)
	}

	if _, err := p.AllShelfLives(); err != nil {
		t.Errorf("the seeded policy set cannot produce shelf lives: %v", err)
	}
}

func sameJSON(t *testing.T, a, b json.RawMessage) bool {
	t.Helper()
	var x, y any
	if err := json.Unmarshal(a, &x); err != nil {
		t.Fatalf("seed value is not JSON: %s", a)
	}
	if err := json.Unmarshal(b, &y); err != nil {
		t.Fatalf("fixture value is not JSON: %s", b)
	}
	ab, _ := json.Marshal(x)
	bb, _ := json.Marshal(y)
	return string(ab) == string(bb)
}
