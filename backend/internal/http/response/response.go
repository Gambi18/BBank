// Package response holds the wire envelopes every endpoint returns.
//
// TRD §6.2: success is {"data": ...} (plus "page" when paginated); failure is
// {"error": {code, message, details[], request_id}}. No endpoint returns a bare
// array, and no endpoint ever returns a raw driver error — a database message can
// leak schema details and is meaningless to a client.
package response

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type Envelope struct {
	Data any   `json:"data"`
	Page *Page `json:"page,omitempty"`
}

type Page struct {
	Total  int64 `json:"total"`
	Limit  int32 `json:"limit"`
	Offset int32 `json:"offset"`
}

type Error struct {
	Code      string   `json:"code"`
	Message   string   `json:"message"`
	Details   []string `json:"details,omitempty"`
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
		slog.Error("failed to encode response", "error", err)
	}
}

func OK(w http.ResponseWriter, data any)      { JSON(w, http.StatusOK, Envelope{Data: data}) }
func Created(w http.ResponseWriter, data any) { JSON(w, http.StatusCreated, Envelope{Data: data}) }

func Paged(w http.ResponseWriter, data any, total int64, limit, offset int32) {
	JSON(w, http.StatusOK, Envelope{Data: data, Page: &Page{Total: total, Limit: limit, Offset: offset}})
}

// Fail writes the error envelope. `message` must be safe to show a client:
// never pass err.Error() from the database layer straight through.
func Fail(w http.ResponseWriter, r *http.Request, status int, code, message string, details ...string) {
	JSON(w, status, errorEnvelope{Error{
		Code:      code,
		Message:   message,
		Details:   details,
		RequestID: requestID(r),
	}})
}

func BadRequest(w http.ResponseWriter, r *http.Request, msg string, details ...string) {
	Fail(w, r, http.StatusBadRequest, "bad_request", msg, details...)
}

func NotFound(w http.ResponseWriter, r *http.Request) {
	Fail(w, r, http.StatusNotFound, "not_found", "resource not found")
}

// Internal logs the real error and returns a generic message. The two must stay
// separate: the log is for operators, the response is for clients.
func Internal(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "unhandled error", "error", err, "path", r.URL.Path)
	Fail(w, r, http.StatusInternalServerError, "internal_error", "an unexpected error occurred")
}

func requestID(r *http.Request) string {
	if r == nil {
		return ""
	}
	if v := r.Header.Get("X-Request-Id"); v != "" {
		return v
	}
	return ""
}
