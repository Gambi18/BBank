package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"bbank/internal/domain"
	"bbank/internal/platform"
)

// Identity is what the rest of the request pipeline knows about the caller.
// It is derived ONLY from a verified token — never from a query parameter, a
// header, or a body field. That distinction is the whole point: the original
// getAppointment bug treated a query parameter as identity (A14/WI-02).
type Identity struct {
	UserID     int64
	SessionID  string
	Role       domain.Role
	CenterID   *int64
	HospitalID *int64
}

type identityKey struct{}

func IdentityFrom(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey{}).(Identity)
	return id, ok
}

// TokenVerifier is implemented by service.AuthService. Declared here as an
// interface so middleware does not import service (dependency rule, TRD §4.2).
type TokenVerifier interface {
	VerifyAccessToken(ctx context.Context, token string) (*platform.Claims, error)
}

const accessCookieName = "bb_at"

// Authenticate verifies the access token and attaches the Identity. A request
// without a valid token continues as anonymous; RequireRole decides whether that
// is acceptable. Splitting it this way keeps public endpoints simple.
func Authenticate(v TokenVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := bearerOrCookie(r)
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}

			claims, err := v.VerifyAccessToken(r.Context(), token)
			if err != nil {
				// A token that fails verification is not "anonymous" — it is a
				// signal worth logging. Distinguish the version mismatch, which
				// means credentials or role changed under an active session.
				switch {
				case errors.Is(err, platform.ErrTokenVerMismatch):
					slog.WarnContext(r.Context(), "access token rejected: stale token_version")
				case errors.Is(err, platform.ErrTokenExpired):
					// Ordinary; the client should refresh.
				default:
					slog.WarnContext(r.Context(), "access token rejected", "error", err)
				}
				next.ServeHTTP(w, r)
				return
			}

			uid, err := strconv.ParseInt(claims.Subject, 10, 64)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			id := Identity{
				UserID:     uid,
				SessionID:  claims.SessionID,
				Role:       domain.Role(claims.Role),
				CenterID:   claims.CenterID,
				HospitalID: claims.HospitalID,
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, id)))
		})
	}
}

// RequireAuth rejects anonymous requests.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := IdentityFrom(r.Context()); !ok {
			unauthorized(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequirePermission enforces one cell of the TRD §7.6 matrix. The granted Scope
// is placed on the context so the service layer can apply the mandatory WHERE
// clause — a scope is a requirement, not a hint.
func RequirePermission(resource string, action domain.Action) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, ok := IdentityFrom(r.Context())
			if !ok {
				unauthorized(w)
				return
			}
			scope, allowed := domain.Can(id.Role, resource, action)
			if !allowed {
				slog.WarnContext(r.Context(), "permission denied",
					"user_id", id.UserID, "role", id.Role, "resource", resource, "action", action)
				forbidden(w)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), scopeKey{}, scope)))
		})
	}
}

type scopeKey struct{}

// ScopeFrom returns the scope granted by RequirePermission.
func ScopeFrom(ctx context.Context) domain.Scope {
	s, _ := ctx.Value(scopeKey{}).(domain.Scope)
	return s
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":{"code":"unauthenticated","message":"authentication required"}}`))
}

func forbidden(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`{"error":{"code":"forbidden","message":"you do not have permission to do that"}}`))
}

func bearerOrCookie(r *http.Request) string {
	if h := r.Header.Get("Authorization"); len(h) > 7 && (h[:7] == "Bearer " || h[:7] == "bearer ") {
		return h[7:]
	}
	if c, err := r.Cookie(accessCookieName); err == nil {
		return c.Value
	}
	return ""
}

// RequireTransition enforces a *named* state transition (TRD §7.6's `X-approve`,
// `X-cancel`, …) rather than a bare X. Use it on routes where the URL itself
// names the transition, e.g. POST /requests/{id}/confirm.
//
// Without this, a donor holding `X own` on donation_requests could approve their
// own request — which is the one thing the review flow exists to prevent.
func RequireTransition(resource, transition string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, ok := IdentityFrom(r.Context())
			if !ok {
				unauthorized(w)
				return
			}
			scope, allowed := domain.CanExecute(id.Role, resource, transition)
			if !allowed {
				slog.WarnContext(r.Context(), "transition denied",
					"user_id", id.UserID, "role", id.Role,
					"resource", resource, "transition", transition)
				forbidden(w)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), scopeKey{}, scope)))
		})
	}
}

// Row is the set of owning columns a scope is evaluated against. A nil pointer
// means the row carries no such column.
type Row struct {
	OwnerID    int64 // the user the row belongs to: donor_id, user_id, …
	CenterID   *int64
	HospitalID *int64
}

// Permits applies TRD §7.7 to one row: does the scope granted to this caller
// actually include it?
//
// Ownership is read from the verified Identity, never from the request. A false
// return must be answered with 404, not 403 — a 403 confirms the row exists.
func Permits(ctx context.Context, row Row) bool {
	id, ok := IdentityFrom(ctx)
	if !ok {
		return false
	}
	switch ScopeFrom(ctx) {
	case domain.ScopeAll:
		return true
	case domain.ScopeOwn:
		return id.UserID == row.OwnerID
	case domain.ScopeCenter:
		return id.CenterID != nil && row.CenterID != nil && *id.CenterID == *row.CenterID
	case domain.ScopeHospital:
		return id.HospitalID != nil && row.HospitalID != nil && *id.HospitalID == *row.HospitalID
	case domain.ScopeAggregate:
		return false // aggregate figures only; row-level detail is never permitted
	}
	return false // an unrecognised scope denies
}

// Deny writes the 404 that a scope violation must produce. Kept next to Permits
// so the two are never used apart.
func Deny(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(`{"error":{"code":"not_found","message":"not found"}}`))
}

// RequireRole gates a route on the caller's role directly, without consulting
// the §7.6 matrix.
//
// Reserved for OPERATIONAL endpoints — the feature-flag console, and anything
// else that administers the process rather than the domain. The matrix owns
// clinical and personal resources, and it denies any resource it does not know
// (WI-20), so inventing a "flags" resource there would both redefine identifiers
// the TRD owns and deny everyone including admin. Domain resources must keep
// going through RequirePermission; reaching for this to skip a matrix cell would
// be a way to quietly opt out of authorization.
func RequireRole(roles ...domain.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, ok := IdentityFrom(r.Context())
			if !ok {
				unauthorized(w)
				return
			}
			for _, want := range roles {
				if id.Role == want {
					next.ServeHTTP(w, r)
					return
				}
			}
			forbidden(w)
		})
	}
}
