// Package legacy holds the original single-file handlers, moved verbatim.
//
// This is the strangler fig: WI-11 moves `donors` to the layered structure
// (domain -> store -> service -> http/handlers) while everything else keeps
// serving from here, unchanged, so the application never breaks mid-refactor.
// Subsequent work items (WI-22) migrate these one resource at a time; this
// package shrinks to nothing and is then deleted.
//
// Do not add anything here. New code goes in the layered packages.
package legacy

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"bbank/internal/domain"
	"bbank/internal/middleware"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"
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

type Donor struct {
	Id           int    `json:"id"`
	FullName     string `json:"full_name"`
	Email        string `json:"email"`
	DOB          string `json:"dob"`
	Gender       string `json:"gender"`
	BloodGroup   string `json:"blood_group"`
	Rhesus       string `json:"rhesus"`
	Contact      string `json:"contact"`
	Address      string `json:"address"`
	Password     string `json:"password,omitempty"`
	LastDonation string `json:"last_donation"`
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

const (
	donorsEndpoint         = "/api/go/donors"
	donorIDEndpoint        = "/api/go/donors/{id}"
	loginEndpoint          = "/api/go/login"
	requestsEndpoint       = "/api/go/requests"
	requestIDEndpoint      = "/api/go/requests/{id}"
	confirmRequestEndpoint = "/api/go/requests/{id}/confirm"
	appointmentsEndpoint   = "/api/go/appointments"
	appointmentIDEndpoint  = "/api/go/appointments/{id}"
)

// config holds every value the process reads from the environment. Nothing here
// has a credential-bearing default: missing required config is a startup failure,
// never a silent guess. (WI-07)

func login(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var creds struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		creds.Email = strings.TrimSpace(strings.ToLower(creds.Email))

		var d Donor
		var hash string
		var lastDonation sql.NullString
		err := db.QueryRow("SELECT id, full_name, email, dob, gender, blood_group, rhesus, contact, address, password, last_donation FROM donors WHERE email = $1", creds.Email).
			Scan(&d.Id, &d.FullName, &d.Email, &d.DOB, &d.Gender, &d.BloodGroup, &d.Rhesus, &d.Contact, &d.Address, &hash, &lastDonation)
		if err != nil {
			http.Error(w, "Invalid email or password", http.StatusUnauthorized)
			return
		}

		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(creds.Password)) != nil {
			http.Error(w, "Invalid email or password", http.StatusUnauthorized)
			return
		}

		d.LastDonation = lastDonation.String
		d.Password = "" // never return the hash
		json.NewEncoder(w).Encode(d)
	}
}

// Donor Handlers
func getDonors(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query("SELECT id, full_name, email, dob, gender, blood_group, rhesus, contact, address, last_donation FROM donors")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		donors := []Donor{}
		for rows.Next() {
			var d Donor
			var lastDonation sql.NullString
			err := rows.Scan(&d.Id, &d.FullName, &d.Email, &d.DOB, &d.Gender, &d.BloodGroup, &d.Rhesus, &d.Contact, &d.Address, &lastDonation)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			d.LastDonation = lastDonation.String
			donors = append(donors, d)
		}
		json.NewEncoder(w).Encode(donors)
	}
}

func getDonor(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var d Donor
		var lastDonation sql.NullString
		err := db.QueryRow("SELECT id, full_name, email, dob, gender, blood_group, rhesus, contact, address, last_donation FROM donors WHERE id = $1", id).
			Scan(&d.Id, &d.FullName, &d.Email, &d.DOB, &d.Gender, &d.BloodGroup, &d.Rhesus, &d.Contact, &d.Address, &lastDonation)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		d.LastDonation = lastDonation.String
		json.NewEncoder(w).Encode(d)
	}
}

func createDonor(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var d Donor
		if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Basic server-side validation
		d.FullName = strings.TrimSpace(d.FullName)
		d.Email = strings.TrimSpace(strings.ToLower(d.Email))
		if d.FullName == "" || d.Email == "" || d.Password == "" {
			http.Error(w, "Full name, email and password are required", http.StatusBadRequest)
			return
		}

		// Hash the password before storing
		hashed, err := bcrypt.GenerateFromPassword([]byte(d.Password), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, "Failed to secure password", http.StatusInternalServerError)
			return
		}

		var dob interface{}
		if d.DOB == "" {
			dob = nil
		} else {
			dob = d.DOB
		}

		var lastDonation interface{}
		if d.LastDonation == "" {
			lastDonation = nil
		} else {
			lastDonation = d.LastDonation
		}

		err = db.QueryRow("INSERT INTO donors (full_name, email, dob, gender, blood_group, rhesus, contact, address, password, last_donation) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id",
			d.FullName, d.Email, dob, d.Gender, d.BloodGroup, d.Rhesus, d.Contact, d.Address, string(hashed), lastDonation).Scan(&d.Id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		d.Password = "" // never echo credentials back
		json.NewEncoder(w).Encode(d)
	}
}

