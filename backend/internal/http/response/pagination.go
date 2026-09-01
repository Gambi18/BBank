package response

import (
	"net/http"
	"strconv"
)

// Pagination limits (TRD §6.3).
const (
	DefaultLimit int32 = 25
	MaxLimit     int32 = 100
)

// Paging is the applied limit/offset — after clamping, not as requested.
type Paging struct {
	Limit  int32
	Offset int32
}

// ParsePaging reads ?limit= and ?offset= and clamps them.
//
// §6.3 says a request over the maximum is **clamped, not rejected**, and the
// response reports the applied value. Rejecting would break a caller for asking
// politely for too much; silently applying a different limit without saying so
// would make their paging loop skip rows. So: clamp, and echo it back in `page`.
//
// A malformed value is a 400, because guessing what someone meant by
// `?limit=abc` is worse than telling them. Returns ok=false when it has already
// written the error response.
func ParsePaging(w http.ResponseWriter, r *http.Request) (Paging, bool) {
	p := Paging{Limit: DefaultLimit}

	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			BadRequest(w, r, "limit must be an integer", Detail{Field: "limit", Issue: "not an integer"})
			return p, false
		}
		switch {
		case n <= 0:
			// Zero or negative is meaningless rather than "give me everything",
			// and reading it as the latter is how unbounded scans come back.
			BadRequest(w, r, "limit must be greater than zero", Detail{Field: "limit", Issue: "must be > 0"})
			return p, false
		case int32(n) > MaxLimit:
			p.Limit = MaxLimit
		default:
			p.Limit = int32(n)
		}
	}

	if v := r.URL.Query().Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			BadRequest(w, r, "offset must be a non-negative integer", Detail{Field: "offset", Issue: "must be >= 0"})
			return p, false
		}
		p.Offset = int32(n)
	}

	return p, true
}
