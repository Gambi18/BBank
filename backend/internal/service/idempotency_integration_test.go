package service_test

import (
	"context"
	"sync"
	"testing"

	"bbank/internal/service"
	"bbank/internal/store"
	"bbank/internal/testsupport"
)

func fp(b byte) []byte {
	// The column CHECKs octet_length(fingerprint) = 32.
	out := make([]byte, 32)
	for i := range out {
		out[i] = b
	}
	return out
}

// The claim is a single `INSERT ... ON CONFLICT DO NOTHING RETURNING`, and this
// is the property that statement exists for: under real concurrency exactly one
// caller may hold a key. A `SELECT`-then-`INSERT` would leave a window in which
// several callers all conclude they are the original, which is precisely the
// double-submit the table is meant to stop.
func TestOnlyOneCallerCanClaimAKey(t *testing.T) {
	pool := testsupport.Pool(t)
	svc := service.NewIdempotencyService(store.New(pool))
	actor := testsupport.NewDonor(t, pool, "idem@example.test", "Idem Actor")

	const attempts = 10
	var wg sync.WaitGroup
	var mu sync.Mutex
	claimed := 0
	var errs []error
	start := make(chan struct{})

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, ok, err := svc.Claim(context.Background(), actor, "shared-key-01", "POST /x", fp(1))
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			if ok {
				claimed++
			}
		}()
	}
	close(start)
	wg.Wait()

	if len(errs) > 0 {
		t.Fatalf("claim errors: %v", errs)
	}
	if claimed != 1 {
		t.Fatalf("%d callers believed they were first, want exactly 1", claimed)
	}
	if n := testsupport.CountRows(t, pool, `SELECT count(*) FROM idempotency_keys`); n != 1 {
		t.Errorf("%d key rows, want 1", n)
	}
}

// Uniqueness is (actor_id, idem_key), so one user cannot burn another's key —
// still less be handed the stored response to somebody else's request.
func TestKeysAreScopedToTheActor(t *testing.T) {
	pool := testsupport.Pool(t)
	svc := service.NewIdempotencyService(store.New(pool))
	ctx := context.Background()

	a := testsupport.NewDonor(t, pool, "actorA@example.test", "Actor A")
	b := testsupport.NewDonor(t, pool, "actorB@example.test", "Actor B")

	if _, ok, err := svc.Claim(ctx, a, "same-key-99", "POST /x", fp(1)); err != nil || !ok {
		t.Fatalf("actor A claim: ok=%v err=%v", ok, err)
	}
	// The same key string, a different person: must be a fresh claim, not a
	// collision with A's.
	if _, ok, err := svc.Claim(ctx, b, "same-key-99", "POST /x", fp(2)); err != nil || !ok {
		t.Fatalf("actor B was blocked by actor A's key: ok=%v err=%v", ok, err)
	}
	if n := testsupport.CountRows(t, pool, `SELECT count(*) FROM idempotency_keys`); n != 2 {
		t.Errorf("%d rows, want 2 — the key was not scoped to the actor", n)
	}
}

