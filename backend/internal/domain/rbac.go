package domain

import "strings"

// The RBAC permission matrix from TRD §7.6, expressed as data rather than as
// scattered `if role == "admin"` checks.
//
// Encoding it here (in domain, which imports nothing) means the whole matrix is
// unit-testable without a database, an HTTP server, or a token — and a test can
// assert every cell, positively and negatively.

type Role string

const (
	RoleDonor            Role = "donor"
	RoleStaff            Role = "staff"
	RoleLabTech          Role = "lab_tech"
	RoleInventoryManager Role = "inventory_manager"
	RoleHospitalUser     Role = "hospital_user"
	RoleAdmin            Role = "admin"
)

func (r Role) Valid() bool {
	switch r {
	case RoleDonor, RoleStaff, RoleLabTech, RoleInventoryManager, RoleHospitalUser, RoleAdmin:
		return true
	}
	return false
}

type Action string

const (
	Create  Action = "C"
	Read    Action = "R"
	Update  Action = "U"
	Delete  Action = "D"
	Execute Action = "X"
)

// Scope narrows a granted action to a subset of rows. The permission check
// returns it so the service layer knows which mandatory WHERE clause to apply —
// a scope is not advisory.
type Scope string

const (
	ScopeAll       Scope = ""     // no narrowing
	ScopeOwn       Scope = "own"  // rows the caller owns
	ScopeCenter    Scope = "ctr"  // rows at the caller's center
	ScopeHospital  Scope = "hosp" // rows for the caller's hospital
	ScopeAggregate Scope = "agg"  // aggregate figures only, no row detail
)

type grant struct {
	actions string // concatenated action letters, e.g. "CRU"
	scope   Scope
}

// permissions[resource][role]. Absent means no access.
var permissions = map[string]map[Role]grant{
	"users": {
		RoleDonor: {"R", ScopeOwn}, RoleAdmin: {"CRUD", ScopeAll},
	},
	"donor_profiles": {
		RoleDonor: {"RU", ScopeOwn}, RoleStaff: {"CRU", ScopeCenter},
		RoleLabTech: {"R", ScopeAll}, RoleAdmin: {"CRUD", ScopeAll},
	},
	"donation_centers": {
		RoleDonor: {"R", ScopeAll}, RoleStaff: {"R", ScopeAll}, RoleLabTech: {"R", ScopeAll},
		RoleInventoryManager: {"R", ScopeAll}, RoleHospitalUser: {"R", ScopeAll},
		RoleAdmin: {"CRUD", ScopeAll},
	},
	"storage_locations": {
		RoleLabTech: {"R", ScopeAll}, RoleInventoryManager: {"CRU", ScopeAll}, RoleAdmin: {"CRUD", ScopeAll},
	},
	"donation_requests": {
		RoleDonor: {"CRX", ScopeOwn}, RoleStaff: {"CRX", ScopeCenter}, RoleAdmin: {"CRUDX", ScopeAll},
	},
	"appointments": {
		RoleDonor: {"RX", ScopeOwn}, RoleStaff: {"CRUX", ScopeCenter},
		RoleLabTech: {"R", ScopeAll}, RoleAdmin: {"CRUDX", ScopeAll},
	},
	"screenings": {
		RoleDonor: {"R", ScopeOwn}, RoleStaff: {"CR", ScopeCenter},
		RoleLabTech: {"R", ScopeAll}, RoleAdmin: {"RU", ScopeAll},
	},
	"deferrals": {
		RoleDonor: {"R", ScopeOwn}, RoleStaff: {"CR", ScopeAll},
		RoleLabTech: {"CR", ScopeAll}, RoleAdmin: {"CRUX", ScopeAll},
	},
	"donations": {
		RoleDonor: {"R", ScopeOwn}, RoleStaff: {"CR", ScopeCenter},
		RoleLabTech: {"R", ScopeAll}, RoleInventoryManager: {"R", ScopeAll}, RoleAdmin: {"RU", ScopeAll},
	},
	"blood_units": {
		RoleStaff: {"R", ScopeCenter}, RoleLabTech: {"RX", ScopeAll},
		RoleInventoryManager: {"CRUX", ScopeAll}, RoleAdmin: {"RX", ScopeAll},
	},
	"unit_status_events": {
		RoleStaff: {"R", ScopeAll}, RoleLabTech: {"CR", ScopeAll},
		RoleInventoryManager: {"CR", ScopeAll}, RoleAdmin: {"R", ScopeAll},
	},
	"test_results": {
		RoleDonor: {"R", ScopeOwn}, RoleLabTech: {"CRU", ScopeAll},
		RoleInventoryManager: {"R", ScopeAll}, RoleAdmin: {"R", ScopeAll},
	},
	"inventory_summary": {
		RoleStaff: {"R", ScopeCenter}, RoleLabTech: {"R", ScopeAll},
		RoleInventoryManager: {"R", ScopeAll}, RoleHospitalUser: {"R", ScopeAggregate},
		RoleAdmin: {"R", ScopeAll},
	},
	"hospitals": {
		RoleInventoryManager: {"R", ScopeAll}, RoleHospitalUser: {"R", ScopeOwn}, RoleAdmin: {"CRUD", ScopeAll},
	},
	"blood_requests": {
		RoleInventoryManager: {"RUX", ScopeAll}, RoleHospitalUser: {"CRX", ScopeHospital},
		RoleAdmin: {"CRUDX", ScopeAll},
	},
	"unit_allocations": {
		RoleInventoryManager: {"CRDX", ScopeAll}, RoleHospitalUser: {"R", ScopeHospital},
		RoleAdmin: {"CRDX", ScopeAll},
	},
	"issuances": {
		RoleInventoryManager: {"CRX", ScopeAll}, RoleHospitalUser: {"RX", ScopeHospital},
		RoleAdmin: {"CRUX", ScopeAll},
	},
	"policies": {
		RoleStaff: {"R", ScopeAll}, RoleLabTech: {"R", ScopeAll},
		RoleInventoryManager: {"R", ScopeAll}, RoleAdmin: {"CRU", ScopeAll},
	},
	// Read only, for admin only, at every role. An admin who can edit the audit
	// log makes the audit log worthless — so there is deliberately no C, U or D
	// cell here for anyone.
	"audit_log": {
		RoleAdmin: {"R", ScopeAll},
	},
	"notifications": {
		RoleDonor: {"R", ScopeOwn}, RoleHospitalUser: {"R", ScopeOwn}, RoleAdmin: {"CR", ScopeAll},
	},
	"reports": {
		RoleStaff: {"R", ScopeCenter}, RoleLabTech: {"R", ScopeAll},
		RoleInventoryManager: {"R", ScopeAll}, RoleHospitalUser: {"R", ScopeHospital},
		RoleAdmin: {"R", ScopeAll},
	},
	"files": {
		RoleDonor: {"CR", ScopeOwn}, RoleStaff: {"CR", ScopeCenter}, RoleLabTech: {"CR", ScopeAll},
		RoleInventoryManager: {"CR", ScopeAll}, RoleHospitalUser: {"CR", ScopeHospital},
		RoleAdmin: {"R", ScopeAll},
	},
}

