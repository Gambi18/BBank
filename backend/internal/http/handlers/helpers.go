package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"bbank/internal/http/response"
	"bbank/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// writeServiceError is the single place a service error becomes a status code.
//
// Handlers must never inspect a driver error to decide what happened — that is
// how `pq: duplicate key value violates unique constraint` ends up in a response
// body. The service returns a sentinel; this maps it, and anything unrecognised
// is a 500 whose detail goes to the log rather than to the caller.
func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	// Checked BEFORE ErrConflict, which ErrIneligible also matches. A clinical
	// refusal flattened into a bare "conflict" loses the criterion the client
	// branches on and the date the donor needs (FR-08, FR-19) — the whole reason
	// the decision travels as a value rather than a string.
	case isIneligible(err):
		writeIneligible(w, r, err)
	case errors.Is(err, service.ErrNotFound):
		response.NotFound(w, r)
	case errors.Is(err, service.ErrConflict):
		// 409: the request was well-formed, the world said no (§6.2).
		response.Conflict(w, r, "conflict", cleanMessage(err))
	case errors.Is(err, service.ErrInvalid):
		response.Unprocessable(w, r, cleanMessage(err))
	default:
		response.Internal(w, r, err)
	}
}

func isIneligible(err error) bool {
	_, ok := service.AsIneligible(err)
	return ok
}

// writeIneligible renders an eligibility decision as the error envelope.
//
// `code` is the headline criterion, so a client branches on the RULE rather than
// on the sentence (TRD §6.2). Every failing criterion appears in `details[]`,
// because FR-17 requires each to be named individually — a donor who fails three
// rules and is told about one will fix that one and be refused again.
func writeIneligible(w http.ResponseWriter, r *http.Request, err error) {
	e, _ := service.AsIneligible(err)
	d := e.Decision

	details := make([]response.Detail, 0, len(d.Failures))
	for _, f := range d.Failures {
		details = append(details, response.Detail{Field: string(f.Criterion), Issue: f.Message})
	}

	var next *string
	if d.NextEligibleOn != nil {
		s := d.NextEligibleOn.Format("2006-01-02")
		next = &s
	}

	message := "This donor is not eligible to donate on that date."
	if len(d.Failures) > 0 {
		message = d.Failures[0].Message
	}
	response.Ineligible(w, r, string(d.Reason()), message, next, details...)
}

// cleanMessage strips the sentinel prefix the service wraps its messages with,
// leaving the part written to be read by a person.
func cleanMessage(err error) string {
	msg := err.Error()
	for _, prefix := range []string{"conflict: ", "invalid: "} {
		if len(msg) > len(prefix) && msg[:len(prefix)] == prefix {
			return msg[len(prefix):]
		}
	}
	return msg
}

// idParam32 parses an :id that addresses an INTEGER primary key.
// donation_requests.id and appointments.id are `integer`, not `bigint`.
func idParam32(w http.ResponseWriter, r *http.Request) (int32, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 32)
	if err != nil || id <= 0 {
		response.BadRequest(w, r, "id must be a positive integer")
		return 0, false
	}
	return int32(id), true
}

// decodeOptional decodes a body that is allowed to be absent.
//
// A donor raising their own donation request supplies nothing at all, because
// everything is taken from their token. Treating that as a malformed body would
// make the correct request the rejected one.
func decodeOptional(r *http.Request, v any) error {
	err := json.NewDecoder(r.Body).Decode(v)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func dateStr(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format("2006-01-02")
}

func tsStr(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format("2006-01-02T15:04:05Z07:00")
}

func deref32(v *int32) int64 {
	if v == nil {
		return 0
	}
	return int64(*v)
}
