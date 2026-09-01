package service

import (
	"context"
	"errors"

	"bbank/internal/platform"
	"bbank/internal/store"

	"github.com/jackc/pgx/v5"
)

// IdempotencyService is the transaction-free half of TRD §6.4: claim a key,
// complete it with the response, or release it so a retry can run.
//
// The interesting decision is all in the query, not here — `INSERT ... ON
// CONFLICT DO NOTHING RETURNING` makes "am I first?" a single atomic statement.
// A SELECT-then-INSERT would leave a gap in which two concurrent retries both
// believe they are the original, which is exactly the double-submit this table
// exists to stop.
type IdempotencyService struct {
	q *store.Queries
}

func NewIdempotencyService(q *store.Queries) *IdempotencyService {
	return &IdempotencyService{q: q}
}

// Claim attempts to take the key for this request.
//
// Returns claimed=true when this caller is the first and should execute the
// handler. Returns claimed=false with the existing record when somebody else
// already holds the key — the caller then decides between replay (same
// fingerprint, completed), 409 (still in flight) and 422 (different
// fingerprint).
func (s *IdempotencyService) Claim(ctx context.Context, actorID int64, key, endpoint string, fingerprint []byte) (rec platform.IdempotencyRecord, claimed bool, err error) {
	row, err := s.q.ClaimIdempotencyKey(ctx, store.ClaimIdempotencyKeyParams{
		IdemKey:     key,
		ActorID:     actorID,
		Endpoint:    endpoint,
		Fingerprint: fingerprint,
	})
	if err == nil {
		return platform.IdempotencyRecord{Fingerprint: row.Fingerprint, Status: row.ResponseStatus, Body: row.ResponseBody}, true, nil
	}
	// DO NOTHING means no row is returned, which pgx reports as "no rows". That
	// is the contention path, not an error.
	if !errors.Is(err, pgx.ErrNoRows) {
		return platform.IdempotencyRecord{}, false, err
	}

	existing, err := s.q.GetIdempotencyKey(ctx, store.GetIdempotencyKeyParams{ActorID: actorID, IdemKey: key})
	if err != nil {
		// Vanishingly rare, but real: the sweep can delete an expired row
		// between the two statements. Report it rather than inventing a record.
		return platform.IdempotencyRecord{}, false, err
	}
	return platform.IdempotencyRecord{Fingerprint: existing.Fingerprint, Status: existing.ResponseStatus, Body: existing.ResponseBody}, false, nil
}

func (s *IdempotencyService) Complete(ctx context.Context, actorID int64, key string, status int32, body []byte) error {
	return s.q.CompleteIdempotencyKey(ctx, store.CompleteIdempotencyKeyParams{
		ActorID: actorID, IdemKey: key, ResponseStatus: &status, ResponseBody: body,
	})
}

func (s *IdempotencyService) Release(ctx context.Context, actorID int64, key string) error {
	return s.q.ReleaseIdempotencyKey(ctx, store.ReleaseIdempotencyKeyParams{ActorID: actorID, IdemKey: key})
}

// Sweep deletes expired keys (§6.4: 24h TTL, nightly). Returns how many went.
func (s *IdempotencyService) Sweep(ctx context.Context) (int64, error) {
	return s.q.SweepIdempotencyKeys(ctx)
}
