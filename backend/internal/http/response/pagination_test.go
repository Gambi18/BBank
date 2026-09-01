package response

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func parse(t *testing.T, query string) (Paging, int, string) {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/donors"+query, nil)
	w := httptest.NewRecorder()
	p, ok := ParsePaging(w, r)
	if ok {
		return p, 0, ""
	}
	return p, w.Code, w.Body.String()
}

// The default is what closes the unbounded scan (A15): a caller who asks for
// nothing in particular must not receive the entire table.
func TestParsePagingDefaultsAreBounded(t *testing.T) {
	p, status, _ := parse(t, "")
	if status != 0 {
		t.Fatalf("a request with no paging parameters was rejected: %d", status)
	}
	if p.Limit != DefaultLimit || p.Offset != 0 {
		t.Fatalf("got limit=%d offset=%d, want %d and 0", p.Limit, p.Offset, DefaultLimit)
	}
	if DefaultLimit <= 0 || MaxLimit <= 0 {
		t.Fatal("a non-positive default or maximum would reintroduce unbounded reads")
	}
}

// §6.3: over the maximum is CLAMPED, not rejected — and the applied value is
// what gets reported, so a caller's paging loop cannot silently skip rows.
func TestParsePagingClampsRatherThanRejects(t *testing.T) {
	p, status, _ := parse(t, "?limit=5000")
	if status != 0 {
		t.Fatalf("an over-large limit was rejected with %d; §6.3 says clamp", status)
	}
	if p.Limit != MaxLimit {
		t.Fatalf("limit = %d, want it clamped to %d", p.Limit, MaxLimit)
	}
}

func TestParsePagingAcceptsValidValues(t *testing.T) {
	p, status, _ := parse(t, "?limit=10&offset=30")
	if status != 0 {
		t.Fatalf("valid paging rejected: %d", status)
	}
	if p.Limit != 10 || p.Offset != 30 {
		t.Fatalf("got limit=%d offset=%d, want 10 and 30", p.Limit, p.Offset)
	}
}

// limit=0 must not be read as "no limit". That reading is exactly how an
// unbounded scan comes back after being closed.
func TestParsePagingRejectsZeroAndNegativeLimit(t *testing.T) {
	for _, q := range []string{"?limit=0", "?limit=-1"} {
		_, status, body := parse(t, q)
		if status != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", q, status)
		}
		if body == "" {
			t.Errorf("%s: rejected with an empty body", q)
		}
	}
}

func TestParsePagingRejectsMalformedValues(t *testing.T) {
	for _, q := range []string{"?limit=abc", "?offset=abc", "?offset=-5"} {
		_, status, body := parse(t, q)
		if status != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", q, status)
		}
		// The error must be actionable: a machine-readable code and the field.
		if body == "" {
			t.Errorf("%s: no error body", q)
		}
	}
}
