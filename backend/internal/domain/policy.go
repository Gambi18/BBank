package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// PolicyKey is a row key in the `policies` table (schema §12.1).
//
// The keys are named here, and only the keys. Their VALUES are not: every
// clinical threshold this system applies — age band, weight, haemoglobin,
// vitals, donation intervals, annual caps, shelf lives — is a database row that
// an administrator can change without a deploy (`FR-20`, `FR-68`). A number
// compiled into Go is a number nobody can correct at 2am, and this file exists
// so that grep for one finds nothing.
type PolicyKey string

const (
	KeyDonorAgeYears          PolicyKey = "donor_age_years"
	KeyDonorMinWeightKg       PolicyKey = "donor_min_weight_kg"
	KeyDonorMinHemoglobin     PolicyKey = "donor_min_hemoglobin_g_dl"
	KeyDonorVitalsRange       PolicyKey = "donor_vitals_range"
	KeyDonationsPerYearMax    PolicyKey = "donations_per_year_max"
	KeyExpiryAlertHours       PolicyKey = "expiry_alert_hours"
	KeyAllocationMinRemaining PolicyKey = "allocation_min_remaining_hours"
)

// Two families of key are per-procedure and per-component, so they are built
// rather than listed. The suffix is the enum value, which is why
// `donation_procedure` and `component_type` can gain a member without this file
// changing.
const (
	prefixInterval  = "donation_interval_days."
	prefixShelfLife = "shelf_life_hours."
)

// IntervalKey is the minimum-gap policy for one donation procedure.
func IntervalKey(p Procedure) PolicyKey { return PolicyKey(prefixInterval + string(p)) }

// ShelfLifeKey is the shelf-life policy for one blood component.
func ShelfLifeKey(c Component) PolicyKey { return PolicyKey(prefixShelfLife + string(c)) }

// Procedure mirrors the `donation_procedure` enum (schema §5).
type Procedure string

const (
	ProcedureWholeBlood        Procedure = "whole_blood"
	ProcedureApheresisPlatelet Procedure = "apheresis_platelet"
	ProcedureApheresisPlasma   Procedure = "apheresis_plasma"
	ProcedureDoubleRedCell     Procedure = "double_red_cell"
)

var ErrInvalidProcedure = errors.New("procedure must be whole_blood, apheresis_platelet, apheresis_plasma or double_red_cell")

// ParseProcedure normalises what an API client sends. An empty value is
// `whole_blood`, matching the column default (schema §6.3) — the overwhelmingly
// common case, and the one a booking form that does not ask means.
func ParseProcedure(s string) (Procedure, error) {
	switch p := Procedure(strings.ToLower(strings.TrimSpace(s))); p {
	case "":
		return ProcedureWholeBlood, nil
	case ProcedureWholeBlood, ProcedureApheresisPlatelet, ProcedureApheresisPlasma, ProcedureDoubleRedCell:
		return p, nil
	default:
		return "", fmt.Errorf("%w: got %q", ErrInvalidProcedure, s)
	}
}

// Component mirrors the `component_type` enum (schema §5).
type Component string

const (
	ComponentWholeBlood       Component = "whole_blood"
	ComponentPackedRedCells   Component = "packed_red_cells"
	ComponentFreshFrozenPlasm Component = "fresh_frozen_plasma"
	ComponentPlatelets        Component = "platelets"
	ComponentCryoprecipitate  Component = "cryoprecipitate"
)

// Components lists every component, so a caller that must handle all of them —
// the shelf-life completeness check, an inventory grid — cannot silently miss
// one that is added later.
func Components() []Component {
	return []Component{
		ComponentWholeBlood, ComponentPackedRedCells, ComponentFreshFrozenPlasm,
		ComponentPlatelets, ComponentCryoprecipitate,
	}
}

var ErrInvalidComponent = errors.New("component is not one of the permitted values")

// ParseComponent normalises a component name. Unlike a procedure there is no
// default: a unit with no stated component has no shelf life, and guessing one
// would put an expiry date on a bag nobody chose.
func ParseComponent(s string) (Component, error) {
	c := Component(strings.ToLower(strings.TrimSpace(s)))
	for _, known := range Components() {
		if c == known {
			return c, nil
		}
	}
	return "", fmt.Errorf("%w: got %q", ErrInvalidComponent, s)
}

