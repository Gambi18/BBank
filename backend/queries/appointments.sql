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

-- Locks the row for a cancel or reschedule, so the status check and the write
-- cannot straddle another transaction's change.
-- name: GetAppointmentForUpdate :one
SELECT id, donation_request_id, donor_id, center_id, status, scheduled_at
FROM appointments
WHERE id = $1
FOR UPDATE;

-- The column is `cancellation_reason` (read from the schema, not guessed), and
-- there is no `cancelled_by`: who did it belongs in `audit_log`, which WI-27
-- writes by trigger so no application path can skip it.
-- name: CancelAppointment :exec
UPDATE appointments
   SET status = 'cancelled', cancelled_at = now(), cancellation_reason = $2
WHERE id = $1;

-- name: RescheduleAppointment :one
UPDATE appointments
   SET scheduled_at = $2
WHERE id = $1
RETURNING id, donation_request_id, donor_id, center_id, status, scheduled_at;

-- The daily no-show sweep (FR-13).
--
-- Idempotent by construction: it only touches rows still `scheduled` whose time
-- has passed, so a second run in the same minute matches nothing. It writes NO
-- deferral — not showing up is an administrative fact, and turning it into a
-- clinical one would put a mark on a donor's record that no clinician made.
-- name: SweepNoShows :execrows
UPDATE appointments
   SET status = 'no_show'
WHERE status = 'scheduled'
  AND scheduled_at < now() - sqlc.arg('grace')::interval;
