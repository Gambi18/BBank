package domain_test

import (
	"errors"
	"testing"
	"time"

	"bbank/internal/domain"
)

// An empty procedure is whole blood, matching the column default (schema §6.3).
// A booking form that does not ask means the ordinary case, and refusing it
// would make every such form send a value it has no reason to know about.
func TestParseProcedure(t *testing.T) {
	cases := map[string]domain.Procedure{
		"":                    domain.ProcedureWholeBlood,
		"whole_blood":         domain.ProcedureWholeBlood,
		"  WHOLE_BLOOD  ":     domain.ProcedureWholeBlood,
		"apheresis_platelet":  domain.ProcedureApheresisPlatelet,
		"apheresis_plasma":    domain.ProcedureApheresisPlasma,
		"double_red_cell":     domain.ProcedureDoubleRedCell,
		"Apheresis_Platelet ": domain.ProcedureApheresisPlatelet,
	}
	for in, want := range cases {
		got, err := domain.ParseProcedure(in)
		if err != nil {
			t.Errorf("ParseProcedure(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseProcedure(%q) = %q, want %q", in, got, want)
		}
	}

	for _, bad := range []string{"plasma", "wholeblood", "whole blood", "apheresis", "56"} {
		if _, err := domain.ParseProcedure(bad); !errors.Is(err, domain.ErrInvalidProcedure) {
			t.Errorf("ParseProcedure(%q) = %v, want ErrInvalidProcedure", bad, err)
		}
	}
}

// A component, unlike a procedure, has NO default. A unit with no stated
// component has no shelf life, and defaulting one would put an expiry date on a
// bag nobody chose.
func TestParseComponent(t *testing.T) {
	for _, c := range domain.Components() {
		got, err := domain.ParseComponent(string(c))
		if err != nil {
			t.Errorf("ParseComponent(%q): %v", c, err)
			continue
		}
		if got != c {
			t.Errorf("ParseComponent(%q) = %q", c, got)
		}
	}
	if got, err := domain.ParseComponent("  PLATELETS "); err != nil || got != domain.ComponentPlatelets {
		t.Errorf("ParseComponent with padding and case = %q, %v", got, err)
	}

	for _, bad := range []string{"", "  ", "red_cells", "plasma", "blood"} {
		if _, err := domain.ParseComponent(bad); !errors.Is(err, domain.ErrInvalidComponent) {
			t.Errorf("ParseComponent(%q) = %v, want ErrInvalidComponent", bad, err)
		}
	}
}

// Components() must list every member of the `component_type` enum. A component
// missing from it is a component `AllShelfLives` never checks, which is how a
// unit ends up with no expiry.
func TestComponentsCoversTheEnum(t *testing.T) {
	// The enum as declared in migration 000001.
	want := []string{"whole_blood", "packed_red_cells", "fresh_frozen_plasma", "platelets", "cryoprecipitate"}
	got := domain.Components()
	if len(got) != len(want) {
		t.Fatalf("Components() has %d entries, the enum has %d", len(got), len(want))
	}
	seen := map[string]bool{}
	for _, c := range got {
		seen[string(c)] = true
	}
	for _, w := range want {
		if !seen[w] {
			t.Errorf("Components() is missing %q — AllShelfLives would never check it", w)
		}
	}
}

func TestKeysListsTheSnapshotSorted(t *testing.T) {
	keys := seededPolicies(t).Keys()
	if len(keys) != len(seededPolicyFixture()) {
		t.Fatalf("Keys() returned %d, the fixture has %d", len(keys), len(seededPolicyFixture()))
	}
	for i := 1; i < len(keys); i++ {
		if keys[i-1] >= keys[i] {
			t.Fatalf("Keys() is not sorted: %q then %q", keys[i-1], keys[i])
		}
	}
}

