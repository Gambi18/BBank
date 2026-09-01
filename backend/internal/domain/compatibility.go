package domain

// ABO/Rh compatibility for RED CELLS.
//
// This table is the authoritative in-process copy of what `abo_compatibility`
// holds in the database (27 rows). It exists here so allocation logic is unit
// testable without a database, and there is a test asserting the two agree —
// if they ever diverge, that test fails rather than a patient receiving the
// wrong unit.
//
// Direction matters and is the easiest thing to get backwards: the map is keyed
// by RECIPIENT and lists the donor groups that recipient may SAFELY RECEIVE.
// O- is the universal donor; AB+ the universal recipient.
//
// PLASMA compatibility is the inverse of this and is NOT covered here. Do not
// reuse this table for plasma — AB is the universal plasma donor, not O.

type TypedUnit struct {
	Group  BloodGroup
	Rhesus Rhesus
}

func (t TypedUnit) String() string {
	sign := "+"
	if t.Rhesus == RhNegative {
		sign = "-"
	}
	return string(t.Group) + sign
}

var redCellCompatibility = map[TypedUnit][]TypedUnit{
	{GroupO, RhNegative}:  {{GroupO, RhNegative}},
	{GroupO, RhPositive}:  {{GroupO, RhNegative}, {GroupO, RhPositive}},
	{GroupA, RhNegative}:  {{GroupO, RhNegative}, {GroupA, RhNegative}},
	{GroupA, RhPositive}:  {{GroupO, RhNegative}, {GroupO, RhPositive}, {GroupA, RhNegative}, {GroupA, RhPositive}},
	{GroupB, RhNegative}:  {{GroupO, RhNegative}, {GroupB, RhNegative}},
	{GroupB, RhPositive}:  {{GroupO, RhNegative}, {GroupO, RhPositive}, {GroupB, RhNegative}, {GroupB, RhPositive}},
	{GroupAB, RhNegative}: {{GroupO, RhNegative}, {GroupA, RhNegative}, {GroupB, RhNegative}, {GroupAB, RhNegative}},
	{GroupAB, RhPositive}: {
		{GroupO, RhNegative}, {GroupO, RhPositive},
		{GroupA, RhNegative}, {GroupA, RhPositive},
		{GroupB, RhNegative}, {GroupB, RhPositive},
		{GroupAB, RhNegative}, {GroupAB, RhPositive},
	},
}

// CompatibleDonorsFor returns the donor types a recipient may receive red cells
// from, most-restrictive first so a caller preserving scarce O- naturally
// prefers an exact match. The slice is a copy; callers may not mutate the table.
func CompatibleDonorsFor(recipient TypedUnit) []TypedUnit {
	src, ok := redCellCompatibility[recipient]
	if !ok {
		return nil
	}
	out := make([]TypedUnit, len(src))
	copy(out, src)
	return out
}

// IsCompatible reports whether `unit` may be transfused into `recipient`.
func IsCompatible(recipient, unit TypedUnit) bool {
	for _, c := range redCellCompatibility[recipient] {
		if c == unit {
			return true
		}
	}
	return false
}
