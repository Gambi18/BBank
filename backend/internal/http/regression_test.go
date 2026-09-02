package http_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// ─── FR-65: authorization is enforced on the server ──────────────────────────

// Every guarded route refuses an anonymous caller. Mounted-ness is the point:
// a route added without its permission middleware would be quietly open, and
// this is what makes that conspicuous.
func TestFR65_AnonymousIsRefusedOnEveryGuardedRoute(t *testing.T) {
	h := newHarness(t)

	guarded := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/donors"},
		{http.MethodGet, "/api/v1/donors/1"},
		{http.MethodPatch, "/api/v1/donors/1"},
		{http.MethodGet, "/api/v1/donation-requests"},
		{http.MethodPost, "/api/v1/donation-requests"},
		{http.MethodGet, "/api/v1/donation-requests/1"},
		{http.MethodPost, "/api/v1/donation-requests/1/approve"},
		{http.MethodPost, "/api/v1/donation-requests/1/reject"},
		{http.MethodPost, "/api/v1/donation-requests/1/cancel"},
		{http.MethodGet, "/api/v1/appointments"},
		{http.MethodGet, "/api/v1/appointments/1"},
		{http.MethodGet, "/api/v1/users"},
		{http.MethodPost, "/api/v1/users"},
		{http.MethodPatch, "/api/v1/users/1"},
		{http.MethodGet, "/api/v1/admin/flags"},
		{http.MethodPatch, "/api/v1/admin/flags"},
	}
	for _, e := range guarded {
		got := h.do(t, e.method, e.path, "", `{}`)
		if got.status != http.StatusUnauthorized {
			t.Errorf("%s %s as anonymous = %d, want 401", e.method, e.path, got.status)
		}
	}
}

// The endpoints that MUST stay public, asserted so a later change that guards
// them shows up as a failure rather than as a support ticket.
func TestFR65_PublicRoutesStayPublic(t *testing.T) {
	h := newHarness(t)

	for _, path := range []string{"/healthz", "/readyz", "/api/v1/auth/public-key"} {
		if got := h.do(t, http.MethodGet, path, "", ""); got.status != http.StatusOK {
			t.Errorf("GET %s = %d, want 200 — operational and public", path, got.status)
		}
	}
	// Self-registration and invitation acceptance are reachable without a
	// session by design: neither caller has one yet.
	if got := h.do(t, http.MethodPost, "/api/v1/register", "", `{}`); got.status == http.StatusUnauthorized {
		t.Error("POST /api/v1/register requires a session; nobody registering has one")
	}
	if got := h.do(t, http.MethodPost, "/api/v1/invites/accept", "", `{}`); got.status == http.StatusUnauthorized {
		t.Error("POST /api/v1/invites/accept requires a session; an invitee has none")
	}
}

// The §7.6 matrix as it is MOUNTED. The middleware tests assert all 660 cells
// against synthetic routes; this asserts the handful that matter most are wired
// to the real paths — which is the failure mode a matrix test cannot see.
func TestFR65_RoleMatrixOverRealRoutes(t *testing.T) {
	h := newHarness(t)
	center := testsupport_CenterID(t, h)

	h.donor(t, "matrix.donor@example.test", "Matrix Donor")
	h.user(t, "matrix.staff@example.test", "staff", &center)
	h.user(t, "matrix.admin@example.test", "admin", nil)

	donor := h.token(t, "matrix.donor@example.test")
	staff := h.token(t, "matrix.staff@example.test")
	admin := h.token(t, "matrix.admin@example.test")

	cases := []struct {
		name   string
		token  string
		method string
		path   string
		want   int
	}{
		// Donors read their own world and nothing administrative.
		{"donor lists requests", donor, http.MethodGet, "/api/v1/donation-requests", http.StatusOK},
		{"donor lists appointments", donor, http.MethodGet, "/api/v1/appointments", http.StatusOK},
		// 200, but scoped to themselves — TestFR65_UsersEndpointDoesNotLeakOtherAccounts
		// asserts it contains nobody else.
		{"donor lists users (own scope)", donor, http.MethodGet, "/api/v1/users", http.StatusOK},
		{"donor reads flags", donor, http.MethodGet, "/api/v1/admin/flags", http.StatusForbidden},

		// Staff work their centre, but do not administer accounts.
		{"staff lists requests", staff, http.MethodGet, "/api/v1/donation-requests", http.StatusOK},
		{"staff lists donors", staff, http.MethodGet, "/api/v1/donors", http.StatusOK},
		{"staff lists users", staff, http.MethodGet, "/api/v1/users", http.StatusForbidden},
		{"staff reads flags", staff, http.MethodGet, "/api/v1/admin/flags", http.StatusForbidden},

		// Admin reaches everything.
		{"admin lists users", admin, http.MethodGet, "/api/v1/users", http.StatusOK},
		{"admin reads flags", admin, http.MethodGet, "/api/v1/admin/flags", http.StatusOK},
		{"admin lists donors", admin, http.MethodGet, "/api/v1/donors", http.StatusOK},
	}
	for _, c := range cases {
		if got := h.do(t, c.method, c.path, c.token, ""); got.status != c.want {
			t.Errorf("%s: %s %s = %d, want %d (body: %s)",
				c.name, c.method, c.path, got.status, c.want, truncate(got.body))
		}
	}
}

