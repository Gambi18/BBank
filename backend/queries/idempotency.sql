-- Idempotency-key storage (TRD §6.4). See migration 000016.

-- Claim the key for this request, or tell us who already holds it.
--
-- The INSERT ... ON CONFLICT DO NOTHING is the whole concurrency story: two
-- simultaneous retries race here, exactly one inserts, and the loser gets no
-- row back and must then read the existing one. Doing this as SELECT-then-INSERT
-- would leave a window in which both callers decide they are first.
-- name: ClaimIdempotencyKey :one
INSERT INTO idempotency_keys (idem_key, actor_id, endpoint, fingerprint)
VALUES ($1, $2, $3, $4)
ON CONFLICT (actor_id, idem_key) DO NOTHING
RETURNING id, idem_key, actor_id, endpoint, fingerprint, response_status, response_body, created_at, completed_at, expires_at;

-- name: GetIdempotencyKey :one
SELECT id, idem_key, actor_id, endpoint, fingerprint, response_status, response_body, created_at, completed_at, expires_at
FROM idempotency_keys
WHERE actor_id = $1 AND idem_key = $2;

-- Store the response so a later retry can be answered without re-executing.
-- name: CompleteIdempotencyKey :exec
UPDATE idempotency_keys
SET response_status = $3,
    response_body   = $4,
    completed_at    = now()
WHERE actor_id = $1 AND idem_key = $2;

-- Release a claim whose handler never produced a storable response, so the
-- client can retry instead of being told for 24 hours that a request it never
-- completed is still in flight.
-- name: ReleaseIdempotencyKey :exec
DELETE FROM idempotency_keys
WHERE actor_id = $1 AND idem_key = $2 AND response_status IS NULL;

-- name: SweepIdempotencyKeys :execrows
DELETE FROM idempotency_keys WHERE expires_at < now();
