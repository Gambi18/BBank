-- 000004_rename_requests (WI-14, schema §11.2 step 4, §11.4)
--
-- `requests` becomes `donation_requests`. The old name was actively misleading:
-- in this codebase a "request" meant a donor asking for a slot, whereas in every
-- real blood bank a request is a hospital asking for units. `blood_requests`
-- (000008) is the demand side; these two must never be confused again.
--
-- Timestamps are interpreted as Africa/Douala (OD-14, resolved 2026-09-01).
-- Cameroon has never observed DST, so no ambiguous or skipped hour can arise.
--
-- Every surviving row keeps status='pending', which is correct: under the old
-- model an approved request was DELETEd, so anything still present is unapproved.

ALTER TABLE requests RENAME TO donation_requests;
ALTER SEQUENCE requests_id_seq RENAME TO donation_requests_id_seq;

ALTER TABLE donation_requests
    ADD COLUMN center_id        BIGINT,
    ADD COLUMN preferred_date   DATE,
    ADD COLUMN procedure        donation_procedure      NOT NULL DEFAULT 'whole_blood',
    ADD COLUMN status           donation_request_status NOT NULL DEFAULT 'pending',
    ADD COLUMN notes            TEXT,
    ADD COLUMN reviewed_by      BIGINT REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN reviewed_at      TIMESTAMPTZ,
    ADD COLUMN rejection_reason TEXT,
    ADD COLUMN updated_at       TIMESTAMPTZ NOT NULL DEFAULT now();

UPDATE donation_requests
   SET center_id      = (SELECT id FROM donation_centers WHERE code = 'MAIN'),
       preferred_date = COALESCE(preferred_date, (created_at + INTERVAL '7 days')::date);

-- ---------------------------------------------------------------------------
-- Orphan guard (added beyond schema §11.4).
--
-- The new FK points at donor_profiles(user_id), but 000002 quarantines any donor
-- lacking a usable email or password hash. A request belonging to such a donor
-- would abort this migration on the FK. Quarantine the row — with its full
-- payload — and remove it. Nothing is lost: the donor still exists in the legacy
-- `donors` table and in migration_rejects, so the pair is reconstructable.
-- ---------------------------------------------------------------------------
INSERT INTO migration_rejects (source_table, source_id, reason, payload)
SELECT 'requests', r.id, 'donor_did_not_migrate', to_jsonb(r)
FROM donation_requests r
WHERE r.donor_id IS NULL
   OR NOT EXISTS (SELECT 1 FROM donor_profiles p WHERE p.user_id = r.donor_id);

DELETE FROM donation_requests r
WHERE r.donor_id IS NULL
   OR NOT EXISTS (SELECT 1 FROM donor_profiles p WHERE p.user_id = r.donor_id);

ALTER TABLE donation_requests
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'Africa/Douala',
    ALTER COLUMN created_at SET DEFAULT now(),
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN center_id      SET NOT NULL,
    ALTER COLUMN preferred_date SET NOT NULL,
    ALTER COLUMN donor_id       SET NOT NULL;

ALTER TABLE donation_requests DROP CONSTRAINT requests_donor_id_fkey;
ALTER TABLE donation_requests
    ADD CONSTRAINT donation_requests_donor_fk  FOREIGN KEY (donor_id)
        REFERENCES donor_profiles(user_id) ON DELETE RESTRICT,
    ADD CONSTRAINT donation_requests_center_fk FOREIGN KEY (center_id)
        REFERENCES donation_centers(id) ON DELETE RESTRICT;

ALTER TABLE donation_requests DROP COLUMN donor_name, DROP COLUMN last_donation;

-- SERIAL -> IDENTITY
ALTER TABLE donation_requests ALTER COLUMN id DROP DEFAULT;
DROP SEQUENCE donation_requests_id_seq;
ALTER TABLE donation_requests ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY;
SELECT setval(pg_get_serial_sequence('donation_requests','id'),
              (SELECT COALESCE(max(id),0)+1 FROM donation_requests), false);