// The two `{"hours": n}` policies that are not shelf lives.
func TestHourPolicies(t *testing.T) {
	p := seededPolicies(t)

	window, err := p.ExpiryAlertWindow()
	if err != nil {
		t.Fatalf("expiry alert window: %v", err)
	}
	if window != 72*time.Hour {
		t.Errorf("expiry alert window = %v, want 72h", window)
	}

	minRemaining, err := p.AllocationMinRemaining()
	if err != nil {
		t.Fatalf("allocation minimum: %v", err)
	}
	if minRemaining != 4*time.Hour {
		t.Errorf("allocation minimum remaining = %v, want 4h", minRemaining)
	}

	// Both go through Hours, which must refuse a value that would silently
	// disable the thing it configures.
	for name, bad := range map[string]string{
		"missing":  `{}`,
		"zero":     `{"hours":0}`,
		"negative": `{"hours":-5}`,
		"a string": `{"hours":"72"}`,
	} {
		t.Run(name, func(t *testing.T) {
			broken := seededPolicies(t, map[domain.PolicyKey]string{domain.KeyExpiryAlertHours: bad})
			if _, err := broken.ExpiryAlertWindow(); err == nil {
				t.Fatalf("a %s hours value was accepted", name)
			}
		})
	}

	missing := seededPolicies(t, map[domain.PolicyKey]string{domain.KeyAllocationMinRemaining: ""})
	if _, err := missing.AllocationMinRemaining(); !errors.Is(err, domain.ErrPolicyMissing) {
		t.Errorf("a missing allocation minimum returned %v, want ErrPolicyMissing", err)
	}
}

// The version must change when any value changes, and must NOT change for
// things that are not values: key order, JSON whitespace, or the order rows
// came back from the database in.
func TestPolicyVersionIsStableAndSensitive(t *testing.T) {
	rows := []domain.PolicyRow{
		{Key: domain.KeyDonorAgeYears, Region: "*", Value: []byte(`{"min":18,"max":65}`)},
		{Key: domain.KeyDonorMinWeightKg, Region: "*", Value: []byte(`{"kg":50}`)},
	}
	base := domain.PolicyVersion(rows)

	reordered := domain.PolicyVersion([]domain.PolicyRow{rows[1], rows[0]})
	if reordered != base {
		t.Error("row order changed the version — the fingerprint depends on the query plan")
	}

	respaced := domain.PolicyVersion([]domain.PolicyRow{
		{Key: domain.KeyDonorAgeYears, Region: "*", Value: []byte(`{ "max" : 65 , "min" : 18 }`)},
		rows[1],
	})
	if respaced != base {
		t.Error("JSON whitespace and key order changed the version")
	}

	changed := domain.PolicyVersion([]domain.PolicyRow{
		{Key: domain.KeyDonorAgeYears, Region: "*", Value: []byte(`{"min":17,"max":65}`)},
		rows[1],
	})
	if changed == base {
		t.Error("changing a threshold did not change the version — two decisions under different numbers would be indistinguishable")
	}

	added := domain.PolicyVersion(append(append([]domain.PolicyRow{}, rows...),
		domain.PolicyRow{Key: domain.KeyExpiryAlertHours, Region: "*", Value: []byte(`{"hours":72}`)}))
	if added == base {
		t.Error("adding a policy did not change the version")
	}
}

// A malformed age band must not become a band nobody intended.
func TestMalformedAgeBandIsRejected(t *testing.T) {
	for name, value := range map[string]string{
		"no min":            `{"max":65}`,
		"no max":            `{"min":18}`,
		"min above max":     `{"min":70,"max":65}`,
		"min as a string":   `{"min":"18","max":65}`,
		"not an object":     `[18,65]`,
		"first_time absent": `{"min":18,"max":65}`, // valid: the cap falls back to max
	} {
		t.Run(name, func(t *testing.T) {
			p := seededPolicies(t, map[domain.PolicyKey]string{domain.KeyDonorAgeYears: value})
			band, err := p.AgeBand()
			if name == "first_time absent" {
				if err != nil {
					t.Fatalf("a band with no first-time cap was rejected: %v", err)
				}
				if band.FirstTimeMax != band.Max {
					t.Errorf("first-time cap = %d, want the general max %d", band.FirstTimeMax, band.Max)
				}
				return
			}
			if err == nil {
				t.Fatalf("a malformed age band was accepted as %+v", band)
			}
			if !errors.Is(err, domain.ErrPolicyMalformed) {
				t.Errorf("error = %v, want ErrPolicyMalformed", err)
			}
		})
	}
}

