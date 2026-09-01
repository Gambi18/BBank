package platform

// IdempotencyRecord is one stored request/response pair (TRD §6.4).
//
// It lives in platform for the same reason Claims does: the middleware that
// reads it and the service that writes it must agree on a type, and neither may
// import the other under the dependency rule (TRD §4.2). platform is the lowest
// package both can see.
type IdempotencyRecord struct {
	Fingerprint []byte
	Status      *int32
	Body        []byte
}

// Completed reports whether the original request finished. A claimed but
// uncompleted row is a request still in flight, which is a 409 — not a replay,
// and not a fresh execution.
func (r IdempotencyRecord) Completed() bool { return r.Status != nil }
