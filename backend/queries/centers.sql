-- Donation centres (WI-24).

-- The scheduling configuration one booking needs, in one read.
--
-- `is_active` comes back rather than being filtered on, so the service can say
-- "that centre is closed" instead of "no such centre". A donor told their usual
-- centre does not exist will go looking for a bug; told it is closed, they will
-- pick another one.
--
-- name: GetCenterScheduling :one
SELECT id, code, name, capacity_per_slot, slot_minutes, opening_hours, timezone, is_active
FROM donation_centers WHERE id = $1;

-- name: ListCenters :many
SELECT id, code, name, address_line, city, region, phone, email,
       capacity_per_slot, slot_minutes, opening_hours, timezone, is_active,
       count(*) OVER () AS total
FROM donation_centers
WHERE (sqlc.narg('active_only')::boolean IS NOT TRUE OR is_active)
  AND (sqlc.narg('region')::text IS NULL OR region = sqlc.narg('region')::text)
-- `id` is the tiebreaker, and it is not decoration: two centres sharing a
-- name have no defined order without it, so paging could show one twice and
-- skip the other.
ORDER BY is_active DESC, name, id
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: GetCenter :one
SELECT id, code, name, address_line, city, region, phone, email,
       capacity_per_slot, slot_minutes, opening_hours, timezone, is_active
FROM donation_centers WHERE id = $1;

-- name: CreateCenter :one
INSERT INTO donation_centers (code, name, address_line, city, region, phone, email,
                              capacity_per_slot, slot_minutes, opening_hours, timezone, is_active)
VALUES (
    sqlc.arg('code'), sqlc.arg('name'), sqlc.arg('address_line'),
    sqlc.arg('city'), sqlc.arg('region'), sqlc.narg('phone'), sqlc.narg('email'),
    COALESCE(sqlc.narg('capacity_per_slot')::smallint, 4),
    COALESCE(sqlc.narg('slot_minutes')::smallint, 30),
    COALESCE(sqlc.narg('opening_hours')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.narg('timezone')::text, 'Africa/Douala'),
    -- Accepted on create, not dropped. `POST {"is_active": false}` used to
    -- return 201 with `"is_active": true`, so a centre opened before anybody was
    -- ready to staff it. Absent still means active, which is the sensible
    -- default for a centre somebody just took the trouble to add.
    COALESCE(sqlc.narg('is_active')::boolean, TRUE)
)
RETURNING id, code, name, address_line, city, region, phone, email,
          capacity_per_slot, slot_minutes, opening_hours, timezone, is_active;

-- PATCH semantics, for the same reason as the donor profile: a full-row UPDATE
-- behind a partial form writes NULL over every field the form did not send.
--
-- `code` is deliberately absent: it is the centre's stable identifier, printed
-- on labels and quoted in reports, and renaming it silently would orphan every
-- one of them. A centre that needs a different code is a different centre.
--
-- name: UpdateCenter :one
UPDATE donation_centers
   SET name              = COALESCE(sqlc.narg('name'), name),
       address_line      = COALESCE(sqlc.narg('address_line'), address_line),
       city              = COALESCE(sqlc.narg('city'), city),
       region            = COALESCE(sqlc.narg('region'), region),
       phone             = COALESCE(sqlc.narg('phone'), phone),
       email             = COALESCE(sqlc.narg('email')::citext, email),
       capacity_per_slot = COALESCE(sqlc.narg('capacity_per_slot')::smallint, capacity_per_slot),
       slot_minutes      = COALESCE(sqlc.narg('slot_minutes')::smallint, slot_minutes),
       opening_hours     = COALESCE(sqlc.narg('opening_hours')::jsonb, opening_hours),
       timezone          = COALESCE(sqlc.narg('timezone')::text, timezone),
       is_active         = COALESCE(sqlc.narg('is_active')::boolean, is_active)
 WHERE id = sqlc.arg('id')
RETURNING id, code, name, address_line, city, region, phone, email,
          capacity_per_slot, slot_minutes, opening_hours, timezone, is_active;

-- How full each slot is on one day, for a booking UI and for staff.
--
-- Counts only live appointments: a cancelled one has given its seat back.
--
-- name: SlotOccupancyOn :many
SELECT a.scheduled_at, count(*)::int AS booked
FROM appointments a
WHERE a.center_id = sqlc.arg('center_id')
  AND a.status <> 'cancelled'
  AND a.scheduled_at >= sqlc.arg('from_at')::timestamptz
  AND a.scheduled_at <  sqlc.arg('to_at')::timestamptz
GROUP BY a.scheduled_at
ORDER BY a.scheduled_at;

-- The centre a booking lands at when the caller names none.
--
-- `MAIN` by code, matching the `COALESCE` inside `CreateDonationRequest`. It is
-- a query rather than a constant so the guard and the insert cannot disagree
-- about which centre "the default" is — they already did once, and deactivating
-- MAIN stopped nothing.
--
-- name: DefaultCenterID :one
SELECT id FROM donation_centers WHERE code = 'MAIN';
