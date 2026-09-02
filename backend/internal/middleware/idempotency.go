package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"log/slog"
	"net/http"

	"bbank/internal/platform"

	"github.com/go-chi/chi/v5"
)

// IdempotencyStore is implemented by service.IdempotencyService. Declared here
// as an interface so this middleware can be tested without a database, and so
// middleware does not import service.
type IdempotencyStore interface {
	Claim(ctx context.Context, actorID int64, key, endpoint string, fingerprint []byte) (rec platform.IdempotencyRecord, claimed bool, err error)
	Complete(ctx context.Context, actorID int64, key string, status int32, body []byte) error
	Release(ctx context.Context, actorID int64, key string) error
}

// MaxIdempotentBody caps what will be fingerprinted and buffered. A request
// larger than this is not something a form double-tap produces.
const MaxIdempotentBody = 1 << 20 // 1 MiB

// Idempotency implements TRD §6.4.
//
// `required` is the WI-21/WI-77 seam. With required=false the middleware
// records and replays whenever a client supplies a key, but a request without
// one passes straight through. That is deliberate: it lets the storage and the
// replay path carry real production traffic before anything starts rejecting
// requests for a missing header. WI-77 flips the endpoints in §6.5 marked
// `Idem` to required=true, at which point a client that forgets the header is
// a client that will double-issue, and gets told so.
//
// The failure being prevented is not hypothetical: a phlebotomist on a laggy
// tablet double-taps "Record collection" and the system mints two donations,
// two bags and two barcodes for one venepuncture.
// Idempotency options.
type IdempotencyOption func(*idempotencyConfig)

type idempotencyConfig struct {
	required    bool
	noStoreBody bool
}

// WithoutResponseBody records the key and the status but NOT the body.
//
// For endpoints whose successful response carries a credential — the invitation
// token is the one today. Storing it would put a one-time secret in a table
// that outlives the request, which is precisely what hashing it everywhere else
// was meant to avoid. A retry then replays the status with an empty body, which
// is the right trade: an idempotent retry of "create an invitation" must not
// hand out the secret a second time either.
func WithoutResponseBody() IdempotencyOption {
	return func(c *idempotencyConfig) { c.noStoreBody = true }
}

func Idempotency(store IdempotencyStore, required bool, opts ...IdempotencyOption) func(http.Handler) http.Handler {
	cfg := idempotencyConfig{required: required}
	for _, o := range opts {
		o(&cfg)
	}
	return idempotency(store, cfg)
}

func idempotency(store IdempotencyStore, cfg idempotencyConfig) func(http.Handler) http.Handler {
	required := cfg.required
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Safe methods are idempotent by definition; a key on one is
			// meaningless rather than wrong, so it is ignored.
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			key := r.Header.Get("Idempotency-Key")
			if key == "" {
				if required {
					writeErr(w, http.StatusBadRequest, "idempotency_key_required",
						"This endpoint requires an Idempotency-Key header.")
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			if len(key) < 8 || len(key) > 255 {
				writeErr(w, http.StatusBadRequest, "idempotency_key_invalid",
					"Idempotency-Key must be between 8 and 255 characters.")
				return
			}

			// The key is scoped to the caller, so an anonymous request has
			// nothing to scope it to. Endpoints needing idempotency are
			// authenticated anyway; this is a guard, not a policy.
			id, ok := IdentityFrom(r.Context())
			if !ok {
				next.ServeHTTP(w, r)
				return
			}

			body, err := io.ReadAll(io.LimitReader(r.Body, MaxIdempotentBody+1))
			if err != nil {
				writeErr(w, http.StatusBadRequest, "bad_request", "could not read the request body")
				return
			}
			if len(body) > MaxIdempotentBody {
				writeErr(w, http.StatusRequestEntityTooLarge, "payload_too_large", "request body is too large")
				return
			}
			// The handler still has to be able to read the body we just consumed.
			r.Body = io.NopCloser(bytes.NewReader(body))

			endpoint := r.Method + " " + routePattern(r)
			fp := fingerprint(r.Method, r.URL.Path, body)

			rec, claimed, err := store.Claim(r.Context(), id.UserID, key, endpoint, fp)
			if err != nil {
				slog.ErrorContext(r.Context(), "idempotency claim failed", "error", err)
				writeErr(w, http.StatusInternalServerError, "internal_error", "an unexpected error occurred")
				return
			}

			if !claimed {
				// Somebody already holds this key. Which of the three answers
				// applies depends on what they did with it.
				if !bytes.Equal(rec.Fingerprint, fp) {
					// Same key, different request. Replaying the stored response
					// would answer a question that was never asked.
					writeErr(w, http.StatusUnprocessableEntity, "idempotency_key_reuse",
						"This Idempotency-Key was already used for a different request.")
					return
				}
				if !rec.Completed() {
					w.Header().Set("Retry-After", "1")
					writeErr(w, http.StatusConflict, "request_in_progress",
						"The original request is still being processed. Retry shortly.")
					return
				}
				w.Header().Set("Idempotent-Replay", "true")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(int(*rec.Status))
				_, _ = w.Write(rec.Body)
				return
			}

			// We hold the claim: run the handler and capture what it produced.
			rec2 := &capturingWriter{ResponseWriter: w, buf: &bytes.Buffer{}}
			next.ServeHTTP(rec2, r)

			status := rec2.status
			if status == 0 {
				status = http.StatusOK
			}

			// A 5xx is NOT a stored outcome. The request may not have been
			// applied at all, and freezing "500" against this key for 24 hours
			// would make the honest client retry — correctly, with the same key —
			// and be handed the failure forever. Release instead, so a retry
			// genuinely retries.
			if status >= 500 {
				if err := store.Release(r.Context(), id.UserID, key); err != nil {
					slog.ErrorContext(r.Context(), "idempotency release failed", "error", err)
				}
				return
			}

			body2 := rec2.buf.Bytes()
			if cfg.noStoreBody {
				body2 = nil
			}
			if err := store.Complete(r.Context(), id.UserID, key, int32(status), body2); err != nil {
				// The response already went to the client, so this cannot change
				// the answer. Log it: the consequence is a retry that executes
				// twice, which is worth being able to find afterwards.
				slog.ErrorContext(r.Context(), "idempotency complete failed", "error", err)
			}
		})
	}
}

// fingerprint is SHA-256 over method, path and body (§6.4). The path is
// included because the same key against a different resource is a different
// intent, whatever the body says.
func fingerprint(method, path string, body []byte) []byte {
	h := sha256.New()
	h.Write([]byte(method))
	h.Write([]byte{0})
	h.Write([]byte(path))
	h.Write([]byte{0})
	h.Write(body)
	return h.Sum(nil)
}

// routePattern prefers the matched template ("/api/v1/donors/{id}") over the raw
// path, so record ids stay out of a table that is not row-level access
// controlled. Falls back to the path when the router has not matched yet.
func routePattern(r *http.Request) string {
	if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
		return rc.RoutePattern()
	}
	return r.URL.Path
}

// capturingWriter buffers the body so it can be stored for replay while still
// streaming it to the client.
type capturingWriter struct {
	http.ResponseWriter
	buf    *bytes.Buffer
	status int
}

func (c *capturingWriter) WriteHeader(code int) {
	c.status = code
	c.ResponseWriter.WriteHeader(code)
}

func (c *capturingWriter) Write(b []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	c.buf.Write(b)
	return c.ResponseWriter.Write(b)
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":{"code":"` + code + `","message":"` + msg + `"}}`))
}
