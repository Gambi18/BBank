package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"bbank/internal/domain"
	"bbank/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Sentinel errors the handlers map onto status codes. They exist so the service
// never needs to know what an HTTP status is, and the handler never needs to
// inspect a driver error to decide what happened.
var (
	// ErrConflict is a well-formed request the world said no to (TRD §6.2:
	// 409, not 400). Approving an already-approved request is the example.
	ErrConflict = errors.New("conflict")
	// ErrInvalid is a validation failure — 422.
	ErrInvalid = errors.New("invalid")
)

// Scope is the authorization filter, passed in explicitly.
//
// The legacy code built this by concatenating " AND r.donor_id = $1" onto a
// query string. It worked, but it put the security rule in string handling: a
// missing clause was a wider result set, not a compile error. Here a nil field
// means "not narrowed on this axis", the handler is the only place that decides
// what the caller's scope is, and the service cannot accidentally widen it.
type Scope struct {
	OwnerID  *int64
	CenterID *int64
}

type DonationRequestService struct {
	pool *pgxpool.Pool
	q    *store.Queries
}

func NewDonationRequestService(pool *pgxpool.Pool, q *store.Queries) *DonationRequestService {
	return &DonationRequestService{pool: pool, q: q}
}

type ListRequestParams struct {
	Scope  Scope
	Status *string
	Limit  int32
	Offset int32
}

func (s *DonationRequestService) List(ctx context.Context, p ListRequestParams) ([]store.ListDonationRequestsRow, int64, error) {
	var status *store.DonationRequestStatus
	if p.Status != nil && *p.Status != "" {
		v := store.DonationRequestStatus(*p.Status)
		status = &v
	}
	rows, err := s.q.ListDonationRequests(ctx, store.ListDonationRequestsParams{
		Status: status, OwnerID: p.Scope.OwnerID, CenterID: p.Scope.CenterID,
		Lim: p.Limit, Off: p.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list donation requests: %w", err)
	}
	total, err := s.q.CountDonationRequests(ctx, store.CountDonationRequestsParams{
		Status: status, OwnerID: p.Scope.OwnerID, CenterID: p.Scope.CenterID,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count donation requests: %w", err)
	}
	return rows, total, nil
}

func (s *DonationRequestService) Get(ctx context.Context, id int32) (store.GetDonationRequestRow, error) {
	row, err := s.q.GetDonationRequest(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrNotFound
	}
	if err != nil {
		return row, fmt.Errorf("get donation request %d: %w", id, err)
	}
	return row, nil
}

type CreateRequestParams struct {
	DonorID       int64
	CenterID      *int64
	PreferredDate *time.Time
	Notes         *string
}

// Create books a donation request.
//
// The open-request check is here as well as in the database (the partial unique
// index `donation_requests_one_open_per_donor`). The index is the thing that
// actually holds under concurrency; this exists so the ordinary case is a clean
// 409 with a sentence a person can read, rather than a constraint violation
// surfacing as a 500.
func (s *DonationRequestService) Create(ctx context.Context, p CreateRequestParams) (store.CreateDonationRequestRow, error) {
	var zero store.CreateDonationRequestRow

	open, err := s.q.HasOpenDonationRequest(ctx, int32(p.DonorID))
	if err != nil {
		return zero, fmt.Errorf("check open request: %w", err)
	}
	if open {
		return zero, fmt.Errorf("%w: this donor already has a pending request", ErrConflict)
	}

	var date pgtype.Date
	if p.PreferredDate != nil {
		date = pgtype.Date{Time: *p.PreferredDate, Valid: true}
	}

	row, err := s.q.CreateDonationRequest(ctx, store.CreateDonationRequestParams{
		DonorID: int32(p.DonorID), CenterID: p.CenterID, PreferredDate: date, Notes: p.Notes,
	})
	if err != nil {
		// The unique index is the real guard; losing the race is still a 409.
		if isUniqueViolation(err) {
			return zero, fmt.Errorf("%w: this donor already has a pending request", ErrConflict)
		}
		if isForeignKeyViolation(err) {
			return zero, fmt.Errorf("%w: no donor profile with that id", ErrInvalid)
		}
		return zero, fmt.Errorf("create donation request: %w", err)
	}
	return row, nil
}

// Approve sets status='approved' and creates the appointment in ONE transaction.
//
// This is the fix for A8/TD-05. The original `confirm` deleted the request row
// after creating the appointment, which destroyed the link back to who asked and
// when — see the quarantined rows in migration_rejects. The row now survives
// with `status='approved'` and a `reviewed_by`/`reviewed_at` trail.
//
// The row is locked with FOR UPDATE before its status is checked. Without the
// lock, two staff approving simultaneously both read 'pending', both pass the
// transition check, and the second insert dies on the UNIQUE constraint over
// appointments.donation_request_id — a 500 for what is really a 409.
func (s *DonationRequestService) Approve(ctx context.Context, id int32, reviewerID int64, scheduled time.Time, permits func(ownerID, centerID int64) bool) (store.CreateAppointmentForRequestRow, error) {
	var appt store.CreateAppointmentForRequestRow

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return appt, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed
	q := s.q.WithTx(tx)

	req, err := q.GetDonationRequestForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return appt, ErrNotFound
	}
	if err != nil {
		return appt, fmt.Errorf("lock request %d: %w", id, err)
	}

	// Ownership is evaluated on the locked row, inside the transaction, so the
	// row that was authorized is the row that gets written. Answering 404 rather
	// than 403 keeps the existence of another centre's request hidden.
	if permits != nil && !permits(int64(req.DonorID), req.CenterID) {
		return appt, ErrNotFound
	}

	if err := domain.EnsureTransition(domain.RequestStatus(req.Status), domain.RequestApproved); err != nil {
		return appt, fmt.Errorf("%w: request is %s", ErrConflict, req.Status)
	}

	if err := q.ApproveDonationRequest(ctx, store.ApproveDonationRequestParams{ID: id, ReviewedBy: &reviewerID}); err != nil {
		return appt, fmt.Errorf("approve request %d: %w", id, err)
	}

	reqID := id
	appt, err = q.CreateAppointmentForRequest(ctx, store.CreateAppointmentForRequestParams{
		DonationRequestID: &reqID,
		DonorID:           req.DonorID,
		CenterID:          req.CenterID,
		ScheduledDate:     pgtype.Date{Time: scheduled, Valid: true},
		CreatedBy:         &reviewerID,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return appt, fmt.Errorf("%w: an appointment already exists for this request", ErrConflict)
		}
		return appt, fmt.Errorf("create appointment: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return appt, fmt.Errorf("commit: %w", err)
	}
	return appt, nil
}

// Reject records the decision with a reason from the controlled list (FR-09).
func (s *DonationRequestService) Reject(ctx context.Context, id int32, reviewerID int64, reason domain.RejectionReason, note string, permits func(ownerID, centerID int64) bool) error {
	if err := domain.ValidateRejection(reason, note); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalid, err.Error())
	}
	return s.decide(ctx, id, permits, domain.RequestRejected, func(q *store.Queries) error {
		reasonStr := string(reason)
		var notePtr *string
		if note != "" {
			notePtr = &note
		}
		return q.RejectDonationRequest(ctx, store.RejectDonationRequestParams{
			ID: id, RejectionReason: &reasonStr, Notes: notePtr, ReviewedBy: &reviewerID,
		})
	})
}

