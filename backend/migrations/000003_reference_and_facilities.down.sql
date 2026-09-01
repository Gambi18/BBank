-- Reverse of 000003.
-- The users.hospital_id FK and its paired CHECK must go before hospitals.

ALTER TABLE IF EXISTS users DROP CONSTRAINT IF EXISTS users_hospital_only_for_hospital_user;
ALTER TABLE IF EXISTS users DROP CONSTRAINT IF EXISTS users_hospital_fk;

DROP TABLE IF EXISTS abo_compatibility;
DROP TABLE IF EXISTS test_types;
DROP TABLE IF EXISTS policies;
DROP TABLE IF EXISTS storage_locations;
DROP TABLE IF EXISTS hospitals;
DROP TABLE IF EXISTS donation_centers;
