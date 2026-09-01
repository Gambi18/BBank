package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"bbank/internal/domain"
	"bbank/internal/platform"

	"github.com/golang-jwt/jwt/v5"
)

// fakeVerifier stands in for service.AuthService. These tests are about the
// authorization decision, not the signature check — platform's own tests cover
// ES256 verification, and mixing the two would obscure which of them failed.
type fakeVerifier struct {
	claims *platform.Claims
	err    error
}

func (f fakeVerifier) VerifyAccessToken(context.Context, string) (*platform.Claims, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.claims, nil
}

func claimsFor(role domain.Role, userID int64, center, hospital *int64) *platform.Claims {
	return &platform.Claims{
		SessionID:        "sess-1",
		Role:             string(role),
		CenterID:         center,
		HospitalID:       hospital,
		RegisteredClaims: jwt.RegisteredClaims{Subject: strconv.FormatInt(userID, 10)},
	}
}

func i64(n int64) *int64 { return &n }

// reached is the terminal handler. Arriving here means the middleware allowed
// the request; the body carries the granted scope, so a test asserts that the
// scope actually propagated instead of assuming it did.
func reached(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, "scope=%s", ScopeFrom(r.Context()))
}

func serve(v TokenVerifier, mw func(http.Handler) http.Handler, sendToken bool) *httptest.ResponseRecorder {
	h := Authenticate(v)(mw(http.HandlerFunc(reached)))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	if sendToken {
		req.Header.Set("Authorization", "Bearer any.token.value")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// ---------------------------------------------------------------------------
// Authentication
// ---------------------------------------------------------------------------

func TestAnonymousRequestIsRejectedWith401(t *testing.T) {
	v := fakeVerifier{claims: claimsFor(domain.RoleAdmin, 1, nil, nil)}
	for name, mw := range map[string]func(http.Handler) http.Handler{
		"RequireAuth":       func(n http.Handler) http.Handler { return RequireAuth(n) },
		"RequirePermission": RequirePermission("donor_profiles", domain.Read),
		"RequireTransition": RequireTransition("donation_requests", "approve"),
	} {
		rec := serve(v, mw, false) // no Authorization header
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: anonymous request got %d, want 401", name, rec.Code)
		}
	}
}

// A token that fails verification must not authenticate. The failure modes are
// listed separately because an expired token and a tampered one are different
// events operationally, and both must land on the same answer: 401.
func TestRejectedTokenDoesNotAuthenticate(t *testing.T) {
	for name, err := range map[string]error{
		"expired":         platform.ErrTokenExpired,
		"stale version":   platform.ErrTokenVerMismatch,
		"bad signature":   errors.New("crypto/ecdsa: verification error"),
		"unknown session": errors.New("session revoked"),
	} {
		rec := serve(fakeVerifier{err: err}, RequirePermission("donor_profiles", domain.Read), true)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s token got %d, want 401", name, rec.Code)
		}
	}
}

// The `sub` claim is the caller's identity. If it is not an integer user id,
// the request must not proceed as *somebody* — it proceeds as nobody.
func TestUnparseableSubjectDoesNotAuthenticate(t *testing.T) {
	c := claimsFor(domain.RoleAdmin, 1, nil, nil)
	c.Subject = "not-a-number"
	rec := serve(fakeVerifier{claims: c}, RequirePermission("donor_profiles", domain.Read), true)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want 401 for a non-numeric sub", rec.Code)
	}
}

