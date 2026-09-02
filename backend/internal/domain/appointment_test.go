package domain

import (
	"errors"
	"testing"
	"time"
)

func TestAppointmentTransitions(t *testing.T) {
	legal := []struct{ from, to AppointmentStatus }{
		{ApptScheduled, ApptCancelled},
		{ApptScheduled, ApptNoShow},
		{ApptScheduled, ApptCheckedIn},
		{ApptCheckedIn, ApptCompleted},
		{ApptCheckedIn, ApptDeferred},
		{ApptCheckedIn, ApptCancelled},
	}
	for _, c := range legal {
		if !CanTransitionAppointment(c.from, c.to) {
			t.Errorf("%s -> %s should be allowed", c.from, c.to)
		}
	}

	illegal := []struct {
		name     string
		from, to AppointmentStatus
	}{
		// Every decided state is terminal. Reviving one would let a completed
		// donation be "cancelled" after the blood is already in a bag.
		{"un-completing", ApptCompleted, ApptScheduled},
		{"cancelling a completed donation", ApptCompleted, ApptCancelled},
		{"reviving a cancellation", ApptCancelled, ApptScheduled},
		{"checking in after cancelling", ApptCancelled, ApptCheckedIn},
		{"un-marking a no-show", ApptNoShow, ApptScheduled},
		{"checking in a no-show", ApptNoShow, ApptCheckedIn},
		{"reviving a deferral", ApptDeferred, ApptScheduled},
		// Skipping check-in would record a donation from someone who was never
		// screened — the safety step the whole flow exists around.
		{"completing without checking in", ApptScheduled, ApptCompleted},
		{"deferring without checking in", ApptScheduled, ApptDeferred},
		{"unknown source state", "", ApptCancelled},
		{"unknown target state", ApptScheduled, "teleported"},
	}
	for _, c := range illegal {
		if CanTransitionAppointment(c.from, c.to) {
			t.Errorf("%s (%s -> %s) should be forbidden", c.name, c.from, c.to)
		}
		if err := EnsureAppointmentTransition(c.from, c.to); !errors.Is(err, ErrIllegalTransition) {
			t.Errorf("%s: EnsureAppointmentTransition = %v, want ErrIllegalTransition", c.name, err)
		}
	}
}

// check_in is deliberately absent from what staff may *drive* until WI-39,
// which must pair it with the FR-19 deferral block. This asserts the omission
// is intentional, so restoring it silently is a visible change.
func TestCheckInIsReachableOnlyFromScheduled(t *testing.T) {
	for _, from := range []AppointmentStatus{ApptCompleted, ApptCancelled, ApptNoShow, ApptDeferred, ApptCheckedIn} {
		if CanTransitionAppointment(from, ApptCheckedIn) {
			t.Errorf("check-in from %s should be forbidden", from)
		}
	}
	if !CanTransitionAppointment(ApptScheduled, ApptCheckedIn) {
		t.Error("check-in from scheduled should be the one legal path")
	}
}

func TestCanCancelAndReschedule(t *testing.T) {
	now := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	future := now.Add(48 * time.Hour)
	past := now.Add(-48 * time.Hour)

	if err := CanCancel(ApptScheduled, future, now); err != nil {
		t.Errorf("cancelling a future scheduled appointment: %v", err)
	}
	if err := CanReschedule(ApptScheduled, future, now); err != nil {
		t.Errorf("rescheduling a future scheduled appointment: %v", err)
	}

	// Once a donor is checked in the appointment is an encounter in progress;
	// moving or cancelling it would rewrite when that encounter happened.
	for _, st := range []AppointmentStatus{ApptCheckedIn, ApptCompleted, ApptCancelled, ApptNoShow, ApptDeferred} {
		if err := CanCancel(st, future, now); !errors.Is(err, ErrApptNotChangeable) {
			t.Errorf("cancelling a %s appointment = %v, want ErrApptNotChangeable", st, err)
		}
		if err := CanReschedule(st, future, now); !errors.Is(err, ErrApptNotChangeable) {
			t.Errorf("rescheduling a %s appointment = %v, want ErrApptNotChangeable", st, err)
		}
	}

	// A slot that has already passed cannot be freed — the capacity is spent —
	// and cancelling it would erase the fact that nobody came.
	if err := CanCancel(ApptScheduled, past, now); !errors.Is(err, ErrNoticeTooShort) {
		t.Errorf("cancelling a past appointment = %v, want ErrNoticeTooShort", err)
	}
	if err := CanReschedule(ApptScheduled, past, now); !errors.Is(err, ErrNoticeTooShort) {
		t.Errorf("rescheduling a past appointment = %v, want ErrNoticeTooShort", err)
	}
	// Exactly now is not "before".
	if err := CanCancel(ApptScheduled, now, now); !errors.Is(err, ErrNoticeTooShort) {
		t.Errorf("cancelling at the appointment time = %v, want ErrNoticeTooShort", err)
	}
}

func TestValidateNewSlotRefusesThePast(t *testing.T) {
	now := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	if err := ValidateNewSlot(now.Add(time.Hour), now); err != nil {
		t.Errorf("a future slot was refused: %v", err)
	}
	for _, at := range []time.Time{now, now.Add(-time.Second), now.Add(-100 * time.Hour)} {
		if err := ValidateNewSlot(at, now); !errors.Is(err, ErrRescheduleToPast) {
			t.Errorf("ValidateNewSlot(%v) = %v, want ErrRescheduleToPast", at, err)
		}
	}
}