// Can reports whether `role` may perform `action` on `resource`, and with what
// scope. A false return is a denial; callers must not fall back to "allow".
func Can(role Role, resource string, action Action) (Scope, bool) {
	byRole, ok := permissions[resource]
	if !ok {
		return ScopeAll, false // unknown resource: deny, never default-allow
	}
	g, ok := byRole[role]
	if !ok {
		return ScopeAll, false
	}
	if !strings.Contains(g.actions, string(action)) {
		return ScopeAll, false
	}
	return g.scope, true
}

// Resources returns every resource in the matrix, for exhaustive testing.
func Resources() []string {
	out := make([]string, 0, len(permissions))
	for r := range permissions {
		out = append(out, r)
	}
	return out
}

// AllRoles is every role, for exhaustive testing.
func AllRoles() []Role {
	return []Role{RoleDonor, RoleStaff, RoleLabTech, RoleInventoryManager, RoleHospitalUser, RoleAdmin}
}

// ---------------------------------------------------------------------------
// State transitions
//
// A single `X` letter is not enough. TRD §7.6 writes the execute cells as
// `X-cancel` for a donor and `X-approve/reject` for staff on the same resource:
// both hold X, but a donor approving their own donation request would defeat the
// entire review flow the application exists to run.
//
// So X is checked twice: `Can(..., Execute)` says the role may transition this
// resource at all, and `CanExecute` says it may perform *this* transition.
// ---------------------------------------------------------------------------

// transitions[resource][transition] = roles permitted to perform it.
//
// A resource absent from this map has no named transitions, and its unqualified
// `X` from the matrix stands on its own. A resource present here is strict: a
// transition name not listed is denied, so adding a new one is a deliberate act
// rather than something that starts working by accident.
var transitions = map[string]map[string][]Role{
	"donation_requests": {
		"cancel":  {RoleDonor, RoleStaff, RoleAdmin},
		"approve": {RoleStaff, RoleAdmin},
		"reject":  {RoleStaff, RoleAdmin},
	},
	"appointments": {
		"cancel":     {RoleDonor, RoleStaff, RoleAdmin},
		"reschedule": {RoleDonor, RoleStaff, RoleAdmin},
		// check_in lands with WI-39, which must also enforce the deferral block
		// (FR-19). It is deliberately absent until then rather than pre-granted.
	},
	"deferrals": {
		"lift": {RoleAdmin},
	},
	"blood_units": {
		"quarantine": {RoleLabTech, RoleAdmin},
		"discard":    {RoleLabTech, RoleInventoryManager, RoleAdmin},
		"move":       {RoleInventoryManager, RoleAdmin},
		"expire":     {RoleInventoryManager, RoleAdmin},
	},
	"blood_requests": {
		"approve": {RoleInventoryManager, RoleAdmin},
		"reject":  {RoleInventoryManager, RoleAdmin},
		"cancel":  {RoleHospitalUser, RoleAdmin},
	},
	"issuances": {
		"issue":   {RoleInventoryManager, RoleAdmin},
		"outcome": {RoleHospitalUser, RoleInventoryManager, RoleAdmin},
	},
}

// CanExecute reports whether `role` may perform the named state transition, and
// with what scope. It is the X column of the matrix, read at transition
// granularity. A false return is a denial.
func CanExecute(role Role, resource, transition string) (Scope, bool) {
	scope, ok := Can(role, resource, Execute)
	if !ok {
		return ScopeAll, false
	}
	named, hasNamed := transitions[resource]
	if !hasNamed {
		return scope, true // the matrix X is unqualified for this resource
	}
	allowed, known := named[transition]
	if !known {
		return ScopeAll, false // an unnamed transition is denied, never assumed
	}
	for _, r := range allowed {
		if r == role {
			return scope, true
		}
	}
	return ScopeAll, false
}

// NamedTransitions returns the transition names declared for a resource, for
// exhaustive testing. Empty means the resource's X is unqualified.
func NamedTransitions(resource string) []string {
	out := make([]string, 0, len(transitions[resource]))
	for t := range transitions[resource] {
		out = append(out, t)
	}
	return out
}
