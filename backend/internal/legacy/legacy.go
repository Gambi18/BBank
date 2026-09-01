// Package legacy holds the original single-file handlers, moved verbatim.
//
// This is the strangler fig: WI-11 moved `donors` to the layered structure
// (domain -> store -> service -> http/handlers) while everything else kept
// serving from here, so the application never broke mid-refactor. WI-22 migrates
// what is left — donation requests and appointments — and this package is then
// deleted.
//
// Do not add anything here. New code goes in the layered packages.
//
// **These handlers are mounted on the canonical /api/v1 paths** (WI-21). The
// deprecated /api/go/ spelling is served by middleware.LegacyShim, which
// rewrites the path before routing, so there is exactly one implementation of
// each endpoint and the two spellings cannot drift apart.
//
// WI-21 also removed two things from this file:
//
//   - The five donor handlers. They stopped being routed when WI-11 moved
//     donors to the layered path, so they had been dead code leaking driver
//     errors at anyone who ever wired them back up. WI-22 rebuilds the writes
//     properly against users + donor_profiles.
//   - `POST /api/go/login`. It checked a password and returned a donor object
//     without issuing a session, which after WI-17 is not a login at all. The
//     shim now points the legacy path at the real handler, so old callers get a
//     genuine ES256 session instead of a second, weaker auth path. This is a
//     deliberate deviation from TRD §6.1, which preserves the old response body:
//     WI-19 already migrated the only client, so there is nobody left to keep
//     compatible, and a second password-checking code path is a liability.
package legacy

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"bbank/internal/domain"
	"bbank/internal/http/response"
	"bbank/internal/middleware"

	"github.com/go-chi/chi/v5"
)

// scopeClause turns the scope granted by the RBAC middleware into a mandatory
// SQL predicate (TRD §7.7). The caller cannot widen it: a `?donor_id=` in the
// query string is a filter, never an identity.
//
// ok=false means the scope cannot be satisfied at all — an unbounded read must
// then return nothing, not everything. That direction of failure is the point.
func scopeClause(ctx context.Context, ownerCol, centerCol string) (string, []any, bool) {
	id, authed := middleware.IdentityFrom(ctx)
	if !authed {
		return "", nil, false
	}
	switch middleware.ScopeFrom(ctx) {
	case domain.ScopeAll:
		return "", nil, true
	case domain.ScopeOwn:
		return " AND " + ownerCol + " = $1", []any{id.UserID}, true
	case domain.ScopeCenter:
		if id.CenterID == nil {
			return "", nil, false // staff with no center sees no rows, not all rows
		}
		return " AND " + centerCol + " = $1", []any{*id.CenterID}, true
	}
	return "", nil, false
}

// rowOf adapts a nullable center column to the Row that Permits evaluates.
func rowOf(ownerID int64, centerID sql.NullInt64) middleware.Row {
	row := middleware.Row{OwnerID: ownerID}
	if centerID.Valid {
		c := centerID.Int64
		row.CenterID = &c
	}
	return row
}

type Request struct {
	Id           int    `json:"id"`
	DonorId      int    `json:"donor_id"`
	DonorName    string `json:"donor_name"`
	LastDonation string `json:"last_donation"`
	CreatedAt    string `json:"created_at"`
}

type Appointment struct {
	Id              int    `json:"id"`
	RequestId       int    `json:"request_id"`
	DonorId         int    `json:"donor_id"`
	DonorName       string `json:"donor_name"`
	AppointmentDate string `json:"appointment_date"`
}

// Canonical paths (TRD §6.1). `requests` became `donation-requests` because a
// `blood-request` — a hospital asking for units — is a different concept, and
// reusing the word would have been a semantic trap.
const (
	requestsEndpoint       = "/api/v1/donation-requests"
	requestIDEndpoint      = "/api/v1/donation-requests/{id}"
	approveRequestEndpoint = "/api/v1/donation-requests/{id}/approve"
	appointmentsEndpoint   = "/api/v1/appointments"
	appointmentIDEndpoint  = "/api/v1/appointments/{id}"
)

// Request Handlers

