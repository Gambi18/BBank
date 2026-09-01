-- 000016_idempotency_keys (WI-21)
--
-- Replay protection for unsafe requests (TRD §6.4).
--
-- The failure this exists to prevent is concrete: a phlebotomist on a laggy
-- tablet double-taps "Record collection" and the system creates two donations,
-- two bags and two barcodes for one venepuncture. That is an inventory-integrity
-- and traceability incident, not a cosmetic duplicate.
--
-- WI-21 builds the table and the middleware; WI-77 turns enforcement on for the
-- endpoints marked `Idem` in TRD §6.5. Until then the middleware records and
-- replays without *requiring* a key, so the storage shape is proven in
-- production traffic before anything starts rejecting requests for want of one.

CREATE TABLE idempotency_keys (
    id              BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    -- Client-generated per user intent (generated when the form is RENDERED, not
    -- when it is submitted, so a retry of the same intent reuses it).
    idem_key        TEXT        NOT NULL,

    -- Scoped to the actor on purpose. If `idem_key` alone were unique, one user
    -- could burn another user's key — either denying them a write or, worse,
    -- being handed the stored response to somebody else's request.
    actor_id        BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Method + route template, e.g. 'POST /api/v1/donation-requests'. The
    -- template, not the raw path: it is what the key is scoped to conceptually,
    -- and it keeps IDs out of a table that is not access-controlled per row.
    endpoint        TEXT        NOT NULL,

    -- SHA-256 of method + path + body. Same key with a DIFFERENT fingerprint is
    -- a client bug (a key reused for a different intent) and must be refused,
    -- not silently answered with the earlier response.
    fingerprint     BYTEA       NOT NULL,

    -- NULL until the handler completes. A row with a NULL status is a request
    -- still in flight; a second arrival while it is NULL gets 409, because
    -- returning "no content yet" as success would be a lie.
    response_status INTEGER,
    response_body   BYTEA,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at    TIMESTAMPTZ,

    -- 24h TTL (§6.4), swept nightly. Stored rather than computed so the sweep is
    -- a plain index scan and the TTL can differ per endpoint later.
    expires_at      TIMESTAMPTZ NOT NULL DEFAULT now() + INTERVAL '24 hours',

    CONSTRAINT idempotency_keys_actor_key_uq UNIQUE (actor_id, idem_key),
    CONSTRAINT idempotency_keys_fingerprint_len CHECK (octet_length(fingerprint) = 32),
    CONSTRAINT idempotency_keys_key_len CHECK (char_length(idem_key) BETWEEN 8 AND 255),
    -- A completed row must carry both halves of its answer, or replay would
    -- return a status with no body and call it a success.
    CONSTRAINT idempotency_keys_completed_shape CHECK (
        (response_status IS NULL AND completed_at IS NULL)
     OR (response_status IS NOT NULL AND completed_at IS NOT NULL)
    )
);

COMMENT ON TABLE idempotency_keys IS
  'TRD §6.4 replay protection. A row is inserted before the handler runs and completed after, so a concurrent retry sees an in-flight row (409) rather than executing twice.';
COMMENT ON COLUMN idempotency_keys.fingerprint IS
  'SHA-256 of method+path+body. Same key + same fingerprint replays the stored response; same key + different fingerprint is 422 idempotency_key_reuse.';

-- The sweep (§6.4: nightly). Partial, because completed rows are the ones that
-- accumulate and the in-flight ones are few and short-lived.
CREATE INDEX idempotency_keys_expiry_idx ON idempotency_keys (expires_at);
