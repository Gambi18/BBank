-- 000014_auth_sessions (WI-17)
--
-- Server-side refresh-token families, and the token_version escape hatch.
--
-- Design (TRD §7.3): the ACCESS token is a 15-minute ES256 JWT verified without
-- a database round trip, so proxy.ts can check every navigation cheaply. The
-- REFRESH token is opaque and server-side, because that is where instant
-- revocation matters and the round trip is rare.
--
-- Only the SHA-256 hash of the refresh token is stored. A database disclosure
-- must not hand the attacker usable sessions.

ALTER TABLE users
    ADD COLUMN token_version INTEGER NOT NULL DEFAULT 1;

COMMENT ON COLUMN users.token_version IS
  'Incremented on role change, password change or forced logout. Carried in the JWT `ver` claim; a mismatch fails verification, invalidating every outstanding access token for this user immediately. This is the escape hatch from the 15-minute revocation window.';

CREATE TABLE sessions (
    id              BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id       TEXT        NOT NULL UNIQUE,
    user_id         BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- The family. Rotation issues a new row sharing family_id; presenting an
    -- already-rotated token revokes the whole family (reuse detection).
    family_id       TEXT        NOT NULL,
    token_hash      BYTEA       NOT NULL,

    issued_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ NOT NULL,
    rotated_at      TIMESTAMPTZ,
    revoked_at      TIMESTAMPTZ,
    revoked_reason  TEXT,

    ip              INET,
    user_agent      TEXT,

    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT sessions_token_hash_key UNIQUE (token_hash),
    CONSTRAINT sessions_expiry_after_issue CHECK (expires_at > issued_at),
    -- A row is rotated, revoked, or live — "rotated and never revoked" is fine,
    -- but a revoked_reason without a revoked_at is a bug.
    CONSTRAINT sessions_revoked_sync CHECK ((revoked_at IS NULL) = (revoked_reason IS NULL))
);

COMMENT ON TABLE sessions IS 'Refresh-token families. One row per issued refresh token; rotation appends a new row in the same family and stamps rotated_at on the old one.';
COMMENT ON COLUMN sessions.token_hash IS 'SHA-256 of the opaque refresh token. The token itself is never stored.';

-- Refresh presents a token; this is the hot lookup.
CREATE INDEX sessions_token_hash_idx ON sessions (token_hash);
-- Family revocation on reuse detection.
CREATE INDEX sessions_family_idx ON sessions (family_id) WHERE revoked_at IS NULL;
-- "Show me my active sessions", and the expiry sweep.
CREATE INDEX sessions_user_active_idx ON sessions (user_id, expires_at DESC) WHERE revoked_at IS NULL;

CREATE TRIGGER sessions_set_updated_at
    BEFORE UPDATE ON sessions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
