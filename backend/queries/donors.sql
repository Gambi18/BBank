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

-- name: UpdateDonorProfile :exec
UPDATE donor_profiles
   SET full_name = $2, date_of_birth = $3, gender = $4,
       blood_group = $5, rhesus = $6, contact_phone = $7,
       address_line = $8, city = $9, region = $10,
       national_id = $11, emergency_contact_name = $12, emergency_contact_phone = $13
 WHERE user_id = $1;
