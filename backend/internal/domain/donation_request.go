package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// RequestStatus mirrors the donation_request_status enum (schema §11.4).
type RequestStatus string

const (
	RequestPending   RequestStatus = "pending"
	RequestApproved  RequestStatus = "approved"
	RequestRejected  RequestStatus = "rejected"
	RequestCancelled RequestStatus = "cancelled"
	RequestExpired   RequestStatus = "expired"
)

// RejectionReason is the controlled vocabulary FR-09 requires.
//
// It lives here, in Go, rather than as a database enum because the column is
// `rejection_reason TEXT` (schema §11.4) — the schema enforces only that a
// rejected request HAS a reason, and this enforces that the reason is one we
// recognise. Keeping it in the domain means the list is unit-testable without a
// database and can gain a value without a migration.
//
// Every value describes the REQUEST, never the person. UI/UX §4 is explicit
// that "rejected" is reserved for requests and must never be said about a
// donor: someone who cannot donate today is *deferred*, which is a different
// concept with its own table (`deferrals`) and its own vocabulary. A reason
// here that read like a verdict on a human being would leak into a
// notification, and that is the failure this list is shaped to prevent.
type RejectionReason string

const (
	// Scheduling and capacity — the ordinary cases.
	ReasonCenterAtCapacity RejectionReason = "center_at_capacity"
	ReasonCenterClosed     RejectionReason = "center_closed"
	ReasonDateUnavailable  RejectionReason = "date_unavailable"

	// The request itself is not actionable.
	ReasonDuplicateRequest RejectionReason = "duplicate_request"
	ReasonIncompleteRecord RejectionReason = "incomplete_record"
	ReasonDonorUnreachable RejectionReason = "donor_unreachable"

	// Deliberately NOT a clinical verdict. If the donor is ineligible, the
	// correct action is a deferral with an eligibility date (WI-26), which the
	// donor can be told about kindly and act on. Rejecting the request is how
	// the *booking* ends; it is not how a clinical decision is recorded.
	ReasonWithdrawnByDonor RejectionReason = "withdrawn_by_donor"

	// Requires a note. Kept so staff are never forced to file a real situation
	// under a wrong label just to get the form to submit.
	ReasonOther RejectionReason = "other"
)

var rejectionReasons = map[RejectionReason]string{
	ReasonCenterAtCapacity: "The centre is fully booked for that date",
	ReasonCenterClosed:     "The centre is closed on that date",
	ReasonDateUnavailable:  "That date is not available",
	ReasonDuplicateRequest: "A request for this donor is already open",
	ReasonIncompleteRecord: "The donor record is missing details needed to book",
	ReasonDonorUnreachable: "The donor could not be reached to confirm",
	ReasonWithdrawnByDonor: "The donor asked to withdraw the request",
	ReasonOther:            "Other (a note is required)",
}

var (
	ErrUnknownRejectionReason = errors.New("rejection reason is not one of the permitted values")
	ErrRejectionNoteRequired  = errors.New("a note is required when the reason is 'other'")
	ErrIllegalTransition      = errors.New("that status change is not allowed")
)

// RejectionReasons returns the permitted values with their labels, sorted, so a
// UI can render the list from the API instead of hardcoding its own copy that
// drifts.
func RejectionReasons() []struct {
	Value RejectionReason
	Label string
} {
	out := make([]struct {
		Value RejectionReason
		Label string
	}, 0, len(rejectionReasons))
	for v, l := range rejectionReasons {
		out = append(out, struct {
			Value RejectionReason
			Label string
		}{v, l})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Value < out[j].Value })
	return out
}

// ValidateRejection checks the reason is recognised, and that 'other' carries a
// note. Free text is allowed only alongside a coded reason, never instead of
// one — otherwise the rejection-reason report (FR-61) degenerates into prose
// nobody can aggregate.
func ValidateRejection(reason RejectionReason, note string) error {
	if _, ok := rejectionReasons[reason]; !ok {
		return fmt.Errorf("%w: got %q", ErrUnknownRejectionReason, reason)
	}
	if reason == ReasonOther && strings.TrimSpace(note) == "" {
		return ErrRejectionNoteRequired
	}
	return nil
}

// requestTransitions is the legal state machine. Absent means forbidden.
//
// Every terminal state is genuinely terminal: there is no path out of
// approved, rejected, cancelled or expired. Re-opening a decided request would
// mean an appointment could exist against a request that later reads
// 'rejected', and the audit chain would no longer explain itself.
var requestTransitions = map[RequestStatus][]RequestStatus{
	RequestPending: {RequestApproved, RequestRejected, RequestCancelled, RequestExpired},
}

// CanTransition reports whether a donation request may move between two states.
func CanTransition(from, to RequestStatus) bool {
	for _, allowed := range requestTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// EnsureTransition is CanTransition with an error that names both states, so a
// 409 can say what was actually wrong.
func EnsureTransition(from, to RequestStatus) error {
	if !CanTransition(from, to) {
		return fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, from, to)
	}
	return nil
}
