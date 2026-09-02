-- Donation requests (WI-22). Replaces the hand-written SQL in internal/legacy.
--
-- Every list query takes the scope predicate as parameters rather than by string
-- concatenation. The legacy version appended " AND r.donor_id = $1" to build the
-- authorization filter, which worked but put the security rule in a string. Here
-- the caller passes the scope explicitly and a NULL means "not scoped this way",
-- so a forgotten scope cannot silently widen the result set.

-- name: ListDonationRequests :many
SELECT r.id, r.donor_id, r.center_id, r.status, r.preferred_date, r.created_at,
       r.rejection_reason, r.notes, r.reviewed_at,
       p.full_name AS donor_name, p.legacy_last_donation
FROM donation_requests r
JOIN donor_profiles p ON p.user_id = r.donor_id
WHERE (sqlc.narg('status')::donation_request_status IS NULL OR r.status = sqlc.narg('status')::donation_request_status)
  AND (sqlc.narg('owner_id')::bigint  IS NULL OR r.donor_id  = sqlc.narg('owner_id')::bigint)
  AND (sqlc.narg('center_id')::bigint IS NULL OR r.center_id = sqlc.narg('center_id')::bigint)
ORDER BY r.id
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountDonationRequests :one
SELECT count(*)
FROM donation_requests r
WHERE (sqlc.narg('status')::donation_request_status IS NULL OR r.status = sqlc.narg('status')::donation_request_status)
  AND (sqlc.narg('owner_id')::bigint  IS NULL OR r.donor_id  = sqlc.narg('owner_id')::bigint)
  AND (sqlc.narg('center_id')::bigint IS NULL OR r.center_id = sqlc.narg('center_id')::bigint);

-- name: GetDonationRequest :one
SELECT r.id, r.donor_id, r.center_id, r.status, r.preferred_date, r.created_at,
       r.rejection_reason, r.notes, r.reviewed_at,
       p.full_name AS donor_name, p.legacy_last_donation
FROM donation_requests r
JOIN donor_profiles p ON p.user_id = r.donor_id
WHERE r.id = $1;

-- Locks the row for the duration of the approving/rejecting transaction.
--
-- FOR UPDATE is what makes the status check meaningful: without it two staff
-- approving the same request both read 'pending', both pass the check, and both
-- insert an appointment. The UNIQUE on appointments.donation_request_id would
-- catch the second as a constraint violation — a 500 rather than a clean 409.
-- name: GetDonationRequestForUpdate :one
SELECT id, donor_id, center_id, status, preferred_date, procedure
FROM donation_requests
WHERE id = $1
FOR UPDATE;

-- `procedure` is written, not left to the column default.
--
-- It used to be omitted, so the row was always `whole_blood` — while the FR-19
-- gate evaluated the procedure the CLIENT sent. A donor who gave whole blood
-- eight days ago could post `{"procedure":"apheresis_platelet"}`, pass the
-- 7-day platelet interval, and have a WHOLE-BLOOD request stored for approval.
-- The gate and the row have to describe the same donation or the gate guards
-- nothing.
--
-- `preferred_date` keeps its `CURRENT_DATE + 7` default in SQL, but the service
-- now resolves that date BEFORE the gate runs and passes it explicitly, so the
-- date checked and the date stored are the same one. The default is left here
-- for any caller that bypasses the service.
--
-- name: CreateDonationRequest :one
INSERT INTO donation_requests (donor_id, center_id, preferred_date, procedure, status, notes)
VALUES (
    sqlc.arg('donor_id'),
    COALESCE(sqlc.narg('center_id')::bigint, (SELECT id FROM donation_centers WHERE code = 'MAIN')),
    COALESCE(sqlc.narg('preferred_date')::date, CURRENT_DATE + 7),
    COALESCE(sqlc.narg('procedure')::donation_procedure, 'whole_blood'),
    'pending',
    sqlc.narg('notes')
)
RETURNING id, donor_id, center_id, status, preferred_date, procedure, created_at;

-- name: ApproveDonationRequest :exec
UPDATE donation_requests
   SET status = 'approved', reviewed_by = $2, reviewed_at = now()
 WHERE id = $1;

-- The schema CHECK (status <> 'rejected' OR rejection_reason IS NOT NULL) makes
-- a reasonless rejection impossible at the storage layer; the domain decides
-- whether the reason is one we recognise.
-- name: RejectDonationRequest :exec
UPDATE donation_requests
   SET status = 'rejected', rejection_reason = $2, notes = $3,
       reviewed_by = $4, reviewed_at = now()
 WHERE id = $1;

-- name: CancelDonationRequest :exec
UPDATE donation_requests
   SET status = 'cancelled', reviewed_by = $2, reviewed_at = now()
 WHERE id = $1;

-- name: HasOpenDonationRequest :one
SELECT EXISTS (
    SELECT 1 FROM donation_requests WHERE donor_id = $1 AND status = 'pending'
);
