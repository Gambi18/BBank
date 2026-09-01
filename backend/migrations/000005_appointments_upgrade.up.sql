-- 000005_appointments_upgrade (WI-14, schema §11.2 step 5, §11.5)
--
-- Adds the columns an appointment actually needs: a real timestamp, a center, a
-- status, and check-in/completion times. The legacy table had a bare DATE and no
-- status at all, so a no-show was unrepresentable (defect D3).
--
-- Times are Africa/Douala (OD-14). The legacy column carried no time component,
-- so 09:00 local is used and is explicitly synthetic.
--
-- Backfilled status is INFERRED FROM DATE: past becomes 'completed', future stays
-- 'scheduled'. This is a guess. No-shows are indistinguishable from attendances in
-- the legacy data because no-shows could not be recorded. Every affected row is
-- listed in migration_rejects for future audit.

ALTER TABLE appointments RENAME COLUMN request_id TO donation_request_id;
ALTER TABLE appointments
    ADD COLUMN center_id           BIGINT,
    ADD COLUMN scheduled_at        TIMESTAMPTZ,
    ADD COLUMN procedure           donation_procedure NOT NULL DEFAULT 'whole_blood',
    ADD COLUMN status              appointment_status NOT NULL DEFAULT 'scheduled',
    ADD COLUMN checked_in_at       TIMESTAMPTZ,
    ADD COLUMN completed_at        TIMESTAMPTZ,
    ADD COLUMN cancelled_at        TIMESTAMPTZ,
    ADD COLUMN cancellation_reason TEXT,
    ADD COLUMN created_by          BIGINT REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN updated_at          TIMESTAMPTZ NOT NULL DEFAULT now();

-- confirmRequest hard-DELETEd the parent row, so historic values are dangling.
-- Record the loss before destroying the evidence of it.
INSERT INTO migration_rejects (source_table, source_id, reason, payload)
SELECT 'appointments', a.id, 'dangling_donation_request_id', to_jsonb(a)
FROM appointments a
WHERE a.donation_request_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM donation_requests r WHERE r.id = a.donation_request_id);

UPDATE appointments a SET donation_request_id = NULL
WHERE a.donation_request_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM donation_requests r WHERE r.id = a.donation_request_id);

-- ---------------------------------------------------------------------------
-- Orphan guard (added beyond schema §11.5).
--
-- §11.5 quarantines a dangling donation_request_id but not a dangling donor_id.
-- 000002 quarantines any donor without a usable email or password hash, so an
-- appointment belonging to such a donor aborts this migration on
-- appointments_donor_fk. Verified against real data: appointment 1 references
-- donor 1, which 000002 quarantined as missing_password.
--
-- Quarantine the appointment with its full payload, then remove it. The donor
-- survives in the legacy `donors` table and in migration_rejects, so the pair
-- stays reconstructable by hand.
-- ---------------------------------------------------------------------------
INSERT INTO migration_rejects (source_table, source_id, reason, payload)
SELECT 'appointments', a.id, 'donor_did_not_migrate', to_jsonb(a)
FROM appointments a
WHERE a.donor_id IS NULL
   OR NOT EXISTS (SELECT 1 FROM donor_profiles p WHERE p.user_id = a.donor_id);

DELETE FROM appointments a
WHERE a.donor_id IS NULL
   OR NOT EXISTS (SELECT 1 FROM donor_profiles p WHERE p.user_id = a.donor_id);

UPDATE appointments
   SET scheduled_at  = (appointment_date + TIME '09:00') AT TIME ZONE 'Africa/Douala',
       center_id     = (SELECT id FROM donation_centers WHERE code = 'MAIN'),
       status        = CASE WHEN appointment_date < CURRENT_DATE THEN 'completed'::appointment_status
                            ELSE 'scheduled'::appointment_status END,
       checked_in_at = CASE WHEN appointment_date < CURRENT_DATE
                            THEN (appointment_date + TIME '09:00') AT TIME ZONE 'Africa/Douala' END,
       completed_at  = CASE WHEN appointment_date < CURRENT_DATE
                            THEN (appointment_date + TIME '09:30') AT TIME ZONE 'Africa/Douala' END;

ALTER TABLE appointments
    ALTER COLUMN scheduled_at SET NOT NULL,
    ALTER COLUMN center_id    SET NOT NULL,
    ALTER COLUMN donor_id     SET NOT NULL,
    DROP COLUMN appointment_date,
    DROP COLUMN donor_name;

ALTER TABLE appointments DROP CONSTRAINT appointments_donor_id_fkey;
ALTER TABLE appointments
    ADD CONSTRAINT appointments_donor_fk   FOREIGN KEY (donor_id)
        REFERENCES donor_profiles(user_id) ON DELETE RESTRICT,
    ADD CONSTRAINT appointments_center_fk  FOREIGN KEY (center_id)
        REFERENCES donation_centers(id) ON DELETE RESTRICT,
    ADD CONSTRAINT appointments_request_fk FOREIGN KEY (donation_request_id)
        REFERENCES donation_requests(id) ON DELETE RESTRICT,
    ADD CONSTRAINT appointments_request_key UNIQUE (donation_request_id);

ALTER TABLE appointments ALTER COLUMN id DROP DEFAULT;
DROP SEQUENCE appointments_id_seq;
ALTER TABLE appointments ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY;
SELECT setval(pg_get_serial_sequence('appointments','id'),
              (SELECT COALESCE(max(id),0)+1 FROM appointments), false);