func updateDonor(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var d Donor
		if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var dob interface{}
		if d.DOB == "" {
			dob = nil
		} else {
			dob = d.DOB
		}
		var lastDonation interface{}
		if d.LastDonation == "" {
			lastDonation = nil
		} else {
			lastDonation = d.LastDonation
		}

		var err error
		if d.Password == "" {
			// No password change: leave the stored hash untouched
			_, err = db.Exec("UPDATE donors SET full_name=$1, email=$2, dob=$3, gender=$4, blood_group=$5, rhesus=$6, contact=$7, address=$8, last_donation=$9 WHERE id=$10",
				d.FullName, d.Email, dob, d.Gender, d.BloodGroup, d.Rhesus, d.Contact, d.Address, lastDonation, id)
		} else {
			hashed, hErr := bcrypt.GenerateFromPassword([]byte(d.Password), bcrypt.DefaultCost)
			if hErr != nil {
				http.Error(w, "Failed to secure password", http.StatusInternalServerError)
				return
			}
			_, err = db.Exec("UPDATE donors SET full_name=$1, email=$2, dob=$3, gender=$4, blood_group=$5, rhesus=$6, contact=$7, address=$8, password=$9, last_donation=$10 WHERE id=$11",
				d.FullName, d.Email, dob, d.Gender, d.BloodGroup, d.Rhesus, d.Contact, d.Address, string(hashed), lastDonation, id)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Ensure the returned ID is set in the donor object
		fmt.Sscanf(id, "%d", &d.Id)
		d.Password = ""
		json.NewEncoder(w).Encode(d)
	}
}

func deleteDonor(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		_, err := db.Exec("DELETE FROM donors WHERE id = $1", id)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode("Donor Deleted")
	}
}

// Request Handlers
func getRequests(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		where, args, ok := scopeClause(r.Context(), "r.donor_id", "r.center_id")
		if !ok {
			_ = json.NewEncoder(w).Encode([]Request{})
			return
		}
		rows, err := db.Query(`SELECT r.id, r.donor_id, p.full_name, p.legacy_last_donation, r.created_at
		                    FROM donation_requests r JOIN donor_profiles p ON p.user_id = r.donor_id
		                    WHERE r.status = 'pending'`+where+` ORDER BY r.id`, args...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		requests := []Request{}
		for rows.Next() {
			var req Request
			var lastDonation sql.NullString
			var createdAt sql.NullString
			if err := rows.Scan(&req.Id, &req.DonorId, &req.DonorName, &lastDonation, &createdAt); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			req.LastDonation = lastDonation.String
			req.CreatedAt = createdAt.String
			requests = append(requests, req)
		}
		json.NewEncoder(w).Encode(requests)
	}
}

func getRequest(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var req Request
		var lastDonation sql.NullString
		var createdAt sql.NullString
		var centerID sql.NullInt64
		err := db.QueryRow(`SELECT r.id, r.donor_id, r.center_id, p.full_name, p.legacy_last_donation, r.created_at
		                       FROM donation_requests r JOIN donor_profiles p ON p.user_id = r.donor_id
		                       WHERE r.id = $1`, id).Scan(&req.Id, &req.DonorId, &centerID, &req.DonorName, &lastDonation, &createdAt)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// 404, not 403 — a caller outside the scope must not learn the row exists.
		if !middleware.Permits(r.Context(), rowOf(int64(req.DonorId), centerID)) {
			middleware.Deny(w)
			return
		}
		req.LastDonation = lastDonation.String
		req.CreatedAt = createdAt.String
		json.NewEncoder(w).Encode(req)
	}
}

func createRequest(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
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
		err := db.QueryRow("SELECT full_name, legacy_last_donation FROM donor_profiles WHERE user_id = $1", req.DonorId).Scan(&req.DonorName, &lastDonation)
		if err != nil {
			http.Error(w, "Donor not found", http.StatusBadRequest)
			return
		}
		req.LastDonation = lastDonation.String

		err = db.QueryRow(`INSERT INTO donation_requests (donor_id, center_id, preferred_date, status)
		                       VALUES ($1, (SELECT id FROM donation_centers WHERE code = 'MAIN'),
		                               (CURRENT_DATE + 7), 'pending')
		                       RETURNING id`, req.DonorId).Scan(&req.Id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(req)
	}
}

