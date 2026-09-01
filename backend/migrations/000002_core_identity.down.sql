-- Reverse of 000002.
-- Safe: `donors` still holds every original row, so nothing is lost.
-- Order matters — donor_profiles references users.

ALTER TABLE IF EXISTS users DROP CONSTRAINT IF EXISTS users_hospital_fk;
DROP TABLE IF EXISTS donor_profiles;
DROP TABLE IF EXISTS migration_rejects;
DROP TABLE IF EXISTS users;