// A donor must not approve their own donation request. The matrix writes a
// donor's cell as `X-cancel` and staff's as `X-approve/reject`, and both hold
// X — so a bare Execute check would delete the review step the application
// exists to perform.
func TestFR65_DonorCannotApproveTheirOwnRequest(t *testing.T) {
	h := newHarness(t)
	center := testsupport_CenterID(t, h)
	donorID := h.donor(t, "selfapprove@example.test", "Self Approver")
	reqID := h.pendingRequest(t, donorID, center)
	donor := h.token(t, "selfapprove@example.test")

	got := h.do(t, http.MethodPost, "/api/v1/donation-requests/"+itoa(reqID)+"/approve", donor, `{"date":"2026-12-01"}`)
	if got.status != http.StatusForbidden {
		t.Fatalf("a donor approving their own request = %d, want 403", got.status)
	}
	if n := h.count(t, `SELECT count(*) FROM appointments WHERE donation_request_id = $1`, reqID); n != 0 {
		t.Fatal("the refused approval still created an appointment")
	}
}

// ─── WI-02 / A14: the ownership bypass, closed ───────────────────────────────

// **The named regression.** `getAppointment` once guarded ownership with
// `if donorId != ""`, so the check ran only when the caller *volunteered*
// `?donor_id=`. Omitting the parameter returned any donor's appointment: the
// guard was opt-in by the attacker.
//
// This asserts the fix at the level the defect lived at — an HTTP request with
// NO query parameter at all — and expects 404 rather than 403, because a 403
// confirms the record exists.
func TestWI02_AppointmentOwnershipHoldsWithNoDonorIdParameter(t *testing.T) {
	h := newHarness(t)
	center := testsupport_CenterID(t, h)

	ownerID := h.donor(t, "owner@example.test", "The Owner")
	h.donor(t, "intruder@example.test", "The Intruder")
	staffID := h.user(t, "wi02.staff@example.test", "staff", &center)

	reqID := h.pendingRequest(t, ownerID, center)
	apptID := h.approve(t, reqID, staffID, center)

	owner := h.token(t, "owner@example.test")
	intruder := h.token(t, "intruder@example.test")

	// The owner sees their own.
	if got := h.do(t, http.MethodGet, "/api/v1/appointments/"+itoa(apptID), owner, ""); got.status != http.StatusOK {
		t.Fatalf("the owner cannot read their own appointment: %d", got.status)
	}

	// The exact shape of the original bug: no `?donor_id=` supplied.
	got := h.do(t, http.MethodGet, "/api/v1/appointments/"+itoa(apptID), intruder, "")
	if got.status != http.StatusNotFound {
		t.Fatalf("another donor read appointment %d with no donor_id parameter: %d — WI-02 has regressed",
			apptID, got.status)
	}

	// And supplying someone else's id does not help either.
	got = h.do(t, http.MethodGet, "/api/v1/appointments/"+itoa(apptID)+"?donor_id="+itoa64(ownerID), intruder, "")
	if got.status != http.StatusNotFound {
		t.Fatalf("asserting the owner's donor_id granted access: %d", got.status)
	}
}

// `?donor_id=` survives only as a filter for callers already scoped wider than
// one donor. It may narrow a result set and must never widen one.
func TestWI02_DonorIdFilterCannotWidenAScope(t *testing.T) {
	h := newHarness(t)
	center := testsupport_CenterID(t, h)

	aID := h.donor(t, "filter.a@example.test", "Filter A")
	bID := h.donor(t, "filter.b@example.test", "Filter B")
	staffID := h.user(t, "filter.staff@example.test", "staff", &center)
	h.approve(t, h.pendingRequest(t, aID, center), staffID, center)
	h.approve(t, h.pendingRequest(t, bID, center), staffID, center)

	a := h.token(t, "filter.a@example.test")
	staff := h.token(t, "filter.staff@example.test")

	// Donor A asking for donor B still gets only their own row.
	got := h.do(t, http.MethodGet, "/api/v1/appointments?donor_id="+itoa64(bID), a, "")
	if got.status != http.StatusOK {
		t.Fatalf("status %d", got.status)
	}
	if total := got.page(t)["total"]; total != float64(1) {
		t.Errorf("donor A filtering to donor B saw total=%v, want 1 (their own)", total)
	}

	// Staff, scoped wider, may genuinely narrow with the same parameter.
	got = h.do(t, http.MethodGet, "/api/v1/appointments?donor_id="+itoa64(bID), staff, "")
	if total := got.page(t)["total"]; total != float64(1) {
		t.Errorf("staff filtering to donor B saw total=%v, want 1", total)
	}
	got = h.do(t, http.MethodGet, "/api/v1/appointments", staff, "")
	if total := got.page(t)["total"]; total != float64(2) {
		t.Errorf("staff unfiltered saw total=%v, want 2", total)
	}
}

// Staff are centre-scoped. Another centre's request is 404 — not 403, which
// would confirm it exists.
func TestFR65_StaffCannotReachAnotherCentresRequest(t *testing.T) {
	h := newHarness(t)
	mine := testsupport_CenterID(t, h)
	theirs := testsupport_SecondCenterID(t, h)

	donorID := h.donor(t, "elsewhere@example.test", "Elsewhere Donor")
	h.user(t, "mycentre.staff@example.test", "staff", &mine)
	reqID := h.pendingRequest(t, donorID, theirs)
	staff := h.token(t, "mycentre.staff@example.test")

	if got := h.do(t, http.MethodGet, "/api/v1/donation-requests/"+itoa(reqID), staff, ""); got.status != http.StatusNotFound {
		t.Errorf("reading another centre's request = %d, want 404", got.status)
	}
	if got := h.do(t, http.MethodPost, "/api/v1/donation-requests/"+itoa(reqID)+"/approve", staff, `{"date":"2026-12-01"}`); got.status != http.StatusNotFound {
		t.Errorf("approving another centre's request = %d, want 404", got.status)
	}
}

