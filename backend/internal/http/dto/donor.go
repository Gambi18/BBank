// Package dto holds wire types. They are deliberately separate from both the
// domain types and the sqlc row types, so a database column rename is not an API
// break and an API field is not forced into the schema.
package dto

type Donor struct {
	ID                    int64   `json:"id"`
	Email                 string  `json:"email"`
	FullName              string  `json:"full_name"`
	DateOfBirth           *string `json:"date_of_birth,omitempty"`
	Gender                *string `json:"gender,omitempty"`
	BloodGroup            *string `json:"blood_group,omitempty"`
	Rhesus                *string `json:"rhesus,omitempty"`
	ContactPhone          string  `json:"contact_phone"`
	AddressLine           *string `json:"address_line,omitempty"`
	City                  *string `json:"city,omitempty"`
	Region                *string `json:"region,omitempty"`
	NationalID            *string `json:"national_id,omitempty"`
	EmergencyContactName  *string `json:"emergency_contact_name,omitempty"`
	EmergencyContactPhone *string `json:"emergency_contact_phone,omitempty"`
	TotalDonations        int32   `json:"total_donations"`
	Status                string  `json:"status"`
}

type DonorSummary struct {
	ID             int64   `json:"id"`
	Email          string  `json:"email"`
	FullName       string  `json:"full_name"`
	BloodGroup     *string `json:"blood_group,omitempty"`
	Rhesus         *string `json:"rhesus,omitempty"`
	ContactPhone   string  `json:"contact_phone"`
	TotalDonations int32   `json:"total_donations"`
	Status         string  `json:"status"`
}

// Eligibility mirrors the donor_eligibility view. Note there is no
// "last_donation" field a client can write: eligibility is computed from real
// donation records, never from a value a donor types in. That was the original
// defect (D4).
type Eligibility struct {
	DonorID             int64   `json:"donor_id"`
	FullName            string  `json:"full_name"`
	AgeYears            *int32  `json:"age_years,omitempty"`
	LastDonatedAt       *string `json:"last_donated_at,omitempty"`
	DonationsLast12m    int64   `json:"donations_last_12m"`
	PermanentlyDeferred bool    `json:"permanently_deferred"`
	DeferredUntil       *string `json:"deferred_until,omitempty"`
	NextEligibleOn      *string `json:"next_eligible_on,omitempty"`
	IsEligibleToday     bool    `json:"is_eligible_today"`
	Reason              string  `json:"reason"`
}