func getRequests(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		where, args, ok := scopeClause(r.Context(), "r.donor_id", "r.center_id")
		if !ok {
			response.Paged(w, []Request{}, 0, response.DefaultLimit, 0)
			return
		}
		paging, ok := response.ParsePaging(w, r)
		if !ok {
			return
		}

		const from = ` FROM donation_requests r JOIN donor_profiles p ON p.user_id = r.donor_id
		               WHERE r.status = 'pending'`

		var total int64
		if err := db.QueryRow(`SELECT count(*)`+from+where, args...).Scan(&total); err != nil {
			response.Internal(w, r, err)
			return
		}

		// LIMIT/OFFSET close the unbounded scan (A15). The placeholders continue
		// the numbering the scope clause started, so a scoped and an unscoped
		// caller both get a valid statement.
		rows, err := db.Query(`SELECT r.id, r.donor_id, p.full_name, p.legacy_last_donation, r.created_at`+from+where+
			fmt.Sprintf(" ORDER BY r.id LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2),
			append(args, paging.Limit, paging.Offset)...)
		if err != nil {
			response.Internal(w, r, err)
			return
		}
		defer rows.Close()

		requests := []Request{}
		for rows.Next() {
			var req Request
			var lastDonation, createdAt sql.NullString
			if err := rows.Scan(&req.Id, &req.DonorId, &req.DonorName, &lastDonation, &createdAt); err != nil {
				response.Internal(w, r, err)
				return
			}
			req.LastDonation = lastDonation.String
			req.CreatedAt = createdAt.String
			requests = append(requests, req)
		}
		if err := rows.Err(); err != nil {
			response.Internal(w, r, err)
			return
		}
		response.Paged(w, requests, total, paging.Limit, paging.Offset)
	}
}

func getRequest(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var req Request
		var lastDonation, createdAt sql.NullString
		var centerID sql.NullInt64
		err := db.QueryRow(`SELECT r.id, r.donor_id, r.center_id, p.full_name, p.legacy_last_donation, r.created_at
		                       FROM donation_requests r JOIN donor_profiles p ON p.user_id = r.donor_id
		                       WHERE r.id = $1`, id).Scan(&req.Id, &req.DonorId, &centerID, &req.DonorName, &lastDonation, &createdAt)
		if err != nil {
			response.NotFound(w, r)
			return
		}
		// 404, not 403 — a caller outside the scope must not learn the row exists.
		if !middleware.Permits(r.Context(), rowOf(int64(req.DonorId), centerID)) {
			middleware.Deny(w)
			return
		}
		req.LastDonation = lastDonation.String
		req.CreatedAt = createdAt.String
		response.OK(w, req)
	}
}

func createRequest(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req Request
		// An empty body is legitimate here: a donor raising their own request
		// supplies nothing, because everything is taken from their token.
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
			response.BadRequest(w, r, "invalid JSON body")
			return
		}

		// A donor raises a request for themselves and for nobody else, whatever
		// the body says. Staff and admin may raise one on a donor's behalf, so
		// their wider scope keeps reading donor_id from the payload.
		if middleware.ScopeFrom(r.Context()) == domain.ScopeOwn {
			id, _ := middleware.IdentityFrom(r.Context())
			req.DonorId = int(id.UserID)
		}

		// Read the name from donor_profiles, the table donation_requests.donor_id
		// actually references. Reading the pre-migration `donors` table here could
		// succeed for a donor with no profile row and then fail the insert on the
		// foreign key, turning a 400 into a 500.
		var lastDonation sql.NullString
		if err := db.QueryRow("SELECT full_name, legacy_last_donation FROM donor_profiles WHERE user_id = $1",
			req.DonorId).Scan(&req.DonorName, &lastDonation); err != nil {
			response.Unprocessable(w, r, "that donor does not exist",
				response.Detail{Field: "donor_id", Issue: "no donor profile with that id"})
			return
		}
		req.LastDonation = lastDonation.String

		if err := db.QueryRow(`INSERT INTO donation_requests (donor_id, center_id, preferred_date, status)
		                       VALUES ($1, (SELECT id FROM donation_centers WHERE code = 'MAIN'),
		                               (CURRENT_DATE + 7), 'pending')
		                       RETURNING id`, req.DonorId).Scan(&req.Id); err != nil {
			response.Internal(w, r, err)
			return
		}
		response.Created(w, req)
	}
}

