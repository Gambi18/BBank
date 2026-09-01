package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"bbank/internal/domain"
	"bbank/internal/http/response"
	"bbank/internal/middleware"
	"bbank/internal/platform"

	"github.com/go-chi/chi/v5"
)

// FlagHandler exposes the runtime feature flags (WI-21).
//
// This exists to satisfy one specific acceptance criterion: the `/api/go/` shim
// must be switchable off and on **without a redeploy**. An environment variable
// cannot do that — changing one means restarting the process, which during an
// incident is exactly the moment you do not want to be waiting on a rollout.
//
// Admin only, and via RequireRole rather than the §7.6 matrix: a feature flag is
// not a clinical resource, and the matrix denies resources it does not know.
type FlagHandler struct {
	flags *platform.Flags
}

func NewFlagHandler(f *platform.Flags) *FlagHandler { return &FlagHandler{flags: f} }

func (h *FlagHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RequireRole(domain.RoleAdmin))
	r.Get("/", h.get)
	r.Patch("/", h.patch)
	return r
}

type flagsDTO struct {
	// Pointer so "absent" and "false" are different things. Without this, a
	// PATCH that mentions only one flag would silently switch the others off.
	LegacyShim *bool `json:"legacy_shim"`
}

func (h *FlagHandler) get(w http.ResponseWriter, _ *http.Request) {
	on := h.flags.LegacyShim()
	response.OK(w, flagsDTO{LegacyShim: &on})
}

func (h *FlagHandler) patch(w http.ResponseWriter, r *http.Request) {
	var in flagsDTO
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		response.BadRequest(w, r, "invalid JSON body")
		return
	}
	if in.LegacyShim == nil {
		response.Unprocessable(w, r, "no known flag was supplied",
			response.Detail{Field: "legacy_shim", Issue: "required"})
		return
	}

	h.flags.SetLegacyShim(*in.LegacyShim)

	// Logged at Warn, not Info: someone changing the shape of the public API
	// surface at runtime is something an operator reading the log after an
	// incident needs to find without looking for it.
	actor, _ := middleware.IdentityFrom(r.Context())
	slog.WarnContext(r.Context(), "feature flag changed",
		slog.String("flag", "legacy_shim"),
		slog.Bool("value", *in.LegacyShim),
		slog.Int64("actor_user_id", actor.UserID),
	)

	on := h.flags.LegacyShim()
	response.OK(w, flagsDTO{LegacyShim: &on})
}