// ─── Auth boundaries (WI-17 / WI-19) ─────────────────────────────────────────

// A token must be refused unless it verifies. Tampering, forgery and
// algorithm confusion are each asserted at the HTTP boundary, because that is
// where an attacker presents them.
func TestFR65_AuthBoundaries(t *testing.T) {
	h := newHarness(t)
	h.donor(t, "boundary@example.test", "Boundary Donor")
	good := h.token(t, "boundary@example.test")

	// Sanity: the untouched token works, so the failures below mean something.
	if got := h.do(t, http.MethodGet, "/api/v1/appointments", good, ""); got.status != http.StatusOK {
		t.Fatalf("the valid token was refused: %d", got.status)
	}

	flipped := flipSignatureByte(t, good)
	forgedRole := forgeClaim(t, good, "role", "admin")
	algNone := algNoneToken(t, good)

	for name, tok := range map[string]string{
		"one flipped signature byte": flipped,
		"payload forged to admin":    forgedRole,
		"alg:none":                   algNone,
		"empty":                      "",
		"not a jwt":                  "definitely-not-a-token",
		"two segments":               strings.Join(strings.Split(good, ".")[:2], "."),
	} {
		if got := h.do(t, http.MethodGet, "/api/v1/appointments", tok, ""); got.status != http.StatusUnauthorized {
			t.Errorf("%s: got %d, want 401", name, got.status)
		}
	}

	// A forged admin role must not reach an admin-only route either.
	if got := h.do(t, http.MethodGet, "/api/v1/users", forgedRole, ""); got.status != http.StatusUnauthorized {
		t.Errorf("a forged admin claim reached /api/v1/users: %d", got.status)
	}
}

// token_version is the escape hatch from the access token's lifetime: bumping
// it must invalidate a live token on its NEXT REQUEST, not at its next login
// (FR-66). Asserted here over HTTP because that is the guarantee's real shape.
func TestFR66_SuspensionKillsALiveTokenOnTheNextRequest(t *testing.T) {
	h := newHarness(t)
	center := testsupport_CenterID(t, h)
	staffID := h.user(t, "tobesuspended@example.test", "staff", &center)
	h.user(t, "suspender@example.test", "admin", nil)

	victim := h.token(t, "tobesuspended@example.test")
	admin := h.token(t, "suspender@example.test")

	if got := h.do(t, http.MethodGet, "/api/v1/donation-requests", victim, ""); got.status != http.StatusOK {
		t.Fatalf("the staff token does not work before suspension: %d", got.status)
	}

	if got := h.do(t, http.MethodPatch, "/api/v1/users/"+itoa64(staffID), admin, `{"status":"suspended"}`); got.status != http.StatusOK {
		t.Fatalf("suspend = %d: %s", got.status, truncate(got.body))
	}

	// Same token, still unexpired, still correctly signed.
	if got := h.do(t, http.MethodGet, "/api/v1/donation-requests", victim, ""); got.status != http.StatusUnauthorized {
		t.Fatalf("a suspended user's live token still works: %d — FR-66 has regressed", got.status)
	}
}

// ─── TD-15: no endpoint returns a raw driver error ───────────────────────────

// Every error path answers with the envelope and a machine-readable code, and
// none of them leak schema detail. `pq:`/`pgx`/`relation "x" does not exist` in
// a response body is how a database's shape reaches whoever asks for it.
func TestTD15_NoEndpointLeaksADriverError(t *testing.T) {
	h := newHarness(t)
	h.user(t, "leak.admin@example.test", "admin", nil)
	admin := h.token(t, "leak.admin@example.test")

	probes := []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/donors/999999", ""},
		{http.MethodGet, "/api/v1/donors/abc", ""},
		{http.MethodGet, "/api/v1/donation-requests/999999", ""},
		{http.MethodGet, "/api/v1/appointments/999999", ""},
		{http.MethodGet, "/api/v1/users/999999", ""},
		{http.MethodPost, "/api/v1/donation-requests/999999/approve", `{"date":"2026-12-01"}`},
		{http.MethodPost, "/api/v1/donation-requests/999999/reject", `{"reason":"center_closed"}`},
		{http.MethodPost, "/api/v1/users", `{"email":"x@y.test","role":"staff"}`},
		{http.MethodPost, "/api/v1/users", `{"email":"not-an-email","role":"admin"}`},
		{http.MethodPatch, "/api/v1/users/1", `{"status":"nonsense"}`},
		{http.MethodGet, "/api/v1/donors?limit=abc", ""},
		{http.MethodPost, "/api/v1/donation-requests", `{ this is not json`},
	}

	leaks := []string{"pq:", "pgx", "sqlstate", "relation \"", "column \"", "syntax error at or near",
		"violates check constraint", "violates unique constraint", "password_hash"}

	for _, p := range probes {
		got := h.do(t, p.method, p.path, admin, p.body)
		if got.status < 400 {
			continue // not an error path today; nothing to leak
		}
		lower := strings.ToLower(got.body)
		for _, needle := range leaks {
			if strings.Contains(lower, strings.ToLower(needle)) {
				t.Errorf("%s %s leaked %q: %s", p.method, p.path, needle, truncate(got.body))
			}
		}
		if code := got.errorCode(t); code == "" {
			t.Errorf("%s %s returned %d with no machine-readable error code: %s",
				p.method, p.path, got.status, truncate(got.body))
		}
	}
}

// ─── TD-17 / A15: list endpoints are bounded ─────────────────────────────────