// Cancel is the donor's own withdrawal, and staff's on their behalf (FR-11).
func (s *DonationRequestService) Cancel(ctx context.Context, id int32, actorID int64, permits func(ownerID, centerID int64) bool) error {
	return s.decide(ctx, id, permits, domain.RequestCancelled, func(q *store.Queries) error {
		return q.CancelDonationRequest(ctx, store.CancelDonationRequestParams{ID: id, ReviewedBy: &actorID})
	})
}

// decide is the shared lock -> authorize -> check-transition -> write path.
// Reject and Cancel differ only in the final statement, and the three steps
// before it are exactly the ones that must not be skipped.
func (s *DonationRequestService) decide(
	ctx context.Context,
	id int32,
	permits func(ownerID, centerID int64) bool,
	to domain.RequestStatus,
	write func(*store.Queries) error,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed
	q := s.q.WithTx(tx)

	req, err := q.GetDonationRequestForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock request %d: %w", id, err)
	}
	if permits != nil && !permits(int64(req.DonorID), req.CenterID) {
		return ErrNotFound
	}
	if err := domain.EnsureTransition(domain.RequestStatus(req.Status), to); err != nil {
		return fmt.Errorf("%w: request is %s", ErrConflict, req.Status)
	}
	if err := write(q); err != nil {
		return fmt.Errorf("update request %d: %w", id, err)
	}
	return tx.Commit(ctx)
}
