// Package middleware holds HTTP middleware: correlation IDs, access logging,
// CORS, and (from WI-20) authentication and authorization.
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type ctxKey string

const requestIDKey ctxKey = "request_id"

// RequestIDFrom returns the correlation ID assigned by RequestID, if any.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// requestIDMiddleware assigns every request a correlation ID, honouring an
// inbound X-Request-Id when the caller supplies one. (WI-10)
// RequestID assigns every request a correlation ID, honouring an inbound
// X-Request-Id when supplied (WI-10).
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get("X-Request-Id"))
		if id == "" || len(id) > 64 {
			b := make([]byte, 16)
			if _, err := rand.Read(b); err != nil {
				id = strconv.FormatInt(time.Now().UnixNano(), 36)
			} else {
				id = hex.EncodeToString(b)
			}
		}
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

// statusRecorder captures the status code so the access log can report it.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

// loggingMiddleware emits one structured line per request. It deliberately logs
// the matched route template rather than the raw path, so IDs and query strings
// (which can carry personal data) never reach the log. (WI-10)
// AccessLog emits one structured line per request. It logs the matched route
// template rather than the raw path, so IDs and query strings never reach the log.
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Health probes would otherwise dominate the log.
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if rec.status == 0 {
			rec.status = http.StatusOK
		}

		route := r.URL.Path
		if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
			route = rc.RoutePattern()
		}

		id, _ := r.Context().Value(requestIDKey).(string)
		level := slog.LevelInfo
		if rec.status >= 500 {
			level = slog.LevelError
		} else if rec.status >= 400 {
			level = slog.LevelWarn
		}
		slog.LogAttrs(r.Context(), level, "http request",
			slog.String("request_id", id),
			slog.String("method", r.Method),
			slog.String("route", route),
			slog.Int("status", rec.status),
			slog.Int("bytes", rec.bytes),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		)
	})
}

// enableCORS returns a middleware that reflects the request Origin only when it
// appears in an explicit allowlist. (WI-03)
//
// The previous implementation sent "Access-Control-Allow-Origin: *", which let any
// site on the internet call this API from a browser. Note that "*" is also illegal
// alongside Allow-Credentials, so the old configuration could never have supported
// the cookie auth this system is moving to.
// CORS reflects the request Origin only when it appears in an explicit
// allowlist (WI-03). Never "*".
func CORS(allowed []string) func(http.Handler) http.Handler {
	allowSet := make(map[string]struct{}, len(allowed))
	for _, o := range allowed {
		allowSet[o] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Mandatory: without it a shared cache can serve one origin's CORS
			// headers to another.
			w.Header().Add("Vary", "Origin")

			origin := r.Header.Get("Origin")
			_, ok := allowSet[origin]
			if origin != "" && ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Idempotency-Key, X-Request-Id")
				w.Header().Set("Access-Control-Expose-Headers", "X-Request-Id, Deprecation, Sunset, Retry-After, Idempotent-Replay")
				w.Header().Set("Access-Control-Max-Age", "600")
			}

			if r.Method == http.MethodOptions {
				// A preflight from a disallowed origin gets no CORS headers, so the
				// browser blocks the real request regardless of this status.
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// JSONContentType sets the response content type.
func JSONContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set JSON Content-Type
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

// Auth Handler