// An unbounded list is how a table scan reaches production. The limit is
// clamped rather than rejected (§6.3), and the value reported is the value
// APPLIED — a caller paging on a number the server did not use skips rows
// silently, which is the bug WI-21 found live.
func TestTD17_ListEndpointsAreBoundedAndReportTheAppliedLimit(t *testing.T) {
	h := newHarness(t)
	h.user(t, "paging.admin@example.test", "admin", nil)
	admin := h.token(t, "paging.admin@example.test")

	for _, path := range []string{"/api/v1/donors", "/api/v1/donation-requests", "/api/v1/appointments", "/api/v1/users"} {
		def := h.do(t, http.MethodGet, path, admin, "")
		if def.status != http.StatusOK {
			t.Fatalf("GET %s = %d", path, def.status)
		}
		if limit := def.page(t)["limit"]; limit != float64(25) {
			t.Errorf("%s default limit = %v, want 25", path, limit)
		}

		over := h.do(t, http.MethodGet, path+"?limit=5000", admin, "")
		if over.status != http.StatusOK {
			t.Errorf("%s?limit=5000 = %d, want it clamped not rejected", path, over.status)
		}
		if limit := over.page(t)["limit"]; limit != float64(100) {
			t.Errorf("%s?limit=5000 reported limit=%v, want the applied 100", path, limit)
		}

		// Zero is meaningless, and reading it as "no limit" is how the
		// unbounded scan comes back.
		if zero := h.do(t, http.MethodGet, path+"?limit=0", admin, ""); zero.status != http.StatusBadRequest {
			t.Errorf("%s?limit=0 = %d, want 400", path, zero.status)
		}
	}
}

// ─── WI-21: the deprecated prefix is an alias, and it can be switched off ────

func TestWI21_LegacyPrefixIsAnAliasWithDeprecationHeaders(t *testing.T) {
	h := newHarness(t)
	h.user(t, "shim.admin@example.test", "admin", nil)
	admin := h.token(t, "shim.admin@example.test")

	legacy := h.do(t, http.MethodGet, "/api/go/requests", admin, "")
	if legacy.status != http.StatusOK {
		t.Fatalf("GET /api/go/requests = %d", legacy.status)
	}
	if legacy.header.Get("Deprecation") != "true" || legacy.header.Get("Sunset") == "" {
		t.Error("a legacy path answered without its Deprecation/Sunset headers")
	}

	canonical := h.do(t, http.MethodGet, "/api/v1/donation-requests", admin, "")
	if canonical.header.Get("Deprecation") != "" {
		t.Error("the canonical path was marked deprecated")
	}

	// Not a blanket prefix swap: only the §6.1 endpoints are aliased, or the
	// deprecated prefix quietly acquires every future endpoint.
	if got := h.do(t, http.MethodGet, "/api/go/users", admin, ""); got.status != http.StatusNotFound {
		t.Errorf("/api/go/users = %d, want 404 — the shim is not a general alias", got.status)
	}
}

// The acceptance criterion: the flag turns the shim off and on in a live
// process, and canonical paths are unaffected either way.
func TestWI21_ShimTogglesWithoutARestart(t *testing.T) {
	h := newHarness(t)
	h.user(t, "toggle.admin@example.test", "admin", nil)
	admin := h.token(t, "toggle.admin@example.test")

	if got := h.do(t, http.MethodGet, "/api/go/requests", admin, ""); got.status != http.StatusOK {
		t.Fatalf("shim on: %d", got.status)
	}

	if got := h.do(t, http.MethodPatch, "/api/v1/admin/flags", admin, `{"legacy_shim":false}`); got.status != http.StatusOK {
		t.Fatalf("switching the shim off = %d: %s", got.status, truncate(got.body))
	}

	off := h.do(t, http.MethodGet, "/api/go/requests", admin, "")
	// 410, not 404: retired by policy, not missing by accident.
	if off.status != http.StatusGone {
		t.Errorf("shim off: %d, want 410", off.status)
	}
	if got := h.do(t, http.MethodGet, "/api/v1/donation-requests", admin, ""); got.status != http.StatusOK {
		t.Errorf("switching the shim off broke the canonical path: %d", got.status)
	}

	if got := h.do(t, http.MethodPatch, "/api/v1/admin/flags", admin, `{"legacy_shim":true}`); got.status != http.StatusOK {
		t.Fatalf("switching it back on = %d", got.status)
	}
	if got := h.do(t, http.MethodGet, "/api/go/requests", admin, ""); got.status != http.StatusOK {
		t.Errorf("shim back on: %d", got.status)
	}
}

// ─── WI-22 / A8: approving does not delete the request ───────────────────────

// The defect that destroyed the audit chain: `confirm` used to DELETE the
// request row after creating the appointment, so every historical appointment
// lost the link back to who asked and when.
func TestWI22_ApprovingDoesNotDeleteTheRequest(t *testing.T) {
	h := newHarness(t)
	center := testsupport_CenterID(t, h)
	donorID := h.donor(t, "audit@example.test", "Audit Chain")
	h.user(t, "audit.staff@example.test", "staff", &center)
	reqID := h.pendingRequest(t, donorID, center)
	staff := h.token(t, "audit.staff@example.test")

	got := h.do(t, http.MethodPost, "/api/v1/donation-requests/"+itoa(reqID)+"/approve", staff, `{"date":"2026-12-01"}`)
	if got.status != http.StatusCreated {
		t.Fatalf("approve = %d: %s", got.status, truncate(got.body))
	}

	if n := h.count(t, `SELECT count(*) FROM donation_requests WHERE id = $1 AND status = 'approved'`, reqID); n != 1 {
		t.Fatal("the request row was deleted or not marked approved — A8 has regressed")
	}
	if n := h.count(t, `SELECT count(*) FROM appointments WHERE donation_request_id = $1`, reqID); n != 1 {
		t.Fatal("the appointment is not linked back to the request")
	}

	// The legacy spelling reaches the same handler and must not delete either:
	// deleting was the bug, so it is not preserved for compatibility.
	if got := h.do(t, http.MethodPost, "/api/go/requests/"+itoa(reqID)+"/confirm", staff, `{"date":"2026-12-02"}`); got.status != http.StatusConflict {
		t.Errorf("the legacy confirm on an approved request = %d, want 409", got.status)
	}
	if n := h.count(t, `SELECT count(*) FROM donation_requests WHERE id = $1`, reqID); n != 1 {
		t.Fatal("the legacy path deleted the request row")
	}
}

