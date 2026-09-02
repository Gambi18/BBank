-- Donor queries. The strangler pilot for WI-11: donors move to the layered
-- structure first, everything else keeps working through internal/legacy.
--
-- Note these read `users` JOIN `donor_profiles`, never the legacy `donors` table.

-- name: GetDonorByID :one
SELECT u.id, u.email, u.role, u.status,
       p.full_name, p.date_of_birth, p.gender, p.blood_group, p.rhesus,
       p.contact_phone, p.address_line, p.city, p.region, p.national_id,
       p.emergency_contact_name, p.emergency_contact_phone,
       p.total_donations, p.legacy_last_donation
FROM users u
JOIN donor_profiles p ON p.user_id = u.id
WHERE u.id = $1 AND u.role = 'donor';

-- name: ListDonors :many
SELECT u.id, u.email, u.status,
       p.full_name, p.blood_group, p.rhesus, p.contact_phone, p.total_donations
FROM users u
JOIN donor_profiles p ON p.user_id = u.id
WHERE u.role = 'donor'
  AND (sqlc.narg('search')::text IS NULL
       OR p.full_name ILIKE '%' || sqlc.narg('search')::text || '%'
       OR u.email::text ILIKE '%' || sqlc.narg('search')::text || '%')
ORDER BY p.full_name
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountDonors :one
SELECT count(*) FROM users u
JOIN donor_profiles p ON p.user_id = u.id
WHERE u.role = 'donor'
  AND (sqlc.narg('search')::text IS NULL
       OR p.full_name ILIKE '%' || sqlc.narg('search')::text || '%'
       OR u.email::text ILIKE '%' || sqlc.narg('search')::text || '%');

-- name: GetDonorEligibility :one
SELECT donor_id, full_name, blood_group, rhesus, age_years, last_donated_at,
       donations_last_12m, permanently_deferred, deferred_until,
       next_eligible_on, is_eligible_today, reason
FROM donor_eligibility WHERE donor_id = $1;

-- name: CreateDonor :one
WITH new_user AS (
    INSERT INTO users (email, password_hash, role, status)
    VALUES ($1, $2, 'donor', 'active')
    RETURNING id
)
INSERT INTO donor_profiles (user_id, full_name, date_of_birth, gender,
                            blood_group, rhesus, contact_phone, address_line)
SELECT id, $3, $4, $5, $6, $7, $8, $9 FROM new_user
RETURNING user_id;

-- PATCH semantics: a field the caller omits keeps its stored value.
--
-- This was a full-row UPDATE of 12 columns, so every save wrote NULL over any
-- field the form did not send — and the donor settings form sends neither
-- national_id nor either emergency contact, so saving a phone number silently
-- erased the person to call in an emergency. COALESCE makes "absent" and
-- "cleared" different things, which is what PATCH means.
--
-- date_of_birth is NOT NULL in the schema, so COALESCE also stops an omitted
-- date becoming a 23502 the error mapper does not translate (a bare 500).
-- name: UpdateDonorProfile :exec
UPDATE donor_profiles
   SET full_name               = COALESCE(sqlc.narg('full_name'), full_name),
       date_of_birth           = COALESCE(sqlc.narg('date_of_birth')::date, date_of_birth),
       gender                  = COALESCE(sqlc.narg('gender')::gender, gender),
       blood_group             = COALESCE(sqlc.narg('blood_group')::blood_group, blood_group),
       rhesus                  = COALESCE(sqlc.narg('rhesus')::rhesus, rhesus),
       contact_phone           = COALESCE(sqlc.narg('contact_phone'), contact_phone),
       address_line            = COALESCE(sqlc.narg('address_line'), address_line),
       city                    = COALESCE(sqlc.narg('city'), city),
       region                  = COALESCE(sqlc.narg('region'), region),
       national_id             = COALESCE(sqlc.narg('national_id'), national_id),
       emergency_contact_name  = COALESCE(sqlc.narg('emergency_contact_name'), emergency_contact_name),
       emergency_contact_phone = COALESCE(sqlc.narg('emergency_contact_phone'), emergency_contact_phone)
 WHERE user_id = sqlc.arg('user_id');
