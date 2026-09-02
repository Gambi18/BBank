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
)

// EligibilityService answers "may this donor donate, and if not, why and when".
//
// It is the ONE place that answer is produced. `FR-19` requires the block to be
// enforced server-side, and a rule enforced in two places is a rule enforced in
// neither — the booking gate, check-in (`WI-39`) and collection (`WI-44`) all
// call this rather than each re-deriving it.
type EligibilityService struct {
	q      *store.Queries
	policy *PolicyService
}

func NewEligibilityService(q *store.Queries, policy *PolicyService) *EligibilityService {
	return &EligibilityService{q: q, policy: policy}
}

// Evaluate decides whether a donor may donate on `on`.
//
// `on` is the date the decision is FOR, which for a booking is the preferred
// date rather than today. Asking "is this donor eligible today?" when they are
// booking three weeks out would refuse a donor whose interval elapses next week
// — the commonest booking there is.
func (s *EligibilityService) Evaluate(ctx context.Context, donorID int64, proc domain.Procedure, on time.Time) (domain.Decision, error) {
	policies, err := s.Policies(ctx)
	if err != nil {
		return domain.Decision{}, err
	}
	return s.EvaluateWith(ctx, s.q, policies, donorID, proc, on)
}

// Policies resolves the snapshot a decision will be made against.
//
// Separate from Evaluate so a caller inside a transaction can resolve it
// BEFORE opening one. That ordering is not a style choice: reading policy needs
// a pool connection, and asking for one while holding a transaction — and a row
// lock — is how N concurrent callers deadlock a pool of fewer than N
// connections. See EvaluateWith.
func (s *EligibilityService) Policies(ctx context.Context) (*domain.Policies, error) {
	return s.policy.Current(ctx)
}

// EvaluateWith decides using the QUERIES THE CALLER HANDS IT.
//
// A caller inside a transaction passes its own `q` so the facts are read on the
// connection it already holds. The first version of the approval gate called
// `Evaluate`, which uses the service's pool-wide queries — so each approving
// goroutine held a transaction and a `FOR UPDATE` lock while waiting for a
// second connection from a pool that every other goroutine was also holding.
// Eight concurrent approvals hung until the test timed out at ten minutes.
//
// The policy snapshot is a parameter for the same reason: resolving it can hit
// the database, so it must already be in hand before the transaction opens.
func (s *EligibilityService) EvaluateWith(
	ctx context.Context,
	q *store.Queries,
	policies *domain.Policies,
	donorID int64,
	proc domain.Procedure,
	on time.Time,
) (domain.Decision, error) {
	facts, err := s.FactsWith(ctx, q, donorID, proc)
	if err != nil {
		return domain.Decision{}, err
	}
	d, err := domain.EvaluateEligibility(facts, policies, on)
	if err != nil {
		// A procedure with no configured interval is a CONFIGURATION gap the
		// caller can act on, not a server fault: `donation_procedure` has four
		// members and the seed defines an interval for two, so
		// `{"procedure":"double_red_cell"}` is a well-formed request this
		// deployment cannot yet decide. It used to reach the client as a 500.
		//
		// It is ErrInvalid — 422, naming the field — rather than ErrConflict,
		// because nothing about the DONOR is wrong. `WI-24` adds the remaining
		// intervals as policy rows, at which point this branch stops firing
		// without any code changing, which is the point of policy being data.
		if errors.Is(err, domain.ErrPolicyMissing) && !policies.Has(domain.IntervalKey(proc)) {
			return domain.Decision{}, fmt.Errorf(
				"%w: this centre has no donation interval configured for %s, so a %s donation cannot be booked yet",
				ErrInvalid, proc, proc)
		}
		// Anything else missing or malformed is an outage, not a verdict.
		// Mapping it to ErrInvalid would tell the donor they are ineligible,
		// which is a clinical statement this system is in no position to make.
		return domain.Decision{}, fmt.Errorf("eligibility policy unavailable: %w", err)
	}
	return d, nil
}

// Facts reads what the decision depends on, on the service's own pool.
func (s *EligibilityService) Facts(ctx context.Context, donorID int64, proc domain.Procedure) (domain.DonorFacts, error) {
	return s.FactsWith(ctx, s.q, donorID, proc)
}

