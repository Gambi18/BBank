package domain

import (
	"errors"
	"strings"
	"testing"
)

// FR-09: rejection requires a reason from a controlled list.
func TestValidateRejectionAcceptsOnlyKnownReasons(t *testing.T) {
	for _, r := range RejectionReasons() {
		note := ""
		if r.Value == ReasonOther {
			note = "a specific explanation"
		}
		if err := ValidateRejection(r.Value, note); err != nil {
			t.Errorf("listed reason %q was rejected: %v", r.Value, err)
		}
	}
}

func TestValidateRejectionRefusesFreeText(t *testing.T) {
	// The whole point of a controlled list: a reason nobody chose from it must
	// not become a value, or FR-61's rejection-reason report is prose.
	for _, bad := range []RejectionReason{"", "donor is unsuitable", "OTHER", "center_at_capacity ", "made_up"} {
		if err := ValidateRejection(bad, "note"); !errors.Is(err, ErrUnknownRejectionReason) {
			t.Errorf("ValidateRejection(%q) = %v, want ErrUnknownRejectionReason", bad, err)
		}
	}
}

// 'other' exists so staff are never forced into a wrong label — but it has to
// say something, or it is just a hole in the vocabulary.
func TestValidateRejectionOtherRequiresANote(t *testing.T) {
	for _, note := range []string{"", "   ", "\t\n"} {
		if err := ValidateRejection(ReasonOther, note); !errors.Is(err, ErrRejectionNoteRequired) {
			t.Errorf("ValidateRejection(other, %q) = %v, want ErrRejectionNoteRequired", note, err)
		}
	}
	if err := ValidateRejection(ReasonOther, "centre flooded"); err != nil {
		t.Errorf("'other' with a note was rejected: %v", err)
	}
}

// UI/UX §4: "rejected" is reserved for requests. A reason that reads as a
// verdict on a person would surface in a notification to that person.
func TestRejectionReasonsDescribeTheRequestNotThePerson(t *testing.T) {
	banned := []string{"unsuitable", "unfit", "failed", "bad ", "unhealthy", "ineligible", "refused"}
	for _, r := range RejectionReasons() {
		hay := strings.ToLower(string(r.Value) + " " + r.Label)
		for _, w := range banned {
			if strings.Contains(hay, w) {
				t.Errorf("reason %q (%q) describes the person, not the request: contains %q", r.Value, r.Label, w)
			}
		}
	}
}

func TestRejectionReasonsAreStableAndLabelled(t *testing.T) {
	first := RejectionReasons()
	if len(first) == 0 {
		t.Fatal("no rejection reasons defined")
	}
	// Sorted, so the UI order does not shuffle between requests (map iteration).
	second := RejectionReasons()
	for i := range first {
		if first[i].Value != second[i].Value {
			t.Fatalf("RejectionReasons() is not stably ordered at %d: %q vs %q", i, first[i].Value, second[i].Value)
		}
		if strings.TrimSpace(first[i].Label) == "" {
			t.Errorf("reason %q has no label; the UI would render a blank option", first[i].Value)
		}
	}
}

// The state machine. Every decided state is terminal: an appointment must never
// end up attached to a request that later reads 'rejected'.
func TestRequestTransitions(t *testing.T) {
	legal := []struct{ from, to RequestStatus }{
		{RequestPending, RequestApproved},
		{RequestPending, RequestRejected},
		{RequestPending, RequestCancelled},
		{RequestPending, RequestExpired},
	}
	for _, c := range legal {
		if !CanTransition(c.from, c.to) {
			t.Errorf("%s -> %s should be allowed", c.from, c.to)
		}
	}

	illegal := []struct{ from, to RequestStatus }{
		{RequestApproved, RequestRejected}, // undoing a decision after booking
		{RequestApproved, RequestPending},  // re-opening
		{RequestRejected, RequestApproved}, // the dangerous one
		{RequestRejected, RequestPending},
		{RequestCancelled, RequestApproved},
		{RequestExpired, RequestApproved},
		{RequestApproved, RequestApproved}, // double approval => double appointment
		{RequestPending, RequestPending},
		{"", RequestApproved}, // unknown state denies
		{RequestPending, "nonsense"},
	}
	for _, c := range illegal {
		if CanTransition(c.from, c.to) {
			t.Errorf("%s -> %s should be forbidden", c.from, c.to)
		}
		if err := EnsureTransition(c.from, c.to); !errors.Is(err, ErrIllegalTransition) {
			t.Errorf("EnsureTransition(%s, %s) = %v, want ErrIllegalTransition", c.from, c.to, err)
		}
	}
}

// Approving twice is the concurrency case the FOR UPDATE lock exists for. The
// domain half of that guarantee is that the second attempt is not a legal move.
func TestDoubleApprovalIsNotALegalTransition(t *testing.T) {
	if CanTransition(RequestApproved, RequestApproved) {
		t.Fatal("approving an already-approved request is allowed; that is a second appointment")
	}
}
