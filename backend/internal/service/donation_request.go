package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	// elig is the FR-19 booking gate. It is a hard dependency rather than an
	// optional one: a nil check here would be a way to construct a service that
	// books without checking, and "the gate was not wired up in that path" is
	// how a safety requirement becomes a changelog entry.
	elig *EligibilityService
	// centers resolves the slot an approval books into, and refuses a centre
	// that is closed (WI-24). A hard dependency for the same reason.
	centers *CenterService
}

func NewDonationRequestService(pool *pgxpool.Pool, q *store.Queries, elig *EligibilityService, centers *CenterService) *DonationRequestService {
	if elig == nil {
		panic("donation request service requires an eligibility service (FR-19)")
	}
	if centers == nil {
		panic("donation request service requires a centre service (FR-14)")
	}
	return &DonationRequestService{pool: pool, q: q, elig: elig, centers: centers}
}

// AppointmentSeatRetries bounds the retry loop that resolves a seat collision.
//
// Two approvals into the same slot can both compute the same lowest free seat;
// the unique index lets exactly one commit and the other retries with the next
// seat. Each retry re-reads what is taken, so the loop shortens every time — it
// terminates because a slot has finitely many seats and a lost race means
// somebody else took one.
//
// The bound is the largest capacity the schema allows (`donation_centers_capacity`
// CHECK, 1-100) plus a little slack, so a full slot is reported as full rather
// than as a retry budget running out. A loop bounded by anything smaller would
// turn heavy contention into a spurious 409 on a slot that had room.
const AppointmentSeatRetries = 110

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

	// Procedure decides WHICH interval and annual cap apply. Empty means whole
	// blood, matching the column default.
	Procedure domain.Procedure

	// Override is set only when an admin is deliberately booking a permanently
	// deferred donor (FR-19). Nil for every ordinary booking.
	Override *PermanentDeferralOverride
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

	// Resolve the date and the procedure ONCE, here, so the booking that is
	// checked is the booking that is stored.
	//
	// The insert defaults an absent date to `CURRENT_DATE + 7`. When the gate
	// defaulted to `time.Now()` instead, a donor whose interval elapsed in three
	// days was refused for a request that would have been dated a week out and
	// was perfectly valid — the gate answering a question about a different day
	// from the one being booked.
	on := time.Now().AddDate(0, 0, defaultBookingLeadDays)
	if p.PreferredDate != nil {
		on = *p.PreferredDate
	}

	// A booking is for a day that has not happened.
	//
	// Nothing refused a past date before: the eligibility gate would evaluate it
	// perfectly happily — "were you eligible last Tuesday?" is a well-formed
	// question — and the request would sit in the queue for a day staff cannot
	// schedule. Compared on the DATE, not the instant, so "today" is still a
	// valid same-day booking.
	if startOfDay(on).Before(startOfDay(time.Now())) {
		return zero, fmt.Errorf("%w: a donation cannot be requested for a date in the past", ErrInvalid)
	}
	proc := p.Procedure
	if proc == "" {
		proc = domain.ProcedureWholeBlood
	}

	// FR-19, gate 1: the deferral and interval block, enforced HERE rather than
	// in the handler.
	//
	// Here, because the acceptance criterion is "the block cannot be bypassed by
	// calling the API directly" — a check in the handler is bypassed by any
	// second handler that calls this method, and there will be more of them
	// (WI-39 check-in, WI-44 collection). The service is the narrowest waist
	// every booking passes through.
	// FR-14: a closed centre takes no new bookings, and it has to be refused
	// HERE as well as at approval.
	//
	// Approval already refused one, via SlotFor. That was not enough: a request
	// is the donor-facing half of a booking, and letting one be raised against a
	// centre that is shut means the donor is told "we will confirm a date soon"
	// for a date that can never come, and staff inherit a queue of requests they
	// can only reject. The trigger on `appointments` is the backstop; this is the
	// answer a person can act on.
	//
	// Resolved through the centre service rather than by reading `is_active`
	// here, so "which centre" and "is it open" are one question with one answer —
	// the two-paths-guessing-differently failure this codebase keeps re-learning.
	centerID := int64(0)
	if p.CenterID != nil {
		centerID = *p.CenterID
	}
	if centerID != 0 {
		if _, active, err := s.centers.Scheduling(ctx, centerID); err != nil {
			return zero, err
		} else if !active {
			return zero, fmt.Errorf("%w: %s", ErrConflict, ErrCenterInactive)
		}
	}

	policies, err := s.elig.Policies(ctx)
	if err != nil {
		return zero, err
	}
	if err := s.gate(ctx, s.q, policies, p.DonorID, proc, on, p.Override); err != nil {
		return zero, err
	}

	date := pgtype.Date{Time: on, Valid: true}
	storedProc := store.DonationProcedure(proc)

	row, err := s.q.CreateDonationRequest(ctx, store.CreateDonationRequestParams{
		DonorID: int32(p.DonorID), CenterID: p.CenterID, PreferredDate: date,
		Procedure: &storedProc, Notes: p.Notes,
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

// startOfDay drops the clock, keeping the location — the same reasoning as the
// domain's: a booking is counted in whole days, so the answer must not depend on
// what time of day the request arrived.
func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// defaultBookingLeadDays mirrors the `CURRENT_DATE + 7` default in
// `CreateDonationRequest`. Named here because two places must agree on it: the
// gate decides for this date, and the insert stores it.
const defaultBookingLeadDays = 7

// gate applies FR-19 to one booking.
//
// The decision is made for the date being BOOKED, not for today. A donor whose
// 56-day interval elapses next Tuesday is booking for next Tuesday, and refusing
// them today would be refusing the commonest booking there is.
//
// A permanent deferral is the one failure an admin may override, and only with a
// reason. Everything else stands for every role: an admin who could wave away an
// interval window would be a way to bleed a donor early, and nothing in FR-19
// asks for that.
func (s *DonationRequestService) gate(
	ctx context.Context,
	q *store.Queries,
	policies *domain.Policies,
	donorID int64,
	proc domain.Procedure,
	on time.Time,
	override *PermanentDeferralOverride,
) error {
	// Validated BEFORE the decision is read, so an override nobody was entitled
	// to send is refused even when the donor turns out to be eligible anyway.
	// Checking it afterwards made the refusal depend on the donor's clinical
	// state: a staff member could send one and get a 201 with no trace, which is
	// the opposite of "refused, not ignored".
	if override != nil {
		if err := override.Validate(); err != nil {
			return fmt.Errorf("%w: %s", ErrInvalid, err.Error())
		}
	}

	decision, err := s.elig.EvaluateWith(ctx, q, policies, donorID, proc, on)
	if err != nil {
		return err
	}
	if decision.Eligible {
		if override != nil {
			// Nothing to override. Refusing rather than ignoring keeps "an
			// override was used" and "an override was needed" the same fact in
			// the audit trail.
			return fmt.Errorf("%w: there is no permanent deferral to override", ErrConflict)
		}
		return nil
	}

	if override == nil {
		return &ErrIneligible{Decision: decision}
	}

	// An override clears the permanent deferral and NOTHING ELSE.
	remaining := make([]domain.Failure, 0, len(decision.Failures))
	for _, f := range decision.Failures {
		if f.Criterion != domain.CriterionPermanentDeferral {
			remaining = append(remaining, f)
		}
	}

	if len(remaining) > 0 {
		// Still refused. The caller gets the criteria that REMAIN, with their
		// plain-language messages and their clearing date — returning a bare
		// conflict here threw away everything FR-17 and FR-08 require, and told
		// an admin only that their override had been unnecessary.
		still := decision
		still.Failures = remaining
		still.Eligible = false
		still.NextEligibleOn = domain.NextEligibleOn(still, on)
		return &ErrIneligible{Decision: still}
	}
	if len(decision.Failures) == 0 {
		return nil
	}

	// WI-27 replaces this with an audit_log row. Until then it is a structured
	// log line, because FR-19 requires the override to be audited and an override
	// with no trace is precisely what the requirement forbids.
	slog.WarnContext(ctx, "permanent deferral overridden",
		"event", "security.deferral_override",
		"donor_id", donorID,
		"actor_id", override.ActorID,
		"actor_role", string(override.ActorRole),
		"reason", override.Reason,
		"policy_version", decision.PolicyVersion,
	)
	return nil
}

// Approve sets status='approved' and creates the appointment in ONE transaction.
//
// Retried as a whole on a seat collision, and it has to be the WHOLE thing: a
// unique-violation aborts the transaction it happens in, so there is no
// retrying the insert from inside. Each attempt re-reads which seats are taken,
// so a lost race costs one round trip and the loop shortens every time.
//
// Rolling back also undoes the status transition, which is what makes retrying
// safe: a failed attempt leaves the request `pending`, exactly as it found it.
func (s *DonationRequestService) Approve(ctx context.Context, id int32, reviewerID int64, scheduled time.Time, permits func(ownerID, centerID int64) bool) (store.CreateAppointmentForRequestRow, error) {
	var appt store.CreateAppointmentForRequestRow

	// Resolved before any transaction opens. Reading policy can hit the
	// database, and asking for a second connection while holding a row lock is
	// how concurrent approvals deadlock the pool — see
	// EligibilityService.EvaluateWith.
	policies, err := s.elig.Policies(ctx)
	if err != nil {
		return appt, err
	}

	for attempt := 0; attempt < AppointmentSeatRetries; attempt++ {
		appt, err = s.approveOnce(ctx, id, reviewerID, scheduled, permits, policies)
		if err == nil {
			return appt, nil
		}
		if !isUniqueViolationOn(err, "appointments_one_per_slot_seat") {
			return appt, err
		}
		// Somebody took the seat between our reading it free and our committing.
		// That is the constraint doing its job; ask again.
	}
	return appt, fmt.Errorf("%w: that slot is full", ErrConflict)
}

func (s *DonationRequestService) approveOnce(
	ctx context.Context,
	id int32,
	reviewerID int64,
	scheduled time.Time,
	permits func(ownerID, centerID int64) bool,
	policies *domain.Policies,
) (store.CreateAppointmentForRequestRow, error) {
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

	// WI-24: the instant this appointment occupies. `scheduled_at` IS the slot,
	// so a requested time is snapped down to the centre's grid before anything
	// is written — otherwise two appointments five minutes apart would be
	// different slots and capacity would mean nothing. A closed centre is
	// refused here (FR-14).
	slot, err := s.centers.SlotForWith(ctx, q, req.CenterID, scheduled)
	if err != nil {
		return appt, err
	}

	// FR-19 again, and not redundantly.
	//
	// The gate at Create answered for the donor's PREFERRED date, using the facts
	// as they stood then. Approval is a different question: staff choose the
	// actual date, which may be weeks earlier, and the donor's record may have
	// changed in between — a screening on Tuesday can record a deferral that did
	// not exist when the request was raised. Approving without re-checking let
	// staff schedule an appointment three weeks inside the interval simply by
	// typing a date, which is the bypass the requirement names.
	//
	// No override here, deliberately: overriding is a decision made when the
	// booking is raised, by an admin, with a reason. An approval screen is not
	// where a permanent deferral should be waved through.
	if err := s.gate(ctx, q, policies, int64(req.DonorID), domain.Procedure(req.Procedure), slot, nil); err != nil {
		return appt, err
	}

	if err := q.ApproveDonationRequest(ctx, store.ApproveDonationRequestParams{ID: id, ReviewedBy: &reviewerID}); err != nil {
		return appt, fmt.Errorf("approve request %d: %w", id, err)
	}

	reqID := id
	appt, err = q.CreateAppointmentForRequest(ctx, store.CreateAppointmentForRequestParams{
		DonationRequestID: &reqID,
		DonorID:           req.DonorID,
		CenterID:          req.CenterID,
		ScheduledAt:       pgTime(slot),
		CreatedBy:         &reviewerID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// The seat query found nothing free. Not an error in the query — the
		// slot is full, which is a real answer a person can act on by choosing
		// another time.
		return appt, fmt.Errorf("%w: every seat in that slot is taken", ErrConflict)
	}
	if err != nil {
		if isUniqueViolation(err) {
			// Two different unique indexes can fire here. One-appointment-per-
			// request is final; a seat collision is retryable, so it is returned
			// as-is for Approve to recognise.
			if isUniqueViolationOn(err, "appointments_one_per_slot_seat") {
				return appt, err
			}
			return appt, fmt.Errorf("%w: an appointment already exists for this request", ErrConflict)
		}
		return appt, fmt.Errorf("create appointment: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		// A serialisation or unique failure can surface at COMMIT rather than at
		// the statement, so the same recognition applies here.
		if isUniqueViolationOn(err, "appointments_one_per_slot_seat") {
			return appt, err
		}
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
