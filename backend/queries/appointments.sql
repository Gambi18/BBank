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
-- Books the appointment into the LOWEST FREE SEAT of its slot (WI-24).
--
-- The seat is chosen in the same statement that inserts it, and the partial
-- unique index `appointments_one_per_slot_seat` is what actually holds under
-- concurrency: two callers can both compute seat 2, and exactly one of them
-- commits it. The other gets 23505 and the service asks again, which is why
-- this query is safe to retry — see `AppointmentSeatRetries`.
--
-- `generate_series(1, capacity) EXCEPT taken` rather than `count(*) + 1`,
-- because seats are freed by cancellation: with seats 1 and 3 live, the next
-- booking belongs in 2, and `count(*) + 1` would say 3 and collide.
--
-- An empty result means the slot is full. That is a real answer, not an error —
-- `:one` turns it into pgx.ErrNoRows and the service maps it to a 409 naming the
-- slot.
--
-- The centre's capacity, activity and timezone are read here rather than passed
-- in, so a caller cannot widen a slot by sending a bigger number. The trigger
-- rejects an out-of-range seat anyway; this keeps the honest path from ever
-- producing one.
--
-- name: CreateAppointmentForRequest :one
WITH centre AS (
    SELECT id, capacity_per_slot FROM donation_centers
     WHERE id = sqlc.arg('center_id') AND is_active
),
free AS (
    SELECT gs.seat
    FROM centre, generate_series(1, centre.capacity_per_slot) AS gs(seat)
    WHERE NOT EXISTS (
        SELECT 1 FROM appointments a
         WHERE a.center_id    = centre.id
           AND a.scheduled_at = sqlc.arg('scheduled_at')::timestamptz
           AND a.slot_seat    = gs.seat
           AND a.status <> 'cancelled'
    )
    ORDER BY gs.seat
    LIMIT 1
)
INSERT INTO appointments (donation_request_id, donor_id, center_id, scheduled_at, slot_seat, status, created_by)
SELECT
    sqlc.arg('donation_request_id'),
    sqlc.arg('donor_id'),
    sqlc.arg('center_id'),
    sqlc.arg('scheduled_at')::timestamptz,
    free.seat,
    'scheduled',
    sqlc.narg('created_by')
FROM free
RETURNING id, donation_request_id, donor_id, center_id, scheduled_at, slot_seat, status;

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
