// Package http wires routes to handlers. Nothing in this project imports it
// except cmd/api — see the dependency rule in TRD §4.2.
package http

import (
	"context"
	"net/http"
	"time"

	"bbank/internal/http/handlers"
	"bbank/internal/middleware"
	"bbank/internal/platform"
	"bbank/internal/service"
	"bbank/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Deps struct {
	Cfg    platform.Config
	Pool   *pgxpool.Pool
	Signer *platform.Signer
	Flags  *platform.Flags
}

// NewRouter builds the full HTTP surface.
//
// **/api/v1 is the only implementation** (WI-21, TRD §6.1). The deprecated
// /api/go/ prefix is not a second set of routes — LegacyShim rewrites those
// paths before chi sees them, so the old spelling reaches the same handler and
// the two cannot drift. `go` named an implementation language, which is a poor
// thing for a public contract to promise.
//
// The strangler is finished (WI-22): internal/legacy is deleted and every
// endpoint runs through handlers -> service -> store.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	q := store.New(d.Pool)
	authSvc := service.NewAuthService(q, d.Signer)

	// chi requires every middleware to be registered before any route, so the
	// whole stack is declared here in one place.
	r.Use(middleware.RequestID)
	r.Use(middleware.AccessLog)
	r.Use(middleware.CORS(d.Cfg.AllowedOrigins))
	r.Use(middleware.JSONContentType)
	// Attaches identity when a valid token is present. Endpoints decide whether
	// they require it; this only verifies and populates.
	r.Use(middleware.Authenticate(authSvc))
	// Rewrites /api/go/* to /api/v1/* before routing. After Authenticate, so a
	// rewritten request is authorized identically to a canonical one — the alias
	// changes the spelling of the path and nothing else.
	r.Use(middleware.LegacyShim(d.Flags))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	r.Get("/readyz", func(w http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
		defer cancel()
		if err := d.Pool.Ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"database unreachable"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})

	// Auth lands on the canonical /api/v1 prefix from the start — it is new, so
	// there is no legacy client to keep compatible (WI-21 moves the rest).
	r.Mount("/api/v1/auth", handlers.NewAuthHandler(authSvc, d.Cfg.CookieSecure).Routes())

	// The frontend needs the PUBLIC key to verify tokens in proxy.ts. Serving it
	// here means no key material is copied between deployments by hand.
	r.Get("/api/v1/auth/public-key", func(w http.ResponseWriter, _ *http.Request) {
		pem, err := d.Signer.PublicKeyPEM()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Header().Set("Cache-Control", "public, max-age=300")
		_, _ = w.Write([]byte(pem))
	})

	idem := service.NewIdempotencyService(q)

	// WI-22: donation requests and appointments are served by the layered
	// handlers. internal/legacy is gone — the strangler finished.
	donors := handlers.NewDonorHandler(service.NewDonorService(q))
	r.Mount("/api/v1/donors", donors.Routes())
	// Self-registration is the one donor endpoint with no session (TRD §6.5).
	// Mounted before the authenticated router so chi matches the bare POST here.
	r.Mount("/api/v1/register", donors.PublicRoutes(idem))

	r.Mount("/api/v1/donation-requests",
		handlers.NewDonationRequestHandler(service.NewDonationRequestService(d.Pool, q), idem).Routes())
	r.Mount("/api/v1/appointments",
		handlers.NewAppointmentHandler(service.NewAppointmentService(q)).Routes())

	// User administration (WI-18). `users` is a resource in the §7.6 matrix, so
	// it is gated there rather than by RequireRole.
	users := handlers.NewUserHandler(service.NewUserService(d.Pool, q, authSvc), idem)
	r.Mount("/api/v1/users", users.Routes())
	// Accepting an invitation is necessarily anonymous: the invitee has no
	// session, which is the whole point of the invitation.
	r.Mount("/api/v1/invites", users.PublicRoutes())

	// Operational, not clinical: gated on the admin role directly rather than on
	// the §7.6 matrix, which owns domain resources and denies unknown ones.
	r.Mount("/api/v1/admin/flags", handlers.NewFlagHandler(d.Flags).Routes())

	return r
}