// FR-09: a rejection reason must come from the controlled list, and 'other'
// must carry a note — enforced at the HTTP boundary, not merely in the domain.
func TestFR09_RejectionRequiresAReasonFromTheControlledList(t *testing.T) {
	h := newHarness(t)
	center := testsupport_CenterID(t, h)
	donorID := h.donor(t, "reject.http@example.test", "Reject Me")
	h.user(t, "reject.staff@example.test", "staff", &center)
	reqID := h.pendingRequest(t, donorID, center)
	staff := h.token(t, "reject.staff@example.test")
	path := "/api/v1/donation-requests/" + itoa(reqID) + "/reject"

	for name, body := range map[string]string{
		"free text":         `{"reason":"donor seemed unsuitable"}`,
		"empty reason":      `{"reason":""}`,
		"other, no note":    `{"reason":"other"}`,
		"other, blank note": `{"reason":"other","note":"   "}`,
	} {
		if got := h.do(t, http.MethodPost, path, staff, body); got.status != http.StatusUnprocessableEntity {
			t.Errorf("%s = %d, want 422", name, got.status)
		}
	}
	if n := h.count(t, `SELECT count(*) FROM donation_requests WHERE id = $1 AND status = 'pending'`, reqID); n != 1 {
		t.Fatal("a refused rejection still changed the row")
	}

	if got := h.do(t, http.MethodPost, path, staff, `{"reason":"center_closed","note":"stocktake"}`); got.status != http.StatusNoContent {
		t.Fatalf("a valid rejection = %d: %s", got.status, truncate(got.body))
	}

	// The vocabulary is served so a UI cannot keep a copy that drifts.
	list := h.do(t, http.MethodGet, "/api/v1/donation-requests/rejection-reasons", staff, "")
	if list.status != http.StatusOK {
		t.Fatalf("rejection-reasons = %d", list.status)
	}
	var env struct {
		Data []struct{ Value, Label string } `json:"data"`
	}
	if err := json.Unmarshal([]byte(list.body), &env); err != nil || len(env.Data) == 0 {
		t.Fatalf("the reason list is empty or unparseable: %s", truncate(list.body))
	}
	for _, r := range env.Data {
		if r.Value == "" || r.Label == "" {
			t.Errorf("reason %+v has an empty value or label", r)
		}
	}
}

// ─── §6.2: everything is enveloped ───────────────────────────────────────────

