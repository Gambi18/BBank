package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"bbank/internal/domain"
	"bbank/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AppointmentService struct {
	pool *pgxpool.Pool
	q    *store.Queries
	// centers resolves the slot a reschedule lands in (WI-24). A hard
	// dependency: a move that skips the grid is a move no capacity rule sees.
	centers *CenterService
}

func NewAppointmentService(pool *pgxpool.Pool, q *store.Queries, centers *CenterService) *AppointmentService {
	if centers == nil {
		panic("appointment service requires a centre service (FR-14)")
	}
	return &AppointmentService{pool: pool, q: q, centers: centers}
}

type ListAppointmentParams struct {
	Scope Scope
	// DonorFilter is a convenience narrowing for callers already scoped wider
	// than one donor. It is a separate field from Scope.OwnerID precisely so it
	// cannot be mistaken for identity: Scope comes from the token, this comes
	// from the query string, and both are ANDed. A filter can narrow a result
	// set; it can never widen one. That distinction is the A14 defect, stated
	// as a type rather than as a comment.
	DonorFilter *int64
	Limit       int32
	Offset      int32
}

func (s *AppointmentService) List(ctx context.Context, p ListAppointmentParams) ([]store.ListAppointmentsRow, int64, error) {
	rows, err := s.q.ListAppointments(ctx, store.ListAppointmentsParams{
		OwnerID: p.Scope.OwnerID, CenterID: p.Scope.CenterID, DonorFilter: p.DonorFilter,
		Lim: p.Limit, Off: p.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list appointments: %w", err)
	}
	total, err := s.q.CountAppointments(ctx, store.CountAppointmentsParams{
		OwnerID: p.Scope.OwnerID, CenterID: p.Scope.CenterID, DonorFilter: p.DonorFilter,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count appointments: %w", err)
	}
	return rows, total, nil
}

func (s *AppointmentService) Get(ctx context.Context, id int32) (store.GetAppointmentRow, error) {
	row, err := s.q.GetAppointment(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrNotFound
	}
	if err != nil {
		return row, fmt.Errorf("get appointment %d: %w", id, err)
	}
	return row, nil
}

// NoShowGrace is how long after a slot an appointment stays `scheduled` before
// the sweep marks it `no_show`.
//
// Generous on purpose: a donor stuck in traffic who arrives late is a donation,
// not an absence, and a mark that says otherwise is unfair and hard to undo.
// The cost of waiting is a slot that reads `scheduled` for a few more hours.
const NoShowGrace = 6 * time.Hour

// Cancel frees the slot (FR-11).
//
// The row is locked before its status is read, so the check and the write
// cannot straddle another transaction — two people cancelling at once, or a
// cancel racing a check-in, both resolve to one outcome rather than to a lost
// update.
func (s *AppointmentService) Cancel(ctx context.Context, id int32, reason string, permits func(ownerID, centerID int64) bool) error {
	return s.mutate(ctx, id, permits, func(q *store.Queries, appt store.GetAppointmentForUpdateRow) error {
		if err := domain.CanCancel(domain.AppointmentStatus(appt.Status), appt.ScheduledAt.Time, time.Now()); err != nil {
			return fmt.Errorf("%w: %s", ErrConflict, err.Error())
		}
		var r *string
		if reason = strings.TrimSpace(reason); reason != "" {
			r = &reason
		}
		return q.CancelAppointment(ctx, store.CancelAppointmentParams{ID: id, CancellationReason: r})
	})
}

// Reschedule moves an appointment to a new time before it starts (FR-11), into
// a real slot with a real seat (WI-24).
//
// Retried on a seat collision for the same reason approval is, and at the same
// level: a unique violation aborts its transaction, so there is no retrying the
// update from inside one.
func (s *AppointmentService) Reschedule(ctx context.Context, id int32, to time.Time, permits func(ownerID, centerID int64) bool) error {
	var err error
	for attempt := 0; attempt < AppointmentSeatRetries; attempt++ {
		err = s.rescheduleOnce(ctx, id, to, permits)
		if err == nil {
			return nil
		}
		if !isUniqueViolationOn(err, "appointments_one_per_slot_seat") {
			return err
		}
	}
	return fmt.Errorf("%w: that slot is full", ErrConflict)
}

func (s *AppointmentService) rescheduleOnce(ctx context.Context, id int32, to time.Time, permits func(ownerID, centerID int64) bool) error {
	return s.mutate(ctx, id, permits, func(q *store.Queries, appt store.GetAppointmentForUpdateRow) error {
		now := time.Now()
		if err := domain.CanReschedule(domain.AppointmentStatus(appt.Status), appt.ScheduledAt.Time, now); err != nil {
			return fmt.Errorf("%w: %s", ErrConflict, err.Error())
		}
		if err := domain.ValidateNewSlot(to, now); err != nil {
			return fmt.Errorf("%w: %s", ErrInvalid, err.Error())
		}

		// The destination must be a real slot on the centre's grid.
		//
		// Without this, an off-grid time — 09:07 with 30-minute slots — became
		// its own slot that no capacity rule could see, so a full 09:00 slot was
		// over-filled by rescheduling into it, and the appointment then vanished
		// from `SlotOccupancyOn`'s grouping so the slots endpoint over-reported
		// availability. The whole capacity mechanism rests on `scheduled_at`
		// being a slot start.
		slot, err := s.centers.SlotForWith(ctx, q, appt.CenterID, AtTime(to))
		if err != nil {
			return err
		}

		_, err = q.RescheduleAppointment(ctx, store.RescheduleAppointmentParams{
			ID: id, ScheduledAt: pgtype.Timestamptz{Time: slot, Valid: true},
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: every seat in that slot is taken", ErrConflict)
		}
		if err != nil {
			// A seat collision is a lost race the caller retries; anything else
			// the trigger raises — a seat above a capacity that was since
			// lowered — is a conflict the caller can act on, not a 500.
			if isUniqueViolationOn(err, "appointments_one_per_slot_seat") {
				return err
			}
			if isCheckViolation(err) {
				return fmt.Errorf("%w: that slot cannot take this appointment", ErrConflict)
			}
			return err
		}
		return nil
	})
}

// mutate is the shared lock -> authorize -> apply path. The three steps before
// the write are the ones that must not be skipped, so they live in one place.
func (s *AppointmentService) mutate(
	ctx context.Context,
	id int32,
	permits func(ownerID, centerID int64) bool,
	apply func(*store.Queries, store.GetAppointmentForUpdateRow) error,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed
	q := s.q.WithTx(tx)

	appt, err := q.GetAppointmentForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock appointment %d: %w", id, err)
	}
	// Authorized against the LOCKED row, so the row that was checked is the row
	// that gets written. 404 rather than 403: existence stays hidden.
	if permits != nil && !permits(int64(appt.DonorID), appt.CenterID) {
		return ErrNotFound
	}
	if err := apply(q, appt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SweepNoShows marks past, un-attended appointments (FR-13).
//
// Safe to run twice: it matches only rows still `scheduled` whose slot has
// passed by more than the grace period, so a second run finds nothing. It
// creates **no deferral** — not turning up is administrative, and recording it
// as a clinical deferral would put a mark on someone's record that no clinician
// made, and that the eligibility view would then act on.
func (s *AppointmentService) SweepNoShows(ctx context.Context) (int64, error) {
	n, err := s.q.SweepNoShows(ctx, pgtype.Interval{
		Microseconds: int64(NoShowGrace / time.Microsecond), Valid: true,
	})
	if err != nil {
		return 0, fmt.Errorf("sweep no-shows: %w", err)
	}
	return n, nil
}
