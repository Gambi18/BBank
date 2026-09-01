package domain

import (
	"os"
	"regexp"
	"sort"
	"testing"
)

// The in-code compatibility matrix and the abo_compatibility seed migration are
// two copies of one clinical fact. This test parses the migration and asserts
// they agree exactly.
//
// It exists because the failure mode of divergence is not a broken build or a
// bad response — it is a patient receiving an incompatible unit. If this test
// ever fails, do not "fix" it by editing whichever side is convenient: work out
// which one is clinically correct.
//
// Parsing the .sql file rather than querying the database keeps this a fast unit
// test with no fixture, so it runs on every commit.
func TestCodeMatrixMatchesSeedMigration(t *testing.T) {
	const seed = "../../migrations/000012_seed_reference_data.up.sql"
	b, err := os.ReadFile(seed)
	if err != nil {
		t.Fatalf("cannot read %s: %v", seed, err)
	}

	// ('red_cells','O','negative','O','negative',1)
	re := regexp.MustCompile(`\('red_cells','(A|B|AB|O)','(positive|negative)','(A|B|AB|O)','(positive|negative)',\d+\)`)
	matches := re.FindAllStringSubmatch(string(b), -1)
	if len(matches) == 0 {
		t.Fatal("parsed zero red_cells rows from the seed migration — has its format changed?")
	}

	fromSeed := map[string]bool{}
	for _, m := range matches {
		recipient := TypedUnit{BloodGroup(m[1]), Rhesus(m[2])}
		donor := TypedUnit{BloodGroup(m[3]), Rhesus(m[4])}
		fromSeed[recipient.String()+"<-"+donor.String()] = true
	}

	fromCode := map[string]bool{}
	for _, r := range []string{"O-", "O+", "A-", "A+", "B-", "B+", "AB-", "AB+"} {
		g, rh, err := ParseBloodGroup(r)
		if err != nil {
			t.Fatalf("ParseBloodGroup(%q): %v", r, err)
		}
		recipient := TypedUnit{g, rh}
		for _, u := range CompatibleDonorsFor(recipient) {
			fromCode[recipient.String()+"<-"+u.String()] = true
		}
	}

	var missingInCode, missingInSeed []string
	for k := range fromSeed {
		if !fromCode[k] {
			missingInCode = append(missingInCode, k)
		}
	}
	for k := range fromCode {
		if !fromSeed[k] {
			missingInSeed = append(missingInSeed, k)
		}
	}
	sort.Strings(missingInCode)
	sort.Strings(missingInSeed)

	if len(missingInCode) > 0 {
		t.Errorf("in the seed migration but NOT in the Go matrix: %v", missingInCode)
	}
	if len(missingInSeed) > 0 {
		t.Errorf("in the Go matrix but NOT in the seed migration: %v", missingInSeed)
	}
	if len(fromSeed) != 27 || len(fromCode) != 27 {
		t.Errorf("expected 27 red-cell pairs on both sides; seed=%d code=%d", len(fromSeed), len(fromCode))
	}
}
