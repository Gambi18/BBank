package dto

import "encoding/json"

// Center is the wire shape of a donation centre (WI-24).
//
// No PHI, which is what lets the directory be public (TRD §6.5). Everything here
// is the sort of thing a centre puts on a poster.
type Center struct {
	ID          int64   `json:"id"`
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	AddressLine string  `json:"address_line"`
	City        string  `json:"city"`
	Region      string  `json:"region"`
	Phone       *string `json:"phone,omitempty"`
	Email       *string `json:"email,omitempty"`

	CapacityPerSlot int `json:"capacity_per_slot"`
	SlotMinutes     int `json:"slot_minutes"`
	// OpeningHours is `{"mon": [["08:00","12:00"], ["13:00","16:30"]], …}`.
	// A day with no entry is closed; an empty object means nobody has configured
	// hours, which is a different thing — see service.LegacyOpeningClock.
	OpeningHours json.RawMessage `json:"opening_hours"`
	Timezone     string          `json:"timezone"`
	IsActive     bool            `json:"is_active"`
}

// Slot is one bookable slot and how full it is.
type Slot struct {
	StartsAt  string `json:"starts_at"`
	Capacity  int    `json:"capacity"`
	Booked    int    `json:"booked"`
	Available int    `json:"available"`
}

// WriteCenter is the create and update body.
//
// Every field is a pointer so PATCH can mean "change what I sent": the update
// query COALESCEs an absent field to its stored value. `code` is absent
// entirely — it is the centre's stable identifier, printed on labels and quoted
// in reports, and a centre needing a different code is a different centre.
type WriteCenter struct {
	Code            *string         `json:"code,omitempty"`
	Name            *string         `json:"name,omitempty"`
	AddressLine     *string         `json:"address_line,omitempty"`
	City            *string         `json:"city,omitempty"`
	Region          *string         `json:"region,omitempty"`
	Phone           *string         `json:"phone,omitempty"`
	Email           *string         `json:"email,omitempty"`
	CapacityPerSlot *int16          `json:"capacity_per_slot,omitempty"`
	SlotMinutes     *int16          `json:"slot_minutes,omitempty"`
	OpeningHours    json.RawMessage `json:"opening_hours,omitempty"`
	Timezone        *string         `json:"timezone,omitempty"`
	// IsActive is how a centre is deactivated and reopened. Deactivating stops
	// new bookings and leaves every existing appointment alone (FR-14).
	IsActive *bool `json:"is_active,omitempty"`
}
