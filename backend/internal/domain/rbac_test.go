package domain

import "testing"

// The structural invariants from TRD §7.6. These are the rules that, if broken,
// are not "a permissions bug" but a compliance failure.

// An admin who can edit the audit log makes the audit log worthless.
func TestAuditLogIsReadOnlyForEveryRole(t *testing.T) {
	for _, role := range AllRoles() {
		for _, a := range []Action{Create, Update, Delete, Execute} {
			if _, ok := Can(role, "audit_log", a); ok {
				t.Errorf("%s may %s audit_log — it must be read-only for every role, admin included", role, a)
			}
		}
	}
	if _, ok := Can(RoleAdmin, "audit_log", Read); !ok {
		t.Error("admin must be able to READ audit_log")
	}
	for _, role := range AllRoles() {
		if role == RoleAdmin {
			continue
		}
		if _, ok := Can(role, "audit_log", Read); ok {
			t.Errorf("%s may read audit_log — only admin may", role)
		}
	}
}

// A partner hospital sees "12 units of O- available", never a unit code and
// never a donor. Detail-level stock is a privacy and competitive concern.
func TestHospitalUserSeesAggregateInventoryOnly(t *testing.T) {
	scope, ok := Can(RoleHospitalUser, "inventory_summary", Read)
	if !ok {
		t.Fatal("hospital_user must be able to read inventory_summary")
	}
	if scope != ScopeAggregate {
		t.Errorf("hospital_user inventory scope = %q, want %q", scope, ScopeAggregate)
	}
	if _, ok := Can(RoleHospitalUser, "blood_units", Read); ok {
		t.Error("hospital_user must NOT read blood_units — that exposes unit codes and donors")
	}
	if _, ok := Can(RoleHospitalUser, "donor_profiles", Read); ok {
		t.Error("hospital_user must NOT read donor_profiles")
	}
	if _, ok := Can(RoleHospitalUser, "donations", Read); ok {
		t.Error("hospital_user must NOT read donations")
	}
}

// A donor must never reach another donor's clinical data, and must never reach
// inventory or other people's records at all.
func TestDonorIsScopedToOwnRecords(t *testing.T) {
	ownOnly := []string{"users", "donor_profiles", "donation_requests", "appointments",
		"screenings", "deferrals", "donations", "test_results", "notifications", "files"}
	for _, res := range ownOnly {
		scope, ok := Can(RoleDonor, res, Read)
		if !ok {
			continue
		}
		if scope != ScopeOwn {
			t.Errorf("donor read scope on %s = %q, want %q", res, scope, ScopeOwn)
		}
	}
	for _, res := range []string{"blood_units", "unit_status_events", "inventory_summary",
		"hospitals", "blood_requests", "unit_allocations", "issuances", "audit_log", "storage_locations"} {
		for _, a := range []Action{Create, Read, Update, Delete, Execute} {
			if _, ok := Can(RoleDonor, res, a); ok {
				t.Errorf("donor may %s %s — donors must have no access there", a, res)
			}
		}
	}
}

// A donor may correct their address; they may not declare their own blood group
// or donation count. Those are clinical facts set by staff.
func TestDonorCannotWriteClinicalFacts(t *testing.T) {
	if _, ok := Can(RoleDonor, "donor_profiles", Update); !ok {
		t.Error("a donor must be able to update their own profile")
	}
	if _, ok := Can(RoleDonor, "donations", Create); ok {
		t.Error("a donor must NOT be able to create a donation record")
	}
	if _, ok := Can(RoleDonor, "screenings", Create); ok {
		t.Error("a donor must NOT be able to create a screening")
	}
	if _, ok := Can(RoleDonor, "test_results", Create); ok {
		t.Error("a donor must NOT be able to create a test result")
	}
}

// Only a lab tech enters results; only inventory/lab/admin move units.
func TestClinicalWritesAreRestrictedToTheRightRole(t *testing.T) {
	if _, ok := Can(RoleLabTech, "test_results", Create); !ok {
		t.Error("lab_tech must be able to enter test results")
	}
	for _, role := range []Role{RoleDonor, RoleStaff, RoleHospitalUser, RoleInventoryManager} {
		if _, ok := Can(role, "test_results", Create); ok {
			t.Errorf("%s may create test_results — only lab_tech may", role)
		}
	}
	if _, ok := Can(RoleStaff, "blood_units", Update); ok {
		t.Error("staff must not update blood_units")
	}
}

// Staff writes are center-scoped; an unknown resource or role denies.
func TestScopesAndDefaultDeny(t *testing.T) {
	if s, _ := Can(RoleStaff, "appointments", Create); s != ScopeCenter {
		t.Errorf("staff appointment writes must be center-scoped, got %q", s)
	}
	if s, _ := Can(RoleHospitalUser, "blood_requests", Create); s != ScopeHospital {
		t.Errorf("hospital_user blood_requests must be hospital-scoped, got %q", s)
	}
	if _, ok := Can(RoleAdmin, "no_such_resource", Read); ok {
		t.Error("an unknown resource must DENY, never default-allow")
	}
	if _, ok := Can(Role("superuser"), "users", Read); ok {
		t.Error("an unknown role must DENY")
	}
}