// No bare arrays and no bare scalars: an unenveloped array cannot grow a `page`
// object later without breaking every client reading it.
func TestTRD62_EveryResponseIsEnveloped(t *testing.T) {
	h := newHarness(t)
	h.user(t, "envelope.admin@example.test", "admin", nil)
	admin := h.token(t, "envelope.admin@example.test")

	for _, path := range []string{
		"/api/v1/donors", "/api/v1/donation-requests", "/api/v1/appointments",
		"/api/v1/users", "/api/v1/admin/flags",
	} {
		got := h.do(t, http.MethodGet, path, admin, "")
		if got.status != http.StatusOK {
			t.Fatalf("GET %s = %d", path, got.status)
		}
		if strings.HasPrefix(strings.TrimSpace(got.body), "[") {
			t.Errorf("%s returned a bare array", path)
		}
		var env map[string]json.RawMessage
		if err := json.Unmarshal([]byte(got.body), &env); err != nil {
			t.Fatalf("%s is not a JSON object: %s", path, truncate(got.body))
		}
		if _, ok := env["data"]; !ok {
			t.Errorf("%s has no `data` key", path)
		}
		if _, ok := env["meta"]; !ok {
			t.Errorf("%s has no `meta` block", path)
		}
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func truncate(s string) string {
	if len(s) > 220 {
		return s[:220] + "…"
	}
	return s
}

func itoa(v int32) string { return itoa64(int64(v)) }
func itoa64(v int64) string {
	return strings.TrimSpace(jsonNumber(v))
}
func jsonNumber(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// flipSignatureByte changes one character in the MIDDLE of the signature.
//
// Flipping the LAST character does not reliably change anything: an ES256
// signature is 64 bytes, which is 86 base64url characters carrying 516 bits, so
// the final character has 4 unused bits and several spellings decode to the
// same signature. A test that mutates only those bits passes while proving
// nothing — which is exactly what this one did until it was checked.
func flipSignatureByte(t *testing.T, tok string) string {
	t.Helper()
	parts := strings.Split(tok, ".")
	if len(parts) != 3 || len(parts[2]) < 8 {
		t.Fatalf("not a signed jwt: %q", tok)
	}
	sig := []byte(parts[2])
	i := len(sig) / 2
	if sig[i] == 'A' {
		sig[i] = 'B'
	} else {
		sig[i] = 'A'
	}
	out := parts[0] + "." + parts[1] + "." + string(sig)
	if out == tok {
		t.Fatal("the flip did not change the token")
	}
	return out
}

// forgeClaim rewrites one payload claim and leaves the signature untouched —
// the cheapest forgery, and the one an unsigned-cookie system was vulnerable to.
func forgeClaim(t *testing.T, tok, key, value string) string {
	t.Helper()
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("not a jwt: %q", tok)
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatalf("parse payload: %v", err)
	}
	claims[key] = value
	out, _ := json.Marshal(claims)
	return parts[0] + "." + base64.RawURLEncoding.EncodeToString(out) + "." + parts[2]
}

// algNoneToken is the classic algorithm-confusion attack: claim the token is
// unsigned and hope the verifier believes it.
func algNoneToken(t *testing.T, tok string) string {
	t.Helper()
	parts := strings.Split(tok, ".")
	raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims map[string]any
	_ = json.Unmarshal(raw, &claims)
	claims["role"] = "admin"

	head, _ := json.Marshal(map[string]string{"alg": "none", "typ": "JWT"})
	body, _ := json.Marshal(claims)
	return base64.RawURLEncoding.EncodeToString(head) + "." +
		base64.RawURLEncoding.EncodeToString(body) + "."
}

// count runs a scalar query against the harness's pool.
func (h *harness) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := h.pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func (h *harness) pendingRequest(t *testing.T, donorID, centerID int64) int32 {
	t.Helper()
	var id int32
	err := h.pool.QueryRow(context.Background(), `
		INSERT INTO donation_requests (donor_id, center_id, preferred_date, status)
		VALUES ($1, $2, CURRENT_DATE + 7, 'pending') RETURNING id`, donorID, centerID).Scan(&id)
	if err != nil {
		t.Fatalf("insert request: %v", err)
	}
	return id
}

// approve inserts the appointment directly, so tests that are not about the
// approve path do not depend on it being correct.
func (h *harness) approve(t *testing.T, reqID int32, staffID, centerID int64) int32 {
	t.Helper()
	var donorID int64
	if err := h.pool.QueryRow(context.Background(),
		`SELECT donor_id FROM donation_requests WHERE id = $1`, reqID).Scan(&donorID); err != nil {
		t.Fatalf("read request: %v", err)
	}
	var apptID int32
	err := h.pool.QueryRow(context.Background(), `
		INSERT INTO appointments (donation_request_id, donor_id, center_id, scheduled_at, status, created_by)
		VALUES ($1, $2, $3, now() + INTERVAL '7 days', 'scheduled', $4) RETURNING id`,
		reqID, donorID, centerID, staffID).Scan(&apptID)
	if err != nil {
		t.Fatalf("insert appointment: %v", err)
	}
	if _, err := h.pool.Exec(context.Background(),
		`UPDATE donation_requests SET status = 'approved', reviewed_at = now() WHERE id = $1`, reqID); err != nil {
		t.Fatalf("mark approved: %v", err)
	}
	return apptID
}

func testsupport_CenterID(t *testing.T, h *harness) int64 {
	t.Helper()
	var id int64
	if err := h.pool.QueryRow(context.Background(),
		`SELECT id FROM donation_centers WHERE code = 'MAIN'`).Scan(&id); err != nil {
		t.Fatalf("seeded MAIN centre missing: %v", err)
	}
	return id
}

func testsupport_SecondCenterID(t *testing.T, h *harness) int64 {
	t.Helper()
	var id int64
	if err := h.pool.QueryRow(context.Background(),
		`SELECT id FROM donation_centers WHERE code <> 'MAIN' ORDER BY id LIMIT 1`).Scan(&id); err != nil {
		t.Fatalf("no second centre seeded: %v", err)
	}
	return id
}

// A donor holds `R` on `users` scoped `own` (§7.6) so they can read their own
// account. That grant must not become a directory of everybody else's.
//
// This caught a real defect in WI-18: `UserHandler.list` required the `users`
// Read permission but never consulted the granted SCOPE, so a donor's own-scope
// grant returned the full list — every email, role, status and last login.
func TestFR65_UsersEndpointDoesNotLeakOtherAccounts(t *testing.T) {
	h := newHarness(t)
	donorID := h.donor(t, "nosy@example.test", "Nosy Donor")
	otherID := h.donor(t, "private@example.test", "Private Donor")
	adminID := h.user(t, "leak.check.admin@example.test", "admin", nil)
	donor := h.token(t, "nosy@example.test")

	list := h.do(t, http.MethodGet, "/api/v1/users", donor, "")
	switch list.status {
	case http.StatusForbidden:
		// Acceptable: the list is admin-only.
	case http.StatusOK:
		// Also acceptable, but ONLY if narrowed to themselves.
		if strings.Contains(list.body, "private@example.test") ||
			strings.Contains(list.body, "leak.check.admin@example.test") {
			t.Fatalf("a donor listing /api/v1/users saw other accounts: %s", truncate(list.body))
		}
		if total := list.page(t)["total"]; total != float64(1) {
			t.Errorf("a donor's own-scope user list had total=%v, want 1", total)
		}
	default:
		t.Fatalf("GET /api/v1/users as donor = %d", list.status)
	}

	// Reading their own record is the point of the grant.
	if got := h.do(t, http.MethodGet, "/api/v1/users/"+itoa64(donorID), donor, ""); got.status != http.StatusOK {
		t.Errorf("a donor cannot read their own user record: %d", got.status)
	}
	// Anyone else's must be 404 — not 403, which confirms it exists.
	for name, id := range map[string]int64{"another donor": otherID, "an admin": adminID} {
		if got := h.do(t, http.MethodGet, "/api/v1/users/"+itoa64(id), donor, ""); got.status != http.StatusNotFound {
			t.Errorf("a donor read %s's user record (id %d): %d — want 404", name, id, got.status)
		}
	}

	// The same rule for writes: a donor must not be able to promote anyone,
	// themselves included.
	for _, id := range []int64{donorID, otherID, adminID} {
		if got := h.do(t, http.MethodPatch, "/api/v1/users/"+itoa64(id), donor, `{"role":"admin"}`); got.status < 400 {
			t.Errorf("a donor changed the role of user %d: %d", id, got.status)
		}
	}
}

// FR-11: a donor may cancel and reschedule their OWN appointment, and nobody
// else's. The transition is declared for donors in §7.6, so the guard that
// matters here is ownership rather than role.
func TestFR11_CancelAndRescheduleAreOwnershipScoped(t *testing.T) {
	h := newHarness(t)
	center := testsupport_CenterID(t, h)

	ownerID := h.donor(t, "mine@example.test", "Mine")
	h.donor(t, "theirs@example.test", "Theirs")
	staffID := h.user(t, "fr11.staff@example.test", "staff", &center)
	apptID := h.approve(t, h.pendingRequest(t, ownerID, center), staffID, center)

	owner := h.token(t, "mine@example.test")
	other := h.token(t, "theirs@example.test")
	base := "/api/v1/appointments/" + itoa(apptID)

	// Another donor cannot touch it, and gets 404 rather than 403.
	for _, action := range []string{"/cancel", "/reschedule"} {
		if got := h.do(t, http.MethodPost, base+action, other, `{"scheduled_at":"2027-01-01T09:00:00Z"}`); got.status != http.StatusNotFound {
			t.Errorf("another donor POST %s = %d, want 404", action, got.status)
		}
	}
	if h.count(t, `SELECT count(*) FROM appointments WHERE id = $1 AND status = 'scheduled'`, apptID) != 1 {
		t.Fatal("an unauthorized call changed the appointment")
	}

	// The owner can reschedule it forward, then cancel it.
	if got := h.do(t, http.MethodPost, base+"/reschedule", owner, `{"scheduled_at":"2027-01-01T09:00:00Z"}`); got.status != http.StatusNoContent {
		t.Fatalf("owner reschedule = %d: %s", got.status, truncate(got.body))
	}
	if got := h.do(t, http.MethodPost, base+"/cancel", owner, `{"reason":"away that week"}`); got.status != http.StatusNoContent {
		t.Fatalf("owner cancel = %d: %s", got.status, truncate(got.body))
	}
	if h.count(t, `SELECT count(*) FROM appointments WHERE id = $1 AND status = 'cancelled'`, apptID) != 1 {
		t.Fatal("the appointment was not cancelled")
	}

	// A decided appointment is terminal: cancelling again is a conflict.
	if got := h.do(t, http.MethodPost, base+"/cancel", owner, `{}`); got.status != http.StatusConflict {
		t.Errorf("cancelling twice = %d, want 409", got.status)
	}

	// A malformed timestamp is a 422 naming the field, not a 500.
	other2 := h.approve(t, h.pendingRequest(t, ownerID, center), staffID, center)
	if got := h.do(t, http.MethodPost, "/api/v1/appointments/"+itoa(other2)+"/reschedule", owner, `{"scheduled_at":"next tuesday"}`); got.status != http.StatusUnprocessableEntity {
		t.Errorf("a malformed timestamp = %d, want 422", got.status)
	}
}

// Staff donor access must be CONSISTENT between the list and the single read.
//
// It was not. §7.6 grants staff `CRU` on `donor_profiles` scoped `ctr`, but
// `donor_profiles` has no centre column — a donor may attend any centre — so
// `ctr` was unimplementable and each code path guessed differently: `list` fell
// through to the *entire* registry, while `resolveOwned` asked Permits for a row
// with a nil centre, which `ScopeCenter` always rejects, so every single-donor
// read was 404. Staff could see every donor at once and none of them
// individually.
//
// TRD §6.5 settles the direction: `GET /api/v1/donors` is granted to staff as
// registry search, with `center_id` an optional FILTER. Check-in (WI-39) needs
// it too — a donor walking in may never have attended this centre before.
func TestFR65_StaffDonorAccessIsConsistent(t *testing.T) {
	h := newHarness(t)
	center := testsupport_CenterID(t, h)
	donorID := h.donor(t, "registry@example.test", "Registry Donor")
	h.user(t, "desk.staff@example.test", "staff", &center)
	staff := h.token(t, "desk.staff@example.test")

	list := h.do(t, http.MethodGet, "/api/v1/donors", staff, "")
	if list.status != http.StatusOK {
		t.Fatalf("staff registry list = %d", list.status)
	}
	listsThem := strings.Contains(list.body, "registry@example.test")

	single := h.do(t, http.MethodGet, "/api/v1/donors/"+itoa64(donorID), staff, "")
	readsThem := single.status == http.StatusOK

	if listsThem != readsThem {
		t.Fatalf("staff can list this donor (%v) but read them individually (%v) — "+
			"the two paths disagree about what `ctr` means on donor_profiles", listsThem, readsThem)
	}
	if !readsThem {
		t.Fatalf("staff cannot read a donor record (%d); check-in (WI-39) depends on it", single.status)
	}
	if got := h.do(t, http.MethodGet, "/api/v1/donors/"+itoa64(donorID)+"/eligibility", staff, ""); got.status != http.StatusOK {
		t.Errorf("staff cannot read donor eligibility: %d", got.status)
	}

	// A donor still sees only themselves, whichever path they take.
	other := h.donor(t, "otherdonor@example.test", "Other Donor")
	self := h.token(t, "registry@example.test")
	own := h.do(t, http.MethodGet, "/api/v1/donors", self, "")
	if strings.Contains(own.body, "otherdonor@example.test") {
		t.Error("a donor listing the registry saw another donor")
	}
	if got := h.do(t, http.MethodGet, "/api/v1/donors/"+itoa64(other), self, ""); got.status != http.StatusNotFound {
		t.Errorf("a donor read another donor's record: %d, want 404", got.status)
	}
}

// WI-24: the centre directory is PUBLIC and writing to it is admin-only.
//
// TRD §6.5 marks `GET /api/v1/centers` as `pub` and `POST`/`PATCH` as `admin`.
// A route table is a claim until something drives it: `WI-30` exists because a
// permission the matrix granted correctly was mounted on the wrong handler, and
// this is a new handler with a public route on it — the exact shape that went
// wrong before.
func TestWI24_CentreDirectoryIsPublicAndWritesAreAdminOnly(t *testing.T) {
	h := newHarness(t)
	center := testsupport_CenterID(t, h)
	_ = center

	// Anonymous can read the directory.
	anon := h.do(t, http.MethodGet, "/api/v1/centers", "", "")
	if anon.status != http.StatusOK {
		t.Fatalf("anonymous GET /centers = %d, want 200 (TRD §6.5 marks it pub)", anon.status)
	}
	if !strings.Contains(anon.body, `"data"`) {
		t.Errorf("the public directory is not enveloped: %s", truncate(anon.body))
	}

	// ...and cannot write to it.
	body := `{"code":"ANON","name":"Anonymous Centre","address_line":"1 Road","city":"Douala","region":"Littoral"}`
	if got := h.do(t, http.MethodPost, "/api/v1/centers", "", body); got.status == http.StatusCreated {
		t.Fatal("an anonymous caller created a donation centre")
	}

	// A donor may read a centre but not create one — §7.6 gives every role R and
	// only admin C.
	h.donor(t, "centrereader@example.test", "Centre Reader")
	donor := h.token(t, "centrereader@example.test")
	if got := h.do(t, http.MethodGet, "/api/v1/centers", donor, ""); got.status != http.StatusOK {
		t.Errorf("a donor cannot read the centre directory: %d", got.status)
	}
	if got := h.do(t, http.MethodPost, "/api/v1/centers", donor, body); got.status != http.StatusForbidden {
		t.Errorf("a donor creating a centre = %d, want 403", got.status)
	}

	// Staff neither.
	staffCenter := testsupport_CenterID(t, h)
	h.user(t, "centrestaff@example.test", "staff", &staffCenter)
	staff := h.token(t, "centrestaff@example.test")
	if got := h.do(t, http.MethodPost, "/api/v1/centers", staff, body); got.status != http.StatusForbidden {
		t.Errorf("staff creating a centre = %d, want 403", got.status)
	}

	// An admin can.
	h.user(t, "centreadmin@example.test", "admin", nil)
	admin := h.token(t, "centreadmin@example.test")
	created := h.do(t, http.MethodPost, "/api/v1/centers", admin, body)
	if created.status != http.StatusCreated {
		t.Fatalf("an admin creating a centre = %d: %s", created.status, truncate(created.body))
	}
}

// A deactivated centre disappears from the public directory but stays readable
// by id — the history has to remain reachable (FR-14).
func TestWI24_DeactivatedCentresLeaveThePublicDirectory(t *testing.T) {
	h := newHarness(t)
	center := testsupport_CenterID(t, h)

	h.user(t, "estateadmin@example.test", "admin", nil)
	admin := h.token(t, "estateadmin@example.test")

	before := h.do(t, http.MethodGet, "/api/v1/centers", "", "")
	if !strings.Contains(before.body, `"MAIN"`) {
		t.Fatalf("the seeded centre is missing from the directory: %s", truncate(before.body))
	}

	if got := h.do(t, http.MethodPatch, "/api/v1/centers/"+itoa64(center), admin, `{"is_active":false}`); got.status != http.StatusOK {
		t.Fatalf("deactivate = %d: %s", got.status, truncate(got.body))
	}

	after := h.do(t, http.MethodGet, "/api/v1/centers", "", "")
	if strings.Contains(after.body, `"MAIN"`) {
		t.Error("a deactivated centre is still in the public directory — that sends people to a locked door")
	}
	// An admin can still find it, or reopening would be impossible through the API.
	admins := h.do(t, http.MethodGet, "/api/v1/centers?include_inactive=true", admin, "")
	if !strings.Contains(admins.body, `"MAIN"`) {
		t.Error("an admin cannot see the centre they closed")
	}
	// And it is still readable by id.
	if got := h.do(t, http.MethodGet, "/api/v1/centers/"+itoa64(center), admin, ""); got.status != http.StatusOK {
		t.Errorf("a closed centre is unreadable by id: %d", got.status)
	}
}

// The slots endpoint is `auth`, not public: how full a centre is on Tuesday is
// operational detail.
func TestWI24_SlotsRequireASession(t *testing.T) {
	h := newHarness(t)
	center := testsupport_CenterID(t, h)
	path := "/api/v1/centers/" + itoa64(center) + "/slots?date=2026-10-05"

	if got := h.do(t, http.MethodGet, path, "", ""); got.status != http.StatusUnauthorized {
		t.Errorf("anonymous GET slots = %d, want 401", got.status)
	}

	h.donor(t, "slotreader@example.test", "Slot Reader")
	donor := h.token(t, "slotreader@example.test")
	got := h.do(t, http.MethodGet, path, donor, "")
	if got.status != http.StatusOK {
		t.Fatalf("a donor reading slots = %d: %s", got.status, truncate(got.body))
	}
	if !strings.Contains(got.body, `"data"`) {
		t.Errorf("the slots response is not enveloped: %s", truncate(got.body))
	}
}
