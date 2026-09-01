-- Appointments (WI-22). Replaces the hand-written SQL in internal/legacy.
--
-- `scheduled_at` is TIMESTAMPTZ; the legacy handlers projected it to a date in
-- Africa/Douala inside every query. That timezone is an assumption (schema Q6),
-- so it is applied in exactly one place per query rather than scattered, and the
-- full timestamp is returned alongside so a client is never forced to guess.

-- name: ListAppointments :many
SELECT a.id, COALESCE(a.donation_request_id, 0)::bigint AS donation_request_id,
       a.donor_id, a.center_id, a.status, a.scheduled_at,
       (a.scheduled_at AT TIME ZONE 'Africa/Douala')::date AS scheduled_date,
       a.checked_in_at, a.completed_at, a.cancelled_at,
       p.full_name AS donor_name
FROM appointments a
JOIN donor_profiles p ON p.user_id = a.donor_id
WHERE (sqlc.narg('owner_id')::bigint  IS NULL OR a.donor_id  = sqlc.narg('owner_id')::bigint)
  AND (sqlc.narg('center_id')::bigint IS NULL OR a.center_id = sqlc.narg('center_id')::bigint)
  AND (sqlc.narg('donor_filter')::bigint IS NULL OR a.donor_id = sqlc.narg('donor_filter')::bigint)
ORDER BY a.scheduled_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountAppointments :one
SELECT count(*)
FROM appointments a
WHERE (sqlc.narg('owner_id')::bigint  IS NULL OR a.donor_id  = sqlc.narg('owner_id')::bigint)
  AND (sqlc.narg('center_id')::bigint IS NULL OR a.center_id = sqlc.narg('center_id')::bigint)
  AND (sqlc.narg('donor_filter')::bigint IS NULL OR a.donor_id = sqlc.narg('donor_filter')::bigint);

-- name: GetAppointment :one
SELECT a.id, COALESCE(a.donation_request_id, 0)::bigint AS donation_request_id,
       a.donor_id, a.center_id, a.status, a.scheduled_at,
       (a.scheduled_at AT TIME ZONE 'Africa/Douala')::date AS scheduled_date,
       a.checked_in_at, a.completed_at, a.cancelled_at,
       p.full_name AS donor_name
FROM appointments a
JOIN donor_profiles p ON p.user_id = a.donor_id
WHERE a.id = $1;

-- Created only by approving a request, inside that request's transaction — never
-- on its own. An appointment with no originating request is the shape of data
-- the old `confirm` left behind when it deleted the row it came from.
-- name: CreateAppointmentForRequest :one
INSERT INTO appointments (donation_request_id, donor_id, center_id, scheduled_at, status, created_by)
VALUES (
    sqlc.arg('donation_request_id'),
    sqlc.arg('donor_id'),
    sqlc.arg('center_id'),
    (sqlc.arg('scheduled_date')::date + TIME '09:00') AT TIME ZONE 'Africa/Douala',
    'scheduled',
    sqlc.narg('created_by')
)
RETURNING id, donation_request_id, donor_id, center_id, scheduled_at, status;