var (
	// ErrPolicyMissing is returned when a threshold a decision needs has no
	// active row.
	//
	// It is deliberately an ERROR and not a fallback. A missing policy means
	// the seed did not run, the effective-dated window lapsed, or somebody
	// deleted a row — and in every one of those cases the honest answer is
	// "this system cannot currently decide", not a number this file invented.
	// Defaulting would make the deletion of a clinical threshold invisible,
	// which is the precise failure `FR-20` exists to prevent.
	ErrPolicyMissing = errors.New("no active policy for that key")

	// ErrPolicyMalformed is returned when a row exists but its JSON is not the
	// shape the key requires — an operator typo reaching a clinical decision.
	ErrPolicyMalformed = errors.New("policy value is not the shape this key requires")
)

// Policies is an immutable snapshot of `active_policies` for one region.
//
// It is a snapshot on purpose. A decision is made against the policy set as it
// stood at one instant, and `Version` identifies that instant, so a decision
// recorded today can still be explained after an administrator changes a
// threshold tomorrow (`FR-68`: "changing a policy never rewrites decisions
// already made under the previous version").
type Policies struct {
	version string
	raw     map[PolicyKey]json.RawMessage
}

// NewPolicies builds a snapshot. The version is opaque to the domain: whoever
// loaded the rows decides how to identify them, and `PolicyVersion` is the
// function that does it.
func NewPolicies(version string, raw map[PolicyKey]json.RawMessage) *Policies {
	cp := make(map[PolicyKey]json.RawMessage, len(raw))
	for k, v := range raw {
		cp[k] = v
	}
	return &Policies{version: version, raw: cp}
}

// Version identifies the exact set of rows this snapshot was built from.
func (p *Policies) Version() string { return p.version }

// Has reports whether the snapshot contains a key at all — distinguishing
// "this deployment has not configured that" from "the policy set is broken".
func (p *Policies) Has(key PolicyKey) bool {
	_, ok := p.raw[key]
	return ok
}

// Keys lists what the snapshot contains, sorted. For diagnostics and for the
// completeness check that runs in CI.
func (p *Policies) Keys() []PolicyKey {
	out := make([]PolicyKey, 0, len(p.raw))
	for k := range p.raw {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// PolicyVersion fingerprints a set of policy rows.
//
// `policies` has no version column — the version IS the effective-dated set of
// rows, so the identifier has to be derived from them. It is built from each
// row's key, region and value in sorted order, which makes it stable across
// query plans and reconnections, and different the moment any value changes.
// That is exactly the property a stamp on a stored decision needs: two
// decisions carrying the same version were made under the same numbers.
//
// It is a fingerprint, not a hash chain — it identifies a configuration, it
// does not authenticate one.
func PolicyVersion(rows []PolicyRow) string {
	sorted := make([]PolicyRow, len(rows))
	copy(sorted, rows)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Key != sorted[j].Key {
			return sorted[i].Key < sorted[j].Key
		}
		return sorted[i].Region < sorted[j].Region
	})

	var b strings.Builder
	for _, r := range sorted {
		b.WriteString(string(r.Key))
		b.WriteByte('|')
		b.WriteString(r.Region)
		b.WriteByte('|')
		b.Write(canonicalJSON(r.Value))
		b.WriteByte('\n')
	}
	return fingerprint(b.String())
}

// PolicyRow is one row of `active_policies`.
type PolicyRow struct {
	Key    PolicyKey
	Region string
	Value  json.RawMessage
}

// canonicalJSON re-encodes a value so that whitespace and key order in the
// stored JSONB cannot change the fingerprint. Postgres already normalises
// `jsonb`, but the fingerprint must not depend on that remaining true.
func canonicalJSON(v json.RawMessage) []byte {
	var any any
	if err := json.Unmarshal(v, &any); err != nil {
		return v // Unparseable: fingerprint it verbatim rather than losing it.
	}
	out, err := json.Marshal(any) // Go marshals map keys in sorted order.
	if err != nil {
		return v
	}
	return out
}

