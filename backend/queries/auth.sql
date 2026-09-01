-- Authentication queries (WI-17).

-- name: GetUserForLogin :one
SELECT u.id, u.email, u.password_hash, u.role, u.status, u.token_version,
       u.center_id, u.hospital_id, u.failed_login_count, u.locked_until
FROM users u
WHERE u.email = sqlc.arg('email')::citext;

-- name: GetUserForToken :one
SELECT id, role, status, token_version, center_id, hospital_id
FROM users WHERE id = $1;

-- name: TouchLastLogin :exec
UPDATE users SET last_login_at = now(), failed_login_count = 0, locked_until = NULL
WHERE id = $1;

-- name: RecordFailedLogin :exec
UPDATE users SET failed_login_count = failed_login_count + 1
WHERE email = sqlc.arg('email')::citext;

-- name: BumpTokenVersion :exec
-- Invalidates every outstanding access token for this user immediately.
UPDATE users SET token_version = token_version + 1 WHERE id = $1;

-- name: CreateSession :one
INSERT INTO sessions (public_id, user_id, family_id, token_hash, expires_at, ip, user_agent)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, public_id, family_id, expires_at;

-- name: GetSessionByTokenHash :one
SELECT id, public_id, user_id, family_id, token_hash, issued_at, expires_at,
       rotated_at, revoked_at, revoked_reason
FROM sessions WHERE token_hash = $1;

-- name: MarkSessionRotated :exec
UPDATE sessions SET rotated_at = now() WHERE id = $1 AND rotated_at IS NULL;

-- name: RevokeSessionFamily :execrows
-- Reuse detection: one presented-twice token revokes every live row in the family.
UPDATE sessions
   SET revoked_at = now(), revoked_reason = sqlc.arg('reason')
 WHERE family_id = sqlc.arg('family_id') AND revoked_at IS NULL;

-- name: RevokeSessionsForUser :execrows
UPDATE sessions
   SET revoked_at = now(), revoked_reason = sqlc.arg('reason')
 WHERE user_id = sqlc.arg('user_id') AND revoked_at IS NULL;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions WHERE expires_at < now() - INTERVAL '30 days';