// A role that is not in the matrix — a forged claim, or a role deleted after the
// token was issued — must be denied everywhere rather than treated as a new role
// with no rules yet.
func TestUnknownRoleIsDeniedEverywhere(t *testing.T) {
	v := fakeVerifier{claims: claimsFor(domain.Role("superuser"), 1, nil, nil)}
	for _, res := range domain.Resources() {
		for _, a := range []domain.Action{domain.Create, domain.Read, domain.Update, domain.Delete, domain.Execute} {
			if rec := serve(v, RequirePermission(res, a), true); rec.Code != http.StatusForbidden {
				t.Fatalf("role=superuser %s %s got %d, want 403", a, res, rec.Code)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// The §7.6 matrix, over real HTTP
// ---------------------------------------------------------------------------

// Every cell of the matrix, asserted positively and negatively through the
// actual middleware — not just through domain.Can. This is the acceptance
// criterion for WI-20: a granted cell reaches the handler with its scope
// attached, and a denied cell returns 403 without reaching it.
func TestEveryMatrixCellIsEnforcedOverHTTP(t *testing.T) {
	actions := []domain.Action{domain.Create, domain.Read, domain.Update, domain.Delete, domain.Execute}
	allowed, denied := 0, 0

	for _, res := range domain.Resources() {
		for _, role := range domain.AllRoles() {
			for _, a := range actions {
				wantScope, wantAllowed := domain.Can(role, res, a)
				v := fakeVerifier{claims: claimsFor(role, 7, i64(3), i64(9))}
				rec := serve(v, RequirePermission(res, a), true)

				switch {
				case wantAllowed && rec.Code != http.StatusOK:
					t.Errorf("%s %s %s: got %d, want 200 (granted by the matrix)", role, a, res, rec.Code)
				case !wantAllowed && rec.Code != http.StatusForbidden:
					t.Errorf("%s %s %s: got %d, want 403 (denied by the matrix)", role, a, res, rec.Code)
				}
				if !wantAllowed {
					denied++
					continue
				}
				allowed++
				// The scope is a mandatory WHERE clause downstream. If it does not
				// arrive, the handler silently reads everything.
				if got := rec.Body.String(); got != "scope="+string(wantScope) {
					t.Errorf("%s %s %s: handler saw %q, want scope=%q", role, a, res, got, wantScope)
				}
			}
		}
	}
	t.Logf("enforced %d granted and %d denied cells over HTTP", allowed, denied)
	if allowed == 0 || denied == 0 {
		t.Fatal("the matrix is not being exercised in both directions")
	}
}

// ---------------------------------------------------------------------------
// Named transitions (§7.6's X-approve vs X-cancel)
// ---------------------------------------------------------------------------

// The rule the whole application exists to enforce: a donor may cancel their own
// donation request but may not approve it. Both are `X own` in the matrix, so a
// bare Execute check passes for each — which is why RequireTransition exists.
func TestDonorMayCancelButNotApproveTheirOwnRequest(t *testing.T) {
	donor := fakeVerifier{claims: claimsFor(domain.RoleDonor, 7, nil, nil)}

	if rec := serve(donor, RequirePermission("donation_requests", domain.Execute), true); rec.Code != http.StatusOK {
		t.Fatalf("precondition: a donor does hold a bare X on donation_requests, got %d", rec.Code)
	}
	if rec := serve(donor, RequireTransition("donation_requests", "cancel"), true); rec.Code != http.StatusOK {
		t.Errorf("donor cancelling own request got %d, want 200", rec.Code)
	}
	if rec := serve(donor, RequireTransition("donation_requests", "approve"), true); rec.Code != http.StatusForbidden {
		t.Errorf("donor approving own request got %d, want 403 — this is the review step", rec.Code)
	}
}

func TestTransitionsFollowTheMatrix(t *testing.T) {
	cases := []struct {
		role       domain.Role
		resource   string
		transition string
		want       int
	}{
		{domain.RoleStaff, "donation_requests", "approve", http.StatusOK},
		{domain.RoleStaff, "donation_requests", "reject", http.StatusOK},
		{domain.RoleAdmin, "donation_requests", "approve", http.StatusOK},
		{domain.RoleLabTech, "donation_requests", "approve", http.StatusForbidden}, // no X at all
		{domain.RoleDonor, "appointments", "cancel", http.StatusOK},
		{domain.RoleDonor, "appointments", "reschedule", http.StatusOK},
		{domain.RoleLabTech, "blood_units", "quarantine", http.StatusOK},
		{domain.RoleLabTech, "blood_units", "move", http.StatusForbidden}, // moving stock is inventory's
		{domain.RoleInventoryManager, "blood_units", "move", http.StatusOK},
		{domain.RoleHospitalUser, "blood_requests", "cancel", http.StatusOK},
		{domain.RoleHospitalUser, "blood_requests", "approve", http.StatusForbidden}, // no self-approval
		{domain.RoleInventoryManager, "blood_requests", "approve", http.StatusOK},
		{domain.RoleAdmin, "deferrals", "lift", http.StatusOK},
		{domain.RoleStaff, "deferrals", "lift", http.StatusForbidden},
		// A transition nobody has declared is denied, not assumed harmless.
		{domain.RoleAdmin, "appointments", "check_in", http.StatusForbidden},
		{domain.RoleAdmin, "donation_requests", "vaporise", http.StatusForbidden},
	}
	for _, c := range cases {
		v := fakeVerifier{claims: claimsFor(c.role, 7, i64(3), i64(9))}
		rec := serve(v, RequireTransition(c.resource, c.transition), true)
		if rec.Code != c.want {
			t.Errorf("%s %s on %s: got %d, want %d", c.role, c.transition, c.resource, rec.Code, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// §7.7 ownership
// ---------------------------------------------------------------------------

func TestPermitsEvaluatesScopeAgainstTheRow(t *testing.T) {
	ctxFor := func(role domain.Role, uid int64, center, hospital *int64, scope domain.Scope) context.Context {
		ctx := context.WithValue(context.Background(), identityKey{}, Identity{
			UserID: uid, Role: role, CenterID: center, HospitalID: hospital,
		})
		return context.WithValue(ctx, scopeKey{}, scope)
	}

	cases := []struct {
		name string
		ctx  context.Context
		row  Row
		want bool
	}{
		{"own: the caller's row", ctxFor(domain.RoleDonor, 7, nil, nil, domain.ScopeOwn), Row{OwnerID: 7}, true},
		{"own: somebody else's row", ctxFor(domain.RoleDonor, 7, nil, nil, domain.ScopeOwn), Row{OwnerID: 8}, false},
		{"all: any row", ctxFor(domain.RoleAdmin, 1, nil, nil, domain.ScopeAll), Row{OwnerID: 999}, true},
		{"ctr: same center", ctxFor(domain.RoleStaff, 2, i64(3), nil, domain.ScopeCenter), Row{OwnerID: 8, CenterID: i64(3)}, true},
		{"ctr: another center", ctxFor(domain.RoleStaff, 2, i64(3), nil, domain.ScopeCenter), Row{OwnerID: 8, CenterID: i64(4)}, false},
		{"ctr: caller has no center", ctxFor(domain.RoleStaff, 2, nil, nil, domain.ScopeCenter), Row{OwnerID: 8, CenterID: i64(3)}, false},
		{"ctr: row has no center", ctxFor(domain.RoleStaff, 2, i64(3), nil, domain.ScopeCenter), Row{OwnerID: 8}, false},
		{"hosp: same hospital", ctxFor(domain.RoleHospitalUser, 5, nil, i64(9), domain.ScopeHospital), Row{OwnerID: 8, HospitalID: i64(9)}, true},
		{"hosp: another hospital", ctxFor(domain.RoleHospitalUser, 5, nil, i64(9), domain.ScopeHospital), Row{OwnerID: 8, HospitalID: i64(2)}, false},
		{"agg: row detail is never permitted", ctxFor(domain.RoleHospitalUser, 5, nil, i64(9), domain.ScopeAggregate), Row{OwnerID: 5, HospitalID: i64(9)}, false},
		{"anonymous", context.Background(), Row{OwnerID: 7}, false},
	}
	for _, c := range cases {
		if got := Permits(c.ctx, c.row); got != c.want {
			t.Errorf("%s: Permits = %v, want %v", c.name, got, c.want)
		}
	}
}

// Regression guard for A14/WI-02. A donor asking for another donor's row must
// get the same answer as for a row that does not exist. A 403 here would confirm
// the record exists, which is itself the leak.
func TestDenyLooksLikeNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	Deny(rec)
	if rec.Code != http.StatusNotFound {
		t.Errorf("Deny wrote %d, want 404 — a 403 confirms the row exists", rec.Code)
	}
}

// A scope value never granted by the matrix must deny. Without this, a typo in a
// future scope constant fails open.
func TestUnrecognisedScopeDenies(t *testing.T) {
	ctx := context.WithValue(context.Background(), identityKey{}, Identity{UserID: 7})
	ctx = context.WithValue(ctx, scopeKey{}, domain.Scope("whatever"))
	if Permits(ctx, Row{OwnerID: 7}) {
		t.Error("an unrecognised scope must deny, not fall through to allow")
	}
}