// AgeBand is the donor age window (`donor_age_years`).
type AgeBand struct {
	Min          int
	Max          int
	FirstTimeMax int
}

// AgeBand reads the age window.
func (p *Policies) AgeBand() (AgeBand, error) {
	var v struct {
		Min          *int `json:"min"`
		Max          *int `json:"max"`
		FirstTimeMax *int `json:"first_time_max"`
	}
	if err := p.decode(KeyDonorAgeYears, &v); err != nil {
		return AgeBand{}, err
	}
	if v.Min == nil || v.Max == nil {
		return AgeBand{}, fmt.Errorf("%w: %s needs min and max", ErrPolicyMalformed, KeyDonorAgeYears)
	}
	if *v.Min > *v.Max {
		return AgeBand{}, fmt.Errorf("%w: %s has min above max", ErrPolicyMalformed, KeyDonorAgeYears)
	}
	band := AgeBand{Min: *v.Min, Max: *v.Max, FirstTimeMax: *v.Max}
	// An absent first-time cap means "no tighter than the general band", which
	// is the correct reading of an optional narrowing rule.
	if v.FirstTimeMax != nil {
		band.FirstTimeMax = *v.FirstTimeMax
	}
	return band, nil
}

// MinWeightKg reads the minimum donor weight.
func (p *Policies) MinWeightKg() (float64, error) {
	var v struct {
		Kg *float64 `json:"kg"`
	}
	if err := p.decode(KeyDonorMinWeightKg, &v); err != nil {
		return 0, err
	}
	if v.Kg == nil {
		return 0, fmt.Errorf("%w: %s needs kg", ErrPolicyMalformed, KeyDonorMinWeightKg)
	}
	return *v.Kg, nil
}

// MinHemoglobin reads the pre-donation haemoglobin floor for a donor.
//
// The policy states a female and a male threshold. For `other` and
// `undisclosed` it returns the HIGHER of the two — the stricter test. That
// direction is deliberate: the threshold protects the DONOR from being bled
// while anaemic, so when the applicable figure is unknown the system must be
// harder to pass, not easier. The alternative — quietly applying the lower
// figure — would make declining to state a gender a way to lower a safety bar.
func (p *Policies) MinHemoglobin(g Gender) (float64, error) {
	var v struct {
		Female *float64 `json:"female"`
		Male   *float64 `json:"male"`
	}
	if err := p.decode(KeyDonorMinHemoglobin, &v); err != nil {
		return 0, err
	}
	if v.Female == nil || v.Male == nil {
		return 0, fmt.Errorf("%w: %s needs female and male", ErrPolicyMalformed, KeyDonorMinHemoglobin)
	}
	switch g {
	case GenderFemale:
		return *v.Female, nil
	case GenderMale:
		return *v.Male, nil
	default:
		return max2(*v.Female, *v.Male), nil
	}
}

// Bound is an inclusive range read from policy. A nil end is "not bounded in
// that direction" — `temperature_c` has a maximum and no minimum.
type Bound struct {
	Min *float64
	Max *float64
}

// Contains reports whether v is inside the bound.
func (b Bound) Contains(v float64) bool {
	if b.Min != nil && v < *b.Min {
		return false
	}
	if b.Max != nil && v > *b.Max {
		return false
	}
	return true
}

// VitalsRange is the acceptable pre-donation vitals window.
type VitalsRange struct {
	BPSystolic  Bound
	BPDiastolic Bound
	Pulse       Bound
	Temperature Bound
}

// VitalsRange reads the acceptable vitals window.
func (p *Policies) VitalsRange() (VitalsRange, error) {
	var v struct {
		BPSystolic  *bound `json:"bp_systolic"`
		BPDiastolic *bound `json:"bp_diastolic"`
		Pulse       *bound `json:"pulse_bpm"`
		Temperature *bound `json:"temperature_c"`
	}
	if err := p.decode(KeyDonorVitalsRange, &v); err != nil {
		return VitalsRange{}, err
	}
	if v.BPSystolic == nil || v.BPDiastolic == nil || v.Pulse == nil || v.Temperature == nil {
		return VitalsRange{}, fmt.Errorf("%w: %s needs bp_systolic, bp_diastolic, pulse_bpm and temperature_c",
			ErrPolicyMalformed, KeyDonorVitalsRange)
	}
	return VitalsRange{
		BPSystolic:  v.BPSystolic.bound(),
		BPDiastolic: v.BPDiastolic.bound(),
		Pulse:       v.Pulse.bound(),
		Temperature: v.Temperature.bound(),
	}, nil
}

