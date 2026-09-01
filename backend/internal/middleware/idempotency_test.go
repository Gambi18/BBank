package middleware

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"bbank/internal/domain"
	"bbank/internal/platform"
)

// fakeIdemStore is an in-memory stand-in with the same atomic claim semantics as
// the INSERT ... ON CONFLICT DO NOTHING it replaces: exactly one caller can hold
// a key, and the rest are told who does.
type fakeIdemStore struct {
	mu   sync.Mutex
	rows map[string]*platform.IdempotencyRecord
}

func newFakeStore() *fakeIdemStore {
	return &fakeIdemStore{rows: map[string]*platform.IdempotencyRecord{}}
}

func (f *fakeIdemStore) Claim(_ context.Context, actorID int64, key, _ string, fp []byte) (platform.IdempotencyRecord, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := key
	if existing, ok := f.rows[k]; ok {
		return *existing, false, nil
	}
	rec := &platform.IdempotencyRecord{Fingerprint: fp}
	f.rows[k] = rec
	return *rec, true, nil
}

func (f *fakeIdemStore) Complete(_ context.Context, _ int64, key string, status int32, body []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if rec, ok := f.rows[key]; ok {
		rec.Status = &status
		rec.Body = body
	}
	return nil
}

func (f *fakeIdemStore) Release(_ context.Context, _ int64, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.rows, key)
	return nil
}

// authed puts a verified identity on the request, which is what scopes the key.
func authed(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), identityKey{},
		Identity{UserID: 7, Role: domain.RoleDonor}))
}

func post(body string, key string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/donation-requests", strings.NewReader(body))
	if key != "" {
		r.Header.Set("Idempotency-Key", key)
	}
	return authed(r)
}

// The double-submit this whole table exists to prevent: the same intent sent
// twice must execute once and answer identically both times.
func TestIdempotencyReplaysInsteadOfReExecuting(t *testing.T) {
	store := newFakeStore()
	calls := 0
	h := Idempotency(store, false)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":1}}`))
	}))

	first := httptest.NewRecorder()
	h.ServeHTTP(first, post(`{"a":1}`, "key-abcdefgh"))
	second := httptest.NewRecorder()
	h.ServeHTTP(second, post(`{"a":1}`, "key-abcdefgh"))

	if calls != 1 {
		t.Fatalf("handler ran %d times; the point of the key is that it runs once", calls)
	}
	if first.Code != http.StatusCreated || second.Code != http.StatusCreated {
		t.Fatalf("statuses %d and %d; the replay must reproduce the original", first.Code, second.Code)
	}
	if first.Body.String() != second.Body.String() {
		t.Fatalf("replay body %q differs from original %q", second.Body.String(), first.Body.String())
	}
	if second.Header().Get("Idempotent-Replay") != "true" {
		t.Errorf("replay was not marked with Idempotent-Replay")
	}
	if first.Header().Get("Idempotent-Replay") != "" {
		t.Errorf("the original response was marked as a replay")
	}
}

// Same key, different body: a client bug. Returning the stored response would
// answer a question that was never asked.
func TestIdempotencyKeyReuseWithDifferentBodyIs422(t *testing.T) {
	store := newFakeStore()
	h := Idempotency(store, false)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":1}}`))
	}))

	h.ServeHTTP(httptest.NewRecorder(), post(`{"a":1}`, "key-abcdefgh"))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, post(`{"a":999}`, "key-abcdefgh"))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d, want 422", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "idempotency_key_reuse") {
		t.Errorf("body %q does not carry the machine-readable code", rec.Body.String())
	}
}

// A claim with no stored response yet is a request still running. Answering
// "success, no content" would be a lie about work that may not have happened.
func TestIdempotencyInFlightIs409(t *testing.T) {
	store := newFakeStore()
	// Claim the key and never complete it: exactly the state a concurrent
	// retry arrives in.
	fp := fingerprint(http.MethodPost, "/api/v1/donation-requests", []byte(`{"a":1}`))
	if _, claimed, _ := store.Claim(context.Background(), 7, "key-abcdefgh", "", fp); !claimed {
		t.Fatal("setup: could not claim the key")
	}

	h := Idempotency(store, false)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("handler ran while the original request was still in flight")
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, post(`{"a":1}`, "key-abcdefgh"))

	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409", rec.Code)
	}
	if rec.Header().Get("Retry-After") != "1" {
		t.Errorf("no Retry-After on an in-flight response")
	}
}

