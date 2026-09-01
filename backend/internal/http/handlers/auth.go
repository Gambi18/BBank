package handlers

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"

	"bbank/internal/http/response"
	"bbank/internal/platform"
	"bbank/internal/service"

	"github.com/go-chi/chi/v5"
)

const (
	accessCookie  = "bb_at"
	refreshCookie = "bb_rt"
	// Narrow path: the refresh cookie is never sent on ordinary navigation,
	// only to the endpoint that consumes it (TRD §7.3).
	refreshPath = "/api/v1/auth/refresh"
)

type AuthHandler struct {
	svc    *service.AuthService
	secure bool
}

func NewAuthHandler(svc *service.AuthService, secure bool) *AuthHandler {
	return &AuthHandler{svc: svc, secure: secure}
}

func (h *AuthHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/login", h.login)
	r.Post("/refresh", h.refresh)
	r.Post("/logout", h.logout)
	return r
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type sessionResponse struct {
	UserID    int64  `json:"user_id"`
	Role      string `json:"role"`
	ExpiresAt string `json:"expires_at"`
}

func (h *AuthHandler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		response.BadRequest(w, r, "invalid JSON body")
		return
	}
	if req.Email == "" || req.Password == "" {
		response.BadRequest(w, r, "email and password are required")
		return
	}

	pair, err := h.svc.Login(r.Context(), service.LoginInput{
		Email: req.Email, Password: req.Password,
		IP: clientIP(r), UserAgent: r.UserAgent(),
	})
	switch {
	case errors.Is(err, service.ErrInvalidCredentials):
		// Never reveal WHICH factor was wrong (NFR-12).
		response.Fail(w, r, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
		return
	case errors.Is(err, service.ErrAccountLocked):
		response.Fail(w, r, http.StatusLocked, "account_locked", "this account is temporarily locked")
		return
	case errors.Is(err, service.ErrAccountInactive):
		response.Fail(w, r, http.StatusForbidden, "account_inactive", "this account is not active")
		return
	case err != nil:
		response.Internal(w, r, err)
		return
	}

	h.setCookies(w, pair)
	response.OK(w, sessionResponse{
		UserID: pair.UserID, Role: pair.Role,
		ExpiresAt: pair.AccessExpiresAt.Format(time.RFC3339),
	})
}

func (h *AuthHandler) refresh(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(refreshCookie)
	if err != nil || c.Value == "" {
		response.Fail(w, r, http.StatusUnauthorized, "no_refresh_token", "no refresh token presented")
		return
	}

	pair, err := h.svc.Refresh(r.Context(), c.Value, clientIP(r), r.UserAgent())
	switch {
	case errors.Is(err, service.ErrRefreshReused):
		// The family is already revoked by the service. Clear the cookies so the
		// legitimate user is forced to log in again rather than looping.
		h.clearCookies(w)
		response.Fail(w, r, http.StatusUnauthorized, "refresh_reused",
			"this session has been ended for your security; please sign in again")
		return
	case errors.Is(err, service.ErrRefreshInvalid), errors.Is(err, service.ErrAccountInactive):
		h.clearCookies(w)
		response.Fail(w, r, http.StatusUnauthorized, "refresh_invalid", "please sign in again")
		return
	case err != nil:
		response.Internal(w, r, err)
		return
	}

	h.setCookies(w, pair)
	response.OK(w, sessionResponse{
		UserID: pair.UserID, Role: pair.Role,
		ExpiresAt: pair.AccessExpiresAt.Format(time.RFC3339),
	})
}

func (h *AuthHandler) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(refreshCookie); err == nil && c.Value != "" {
		if err := h.svc.Logout(r.Context(), c.Value); err != nil {
			response.Internal(w, r, err)
			return
		}
	}
	h.clearCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) setCookies(w http.ResponseWriter, p *service.TokenPair) {
	http.SetCookie(w, &http.Cookie{
		Name: accessCookie, Value: p.AccessToken,
		Path: "/", HttpOnly: true, Secure: h.secure,
		SameSite: http.SameSiteLaxMode, // Lax: top-level navigation must carry it
		Expires:  p.AccessExpiresAt, MaxAge: int(platform.AccessTokenTTL.Seconds()),
	})
	http.SetCookie(w, &http.Cookie{
		Name: refreshCookie, Value: p.RefreshToken,
		Path: refreshPath, HttpOnly: true, Secure: h.secure,
		SameSite: http.SameSiteStrictMode, // Strict: only ever sent to /refresh
		Expires:  p.RefreshExpiresAt, MaxAge: int(platform.RefreshTokenTTL.Seconds()),
	})
}

func (h *AuthHandler) clearCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: accessCookie, Value: "", Path: "/", HttpOnly: true, Secure: h.secure, MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: refreshCookie, Value: "", Path: refreshPath, HttpOnly: true, Secure: h.secure, MaxAge: -1})
}

func clientIP(r *http.Request) string {
	// X-Forwarded-For is only trustworthy behind a proxy that sets it; taking the
	// first entry is right there and harmless (it is used for audit, never authz).
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := len(xff); i > 0 {
			for j := 0; j < len(xff); j++ {
				if xff[j] == ',' {
					return trimSpace(xff[:j])
				}
			}
			return trimSpace(xff)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func trimSpace(s string) string {
	for len(s) > 0 && s[0] == ' ' {
		s = s[1:]
	}
	for len(s) > 0 && s[len(s)-1] == ' ' {
		s = s[:len(s)-1]
	}
	return s
}
