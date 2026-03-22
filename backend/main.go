package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"os"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
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
	Password     string `json:"password"`
	LastDonation string `json:"last_donation"`
}

type Admin struct {
	Id           int    `json:"id"`
	Organization string `json:"organization"`
	NPI          string `json:"npi"`
	Email        string `json:"email"`
	Password     string `json:"password"`
}

type Request struct {
	Id        int    `json:"id"`
	DonorId   int    `json:"donor_id"`
	AdminId   int    `json:"admin_id"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type Appointment struct {
	Id              int    `json:"id"`
	RequestId       int    `json:"request_id"`
	DonorId         int    `json:"donor_id"`
	AdminId         int    `json:"admin_id"`
	AppointmentDate string `json:"appointment_date"`
	Status          string `json:"status"`
}

const (
	donorsEndpoint       = "/api/go/donors"
	donorIDEndpoint      = "/api/go/donors/{id}"
	adminsEndpoint       = "/api/go/admins"
	adminIDEndpoint      = "/api/go/admins/{id}"
	requestsEndpoint      = "/api/go/requests"
	requestIDEndpoint     = "/api/go/requests/{id}"
	confirmRequestEndpoint = "/api/go/requests/{id}/confirm"
	appointmentsEndpoint  = "/api/go/appointments"
	appointmentIDEndpoint = "/api/go/appointments/{id}"
)

func main() {

	// Connect to the database
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://admin:2323@db:5433/bbank?sslmode=disable" // fallback for local go run
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Connected using DSN:", dsn) // ← add this for debugging

	err = db.Ping()
	if err != nil {
		log.Fatal(err)
	} else {
		fmt.Printf("successful")
	}
	defer db.Close()

	// Create tables
	queries := []string{
		`CREATE TABLE IF NOT EXISTS donors (
			id SERIAL PRIMARY KEY,
			full_name TEXT,
			email TEXT UNIQUE,
			dob DATE,
			gender TEXT,
			blood_group TEXT,
			rhesus TEXT,
			contact TEXT,
			address TEXT,
			password TEXT,
			last_donation DATE
		)`,
		`CREATE TABLE IF NOT EXISTS admins (
			id SERIAL PRIMARY KEY,
			organization TEXT,
			npi TEXT,
			email TEXT UNIQUE,
			password TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS requests (
			id SERIAL PRIMARY KEY,
			donor_id INTEGER REFERENCES donors(id),
			admin_id INTEGER REFERENCES admins(id),
			status TEXT DEFAULT 'pending',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS appointments (
			id SERIAL PRIMARY KEY,
			request_id INTEGER REFERENCES requests(id),
			donor_id INTEGER REFERENCES donors(id),
			admin_id INTEGER REFERENCES admins(id),
			appointment_date DATE,
			status TEXT DEFAULT 'scheduled'
		)`,
	}

	for _, q := range queries {
		_, err = db.Exec(q)
		if err != nil {
			log.Fatal(err)
		}
	}

	// Create router
	router := mux.NewRouter()

	// Donor routes
	router.HandleFunc(donorsEndpoint, getDonors(db)).Methods("GET")
	router.HandleFunc(donorsEndpoint, createDonor(db)).Methods("POST")
	router.HandleFunc(donorIDEndpoint, getDonor(db)).Methods("GET")
	router.HandleFunc(donorIDEndpoint, updateDonor(db)).Methods("PUT")
	router.HandleFunc(donorIDEndpoint, deleteDonor(db)).Methods("DELETE")

	// Admin routes
	router.HandleFunc(adminsEndpoint, getAdmins(db)).Methods("GET")
	router.HandleFunc(adminsEndpoint, createAdmin(db)).Methods("POST")
	router.HandleFunc(adminIDEndpoint, getAdmin(db)).Methods("GET")
	router.HandleFunc(adminIDEndpoint, updateAdmin(db)).Methods("PUT")
	router.HandleFunc(adminIDEndpoint, deleteAdmin(db)).Methods("DELETE")

	// Request routes
	router.HandleFunc(requestsEndpoint, getRequests(db)).Methods("GET")
	router.HandleFunc(requestsEndpoint, createRequest(db)).Methods("POST")
	router.HandleFunc(requestIDEndpoint, getRequest(db)).Methods("GET")
	router.HandleFunc(confirmRequestEndpoint, confirmRequest(db)).Methods("POST")

	// Appointment routes
	router.HandleFunc(appointmentsEndpoint, getAppointments(db)).Methods("GET")
	router.HandleFunc(appointmentIDEndpoint, getAppointment(db)).Methods("GET")

	// Wrap router with CORS and JSON content type middlewares
	enhancedRouter := enableCORS(jsonContentTypeMiddleware(router))

	// Start server
	log.Fatal(http.ListenAndServe(":8000", enhancedRouter))
}

func enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func jsonContentTypeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set JSON Content-Type
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

// Donor Handlers
func getDonors(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query("SELECT id, full_name, email, dob, gender, blood_group, rhesus, contact, address, password, last_donation FROM donors")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		donors := []Donor{}
		for rows.Next() {
			var d Donor
			err := rows.Scan(&d.Id, &d.FullName, &d.Email, &d.DOB, &d.Gender, &d.BloodGroup, &d.Rhesus, &d.Contact, &d.Address, &d.Password, &d.LastDonation)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			donors = append(donors, d)
		}
		json.NewEncoder(w).Encode(donors)
	}
}

func getDonor(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		var d Donor
		err := db.QueryRow("SELECT id, full_name, email, dob, gender, blood_group, rhesus, contact, address, password, last_donation FROM donors WHERE id = $1", id).
			Scan(&d.Id, &d.FullName, &d.Email, &d.DOB, &d.Gender, &d.BloodGroup, &d.Rhesus, &d.Contact, &d.Address, &d.Password, &d.LastDonation)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
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
		err := db.QueryRow("INSERT INTO donors (full_name, email, dob, gender, blood_group, rhesus, contact, address, password, last_donation) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id",
			d.FullName, d.Email, d.DOB, d.Gender, d.BloodGroup, d.Rhesus, d.Contact, d.Address, d.Password, d.LastDonation).Scan(&d.Id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
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
		_, err := db.Exec("UPDATE donors SET full_name=$1, email=$2, dob=$3, gender=$4, blood_group=$5, rhesus=$6, contact=$7, address=$8, password=$9, last_donation=$10 WHERE id=$11",
			d.FullName, d.Email, d.DOB, d.Gender, d.BloodGroup, d.Rhesus, d.Contact, d.Address, d.Password, d.LastDonation, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Ensure the returned ID is set in the donor object
		fmt.Sscanf(id, "%d", &d.Id)
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

// Admin Handlers
func getAdmins(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query("SELECT id, organization, npi, email, password FROM admins")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		admins := []Admin{}
		for rows.Next() {
			var a Admin
			if err := rows.Scan(&a.Id, &a.Organization, &a.NPI, &a.Email, &a.Password); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			admins = append(admins, a)
		}
		json.NewEncoder(w).Encode(admins)
	}
}

func getAdmin(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		var a Admin
		err := db.QueryRow("SELECT id, organization, npi, email, password FROM admins WHERE id = $1", id).Scan(&a.Id, &a.Organization, &a.NPI, &a.Email, &a.Password)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(a)
	}
}

func createAdmin(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var a Admin
		if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		err := db.QueryRow("INSERT INTO admins (organization, npi, email, password) VALUES ($1, $2, $3, $4) RETURNING id", a.Organization, a.NPI, a.Email, a.Password).Scan(&a.Id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(a)
	}
}

func updateAdmin(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		var a Admin
		if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_, err := db.Exec("UPDATE admins SET organization=$1, npi=$2, email=$3, password=$4 WHERE id=$5", a.Organization, a.NPI, a.Email, a.Password, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Sscanf(id, "%d", &a.Id)
		json.NewEncoder(w).Encode(a)
	}
}

func deleteAdmin(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		_, err := db.Exec("DELETE FROM admins WHERE id = $1", id)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode("Admin Deleted")
	}
}

// Request Handlers
func getRequests(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query("SELECT id, donor_id, admin_id, status, created_at FROM requests")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		requests := []Request{}
		for rows.Next() {
			var req Request
			if err := rows.Scan(&req.Id, &req.DonorId, &req.AdminId, &req.Status, &req.CreatedAt); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			requests = append(requests, req)
		}
		json.NewEncoder(w).Encode(requests)
	}
}

func getRequest(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		var req Request
		err := db.QueryRow("SELECT id, donor_id, admin_id, status, created_at FROM requests WHERE id = $1", id).Scan(&req.Id, &req.DonorId, &req.AdminId, &req.Status, &req.CreatedAt)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
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
		err := db.QueryRow("INSERT INTO requests (donor_id, admin_id) VALUES ($1, $2) RETURNING id", req.DonorId, req.AdminId).Scan(&req.Id)
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

		// 1. Get request details
		var req Request
		err := db.QueryRow("SELECT id, donor_id, admin_id FROM requests WHERE id = $1", id).Scan(&req.Id, &req.DonorId, &req.AdminId)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// 2. Update request status
		_, err = db.Exec("UPDATE requests SET status = 'confirmed' WHERE id = $1", id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// 3. Create appointment
		var appt Appointment
		err = db.QueryRow("INSERT INTO appointments (request_id, donor_id, admin_id, appointment_date) VALUES ($1, $2, $3, $4) RETURNING id",
			req.Id, req.DonorId, req.AdminId, payload.Date).Scan(&appt.Id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		appt.RequestId = req.Id
		appt.DonorId = req.DonorId
		appt.AdminId = req.AdminId
		appt.AppointmentDate = payload.Date
		appt.Status = "scheduled"

		json.NewEncoder(w).Encode(appt)
	}
}

// Appointment Handlers
func getAppointments(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query("SELECT id, request_id, donor_id, admin_id, appointment_date, status FROM appointments")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		appointments := []Appointment{}
		for rows.Next() {
			var appt Appointment
			if err := rows.Scan(&appt.Id, &appt.RequestId, &appt.DonorId, &appt.AdminId, &appt.AppointmentDate, &appt.Status); err != nil {
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
		var appt Appointment
		err := db.QueryRow("SELECT id, request_id, donor_id, admin_id, appointment_date, status FROM appointments WHERE id = $1", id).Scan(&appt.Id, &appt.RequestId, &appt.DonorId, &appt.AdminId, &appt.AppointmentDate, &appt.Status)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(appt)
	}
}
