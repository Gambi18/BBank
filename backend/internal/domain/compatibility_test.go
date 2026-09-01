package domain

import "testing"

// The counts come from the clinical matrix and are seeded identically into
// abo_compatibility (27 rows total). A change to either side must break a test.
func TestCompatibilityCounts(t *testing.T) {
	want := map[string]int{
		"O-": 1, "O+": 2, "A-": 2, "A+": 4,
		"B-": 2, "B+": 4, "AB-": 4, "AB+": 8,
	}
	total := 0
	for recipient, n := range want {
		g, rh, err := ParseBloodGroup(recipient)
		if err != nil {
			t.Fatalf("ParseBloodGroup(%q): %v", recipient, err)
		}
		got := len(CompatibleDonorsFor(TypedUnit{g, rh}))
		if got != n {
			t.Errorf("%s: got %d compatible donor types, want %d", recipient, got, n)
		}
		total += got
	}
	if total != 27 {
		t.Errorf("total compatibility pairs = %d, want 27 (must match abo_compatibility)", total)
	}
}

func TestUniversalDonorAndRecipient(t *testing.T) {
	all := []TypedUnit{
		{GroupO, RhNegative}, {GroupO, RhPositive}, {GroupA, RhNegative}, {GroupA, RhPositive},
		{GroupB, RhNegative}, {GroupB, RhPositive}, {GroupAB, RhNegative}, {GroupAB, RhPositive},
	}
	oNeg := TypedUnit{GroupO, RhNegative}
	for _, r := range all {
		if !IsCompatible(r, oNeg) {
			t.Errorf("O- must be transfusable into %s (universal donor)", r)
		}
	}
	abPos := TypedUnit{GroupAB, RhPositive}
	for _, u := range all {
		if !IsCompatible(abPos, u) {
			t.Errorf("AB+ must be able to receive %s (universal recipient)", u)
		}
	}
}

// The direction of the table is the single easiest thing to get backwards, and
// getting it backwards kills someone. Assert the asymmetry explicitly.
func TestCompatibilityIsDirectional(t *testing.T) {
	oNeg := TypedUnit{GroupO, RhNegative}
	abPos := TypedUnit{GroupAB, RhPositive}
	if !IsCompatible(abPos, oNeg) {
		t.Error("an AB+ recipient must accept an O- unit")
	}
	if IsCompatible(oNeg, abPos) {
		t.Error("an O- recipient must NOT accept an AB+ unit — the table is reversed")
	}
}

func TestRhNegativeRecipientNeverGetsPositive(t *testing.T) {
	for _, r := range []TypedUnit{{GroupO, RhNegative}, {GroupA, RhNegative}, {GroupB, RhNegative}, {GroupAB, RhNegative}} {
		for _, u := range CompatibleDonorsFor(r) {
			if u.Rhesus == RhPositive {
				t.Errorf("Rh-negative recipient %s must never receive Rh-positive unit %s", r, u)
			}
		}
	}
}

func TestParseBloodGroupHandlesLegacyFreeText(t *testing.T) {
	cases := []struct {
		in    string
		group BloodGroup
		rh    Rhesus
		ok    bool
	}{
		{"O+", GroupO, RhPositive, true}, // the real legacy value on donor 1
		{" a ", GroupA, "", true},
		{"ab-", GroupAB, RhNegative, true},
		{"O", GroupO, "", true},
		{"", "", "", false},
		{"C", "", "", false},
		{"XYZ+", "", "", false},
	}
	for _, c := range cases {
		g, rh, err := ParseBloodGroup(c.in)
		if (err == nil) != c.ok {
			t.Errorf("ParseBloodGroup(%q): err=%v, want ok=%v", c.in, err, c.ok)
			continue
		}
		if c.ok && (g != c.group || rh != c.rh) {
			t.Errorf("ParseBloodGroup(%q) = (%q,%q), want (%q,%q)", c.in, g, rh, c.group, c.rh)
		}
	}
}
