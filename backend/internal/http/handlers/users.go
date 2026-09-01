package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"bbank/internal/domain"
	"bbank/internal/http/response"
	"bbank/internal/middleware"
	"bbank/internal/service"
	"bbank/internal/store"

	"github.com/go-chi/chi/v5"
)

// UserHandler serves /api/v1/users — invite, list, suspend, reactivate,
// change role (FR-66, TRD §6.5).
//
// Gated by the §7.6 matrix rather than by RequireRole: `users` IS a resource in
// that matrix, unlike the operational feature flags, so going through it keeps
// one place to read who may administer accounts.
//
// Administration is admin-only, but the resource is not: a donor holds `R`
// scoped `own`, so they may read their own account and nothing else. Every
// read here therefore consults the granted SCOPE as well as the permission —
// the two are different questions, and answering only the first turned this
// into a user directory once already.
type UserHandler struct {
	svc  *service.UserService
	idem middleware.IdempotencyStore
}

func NewUserHandler(svc *service.UserService, idem middleware.IdempotencyStore) *UserHandler {
	return &UserHandler{svc: svc, idem: idem}
}

func (h *UserHandler) Routes() chi.Router {
	r := chi.NewRouter()
	replay := middleware.Idempotency(h.idem, false)

	r.With(middleware.RequirePermission("users", domain.Read)).Get("/", h.list)
	r.With(middleware.RequirePermission("users", domain.Read)).Get("/{id}", h.get)
	r.With(middleware.RequirePermission("users", domain.Create), replay).Post("/", h.invite)
	r.With(middleware.RequirePermission("users", domain.Update), replay).Patch("/{id}", h.patch)
	return r
}

// PublicRoutes carries the one endpoint an invitee can reach without a session.
//
// Separate from Routes rather than an exception inside it: an authenticated
// router with a carve-out is how the next endpoint quietly becomes public too.
func (h *UserHandler) PublicRoutes() chi.Router {
	r := chi.NewRouter()
	r.Post("/accept", h.acceptInvite)
	return r
}

type userDTO struct {
	ID            int64   `json:"id"`
	Email         string  `json:"email"`
	Role          string  `json:"role"`
	Status        string  `json:"status"`
	CenterID      *int64  `json:"center_id,omitempty"`
	HospitalID    *int64  `json:"hospital_id,omitempty"`
	LastLoginAt   *string `json:"last_login_at,omitempty"`
	CreatedAt     string  `json:"created_at"`
	InvitePending bool    `json:"invite_pending"`
}

// toUserDTO is shared by the single read and the own-scope list, so the two
// cannot drift in what they expose.
func toUserDTO(u store.GetUserRow) userDTO {
	return userDTO{
		ID: u.ID, Email: u.Email, Role: string(u.Role), Status: string(u.Status),
		CenterID: u.CenterID, HospitalID: u.HospitalID,
		LastLoginAt: tsPtr(u.LastLoginAt), CreatedAt: tsStr(u.CreatedAt),
	}
}

// list is scoped, not merely permissioned.
//
// A donor holds `R` on `users` with scope `own` (§7.6) so they can read their
// own account — and requiring the permission without consulting the SCOPE
// turned that grant into a directory of every account's email, role, status and
// last login. Holding a permission and being allowed to see every row are
// different questions, and the second one is answered here.
func (h *UserHandler) list(w http.ResponseWriter, r *http.Request) {
	paging, ok := response.ParsePaging(w, r)
	if !ok {
		return
	}

	// Anything narrower than "all" sees only itself. Written as a default-deny
	// switch rather than `if scope == own`, so a scope added later (hospital,
	// centre) is refused until somebody decides what it should mean, instead of
	// silently inheriting the unfiltered list.
	switch middleware.ScopeFrom(r.Context()) {
	case domain.ScopeAll:
		// Full listing: admin.
	case domain.ScopeOwn:
		if id, ok := middleware.IdentityFrom(r.Context()); ok {
			self, err := h.svc.Get(r.Context(), id.UserID)
			if err != nil {
				writeServiceError(w, r, err)
				return
			}
			response.Paged(w, []userDTO{toUserDTO(self)}, 1, paging.Limit, paging.Offset)
			return
		}
		response.Paged(w, []userDTO{}, 0, paging.Limit, paging.Offset)
		return
	default:
		response.Paged(w, []userDTO{}, 0, paging.Limit, paging.Offset)
		return
	}

	p := service.ListUserParams{Limit: paging.Limit, Offset: paging.Offset}
	if v := r.URL.Query().Get("role"); v != "" {
		p.Role = &v
	}
	if v := r.URL.Query().Get("status"); v != "" {
		p.Status = &v
	}
	if v := r.URL.Query().Get("q"); v != "" {
		p.Search = &v
	}

	rows, total, err := h.svc.List(r.Context(), p)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	out := make([]userDTO, 0, len(rows))
	for _, u := range rows {
		out = append(out, userDTO{
			ID: u.ID, Email: u.Email, Role: string(u.Role), Status: string(u.Status),
			CenterID: u.CenterID, HospitalID: u.HospitalID,
			LastLoginAt: tsPtr(u.LastLoginAt), CreatedAt: tsStr(u.CreatedAt),
			InvitePending: u.InvitePending,
		})
	}
	response.Paged(w, out, total, paging.Limit, paging.Offset)
}

