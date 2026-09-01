-- Reverse of 000005.
--
-- Structurally reversible. `appointment_date` is recovered from `scheduled_at` and
-- `donor_name` is re-derived by joining donor_profiles, so neither is lost. Status
-- and the check-in/completion timestamps are dropped — they did not exist before.
-- Rows removed by the orphan guard are not restored; they remain in
-- migration_rejects with their full payload.
--
-- Note: repeated down/up cycles can add duplicate migration_rejects rows for any
-- record that survives the rollback. That is deliberate — a quarantine ledger that
-- forgets is worse than one that repeats.

ALTER TABLE appointments DROP CONSTRAINT IF EXISTS appointments_request_key;
ALTER TABLE appointments DROP CONSTRAINT IF EXISTS appointments_request_fk;
ALTER TABLE appointments DROP CONSTRAINT IF EXISTS appointments_center_fk;
ALTER TABLE appointments DROP CONSTRAINT IF EXISTS appointments_donor_fk;

ALTER TABLE appointments
    ADD COLUMN IF NOT EXISTS appointment_date DATE,
    ADD COLUMN IF NOT EXISTS donor_name       TEXT;

UPDATE appointments a
   SET appointment_date = (a.scheduled_at AT TIME ZONE 'Africa/Douala')::date,
       donor_name       = p.full_name
  FROM donor_profiles p
 WHERE p.user_id = a.donor_id;

ALTER TABLE appointments
    DROP COLUMN IF EXISTS center_id,
    DROP COLUMN IF EXISTS scheduled_at,
    DROP COLUMN IF EXISTS procedure,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS checked_in_at,
    DROP COLUMN IF EXISTS completed_at,
    DROP COLUMN IF EXISTS cancelled_at,
    DROP COLUMN IF EXISTS cancellation_reason,
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS created_at,
    DROP COLUMN IF EXISTS updated_at;

ALTER TABLE appointments ALTER COLUMN donor_id DROP NOT NULL;

ALTER TABLE appointments ALTER COLUMN id DROP IDENTITY IF EXISTS;
CREATE SEQUENCE IF NOT EXISTS appointments_id_seq OWNED BY appointments.id;
SELECT setval('appointments_id_seq', (SELECT COALESCE(max(id),0)+1 FROM appointments), false);
ALTER TABLE appointments ALTER COLUMN id SET DEFAULT nextval('appointments_id_seq');

ALTER TABLE appointments
    ADD CONSTRAINT appointments_donor_id_fkey FOREIGN KEY (donor_id) REFERENCES donors(id);

ALTER TABLE appointments RENAME COLUMN donation_request_id TO request_id;