// FactsWith reads them on the caller's queries — its transaction, if it has one.
func (s *EligibilityService) FactsWith(ctx context.Context, q *store.Queries, donorID int64, proc domain.Procedure) (domain.DonorFacts, error) {
	row, err := q.GetDonorEligibilityFacts(ctx, store.GetDonorEligibilityFactsParams{
		DonorID: donorID, Procedure: store.DonationProcedure(proc),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.DonorFacts{}, fmt.Errorf("%w: no donor profile with that id", ErrNotFound)
		}
		return domain.DonorFacts{}, fmt.Errorf("read eligibility facts: %w", err)
	}

	f := domain.DonorFacts{
		DateOfBirth:         row.DateOfBirth.Time,
		Gender:              domain.Gender(row.Gender),
		AccountActive:       row.AccountActive,
		FirstTime:           row.FirstTime,
		Procedure:           proc,
		DonationsLast12M:    int(row.DonationsLast12m),
		PermanentlyDeferred: row.PermanentlyDeferred,
	}
	if row.LastDonationAt.Valid {
		t := row.LastDonationAt.Time
		f.LastDonationAt = &t
	}
	if row.DeferredUntil.Valid {
		t := row.DeferredUntil.Time
		f.DeferredUntil = &t
	}
	return f, nil
}

// ErrIneligible is returned when a donor cannot donate on the requested date.
//
// It carries the whole decision, not a string. `FR-19` requires the donor to see
// "a plain-language explanation, not an error code", and `FR-08` requires them
// to be told "the date they become eligible" — neither survives being flattened
// into an error message, so the handler unwraps this and renders the decision.
type ErrIneligible struct {
	Decision domain.Decision
}

func (e *ErrIneligible) Error() string {
	if len(e.Decision.Failures) == 0 {
		return "donor is not eligible"
	}
	return "donor is not eligible: " + e.Decision.Failures[0].Message
}

// Is makes ErrIneligible match ErrConflict for any handler that has not been
// taught about it.
//
// A booking refused on clinical grounds is a 409: the request is well-formed and
// authorized, and it conflicts with the state of the donor's record. Defaulting
// to that means a new call site which forgets to unwrap still returns something
// defensible rather than a 500.
func (e *ErrIneligible) Is(target error) bool { return target == ErrConflict }

// AsIneligible unwraps an ErrIneligible, for handlers that render the decision.
func AsIneligible(err error) (*ErrIneligible, bool) {
	var e *ErrIneligible
	ok := errors.As(err, &e)
	return e, ok
}

// PermanentDeferralOverride records an admin's decision to book despite a
// permanent deferral.
//
// `FR-19`: "A permanent deferral cannot be bypassed by any role except `admin`,
// with audit." The override is therefore not a boolean a caller passes — it is a
// role check plus a reason, and both are required. `WI-27` turns the log line
// below into an `audit_log` row; until it exists, the reason is recorded where
// it can at least be read, because an override with no trace is exactly what
// this requirement forbids.
type PermanentDeferralOverride struct {
	ActorID   int64
	ActorRole domain.Role
	Reason    string
}

var (
	// ErrOverrideNotPermitted is returned when a non-admin attempts one.
	ErrOverrideNotPermitted = errors.New("only an admin may override a permanent deferral")
	// ErrOverrideNeedsReason is returned when an admin attempts one without saying why.
	ErrOverrideNeedsReason = errors.New("an override requires a reason")
)

// Validate checks that an override is one this system will accept.
func (o *PermanentDeferralOverride) Validate() error {
	if o.ActorRole != domain.RoleAdmin {
		return fmt.Errorf("%w: %s", ErrOverrideNotPermitted, o.ActorRole)
	}
	if len(strings.TrimSpace(o.Reason)) < minOverrideReason {
		return ErrOverrideNeedsReason
	}
	return nil
}

// minOverrideReason is a length, not a vocabulary: unlike a rejection reason
// (`FR-09`), an override is rare, individual and clinical, so a controlled list
// would push a real situation into a wrong label. What it must not be is empty
// or "ok" — a reason nobody can act on later is the same as no reason.
const minOverrideReason = 10