func confirmRequest(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		type ConfirmPayload struct {
			Date string `json:"date"`
		}
		var payload ConfirmPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Run the whole confirm flow in one transaction so a failure can't lose the request.
		tx, err := db.Begin()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer tx.Rollback() // no-op once committed

		// 1. Get request details
		var req Request
		var centerID sql.NullInt64
		err = tx.QueryRow(`SELECT r.id, r.donor_id, r.center_id, p.full_name
		                       FROM donation_requests r JOIN donor_profiles p ON p.user_id = r.donor_id
		                       WHERE r.id = $1 AND r.status = 'pending'`, id).Scan(&req.Id, &req.DonorId, &centerID, &req.DonorName)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// RequireTransition has already established that this role may approve at
		// all; this establishes that it may approve *this* request. Staff are
		// center-scoped, so a request raised at another center is not theirs.
		if !middleware.Permits(r.Context(), rowOf(int64(req.DonorId), centerID)) {
			middleware.Deny(w)
			return
		}

		// 2. Create appointment
		var appt Appointment
		err = tx.QueryRow(`INSERT INTO appointments (donation_request_id, donor_id, center_id, scheduled_at, status)
		                       VALUES ($1, $2, (SELECT center_id FROM donation_requests WHERE id = $1),
		                               ($3::date + TIME '09:00') AT TIME ZONE 'Africa/Douala', 'scheduled')
		                       RETURNING id`, req.Id, req.DonorId, payload.Date).Scan(&appt.Id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// 3. Mark the request approved. It is NOT deleted.
		//
		// The original code ran `DELETE FROM requests` here. That is the defect
		// that destroyed the link back to "who asked and when" for every
		// historical appointment — see the quarantined rows in migration_rejects.
		// WI-22 formalises this transition; it is fixed here because leaving the
		// DELETE in place would keep destroying the audit chain in the meantime.
		if _, err = tx.Exec(
			`UPDATE donation_requests SET status = 'approved', reviewed_at = now() WHERE id = $1`,
			req.Id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err = tx.Commit(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		appt.RequestId = req.Id
		appt.DonorId = req.DonorId
		appt.DonorName = req.DonorName
		appt.AppointmentDate = payload.Date

		json.NewEncoder(w).Encode(appt)
	}
}

// Appointment Handlers
func getAppointments(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		where, args, ok := scopeClause(r.Context(), "a.donor_id", "a.center_id")
		if !ok {
			_ = json.NewEncoder(w).Encode([]Appointment{})
			return
		}

		// `?donor_id=` survives only as a convenience filter for callers whose
		// scope is already wider than one donor. It can narrow the result set and
		// never widen it, because `where` is appended first and unconditionally.
		if middleware.ScopeFrom(r.Context()) != domain.ScopeOwn {
			if donorId := r.URL.Query().Get("donor_id"); donorId != "" {
				n, convErr := strconv.ParseInt(donorId, 10, 64)
				if convErr != nil {
					http.Error(w, "donor_id must be an integer", http.StatusBadRequest)
					return
				}
				where += fmt.Sprintf(" AND a.donor_id = $%d", len(args)+1)
				args = append(args, n)
			}
		}

		rows, err := db.Query(`SELECT a.id, COALESCE(a.donation_request_id, 0), a.donor_id, p.full_name,
		                          (a.scheduled_at AT TIME ZONE 'Africa/Douala')::date
		                   FROM appointments a JOIN donor_profiles p ON p.user_id = a.donor_id
		                   WHERE true`+where+` ORDER BY a.scheduled_at DESC`, args...)

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		appointments := []Appointment{}
		for rows.Next() {
			var appt Appointment
			if err := rows.Scan(&appt.Id, &appt.RequestId, &appt.DonorId, &appt.DonorName, &appt.AppointmentDate); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			appointments = append(appointments, appt)
		}
		json.NewEncoder(w).Encode(appointments)
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
			w.WriteHeader(http.StatusNotFound)
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

		json.NewEncoder(w).Encode(appt)
	}
}

// RegisterRoutes mounts the legacy endpoints that have not yet been migrated to
// the layered structure. `donors` is deliberately absent — it is served by
// internal/http/handlers as the WI-11 pilot.
func RegisterRoutes(r chi.Router, db *sql.DB) {
	// Public: this is how a caller obtains a session in the first place.
	r.Post(loginEndpoint, login(db))

	// Everything else carries its cell of the TRD §7.6 matrix. RequirePermission
	// rejects an anonymous caller with 401 before it consults the matrix, so no
	// separate RequireAuth is needed — and a route added here without a
	// permission would be conspicuous rather than quietly open.
	rq := func(a domain.Action) func(http.Handler) http.Handler {
		return middleware.RequirePermission("donation_requests", a)
	}
	r.With(rq(domain.Read)).Get(requestsEndpoint, getRequests(db))
	r.With(rq(domain.Create)).Post(requestsEndpoint, createRequest(db))
	r.With(rq(domain.Read)).Get(requestIDEndpoint, getRequest(db))
	// Confirming is `X-approve`, which staff and admin hold and a donor does not,
	// even on their own request (§7.6). A bare Execute check would let a donor
	// approve themselves and delete the review step the system exists for.
	r.With(middleware.RequireTransition("donation_requests", "approve")).
		Post(confirmRequestEndpoint, confirmRequest(db))

	appt := middleware.RequirePermission("appointments", domain.Read)
	r.With(appt).Get(appointmentsEndpoint, getAppointments(db))
	r.With(appt).Get(appointmentIDEndpoint, getAppointment(db))
}
