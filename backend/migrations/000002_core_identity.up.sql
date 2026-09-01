-- 000002_core_identity (WI-13, schema §11.2 step 2, §11.3)
-- Creates users, donor_profiles and migration_rejects, then backfills from the
-- legacy `donors` table. `donors` is left in place and untouched — it is not
-- dropped until 000013, a deliberately separate release (WI-37).
--
-- Critical invariant: donors.id survives as users.id, because requests.donor_id
-- and appointments.donor_id reference it and are not being rewritten here.
--
-- Rows that cannot become a user (no email or no password) are quarantined in
-- migration_rejects, never silently dropped.

-- Every authenticated principal in the system, whatever their role.
CREATE TABLE users (
    id                 BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id          UUID        NOT NULL DEFAULT uuidv7(),
    email              CITEXT      NOT NULL,
    password_hash      TEXT        NOT NULL,
    role               user_role   NOT NULL DEFAULT 'donor',
    status             user_status NOT NULL DEFAULT 'pending_verification',
    phone              TEXT,
    hospital_id        BIGINT,                     -- FK added after hospitals exists
    last_login_at      TIMESTAMPTZ,
    failed_login_count SMALLINT    NOT NULL DEFAULT 0,
    locked_until       TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    deactivated_at     TIMESTAMPTZ,
    CONSTRAINT users_email_key        UNIQUE (email),
    CONSTRAINT users_public_id_key    UNIQUE (public_id),
    CONSTRAINT users_email_format     CHECK (email ~ '^[^@[:space:]]+@[^@[:space:]]+\.[^@[:space:]]+$'),
    CONSTRAINT users_hash_not_plain   CHECK (password_hash ~ '^\$(2[aby]|argon2(i|d|id))\$'),
    CONSTRAINT users_deactivated_sync CHECK ((status = 'deactivated') = (deactivated_at IS NOT NULL))
);

-- Donor-specific attributes. 1:1 with users where role = 'donor'.
CREATE TABLE donor_profiles (
    user_id                 BIGINT      PRIMARY KEY REFERENCES users(id) ON DELETE RESTRICT,
    full_name               TEXT        NOT NULL,
    date_of_birth           DATE        NOT NULL,
    gender                  gender      NOT NULL DEFAULT 'undisclosed',
    blood_group             blood_group,
    rhesus                  rhesus,
    blood_group_verified_at TIMESTAMPTZ,
    contact_phone           TEXT        NOT NULL,
    address_line            TEXT,
    city                    TEXT,
    region                  TEXT,
    national_id             TEXT,
    emergency_contact_name  TEXT,
    emergency_contact_phone TEXT,
    total_donations         INTEGER     NOT NULL DEFAULT 0,
    legacy_last_donation    DATE,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT donor_profiles_national_id_key UNIQUE (national_id),
    CONSTRAINT donor_profiles_dob_sane        CHECK (date_of_birth > DATE '1900-01-01'),
    CONSTRAINT donor_profiles_abo_paired      CHECK ((blood_group IS NULL) = (rhesus IS NULL)),
    CONSTRAINT donor_profiles_verified_needs_group
        CHECK (blood_group_verified_at IS NULL OR blood_group IS NOT NULL),
    CONSTRAINT donor_profiles_total_nonneg    CHECK (total_donations >= 0)
);

