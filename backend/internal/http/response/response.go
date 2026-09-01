// Package response holds the wire envelopes every endpoint returns.
//
// TRD §6.2: success is {"data": ..., "meta": {...}} (plus "page" when
// paginated); failure is {"error": {code, message, details[], request_id}}.
//
// Two rules this package exists to make unavoidable:
//
//  1. **No bare arrays or bare scalars.** An unenveloped array cannot grow a
//     `page` object later without breaking every client that reads it.
//  2. **No raw driver errors.** `err.Error()` from the database layer leaks
//     schema details to whoever asks and means nothing to a client. `Internal`
//     logs the real error and returns a generic one; the two audiences are
//     different and are served differently.
package response

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

type Envelope struct {
	Data any   `json:"data"`
	Page *Page `json:"page,omitempty"`
	Meta *Meta `json:"meta,omitempty"`
}

// Meta carries the correlation id on success too, not only on failure. A user
// reporting "the list looked wrong" can quote it just as usefully as a user
// reporting an error, and the whole trace is then one query away.
type Meta struct {
	RequestID  string `json:"request_id,omitempty"`
	ServerTime string `json:"server_time"`
}

// Page is the offset form (TRD §6.3), which is what the currently bounded list
// endpoints use. `Limit` reports the limit actually APPLIED after clamping, not
// what the caller asked for — a caller who sends limit=5000 needs to be able to
// see that it became 100, or their paging loop silently misses rows.
//
// §6.3 makes cursor/keyset the default for sets that grow while being paged
// (`blood_units`, `audit_log`). Those endpoints do not exist yet; when they
// arrive they add a cursor variant of this struct rather than retrofitting
// offsets onto a moving table.
type Page struct {
	Total  int64 `json:"total"`
	Limit  int32 `json:"limit"`
	Offset int32 `json:"offset"`
}

// Detail is one field-level validation failure (§6.2). Structured rather than a
// sentence so a client can attach the message to the offending input.
type Detail struct {
	Field string `json:"field"`
	Issue string `json:"issue"`
}

type Error struct {
	Code      string   `json:"code"`
	Message   string   `json:"message"`
	Details   []Detail `json:"details,omitempty"`
	RequestID string   `json:"request_id,omitempty"`
}

type errorEnvelope struct {
	Error Error `json:"error"`
}

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// The status and some bytes may already be on the wire, so this cannot
		// be turned into a 500. Logging it is the only honest option left.
		slog.Error("failed to encode response", "error", err)
	}
}

func OK(w http.ResponseWriter, data any) {
	JSON(w, http.StatusOK, Envelope{Data: data, Meta: meta(w)})
}

func Created(w http.ResponseWriter, data any) {
	JSON(w, http.StatusCreated, Envelope{Data: data, Meta: meta(w)})
}

// NoContent is 204 — deletes and revocations. Deliberately writes no body:
// an envelope with a null `data` would imply there was something to send.
func NoContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }

func Paged(w http.ResponseWriter, data any, total int64, limit, offset int32) {
	JSON(w, http.StatusOK, Envelope{
		Data: data,
		Page: &Page{Total: total, Limit: limit, Offset: offset},
		Meta: meta(w),
	})
}

// Fail writes the error envelope. `message` must be safe to show a client:
// never pass err.Error() from the database layer straight through.
func Fail(w http.ResponseWriter, r *http.Request, status int, code, message string, details ...Detail) {
	JSON(w, status, errorEnvelope{Error{
		Code:      code,
		Message:   message,
		Details:   details,
		RequestID: requestID(w, r),
	}})
}

func BadRequest(w http.ResponseWriter, r *http.Request, msg string, details ...Detail) {
	Fail(w, r, http.StatusBadRequest, "bad_request", msg, details...)
}

// Unprocessable is 422 — the request was well-formed but a field is invalid.
// Distinct from 400 (malformed) and from 409 (well-formed, but the world said no).
func Unprocessable(w http.ResponseWriter, r *http.Request, msg string, details ...Detail) {
	Fail(w, r, http.StatusUnprocessableEntity, "validation_failed", msg, details...)
}

// Conflict is 409 — a domain rule refused a well-formed request (donor not
// eligible, unit expired). §6.2 is explicit that these are NOT 400: the client
// did nothing wrong, the world said no, and clients branch on that difference.
func Conflict(w http.ResponseWriter, r *http.Request, code, msg string, details ...Detail) {
	Fail(w, r, http.StatusConflict, code, msg, details...)
}

func Unauthorized(w http.ResponseWriter, r *http.Request, msg string) {
	Fail(w, r, http.StatusUnauthorized, "unauthenticated", msg)
}

// NotFound is also the answer for a record the caller may not see. §6.2: a 403
// confirms the record exists, which is itself a disclosure.
func NotFound(w http.ResponseWriter, r *http.Request) {
	Fail(w, r, http.StatusNotFound, "not_found", "resource not found")
}

func MethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	Fail(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "that method is not supported on this path")
}

// Internal logs the real error and returns a generic message. The two must stay
// separate: the log is for operators, the response is for clients.
func Internal(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "unhandled error", "error", err, "path", r.URL.Path)
	Fail(w, r, http.StatusInternalServerError, "internal_error", "an unexpected error occurred")
}

func meta(w http.ResponseWriter) *Meta {
	return &Meta{
		RequestID:  w.Header().Get("X-Request-Id"),
		ServerTime: time.Now().UTC().Format(time.RFC3339),
	}
}

// requestID reads the correlation id the RequestID middleware put on the
// RESPONSE header.
//
// It deliberately does not read the request header: that only carries a value
// when the caller supplied one, so a generated id — the normal case — would
// come back empty and every error envelope would be untraceable. Reading it off
// the ResponseWriter also keeps this package from importing middleware, which
// would be a cycle waiting to happen.
func requestID(w http.ResponseWriter, r *http.Request) string {
	if w != nil {
		if v := w.Header().Get("X-Request-Id"); v != "" {
			return v
		}
	}
	if r != nil {
		return r.Header.Get("X-Request-Id")
	}
	return ""
}
