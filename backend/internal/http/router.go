// Package http wires routes to handlers. Nothing in this project imports it
// except cmd/api — see the dependency rule in TRD §4.2.
package http

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"bbank/internal/http/handlers"
	"bbank/internal/legacy"
	"bbank/internal/middleware"
	"bbank/internal/platform"
	"bbank/internal/service"
	"bbank/internal/store"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Deps struct {
	Cfg     platform.Config
	Pool    *pgxpool.Pool // new layered path (pgx + sqlc)
	LegacyD *sql.DB       // strangler: resources not yet migrated
	Signer  *platform.Signer
}

// NewRouter builds the full HTTP surface.
//
// Two paths coexist deliberately during WI-11's strangler migration:
//   - /api/go/donors*  -> layered (handlers -> service -> store)
//   - everything else  -> internal/legacy, unchanged
//
// WI-21 introduces /api/v1 as canonical and demotes /api/go to a deprecated
// alias; until then the legacy prefix stays authoritative so nothing breaks.
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

	donors := handlers.NewDonorHandler(service.NewDonorService(q))
	r.Mount("/api/go/donors", donors.Routes())

	legacy.RegisterRoutes(r, d.LegacyD)

	return r
}
