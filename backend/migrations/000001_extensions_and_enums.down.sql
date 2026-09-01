-- Reverse of 000001.
-- Extensions are intentionally left in place: they are harmless, `CREATE EXTENSION
-- IF NOT EXISTS` makes re-running idempotent, and dropping citext would fail while
-- any column still uses it.

DROP TYPE IF EXISTS notification_status;
DROP TYPE IF EXISTS notification_channel;
DROP TYPE IF EXISTS storage_kind;
DROP TYPE IF EXISTS issuance_outcome;
DROP TYPE IF EXISTS crossmatch_result;
DROP TYPE IF EXISTS urgency_level;
DROP TYPE IF EXISTS blood_request_status;
DROP TYPE IF EXISTS test_result_status;
DROP TYPE IF EXISTS blood_unit_status;
DROP TYPE IF EXISTS deferral_type;
DROP TYPE IF EXISTS screening_outcome;
DROP TYPE IF EXISTS donation_request_status;
DROP TYPE IF EXISTS appointment_status;
DROP TYPE IF EXISTS hospital_status;
DROP TYPE IF EXISTS user_status;
DROP TYPE IF EXISTS user_role;
DROP TYPE IF EXISTS donation_procedure;
DROP TYPE IF EXISTS component_type;
DROP TYPE IF EXISTS gender;
DROP TYPE IF EXISTS rhesus;
DROP TYPE IF EXISTS blood_group;