// Claim -> Complete -> replay is the full happy path, and an incomplete row
// must be distinguishable from a completed one (409 vs replay).
func TestCompleteMakesTheRecordReplayable(t *testing.T) {
	pool := testsupport.Pool(t)
	svc := service.NewIdempotencyService(store.New(pool))
	ctx := context.Background()
	actor := testsupport.NewDonor(t, pool, "replay@example.test", "Replay Actor")

	rec, ok, err := svc.Claim(ctx, actor, "replay-key-1", "POST /x", fp(7))
	if err != nil || !ok {
		t.Fatalf("first claim: ok=%v err=%v", ok, err)
	}
	if rec.Completed() {
		t.Fatal("a freshly claimed key reports as completed")
	}

	// A second arrival while the first is still running is in flight, not a replay.
	rec, ok, err = svc.Claim(ctx, actor, "replay-key-1", "POST /x", fp(7))
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if ok {
		t.Fatal("a second caller claimed a held key")
	}
	if rec.Completed() {
		t.Fatal("an in-flight request reported as completed; the caller would replay an empty response")
	}

	body := []byte(`{"data":{"id":1}}`)
	if err := svc.Complete(ctx, actor, "replay-key-1", 201, body); err != nil {
		t.Fatalf("complete: %v", err)
	}

	rec, ok, err = svc.Claim(ctx, actor, "replay-key-1", "POST /x", fp(7))
	if err != nil {
		t.Fatalf("third claim: %v", err)
	}
	if ok {
		t.Fatal("the key was claimable again after completion")
	}
	if !rec.Completed() {
		t.Fatal("the completed record does not report as completed")
	}
	if *rec.Status != 201 || string(rec.Body) != string(body) {
		t.Errorf("stored response = (%d, %q), want (201, %q)", *rec.Status, rec.Body, body)
	}
}

// A 5xx releases the claim so an honest retry genuinely retries, rather than
// being handed the same failure for the next 24 hours.
func TestReleaseFreesAnIncompleteClaim(t *testing.T) {
	pool := testsupport.Pool(t)
	svc := service.NewIdempotencyService(store.New(pool))
	ctx := context.Background()
	actor := testsupport.NewDonor(t, pool, "release@example.test", "Release Actor")

	if _, ok, err := svc.Claim(ctx, actor, "release-key-1", "POST /x", fp(3)); err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	if err := svc.Release(ctx, actor, "release-key-1"); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, ok, err := svc.Claim(ctx, actor, "release-key-1", "POST /x", fp(3)); err != nil || !ok {
		t.Fatalf("the key was not reclaimable after release: ok=%v err=%v", ok, err)
	}
}

// Release must NOT discard a completed record — that would turn a replayable
// answer back into an executable request.
func TestReleaseLeavesCompletedRecordsAlone(t *testing.T) {
	pool := testsupport.Pool(t)
	svc := service.NewIdempotencyService(store.New(pool))
	ctx := context.Background()
	actor := testsupport.NewDonor(t, pool, "keep@example.test", "Keep Actor")

	if _, _, err := svc.Claim(ctx, actor, "keep-key-1", "POST /x", fp(4)); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := svc.Complete(ctx, actor, "keep-key-1", 200, []byte(`{}`)); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if err := svc.Release(ctx, actor, "keep-key-1"); err != nil {
		t.Fatalf("release: %v", err)
	}
	if n := testsupport.CountRows(t, pool,
		`SELECT count(*) FROM idempotency_keys WHERE idem_key = 'keep-key-1'`); n != 1 {
		t.Fatal("release deleted a completed record; a retry would now execute twice")
	}
}

func TestSweepRemovesOnlyExpiredKeys(t *testing.T) {
	pool := testsupport.Pool(t)
	svc := service.NewIdempotencyService(store.New(pool))
	ctx := context.Background()
	actor := testsupport.NewDonor(t, pool, "sweep@example.test", "Sweep Actor")

	if _, _, err := svc.Claim(ctx, actor, "fresh-key-1", "POST /x", fp(5)); err != nil {
		t.Fatalf("claim fresh: %v", err)
	}
	if _, _, err := svc.Claim(ctx, actor, "stale-key-1", "POST /x", fp(6)); err != nil {
		t.Fatalf("claim stale: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE idempotency_keys SET expires_at = now() - INTERVAL '1 hour' WHERE idem_key = 'stale-key-1'`); err != nil {
		t.Fatalf("age the row: %v", err)
	}

	n, err := svc.Sweep(ctx)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if n != 1 {
		t.Errorf("swept %d rows, want 1", n)
	}
	if left := testsupport.CountRows(t, pool,
		`SELECT count(*) FROM idempotency_keys WHERE idem_key = 'fresh-key-1'`); left != 1 {
		t.Error("the sweep took a key that had not expired")
	}
}
