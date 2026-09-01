package middleware

import (
	"log/slog"
	"net/http"
	"strings"

	"bbank/internal/platform"
)

// The deprecated prefix and its sunset date (TRD §6.1).
const (
	LegacyPrefix = "/api/go/"
	V1Prefix     = "/api/v1"

	// RFC 8594: `Deprecation` marks the resource as deprecated, `Sunset` says
	// when it stops responding. A date, not a vague promise, because a
	// deprecation without a date is a permanent second API surface.
	SunsetDate = "Wed, 31 Mar 2027 00:00:00 GMT"
)

// LegacyShim serves the deprecated /api/go/ prefix by rewriting to /api/v1
// before routing, so there is exactly ONE implementation of each endpoint.
//
// Registering the legacy paths as a second set of routes was the obvious
// alternative and is a trap: two route sets drift, and the legacy copy is the
// one nobody re-reads when a rule changes. Here the old path is a spelling of
// the new one, and it cannot diverge because there is nothing to diverge from.
//
// It is deliberately NOT a blanket prefix swap. Only the endpoints listed in
// §6.1 are aliased; anything else under /api/go/ is a 404. A general alias would
// silently expose every future v1 endpoint under a deprecated name with no
// deprecation clock of its own — which is how a prefix scheduled for deletion
// becomes permanent.
func LegacyShim(flags *platform.Flags) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, LegacyPrefix) {
				next.ServeHTTP(w, r)
				return
			}

			target, ok := rewriteLegacyPath(r.URL.Path)
			if !ok {
				// Not one of the aliased endpoints. Answer as if it never
				// existed, because as far as the contract goes it never did.
				notFound(w)
				return
			}

			// The headers go on before the flag check, so a client polling a
			// disabled endpoint still learns it is deprecated and when it dies.
			w.Header().Set("Deprecation", "true")
			w.Header().Set("Sunset", SunsetDate)
			w.Header().Set("Link", `<`+target+`>; rel="successor-version"`)

			if !flags.LegacyShim() {
				// 410, not 404: the resource is gone by policy, not missing by
				// accident, and the difference tells an integrator whether to
				// fix their path or their calendar.
				gone(w, target)
				return
			}

			slog.DebugContext(r.Context(), "legacy path rewritten",
				slog.String("from", r.URL.Path), slog.String("to", target))

			// Rewrite before chi routes. Both fields are set: RequestURI is what
			// handlers and logs echo, Path is what the router matches, and
			// leaving them disagreeing produces bugs that are miserable to find.
			r2 := r.Clone(r.Context())
			r2.URL.Path = target
			r2.RequestURI = target
			if r.URL.RawQuery != "" {
				r2.RequestURI = target + "?" + r.URL.RawQuery
			}
			next.ServeHTTP(w, r2)
		})
	}
}

// rewriteLegacyPath maps one deprecated path to its canonical successor.
//
// The mapping is the table in TRD §6.1. Two of these are renames with a changed
// meaning downstream — `requests` became `donation-requests` because a
// `blood-request` is a hospital asking for units, a completely different thing,
// and `confirm` became `approve` because the v1 handler approves rather than
// deleting the row (WI-22 fixes A8). The legacy spelling keeps working; the
// destructive legacy *behaviour* deliberately does not come with it.
func rewriteLegacyPath(p string) (string, bool) {
	rest, ok := strings.CutPrefix(p, LegacyPrefix)
	if !ok || rest == "" {
		return "", false
	}
	segs := strings.Split(strings.Trim(rest, "/"), "/")

	switch segs[0] {
	case "login":
		if len(segs) != 1 {
			return "", false
		}
		return V1Prefix + "/auth/login", true

	case "donors", "appointments":
		// Same name under v1; only the prefix moves.
		if len(segs) > 2 {
			return "", false
		}
		return V1Prefix + "/" + strings.Join(segs, "/"), true

	case "requests":
		switch {
		case len(segs) == 1:
			return V1Prefix + "/donation-requests", true
		case len(segs) == 2:
			return V1Prefix + "/donation-requests/" + segs[1], true
		case len(segs) == 3 && segs[2] == "confirm":
			return V1Prefix + "/donation-requests/" + segs[1] + "/approve", true
		}
		return "", false
	}
	return "", false
}

// Written by hand rather than through the response package: middleware sits
// below it in the import order, and a cycle here would be paid for forever.
func gone(w http.ResponseWriter, successor string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusGone)
	_, _ = w.Write([]byte(`{"error":{"code":"endpoint_retired","message":"This deprecated path has been switched off. Use ` +
		successor + ` instead."}}`))
}

func notFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(`{"error":{"code":"not_found","message":"resource not found"}}`))
}