// A 5xx must not be frozen against the key. If it were, the honest client would
// retry with the same key — correctly — and be handed the failure forever.
func TestIdempotencyDoesNotStoreServerErrors(t *testing.T) {
	store := newFakeStore()
	calls := 0
	h := Idempotency(store, false)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":1}}`))
	}))

	h.ServeHTTP(httptest.NewRecorder(), post(`{"a":1}`, "key-abcdefgh"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, post(`{"a":1}`, "key-abcdefgh"))

	if calls != 2 {
		t.Fatalf("handler ran %d times; a retry after a 5xx must genuinely retry", calls)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("retry got %d, want the successful 201", rec.Code)
	}
}

// WI-21 records without requiring; WI-77 flips this per endpoint.
func TestIdempotencyRequiredFlag(t *testing.T) {
	ran := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		ran = true
		w.WriteHeader(http.StatusCreated)
	})

	optional := httptest.NewRecorder()
	Idempotency(newFakeStore(), false)(handler).ServeHTTP(optional, post(`{}`, ""))
	if !ran || optional.Code != http.StatusCreated {
		t.Fatalf("optional mode blocked a keyless request: status %d", optional.Code)
	}

	ran = false
	required := httptest.NewRecorder()
	Idempotency(newFakeStore(), true)(handler).ServeHTTP(required, post(`{}`, ""))
	if ran {
		t.Error("required mode let a keyless request through to the handler")
	}
	if required.Code != http.StatusBadRequest ||
		!strings.Contains(required.Body.String(), "idempotency_key_required") {
		t.Fatalf("required mode: status %d body %q", required.Code, required.Body.String())
	}
}

// A GET carries no risk of double-execution, so a key on one is ignored rather
// than stored — otherwise reads would fill the table.
func TestIdempotencyIgnoresSafeMethods(t *testing.T) {
	store := newFakeStore()
	h := Idempotency(store, true)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := authed(httptest.NewRequest(http.MethodGet, "/api/v1/donors", nil))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("a GET was rejected for want of an Idempotency-Key: %d", rec.Code)
	}
	if len(store.rows) != 0 {
		t.Errorf("a safe method wrote %d idempotency rows", len(store.rows))
	}
}

// The handler must still be able to read a body the middleware has consumed to
// fingerprint it.
func TestIdempotencyRestoresTheRequestBody(t *testing.T) {
	var got bytes.Buffer
	h := Idempotency(newFakeStore(), false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = got.ReadFrom(r.Body)
		w.WriteHeader(http.StatusCreated)
	}))

	h.ServeHTTP(httptest.NewRecorder(), post(`{"donor_id":3}`, "key-abcdefgh"))

	if got.String() != `{"donor_id":3}` {
		t.Fatalf("handler read %q; the body was not restored after fingerprinting", got.String())
	}
}

// The key is scoped to the actor: one user must never be able to burn another's
// key, still less be handed their stored response.
func TestIdempotencyKeysAreScopedToTheActor(t *testing.T) {
	fp := fingerprint(http.MethodPost, "/x", []byte(`{}`))
	store := newFakeStore()

	if _, claimed, _ := store.Claim(context.Background(), 7, "shared-key-1", "", fp); !claimed {
		t.Fatal("first actor could not claim")
	}
	// The real store's uniqueness is (actor_id, idem_key); this asserts the
	// middleware passes the caller's id through, which is what makes that work.
	r := httptest.NewRequest(http.MethodPost, "/api/v1/donation-requests", strings.NewReader(`{}`))
	r.Header.Set("Idempotency-Key", "shared-key-1")
	r = r.WithContext(context.WithValue(r.Context(), identityKey{}, Identity{UserID: 8, Role: domain.RoleDonor}))

	var sawActor int64
	spy := &actorSpy{inner: store, seen: &sawActor}
	h := Idempotency(spy, false)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	h.ServeHTTP(httptest.NewRecorder(), r)

	if sawActor != 8 {
		t.Fatalf("store saw actor %d; the key was not scoped to the caller", sawActor)
	}
}

type actorSpy struct {
	inner *fakeIdemStore
	seen  *int64
}

func (a *actorSpy) Claim(ctx context.Context, actorID int64, key, ep string, fp []byte) (platform.IdempotencyRecord, bool, error) {
	*a.seen = actorID
	return platform.IdempotencyRecord{Fingerprint: fp}, true, nil
}
func (a *actorSpy) Complete(ctx context.Context, actorID int64, key string, status int32, body []byte) error {
	return nil
}
func (a *actorSpy) Release(ctx context.Context, actorID int64, key string) error { return nil }
