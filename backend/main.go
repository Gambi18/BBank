package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

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
type config struct {
	databaseURL    string
	port           string
	allowedOrigins []string
	logLevel       slog.Level
	shutdownGrace  time.Duration
}

func loadConfig() (config, error) {
	c := config{port: envOr("PORT", "8000"), shutdownGrace: 20 * time.Second}

	// WI-07: no fallback DSN. The old hardcoded localhost default meant a
	// misconfigured deployment silently pointed at the wrong database.
	c.databaseURL = os.Getenv("DATABASE_URL")
	if c.databaseURL == "" {
		return c, errors.New("DATABASE_URL is required (see .env.example); refusing to guess a connection string")
	}

	// WI-03: CORS is an explicit allowlist. Empty means no cross-origin browser
	// access, which is correct for the server-to-server topology we actually use.
	if raw := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS")); raw != "" {
		for _, o := range strings.Split(raw, ",") {
			if o = strings.TrimSpace(o); o != "" {
				c.allowedOrigins = append(c.allowedOrigins, o)
			}
		}
	}

	switch strings.ToLower(envOr("LOG_LEVEL", "info")) {
	case "debug":
		c.logLevel = slog.LevelDebug
	case "warn":
		c.logLevel = slog.LevelWarn
	case "error":
		c.logLevel = slog.LevelError
	default:
		c.logLevel = slog.LevelInfo
	}
	return c, nil
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// safeDSN renders a connection string for logs with the password removed.
// WI-01: the previous code printed the raw DSN, password included, on every boot.
func safeDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return "(unparseable DSN)"
	}
	if u.User != nil {
		if _, hasPassword := u.User.Password(); hasPassword {
			u.User = url.UserPassword(u.User.Username(), "xxxxx")
		}
	}
	return u.Redacted()
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}

	// WI-10: structured JSON logging replaces log/fmt.
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.logLevel}))
	slog.SetDefault(logger)

	db, err := sql.Open("postgres", cfg.databaseURL)
	if err != nil {
		logger.Error("cannot open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// WI-04: bound the pool. Go's default MaxOpenConns is unlimited, which lets one
	// replica exhaust Postgres' max_connections and lock out migrations and psql
	// alongside it. Values per TRD 11.3.
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	logger.Info("connecting to database", "dsn", safeDSN(cfg.databaseURL))

	// Retry instead of dying: in Docker the DB may still be initializing when this
	// container starts, and exiting here would take the container down with it.
	for attempt := 1; ; attempt++ {
		if err = db.Ping(); err == nil {
			logger.Info("database connection successful")
			break
		}
		if attempt >= 30 {
			logger.Error("database unreachable", "attempts", attempt, "error", err)
			os.Exit(1)
		}
		logger.Warn("waiting for database", "attempt", attempt, "max_attempts", 30, "error", err)
		time.Sleep(2 * time.Second)
	}

	// Schema is owned by golang-migrate (backend/migrations/), not by this process.
	// WI-08 removed the CREATE TABLE IF NOT EXISTS block that used to run here.
	// It could not detect drift (IF NOT EXISTS silently skips a changed table),
	// carried no version, had no down path, and raced itself across replicas.
	// The `migrate` compose service runs to completion before this container starts.

	// Create router
	router := mux.NewRouter()

	// Ops endpoints (unversioned, per TRD 6). Liveness never touches the database.
	router.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}).Methods("GET")
	router.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"database unreachable"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	}).Methods("GET")

	// Auth
	router.HandleFunc(loginEndpoint, login(db)).Methods("POST")

	// Donor routes
	router.HandleFunc(donorsEndpoint, getDonors(db)).Methods("GET")
	router.HandleFunc(donorsEndpoint, createDonor(db)).Methods("POST")
	router.HandleFunc(donorIDEndpoint, getDonor(db)).Methods("GET")
	router.HandleFunc(donorIDEndpoint, updateDonor(db)).Methods("PUT")
	router.HandleFunc(donorIDEndpoint, deleteDonor(db)).Methods("DELETE")

	// Request routes
	router.HandleFunc(requestsEndpoint, getRequests(db)).Methods("GET")
	router.HandleFunc(requestsEndpoint, createRequest(db)).Methods("POST")
	router.HandleFunc(requestIDEndpoint, getRequest(db)).Methods("GET")
	router.HandleFunc(confirmRequestEndpoint, confirmRequest(db)).Methods("POST")

	// Appointment routes
	router.HandleFunc(appointmentsEndpoint, getAppointments(db)).Methods("GET")
	router.HandleFunc(appointmentIDEndpoint, getAppointment(db)).Methods("GET")

	// Middleware chain, outermost first: request ID -> access log -> CORS -> JSON.
	handler := requestIDMiddleware(
		loggingMiddleware(
			enableCORS(cfg.allowedOrigins)(
				jsonContentTypeMiddleware(router))))

	// WI-05: a configured server, not http.ListenAndServe. Without these timeouts a
	// single slow client pins a goroutine and a DB connection indefinitely.
	srv := &http.Server{
		Addr:              ":" + cfg.port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	// WI-05: graceful shutdown. SIGTERM drains in-flight requests instead of
	// severing them mid-transaction.
	shutdownDone := make(chan struct{})
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		sig := <-sigCh
		logger.Info("shutdown signal received, draining", "signal", sig.String(), "grace", cfg.shutdownGrace.String())
		ctx, cancel := context.WithTimeout(context.Background(), cfg.shutdownGrace)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			logger.Error("graceful shutdown failed", "error", err)
		}
		close(shutdownDone)
	}()

	logger.Info("server listening", "port", cfg.port, "allowed_origins", cfg.allowedOrigins)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
	<-shutdownDone
	logger.Info("shutdown complete")
}