-- Rows that cannot become a user are quarantined, never silently dropped.
CREATE TABLE migration_rejects (
    id           BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_table TEXT        NOT NULL,
    source_id    BIGINT      NOT NULL,
    reason       TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Pre-flight guard. donors.email is TEXT; users.email is CITEXT. A pair differing
-- only in case collides on the unique index. Aborting with a named error beats an
-- opaque constraint violation — a duplicate account needs a human decision.
--
-- OPERATOR NOTE: if this fires, golang-migrate marks the database dirty at
-- version 2 (standard behaviour for any failed migration; the transaction itself
-- rolled back, so no partial schema exists). Recover with:
--     migrate ... force 1      # back to the last cleanly applied version
--     <resolve the duplicates in `donors`>
--     migrate ... up
-- Verified 2026-09-01 by injecting a case-only duplicate and recovering.
DO $$
DECLARE dupes TEXT;
BEGIN
    SELECT string_agg(e, ', ') INTO dupes
    FROM (SELECT lower(btrim(email)) AS e FROM donors
          WHERE email IS NOT NULL AND btrim(email) <> ''
          GROUP BY 1 HAVING count(*) > 1) x;
    IF dupes IS NOT NULL THEN
        RAISE EXCEPTION 'Case-insensitive duplicate donor emails block this migration: %. Resolve them in `donors` before re-running.', dupes;
    END IF;
END $$;

-- Quarantine predicate. The schema doc's prose states that a row whose password is
-- not a recognised hash format is quarantined; the original SQL only tested for
-- NULL/empty, so a non-empty plaintext password would have passed the filter and
-- then aborted the whole migration on users_hash_not_plain. Both the email format
-- and the hash format are now checked here, so the reject table and the users
-- filter agree exactly and the migration cannot fail on a bad legacy row.
INSERT INTO migration_rejects (source_table, source_id, reason, payload)
SELECT 'donors', d.id,
       CASE
           WHEN d.email IS NULL OR btrim(d.email) = ''                       THEN 'missing_email'
           WHEN lower(btrim(d.email)) !~ '^[^@[:space:]]+@[^@[:space:]]+\.[^@[:space:]]+$'
                                                                            THEN 'malformed_email'
           WHEN d.password IS NULL OR btrim(d.password) = ''                 THEN 'missing_password'
           ELSE 'unrecognised_password_hash'
       END,
       to_jsonb(d)
FROM donors d
WHERE d.email IS NULL OR btrim(d.email) = ''
   OR lower(btrim(d.email)) !~ '^[^@[:space:]]+@[^@[:space:]]+\.[^@[:space:]]+$'
   OR d.password IS NULL OR btrim(d.password) = ''
   OR d.password !~ '^\$(2[aby]|argon2(i|d|id))\$';

-- Preserve the primary keys. GENERATED ALWAYS requires the explicit override,
-- which is itself a useful signal in the migration diff.
INSERT INTO users (id, email, password_hash, role, status, created_at, updated_at)
OVERRIDING SYSTEM VALUE
SELECT d.id, lower(btrim(d.email)), d.password, 'donor', 'active', now(), now()
FROM donors d
WHERE d.email IS NOT NULL AND btrim(d.email) <> ''
  AND lower(btrim(d.email)) ~ '^[^@[:space:]]+@[^@[:space:]]+\.[^@[:space:]]+$'
  AND d.password IS NOT NULL AND btrim(d.password) <> ''
  AND d.password ~ '^\$(2[aby]|argon2(i|d|id))\$';

SELECT setval(pg_get_serial_sequence('users','id'),
              (SELECT COALESCE(max(id), 0) + 1 FROM users), false);

INSERT INTO donor_profiles (user_id, full_name, date_of_birth, gender, blood_group, rhesus,
                            contact_phone, address_line, legacy_last_donation)
SELECT u.id,
       COALESCE(NULLIF(btrim(d.full_name), ''), 'Unknown donor ' || d.id),
       COALESCE(d.dob, DATE '1900-01-02'),                     -- sentinel; flagged for follow-up
       CASE lower(btrim(COALESCE(d.gender,'')))
            WHEN 'male'   THEN 'male'::gender   WHEN 'm' THEN 'male'::gender
            WHEN 'female' THEN 'female'::gender WHEN 'f' THEN 'female'::gender
            WHEN 'other'  THEN 'other'::gender  ELSE 'undisclosed'::gender END,
       CASE upper(btrim(COALESCE(d.blood_group,'')))
            WHEN 'A'  THEN 'A'::blood_group  WHEN 'B' THEN 'B'::blood_group
            WHEN 'AB' THEN 'AB'::blood_group WHEN 'O' THEN 'O'::blood_group
            ELSE NULL END,
       CASE WHEN upper(btrim(COALESCE(d.blood_group,''))) NOT IN ('A','B','AB','O') THEN NULL
            WHEN btrim(COALESCE(d.rhesus,'')) IN ('+','positive','POSITIVE','Positive') THEN 'positive'::rhesus
            WHEN btrim(COALESCE(d.rhesus,'')) IN ('-','negative','NEGATIVE','Negative') THEN 'negative'::rhesus
            ELSE NULL END,
       COALESCE(NULLIF(btrim(d.contact), ''), 'UNKNOWN'),
       d.address,
       d.last_donation
FROM donors d
JOIN users u ON u.id = d.id;

-- A group that parsed but a rhesus that did not must lose both (paired CHECK).
UPDATE donor_profiles SET blood_group = NULL WHERE rhesus IS NULL AND blood_group IS NOT NULL;
