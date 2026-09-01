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

	r.Use(middleware.RequestID)
	r.Use(middleware.AccessLog)
	r.Use(middleware.CORS(d.Cfg.AllowedOrigins))
	r.Use(middleware.JSONContentType)

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

	donors := handlers.NewDonorHandler(service.NewDonorService(store.New(d.Pool)))
	r.Mount("/api/go/donors", donors.Routes())

	legacy.RegisterRoutes(r, d.LegacyD)

	return r
}