func (h *UserHandler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	// §7.7: the id in the URL is not identity. An own-scoped caller asking for
	// somebody else gets 404, not 403 — a 403 confirms the account exists.
	if !resolveOwned(w, r, id) {
		return
	}
	u, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	response.OK(w, toUserDTO(u))
}

type inviteRequest struct {
	Email      string `json:"email"`
	Role       string `json:"role"`
	CenterID   *int64 `json:"center_id,omitempty"`
	HospitalID *int64 `json:"hospital_id,omitempty"`
}

func (h *UserHandler) invite(w http.ResponseWriter, r *http.Request) {
	var in inviteRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		response.BadRequest(w, r, "invalid JSON body")
		return
	}
	actor, _ := middleware.IdentityFrom(r.Context())
	actorID := actor.UserID

	userID, token, err := h.svc.Invite(r.Context(), service.InviteParams{
		Email: in.Email, Role: in.Role, CenterID: in.CenterID, HospitalID: in.HospitalID,
		InvitedBy: &actorID,
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	// Who invited whom, at Info: this is an account being created, and an
	// auditor asking "where did this staff login come from?" needs the answer.
	// The TOKEN is deliberately absent from the log — it is a credential.
	slog.InfoContext(r.Context(), "user invited",
		slog.Int64("user_id", userID), slog.String("role", in.Role),
		slog.Int64("invited_by", actorID))

	w.Header().Set("Location", "/api/v1/users/"+strconv.FormatInt(userID, 10))
	// Returned exactly once. There is no endpoint that can show it again,
	// because only its hash is stored (WI-79 will email it instead).
	response.Created(w, map[string]any{
		"id":              userID,
		"invite_token":    token,
		"expires_in_days": int(service.InviteTTL.Hours() / 24),
	})
}

type patchUserRequest struct {
	// Pointers so "absent" and "empty" differ: a PATCH that mentions only the
	// status must not also blank the role.
	Status     *string `json:"status,omitempty"`
	Role       *string `json:"role,omitempty"`
	CenterID   *int64  `json:"center_id,omitempty"`
	HospitalID *int64  `json:"hospital_id,omitempty"`
}

func (h *UserHandler) patch(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var in patchUserRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		response.BadRequest(w, r, "invalid JSON body")
		return
	}
	if in.Status == nil && in.Role == nil {
		response.Unprocessable(w, r, "nothing to change",
			response.Detail{Field: "status|role", Issue: "one is required"})
		return
	}

	actor, _ := middleware.IdentityFrom(r.Context())

	if in.Role != nil {
		if err := h.svc.SetRole(r.Context(), id, *in.Role, in.CenterID, in.HospitalID, actor.UserID); err != nil {
			writeServiceError(w, r, err)
			return
		}
		slog.WarnContext(r.Context(), "user role changed",
			slog.Int64("user_id", id), slog.String("role", *in.Role),
			slog.Int64("actor_user_id", actor.UserID))
	}
	if in.Status != nil {
		if err := h.svc.SetStatus(r.Context(), id, *in.Status, actor.UserID); err != nil {
			writeServiceError(w, r, err)
			return
		}
		slog.WarnContext(r.Context(), "user status changed",
			slog.Int64("user_id", id), slog.String("status", *in.Status),
			slog.Int64("actor_user_id", actor.UserID))
	}

	u, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	response.OK(w, userDTO{
		ID: u.ID, Email: u.Email, Role: string(u.Role), Status: string(u.Status),
		CenterID: u.CenterID, HospitalID: u.HospitalID, CreatedAt: tsStr(u.CreatedAt),
	})
}

type acceptInviteRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

func (h *UserHandler) acceptInvite(w http.ResponseWriter, r *http.Request) {
	var in acceptInviteRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		response.BadRequest(w, r, "invalid JSON body")
		return
	}
	if in.Token == "" || in.Password == "" {
		response.Unprocessable(w, r, "a token and a password are required")
		return
	}

	err := h.svc.AcceptInvite(r.Context(), in.Token, in.Password)
	if errors.Is(err, service.ErrInviteInvalid) {
		// One answer for unknown, expired and already-used. Distinguishing them
		// would let someone probe for live tokens.
		response.Fail(w, r, http.StatusBadRequest, "invite_invalid",
			"This invitation link is not valid. Ask an administrator for a new one.")
		return
	}
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	response.NoContent(w)
}