type bound struct {
	Min *float64 `json:"min"`
	Max *float64 `json:"max"`
}

func (b *bound) bound() Bound { return Bound{Min: b.Min, Max: b.Max} }

// IntervalDays reads the minimum gap between donations of one procedure.
func (p *Policies) IntervalDays(proc Procedure) (int, error) {
	var v struct {
		Days *int `json:"days"`
	}
	key := IntervalKey(proc)
	if err := p.decode(key, &v); err != nil {
		return 0, err
	}
	if v.Days == nil {
		return 0, fmt.Errorf("%w: %s needs days", ErrPolicyMalformed, key)
	}
	if *v.Days < 0 {
		return 0, fmt.Errorf("%w: %s is negative", ErrPolicyMalformed, key)
	}
	return *v.Days, nil
}

// AnnualCap reads the maximum donations in a rolling twelve months.
//
// Whole blood is capped by sex; apheresis platelet has its own figure that does
// not vary by sex. A procedure with no configured cap is uncapped, which is a
// real state — `apheresis_plasma` has no cap in the seeded set — and is
// reported as such rather than as zero, because a zero cap would block every
// donation of that procedure.
func (p *Policies) AnnualCap(g Gender, proc Procedure) (cap int, capped bool, err error) {
	var v map[string]*int
	if err := p.decode(KeyDonationsPerYearMax, &v); err != nil {
		return 0, false, err
	}

	// A per-procedure figure wins over the by-sex one: the by-sex numbers are
	// the whole-blood rule, and a procedure that states its own is stating it
	// because the whole-blood rule does not apply.
	if n, present := v[string(proc)]; present && n != nil {
		return *n, true, nil
	}
	if proc != ProcedureWholeBlood {
		return 0, false, nil
	}

	female, male := v["female"], v["male"]
	if female == nil || male == nil {
		return 0, false, fmt.Errorf("%w: %s needs female and male, or a %s entry",
			ErrPolicyMalformed, KeyDonationsPerYearMax, proc)
	}
	switch g {
	case GenderFemale:
		return *female, true, nil
	case GenderMale:
		return *male, true, nil
	default:
		// The stricter cap, for the same reason as the haemoglobin floor: an
		// unstated sex must not be the loosest option.
		return min2(*female, *male), true, nil
	}
}

// Hours reads a policy whose value is `{"hours": n}`.
func (p *Policies) Hours(key PolicyKey) (time.Duration, error) {
	var v struct {
		Hours *float64 `json:"hours"`
	}
	if err := p.decode(key, &v); err != nil {
		return 0, err
	}
	if v.Hours == nil {
		return 0, fmt.Errorf("%w: %s needs hours", ErrPolicyMalformed, key)
	}
	if *v.Hours <= 0 {
		return 0, fmt.Errorf("%w: %s must be positive", ErrPolicyMalformed, key)
	}
	return time.Duration(*v.Hours * float64(time.Hour)), nil
}

// ExpiryAlertWindow is how far ahead a unit counts as "expiring soon".
func (p *Policies) ExpiryAlertWindow() (time.Duration, error) {
	return p.Hours(KeyExpiryAlertHours)
}

// AllocationMinRemaining is the shelf life a unit must still have to be
// allocated.
func (p *Policies) AllocationMinRemaining() (time.Duration, error) {
	return p.Hours(KeyAllocationMinRemaining)
}

func (p *Policies) decode(key PolicyKey, into any) error {
	raw, ok := p.raw[key]
	if !ok {
		return fmt.Errorf("%w: %s", ErrPolicyMissing, key)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("%w: %s: %s", ErrPolicyMalformed, key, err)
	}
	return nil
}

// fingerprint is a short, stable digest. Twelve hex characters: long enough
// that two different policy sets will not collide in any realistic deployment,
// short enough to read in a log line or a stored decision.
func fingerprint(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:6])
}

func max2(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}
