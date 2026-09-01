package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"bbank/internal/platform"
)

// The mapping table from TRD §6.1, asserted case by case. Written out rather
// than looped so a wrong destination names itself in the failure.
func TestRewriteLegacyPath(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"/api/go/donors", "/api/v1/donors", true},
		{"/api/go/donors/42", "/api/v1/donors/42", true},
		{"/api/go/appointments", "/api/v1/appointments", true},
		{"/api/go/appointments/7", "/api/v1/appointments/7", true},
		{"/api/go/login", "/api/v1/auth/login", true},

		// The two renames. `requests` -> `donation-requests` because a
		// blood-request is a different concept; `confirm` -> `approve` because
		// the v1 handler approves rather than deleting the row.
		{"/api/go/requests", "/api/v1/donation-requests", true},
		{"/api/go/requests/9", "/api/v1/donation-requests/9", true},
		{"/api/go/requests/9/confirm", "/api/v1/donation-requests/9/approve", true},

		// NOT a general prefix alias. Anything outside the table is absent, or
		// the deprecated prefix quietly acquires every future endpoint and never
		// dies.
		{"/api/go/blood-units", "", false},
		{"/api/go/donors/42/eligibility", "", false},
		{"/api/go/requests/9/reject", "", false},
		{"/api/go/login/extra", "", false},
		{"/api/go/", "", false},
		{"/api/v1/donors", "", false},
	}

	for _, c := range cases {
		got, ok := rewriteLegacyPath(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("rewriteLegacyPath(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestLegacyShimRewritesAndMarksDeprecated(t *testing.T) {
	var seen string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})
	h := LegacyShim(platform.NewFlags(true))(next)

	req := httptest.NewRequest(http.MethodGet, "/api/go/requests?limit=5", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if seen != "/api/v1/donation-requests" {
		t.Fatalf("handler saw path %q, want the rewritten v1 path", seen)
	}
	if got := rec.Header().Get("Deprecation"); got != "true" {
		t.Errorf("Deprecation header = %q, want \"true\"", got)
	}
	if got := rec.Header().Get("Sunset"); got != SunsetDate {
		t.Errorf("Sunset header = %q, want %q", got, SunsetDate)
	}
}

// The query string must survive the rewrite, or pagination silently resets to
// the default for every legacy caller.
func TestLegacyShimPreservesQueryString(t *testing.T) {
	var raw string
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { raw = r.URL.RawQuery })
	h := LegacyShim(platform.NewFlags(true))(next)

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/go/appointments?limit=5&offset=10", nil))
	if raw != "limit=5&offset=10" {
		t.Fatalf("query string = %q, want it preserved through the rewrite", raw)
	}
}

// The acceptance criterion: the flag turns the shim off and on, in a live
// process, with no restart.
func TestLegacyShimFlagTogglesAtRuntime(t *testing.T) {
	reached := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached++
		w.WriteHeader(http.StatusOK)
	})
	flags := platform.NewFlags(true)
	h := LegacyShim(flags)(next)

	call := func() int {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/go/requests", nil))
		return rec.Code
	}

	if code := call(); code != http.StatusOK || reached != 1 {
		t.Fatalf("shim on: status %d, handler reached %d times; want 200 and 1", code, reached)
	}

	flags.SetLegacyShim(false)
	// 410 Gone, not 404: retired by policy, not missing by accident. The
	// difference tells an integrator whether to fix their path or their calendar.
	if code := call(); code != http.StatusGone {
		t.Fatalf("shim off: status %d, want 410", code)
	}
	if reached != 1 {
		t.Fatalf("handler was reached %d times with the shim off; want it untouched", reached)
	}

	flags.SetLegacyShim(true)
	if code := call(); code != http.StatusOK || reached != 2 {
		t.Fatalf("shim back on: status %d, reached %d; want 200 and 2", code, reached)
	}
}

// Even a disabled endpoint should still say it is deprecated and when it dies —
// that is the information a caller polling it actually needs.
func TestLegacyShimDisabledStillCarriesSunset(t *testing.T) {
	h := LegacyShim(platform.NewFlags(false))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/go/requests", nil))

	if rec.Header().Get("Sunset") != SunsetDate {
		t.Errorf("a switched-off legacy path lost its Sunset header")
	}
}

// A canonical path must not be touched by the shim at all.
func TestLegacyShimIgnoresCanonicalPaths(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := LegacyShim(platform.NewFlags(false))(next)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/donors", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("canonical path got %d with the shim disabled; it must be unaffected", rec.Code)
	}
	if rec.Header().Get("Deprecation") != "" {
		t.Errorf("canonical path was marked deprecated")
	}
}
