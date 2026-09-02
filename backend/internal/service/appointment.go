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
}

func NewAppointmentService(pool *pgxpool.Pool, q *store.Queries) *AppointmentService {
	return &AppointmentService{pool: pool, q: q}
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

// Reschedule moves an appointment to a new time before it starts (FR-11).
func (s *AppointmentService) Reschedule(ctx context.Context, id int32, to time.Time, permits func(ownerID, centerID int64) bool) error {
	return s.mutate(ctx, id, permits, func(q *store.Queries, appt store.GetAppointmentForUpdateRow) error {
		now := time.Now()
		if err := domain.CanReschedule(domain.AppointmentStatus(appt.Status), appt.ScheduledAt.Time, now); err != nil {
			return fmt.Errorf("%w: %s", ErrConflict, err.Error())
		}
		if err := domain.ValidateNewSlot(to, now); err != nil {
			return fmt.Errorf("%w: %s", ErrInvalid, err.Error())
		}
		_, err := q.RescheduleAppointment(ctx, store.RescheduleAppointmentParams{
			ID: id, ScheduledAt: pgtype.Timestamptz{Time: to, Valid: true},
		})
		return err
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