// A negative interval would make every donor perpetually eligible.
func TestMalformedIntervalIsRejected(t *testing.T) {
	for name, value := range map[string]string{
		"no days":  `{}`,
		"negative": `{"days":-1}`,
		"a string": `{"days":"56"}`,
	} {
		t.Run(name, func(t *testing.T) {
			p := seededPolicies(t, map[domain.PolicyKey]string{
				domain.IntervalKey(domain.ProcedureWholeBlood): value,
			})
			if _, err := p.IntervalDays(domain.ProcedureWholeBlood); !errors.Is(err, domain.ErrPolicyMalformed) {
				t.Errorf("error = %v, want ErrPolicyMalformed", err)
			}
		})
	}

	// Zero is legal: a procedure with no minimum gap is a real configuration,
	// and refusing it would make policy less expressive than the clinicians
	// setting it.
	p := seededPolicies(t, map[domain.PolicyKey]string{
		domain.IntervalKey(domain.ProcedureWholeBlood): `{"days":0}`,
	})
	if days, err := p.IntervalDays(domain.ProcedureWholeBlood); err != nil || days != 0 {
		t.Errorf("a zero interval = %d, %v; want 0 and no error", days, err)
	}
}

// Bound.Contains must treat an absent end as unbounded, not as zero. A
// temperature policy with only a maximum must not reject every reading above
// absolute zero — or below it.
func TestBoundTreatsAnAbsentEndAsUnbounded(t *testing.T) {
	p := seededPolicies(t)
	r, err := p.VitalsRange()
	if err != nil {
		t.Fatalf("vitals range: %v", err)
	}

	if r.Temperature.Min != nil {
		t.Error("the seeded temperature policy has no minimum; one was parsed")
	}
	if !r.Temperature.Contains(30) {
		t.Error("a low temperature was rejected by a policy with no minimum")
	}
	if r.Temperature.Contains(38) {
		t.Error("a temperature above the maximum was accepted")
	}
	if !r.Pulse.Contains(50) || !r.Pulse.Contains(100) {
		t.Error("the pulse bounds are not inclusive at their ends")
	}
}

func TestMalformedVitalsRangeIsRejected(t *testing.T) {
	for name, value := range map[string]string{
		"missing pulse":       `{"bp_systolic":{"min":90,"max":180},"bp_diastolic":{"min":50,"max":100},"temperature_c":{"max":37.5}}`,
		"missing temperature": `{"bp_systolic":{"min":90,"max":180},"bp_diastolic":{"min":50,"max":100},"pulse_bpm":{"min":50,"max":100}}`,
		"not an object":       `"90/180"`,
	} {
		t.Run(name, func(t *testing.T) {
			p := seededPolicies(t, map[domain.PolicyKey]string{domain.KeyDonorVitalsRange: value})
			if _, err := p.VitalsRange(); !errors.Is(err, domain.ErrPolicyMalformed) {
				t.Errorf("error = %v, want ErrPolicyMalformed", err)
			}
		})
	}
}

func TestMalformedHemoglobinAndWeightAreRejected(t *testing.T) {
	half := seededPolicies(t, map[domain.PolicyKey]string{
		domain.KeyDonorMinHemoglobin: `{"female":12.5}`,
	})
	if _, err := half.MinHemoglobin(domain.GenderMale); !errors.Is(err, domain.ErrPolicyMalformed) {
		t.Errorf("a haemoglobin policy with only one threshold returned %v, want ErrPolicyMalformed", err)
	}

	noKg := seededPolicies(t, map[domain.PolicyKey]string{domain.KeyDonorMinWeightKg: `{}`})
	if _, err := noKg.MinWeightKg(); !errors.Is(err, domain.ErrPolicyMalformed) {
		t.Errorf("a weight policy with no kg returned %v, want ErrPolicyMalformed", err)
	}
}

// An annual-cap policy that states neither a per-procedure figure nor the
// by-sex pair cannot answer for whole blood, and must say so rather than
// silently uncapping the one procedure everybody uses.
func TestMalformedAnnualCapIsRejected(t *testing.T) {
	p := seededPolicies(t, map[domain.PolicyKey]string{
		domain.KeyDonationsPerYearMax: `{"apheresis_platelet":24}`,
	})
	if _, _, err := p.AnnualCap(domain.GenderFemale, domain.ProcedureWholeBlood); !errors.Is(err, domain.ErrPolicyMalformed) {
		t.Errorf("error = %v, want ErrPolicyMalformed", err)
	}
	// The procedure that IS configured still answers.
	if cap, capped, err := p.AnnualCap(domain.GenderFemale, domain.ProcedureApheresisPlatelet); err != nil || !capped || cap != 24 {
		t.Errorf("apheresis cap = %d, capped=%v, err=%v; want 24, true, nil", cap, capped, err)
	}
}
