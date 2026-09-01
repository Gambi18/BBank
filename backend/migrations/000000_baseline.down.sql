-- Reverse of 000000_baseline.
--
-- DESTRUCTIVE: drops the three legacy tables and everything in them. This exists so
-- the migration is genuinely reversible in development and CI (where `up -> down -> up`
-- is asserted). Do not run it against an environment holding real donor data.
--
-- Order matters: requests and appointments both reference donors.

DROP TABLE IF EXISTS appointments;
DROP TABLE IF EXISTS requests;
DROP TABLE IF EXISTS donors;
