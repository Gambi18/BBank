package dto

// DonationRequest is the wire shape of a donation request (WI-22).
//
// `last_donation` is retained from the legacy shape so the existing frontend
// keeps rendering; it reads `donor_profiles.legacy_last_donation`, a
// pre-migration free-text field that `WI-37` removes. It is NOT eligibility
// evidence — that comes from `donor_eligibility`, computed from real donation
// records (defect D4).
type DonationRequest struct {
	ID              int64   `json:"id"`
	DonorID         int64   `json:"donor_id"`
	DonorName       string  `json:"donor_name"`
	CenterID        int64   `json:"center_id"`
	Status          string  `json:"status"`
	PreferredDate   string  `json:"preferred_date"`
	CreatedAt       string  `json:"created_at"`
	ReviewedAt      *string `json:"reviewed_at,omitempty"`
	RejectionReason *string `json:"rejection_reason,omitempty"`
	Notes           *string `json:"notes,omitempty"`
	LastDonation    string  `json:"last_donation"`
}

// CreateDonationRequest is deliberately all-optional.
//
// A donor sends `{}`: their identity comes from the token, not the body. The
// `donor_id` field is honoured only for callers whose scope is wider than one
// donor — staff booking on someone's behalf — and is ignored entirely for a
// donor, so it cannot be used to raise a request in another person's name.
type CreateDonationRequest struct {
	DonorID       *int64  `json:"donor_id,omitempty"`
	CenterID      *int64  `json:"center_id,omitempty"`
	PreferredDate *string `json:"preferred_date,omitempty"`
	Notes         *string `json:"notes,omitempty"`

	// Procedure selects which interval and annual cap apply. Absent means whole
	// blood, matching the column default.
	Procedure *string `json:"procedure,omitempty"`

	// OverridePermanentDeferralReason books a permanently deferred donor anyway
	// (`FR-19`).
	//
	// A REASON, not a boolean, and that is the design. A boolean can be sent by
	// accident and says nothing afterwards; a reason cannot be supplied without
	// someone deciding what to write, and it is what the audit entry records.
	// The field is honoured only for an admin — for anyone else it is refused,
	// not ignored, so an attempt is visible rather than silent.
	OverridePermanentDeferralReason *string `json:"override_permanent_deferral_reason,omitempty"`
}

type ApproveDonationRequest struct {
	Date string `json:"date"`
}

// RejectDonationRequest carries a coded reason plus an optional note (FR-09).
// Free text is allowed alongside a code, never instead of one — otherwise the
// rejection-reason report (FR-61) becomes prose nobody can aggregate.
type RejectDonationRequest struct {
	Reason string `json:"reason"`
	Note   string `json:"note,omitempty"`
}

type RejectionReason struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Appointment is the wire shape of a scheduled appointment.
//
// Both `scheduled_at` (the full timestamp) and `scheduled_date` (projected into
// Africa/Douala) are sent. The legacy API sent only the date, which meant a
// client had to guess the timezone to display a time — and the timezone is an
// open question (schema Q6), so guessing was not safe.
type Appointment struct {
	ID            int64   `json:"id"`
	RequestID     int64   `json:"request_id"`
	DonorID       int64   `json:"donor_id"`
	DonorName     string  `json:"donor_name"`
	CenterID      int64   `json:"center_id"`
	Status        string  `json:"status"`
	ScheduledAt   string  `json:"scheduled_at"`
	ScheduledDate string  `json:"scheduled_date"`
	CheckedInAt   *string `json:"checked_in_at,omitempty"`
	CompletedAt   *string `json:"completed_at,omitempty"`
	CancelledAt   *string `json:"cancelled_at,omitempty"`

	// Retained so the existing frontend, which reads `appointment_date`, keeps
	// working through the migration. Same value as ScheduledDate.
	AppointmentDate string `json:"appointment_date"`
}

// CreateDonor is a registration. Note what is absent: blood group and rhesus.
// They are laboratory results (FR-21), not self-reported attributes — letting
// people type them is how "O+", "o" and " A " ended up in one column (D7).
type CreateDonor struct {
	Email       string  `json:"email"`
	Password    string  `json:"password"`
	FullName    string  `json:"full_name"`
	DateOfBirth string  `json:"date_of_birth"`
	Gender      string  `json:"gender"`
	Phone       string  `json:"contact_phone"`
	Address     *string `json:"address_line,omitempty"`

	// Honoured only for staff and admin; ignored on self-registration.
	BloodGroup *string `json:"blood_group,omitempty"`
	Rhesus     *string `json:"rhesus,omitempty"`
}

type UpdateDonor struct {
	FullName              string  `json:"full_name"`
	DateOfBirth           string  `json:"date_of_birth"`
	Gender                string  `json:"gender"`
	Phone                 string  `json:"contact_phone"`
	Address               *string `json:"address_line,omitempty"`
	City                  *string `json:"city,omitempty"`
	Region                *string `json:"region,omitempty"`
	NationalID            *string `json:"national_id,omitempty"`
	EmergencyContactName  *string `json:"emergency_contact_name,omitempty"`
	EmergencyContactPhone *string `json:"emergency_contact_phone,omitempty"`

	BloodGroup *string `json:"blood_group,omitempty"`
	Rhesus     *string `json:"rhesus,omitempty"`
}

// CancelAppointment carries an optional free-text reason. Optional because a
// donor cancelling their own slot owes nobody an explanation; staff cancelling
// on someone's behalf usually have one worth recording.
type CancelAppointment struct {
	Reason string `json:"reason,omitempty"`
}

// RescheduleAppointment takes an RFC3339 timestamp, not a bare date: the
// deployment timezone is an open question (schema Q6), and a date would make
// the server guess which 09:00 was meant.
type RescheduleAppointment struct {
	ScheduledAt string `json:"scheduled_at"`
}
