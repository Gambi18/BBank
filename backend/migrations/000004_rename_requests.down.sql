-- Reverse of 000004.
--
-- Structurally reversible. Values added by the up migration (status, preferred_date,
-- review columns) are lost, and `donor_name`/`last_donation` come back empty — they
-- were dropped as denormalisation and unverified data respectively. Rows removed by
-- the orphan guard are NOT restored here; they remain in migration_rejects with
-- their full payload and can be reinstated by hand if ever needed.
--
-- Note: repeated down/up cycles can add duplicate migration_rejects rows for any
-- record that survives the rollback. That is deliberate — a quarantine ledger that
-- forgets is worse than one that repeats.

ALTER TABLE donation_requests DROP CONSTRAINT IF EXISTS donation_requests_donor_fk;
ALTER TABLE donation_requests DROP CONSTRAINT IF EXISTS donation_requests_center_fk;

ALTER TABLE donation_requests
    ADD COLUMN IF NOT EXISTS donor_name    TEXT,
    ADD COLUMN IF NOT EXISTS last_donation DATE;

ALTER TABLE donation_requests
    DROP COLUMN IF EXISTS center_id,
    DROP COLUMN IF EXISTS preferred_date,
    DROP COLUMN IF EXISTS procedure,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS notes,
    DROP COLUMN IF EXISTS reviewed_by,
    DROP COLUMN IF EXISTS reviewed_at,
    DROP COLUMN IF EXISTS rejection_reason,
    DROP COLUMN IF EXISTS updated_at;

ALTER TABLE donation_requests ALTER COLUMN created_at TYPE TIMESTAMP
    USING created_at AT TIME ZONE 'Africa/Douala';
ALTER TABLE donation_requests ALTER COLUMN donor_id DROP NOT NULL;

-- IDENTITY -> SERIAL
ALTER TABLE donation_requests ALTER COLUMN id DROP IDENTITY IF EXISTS;
CREATE SEQUENCE IF NOT EXISTS requests_id_seq OWNED BY donation_requests.id;
SELECT setval('requests_id_seq', (SELECT COALESCE(max(id),0)+1 FROM donation_requests), false);
ALTER TABLE donation_requests ALTER COLUMN id SET DEFAULT nextval('requests_id_seq');

ALTER TABLE donation_requests
    ADD CONSTRAINT requests_donor_id_fkey FOREIGN KEY (donor_id) REFERENCES donors(id);

ALTER TABLE donation_requests RENAME TO requests;