// ctxKey is unexported so no other package can collide with our context keys.
type ctxKey string

const requestIDKey ctxKey = "request_id"

// requestIDMiddleware assigns every request a correlation ID, honouring an
// inbound X-Request-Id when the caller supplies one. (WI-10)
func requestIDMiddleware(next http.Handler) http.Handler {
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
func loggingMiddleware(next http.Handler) http.Handler {
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
		if m := mux.CurrentRoute(r); m != nil {
			if tmpl, err := m.GetPathTemplate(); err == nil {
				route = tmpl
			}
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
func enableCORS(allowed []string) func(http.Handler) http.Handler {
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

func jsonContentTypeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set JSON Content-Type
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

// Auth Handler
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
		id := mux.Vars(r)["id"]
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
		id := mux.Vars(r)["id"]
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
		id := mux.Vars(r)["id"]
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
		rows, err := db.Query("SELECT id, donor_id, donor_name, last_donation, created_at FROM requests")
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
		id := mux.Vars(r)["id"]
		var req Request
		var lastDonation sql.NullString
		var createdAt sql.NullString
		err := db.QueryRow("SELECT id, donor_id, donor_name, last_donation, created_at FROM requests WHERE id = $1", id).Scan(&req.Id, &req.DonorId, &req.DonorName, &lastDonation, &createdAt)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
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

		// Fetch donor info - handle NULL last_donation
		var lastDonation sql.NullString
		err := db.QueryRow("SELECT full_name, last_donation FROM donors WHERE id = $1", req.DonorId).Scan(&req.DonorName, &lastDonation)
		if err != nil {
			http.Error(w, "Donor not found", http.StatusBadRequest)
			return
		}
		req.LastDonation = lastDonation.String

		err = db.QueryRow("INSERT INTO requests (donor_id, donor_name, last_donation) VALUES ($1, $2, $3) RETURNING id", req.DonorId, req.DonorName, lastDonation).Scan(&req.Id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(req)
	}
}

func confirmRequest(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
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
		err = tx.QueryRow("SELECT id, donor_id, donor_name FROM requests WHERE id = $1", id).Scan(&req.Id, &req.DonorId, &req.DonorName)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// 2. Create appointment
		var appt Appointment
		err = tx.QueryRow("INSERT INTO appointments (request_id, donor_id, donor_name, appointment_date) VALUES ($1, $2, $3, $4) RETURNING id",
			req.Id, req.DonorId, req.DonorName, payload.Date).Scan(&appt.Id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// 3. Delete the now-fulfilled request
		if _, err = tx.Exec("DELETE FROM requests WHERE id = $1", req.Id); err != nil {
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
		donorId := r.URL.Query().Get("donor_id")
		var rows *sql.Rows
		var err error

		if donorId != "" {
			rows, err = db.Query("SELECT id, request_id, donor_id, donor_name, appointment_date FROM appointments WHERE donor_id = $1", donorId)
		} else {
			rows, err = db.Query("SELECT id, request_id, donor_id, donor_name, appointment_date FROM appointments")
		}

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
		id := mux.Vars(r)["id"]
		donorId := r.URL.Query().Get("donor_id")

		// WI-02 — authorization bypass fix.
		//
		// This check used to run only `if donorId != ""`, i.e. only when the caller
		// volunteered the parameter. Omitting `?donor_id=` skipped it entirely and
		// returned any donor's appointment. The guard was opt-in by the attacker.
		//
		// Ownership is now unconditional. Until the backend issues and verifies its
		// own session (WI-17/WI-20), donor_id still arrives as a parameter, so this
		// is a *bypass* fix rather than full authorization: a caller may still assert
		// an identity. Deriving the donor from a verified session is Phase 1 work and
		// must not be assumed done here.
		if donorId == "" {
			http.Error(w, "donor_id is required", http.StatusBadRequest)
			return
		}
		dId, convErr := strconv.Atoi(donorId)
		if convErr != nil {
			http.Error(w, "donor_id must be an integer", http.StatusBadRequest)
			return
		}

		var appt Appointment
		err := db.QueryRow("SELECT id, request_id, donor_id, donor_name, appointment_date FROM appointments WHERE id = $1", id).Scan(&appt.Id, &appt.RequestId, &appt.DonorId, &appt.DonorName, &appt.AppointmentDate)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// 404, not 403: a non-owner must not learn that this appointment exists.
		if appt.DonorId != dId {
			slog.WarnContext(r.Context(), "appointment ownership check failed",
				slog.String("appointment_id", id),
				slog.Int("claimed_donor_id", dId),
			)
			w.WriteHeader(http.StatusNotFound)
			return
		}

		json.NewEncoder(w).Encode(appt)
	}
}
