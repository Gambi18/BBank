-- 000017_user_invites (WI-18)
--
-- Replaces the hardcoded admin credential with an invite (FR-66, TD-02).
--
-- Design: an invite creates the `users` row immediately, with
-- `status = 'pending_verification'` and a password nobody knows (a random
-- 32-byte secret, hashed and discarded — the column has a CHECK requiring
-- bcrypt/argon2 shape, so a placeholder string is not an option and a
-- guessable one would be a backdoor). Accepting the invite sets a real
-- password and flips the status to 'active'.
--
-- Creating the user up front rather than at acceptance means:
--   - email uniqueness is enforced when the invite is sent, not discovered
--     later when two invites collide;
--   - an invited-but-not-joined account is visible in the admin list, so
--     "did I invite them?" is answerable;
--   - the role/centre CHECK is validated at invite time, so an impossible
--     assignment (staff with no centre) fails in front of the admin.

CREATE TABLE user_invites (
    id           BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id      BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Only the SHA-256 of the token is stored. The token is shown once, at
    -- creation, and is unrecoverable afterwards — the same reasoning as
    -- `sessions.token_hash` (WI-17): a database disclosure must not hand the
    -- attacker a way in.
    token_hash   BYTEA       NOT NULL,

    invited_by   BIGINT      REFERENCES users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ NOT NULL,
    accepted_at  TIMESTAMPTZ,

    CONSTRAINT user_invites_token_hash_key UNIQUE (token_hash),
    CONSTRAINT user_invites_hash_len CHECK (octet_length(token_hash) = 32),
    CONSTRAINT user_invites_expiry_after_creation CHECK (expires_at > created_at)
);

COMMENT ON TABLE user_invites IS
  'FR-66 invitations. One live invite per user is enforced by user_invites_one_open_idx; the token is stored only as a SHA-256 hash and shown once.';

-- At most one OUTSTANDING invite per user. Re-inviting is expected (the first
-- link expires, or is never received), so accepted and expired rows are
-- excluded rather than the whole user being blocked.
CREATE UNIQUE INDEX user_invites_one_open_idx
    ON user_invites (user_id) WHERE accepted_at IS NULL;

CREATE INDEX user_invites_expiry_idx ON user_invites (expires_at) WHERE accepted_at IS NULL;