// approveRequest schedules the appointment and marks the request approved.
//
// It is `approve`, not `confirm`, and it does NOT delete the request row. The
// original code ran `DELETE FROM requests` here, which destroyed the link back
// to "who asked and when" for every historical appointment — see the
// quarantined rows in migration_rejects. WI-22 formalises the transition; the
// DELETE was removed early because leaving it in would have kept destroying the
// audit chain in the meantime.
func approveRequest(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var payload struct {
			Date string `json:"date"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			response.BadRequest(w, r, "invalid JSON body")
			return
		}
		if payload.Date == "" {
			response.Unprocessable(w, r, "a date is required",
				response.Detail{Field: "date", Issue: "required"})
			return
		}

		// One transaction, so a failure cannot leave an appointment without its
		// approved request or vice versa.
		tx, err := db.Begin()
		if err != nil {
			response.Internal(w, r, err)
			return
		}
		defer tx.Rollback() //nolint:errcheck // no-op once committed

		var req Request
		var centerID sql.NullInt64
		if err := tx.QueryRow(`SELECT r.id, r.donor_id, r.center_id, p.full_name
		                       FROM donation_requests r JOIN donor_profiles p ON p.user_id = r.donor_id
		                       WHERE r.id = $1 AND r.status = 'pending'`, id).
			Scan(&req.Id, &req.DonorId, &centerID, &req.DonorName); err != nil {
			response.NotFound(w, r)
			return
		}

		// RequireTransition has already established that this role may approve at
		// all; this establishes that it may approve *this* request. Staff are
		// center-scoped, so a request raised at another center is not theirs.
		if !middleware.Permits(r.Context(), rowOf(int64(req.DonorId), centerID)) {
			middleware.Deny(w)
			return
		}

		var appt Appointment
		if err := tx.QueryRow(`INSERT INTO appointments (donation_request_id, donor_id, center_id, scheduled_at, status)
		                       VALUES ($1, $2, (SELECT center_id FROM donation_requests WHERE id = $1),
		                               ($3::date + TIME '09:00') AT TIME ZONE 'Africa/Douala', 'scheduled')
		                       RETURNING id`, req.Id, req.DonorId, payload.Date).Scan(&appt.Id); err != nil {
			response.Internal(w, r, err)
			return
		}

		if _, err := tx.Exec(
			`UPDATE donation_requests SET status = 'approved', reviewed_at = now() WHERE id = $1`,
			req.Id); err != nil {
			response.Internal(w, r, err)
			return
		}

		if err := tx.Commit(); err != nil {
			response.Internal(w, r, err)
			return
		}

		appt.RequestId = req.Id
		appt.DonorId = req.DonorId
		appt.DonorName = req.DonorName
		appt.AppointmentDate = payload.Date
		response.Created(w, appt)
	}
}

// Appointment Handlers

func getAppointments(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		where, args, ok := scopeClause(r.Context(), "a.donor_id", "a.center_id")
		if !ok {
			response.Paged(w, []Appointment{}, 0, response.DefaultLimit, 0)
			return
		}
		paging, ok := response.ParsePaging(w, r)
		if !ok {
			return
		}

		// `?donor_id=` survives only as a convenience filter for callers whose
		// scope is already wider than one donor. It can narrow the result set and
		// never widen it, because `where` is appended first and unconditionally.
		if middleware.ScopeFrom(r.Context()) != domain.ScopeOwn {
			if donorId := r.URL.Query().Get("donor_id"); donorId != "" {
				n, convErr := strconv.ParseInt(donorId, 10, 64)
				if convErr != nil {
					response.BadRequest(w, r, "donor_id must be an integer",
						response.Detail{Field: "donor_id", Issue: "not an integer"})
					return
				}
				where += fmt.Sprintf(" AND a.donor_id = $%d", len(args)+1)
				args = append(args, n)
			}
		}

		const from = ` FROM appointments a JOIN donor_profiles p ON p.user_id = a.donor_id WHERE true`

		var total int64
		if err := db.QueryRow(`SELECT count(*)`+from+where, args...).Scan(&total); err != nil {
			response.Internal(w, r, err)
			return
		}

		rows, err := db.Query(`SELECT a.id, COALESCE(a.donation_request_id, 0), a.donor_id, p.full_name,
		                          (a.scheduled_at AT TIME ZONE 'Africa/Douala')::date`+from+where+
			fmt.Sprintf(" ORDER BY a.scheduled_at DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2),
			append(args, paging.Limit, paging.Offset)...)
		if err != nil {
			response.Internal(w, r, err)
			return
		}
		defer rows.Close()

		appointments := []Appointment{}
		for rows.Next() {
			var appt Appointment
			if err := rows.Scan(&appt.Id, &appt.RequestId, &appt.DonorId, &appt.DonorName, &appt.AppointmentDate); err != nil {
				response.Internal(w, r, err)
				return
			}
			appointments = append(appointments, appt)
		}
		if err := rows.Err(); err != nil {
			response.Internal(w, r, err)
			return
		}
		response.Paged(w, appointments, total, paging.Limit, paging.Offset)
	}
}

func getAppointment(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		// WI-02 -> WI-20 — the authorization bypass, closed properly.
		//
		// WI-02 made the ownership check unconditional but still compared against
		// `?donor_id=`, a value the caller supplied: a bypass fix, not authorization.
		// The comparison is now against the `sub` claim of a verified token, so
		// asserting someone else's identity is no longer something the request can
		// express. `?donor_id=` is not read here at all.
		var appt Appointment
		var centerID sql.NullInt64
		err := db.QueryRow(`SELECT a.id, COALESCE(a.donation_request_id, 0), a.donor_id, a.center_id, p.full_name,
		                       (a.scheduled_at AT TIME ZONE 'Africa/Douala')::date
		                FROM appointments a JOIN donor_profiles p ON p.user_id = a.donor_id
		                WHERE a.id = $1`, id).Scan(&appt.Id, &appt.RequestId, &appt.DonorId, &centerID, &appt.DonorName, &appt.AppointmentDate)
		if err != nil {
			response.NotFound(w, r)
			return
		}

		// 404, not 403: a non-owner must not learn that this appointment exists.
		if !middleware.Permits(r.Context(), rowOf(int64(appt.DonorId), centerID)) {
			caller, _ := middleware.IdentityFrom(r.Context())
			slog.WarnContext(r.Context(), "appointment ownership check failed",
				slog.String("appointment_id", id),
				slog.Int64("caller_user_id", caller.UserID),
				slog.String("caller_role", string(caller.Role)),
			)
			middleware.Deny(w)
			return
		}

		response.OK(w, appt)
	}
}

// RegisterRoutes mounts the legacy endpoints that have not yet been migrated to
// the layered structure, on their canonical /api/v1 paths. `donors` is
// deliberately absent — it is served by internal/http/handlers as the WI-11
// pilot.
func RegisterRoutes(r chi.Router, db *sql.DB, idem middleware.IdempotencyStore) {
	// Every route carries its cell of the TRD §7.6 matrix. RequirePermission
	// rejects an anonymous caller with 401 before it consults the matrix, so no
	// separate RequireAuth is needed — and a route added here without a
	// permission would be conspicuous rather than quietly open.
	rq := func(a domain.Action) func(http.Handler) http.Handler {
		return middleware.RequirePermission("donation_requests", a)
	}

	// Idempotency is recorded but not yet required (WI-21 scaffolding); WI-77
	// turns `required` on for the endpoints §6.5 marks `Idem`.
	replay := middleware.Idempotency(idem, false)

	r.With(rq(domain.Read)).Get(requestsEndpoint, getRequests(db))
	r.With(rq(domain.Create), replay).Post(requestsEndpoint, createRequest(db))
	r.With(rq(domain.Read)).Get(requestIDEndpoint, getRequest(db))
	// Approving is `X-approve`, which staff and admin hold and a donor does not,
	// even on their own request (§7.6). A bare Execute check would let a donor
	// approve themselves and delete the review step the system exists for.
	r.With(middleware.RequireTransition("donation_requests", "approve"), replay).
		Post(approveRequestEndpoint, approveRequest(db))

	appt := middleware.RequirePermission("appointments", domain.Read)
	r.With(appt).Get(appointmentsEndpoint, getAppointments(db))
	r.With(appt).Get(appointmentIDEndpoint, getAppointment(db))
}
