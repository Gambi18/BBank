package domain

import (
	"errors"
	"fmt"
	"time"
)

// AppointmentStatus mirrors the appointment_status enum.
type AppointmentStatus string

const (
	ApptScheduled AppointmentStatus = "scheduled"
	ApptCheckedIn AppointmentStatus = "checked_in"
	ApptCompleted AppointmentStatus = "completed"
	ApptNoShow    AppointmentStatus = "no_show"
	ApptCancelled AppointmentStatus = "cancelled"
	ApptDeferred  AppointmentStatus = "deferred"
)

var (
	ErrApptNotChangeable = errors.New("this appointment can no longer be changed")
	ErrRescheduleToPast  = errors.New("an appointment cannot be moved into the past")
	ErrNoticeTooShort    = errors.New("changes must be made before the appointment time")
)

// apptTransitions is the legal state machine (`FR-11`, `FR-13`).
//
// `checked_in` onward is deliberately absent: check-in arrives with `WI-39`,
// which must also enforce the `FR-19` deferral block, and pre-granting the
// transition here would let that requirement be skipped by whoever wires the
// endpoint. A transition nobody has declared is denied, not assumed harmless —
// the same rule the RBAC matrix follows.
var apptTransitions = map[AppointmentStatus][]AppointmentStatus{
	ApptScheduled: {ApptCancelled, ApptNoShow, ApptCheckedIn},
	// A checked-in donor who is turned away is deferred, not cancelled: the
	// distinction is clinical, and `deferrals` is where it is recorded.
	ApptCheckedIn: {ApptCompleted, ApptDeferred, ApptCancelled},
}

func CanTransitionAppointment(from, to AppointmentStatus) bool {
	for _, allowed := range apptTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

func EnsureAppointmentTransition(from, to AppointmentStatus) error {
	if !CanTransitionAppointment(from, to) {
		return fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, from, to)
	}
	return nil
}

// CanReschedule reports whether an appointment may be moved.
//
// Only from `scheduled`, and only before it starts. Once a donor has been
// checked in, the appointment is a clinical encounter in progress and moving it
// would rewrite when that encounter happened; after it has passed, the honest
// record is `no_show` or `completed`, not a quietly relocated slot.
func CanReschedule(status AppointmentStatus, scheduledAt, now time.Time) error {
	if status != ApptScheduled {
		return fmt.Errorf("%w: it is %s", ErrApptNotChangeable, status)
	}
	if !scheduledAt.After(now) {
		return ErrNoticeTooShort
	}
	return nil
}

// CanCancel is CanReschedule's sibling and deliberately has the same rule.
//
// Cancelling a slot that has already passed does not free anything — the
// capacity is spent either way — and it would erase the fact that nobody came,
// which is what the no-show sweep exists to record.
func CanCancel(status AppointmentStatus, scheduledAt, now time.Time) error {
	if status != ApptScheduled {
		return fmt.Errorf("%w: it is %s", ErrApptNotChangeable, status)
	}
	if !scheduledAt.After(now) {
		return ErrNoticeTooShort
	}
	return nil
}

// ValidateNewSlot checks a proposed reschedule target.
func ValidateNewSlot(newTime, now time.Time) error {
	if !newTime.After(now) {
		return ErrRescheduleToPast
	}
	return nil
}
