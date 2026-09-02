-- User administration (WI-18, FR-66).

-- name: ListUsers :many
SELECT u.id, u.email, u.role, u.status, u.center_id, u.hospital_id,
       u.last_login_at, u.created_at,
       (i.id IS NOT NULL)::boolean AS invite_pending
FROM users u
LEFT JOIN user_invites i ON i.user_id = u.id AND i.accepted_at IS NULL AND i.expires_at > now()
WHERE (sqlc.narg('role')::user_role IS NULL OR u.role = sqlc.narg('role')::user_role)
  AND (sqlc.narg('status')::user_status IS NULL OR u.status = sqlc.narg('status')::user_status)
  AND (sqlc.narg('search')::text IS NULL OR u.email::text ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY u.id
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountUsers :one
SELECT count(*) FROM users u
WHERE (sqlc.narg('role')::user_role IS NULL OR u.role = sqlc.narg('role')::user_role)
  AND (sqlc.narg('status')::user_status IS NULL OR u.status = sqlc.narg('status')::user_status)
  AND (sqlc.narg('search')::text IS NULL OR u.email::text ILIKE '%' || sqlc.narg('search')::text || '%');

-- name: GetUser :one
SELECT id, email, role, status, center_id, hospital_id, last_login_at, created_at
FROM users WHERE id = $1;

-- name: CountAdmins :one
SELECT count(*) FROM users WHERE role = 'admin' AND status = 'active';

-- The same count, but holding a row lock on every active admin.
--
-- `ensureNotLastAdmin` counted outside any transaction, so two concurrent
-- demotions of the last two admins both saw 2 and both succeeded — leaving zero
-- admins, from which there is no recovery short of SQL. Locking the rows the
-- decision depends on serialises the two callers.
-- name: LockAndCountAdmins :one
SELECT count(*) FROM (
    SELECT 1 FROM users WHERE role = 'admin' AND status = 'active' FOR UPDATE
) AS locked;

-- Re-inviting is expected: the first link expires, or never arrives. The old
-- account row is reused rather than refused, so the user id, and anything
-- already pointing at it, survives.
-- name: ResetInvitedUser :exec
UPDATE users
   SET password_hash = $2, role = $3, status = 'pending_verification',
       center_id = sqlc.narg('center_id'), hospital_id = sqlc.narg('hospital_id'),
       deactivated_at = NULL
WHERE id = $1 AND status = 'pending_verification';

-- name: GetUserByEmail :one
SELECT id, email, role, status, center_id, hospital_id, last_login_at, created_at
FROM users WHERE email = sqlc.arg('email')::citext;

-- Creates the account an invite belongs to. The password hash is a discarded
-- random secret: the column CHECK requires bcrypt/argon2 shape, so it cannot be
-- a placeholder string, and it must not be anything guessable.
-- name: CreateInvitedUser :one
INSERT INTO users (email, password_hash, role, status, center_id, hospital_id)
VALUES ($1, $2, $3, 'pending_verification', sqlc.narg('center_id'), sqlc.narg('hospital_id'))
RETURNING id, email, role, status, center_id, hospital_id, created_at;

-- name: CreateInvite :one
INSERT INTO user_invites (user_id, token_hash, invited_by, expires_at)
VALUES ($1, $2, sqlc.narg('invited_by'), $3)
RETURNING id, user_id, expires_at;

-- name: GetOpenInviteByTokenHash :one
SELECT i.id, i.user_id, i.expires_at, u.email, u.role, u.status
FROM user_invites i JOIN users u ON u.id = i.user_id
WHERE i.token_hash = $1 AND i.accepted_at IS NULL;

-- name: RevokeOpenInvitesForUser :exec
UPDATE user_invites SET accepted_at = now()
WHERE user_id = $1 AND accepted_at IS NULL;

-- name: AcceptInvite :exec
UPDATE user_invites SET accepted_at = now() WHERE id = $1;

-- name: ActivateUserWithPassword :exec
UPDATE users SET password_hash = $2, status = 'active', deactivated_at = NULL
WHERE id = $1;

-- name: SetUserRole :exec
UPDATE users SET role = $2, center_id = sqlc.narg('center_id'), hospital_id = sqlc.narg('hospital_id')
WHERE id = $1;

-- `deactivated_at` is kept in step with the status by users_deactivated_sync,
-- so it is set and cleared here rather than left to a separate statement that
-- could be forgotten.
-- name: SetUserStatus :exec
UPDATE users
   SET status = $2,
       deactivated_at = CASE WHEN $2::user_status = 'deactivated' THEN now() ELSE NULL END
WHERE id = $1;