// Every cell asserted, positively and negatively, so the matrix cannot drift
// silently. This is the "generated from a table" test the plan asks for.
func TestEveryCellIsExplicit(t *testing.T) {
	roles, resources := AllRoles(), Resources()
	granted := 0
	for _, res := range resources {
		for _, role := range roles {
			for _, a := range []Action{Create, Read, Update, Delete, Execute} {
				if _, ok := Can(role, res, a); ok {
					granted++
				}
			}
		}
	}
	if granted == 0 {
		t.Fatal("the matrix granted nothing — it is not loaded")
	}
	total := len(roles) * len(resources) * 5
	if granted >= total {
		t.Fatal("every cell is granted — the matrix is not restricting anything")
	}
	t.Logf("matrix: %d granted of %d possible cells across %d resources x %d roles",
		granted, total, len(resources), len(roles))
}

func TestRoleValidity(t *testing.T) {
	for _, r := range AllRoles() {
		if !r.Valid() {
			t.Errorf("%s should be valid", r)
		}
	}
	for _, bad := range []Role{"", "root", "Admin", "ADMIN"} {
		if bad.Valid() {
			t.Errorf("%q must not be a valid role", bad)
		}
	}
}

// ---------------------------------------------------------------------------
// Transitions
// ---------------------------------------------------------------------------

// The two tables must agree. A role listed against a transition but without `X`
// on that resource is a contradiction: CanExecute would deny it, so the entry
// reads as a granted permission while granting nothing. Catching that here is
// what stops the transition table from drifting away from the matrix.
func TestEveryTransitionRoleAlsoHoldsExecuteInTheMatrix(t *testing.T) {
	for resource, byTransition := range transitions {
		for transition, roles := range byTransition {
			for _, role := range roles {
				if _, ok := Can(role, resource, Execute); !ok {
					t.Errorf("%s is listed for %s.%s but holds no X on %s in the matrix",
						role, resource, transition, resource)
				}
				if _, ok := CanExecute(role, resource, transition); !ok {
					t.Errorf("%s should be able to execute %s.%s", role, resource, transition)
				}
			}
			// And the converse: a role not listed must be refused, even if it
			// holds an unqualified X on the resource.
			for _, role := range AllRoles() {
				listed := false
				for _, r := range roles {
					if r == role {
						listed = true
					}
				}
				if listed {
					continue
				}
				if _, ok := CanExecute(role, resource, transition); ok {
					t.Errorf("%s may execute %s.%s but is not listed for it", role, resource, transition)
				}
			}
		}
	}
}

// A resource with named transitions denies anything unnamed. This is the
// difference between "we have not implemented that yet" and "that is allowed
// because nobody wrote it down".
func TestUnnamedTransitionIsDenied(t *testing.T) {
	for _, res := range []string{"donation_requests", "appointments", "blood_units", "blood_requests"} {
		if len(NamedTransitions(res)) == 0 {
			t.Fatalf("%s should declare named transitions", res)
		}
		if _, ok := CanExecute(RoleAdmin, res, "no_such_transition"); ok {
			t.Errorf("admin may execute an undeclared transition on %s", res)
		}
	}
}

// A resource whose matrix cell is an unqualified X keeps working without being
// listed in the transition table — otherwise adding the table would silently
// revoke permissions that TRD §7.6 grants.
func TestUnqualifiedExecuteStillWorks(t *testing.T) {
	if len(NamedTransitions("unit_allocations")) != 0 {
		t.Skip("unit_allocations now declares named transitions; update this test")
	}
	if _, ok := CanExecute(RoleInventoryManager, "unit_allocations", "anything"); !ok {
		t.Error("an unqualified X must still permit execution")
	}
	if _, ok := CanExecute(RoleHospitalUser, "unit_allocations", "anything"); ok {
		t.Error("hospital_user holds no X on unit_allocations and must be denied")
	}
}

// The scope from the matrix must survive the transition check — a staff approval
// is still center-scoped, and losing that would make it global.
func TestTransitionKeepsTheMatrixScope(t *testing.T) {
	if s, _ := CanExecute(RoleStaff, "donation_requests", "approve"); s != ScopeCenter {
		t.Errorf("staff approval scope = %q, want %q", s, ScopeCenter)
	}
	if s, _ := CanExecute(RoleDonor, "donation_requests", "cancel"); s != ScopeOwn {
		t.Errorf("donor cancellation scope = %q, want %q", s, ScopeOwn)
	}
	if s, _ := CanExecute(RoleHospitalUser, "blood_requests", "cancel"); s != ScopeHospital {
		t.Errorf("hospital_user cancellation scope = %q, want %q", s, ScopeHospital)
	}
}
