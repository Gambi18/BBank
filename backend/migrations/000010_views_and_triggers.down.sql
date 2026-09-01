-- Reverse of 000010. Views first (they may depend on functions), then triggers,
-- then the functions themselves. Names are taken from the up migration, not memory.

DROP VIEW IF EXISTS unit_provenance CASCADE;
DROP VIEW IF EXISTS donor_eligibility CASCADE;
DROP VIEW IF EXISTS inventory_summary CASCADE;
DROP VIEW IF EXISTS active_policies CASCADE;
DROP TRIGGER IF EXISTS audit_log_append_only ON audit_log;
DROP TRIGGER IF EXISTS unit_status_events_append_only ON unit_status_events;
DROP TRIGGER IF EXISTS donations_sync_counter ON donations;
DROP TRIGGER IF EXISTS blood_units_log_status ON blood_units;
DROP TRIGGER IF EXISTS blood_units_bump_version ON blood_units;
DROP TRIGGER IF EXISTS blood_units_guard_release ON blood_units;
DROP TRIGGER IF EXISTS blood_requests_set_updated_at ON blood_requests;
DROP TRIGGER IF EXISTS appointments_set_updated_at ON appointments;
DROP TRIGGER IF EXISTS donation_requests_set_updated_at ON donation_requests;
DROP TRIGGER IF EXISTS donor_profiles_set_updated_at ON donor_profiles;
DROP TRIGGER IF EXISTS users_set_updated_at ON users;
DROP FUNCTION IF EXISTS forbid_mutation() CASCADE;
DROP FUNCTION IF EXISTS sync_donor_donation_count() CASCADE;
DROP FUNCTION IF EXISTS bump_unit_version() CASCADE;
DROP FUNCTION IF EXISTS guard_unit_release() CASCADE;
DROP FUNCTION IF EXISTS log_unit_status_change() CASCADE;
DROP FUNCTION IF EXISTS set_updated_at() CASCADE;
DROP FUNCTION IF EXISTS current_actor_id() CASCADE;
